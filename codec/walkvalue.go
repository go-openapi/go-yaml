// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// valueBuilder folds a document into Go values as the parse reaches each node,
// so that a document is read once rather than built into a tree and walked.
//
// It is codec.jsonTokener's walk with a different fold: the same Enter and
// Leave over the same nodes, keeping Go values where the converter keeps JSON
// tokens.
// Four things differ, and they are what a JSON writer has no use for -- a
// number too wide for a machine word stays a *big.Int or *big.Float, a
// "!!timestamp" becomes a time.Time, "!!binary" becomes []byte, and an infinity
// stays one where JSON has to refuse it.
type valueBuilder struct {
	// docs holds the value of each document body, in stream order.
	docs []any
	// stack is what is being filled, innermost last: a mapping, a sequence, or
	// a property standing around the node it names.
	stack []buildFrame
	// named holds what each anchor of the document named, for an alias to name
	// again. It is emptied at each document, since an anchor belongs to one.
	named map[string]any
	// declared holds the anchors parser.WithAnchors passed, which the walk
	// never hands over, and declaredValues the value each one built the first
	// time an alias named it. Both serve every document of the stream.
	declared       map[string]ast.Node
	declaredValues map[string]any
	// strs holds the text of every string this builds, copied out of the
	// document so that the values outlive it.
	strs arena
	// open holds the anchors whose node is being read, innermost last. An alias
	// naming one of them stands inside what it names.
	open []string
	// share hands every alias of one anchor the same value. See
	// codec.ShareAliases.
	share bool
	// built counts the values copied for an alias, and budget bounds them. A
	// chain of aliases multiplies, so an independent copy is what a document
	// written to exhaust memory abuses.
	built  int
	budget int
	err    error
}

type frameKind uint8

const (
	// frameMapping and frameSequence are collections being filled.
	frameMapping frameKind = iota
	frameSequence
	// frameProperty is an anchor, a tag or a "?" standing around one node. It
	// takes the value that node builds and hands it on transformed.
	frameProperty
)

type buildFrame struct {
	kind frameKind
	// asKey records whether the node this frame stands for arrived as a mapping key,
	// since its Leave delivers what it built and the walk has moved on by then.
	asKey bool

	// mapping
	//
	// m holds a mapping whose keys are all strings, which is nearly all of
	// them. anyKeyed replaces it on the first key that resolves to anything
	// else -- an integer, a float, a null, a Base64, a time.Time -- so the key
	// keeps the value it resolves to rather than the text that spells it, and
	// two keys alike in text stay two entries.
	m        map[string]any
	anyKeyed map[any]any
	// key is the arena's copy of the name the entry is written under, keyValue
	// what the key resolves to, and keyIsString which of the two a widened
	// mapping writes.
	key         string
	keyValue    any
	keyIsString bool
	hasKey      bool
	merging     bool
	// broughtByValue says a "<<" brought in a key a Go map lookup cannot match
	// by value -- a time.Time, a *big.Int, a *big.Float. An own key that is one
	// YAML key with it then takes that entry's place.
	broughtByValue bool
	// keyErr is a key this destination cannot hold, kept until the mapping
	// closes. The parser hangs a mapping's repeated keys on it as it closes, so
	// reporting an unusable key where it is met would speak over a repeat --
	// "{[a]: 1, [ a ]: 2}" is a repeat first and an unusable key second.
	keyErr error

	// sequence
	seq []any
	// mergeSource says the sequence is the value of a "<<", so every element
	// has to be a mapping. Each is checked as it arrives, where its node is to
	// hand for the complaint to point at.
	mergeSource bool

	// property
	node     ast.Node
	received any
	got      bool
}

// Enter is called before anything a node holds.
func (b *valueBuilder) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsRoot() {
		if _, isDirective := node.(*ast.DirectiveNode); isDirective {
			// A directive opens a document of its own and holds no value.
			return parser.SkipNode
		}
		b.named = nil
	}

	if isMergeKey(node) {
		// "<<" names no key of its own: what it brings in is folded into the
		// mapping when the value below it arrives.
		if n := len(b.stack); n > 0 && b.stack[n-1].kind == frameMapping {
			b.stack[n-1].merging = true
		}

		return parser.SkipNode
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		b.stack = append(b.stack, buildFrame{kind: frameMapping, asKey: at.IsKey(), m: map[string]any{}})
	case *ast.SequenceNode:
		// An empty sequence is an empty slice and not a nil one, which is what
		// the tree-walking decoder gives and what a caller comparing against
		// "[]any{}" expects.
		b.stack = append(b.stack,
			buildFrame{kind: frameSequence, asKey: at.IsKey(), seq: []any{}, mergeSource: b.mergingValue(at.IsKey())})
	case *ast.AnchorNode:
		b.open = append(b.open, anchorName(n.Name))
		b.stack = append(b.stack, buildFrame{kind: frameProperty, asKey: at.IsKey(), node: node})
	case *ast.TagNode, *ast.MappingKeyNode:
		b.stack = append(b.stack, buildFrame{kind: frameProperty, asKey: at.IsKey(), node: node})
	case *ast.AliasNode:
		b.deliver(b.aliasValue(n), node, at.IsKey())
	case *ast.CommentGroupNode:
		return parser.SkipNode
	default:
		v, err := b.scalarValue(node)
		if err != nil {
			return b.fail(err)
		}
		b.deliver(v, node, at.IsKey())
	}

	// aliasValue and copyValue record their own refusals, so the walk is told here.
	return b.err
}

// Leave is called once everything a node holds has been.
func (b *valueBuilder) Leave(node ast.Node, at parser.Closing) error {
	if b.err != nil {
		// The walk hands nothing more over once a visitor has failed, and still leaves the nodes it had open.
		// The frames below stay as they were, since nothing reads them again.
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
		// The parser hangs the repeats on the mapping as it closes, so they are
		// read here rather than as it opened.
		if err := refuseDuplicateKeys(node); err != nil {
			return b.fail(err)
		}
		if frame.keyErr != nil {
			return b.fail(frame.keyErr)
		}
		b.deliver(frame.value(), node, frame.asKey)
	case frameSequence:
		b.deliver(frame.seq, node, frame.asKey)
	case frameProperty:
		v, err := b.closeProperty(frame)
		if err != nil {
			return b.fail(err)
		}
		b.deliver(v, node, frame.asKey)
	}

	return b.err
}

// closeProperty turns what a property's node built into what the property makes
// of it.
func (b *valueBuilder) closeProperty(frame buildFrame) (any, error) {
	value := frame.received
	if !frame.got {
		// A tag on a scalar hands nothing over -- parseScalarValue builds the
		// value without going through parseToken -- so it is read from the node
		// here, as jsonTokener.closeTag does. An anchor between a tag and its
		// scalar is the same shape. A property standing on nothing reads as the
		// empty node.
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
		if name := anchorName(n.Name); name != "" {
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
			// The anchor closed before the tag was applied and recorded what it
			// held untagged. 6.9 gives the node both properties, so the name
			// stands for the tagged value: "a: !!int &a1 \"5\"" reads 5 at a,
			// and an alias to a1 reads 5 rather than "5".
			//
			// A flow key written alone, "{!!null &a1 null, k: *a1}", hands the
			// tag over without what it stands on, so the anchor never opened
			// and this is the only place its name is recorded.
			if name := anchorName(anchor.Name); name != "" {
				if b.named == nil {
					b.named = map[string]any{}
				}
				b.named[name] = tagged
			}
		}

		return tagged, nil
	default:
		// A "?" key stands around the node it addresses the entry by.
		return value, nil
	}
}

// propertyValue reads the node a property stands on, for the scalars the parse
// builds without handing over.
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

// aliasValue returns what an alias names: the value the document's anchor
// recorded as it closed, or else the value of an anchor parser.WithAnchors
// declared.
func (b *valueBuilder) aliasValue(n *ast.AliasNode) any {
	name := anchorName(n.Value)
	if v, named := b.named[name]; named {
		if b.share {
			return v
		}

		// Two aliases of one anchor give two values, so writing through one
		// leaves the other alone. The walk has handed the anchored nodes over
		// already and cannot read them again, so the value it built is copied
		// rather than rebuilt. See codec.ShareAliases.
		return b.copyValue(v, n)
	}
	if slices.Contains(b.open, name) {
		// The anchor is being read right now, so the alias stands inside what
		// its own anchor names. A Go value is built by walking and has nowhere
		// to put a cycle. AliasNode.Target does not answer this during a walk:
		// the parser fills it at the document's end, which is after the alias
		// has been handed over.
		b.fail(yamlerrors.NewRecursiveAlias(name, n.GetToken()))

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

	b.fail(yamlerrors.NewUnknownAnchor(name, n.GetToken()))

	return nil
}

// declaredValue returns the value a declared anchor names, built once from the
// tree the caller passed.
//
// The tree comes from another parse and is whole, so the tree decoder reads it.
// Built once, every alias of the anchor gets the same value under ShareAliases,
// as it does for an anchor of the document.
func (b *valueBuilder) declaredValue(name string, node ast.Node, alias *ast.AliasNode) (any, error) {
	if v, built := b.declaredValues[name]; built {
		return v, nil
	}
	if node == nil {
		// The parser takes a nil entry as a declaration, and ToJSON refuses it
		// the same way.
		return nil, yamlerrors.NewUnknownAnchor(name, alias.GetToken())
	}

	dec := NewDecoder(nil)
	dec.shareAliases = b.share
	v, err := dec.nodeToValue(context.Background(), node)
	if err != nil {
		return nil, err
	}
	if b.declaredValues == nil {
		b.declaredValues = map[string]any{}
	}
	b.declaredValues[name] = v

	return v, nil
}

// copyValue returns v with nothing shared with it: a map and a slice are built
// again and what they hold is copied in turn, and everything else is a value a
// caller cannot write through.
func (b *valueBuilder) copyValue(v any, at ast.Node) any {
	if b.err != nil {
		return nil
	}
	b.built++
	if b.budget > 0 && b.built > b.budget {
		b.fail(yamlerrors.NewExcessiveAliasing(b.built, b.budget, at.GetToken()))

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

// deliver puts a value where the node that built it belongs.
func (b *valueBuilder) deliver(v any, node ast.Node, key bool) {
	if n := len(b.stack); n > 0 {
		top := &b.stack[n-1]
		switch top.kind {
		case frameProperty:
			top.received, top.got = v, true

			return
		case frameMapping:
			if key {
				// Named from the node and not from the value, so that the type
				// spells it: mapKeyString writes an integer in decimal and a
				// float with the point that tells it from one.
				name, err := mapKeyString(node, v)
				if err != nil {
					if top.keyErr == nil {
						top.keyErr = err
					}
					top.key, top.hasKey = "", false

					return
				}
				top.keyed(b.strs.clone(name), v)

				return
			}
			if top.merging {
				b.merge(top, v, node)
				top.merging = false

				return
			}
			top.put(v)
			top.key, top.hasKey = "", false

			return
		case frameSequence:
			if top.mergeSource && !isMapValue(v) {
				// "<<" takes a mapping or a sequence of them, and nothing else:
				// not a null, and not a sequence inside the sequence. mergingValue
				// set the flag only on a sequence the merging mapping holds, so
				// that mapping stands just below it.
				b.stack[n-2].refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

				return
			}
			top.seq = append(top.seq, v)

			return
		}
	}

	b.docs = append(b.docs, v)
}

// keyed records the key an entry is being written under, and whether it is a
// string.
//
// The name is kept for a string key so the arena's copy is the one the map
// holds; keyValue carries what the key resolves to for every other type, which
// is what the mapping is widened to hold.
func (f *buildFrame) keyed(name string, value any) {
	f.key, f.hasKey = name, true
	_, f.keyIsString = value.(string)
	f.keyValue = value
}

// put writes value under the key the frame is holding, widening the mapping
// where the key is not a string.
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
		// The mapping's own key replaces what a "<<" brought in under it: for a
		// timestamp the same instant in any zone, for a wide number its value.
		if held, found := sameKeyIn(f.anyKeyed, key); found {
			delete(f.anyKeyed, held)
		}
	}
	f.anyKeyed[key] = value
}

// mapKey is the key a widened mapping is written under: the arena's copy of the
// name for a string, and what the key resolves to for anything else.
func (f *buildFrame) mapKey() any {
	if f.keyIsString {
		return f.key
	}

	return f.keyValue
}

// widen moves a mapping from map[string]any to map[any]any, which happens on
// the first key that is not a string.
//
// The keys already written are strings and go over as they stand. A document
// whose keys are all strings never reaches this, and keeps the map a caller has
// always been handed.
func (f *buildFrame) widen() {
	f.anyKeyed = make(map[any]any, len(f.m)+1)
	for key, value := range f.m {
		f.anyKeyed[key] = value
	}
	f.m = nil
}

// value is the mapping the frame built, under whichever key type it settled on.
func (f *buildFrame) value() any {
	if f.anyKeyed != nil {
		return f.anyKeyed
	}

	return f.m
}

// holds reports whether the mapping already writes key, which a "<<" asks
// before bringing one in.
func (f *buildFrame) holds(key any) bool {
	if f.anyKeyed != nil {
		if _, held := f.anyKeyed[key]; held {
			return true
		}
		_, held := sameKeyIn(f.anyKeyed, key)

		return held
	}
	name, isString := key.(string)
	if !isString {
		return false
	}
	_, held := f.m[name]

	return held
}

// bring writes what a "<<" names under key, where the mapping holds none.
func (f *buildFrame) bring(key any, value any) {
	if _, isString := key.(string); isString && f.anyKeyed == nil {
		f.m[key.(string)] = value

		return
	}
	if f.anyKeyed == nil {
		f.widen()
	}
	if comparesByValue(key) {
		f.broughtByValue = true
	}
	f.anyKeyed[key] = value
}

// merge folds what a "<<" names into the mapping holding it.
//
// The merged entries are defaults: a key the mapping writes itself wins,
// whichever side of the "<<" it stands on. Writing them without overwriting
// gives that either way -- a "<<" written first fills an empty mapping and the
// mapping's own keys replace what they repeat, and one written last finds those
// keys already there.
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
			if !isMapValue(one) {
				// Reached through an anchor or an alias, where the element's own
				// node is not to hand; a sequence written in place was checked
				// element by element as it was built.
				top.refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

				return
			}
			b.merge(top, one, node)
		}
	default:
		// "<<" takes a mapping or a sequence of them, and nothing else. A null
		// is no mapping: "<<:" with no value is refused, as the tree refuses it.
		top.refuseMerge(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))
	}
}

// refuseMerge keeps err on the mapping a "<<" stands in, for its close to
// report after the repeats the parse recorded. "{<<: {x: 1}, <<: }" repeats a
// key before it merges a null, and the tree reports the repeat.
func (f *buildFrame) refuseMerge(err error) {
	if f.keyErr == nil {
		f.keyErr = err
	}
}

// mergingValue reports whether the node entering at at is the value of a "<<".
func (b *valueBuilder) mergingValue(key bool) bool {
	n := len(b.stack)

	return n > 0 && b.stack[n-1].kind == frameMapping && b.stack[n-1].merging && !key
}

// isMapValue reports whether v is a mapping the walk built.
func isMapValue(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any:
		return true
	default:
		return false
	}
}

// fail keeps the first error and returns it, so a caller can return it to the walk in one line.
func (b *valueBuilder) fail(err error) error {
	if b.err == nil {
		b.err = err
	}

	return b.err
}

// scalarValue reads the Go value a scalar node denotes.
func (b *valueBuilder) scalarValue(n ast.Node) (any, error) {
	switch t := n.(type) {
	case *ast.NullNode:
		return nil, nil
	case *ast.LiteralNode:
		if t.Value == nil {
			return "", nil
		}

		return b.strs.value(t.Value.Value), nil
	case *ast.StringNode:
		return b.strs.value(t.Value), nil
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
			return b.strs.value(res.Text), nil
		}

		return nil, yamlerrors.NewSyntax(
			fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), n.Value.GetToken())
	}

	if res.Empty {
		return tagZero(res.Tag)
	}

	// What each tag makes of the node is where this parts company with the JSON
	// converter: a number too wide for a machine word stays a *big.Int or
	// *big.Float, "!!binary" is bytes and "!!timestamp" a time.Time, none of
	// which JSON has a spelling for.
	switch res.Tag {
	case token.StringTag:
		return b.strs.value(res.Text), nil
	case token.NullTag:
		return nil, nil
	case token.IntegerTag:
		// From the text and the document's schema, as Decoder.taggedValue
		// reads it. Converting the walked value instead took a quoted scalar
		// as the string it is, and the two paths parted on `!!float -0`. It
		// keeps the Go type the untagged number decodes to.
		return taggedInteger(res.Text, res.Schema), nil
	case token.FloatTag:
		return castToFloatValue(taggedFloat(res.Text, res.Schema)), nil
	case token.BooleanTag:
		// Resolve has agreed there is a boolean to read, in whatever case the
		// document wrote it.
		b, _ := token.ParseBool(strings.ToLower(res.Text))

		return b, nil
	case token.BinaryTag:
		// A Base64, as Decoder.taggedValue reads it. The two paths have to
		// agree, and they parted on "!!float -0" once already.
		return Base64(res.Text), nil
	case token.TimestampTag:
		t, _ := ast.ParseTimestamp(res.Text)

		return t, nil
	case token.OrderedMapTag:
		return orderedMapOfWalked(value, n)
	default:
		// A tag naming a kind -- !!seq, !!map, !!set, !!merge -- takes the node
		// as it stands.
		return value, nil
	}
}

// oneEntryOf is the single entry a mapping holds, and whether it is a mapping
// holding exactly one.
//
// A mapping the walk built is a map[string]any while its keys are all strings
// and a map[any]any once one is not, so both are read here -- "!!omap [:]"
// holds a null key and arrives as the second.
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

// orderedMapOfWalked folds what an "!!omap" node built into a [MapSliceSeq].
//
// The walk has already built the sequence and each of its mappings, so this
// reads the values and not the nodes. It holds the shape Decoder.orderedMapOf
// holds and is lenient the same way: a sequence that is not one-entry mappings
// stands as it is written. A key repeated across entries is refused through the
// record the parser keeps on the sequence, as Decoder.orderedMapOf refuses it.
//
// The key keeps the value it resolves to, as it does on the tree: a mapping
// widens to map[any]any on the first key that is not a string, so
// "!!omap [{1: a}]" reads uint64(1) here as well and "!!omap [:]" reads nil.
func orderedMapOfWalked(value any, n *ast.TagNode) (any, error) {
	seq, isSeq := value.([]any)
	if !isSeq {
		return nil, notAnOrderedMap(n.Value)
	}
	for _, entry := range seq {
		if _, isMapping := oneEntryOf(entry); !isMapping {
			return nil, notAnOrderedMap(n.Value)
		}
	}
	if err := refuseOrderedMapDuplicates(orderedMapSequence(n.Value)); err != nil {
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

// WalkValue reads the first document of src by walking it. EXPERIMENT.
func WalkValue(src []byte) (any, error) {
	docs, err := WalkValues(src)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}

	return docs[0], nil
}

// WalkValues reads every document of src by walking it, without building a
// tree.
func WalkValues(src []byte, opts ...parser.Option) ([]any, error) {
	return walkValues(src, false, opts...)
}

func walkValues(src []byte, share bool, opts ...parser.Option) ([]any, error) {
	opts = append(opts, parser.WithOmitNodePaths())
	p := parser.New(opts...)
	b := &valueBuilder{share: share, budget: aliasBudget(len(src)), declared: p.DeclaredAnchors()}

	file, err := p.Walk(src, b)
	if err != nil {
		return nil, err
	}
	if b.err != nil {
		return nil, b.err
	}

	return b.documentValues(file), nil
}

// documentValues lines the values up with the documents of the stream.
//
// A document holding no node hands nothing over -- there is nothing for the
// walk to visit -- so a value has to be put back for it. "%YAML 1.1" over "---"
// and "# comment" over "..." are both documents denoting null: a marker opens
// or closes a document whatever stands between the markers, and a directive or
// a run of comments is not a node. isEmptyDocument names exactly those, and
// holdsAValue drops what is no document at all.
//
// The file the walk returns holds the documents, which is all this reads: the
// markers around each body and the type of the body, and no token.
func (b *valueBuilder) documentValues(file *ast.File) []any {
	if file == nil || len(file.Docs) == 0 {
		return b.docs
	}

	var (
		values []any
		walked int
	)
	for _, doc := range file.Docs {
		if !holdsAValue(doc) {
			continue
		}
		if isEmptyDocument(doc) {
			values = append(values, nil)

			continue
		}
		if walked < len(b.docs) {
			values = append(values, b.docs[walked])
			walked++
		}
	}

	return values
}
