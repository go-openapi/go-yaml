// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"maps"
	"reflect"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// errStreamFallback stops a streamed encode that met a value the stream cannot write as the tree does: a collection
// used as a key, or an ast.Node the caller passed in. Encode then drops what it streamed and builds the tree.
var errStreamFallback = errors.New("codec: value needs the tree encoder")

// streams reports whether Encode writes through an [ast.Stream] instead of building the whole tree first.
//
// The tree stays for a comment map, which filters the tree by path; for smart anchors, which encode twice; for an
// anchor callback, which receives the anchor nodes; and for a flow document, which the stream does not write.
func (e *Encoder) streams() bool {
	return !e.treeOnly && e.commentMap == nil && !e.enableSmartAnchor && e.anchorCallback == nil && !e.isFlowStyle
}

// encodeStream writes v as one document through s.
//
// It walks v as encodeValue does, and streams each map, slice, array and struct encodeValue would write as a block
// collection. It builds every other value as a node and hands it to [ast.Stream.Value] whole, so the stream writes
// what [ast.Renderer.Render] writes for the same tree.
func (e *Encoder) encodeStream(s *ast.Stream, v any) error {
	if err := e.streamValue(s, reflect.ValueOf(v), 1); err != nil {
		return err
	}

	return s.Close()
}

// anchorState holds the maps an encode fills with anchors, so that a streamed encode that falls back to the tree
// can start again from them.
type anchorState struct {
	aliasRefToName  map[uintptr]string
	anchorRefToName map[uintptr]string
	anchorNameMap   map[string]struct{}
}

func (e *Encoder) saveAnchors() anchorState {
	return anchorState{
		aliasRefToName:  maps.Clone(e.aliasRefToName),
		anchorRefToName: maps.Clone(e.anchorRefToName),
		anchorNameMap:   maps.Clone(e.anchorNameMap),
	}
}

func (e *Encoder) restoreAnchors(saved anchorState) {
	e.aliasRefToName, e.anchorRefToName, e.anchorNameMap = saved.aliasRefToName, saved.anchorRefToName, saved.anchorNameMap
}

// streamKind names the collection streamValue writes as stream events.
type streamKind uint8

const (
	streamNone streamKind = iota
	streamMapping
	streamSequence
	streamStruct
	// streamChecked marks a value that passed every check of encodeValue before its kind switch: a scalar, which
	// encodeKind writes.
	streamChecked
	// streamHeldNode marks an ast.Node the caller passed in. Its comments and blank lines are kept by the tree's
	// render and dropped by a stream, so the whole document falls back to the tree.
	streamHeldNode
)

func (e *Encoder) streamValue(s *ast.Stream, v reflect.Value, column int) error {
	target, kind := e.streamTarget(v)
	switch kind {
	case streamMapping:
		return e.streamMap(s, target, column)
	case streamSequence:
		return e.streamSeq(s, target)
	case streamStruct:
		return e.streamStruct(s, target, column)
	case streamHeldNode:
		return errStreamFallback
	case streamChecked:
		// Stream.Value is done with the node when it returns, so every value shares one scratch set.
		e.scratch = &e.valueScratch
		node, err := e.encodeKind(target, column)
		e.scratch = nil
		if err != nil {
			return err
		}

		return s.Value(node)
	default:
		node, err := e.encodeValue(v, column)
		if err != nil {
			return err
		}

		return s.Value(node)
	}
}

// streamTarget returns the collection under v that the stream writes as events, and its kind, or streamNone for a
// value encodeValue writes in any other way.
//
// It runs the checks of encodeValue in the same order, and stops at the first that writes v otherwise: a null, a
// tagged time, a node, a go.yaml.in/yaml/v3 Node, a marshaler, a Base64, an alias to an anchored pointer, slice or
// map, and the MapSlice, MapSliceSeq, MapItem and time.Time structs. It unwraps pointers and interfaces as
// encodeValue does.
func (e *Encoder) streamTarget(v reflect.Value) (reflect.Value, streamKind) {
	for {
		if e.isInvalidValue(v) {
			return v, streamNone
		}
		if v.Kind() == reflect.Interface {
			if _, held := e.heldTime(v.Elem()); held {
				return v, streamNone
			}
		}
		if _, isNode := e.heldNode(v); isNode {
			return v, streamHeldNode
		}
		if _, layout := v3NodeValue(v); layout != nil {
			return v, streamNone
		}
		if e.canEncodeByMarshaler(v) || v.Type() == reflect.TypeFor[Base64]() {
			return v, streamNone
		}

		switch v.Kind() {
		case reflect.Pointer:
			if _, anchored := e.getAnchor(v.Pointer()); anchored {
				return v, streamNone
			}
			v = v.Elem()
		case reflect.Interface:
			v = v.Elem()
		case reflect.Slice:
			if _, anchored := e.getAnchor(v.Pointer()); anchored {
				return v, streamNone
			}

			return v, streamSequence
		case reflect.Array:
			return v, streamSequence
		case reflect.Map:
			if _, anchored := e.getAnchor(v.Pointer()); anchored {
				return v, streamNone
			}

			return v, streamMapping
		case reflect.Struct:
			if v.CanInterface() && isWrittenStruct(v) {
				return v, streamNone
			}
			plan, err := planOf(v.Type(), e.tagMode())
			if err != nil || plan.inlineAnchor {
				// encodeStruct returns the error, and an inline anchor field anchors the whole mapping.
				return v, streamNone
			}

			return v, streamStruct
		default:
			return v, streamChecked
		}
	}
}

// isWrittenStruct reports a struct type that encodeValue writes by itself instead of field by field.
func isWrittenStruct(v reflect.Value) bool {
	switch v.Type() {
	case reflect.TypeFor[MapSlice](), reflect.TypeFor[MapSliceSeq](), reflect.TypeFor[MapItem](), reflect.TypeFor[time.Time]():
		return true
	default:
		return false
	}
}

func (e *Encoder) streamSeq(s *ast.Stream, value reflect.Value) error {
	e.streamDepth++
	defer func() { e.streamDepth-- }()

	if e.indentSequence {
		e.column += e.indentNum
		defer func() { e.column -= e.indentNum }()
	}
	column := e.column
	if err := s.BeginSequence(); err != nil {
		return err
	}
	for i := range value.Len() {
		if err := e.streamValue(s, value.Index(i), column); err != nil {
			return err
		}
	}

	return s.End()
}

// streamMap writes a Go map's entries in the order encodeMap writes them. It builds each key before its value, where
// encodeMap builds the value first: a stream takes the key first.
func (e *Encoder) streamMap(s *ast.Stream, value reflect.Value, column int) error {
	n := value.Len()
	names, order := make([]string, n), make([]int, n)
	keys, values, order, err := e.mapEntries(value, names, order)
	if err != nil {
		return err
	}
	if err := s.BeginMapping(); err != nil {
		return err
	}
	depth := e.streamDepth
	e.streamDepth++
	defer func() { e.streamDepth-- }()
	for _, idx := range order {
		e.scratch = e.keyScratchAt(depth)
		key := e.mapKeyNode(keys.Index(idx), names[idx], column)
		e.scratch = nil
		if isCollectionNode(key) {
			return errStreamFallback
		}
		if err := s.Key(key); err != nil {
			return err
		}
		if err := e.streamValue(s, values.Index(idx), column); err != nil {
			return err
		}
	}

	return s.End()
}

// streamStruct writes a struct's fields in the order encodeStruct writes them. A field the plan marks as streamed
// writes its key and streams its value. Any other field runs structField, which encodeStruct also runs, and hands
// each entry it returns to the stream whole.
func (e *Encoder) streamStruct(s *ast.Stream, value reflect.Value, column int) error {
	plan, err := planOf(value.Type(), e.tagMode())
	if err != nil {
		return err
	}
	if err := s.BeginMapping(); err != nil {
		return err
	}
	depth := e.streamDepth
	e.streamDepth++
	defer func() { e.streamDepth-- }()
	var entries []*ast.MappingValueNode
	for _, fp := range plan.write {
		fieldValue := value.Field(fp.index)
		if e.omitsField(fp.info, fieldValue) {
			continue
		}
		if fp.streamed {
			e.scratch = e.keyScratchAt(depth)
			key := e.fieldKey(fp, column)
			e.scratch = nil
			if err := s.Key(key); err != nil {
				return err
			}
			if err := e.streamValue(s, fieldValue, column); err != nil {
				return err
			}

			continue
		}

		entries, _, err = e.structField(plan.fields, fp, fieldValue, column, entries[:0])
		if err != nil {
			return err
		}
		for _, mv := range entries {
			if isCollectionNode(mv.Key) {
				return errStreamFallback
			}
			if err := s.Key(mv.Key); err != nil {
				return err
			}
			if err := s.Value(mv.Value); err != nil {
				return err
			}
		}
	}

	return s.End()
}

// isCollectionNode reports a key the stream cannot take: a mapping or a sequence.
func isCollectionNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.MappingNode, *ast.SequenceNode:
		return true
	default:
		return false
	}
}

// scalarScratch holds one token and one node of each scalar kind, which the scalar encoders refill instead of
// allocating while Encoder.scratch points at it.
//
// The stream keeps a value's node until Value returns, and a key's node until the entry it opens is written, which
// may be after the entries of a nested collection. Values therefore share one set, Encoder.valueScratch, and keys take
// one set per depth, from keyScratchAt. A collection used as a key builds its scalars from a single set and comes out
// wrong, but the stream rejects such a key and the document falls back to the tree.
type scalarScratch struct {
	tk       token.Token
	str      ast.StringNode
	integer  ast.IntegerNode
	float    ast.FloatNode
	boolean  ast.BoolNode
	null     ast.NullNode
	infinity ast.InfinityNode
	nan      ast.NanNode
}

// keyScratchAt returns the scratch set for the keys of the collection at depth.
func (e *Encoder) keyScratchAt(depth int) *scalarScratch {
	for len(e.keyScratch) <= depth {
		e.keyScratch = append(e.keyScratch, new(scalarScratch))
	}

	return e.keyScratch[depth]
}

// newToken returns the token for value at column: the scratch token when one is set, a new token otherwise.
func (e *Encoder) newToken(value string, column int) *token.Token {
	if s := e.scratch; s != nil {
		s.tk = token.Make(value, value, e.pos(column))

		return &s.tk
	}

	return token.New(value, value, e.pos(column))
}

func (e *Encoder) stringNode(tk *token.Token) *ast.StringNode {
	if s := e.scratch; s != nil {
		s.str.Fill(tk)

		return &s.str
	}

	return ast.String(tk)
}

func (e *Encoder) integerNode(tk *token.Token) *ast.IntegerNode {
	if s := e.scratch; s != nil {
		s.integer.Fill(tk)

		return &s.integer
	}

	return ast.Integer(tk)
}

func (e *Encoder) floatNode(tk *token.Token) *ast.FloatNode {
	if s := e.scratch; s != nil {
		s.float.Fill(tk)

		return &s.float
	}

	return ast.Float(tk)
}

func (e *Encoder) boolNode(tk *token.Token) *ast.BoolNode {
	if s := e.scratch; s != nil {
		s.boolean.Fill(tk)

		return &s.boolean
	}

	return ast.Bool(tk)
}

func (e *Encoder) nullNode(tk *token.Token) *ast.NullNode {
	if s := e.scratch; s != nil {
		s.null.Fill(tk)

		return &s.null
	}

	return ast.Null(tk)
}

func (e *Encoder) infinityNode(tk *token.Token) *ast.InfinityNode {
	if s := e.scratch; s != nil {
		s.infinity.Fill(tk)

		return &s.infinity
	}

	return ast.Infinity(tk)
}

func (e *Encoder) nanNode(tk *token.Token) *ast.NanNode {
	if s := e.scratch; s != nil {
		s.nan.Fill(tk)

		return &s.nan
	}

	return ast.Nan(tk)
}
