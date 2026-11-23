// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-openapi/go-yaml/codec/internal/structfields"

	"github.com/go-openapi/go-yaml/codec/internal/astutils"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// errNeedsTheTree reports a document or a destination that the walk cannot fill.
//
// The walk stops at the first such node, and decodeWalking parses the whole source
// again into a tree. The tree decodes the current document from its start, over
// the values written by the walk, and serves every later Decode of the Decoder.
//
// A fallback costs the walk up to that node, then a tree decode of the source.
// citm_catalog (0.56 MiB) into its Go types peaks at 1.30 MiB walked and
// 20.07 MiB on the tree (peakLive, max of 5 runs).
// canWalk and walkableType choose the tree before any walk when an option or
// the Go type alone decides. Those decodes cost the tree and no more.
//
// The walk falls back on these documents:
//
//   - an anchor or an alias inside an "any" field, which valueBuilder would
//     record as its own;
//   - an anchor or an alias used as a key, or inside a "?" or a tagged key;
//   - an alias with no anchor recorded by the walk: one inside its own anchor,
//     one naming no anchor, one naming an anchor declared by parser.WithAnchors
//     with a nil node;
//   - a tagged node, or a collection of another kind than its field, when it
//     holds an anchor or an alias. The walk passes such a node alone to the
//     tree decoder;
//   - a "<<" into a struct with an ",inline,alias" field;
//   - a "<<" naming anything but a mapping, an alias to one, or a sequence of
//     those: a null, a scalar, an anchored or tagged node. The tree returns
//     the error;
//   - an anchor named again while a "<<" waits for its mapping to close;
//   - a collection used as the key of a map.
//
// It falls back on a destination it cannot set, such as an embedded pointer to
// an unexported struct.
//
// A value of another kind than its field does not fall back. The walk passes
// that one node to the tree decoder, which converts it (a number into a
// string) or rejects it. A rejected value is written as far as it decoded.
// The walk keeps the first error for the document to return, and decodes the
// fields after it, as the tree does. See decodeNode.
//
// Nor does a field the tree decoder reads by rules of its own: a time.Time, a
// time.Duration, a MapSlice, a MapSliceSeq, a MapItem, a yaml.Node of
// go.yaml.in/yaml/v3, and a type that decodes itself from text or through a
// value (Unmarshaler, GoYAMLUnmarshaler, encoding.TextUnmarshaler, v3's
// UnmarshalYAML(*yaml.Node), RegisterCustomUnmarshaler). The walk keeps that
// field's node whole and passes it to the tree decoder alone. Such a node
// holding an anchor or an alias falls back, as a tagged node does.
//
// walkableType sends a decode to the tree before the walk for a destination
// handed the node itself, which it may keep: a NodeUnmarshaler or an ast.Node
// field. The walk recycles its nodes after their Leave. So does a RawMessage,
// whose source text the walk drops, and a ",inline" field that decodes itself,
// which reads the whole mapping.
//
// The remaining cases guard against shapes that no document has produced so
// far: an anchor with no node under it, and a lone entry
// (an ast.MappingValueNode) replayed for an alias.
var errNeedsTheTree = errors.New("this decode needs the tree")

// errNotWritten reports a scalar that setScalar does not write itself, because
// the field is of another kind than the node. scalar passes it to convertScalar.
var errNotWritten = errors.New("this scalar needs a conversion")

type typedFrameKind uint8

const (
	typedStruct typedFrameKind = iota
	typedMap
	typedSlice
	// typedAnchor marks an anchor around one node. Its frame holds the
	// destination of the node, read once as the anchor opens. As the anchor
	// closes, the frame passes the node's value on to the enclosing collection.
	// An alias decoded from its anchor's node also uses one, with no name.
	typedAnchor
)

// typedAnchored records what an anchor of the document named, for a later
// alias. node holds a clone of the anchored node, because the parse reuses the
// original once the anchor's Leave returns. value holds the value written by
// the walk for it.
type typedAnchored struct {
	node  ast.Node
	value reflect.Value
}

type typedFrame struct {
	kind typedFrameKind

	// dst holds the collection being filled, after following the pointers to it.
	dst reflect.Value
	// out holds the value passed to the enclosing collection. It is dst again,
	// unless pointers stood in front of dst: then out is the outermost pointer.
	out reflect.Value

	// target holds the destination of the entry being read. For a struct it is
	// the field named by the key; for a map and a slice it is a fresh element.
	target reflect.Value
	// key holds a map's key until its value arrives.
	key reflect.Value
	// skip marks an entry that no field claims. Its value goes nowhere.
	// dropped marks a whole collection that goes nowhere. The walk looks up
	// nothing inside it.
	skip    bool
	dropped bool
	// got records that the node under an anchor has settled into out. name holds
	// the anchor's name, and is empty for the frame of an alias.
	got  bool
	name string

	// written marks, by ord, the struct fields written by the mapping itself.
	// A "<<" applied at the close skips them. mergeNext records that the key
	// just read was a "<<": the node that follows holds the merged entries.
	// merges holds those nodes until the mapping closes.
	written   writtenFields
	mergeNext bool
	merges    []ast.Node
	// field holds the struct field written by the entry being read, for
	// fieldError to name.
	field structfields.Flat
	// wrote records the keys written by a mapping into a map that already held
	// the caller's entries when it opened. A merge uses it to tell the two apart.
	// It is nil for a map that opened empty, where the walk wrote every key.
	wrote map[string]struct{}
	// unknown holds the first key of a struct's mapping that names no field, own or merged, under
	// DisallowUnknownField. fieldFailed records a field error inside the struct, which the tree returns
	// in place of the unknown key. See closeStruct.
	unknown     *token.Token
	fieldFailed bool

	// place holds what a validator error needs, and is nil unless a validator is set. See walkPlaces.
	place *framePlaces
}

// framePlaces records, for one struct or map frame, the tokens that place a validator error, as the tree reads
// them from the mapping again. Every token is a copy, because the walk recycles its tokens.
type framePlaces struct {
	// key holds the key token of the entry being read, and pending the name of the field its value goes to.
	key        *token.Token
	pending    string
	hasPending bool
	// fields holds, by name, the token of the first entry that writes each field, own entries before merged ones.
	fields map[string]*token.Token
	// firstKey holds the token of the mapping's first key, entryKey the key of the entry that writes the
	// struct, nil for a sequence element or a document's root, and whole the token of the mapping.
	firstKey, entryKey, whole *token.Token
}

// typedBuilder fills a Go value as the parse passes the document over, without
// a tree in between.
//
// It holds no Decoder: a walkStream runs it inside a coroutine, which must not
// keep the Decoder alive. See walkStream.
type typedBuilder struct {
	// mode holds the Decoder's Mode, which sets how struct fields are named.
	mode structfields.Mode
	// strict holds DisallowUnknownField, and allowedPrefixes the prefixes of AllowFieldPrefixes, which let an
	// unknown key through.
	strict          bool
	allowedPrefixes []string
	root            reflect.Value
	stack           []typedFrame
	strs            *codecutils.Arena
	err             error

	// values builds the subtree under a destination of type "any", where the
	// document decides the value instead of the Go type. The walk creates it on
	// reaching such a destination and drops it when that subtree closes.
	// anyInto holds the destination of the built value.
	values  *valueBuilder
	anyInto reflect.Value
	share   bool
	// validator holds the function of codec.Validator, nil when unset. closeStruct runs it on each struct.
	validator func(any) error
	// stringKeys passes codec.UseStringKeys to values, and ordered codec.UseOrderedMap.
	stringKeys bool
	ordered    bool
	// budget bounds the nodes decoded again for an alias over the whole
	// stream, and built counts them. See codecutils.AliasBudget.
	budget int
	built  int
	// anchors holds what each anchor of the document named. reset empties it at
	// each document, because an anchor belongs to one document. replaying counts
	// the aliases being decoded from their anchor's node. While it is positive,
	// record skips an anchor inside that node.
	anchors   map[string]typedAnchored
	replaying int
	// declared holds the anchors passed by parser.WithAnchors. Their trees come
	// from another parse and stay whole, so an alias decodes one as it decodes
	// a recorded clone. See named.
	declared map[string]ast.Node
	// keeping records that the walk holds the mapping or sequence of a "<<"
	// whole, for Leave to clone. pending counts the frames holding merges not
	// yet applied. While it is positive, an anchor named again sends the decode
	// to the tree, because the merges read their aliases by name as the mapping
	// closes.
	keeping bool
	pending int
	// decoding records that the walk holds a node whole. Leave decodes it into
	// decodeInto with nodes, a tree decoder made from opts the first time one
	// is needed. See keepWhole.
	decoding   bool
	decodeInto reflect.Value
	// docErr holds the first field error of the document. The document returns
	// it once it is read. See fieldError.
	docErr error
	opts   []DecodeOption
	nodes  *Decoder
	// schema is the schema of the document being walked, which the node decoder reads a kept node under.
	schema token.Schema
	// naming records that the walk holds a key whole, an explicit "?" or a
	// tagged key, for Leave to name the entry by. See keepKey.
	naming bool
}

func (b *typedBuilder) Enter(node ast.Node, at parser.Cursor) error {
	b.schema = at.Schema()
	if _, isDirective := node.(*ast.DirectiveNode); isDirective && at.IsRoot() {
		// The parse passes a "%YAML" or "%TAG" line as a root of its own, ahead
		// of the document it opens. It holds no value.
		return parser.ErrSkipNode
	}
	if b.values == nil && !at.IsKey() && len(b.stack) > 0 {
		b.placeValue(&b.stack[len(b.stack)-1], node)
	}
	if b.values == nil && !at.IsKey() && len(b.stack) > 0 && b.stack[len(b.stack)-1].mergeNext {
		return b.mergeValue(&b.stack[len(b.stack)-1], node)
	}

	switch n := node.(type) {
	case *ast.AnchorNode:
		if b.values != nil || at.IsKey() {
			// Inside an "any" subtree, the nested builder would record the anchor
			// as its own instead of the document's. An anchored key names its
			// entry through the anchor, and openEntry does not read anchors yet.
			return b.fail(errNeedsTheTree)
		}

		return b.openAnchor(astutils.AnchorName(n.Name))
	case *ast.AliasNode:
		if b.values != nil || at.IsKey() {
			// Inside an "any" subtree, see the anchor above. An alias as a key
			// names its entry through the anchor's node.
			return b.fail(errNeedsTheTree)
		}

		return b.alias(n)
	case *ast.TagNode:
		if b.values == nil && at.IsKey() && len(b.stack) > 0 {
			return b.keepKey(&b.stack[len(b.stack)-1])
		}
	case *ast.MappingKeyNode:
		if b.values == nil {
			if !at.IsKey() || len(b.stack) == 0 {
				return b.fail(errNeedsTheTree)
			}

			return b.keepKey(&b.stack[len(b.stack)-1])
		}
	case *ast.CommentGroupNode:
		return parser.ErrSkipNode
	}

	if b.values != nil {
		return b.forwardEnter(node, at)
	}

	// A key names the entry that follows and holds no value. The walk reads
	// it before looking up the destination.
	if at.IsKey() && len(b.stack) > 0 {
		return b.openEntry(&b.stack[len(b.stack)-1], node)
	}

	// Read the destination once and pass it on. destination makes a new
	// sequence entry on each call, so two calls make two entries.
	dst, wanted := b.destination()
	if wanted && codecutils.HoldsAnything(dst) {
		b.anyInto = dst
		b.values = &valueBuilder{share: b.share, budget: b.budget, stringKeys: b.stringKeys, ordered: b.ordered}

		return b.forwardEnter(node, at)
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		return b.openMapping(dst, wanted, node)
	case *ast.SequenceNode:
		return b.openSequence(dst, wanted)
	case *ast.TagNode:
		return b.keepWhole(dst, wanted)
	default:
		return b.scalar(n, dst, wanted)
	}
}

func (b *typedBuilder) Leave(node ast.Node, at parser.Closing) error {
	if b.err != nil {
		// After a visitor fails, the walk passes no more nodes, but still leaves the nodes it had open.
		return b.err
	}
	if b.values != nil {
		return b.forwardLeave(node, at)
	}
	if b.decoding {
		// The walk passes nothing inside a kept node, so this Leave closes the kept node.
		b.decoding = false

		return b.decodeKept(node)
	}
	if b.naming {
		// The walk passes nothing inside a kept node, so this Leave closes the kept node.
		b.naming = false

		return b.openKeptEntry(node)
	}
	if b.keeping {
		// The walk passes nothing inside a kept node, so this Leave closes the kept node.
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
		// The parser attaches the repeated keys to the mapping as it closes, so
		// the walk reads them here and not as the mapping opens. A struct and a
		// string-keyed map key their entries by name. A dropped mapping holds nothing.
		if err := astutils.RefuseRepeats(node, !b.stack[len(b.stack)-1].dropped); err != nil {
			return b.fail(err)
		}
		if err := b.applyMerges(&b.stack[len(b.stack)-1]); err != nil {
			return err
		}
		if err := b.closeStruct(&b.stack[len(b.stack)-1]); err != nil {
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

// forwardEnter and forwardLeave pass a node to the nested value builder, and close the
// subtree once the builder has finished one. They are two methods because Leave receives
// a parser.Closing and Enter a parser.Cursor, which lacks HoldsKey.
func (b *typedBuilder) forwardEnter(node ast.Node, at parser.Cursor) error {
	return b.forwarded(b.values.Enter(node, at))
}

func (b *typedBuilder) forwardLeave(node ast.Node, at parser.Closing) error {
	return b.forwarded(b.values.Leave(node, at))
}

// forwarded returns the error of the nested builder for the node passed to it,
// and closes the subtree once the builder has built a value.
func (b *typedBuilder) forwarded(err error) error {
	// Read before closeAny, which drops the builder once it has made a value.
	// A failed builder has handed its open values up, so closeAny writes what it decoded before the error.
	if failed := b.values.err; failed != nil {
		b.closeAny()

		return b.fail(failed)
	}
	b.closeAny()

	return err
}

// closeAny writes the value built by the nested builder into anyInto, once the
// builder has built one.
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

// destination returns the destination of the current node, and false when the
// node goes nowhere.
func (b *typedBuilder) destination() (reflect.Value, bool) {
	if len(b.stack) == 0 {
		return b.root, true
	}
	top := &b.stack[len(b.stack)-1]
	if top.skip || top.dropped {
		return reflect.Value{}, false
	}
	if top.kind == typedSlice && !top.target.IsValid() {
		// A sequence names nothing before its entries, so destination makes
		// each one as it arrives.
		top.target = reflect.New(top.dst.Type().Elem()).Elem()
	}
	if !top.target.IsValid() {
		return reflect.Value{}, false
	}

	return top.target, true
}

// settle passes a finished value to the enclosing collection.
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

// openAnchor pushes an anchor frame on the destination of the anchored node.
//
// It reads the destination here, once, and the node takes it from the frame.
// A sequence makes a new element on each call to destination.
func (b *typedBuilder) openAnchor(name string) error {
	dst, wanted := b.destination()
	b.stack = append(b.stack, typedFrame{kind: typedAnchor, target: dst, skip: !wanted, name: name})

	return nil
}

// closeAnchor passes the value written by the anchored node to the enclosing
// collection, and records the anchor for later aliases.
//
// It clones the node because the parse reuses its cells once Leave returns.
// A later alias would read another part of the document through them.
// The clone costs the anchored subtrees and nothing else.
func (b *typedBuilder) closeAnchor(n *ast.AnchorNode) error {
	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	if frame.skip {
		b.record(frame.name, n, reflect.Value{})
		b.settle(reflect.Value{})

		return nil
	}
	if !frame.got {
		// The anchor stands on nothing, or on a node that the walk does not
		// pass on its own: "&a" alone.
		return b.fail(errNeedsTheTree)
	}
	b.record(frame.name, n, frame.out)
	b.settle(frame.out)

	return b.err
}

// record keeps what anchor n named. It skips an anchor inside a node replayed
// for an alias: the walk recorded that anchor where the document wrote it, and
// keeps that record.
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

// named returns the anchor called name: the one written in the document, or
// else one declared by parser.WithAnchors, in the order the parse resolves an alias.
//
// A declared anchor carries no value. The tree decoder reads its node into each
// destination again, ShareAliases or not, and so does the walk. named skips a
// name declared with a nil node, and the tree returns the error.
func (b *typedBuilder) named(name string) (typedAnchored, bool) {
	if anchored, known := b.anchors[name]; known {
		return anchored, anchored.node != nil
	}
	if node := b.declared[name]; node != nil {
		return typedAnchored{node: node}, true
	}

	return typedAnchored{}, false
}

// alias writes the value named by alias n into its destination.
//
// Under ShareAliases it passes the value written by the anchor, wherever
// codecutils.CastToAssignableValue accepts it for the destination: the value itself, or a
// pointer to it for a pointer destination. The caller then holds one value under
// both names. Otherwise, and where the cast fails, alias decodes the anchor's
// node again into the destination, as the tree does. Two aliases of one anchor
// then give two values.
//
// When named finds no anchor (the alias stands inside its own anchor, or
// names none), the decode goes to the tree, which reads or rejects it.
func (b *typedBuilder) alias(n *ast.AliasNode) error {
	anchored, known := b.named(astutils.AnchorName(n.Value))
	if !known {
		return b.fail(errNeedsTheTree)
	}

	dst, wanted := b.destination()
	if !wanted {
		b.settle(reflect.Value{})

		return nil
	}
	if b.share && anchored.value.IsValid() {
		if v, err := codecutils.CastToAssignableValue(anchored.value, dst.Type(), n); err == nil {
			dst.Set(v)
			b.settle(dst)

			return nil
		}
	}

	return b.replay(anchored.node, dst, n)
}

// replay decodes node, a clone recorded by an anchor, into dst. It passes the
// nodes to the builder in the order the parse passes a document's nodes.
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

// replayNode passes node and everything under it to the builder. at is the
// alias that node is decoded for, and counts against codecutils.AliasBudget.
// A merge written in place passes nil and counts nothing, as the tree counts nothing.
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

	cursor := replayCursor{key: key, schema: b.schema}
	switch err := b.Enter(node, cursor); {
	case errors.Is(err, parser.ErrSkipNode):
		return nil
	case errors.Is(err, parser.ErrKeepNode):
		// The node is whole already, so pass it straight to Leave. A tagged
		// node decodes whole, so the nodes under it count here. The tree also
		// counts each node it reads again for an alias.
		if _, tagged := node.(*ast.TagNode); tagged && at != nil {
			b.built += shapeOf(node).nodes - 1
			if b.budget > 0 && b.built > b.budget {
				return b.fail(yamlerrors.NewExcessiveAliasing(b.built, b.budget, at.GetToken()))
			}
		}

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
	case *ast.TagNode:
		// The walk reaches this case only where the builder reads the tag itself:
		// valueBuilder, for an "any". A typed destination keeps the tagged node, above.
		if err := b.replayNode(n.Value, false, at); err != nil {
			return err
		}
	case *ast.MappingValueNode:
		// A single entry standing in place of a mapping. The walk passes a
		// MappingNode, and so does a clone of one. The tree also builds this
		// node, and nothing here turns it into a mapping.
		return b.fail(errNeedsTheTree)
	}

	return b.Leave(node, cursor)
}

// openMerge reads a "<<" key. The node after it names the merged entries.
//
// A struct with a `,inline,alias` field goes to the tree, which points that
// field at the anchor named by the "<<". The encoder then writes the alias back.
func (b *typedBuilder) openMerge(top *typedFrame) error {
	if top.kind == typedStruct {
		fields, err := structfields.Of(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		if slices.ContainsFunc(fields.Inline, func(sf *structfields.Info) bool { return sf.IsAutoAlias }) {
			return b.fail(errNeedsTheTree)
		}
	}
	top.mergeNext, top.skip, top.target = true, false, reflect.Value{}

	return parser.ErrSkipNode
}

// mergeValue holds the node named by a "<<" until its mapping closes.
//
// The walk cannot write the merged entries when it meets the "<<". A mapping's
// own entries win over merged ones on either side of the "<<". An own entry
// written after the "<<" would decode into a field already filled by the merge,
// and keep the merged values in the parts that the entry leaves out. The tree
// decoder reads the own entries first and the merges after, and so does
// applyMerges.
//
// An alias holds the clone recorded by its anchor. The walk keeps a mapping or
// a sequence written in place whole, and keepMerge clones it at its Leave.
// Anything else (a null, a scalar, an anchor or a tag on the merged node)
// goes to the tree, which reads or rejects it.
func (b *typedBuilder) mergeValue(top *typedFrame, node ast.Node) error {
	top.mergeNext = false
	switch n := node.(type) {
	case *ast.AliasNode:
		anchored, known := b.named(astutils.AnchorName(n.Value))
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

// keepMerge clones the mapping, or the sequence of mappings, written in place
// by a "<<", now that the walk passes it whole. It reads the aliases of a
// sequence here, while each still names the anchor written before it.
func (b *typedBuilder) keepMerge(top *typedFrame, node ast.Node) error {
	seq, isSeq := node.(*ast.SequenceNode)
	if !isSeq {
		if err := astutils.RefuseDuplicateKeys(node); err != nil {
			return b.fail(err)
		}
		b.holdMerge(top, ast.Clone(node))

		return nil
	}
	for _, value := range seq.Values {
		switch v := value.(type) {
		case *ast.AliasNode:
			anchored, known := b.named(astutils.AnchorName(v.Value))
			if !known {
				return b.fail(errNeedsTheTree)
			}
			b.holdMerge(top, anchored.node)
		case *ast.MappingNode:
			if err := astutils.RefuseDuplicateKeys(v); err != nil {
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

// applyMerges writes the merged entries of the mapping on top of the stack, once
// its own entries are in. It applies each "<<" in document order, and writes each
// entry only where neither the mapping nor an earlier merge wrote one.
func (b *typedBuilder) applyMerges(top *typedFrame) error {
	if len(top.merges) == 0 {
		return nil
	}
	merges := top.merges
	top.merges = nil
	b.pending--

	// Decoding a merged value pushes frames and can move the stack, so the
	// loop finds the frame again by its index.
	at := len(b.stack) - 1
	for _, node := range merges {
		if err := b.mergeFrom(at, node, nil); err != nil {
			return err
		}
	}

	return nil
}

// mergeFrom writes the entries of node, a mapping, into the frame at index i.
// It writes the node's own entries first, then the entries merged by its own
// "<<", in the order used by rangeMergedEntries on the tree. alias is the alias
// through which node was reached, for codecutils.AliasBudget to count, and nil
// for a mapping written in place.
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
		anchored, known := b.named(astutils.AnchorName(n.Value))
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

// mergeNested writes the node named by a "<<" inside a merged mapping: a mapping,
// an alias of one, or a sequence of either.
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
// mapping or an earlier merge wrote it. A struct skips a field marked in written.
// A map that opened empty skips a key it holds. A map that opened with the
// caller's entries skips a key in its wrote set instead, so a merged entry
// writes over a caller's entry.
func (b *typedBuilder) mergeEntry(i int, keyNode, value ast.Node, alias *ast.AliasNode) error {
	top := &b.stack[i]
	name, kind := ast.KeyName(keyNode)
	named := kind != token.KeyOther
	if !named {
		if top.kind == typedMap || b.strict {
			return b.fail(errNeedsTheTree)
		}

		return nil
	}

	switch top.kind {
	case typedStruct:
		fields, err := structfields.Of(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		ff, known := fields.Lookup(name)
		if !known {
			b.noteUnknown(top, name, keyNode)

			return nil
		}
		if top.written.has(ff.Ord) {
			return nil
		}
		top.written.mark(ff.Ord)
		top.field = ff
		top.place.mergedKey(keyNode)
		top.place.expect(name)
		field := top.dst.Field(ff.SF.Index)
		if ff.At != nil {
			var ok bool
			if field, ok = codecutils.FieldAt(top.dst, ff.At); !ok {
				return b.fail(errNeedsTheTree)
			}
		}
		top.skip, top.target = false, field
	case typedMap:
		name = b.strs.Clone(name)
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
		top.place.mergedKey(keyNode)
	default:
		return b.fail(errNeedsTheTree)
	}

	return b.replayNode(value, false, alias)
}

// replayCursor implements parser.Closing for the nodes passed by replay.
// typedBuilder reads IsKey, and the valueBuilder of an "any" inside the node
// reads IsKey and IsRoot. None of the nodes is a document's root.
type replayCursor struct {
	key bool
	// schema is the schema of the document the replayed node was recorded in.
	schema token.Schema
}

func (c replayCursor) Depth() int           { return 1 }
func (c replayCursor) In() parser.Kind      { return parser.KindNone }
func (c replayCursor) IsRoot() bool         { return false }
func (c replayCursor) IsKey() bool          { return c.key }
func (c replayCursor) Document() int        { return 0 }
func (c replayCursor) Schema() token.Schema { return c.schema }
func (c replayCursor) HoldsKey(string) bool { return false }

func (b *typedBuilder) openMapping(dst reflect.Value, wanted bool, node ast.Node) error {
	if !wanted {
		// Inside an entry that no field claims. Read the mapping and drop it.
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dropped: true})

		return nil
	}

	if kind := codecutils.PointedKind(dst.Type()); kind != reflect.Struct && kind != reflect.Map {
		// The tree decoder converts the mapping or returns the error.
		return b.keepWhole(dst, true)
	}
	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		// The tree decoder reads this one node.
		return b.keepWhole(out, true)
	}
	switch dst.Kind() {
	case reflect.Struct:
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dst: dst, out: out, place: b.newPlaces(node)})

		return nil
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			// The tree decoder reads the keys of this one node.
			return b.keepWhole(out, true)
		}
		if dst.IsNil() {
			if !dst.CanSet() {
				return b.fail(errNeedsTheTree)
			}
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		frame := typedFrame{kind: typedMap, dst: dst, out: out, place: b.newPlaces(node)}
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

	if codecutils.PointedKind(dst.Type()) != reflect.Slice {
		// The tree decoder converts the sequence or returns the error.
		return b.keepWhole(dst, true)
	}
	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		// The tree decoder reads this one node.
		return b.keepWhole(out, true)
	}
	if dst.Kind() != reflect.Slice || !dst.CanSet() {
		return b.fail(errNeedsTheTree)
	}
	// An empty sequence decodes to an empty slice, not a nil one. The tree
	// decoder does the same, and a caller comparing against "[]T{}" expects it.
	if dst.IsNil() {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
	} else {
		dst.Set(dst.Slice(0, 0))
	}
	b.stack = append(b.stack, typedFrame{kind: typedSlice, dst: dst, out: out})

	return nil
}

// indirect follows the pointers between a destination and the value written
// into it, and allocates the nil ones it can set.
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

// scalar writes the value of a leaf into dst. A key never reaches it: Enter sends each key to keepKey or openEntry
// first.
func (b *typedBuilder) scalar(node ast.Node, dst reflect.Value, wanted bool) error {
	if !wanted {
		b.settle(reflect.Value{})

		return parser.ErrSkipNode
	}
	err := b.setScalar(dst, node)
	if errors.Is(err, errNotWritten) {
		err = b.convertScalar(dst, node)
	}
	if err != nil {
		return b.fail(err)
	}
	b.settle(dst)

	// A leaf holds nothing to descend into, and Leave has nothing to close.
	return parser.ErrSkipNode
}

// keepKey holds an explicit "?" key or a tagged key whole, for openKeptEntry to
// name the entry by at its Leave.
//
// Both enclose the key's content, and the parse passes them as they open, before
// the content is read. ast.KeyName looks through the "?" and reads the tag, as the
// tree decoder does when it names the entry. The whole key therefore gives the name.
func (b *typedBuilder) keepKey(top *typedFrame) error {
	if top.dropped {
		return parser.ErrSkipNode
	}
	b.naming = true

	return parser.ErrKeepNode
}

// openKeptEntry opens the entry named by the key that keepKey held. A key
// holding an anchor or an alias goes to the tree, which records the anchor and
// reads the alias.
func (b *typedBuilder) openKeptEntry(key ast.Node) error {
	if shapeOf(key).named {
		return b.fail(errNeedsTheTree)
	}
	if tag := keyTag(key); tag != nil && !tag.IsMergeKey() {
		// openEntry names the entry by the key's text, which leaves the tag unread: "!!float x: 1" was the key
		// "x" here, and refused with `cannot read "x" as !!float` on the tree and the walk into an any. The key is
		// read as the tree reads every key, so a tag that cannot read its scalar fails here too.
		d, err := b.nodeDecoder()
		if err != nil {
			return b.fail(err)
		}
		if _, err := d.nodeToValue(tag); err != nil {
			return b.fail(err)
		}
	}
	if err := b.openEntry(&b.stack[len(b.stack)-1], key); !errors.Is(err, parser.ErrSkipNode) {
		return err
	}

	return nil
}

// keepWhole holds a node whole, for decodeKept to pass to the tree decoder at
// its Leave. The node is a tagged node, or a collection of another kind than
// its destination.
//
// A tag changes the decoded value: "!!str 1" is a string, "!!binary" a []byte,
// "!!omap" an ordered map. The tree decoder holds those rules, and also the
// error for a mapping written where a string goes. The walk asks it to decode
// that one node, and walks the rest of the document on its own.
func (b *typedBuilder) keepWhole(dst reflect.Value, wanted bool) error {
	if !wanted {
		b.settle(reflect.Value{})

		return parser.ErrSkipNode
	}
	b.decoding, b.decodeInto = true, dst

	return parser.ErrKeepNode
}

// decodeKept decodes the node held by keepWhole into the destination it read,
// with the tree decoder. A node holding an anchor or an alias sends the whole
// decode to the tree: the node decoder reads neither the anchors recorded by
// the walk nor the later ones.
func (b *typedBuilder) decodeKept(node ast.Node) error {
	dst := b.decodeInto
	b.decodeInto = reflect.Value{}
	if shapeOf(node).named || !dst.CanSet() {
		return b.fail(errNeedsTheTree)
	}
	b.decodeNode(dst, node)
	if len(b.stack) > 0 {
		b.settle(dst)
	}

	return b.err
}

// convertScalar writes a scalar of another kind than its field: "name: 1"
// into a string, "i: 1.5" into an int. The tree decoder converts it through
// decodeNode, so the walk and the tree convert by the same rules.
func (b *typedBuilder) convertScalar(dst reflect.Value, node ast.Node) error {
	if !dst.CanSet() {
		return errNeedsTheTree
	}
	b.decodeNode(dst, node)

	return b.err
}

// decodeNode decodes node into dst with the tree decoder's rules. A field, an
// element and a map value go through createDecodedNewValue over the value held
// by dst, as on the tree. The root goes through decodeValue.
//
// decodeNode writes the decoded part whatever the error. fieldError keeps the
// error for the document to return, as the tree keeps a field's error. The walk
// goes on to the fields after it.
func (b *typedBuilder) decodeNode(dst reflect.Value, node ast.Node) {
	d, err := b.nodeDecoder()
	if err != nil {
		_ = b.fail(err)

		return
	}
	// A bytes unmarshaler reads its fragment under the document's version, as on the tree.
	d.docSchema = b.schema
	// A validator error about a field missing from a struct in the node goes to the entry's key, as on the tree.
	d.entryKey = b.entryKey()
	if len(b.stack) == 0 {
		b.fieldError(d.decodeValue(dst, node))

		return
	}
	v, err := d.createDecodedNewValue(dst.Type(), dst, node)
	codecutils.SetDecoded(dst, v)
	b.fieldError(err)
}

// fieldError keeps err for the document to return, unless it already keeps an
// earlier one, and the walk carries on. fieldError names a type mismatch after
// the field of the outermost struct being read. On the tree, each struct on
// the error's path names it again, and the outermost names it last.
//
// An error that stops the document ends the walk of that document instead.
// One example is a repeated key found by the tree decoder inside the node
// passed to it.
func (b *typedBuilder) fieldError(err error) {
	if err == nil {
		return
	}
	if stopsTheDocument(err) {
		_ = b.fail(err)

		return
	}
	for i := range b.stack {
		if b.stack[i].kind == typedStruct {
			b.stack[i].fieldFailed = true
		}
	}
	recordFieldError(&b.docErr, err, func(te *yamlerrors.Error) string {
		for _, frame := range b.stack {
			if frame.kind == typedStruct && !frame.dropped && frame.field.SF != nil {
				return located{sf: frame.field.SF, at: frame.field.At}.name(frame.dst.Type())
			}
		}

		return te.StructField()
	})
}

// noteUnknown records name, a key of top's mapping that names no field, under DisallowUnknownField. Only the first
// such key of a struct counts, and a key starting with one of the allowed prefixes does not count.
func (b *typedBuilder) noteUnknown(top *typedFrame, name string, keyNode ast.Node) {
	if !b.strict || top.dropped || top.unknown != nil {
		return
	}
	if slices.ContainsFunc(b.allowedPrefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
		return
	}
	// The walk recycles the key's node and token, so the error keeps a token of its own at the key's position.
	at := keyNode.GetToken().Position
	top.unknown = token.New(name, name, at)
}

// closeStruct returns the error for the first unknown key of the struct closing in top, as the tree's finishStruct
// does: only when no field of the struct failed, since a field error takes precedence. The error stops the document.
//
// It then runs the validator of codec.Validator on the struct, as finishStruct does, unless a field failed. A
// validation error is a field error, and the fields after the struct still decode.
func (b *typedBuilder) closeStruct(top *typedFrame) error {
	if top.kind != typedStruct || top.dropped || top.fieldFailed {
		return nil
	}
	if top.unknown != nil {
		return b.fail(yamlerrors.NewUnknownField(fmt.Sprintf(`unknown field "%s"`, top.unknown.Value), top.unknown))
	}
	if b.validator == nil {
		return nil
	}
	err := b.validator(top.dst.Interface())
	if err == nil {
		return nil
	}
	fields, ferr := structfields.Of(top.dst.Type(), b.mode)
	if ferr != nil {
		return b.fail(ferr)
	}
	b.fieldError(validationError(err, fields, walkPlaces{top.place}))

	return b.err
}

// newPlaces returns the framePlaces of a struct or a map frame opening on node, or nil without a validator.
// The entry that writes the frame is the one being read by the frame below it.
func (b *typedBuilder) newPlaces(node ast.Node) *framePlaces {
	if b.validator == nil {
		return nil
	}

	return &framePlaces{entryKey: b.entryKey(), whole: ownToken(node.GetToken())}
}

// entryKey returns the key token of the entry whose value the walk is reading: the entry of the nearest struct
// or map frame, past any anchor. A sequence element, and a document's root, have none. The tree decoder keeps the
// same token in Decoder.entryKey.
func (b *typedBuilder) entryKey() *token.Token {
	for _, frame := range slices.Backward(b.stack) {
		switch frame.kind {
		case typedAnchor:
			continue
		case typedStruct, typedMap:
			if p := frame.place; p != nil {
				return p.key
			}

			return nil
		default:
			return nil
		}
	}

	return nil
}

// placeValue records where a validator error about the field whose value starts at node goes:
// the value's own token for a scalar, the key's for a mapping or a sequence.
func (b *typedBuilder) placeValue(top *typedFrame, node ast.Node) {
	p := top.place
	if p == nil || !p.hasPending {
		return
	}
	p.hasPending = false
	if _, held := p.fields[p.pending]; held {
		return
	}
	if p.fields == nil {
		p.fields = map[string]*token.Token{}
	}
	p.fields[p.pending] = ownToken(astutils.ValidationErrorToken(node, p.key))
}

// enterKey records the key of an entry of the frame's own mapping. A nil p records nothing.
func (p *framePlaces) enterKey(keyNode ast.Node) {
	if p == nil {
		return
	}
	p.key, p.hasPending = ownToken(keyNode.GetToken()), false
	if p.firstKey == nil {
		p.firstKey = p.key
	}
}

// mergedKey records the key of an entry brought in by a "<<". It is not the mapping's own first key.
func (p *framePlaces) mergedKey(keyNode ast.Node) {
	if p == nil {
		return
	}
	p.key, p.hasPending = ownToken(keyNode.GetToken()), false
}

// expect records that the next value the walk passes writes the field named name.
func (p *framePlaces) expect(name string) {
	if p == nil {
		return
	}
	p.pending, p.hasPending = name, true
}

// walkPlaces places a validation error from the tokens a struct frame recorded, as treePlaces does on the tree.
type walkPlaces struct{ p *framePlaces }

func (w walkPlaces) field(name string) *token.Token { return w.p.fields[name] }

func (w walkPlaces) missing() *token.Token {
	if w.p.entryKey != nil {
		return w.p.entryKey
	}
	if w.p.firstKey != nil {
		return w.p.firstKey
	}

	return w.p.whole
}

func (w walkPlaces) whole() *token.Token { return w.p.whole }

// ownToken returns a copy of tk, or nil. A Token holds no pointer, so the copy shares nothing with the walk.
func ownToken(tk *token.Token) *token.Token {
	if tk == nil {
		return nil
	}
	c := *tk

	return &c
}

// nodeDecoder returns the tree decoder used by decodeNode. It makes it from the
// options of the Decoder served by the walk, and the result holds no pointer to it.
//
// Its budget stays 0, which isOverBudget reads as no bound: a kept node holds no alias, and replayNode counts a
// tagged node that an alias decodes again against the walk's own budget.
func (b *typedBuilder) nodeDecoder() (*Decoder, error) {
	if b.nodes != nil {
		return b.nodes, nil
	}
	d := NewDecoder(nil, b.opts...)
	if err := d.resolveReference(); err != nil {
		return nil, err
	}
	d.isResolvedReference = true
	b.nodes = d

	return d, nil
}

// keptShape counts the nodes of a subtree, and records whether it holds an
// anchor or an alias.
type keptShape struct {
	nodes int
	named bool
}

func (k *keptShape) Visit(node ast.Node) ast.Visitor {
	switch node.(type) {
	case *ast.CommentGroupNode, *ast.CommentNode:
		return nil
	case *ast.AnchorNode, *ast.AliasNode:
		k.named = true
	}
	k.nodes++

	return k
}

// openEntry names the entry opened by a key, and sets the destination of its value.
func (b *typedBuilder) openEntry(top *typedFrame, keyNode ast.Node) error {
	if top.dropped {
		return parser.ErrSkipNode
	}
	top.place.enterKey(keyNode)

	if key, isKey := keyNode.(ast.MapKeyNode); isKey && key.IsMergeKey() {
		return b.openMerge(top)
	}

	name, kind := ast.KeyName(keyNode)
	named := kind != token.KeyOther
	if !named {
		if top.kind == typedMap || b.strict {
			return b.fail(errNeedsTheTree)
		}
		// No field can be named after it.
		top.skip = true

		return parser.ErrSkipNode
	}

	switch top.kind {
	case typedStruct:
		fields, err := structfields.Of(top.dst.Type(), b.mode)
		if err != nil {
			return b.fail(err)
		}
		ff, known := fields.Lookup(name)
		if !known {
			top.skip, top.target = true, reflect.Value{}
			b.noteUnknown(top, name, keyNode)

			return parser.ErrSkipNode
		}
		top.written.mark(ff.Ord)
		top.field = ff
		top.place.expect(name)
		if ff.At != nil {
			// The type embeds another, and the name selects a field of the embedded type.
			field, ok := codecutils.FieldAt(top.dst, ff.At)
			if !ok {
				return b.fail(errNeedsTheTree)
			}
			top.skip, top.target = false, field

			return parser.ErrSkipNode
		}
		top.skip = false
		top.target = top.dst.Field(ff.SF.Index)
	case typedMap:
		top.skip = false
		name = b.strs.Clone(name)
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

func (b *typedBuilder) setScalar(dst reflect.Value, node ast.Node) error {
	if _, isNull := node.(*ast.NullNode); isNull {
		// A null sets a pointer to nil, as the tree decoder and encoding/json do.
		if dst.Kind() == reflect.Pointer && dst.CanSet() {
			dst.Set(reflect.Zero(dst.Type()))

			return nil
		}
		// It leaves any other destination as it stands. The tree decoder does the
		// same: it builds a zero value and then writes the default over it.
		// "nested:" with nothing under it keeps the defaults set by the caller.
		return nil
	}

	dst = b.indirect(dst)
	if !dst.CanSet() {
		return errNeedsTheTree
	}
	if b.handled(dst) {
		// convertScalar passes the scalar to the tree decoder.
		return errNotWritten
	}

	switch dst.Kind() {
	case reflect.String:
		s, isText := node.(*ast.StringNode)
		if !isText {
			return errNotWritten
		}
		dst.SetString(b.strs.Clone(s.Value))

		return nil
	case reflect.Bool:
		v, isBool := node.(*ast.BoolNode)
		if !isBool {
			return errNotWritten
		}
		dst.SetBool(v.Value)

		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := astutils.ScalarNumber(node).(type) {
		case int64:
			if dst.OverflowInt(v) {
				return errNotWritten
			}
			dst.SetInt(v)

			return nil
		case uint64:
			if v > 1<<63-1 || dst.OverflowInt(int64(v)) {
				return errNotWritten
			}
			dst.SetInt(int64(v))

			return nil
		}

		return errNotWritten
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := astutils.ScalarNumber(node).(type) {
		case uint64:
			if dst.OverflowUint(v) {
				return errNotWritten
			}
			dst.SetUint(v)

			return nil
		case int64:
			if v < 0 || dst.OverflowUint(uint64(v)) {
				return errNotWritten
			}
			dst.SetUint(uint64(v))

			return nil
		}

		return errNotWritten
	case reflect.Float32, reflect.Float64:
		switch v := astutils.ScalarNumber(node).(type) {
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

		return errNotWritten
	default:
		return errNotWritten
	}
}

// handled reports whether the tree decoder reads dst for itself: a custom
// unmarshaler, a time.Time or time.Duration, a MapSlice. This walk writes none
// of them.
func (b *typedBuilder) handled(dst reflect.Value) bool {
	if !dst.CanAddr() {
		return true
	}
	// canWalk skips the walk for a Decoder holding a custom unmarshaler
	// or UseJSONUnmarshaler, so neither applies here.
	if decodesItself(dst.Addr(), false) || v3NodeLayoutOf(dst.Type()) != nil || registeredUnmarshaler(dst.Addr().Type()) {
		return true
	}
	switch dst.Interface().(type) {
	case MapSlice, MapSliceSeq, MapItem:

		return true
	}

	return false
}

// walkableTypes caches, keyed by reflect.Type, whether the typed walk can fill
// a destination type. It keeps one map per Mode, because the mode sets the
// fields of a type, and so whether the walk can fill it.
var walkableTypes [structfields.ModeCount]structfields.Cache[bool]

// walkableType reports whether the walk can fill a destination of type t,
// from the Go type alone.
//
// It answers before the parse. A walk that gives up halfway has read the
// document for nothing, and the decode reads it again to build a tree. A
// destination that the walk could never fill then costs two parses instead of
// one. walkableType therefore checks here, once per type, every reason to give
// up that depends on the type and not on the document.
func walkableType(t reflect.Type, mode structfields.Mode) bool {
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
	// keptForTheTree holds the interfaces of a type the walk keeps the node of, for the tree decoder to read at
	// its Leave. Each one receives text, or a value the tree decoder builds, and keeps nothing of the node.
	keptForTheTree = []reflect.Type{
		reflect.TypeFor[Unmarshaler](),
		reflect.TypeFor[GoYAMLUnmarshaler](),
		reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	// heldNodeTypes holds the interfaces of a type handed the node itself. A NodeUnmarshaler may keep the node it
	// receives, and so may an ast.Node field. The walk recycles its nodes after their Leave, so such a type sends
	// the whole decode to the tree, whose nodes live as long as the file.
	heldNodeTypes = []reflect.Type{
		reflect.TypeFor[NodeUnmarshaler](),
		reflect.TypeFor[ast.Node](),
	}
	// readByTheTree holds the types the tree decoder reads by rules of its own, which the walk keeps the node of.
	readByTheTree = []reflect.Type{
		reflect.TypeFor[time.Time](),
		reflect.TypeFor[time.Duration](),
		reflect.TypeFor[MapSlice](),
		reflect.TypeFor[MapSliceSeq](),
		reflect.TypeFor[MapItem](),
	}
	rawMessageType = reflect.TypeFor[RawMessage]()
)

// init registers unmarshalerTypes with structfields.SelfDecoding: a `,inline` field whose type
// implements one of them reads the whole mapping itself.
func init() { structfields.SelfDecoding(unmarshalerTypes...) }

func readWalkable(t reflect.Type, seen map[reflect.Type]bool, mode structfields.Mode) bool {
	if seen[t] {
		// A type that holds itself. The cycle does not decide the answer, and
		// the document's depth is capped elsewhere.
		return true
	}
	seen[t] = true

	pointer := reflect.PointerTo(t)
	switch {
	case t == rawMessageType:
		// A RawMessage holds the source text of its node, which the walk does not keep.
		return false
	case implementsAny(t, pointer, heldNodeTypes):
		return false
	case keptWhole(t, pointer):
		return true
	}

	switch t.Kind() {
	case reflect.Pointer:
		return readWalkable(t.Elem(), seen, mode)
	case reflect.Struct:
		fields, err := structfields.Of(t, mode)
		if err != nil {
			return false
		}
		for _, sf := range fields.ByGoName {
			ft := t.Field(sf.Index).Type
			if sf.IsInline && structfields.InlineReadsItsOwnNode(ft) {
				// The field reads the whole mapping, which the walk passes entry by entry.
				return false
			}
			if sf.IsInline && structfields.InlineTakesUnclaimedEntries(ft) {
				// The walk meets an entry once and must place it then, through
				// an index path named in structfields.Fields.flat. A map has no
				// such path, so the tree decoder reads this struct.
				return false
			}
			if !readWalkable(ft, seen, mode) {
				return false
			}
		}

		return true
	case reflect.Map:
		// A map keyed by anything but a string is kept whole for the tree decoder, which reads its keys.
		return readWalkable(t.Key(), seen, mode) && readWalkable(t.Elem(), seen, mode)
	case reflect.Slice, reflect.Array:
		// An array is kept whole for the tree decoder, which fills it.
		return readWalkable(t.Elem(), seen, mode)
	case reflect.Interface:
		// An "any" holds whatever the document writes, and valueBuilder builds it. The walk keeps the node of a
		// narrower interface for the tree decoder, which sets it or returns the error.
		return true
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		// convertScalar passes a complex number to the tree decoder.
		reflect.Complex64, reflect.Complex128:
		return true
	default:
		// A channel, a function, an unsafe pointer.
		return false
	}
}

// keptWhole reports whether the walk keeps the node of a destination of type t, for the tree decoder to read: a
// type the tree decoder reads by rules of its own, or one that decodes itself from text or through a value.
func keptWhole(t, pointer reflect.Type) bool {
	return slices.Contains(readByTheTree, t) ||
		v3NodeLayoutOf(t) != nil ||
		v3NodeHook(pointer) != nil ||
		registeredUnmarshaler(pointer) ||
		implementsAny(t, pointer, keptForTheTree)
}

func implementsAny(t, pointer reflect.Type, interfaces []reflect.Type) bool {
	for _, iface := range interfaces {
		if t.Implements(iface) || pointer.Implements(iface) {
			return true
		}
	}

	return false
}

// reset readies b to fill root with the next document, and keeps the capacity
// of its stack.
func (b *typedBuilder) reset(root reflect.Value) {
	b.root = root
	b.stack = b.stack[:0]
	b.err = nil
	b.values, b.anyInto = nil, reflect.Value{}
	clear(b.anchors)
	b.keeping, b.pending = false, 0
	b.decoding, b.decodeInto = false, reflect.Value{}
	b.docErr = nil
	b.naming = false
}

func shapeOf(node ast.Node) keptShape {
	var k keptShape
	ast.Walk(&k, node)

	return k
}
