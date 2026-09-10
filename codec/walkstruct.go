// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding"
	"errors"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// errNeedsTheTree says a document or a destination this walk cannot serve.
// The decode reads the source again and builds a tree for it.
//
// SPIKE. What it refuses is the measure of what is left to build: anchors,
// aliases and merge keys, which need a subtree the walk has handed over; any
// destination holding an interface, a custom unmarshaler, an ast.Node, a
// non-string map key, or a Go kind the setter below does not write.
var errNeedsTheTree = errors.New("this decode needs the tree")

type typedFrameKind uint8

const (
	typedStruct typedFrameKind = iota
	typedMap
	typedSlice
)

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
}

// typedBuilder fills a Go value as the parse hands the document over, without
// a tree in between.
type typedBuilder struct {
	dec   *Decoder
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
	budget  int
}

// holdsAnything reports a destination that takes whatever the document writes.
func holdsAnything(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.Type().NumMethod() == 0
}

// forward hands a node to the nested value builder and closes the subtree once
// the builder has finished one.
func (b *typedBuilder) forward(node ast.Node, at parser.Step, entering bool) bool {
	var proceed bool
	if entering {
		proceed = b.values.Enter(node, at)
	} else {
		b.values.Leave(node, at)
	}
	if b.values.err != nil {
		b.fail(b.values.err)

		return false
	}
	b.closeAny()

	return proceed
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

func (b *typedBuilder) fail(err error) {
	if b.err == nil {
		b.err = err
	}
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
	}
	top.target = reflect.Value{}
}

func (b *typedBuilder) Enter(node ast.Node, at parser.Step) bool {
	if b.err != nil {
		return false
	}

	switch node.(type) {
	case *ast.AnchorNode, *ast.AliasNode, *ast.TagNode, *ast.MappingKeyNode:
		// A property or an alias, inside an "any" subtree or out of one. The
		// nested builder reads anchors of its own and would answer for the
		// document's, so both go to the tree together.
		b.fail(errNeedsTheTree)

		return false
	case *ast.CommentGroupNode:
		return false
	}

	if b.values != nil {
		return b.forward(node, at, true)
	}

	// A key names the entry that follows rather than being a value, so it is
	// read before the destination is looked at.
	if at.Key && len(b.stack) > 0 {
		return b.openEntry(&b.stack[len(b.stack)-1], node)
	}

	// Read where this goes once and hand it on: destination makes a sequence's
	// entry as it is asked for, so asking twice makes two.
	dst, wanted := b.destination()
	if wanted && holdsAnything(dst) {
		b.anyInto = dst
		b.values = &valueBuilder{share: b.share, budget: b.budget}

		return b.forward(node, at, true)
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

func (b *typedBuilder) openMapping(dst reflect.Value, wanted bool) bool {
	if !wanted {
		// Inside an entry no field claims. Read the mapping and drop it.
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dropped: true})

		return true
	}

	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		b.fail(errNeedsTheTree)

		return false
	}
	switch dst.Kind() {
	case reflect.Struct:
		b.stack = append(b.stack, typedFrame{kind: typedStruct, dst: dst, out: out})

		return true
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			b.fail(errNeedsTheTree)

			return false
		}
		if dst.IsNil() {
			if !dst.CanSet() {
				b.fail(errNeedsTheTree)

				return false
			}
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		b.stack = append(b.stack, typedFrame{kind: typedMap, dst: dst, out: out})

		return true
	default:
		b.fail(errNeedsTheTree)

		return false
	}
}

func (b *typedBuilder) openSequence(dst reflect.Value, wanted bool) bool {
	if !wanted {
		b.stack = append(b.stack, typedFrame{kind: typedSlice, dropped: true})

		return true
	}

	out := dst
	dst = b.indirect(dst)
	if b.handled(dst) {
		b.fail(errNeedsTheTree)

		return false
	}
	if dst.Kind() != reflect.Slice || !dst.CanSet() {
		b.fail(errNeedsTheTree)

		return false
	}
	// An empty sequence is an empty slice and not a nil one, which is what the
	// tree decoder gives and what a caller comparing against "[]T{}" expects.
	if dst.IsNil() {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
	} else {
		dst.Set(dst.Slice(0, 0))
	}
	b.stack = append(b.stack, typedFrame{kind: typedSlice, dst: dst, out: out})

	return true
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
func (b *typedBuilder) scalar(node ast.Node, dst reflect.Value, wanted bool) bool {
	if !wanted {
		b.settle(reflect.Value{})

		return false
	}
	if err := b.setScalar(dst, node); err != nil {
		b.fail(err)

		return false
	}
	b.settle(dst)

	return false
}

// openEntry names the entry a key opens, and says where its value goes.
func (b *typedBuilder) openEntry(top *typedFrame, keyNode ast.Node) bool {
	if top.dropped {
		return false
	}

	if key, isKey := keyNode.(ast.MapKeyNode); isKey && key.IsMergeKey() {
		// "<<" folds another mapping's entries into this one, which the walk
		// does not do yet. A merge written as an alias gives up on the alias;
		// one written in place -- "<<: {a: 1}" -- has nothing else to give up
		// on, and was read as a key no field claims and dropped.
		b.fail(errNeedsTheTree)

		return false
	}

	name, named := entryText(keyNode)
	if !named {
		if top.kind == typedMap {
			b.fail(errNeedsTheTree)

			return false
		}
		// No field can be named after it.
		top.skip = true

		return false
	}

	switch top.kind {
	case typedStruct:
		fields, err := structFields(top.dst.Type(), b.dec.tagMode())
		if err != nil {
			b.fail(err)

			return false
		}
		sf, at, _, known := fields.lookup(name)
		if !known {
			top.skip, top.target = true, reflect.Value{}

			return false
		}
		if at != nil {
			// The type embeds another, and the name reaches a field of it.
			field, err := fieldAt(top.dst, at)
			if err != nil {
				b.fail(err)

				return false
			}
			top.skip, top.target = false, field

			return false
		}
		top.skip = false
		top.target = top.dst.Field(sf.Index)
	case typedMap:
		top.skip = false
		top.key = reflect.ValueOf(b.strs.clone(name)).Convert(top.dst.Type().Key())
		top.target = reflect.New(top.dst.Type().Elem()).Elem()
	case typedSlice:
		b.fail(errNeedsTheTree)

		return false
	}

	return false
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

func (b *typedBuilder) Leave(node ast.Node, at parser.Step) {
	if b.err != nil {
		return
	}
	if b.values != nil {
		b.forward(node, at, false)

		return
	}
	switch node.(type) {
	case *ast.MappingNode, *ast.SequenceNode:
	default:
		return
	}

	if _, isMapping := node.(*ast.MappingNode); isMapping {
		// The parser hangs the repeats on the mapping as it closes, so they are
		// read here rather than as it opened.
		if err := refuseDuplicateKeys(node); err != nil {
			b.fail(err)

			return
		}
	}

	frame := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	if frame.dropped {
		b.settle(reflect.Value{})

		return
	}
	b.settle(frame.out)
}

// handled reports a destination the tree decoder reads for itself: a custom
// unmarshaler, a time.Time or time.Duration, a MapSlice. This walk writes none
// of them.
func (b *typedBuilder) handled(dst reflect.Value) bool {
	if !dst.CanAddr() {

		return true
	}
	if b.dec.canDecodeByUnmarshaler(dst) {

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
var walkableTypes [modeCount]sync.Map

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
		return cached.(bool)
	}
	ok := readWalkable(t, map[reflect.Type]bool{}, mode)
	cache.Store(t, ok)

	return ok
}

var (
	unmarshalerTypes = []reflect.Type{
		reflect.TypeFor[ContextUnmarshaler](),
		reflect.TypeFor[Unmarshaler](),
		reflect.TypeFor[ContextGoYAMLUnmarshaler](),
		reflect.TypeFor[GoYAMLUnmarshaler](),
		reflect.TypeFor[NodeUnmarshaler](),
		reflect.TypeFor[ContextNodeUnmarshaler](),
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

// walkInto reads src into v by walking it. It returns errNeedsTheTree where the
// document or the destination needs one.
func (d *Decoder) walkInto(src []byte, v reflect.Value) error {
	b := &typedBuilder{
		dec:    d,
		root:   v.Elem(),
		strs:   &d.strs,
		share:  d.shareAliases,
		budget: aliasBudget(len(src)),
	}
	if _, err := parser.New(parser.WithOmitNodePaths()).Walk(src, b); err != nil {
		return err
	}

	return b.err
}

// typedWalkEnabled turns the spike on. SPIKE.
const typedWalkEnabled = true
