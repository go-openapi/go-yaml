// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/codec/internal/astutils"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// valueBuilder folds a document into Go values as the parse reaches each node.
// The document is read once, instead of built into a tree and walked.
//
// It runs codec.jsonTokener's walk with a different fold: the same Enter and
// Leave over the same nodes, keeping Go values where the converter keeps JSON
// tokens.
// It differs in four values that a JSON writer cannot use:
//
//   - a number too wide for a machine word stays a *big.Int or *big.Float;
//   - a "!!timestamp" becomes a time.Time;
//   - "!!binary" becomes []byte;
//   - an infinity stays one, where JSON must reject it.
type valueBuilder struct {
	// docs holds the value of each document body, in stream order.
	docs []any
	// stack holds the frames being filled, innermost last: a mapping, a sequence,
	// or a property standing around the node it names.
	stack []buildFrame
	// named holds what each anchor of the document named, for a later alias.
	// Enter empties it at each document, because an anchor belongs to one document.
	named map[string]any
	// declared holds the anchors passed by parser.WithAnchors, which the walk
	// never passes over. declaredValues holds the value built for each one the
	// first time an alias named it. Both serve every document of the stream.
	declared       map[string]ast.Node
	declaredValues map[string]any
	// strs holds the text of every string built here, copied out of the
	// document so that the values outlive it.
	strs codecutils.Arena
	// open holds the anchors whose node is being read, innermost last. An alias
	// naming one of them stands inside its own anchor.
	open []string
	// share gives every alias of one anchor the same value. See
	// codec.ShareAliases.
	share bool
	// built counts the values copied for an alias, and budget bounds them. A
	// chain of aliases multiplies the copies, and a document written to exhaust
	// memory abuses exactly that.
	built  int
	budget int
	err    error
}

type frameKind uint8

const (
	// frameMapping and frameSequence mark collections being filled.
	frameMapping frameKind = iota
	frameSequence
	// frameProperty marks an anchor, a tag or a "?" standing around one node. It
	// takes the value built by that node and passes it on transformed.
	frameProperty
)

type buildFrame struct {
	kind frameKind
	// asKey records whether the node of this frame arrived as a mapping key.
	// Its Leave delivers the built value, and the walk has moved on by then.
	asKey bool

	// mapping
	//
	// m holds a mapping whose keys are all strings, as nearly all are.
	// anyKeyed replaces it on the first key that resolves to anything else:
	// an integer, a float, a null, a Base64, a time.Time. The key then keeps
	// its resolved value instead of its text, and two keys alike in text stay
	// two entries.
	m        map[string]any
	anyKeyed map[any]any
	// key holds the arena's copy of the entry's name, and keyValue the resolved
	// value of the key. keyIsString selects which of the two a widened mapping
	// writes.
	key         string
	keyValue    any
	keyIsString bool
	hasKey      bool
	merging     bool
	// broughtByValue records that a "<<" brought in a key that a Go map lookup
	// cannot match by value: a time.Time, a *big.Int, a *big.Float. An own key
	// equal to it in YAML then replaces that entry.
	broughtByValue bool
	// keyErr holds the error for a key that this destination cannot hold, kept
	// until the mapping closes. The parser attaches a mapping's repeated keys to
	// it as it closes. Returning the unusable key at once would hide a repeat:
	// "{[a]: 1, [ a ]: 2}" is a repeat first and an unusable key second.
	keyErr error

	// sequence
	seq []any
	// mergeSource records that the sequence is the value of a "<<", so every
	// element must be a mapping. deliver checks each one as it arrives, while
	// its node is at hand for the error to point at.
	mergeSource bool

	// property
	node     ast.Node
	received any
	// mergeKey records that the property stands as a key on a "<<" and names no entry. See markMerge.
	mergeKey bool
	got      bool
}

// Enter is called before anything a node holds.
func (b *valueBuilder) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsRoot() {
		if _, isDirective := node.(*ast.DirectiveNode); isDirective {
			// A directive opens a document of its own and holds no value.
			return parser.ErrSkipNode
		}
		b.named = nil
	}

	if key, isKey := node.(ast.MapKeyNode); isKey && key.IsMergeKey() {
		// "<<" names no key of its own. merge folds its entries into the
		// mapping when the value below it arrives.
		b.markMerge()

		return parser.ErrSkipNode
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		b.stack = append(b.stack, buildFrame{kind: frameMapping, asKey: at.IsKey(), m: map[string]any{}})
	case *ast.SequenceNode:
		// An empty sequence decodes to an empty slice, not a nil one. The
		// tree-walking decoder does the same, and a caller comparing against
		// "[]any{}" expects it.
		b.stack = append(b.stack,
			buildFrame{kind: frameSequence, asKey: at.IsKey(), seq: []any{}, mergeSource: b.mergingValue(at.IsKey())})
	case *ast.AnchorNode:
		b.open = append(b.open, astutils.AnchorName(n.Name))
		b.stack = append(b.stack, buildFrame{kind: frameProperty, asKey: at.IsKey(), node: node})
	case *ast.TagNode, *ast.MappingKeyNode:
		b.stack = append(b.stack, buildFrame{kind: frameProperty, asKey: at.IsKey(), node: node})
	case *ast.AliasNode:
		b.deliver(b.aliasValue(n), node, at.IsKey())
	case *ast.CommentGroupNode:
		return parser.ErrSkipNode
	default:
		v, err := b.scalarValue(node)
		if err != nil {
			return b.fail(err)
		}
		b.deliver(v, node, at.IsKey())
	}

	// aliasValue and copyValue record their own errors in b.err, and Enter returns it here.
	return b.err
}

// Leave is called once everything a node holds has been.
func (b *valueBuilder) Leave(node ast.Node, _ parser.Closing) error {
	if b.err != nil {
		// After a visitor fails, the walk passes no more nodes, but still leaves the nodes it had open.
		// The frames below stay as they were, because nothing reads them again.
		return b.err
	}

	switch node.(type) {
	case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.TagNode, *ast.MappingKeyNode:
	default:
		return nil
	}

	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]

	switch frame.kind {
	case frameMapping:
		// The parser attaches the repeated keys to the mapping as it closes, so
		// Leave reads them here and not as the mapping opens.
		if err := astutils.RefuseDuplicateKeys(node); err != nil {
			return b.fail(err)
		}
		if frame.keyErr != nil {
			return b.fail(frame.keyErr)
		}
		b.deliver(frame.value(), node, frame.asKey)
	case frameSequence:
		b.deliver(frame.seq, node, frame.asKey)
	case frameProperty:
		if frame.mergeKey {
			// The property stood on a "<<", which names no entry. Nothing goes to the mapping, and its anchor
			// names no value.
			if _, anchored := frame.node.(*ast.AnchorNode); anchored && len(b.open) > 0 {
				b.open = b.open[:len(b.open)-1]
			}

			return b.err
		}
		v, err := b.closeProperty(frame)
		if err != nil {
			return b.fail(err)
		}
		b.deliver(v, node, frame.asKey)
	default:
		panic(fmt.Sprintf("codec: unknown frame kind %d", frame.kind))
	}

	return b.err
}

// markMerge records that a "<<" opens the next entry of the mapping being built.
//
// The "<<" may stand under an anchor written as the key, "&a <<: *b". The walk passes the anchor before the node
// it names, so the anchor's frame is on top when the "<<" arrives. The mapping sits in the frame below the key's
// properties. markMerge marks those properties so that they deliver no key as they close.
func (b *valueBuilder) markMerge() {
	i := len(b.stack) - 1
	for i >= 0 && b.stack[i].kind == frameProperty && b.stack[i].asKey {
		i--
	}
	if i < 0 || b.stack[i].kind != frameMapping {
		return
	}
	b.stack[i].merging = true
	for j := i + 1; j < len(b.stack); j++ {
		b.stack[j].mergeKey = true
	}
}

// closeProperty applies a property to the value built by its node, and returns
// the result.
func (b *valueBuilder) closeProperty(frame buildFrame) (any, error) {
	value := frame.received
	if !frame.got {
		// The walk passes nothing under a tag on a scalar, because
		// parseScalarValue builds the value without going through parseToken.
		// closeProperty reads it from the node here, as jsonTokener.closeTag does.
		// An anchor between a tag and its scalar has the same shape. A property
		// standing on nothing reads as the empty node.
		v, err := b.propertyValue(frame.node)
		if err != nil {
			return nil, err
		}
		value = v
	}

	switch n := frame.node.(type) {
	case *ast.AnchorNode:
		if n := len(b.open); n > 0 {
			b.open = b.open[:n-1]
		}
		if name := astutils.AnchorName(n.Name); name != "" {
			if b.named == nil {
				b.named = map[string]any{}
			}
			b.named[name] = value
		}

		return value, nil
	case *ast.TagNode:
		tagged, err := b.taggedWalkValue(n, value)
		if err != nil {
			return nil, err
		}
		if anchor, anchored := n.Value.(*ast.AnchorNode); anchored {
			// The anchor closed before the tag was applied, and recorded its
			// value untagged. 6.9 gives the node both properties, so the name
			// stands for the tagged value: "a: !!int &a1 \"5\"" reads 5 at a,
			// and an alias to a1 reads 5, not "5".
			//
			// For a flow key written alone, "{!!null &a1 null, k: *a1}", the walk
			// passes the tag without the node under it. The anchor never opened,
			// and only this branch records its name.
			if name := astutils.AnchorName(anchor.Name); name != "" {
				if b.named == nil {
					b.named = map[string]any{}
				}
				b.named[name] = tagged
			}
		}

		return tagged, nil
	default:
		// A "?" key stands around the node that names the entry.
		return value, nil
	}
}

// propertyValue reads the node under a property, for the scalars that the parse
// builds without passing them to the walk.
func (b *valueBuilder) propertyValue(n ast.Node) (any, error) {
	var inner ast.Node
	switch t := n.(type) {
	case *ast.AnchorNode:
		inner = t.Value
	case *ast.TagNode:
		inner = t.Value
	case *ast.MappingKeyNode:
		inner = t.Value
	}
	if inner == nil {
		return nil, nil
	}
	if _, scalar := inner.(ast.ScalarNode); !scalar {
		return nil, nil
	}

	return b.scalarValue(inner)
}

// aliasValue returns the value named by an alias: the value recorded by the
// document's anchor as it closed, or else the value of an anchor declared by
// parser.WithAnchors.
func (b *valueBuilder) aliasValue(n *ast.AliasNode) any {
	name := astutils.AnchorName(n.Value)
	if v, named := b.named[name]; named {
		if b.share {
			return v
		}

		// Two aliases of one anchor give two values, so writing through one
		// leaves the other alone. The walk has already passed the anchored
		// nodes and cannot read them again, so aliasValue copies the built
		// value instead of rebuilding it. See codec.ShareAliases.
		return b.copyValue(v, n)
	}

	if slices.Contains(b.open, name) {
		// The anchor is being read right now, so the alias stands inside its
		// own anchor.
		// The walk builds a Go value and has nowhere to put a cycle.
		// AliasNode.Target cannot detect this during a walk:
		// the parser fills it at the document's end, after the walk has passed the alias.
		_ = b.fail(yamlerrors.NewRecursiveAlias(name, n.GetToken()))

		return nil
	}

	if node, declared := b.declared[name]; declared {
		v, err := b.declaredValue(name, node, n)
		if err != nil {
			// Enter returns b.err once this returns.
			_ = b.fail(err)

			return nil
		}
		if b.share {
			return v
		}

		return b.copyValue(v, n)
	}

	_ = b.fail(yamlerrors.NewUnknownAnchor(name, n.GetToken()))

	return nil
}

// declaredValue returns the value named by a declared anchor, built once from
// the tree passed by the caller.
//
// The tree comes from another parse and is whole, so the tree decoder reads it.
// Because the value is built once, every alias of the anchor gets the same value
// under ShareAliases, as it does for an anchor of the document.
func (b *valueBuilder) declaredValue(name string, node ast.Node, alias *ast.AliasNode) (any, error) {
	if v, built := b.declaredValues[name]; built {
		return v, nil
	}
	if node == nil {
		// The parser takes a nil entry as a declaration, and ToJSON rejects it
		// the same way.
		return nil, yamlerrors.NewUnknownAnchor(name, alias.GetToken())
	}

	dec := NewDecoder(nil)
	dec.shareAliases = b.share
	v, err := dec.nodeToValue(node)
	if err != nil {
		return nil, err
	}
	if b.declaredValues == nil {
		b.declaredValues = map[string]any{}
	}
	b.declaredValues[name] = v

	return v, nil
}

// copyValue returns a copy of v that shares nothing with it. It builds a map
// and a slice again, and copies their elements in turn. A caller cannot write
// through any other value, so copyValue returns it as is.
func (b *valueBuilder) copyValue(v any, at ast.Node) any {
	if b.err != nil {
		return nil
	}
	b.built++
	if b.budget > 0 && b.built > b.budget {
		_ = b.fail(yamlerrors.NewExcessiveAliasing(b.built, b.budget, at.GetToken()))

		return nil
	}

	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = b.copyValue(e, at)
		}

		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = b.copyValue(e, at)
		}

		return out
	case []byte:
		return bytes.Clone(t)
	case *big.Int:
		return new(big.Int).Set(t)
	case *big.Float:
		return new(big.Float).Set(t)
	default:
		// A string, a number, a bool, a time.Time, nil: a caller holding two of
		// these cannot write through one and see the other.
		return v
	}
}

// deliver puts v into the frame enclosing node, or into docs at the top level.
func (b *valueBuilder) deliver(v any, node ast.Node, key bool) {
	n := len(b.stack)
	if n == 0 {
		b.docs = append(b.docs, v)

		return
	}

	top := &b.stack[n-1]
	switch top.kind {
	case frameProperty:
		top.received, top.got = v, true
	case frameMapping:
		b.deliverToMapping(top, v, node, key)
	case frameSequence:
		if top.mergeSource && !codecutils.IsMapValue(v) {
			// "<<" takes a mapping or a sequence of them, and nothing else:
			// not a null, and not a sequence inside the sequence. mergingValue
			// sets the flag only on a sequence held by the merging mapping, so
			// that mapping stands just below it.
			b.stack[n-2].refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

			return
		}
		top.seq = append(top.seq, v)
	default:
		panic(fmt.Sprintf("codec: unknown frame kind %d", top.kind))
	}
}

// deliverToMapping puts v in the mapping built by top: as the key of the next
// entry when key is set, and as the value of the current entry otherwise.
func (b *valueBuilder) deliverToMapping(top *buildFrame, v any, node ast.Node, key bool) {
	if key {
		// The name comes from the node and not from the value, so that the type
		// sets its spelling: astutils.MapKeyString writes an integer in decimal and
		// a float with a decimal point.
		name, err := astutils.MapKeyString(node, v)
		if err != nil {
			if top.keyErr == nil {
				top.keyErr = err
			}
			top.key, top.hasKey = "", false

			return
		}
		top.keyed(b.strs.Clone(name), v)

		return
	}
	if top.merging {
		b.merge(top, v, node)
		top.merging = false

		return
	}
	top.put(v)
	top.key, top.hasKey = "", false
}

// keyed records the key of the entry being written, and whether it is a
// string.
//
// keyed keeps the name for a string key, so that the map holds the arena's copy.
// For every other type, keyValue holds the resolved value of the key, and the
// widened mapping holds that value.
func (f *buildFrame) keyed(name string, value any) {
	f.key, f.hasKey = name, true
	_, f.keyIsString = value.(string)
	f.keyValue = value
}

// put writes value under the key held by the frame. It widens the mapping
// when the key is not a string.
func (f *buildFrame) put(value any) {
	if f.keyIsString && f.anyKeyed == nil {
		f.m[f.key] = value

		return
	}
	if f.anyKeyed == nil {
		f.widen()
	}
	key := f.mapKey()
	if f.broughtByValue {
		// The mapping's own key replaces the entry brought in by a "<<" under an
		// equal key: for a timestamp the same instant in any zone, for a wide number
		// the same value.
		if held, found := codecutils.SameKeyIn(f.anyKeyed, key); found {
			delete(f.anyKeyed, held)
		}
	}
	f.anyKeyed[key] = value
}

// mapKey returns the key for a widened mapping: the arena's copy of the name for
// a string, and the resolved value of the key for anything else.
func (f *buildFrame) mapKey() any {
	if f.keyIsString {
		return f.key
	}

	return f.keyValue
}

// widen moves a mapping from map[string]any to map[any]any, on the first key
// that is not a string.
//
// The keys already written are strings, and widen copies them as they stand.
// A document whose keys are all strings never calls widen, and keeps the
// map[string]any that callers have always received.
func (f *buildFrame) widen() {
	f.anyKeyed = make(map[any]any, len(f.m)+1)
	for key, value := range f.m {
		f.anyKeyed[key] = value
	}
	f.m = nil
}

// value returns the mapping built by the frame, with whichever key type it settled on.
func (f *buildFrame) value() any {
	if f.anyKeyed != nil {
		return f.anyKeyed
	}

	return f.m
}

// holds reports whether the mapping already writes key. merge calls it before
// bringing a key in.
func (f *buildFrame) holds(key any) bool {
	if f.anyKeyed != nil {
		if _, held := f.anyKeyed[key]; held {
			return true
		}
		_, held := codecutils.SameKeyIn(f.anyKeyed, key)

		return held
	}
	name, isString := key.(string)
	if !isString {
		return false
	}
	_, held := f.m[name]

	return held
}

// bring writes value, brought in by a "<<", under key, where the mapping holds none.
func (f *buildFrame) bring(key any, value any) {
	if name, isString := key.(string); isString && f.anyKeyed == nil {
		f.m[name] = value

		return
	}
	if f.anyKeyed == nil {
		f.widen()
	}
	if codecutils.ComparesByValue(key) {
		f.broughtByValue = true
	}
	f.anyKeyed[key] = value
}

// merge folds the value named by a "<<" into the mapping holding it.
//
// The merged entries are defaults: a key written by the mapping itself wins,
// whichever side of the "<<" it stands on. merge writes them without
// overwriting, and that gives the same result either way. A "<<" written first
// fills an empty mapping, and the mapping's own keys replace the entries they
// repeat. A "<<" written last finds those keys already there.
//
// A "<<" takes a mapping or a sequence of them, and an earlier mapping of the
// sequence wins over a later one.
func (b *valueBuilder) merge(top *buildFrame, v any, node ast.Node) {
	switch t := v.(type) {
	case map[string]any:
		for key, value := range t {
			if !top.holds(key) {
				top.bring(key, value)
			}
		}
	case map[any]any:
		for key, value := range t {
			if !top.holds(key) {
				top.bring(key, value)
			}
		}
	case []any:
		for _, one := range t {
			if !codecutils.IsMapValue(one) {
				// The walk reaches this case through an anchor or an alias, where
				// the element's own node is not at hand. deliver checked a sequence
				// written in place element by element as it was built.
				top.refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

				return
			}
			b.merge(top, one, node)
		}
	default:
		// "<<" takes a mapping or a sequence of them, and nothing else. A null
		// is no mapping: merge rejects "<<:" with no value, as the tree does.
		top.refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))
	}
}

// refuseMerge keeps err on the mapping holding a "<<". Leave returns it at the
// close, after the repeats recorded by the parse. "{<<: {x: 1}, <<: }" repeats
// a key before it merges a null, and the tree returns the repeat.
func (f *buildFrame) refuseMerge(err error) {
	if f.keyErr == nil {
		f.keyErr = err
	}
}

// mergingValue reports whether the node being entered is the value of a "<<" key.
func (b *valueBuilder) mergingValue(key bool) bool {
	n := len(b.stack)

	return n > 0 && b.stack[n-1].kind == frameMapping && b.stack[n-1].merging && !key
}

// fail keeps the first error and returns it, so a caller can return it to the walk in one line.
func (b *valueBuilder) fail(err error) error {
	if b.err == nil {
		b.err = err
	}

	return b.err
}

// scalarValue returns the Go value of a scalar node.
func (b *valueBuilder) scalarValue(n ast.Node) (any, error) {
	switch t := n.(type) {
	case *ast.NullNode:
		return nil, nil
	case *ast.LiteralNode:
		if t.Value == nil {
			return "", nil
		}

		return b.strs.Value(t.Value.Value), nil
	case *ast.StringNode:
		return b.strs.Value(t.Value), nil
	case ast.ScalarNode:
		return t.GetValue(), nil
	default:
		return nil, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.StringType, n.GetToken())
	}
}

// taggedWalkValue applies a tag to the value its node built.
func (b *valueBuilder) taggedWalkValue(n *ast.TagNode, value any) (any, error) {
	res := n.Resolve()

	switch res.Verdict {
	case ast.TagUnresolved:
		return value, nil
	case ast.TagKindMismatch:
		return nil, yamlerrors.NewSyntax(
			fmt.Sprintf("%s does not support this kind of node", res.Tag), n.GetToken())
	case ast.TagValueMismatch:
		if res.Lax {
			return b.strs.Value(res.Text), nil
		}

		return nil, yamlerrors.NewSyntax(
			fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), n.Value.GetToken())
	default:
		// nothing to do
	}

	if res.Empty {
		return tagZero(res.Tag)
	}

	// Here the walk departs from the JSON converter. A number too wide for a
	// machine word stays a *big.Int or *big.Float, "!!binary" gives bytes and
	// "!!timestamp" a time.Time. JSON has no spelling for any of them.
	switch res.Tag {
	case token.StringTag:
		return b.strs.Value(res.Text), nil
	case token.NullTag:
		return nil, nil
	case token.IntegerTag:
		// Read from the text and the document's schema, as Decoder.taggedValue
		// reads it. Converting the walked value instead took a quoted scalar
		// as a string, and the two paths parted on `!!float -0`. The result
		// keeps the Go type of the untagged number.
		return taggedInteger(res.Text, res.Schema), nil
	case token.FloatTag:
		return codecutils.CastToFloatValue(taggedFloat(res.Text, res.Schema)), nil
	case token.BooleanTag:
		// Resolve has found a boolean to read, in whatever case the
		// document wrote it.
		b, _ := token.ParseBool(strings.ToLower(res.Text))

		return b, nil
	case token.BinaryTag:
		// A Base64, as Decoder.taggedValue reads it. The two paths must
		// agree, and they parted on "!!float -0" once already.
		return Base64(res.Text), nil
	case token.TimestampTag:
		t, _ := token.ParseTimestamp(res.Text)

		return t, nil
	case token.OrderedMapTag:
		return orderedMapOfWalked(value, n)
	default:
		// A tag naming a kind (!!seq, !!map, !!set, !!merge) takes the node
		// as it stands.
		return value, nil
	}
}

// oneEntryOf returns the single entry of a mapping, and reports whether value
// is a mapping holding exactly one entry.
//
// The walk builds a map[string]any while all keys are strings, and a
// map[any]any once one is not, so oneEntryOf reads both. "!!omap [:]" holds
// a null key and arrives as a map[any]any.
func oneEntryOf(value any) (MapItem, bool) {
	switch m := value.(type) {
	case map[string]any:
		if len(m) != 1 {
			return MapItem{}, false
		}
		for key, v := range m {
			return MapItem{Key: key, Value: v}, true
		}
	case map[any]any:
		if len(m) != 1 {
			return MapItem{}, false
		}
		for key, v := range m {
			return MapItem{Key: key, Value: v}, true
		}
	}

	return MapItem{}, false
}

// orderedMapOfWalked folds the value built by an "!!omap" node into a [MapSliceSeq].
//
// The walk has already built the sequence and each of its mappings, so
// orderedMapOfWalked reads the values and not the nodes. It checks the shape that Decoder.orderedMapOf checks, and
// returns the same error for a value that is not a sequence of one-entry mappings. It rejects a key repeated
// across entries through the record kept by the parser on the sequence, as
// Decoder.orderedMapOf does.
//
// The key keeps its resolved value, as it does on the tree: a mapping widens
// to map[any]any on the first key that is not a string, so "!!omap [{1: a}]"
// reads uint64(1) here as well and "!!omap [:]" reads nil.
func orderedMapOfWalked(value any, n *ast.TagNode) (any, error) {
	seq, isSeq := value.([]any)
	if !isSeq {
		return nil, astutils.NotAnOrderedMap(n.Value)
	}
	for _, entry := range seq {
		if _, isMapping := oneEntryOf(entry); !isMapping {
			return nil, astutils.NotAnOrderedMap(n.Value)
		}
	}
	node, _ := ast.Unwrap(n.Value).(*ast.SequenceNode)
	if err := astutils.RefuseOrderedMapDuplicates(node); err != nil {
		return nil, err
	}

	var m MapSlice
	for _, entry := range seq {
		one, _ := oneEntryOf(entry)
		if err := m.Set(one.Key, one.Value); err != nil {
			return nil, err
		}
	}

	return MapSliceSeq(m), nil
}
