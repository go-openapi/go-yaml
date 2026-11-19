// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// errNeedsTheTree says a document or a destination this walk cannot serve.
// The decode reads the source again and builds a tree for it.
//
// SPIKE. It falls back on tags, explicit "?" keys, an anchor or an alias used
// as a key or inside an "any", and on
// any destination holding an interface, a custom unmarshaler, an ast.Node, a
// non-string map key, or a Go kind the setter below does not write.
var errNeedsTheTree = errors.New("this decode needs the tree")

type typedFrameKind uint8

const (
	typedStruct typedFrameKind = iota
	typedMap
	typedSlice
	// typedAnchor stands for an anchor around one node. It holds the
	// destination the node writes into, read once as the anchor opens, and
	// passes the node's value on to the collection around it as it closes. An
	// alias decoded from its anchor's node uses one too, with no name.
	typedAnchor
)

// typedAnchored is what an anchor of the document named, for an alias to name
// again: a clone of the node, which the parse reuses once the anchor's Leave
// returns, and the value the walk wrote for it.
type typedAnchored struct {
	node  ast.Node
	value reflect.Value
}

type typedFrame struct {
	kind typedFrameKind

	// dst is the collection being filled, with the pointers to it followed.
	dst reflect.Value
	// out is what the collection around this one is handed: dst again, unless
	// pointers stood in front of it and out is the outermost.
	out reflect.Value

	// target is where the entry being read goes. For a struct it is the field
	// the key named; for a map and a slice it is a fresh element.
	target reflect.Value
	// key holds a map's key until its value arrives.
	key reflect.Value
	// skip says no field claims the entry being read, so its value goes
	// nowhere. dropped says the whole collection goes nowhere, so nothing in it
	// is looked up at all.
	skip    bool
	dropped bool
	// got says the node under an anchor has settled, into out. name is the
	// anchor's, and empty for the frame an alias decodes into.
	got  bool
	name string

	// written marks the fields of a struct the mapping wrote itself, by their
	// ord, so a "<<" applied at the close skips them. mergeNext says the key
	// just read was a "<<", so the node that follows is what it merges, and
	// merges holds those nodes until the mapping closes.
	written   writtenFields
	mergeNext bool
	merges    []ast.Node
	// wrote records the keys a mapping writes into a map that held the
	// caller's entries when it opened, so a merge tells the two apart. It is
	// nil for a map that opened empty, where every key it holds was written
	// here.
	wrote map[string]struct{}
}

// typedBuilder fills a Go value as the parse hands the document over, without
// a tree in between.
//
// It holds no Decoder: a walkStream runs it inside a coroutine, which must not
// keep the Decoder alive. See walkStream.
type typedBuilder struct {
	// mode is the Decoder's tagMode, which names the fields of a struct.
	mode  tagMode
	root  reflect.Value
	stack []typedFrame
	strs  *arena
	err   error

	// values builds the subtree under a destination that holds an "any", where
	// the document says what the value is and the type does not. It is made
	// when the walk reaches one and dropped when that subtree closes; anyInto is
	// where the value it built goes.
	values  *valueBuilder
	anyInto reflect.Value
	share   bool
	// budget bounds the nodes decoded again for an alias over the whole
	// stream, and built counts them. See aliasBudget.
	budget int
	built  int
	// anchors holds what each anchor of the document named, and is emptied at
	// each document, since an anchor belongs to one. replaying counts the
	// aliases being decoded from their anchor's node, during which an anchor
	// inside that node is not recorded again.
	anchors   map[string]typedAnchored
	replaying int
	// declared holds the anchors parser.WithAnchors passed. Their trees come
	// from another parse and stay whole, so an alias decodes one as it decodes
	// a recorded clone. See named.
	declared map[string]ast.Node
	// keeping says the walk holds a "<<"'s mapping or sequence whole, for
	// Leave to clone it. pending counts the frames holding merges not yet
	// applied, during which an anchor named again goes to the tree: the
	// merges read their aliases by name as the mapping closes.
	keeping bool
	pending int
}

// holdsAnything reports a destination that takes whatever the document writes.
func holdsAnything(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.Type().NumMethod() == 0
}

func (b *typedBuilder) Enter(node ast.Node, at parser.Cursor) error {
	if _, isDirective := node.(*ast.DirectiveNode); isDirective && at.IsRoot() {
		// A "%YAML" or "%TAG" line is handed over as a root of its own, ahead
		// of the document it opens, and holds no value.
		return parser.ErrSkipNode
	}
	if b.values == nil && !at.IsKey() && len(b.stack) > 0 && b.stack[len(b.stack)-1].mergeNext {
		return b.mergeValue(&b.stack[len(b.stack)-1], node)
	}

	switch n := node.(type) {
	case *ast.AnchorNode:
		if b.values != nil || at.IsKey() {
			// Inside an "any" subtree, the nested builder would read the anchor
			// as its own and answer for the document's. An anchored key names
			// its entry through the anchor, which openEntry does not read yet.
			return b.fail(errNeedsTheTree)
		}

		return b.openAnchor(anchorName(n.Name))
	case *ast.AliasNode:
		if b.values != nil || at.IsKey() {
			// Inside an "any" subtree, see the anchor above. An alias as a key
			// names its entry through the anchor's node.
			return b.fail(errNeedsTheTree)
		}

		return b.alias(n)
	case *ast.TagNode, *ast.MappingKeyNode:
		// A property this walk does not read yet.
		return b.fail(errNeedsTheTree)
	case *ast.CommentGroupNode:
		return parser.ErrSkipNode
	}

	if b.values != nil {
		return b.forwardEnter(node, at)
	}

	// A key names the entry that follows rather than being a value, so it is
	// read before the destination is looked at.
	if at.IsKey() && len(b.stack) > 0 {
		return b.openEntry(&b.stack[len(b.stack)-1], node)
	}

	// Read where this goes once and hand it on: destination makes a sequence's
	// entry as it is asked for, so asking twice makes two.
	dst, wanted := b.destination()
	if wanted && holdsAnything(dst) {
		b.anyInto = dst
		b.values = &valueBuilder{share: b.share, budget: b.budget}

		return b.forwardEnter(node, at)
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		return b.openMapping(dst, wanted)
	case *ast.SequenceNode:
		return b.openSequence(dst, wanted)
	default:
		return b.scalar(n, dst, wanted)
	}
}

func (b *typedBuilder) Leave(node ast.Node, at parser.Closing) error {
	if b.err != nil {
		// The walk hands nothing more over once a visitor has failed, and still leaves the nodes it had open.
		return b.err
	}
	if b.values != nil {
		return b.forwardLeave(node, at)
	}
	if b.keeping {
		// Nothing inside a kept node is handed over, so this Leave is its own.
		b.keeping = false

		return b.keepMerge(&b.stack[len(b.stack)-1], node)
	}
	switch n := node.(type) {
	case *ast.MappingNode, *ast.SequenceNode:
	case *ast.AnchorNode:
		return b.closeAnchor(n)
	default:
		return nil
	}

	if _, isMapping := node.(*ast.MappingNode); isMapping {
		// The parser hangs the repeats on the mapping as it closes, so they are
		// read here rather than as it opened.
		if err := refuseDuplicateKeys(node); err != nil {
			return b.fail(err)
		}
		if err := b.applyMerges(&b.stack[len(b.stack)-1]); err != nil {
			return err
		}
	}

	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	if frame.dropped {
		b.settle(reflect.Value{})

		return nil
	}
	b.settle(frame.out)

	return nil
}

// forwardEnter and forwardLeave hand a node to the nested value builder and close the
// subtree once the builder has finished one. They are two methods because Leave receives
// a parser.Closing and Enter a parser.Cursor, which answers less.
func (b *typedBuilder) forwardEnter(node ast.Node, at parser.Cursor) error {
	return b.forwarded(b.values.Enter(node, at))
}

func (b *typedBuilder) forwardLeave(node ast.Node, at parser.Closing) error {
	return b.forwarded(b.values.Leave(node, at))
}

// forwarded reports what the nested builder made of the node it was handed.
func (b *typedBuilder) forwarded(err error) error {
	// Read before closeAny, which drops the builder once it has made a value.
	if b.values.err != nil {
		return b.fail(b.values.err)
	}
	b.closeAny()

	return err
}

// closeAny writes what the nested builder made into the destination that asked
// for it, once it has made one.
func (b *typedBuilder) closeAny() {
	if b.values == nil || len(b.values.stack) > 0 || len(b.values.docs) == 0 {
		return
	}

	built := b.values.docs[0]
	dst := b.anyInto
	b.values, b.anyInto = nil, reflect.Value{}

	if dst.IsValid() && dst.CanSet() {
		if built == nil {
			dst.Set(reflect.Zero(dst.Type()))
		} else {
			dst.Set(reflect.ValueOf(built))
		}
	}
	b.settle(dst)
}

// fail keeps the first error and returns it, so a caller can return it to the walk in one line.
func (b *typedBuilder) fail(err error) error {
	if b.err == nil {
		b.err = err
	}

	return b.err
}

// destination returns where the node being handed over goes, and false where it
// goes nowhere.
func (b *typedBuilder) destination() (reflect.Value, bool) {
	if len(b.stack) == 0 {
		return b.root, true
	}
	top := &b.stack[len(b.stack)-1]
	if top.skip || top.dropped {
		return reflect.Value{}, false
	}
	if top.kind == typedSlice && !top.target.IsValid() {
		// A sequence names nothing before its entries, so each one is made as
		// it arrives.
		top.target = reflect.New(top.dst.Type().Elem()).Elem()
	}
	if !top.target.IsValid() {
		return reflect.Value{}, false
	}

	return top.target, true
}

// settle hands a finished value to the collection around it.
func (b *typedBuilder) settle(v reflect.Value) {
	if len(b.stack) == 0 {
		return
	}
	top := &b.stack[len(b.stack)-1]
	switch {
	case top.skip, top.dropped:
	case top.kind == typedStruct:
		// The field was written in place.
	case top.kind == typedMap:
		if top.key.IsValid() && v.IsValid() {
			top.dst.SetMapIndex(top.key, v)
		}
		top.key = reflect.Value{}
	case top.kind == typedSlice:
		if v.IsValid() {
			top.dst.Set(reflect.Append(top.dst, v))
		}
	case top.kind == typedAnchor:
		top.out, top.got = v, true
	}
	top.target = reflect.Value{}
}

// openAnchor opens an anchor on the destination of the node it stands on.
//
// The destination is read here, once, and the node takes it from the frame: a
// sequence makes an element each time it is asked for one.
func (b *typedBuilder) openAnchor(name string) error {
	dst, wanted := b.destination()
	b.stack = append(b.stack, typedFrame{kind: typedAnchor, target: dst, skip: !wanted, name: name})

	return nil
}

// closeAnchor hands what the anchored node wrote to the collection around the
// anchor, and records the anchor for the aliases after it.
//
// The node is cloned here because the parse hands its cells out again once
// Leave returns, and an alias further on would read another part of the
// document through them. The clone costs the anchored subtrees and nothing
// else.
func (b *typedBuilder) closeAnchor(n *ast.AnchorNode) error {
	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	if frame.skip {
		b.record(frame.name, n, reflect.Value{})
		b.settle(reflect.Value{})

		return nil
	}
	if !frame.got {
		// The anchor stands on nothing, or on a node the walk does not hand
		// over on its own: "&a" alone, or "&a !!str 1".
		return b.fail(errNeedsTheTree)
	}
	b.record(frame.name, n, frame.out)
	b.settle(frame.out)

	return b.err
}

// record keeps what anchor n named. An anchor inside the node an alias is
// decoded from was recorded where the document wrote it, and stays as it was.
func (b *typedBuilder) record(name string, n *ast.AnchorNode, value reflect.Value) {
	if name == "" || b.replaying > 0 {
		return
	}
	_, again := b.anchors[name]
	if _, declared := b.declared[name]; (again || declared) && b.pending > 0 {
		// A merge waiting for its mapping to close would read the new one.
		_ = b.fail(errNeedsTheTree)

		return
	}
	if b.anchors == nil {
		b.anchors = map[string]typedAnchored{}
	}
	b.anchors[name] = typedAnchored{node: ast.Clone(n.Value), value: value}
}

// named returns what the anchor name stands for: the anchor the document wrote,
// or else one parser.WithAnchors declared, as the parse resolves an alias.
//
// A declared anchor carries no value. The tree decoder reads its node into each
// destination again, ShareAliases or not, and so does the walk. A name declared
// with a nil node is not one: the tree reports it.
func (b *typedBuilder) named(name string) (typedAnchored, bool) {
	if anchored, known := b.anchors[name]; known {
		return anchored, anchored.node != nil
	}
	if node := b.declared[name]; node != nil {
		return typedAnchored{node: node}, true
	}

	return typedAnchored{}, false
}

// alias writes what alias n names into the destination it stands at.
//
// Under ShareAliases it hands over the value the anchor wrote, wherever
// castToAssignableValue accepts it for the destination: the value itself, or a
// pointer to it for a pointer destination, so the caller holds one value under
// both names. Otherwise, and where the cast is refused, it decodes the anchor's
// node again into the destination, as the tree does, so two aliases of one
// anchor give two values.
//
// Where named finds no anchor -- the alias stands inside its own anchor, or
// names none -- the decode goes to the tree, which reads or refuses it.
func (b *typedBuilder) alias(n *ast.AliasNode) error {
	anchored, known := b.named(anchorName(n.Value))
	if !known {
		return b.fail(errNeedsTheTree)
	}

	dst, wanted := b.destination()
	if !wanted {
		b.settle(reflect.Value{})

		return nil
	}
	if b.share && anchored.value.IsValid() {
		if v, err := castToAssignableValue(anchored.value, dst.Type(), n); err == nil {
			dst.Set(v)
			b.settle(dst)

			return nil
		}
	}

	return b.replay(anchored.node, dst, n)
}

// replay decodes node, a clone an anchor recorded, into dst, handing its nodes
// to the builder in the order the parse hands a document's over.
func (b *typedBuilder) replay(node ast.Node, dst reflect.Value, at *ast.AliasNode) error {
	b.stack = append(b.stack, typedFrame{kind: typedAnchor, target: dst})
	b.replaying++
	err := b.replayNode(node, false, at)
	b.replaying--
	if err != nil {
		return err
	}

	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	if !frame.got {
		return b.fail(errNeedsTheTree)
	}
	b.settle(frame.out)

	return nil
}

// replayNode hands node and everything under it to the builder. at is the
// alias the node is decoded for, and counts against aliasBudget; a merge
// written in place passes nil and counts nothing, as the tree counts nothing.
func (b *typedBuilder) replayNode(node ast.Node, key bool, at *ast.AliasNode) error {
	if b.err != nil {
		return b.err
	}
	if at != nil {
		b.built++
		if b.budget > 0 && b.built > b.budget {
			return b.fail(yamlerrors.NewExcessiveAliasing(b.built, b.budget, at.GetToken()))
		}
	}

	cursor := replayCursor{key: key}
	switch err := b.Enter(node, cursor); {
	case errors.Is(err, parser.ErrSkipNode):
		return nil
	case errors.Is(err, parser.ErrKeepNode):
		// The node is whole already: hand it straight to Leave.
		return b.Leave(node, cursor)
	case err != nil:
		return err
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		for _, entry := range n.Values {
			if err := b.replayNode(entry.Key, true, at); err != nil {
				return err
			}
			if err := b.replayNode(entry.Value, false, at); err != nil {
				return err
			}
		}
	case *ast.SequenceNode:
		for _, value := range n.Values {
			if err := b.replayNode(value, false, at); err != nil {
				return err
			}
		}
	case *ast.AnchorNode:
		if err := b.replayNode(n.Value, false, at); err != nil {
			return err
		}
	case *ast.MappingValueNode:
		// A single entry standing where a mapping stands. The walk hands a
		// MappingNode over; a clone of one is the same, but the tree builds this
		// too, and nothing here turns it into a mapping.
		return b.fail(errNeedsTheTree)
	}

	return b.Leave(node, cursor)
}

// openMerge reads a "<<" key: the node after it names what the mapping merges.
//
// A struct with a `,inline,alias` field goes to the tree, which points that
// field at the anchor the "<<" names, so the encoder writes the alias back.
func (b *typedBuilder) openMerge(top *typedFrame) error {
	if top.kind == typedStruct {
		fields, err := structFields(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		if slices.ContainsFunc(fields.inline, func(sf *StructField) bool { return sf.IsAutoAlias }) {
			return b.fail(errNeedsTheTree)
		}
	}
	top.mergeNext, top.skip, top.target = true, false, reflect.Value{}

	return parser.ErrSkipNode
}

// mergeValue holds what a "<<" names until its mapping closes.
//
// The merged entries cannot be written as the "<<" is met. A mapping's own
// entries win over merged ones on either side of the "<<", and an own entry
// written after it would decode into a field the merge had already filled,
// keeping what the merge put in the parts the entry leaves out. The tree
// decoder reads the own entries first and the merges after, and so does
// applyMerges.
//
// An alias holds the clone its anchor recorded. A mapping or a sequence written
// in place is kept whole by the walk and cloned at its Leave, in keepMerge.
// Anything else -- a null, a scalar, an anchor or a tag on the merged node --
// goes to the tree, which reads or rejects it.
func (b *typedBuilder) mergeValue(top *typedFrame, node ast.Node) error {
	top.mergeNext = false
	switch n := node.(type) {
	case *ast.AliasNode:
		anchored, known := b.named(anchorName(n.Value))
		if !known {
			return b.fail(errNeedsTheTree)
		}
		b.holdMerge(top, anchored.node)

		return nil
	case *ast.MappingNode, *ast.SequenceNode:
		b.keeping = true

		return parser.ErrKeepNode
	default:
		return b.fail(errNeedsTheTree)
	}
}

// keepMerge clones the mapping or the sequence of mappings a "<<" writes in
// place, now the walk hands it over whole. The aliases of a sequence are read
// here, while each names the anchor written before it.
func (b *typedBuilder) keepMerge(top *typedFrame, node ast.Node) error {
	seq, isSeq := node.(*ast.SequenceNode)
	if !isSeq {
		if err := refuseDuplicateKeys(node); err != nil {
			return b.fail(err)
		}
		b.holdMerge(top, ast.Clone(node))

		return nil
	}
	for _, value := range seq.Values {
		switch v := value.(type) {
		case *ast.AliasNode:
			anchored, known := b.named(anchorName(v.Value))
			if !known {
				return b.fail(errNeedsTheTree)
			}
			b.holdMerge(top, anchored.node)
		case *ast.MappingNode:
			if err := refuseDuplicateKeys(v); err != nil {
				return b.fail(err)
			}
			b.holdMerge(top, ast.Clone(v))
		default:
			return b.fail(errNeedsTheTree)
		}
	}

	return nil
}

func (b *typedBuilder) holdMerge(top *typedFrame, node ast.Node) {
	if len(top.merges) == 0 {
		b.pending++
	}
	top.merges = append(top.merges, node)
}

// applyMerges writes what the mapping on top of the stack merges, once its own
// entries are in: each "<<" in the order the document wrote them, each entry
// only where neither the mapping nor an earlier merge wrote one.
func (b *typedBuilder) applyMerges(top *typedFrame) error {
	if len(top.merges) == 0 {
		return nil
	}
	merges := top.merges
	top.merges = nil
	b.pending--

	// Decoding a merged value pushes frames, which can move the stack, so the
	// frame is found again by its index.
	at := len(b.stack) - 1
	for _, node := range merges {
		if err := b.mergeFrom(at, node, nil); err != nil {
			return err
		}
	}

	return nil
}

// mergeFrom writes the entries of node, a mapping, into the frame at index i:
// its own entries first, then what its own "<<" merge, as rangeMergedEntries
// orders them on the tree. alias is the alias node was reached through, for
// aliasBudget to count, and nil for a mapping written in place.
func (b *typedBuilder) mergeFrom(i int, node ast.Node, alias *ast.AliasNode) error {
	switch n := node.(type) {
	case *ast.MappingNode:
		var nested []ast.Node
		for _, entry := range n.Values {
			if entry.Key != nil && entry.Key.IsMergeKey() {
				nested = append(nested, entry.Value)

				continue
			}
			if err := b.mergeEntry(i, entry.Key, entry.Value, alias); err != nil {
				return err
			}
		}
		for _, value := range nested {
			if err := b.mergeNested(i, value, alias); err != nil {
				return err
			}
		}

		return nil
	case *ast.AliasNode:
		anchored, known := b.named(anchorName(n.Value))
		if !known {
			return b.fail(errNeedsTheTree)
		}
		if _, isMapping := anchored.node.(*ast.MappingNode); !isMapping {
			return b.fail(errNeedsTheTree)
		}

		return b.mergeFrom(i, anchored.node, n)
	default:
		return b.fail(errNeedsTheTree)
	}
}

// mergeNested writes what a "<<" inside a merged mapping names: a mapping, an
// alias of one, or a sequence of either.
func (b *typedBuilder) mergeNested(i int, value ast.Node, alias *ast.AliasNode) error {
	seq, isSeq := value.(*ast.SequenceNode)
	if !isSeq {
		return b.mergeFrom(i, value, alias)
	}
	for _, one := range seq.Values {
		if _, isSeq := one.(*ast.SequenceNode); isSeq {
			return b.fail(errNeedsTheTree)
		}
		if err := b.mergeFrom(i, one, alias); err != nil {
			return err
		}
	}

	return nil
}

// mergeEntry writes one merged entry into the frame at index i, unless the
// mapping or an earlier merge wrote it. A struct skips a field its written
// marks hold. A map skips a key it holds, or one its wrote set holds where it
// opened with the caller's entries, which a merged entry writes over.
func (b *typedBuilder) mergeEntry(i int, keyNode, value ast.Node, alias *ast.AliasNode) error {
	top := &b.stack[i]
	name, named := entryText(keyNode)
	if !named {
		if top.kind == typedMap {
			return b.fail(errNeedsTheTree)
		}

		return nil
	}

	switch top.kind {
	case typedStruct:
		fields, err := structFields(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		ff, known := fields.lookup(name)
		if !known || top.written.has(ff.ord) {
			return nil
		}
		top.written.mark(ff.ord)
		field := top.dst.Field(ff.sf.Index)
		if ff.at != nil {
			if field, err = fieldAt(top.dst, ff.at); err != nil {
				return b.fail(err)
			}
		}
		top.skip, top.target = false, field
	case typedMap:
		name = b.strs.clone(name)
		key := reflect.ValueOf(name).Convert(top.dst.Type().Key())
		if top.wrote != nil {
			if _, wrote := top.wrote[name]; wrote {
				return nil
			}
			top.wrote[name] = struct{}{}
		} else if top.dst.MapIndex(key).IsValid() {
			return nil
		}
		top.skip, top.key = false, key
		top.target = reflect.New(top.dst.Type().Elem()).Elem()
	default:
		return b.fail(errNeedsTheTree)
	}

	return b.replayNode(value, false, alias)
}

// replayCursor answers for the nodes replay hands over. typedBuilder reads
// IsKey, and the valueBuilder of an "any" inside the node reads IsKey and
// IsRoot; none of the nodes is a document's root.
type replayCursor struct {
	key bool
}

func (c replayCursor) Depth() int           { return 1 }
func (c replayCursor) In() parser.Kind      { return parser.KindNone }
func (c replayCursor) IsRoot() bool         { return false }
func (c replayCursor) IsKey() bool          { return c.key }
func (c replayCursor) Document() int        { return 0 }
func (c replayCursor) HoldsKey(string) bool { return false }

func (b *typedBuilder) openMapping(dst reflect.Value, wanted bool) error {
	if !wanted {
		// Inside an entry no field claims. Read the mapping and drop it.
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dropped: true})

		return nil
	}

	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		return b.fail(errNeedsTheTree)
	}
	switch dst.Kind() {
	case reflect.Struct:
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dst: dst, out: out})

		return nil
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			return b.fail(errNeedsTheTree)
		}
		if dst.IsNil() {
			if !dst.CanSet() {
				return b.fail(errNeedsTheTree)
			}
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		frame := typedFrame{kind: typedMap, dst: dst, out: out}
		if dst.Len() > 0 {
			frame.wrote = map[string]struct{}{}
		}
		b.stack = append(b.stack, frame)

		return nil
	default:
		return b.fail(errNeedsTheTree)
	}
}

func (b *typedBuilder) openSequence(dst reflect.Value, wanted bool) error {
	if !wanted {
		b.stack = append(b.stack, typedFrame{kind: typedSlice, dropped: true})

		return nil
	}

	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		return b.fail(errNeedsTheTree)
	}
	if dst.Kind() != reflect.Slice || !dst.CanSet() {
		return b.fail(errNeedsTheTree)
	}
	// An empty sequence is an empty slice and not a nil one, which is what the
	// tree decoder gives and what a caller comparing against "[]T{}" expects.
	if dst.IsNil() {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
	} else {
		dst.Set(dst.Slice(0, 0))
	}
	b.stack = append(b.stack, typedFrame{kind: typedSlice, dst: dst, out: out})

	return nil
}

// indirect follows and allocates the pointers between a destination and the
// value a document writes into it.
func (b *typedBuilder) indirect(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if !v.CanSet() {
				return v
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	return v
}

// scalar reads a leaf: a mapping's key names the entry that follows, and
// anything else is a value to write.
func (b *typedBuilder) scalar(node ast.Node, dst reflect.Value, wanted bool) error {
	if !wanted {
		b.settle(reflect.Value{})

		return parser.ErrSkipNode
	}
	if err := b.setScalar(dst, node); err != nil {
		return b.fail(err)
	}
	b.settle(dst)

	// A leaf holds nothing to descend into, and Leave has nothing to close.
	return parser.ErrSkipNode
}

// fieldAt walks the index path an embedded field stands at, making the pointers
// between as it goes. reflect.Value.FieldByIndex panics on a nil one and
// FieldByIndexErr refuses rather than filling it, and a document writing through
// an embedded pointer expects it filled.
func fieldAt(v reflect.Value, at []int) (reflect.Value, error) {
	for i, step := range at {
		if i > 0 {
			for v.Kind() == reflect.Pointer {
				if v.IsNil() {
					if !v.CanSet() {
						return reflect.Value{}, errNeedsTheTree
					}
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
			if v.Kind() != reflect.Struct {
				return reflect.Value{}, errNeedsTheTree
			}
		}
		v = v.Field(step)
	}

	return v, nil
}

// entryText reads the name a mapping key addresses its entry by, in the type's
// own canonical spelling: "true: x" addresses a field tagged "true" and
// "1.0: x" one tagged "1.0", which is how the same document reads into a
// map[string]any. keyName holds the rule, so the walk, the tree decoder and
// ToJSON name an entry the same way.
func entryText(n ast.Node) (string, bool) {
	name, kind := ast.KeyName(n)

	return name, kind != token.KeyOther
}

// openEntry names the entry a key opens, and says where its value goes.
func (b *typedBuilder) openEntry(top *typedFrame, keyNode ast.Node) error {
	if top.dropped {
		return parser.ErrSkipNode
	}

	if key, isKey := keyNode.(ast.MapKeyNode); isKey && key.IsMergeKey() {
		return b.openMerge(top)
	}

	name, named := entryText(keyNode)
	if !named {
		if top.kind == typedMap {
			return b.fail(errNeedsTheTree)
		}
		// No field can be named after it.
		top.skip = true

		return parser.ErrSkipNode
	}

	switch top.kind {
	case typedStruct:
		fields, err := structFields(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		ff, known := fields.lookup(name)
		if !known {
			top.skip, top.target = true, reflect.Value{}

			return parser.ErrSkipNode
		}
		top.written.mark(ff.ord)
		if ff.at != nil {
			// The type embeds another, and the name reaches a field of it.
			field, err := fieldAt(top.dst, ff.at)
			if err != nil {
				return b.fail(err)
			}
			top.skip, top.target = false, field

			return parser.ErrSkipNode
		}
		top.skip = false
		top.target = top.dst.Field(ff.sf.Index)
	case typedMap:
		top.skip = false
		name = b.strs.clone(name)
		if top.wrote != nil {
			top.wrote[name] = struct{}{}
		}
		top.key = reflect.ValueOf(name).Convert(top.dst.Type().Key())
		top.target = reflect.New(top.dst.Type().Elem()).Elem()
	case typedSlice:
		return b.fail(errNeedsTheTree)
	default:
		panic(fmt.Sprintf("codec: unknown typed frame kind %d", top.kind))
	}

	// A key names the entry and holds nothing the walk descends into.
	return parser.ErrSkipNode
}

func scalarNumber(node ast.Node) any {
	switch n := node.(type) {
	case *ast.IntegerNode:
		return n.GetValue()
	case *ast.FloatNode:
		return n.GetValue()
	default:
		return nil
	}
}

func (b *typedBuilder) setScalar(dst reflect.Value, node ast.Node) error {
	if _, isNull := node.(*ast.NullNode); isNull {
		// A null leaves the destination as it stands, which is what the tree
		// decoder does: it builds a zero value and then writes the default over
		// it. "nested:" with nothing under it keeps the defaults the caller set.
		return nil
	}

	dst = b.indirect(dst)
	if !dst.CanSet() || b.handled(dst) {
		return errNeedsTheTree
	}

	switch dst.Kind() {
	case reflect.String:
		s, isText := node.(*ast.StringNode)
		if !isText {
			return errNeedsTheTree
		}
		dst.SetString(b.strs.clone(s.Value))

		return nil
	case reflect.Bool:
		v, isBool := node.(*ast.BoolNode)
		if !isBool {
			return errNeedsTheTree
		}
		dst.SetBool(v.Value)

		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := scalarNumber(node).(type) {
		case int64:
			if dst.OverflowInt(v) {
				return errNeedsTheTree
			}
			dst.SetInt(v)

			return nil
		case uint64:
			if v > 1<<63-1 || dst.OverflowInt(int64(v)) {
				return errNeedsTheTree
			}
			dst.SetInt(int64(v))

			return nil
		}

		return errNeedsTheTree
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := scalarNumber(node).(type) {
		case uint64:
			if dst.OverflowUint(v) {
				return errNeedsTheTree
			}
			dst.SetUint(v)

			return nil
		case int64:
			if v < 0 || dst.OverflowUint(uint64(v)) {
				return errNeedsTheTree
			}
			dst.SetUint(uint64(v))

			return nil
		}

		return errNeedsTheTree
	case reflect.Float32, reflect.Float64:
		switch v := scalarNumber(node).(type) {
		case float64:
			dst.SetFloat(v)

			return nil
		case int64:
			dst.SetFloat(float64(v))

			return nil
		case uint64:
			dst.SetFloat(float64(v))

			return nil
		}

		return errNeedsTheTree
	default:
		return errNeedsTheTree
	}
}

// handled reports a destination the tree decoder reads for itself: a custom
// unmarshaler, a time.Time or time.Duration, a MapSlice. This walk writes none
// of them.
func (b *typedBuilder) handled(dst reflect.Value) bool {
	if !dst.CanAddr() {
		return true
	}
	// canWalk turns the walk away from a Decoder holding a custom
	// unmarshaler or UseJSONUnmarshaler, so neither applies here.
	if decodesItself(dst.Addr(), false) {
		return true
	}
	switch dst.Interface().(type) {
	case MapSlice, MapSliceSeq, MapItem:

		return true
	}

	return false
}

// walkableTypes caches which destination types the typed walk can fill, keyed
// by reflect.Type, one map per tagMode: the fields a type shows depend on the
// mode, and so does whether the walk can fill it.
var walkableTypes [modeCount]typeCache[bool]

// walkableType reports whether a destination of type t can be filled by the
// walk, looking at the Go type alone.
//
// Asking before the parse is the point. A walk that gives up halfway has read
// the document for nothing and the decode reads it again to build a tree, so a
// destination the walk was never going to fill costs two parses instead of one.
// Every reason it could give up on the type rather than on the document is
// therefore settled here, once per type.
func walkableType(t reflect.Type, mode tagMode) bool {
	cache := &walkableTypes[mode]
	if cached, known := cache.Load(t); known {
		return cached
	}
	ok := readWalkable(t, map[reflect.Type]bool{}, mode)
	cache.Store(t, ok)

	return ok
}

var (
	unmarshalerTypes = []reflect.Type{
		reflect.TypeFor[Unmarshaler](),
		reflect.TypeFor[GoYAMLUnmarshaler](),
		reflect.TypeFor[NodeUnmarshaler](),
		reflect.TypeFor[encoding.TextUnmarshaler](),
		reflect.TypeFor[ast.Node](),
	}
	readByTheTree = []reflect.Type{
		reflect.TypeFor[time.Time](),
		reflect.TypeFor[time.Duration](),
		reflect.TypeFor[MapSlice](),
		reflect.TypeFor[MapSliceSeq](),
		reflect.TypeFor[MapItem](),
		reflect.TypeFor[RawMessage](),
	}
)

func readWalkable(t reflect.Type, seen map[reflect.Type]bool, mode tagMode) bool {
	if seen[t] {
		// A type that holds itself. The cycle says nothing either way, and the
		// document's depth is capped elsewhere.
		return true
	}
	seen[t] = true

	if slices.Contains(readByTheTree, t) {
		return false
	}
	pointer := reflect.PointerTo(t)
	for _, iface := range unmarshalerTypes {
		if t.Implements(iface) || pointer.Implements(iface) {
			return false
		}
	}

	switch t.Kind() {
	case reflect.Pointer:
		return readWalkable(t.Elem(), seen, mode)
	case reflect.Struct:
		fields, err := structFields(t, mode)
		if err != nil {
			return false
		}
		for _, sf := range fields.fields {
			ft := t.Field(sf.Index).Type
			if sf.IsInline && inlineTakesUnclaimedEntries(ft) {
				// The walk meets an entry once and has to place it then,
				// through an index path readFields.flat names. A map has no
				// such name, so the tree decoder reads this struct.
				return false
			}
			if !readWalkable(ft, seen, mode) {
				return false
			}
		}

		return true
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return false
		}

		return readWalkable(t.Elem(), seen, mode)
	case reflect.Slice:
		return readWalkable(t.Elem(), seen, mode)
	case reflect.Interface:
		// An "any" holds whatever the document writes, which valueBuilder
		// builds. Anything narrower needs a type this walk cannot pick.
		return t.NumMethod() == 0
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		// An interface, an array, a channel, a function, a complex number.
		return false
	}
}

// reset readies b to fill root with the next document, keeping the room its
// stack has grown.
func (b *typedBuilder) reset(root reflect.Value) {
	b.root = root
	b.stack = b.stack[:0]
	b.err = nil
	b.values, b.anyInto = nil, reflect.Value{}
	clear(b.anchors)
	b.keeping, b.pending = false, 0
}

// typedWalkEnabled turns the spike on. SPIKE.
const typedWalkEnabled = true
