// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/astutils"
	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
	"github.com/go-openapi/go-yaml/codec/internal/structfields"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Encoder writes Go values to a YAML stream, one document for each call to [Encoder.Encode].
type Encoder struct {
	writer             io.Writer
	opts               []EncodeOption
	singleQuote        bool
	isFlowStyle        bool
	isJSONStyle        bool
	useJSONMarshaler   bool
	enableSmartAnchor  bool
	aliasRefToName     map[uintptr]string
	anchorRefToName    map[uintptr]string
	anchorNameMap      map[string]struct{}
	anchorCallback     func(*ast.AnchorNode, any) error
	customMarshalerMap map[reflect.Type]func(any) ([]byte, error)
	// noCustomMarshalers records, for one EncodeToNode call, that neither the Encoder nor RegisterCustomMarshaler
	// holds a custom marshaler, so the lookups for one can be skipped on every value. False on a zero Encoder.
	noCustomMarshalers bool
	// treeOnly makes Encode build the whole tree even where it could stream. Tests set it to compare the two.
	treeOnly bool
	// scratch, when set, makes the scalar encoders refill its token and node instead of allocating. The stream sets
	// it around one scalar at a time; see scalarScratch.
	scratch *scalarScratch
	// valueScratch serves every value the stream writes, and keyScratch one key per depth of the stream.
	valueScratch               scalarScratch
	keyScratch                 []*scalarScratch
	streamDepth                int
	omitZero                   bool
	omitEmpty                  bool
	writeJSONTags              bool
	writeInferredNames         bool
	autoInt                    bool
	useLiteralStyleIfMultiline bool
	skipDuplicateMapKey        bool
	commentMap                 map[nodeFilter][]*Comment
	written                    bool

	line           int
	column         int
	offset         int
	indentNum      int
	indentSequence bool
}

// NewEncoder returns an encoder that writes to w, with the given options. Each call to [Encoder.Encode] writes a whole
// document to w, so there is nothing to flush.
func NewEncoder(w io.Writer, opts ...EncodeOption) *Encoder {
	return &Encoder{
		writer:             w,
		opts:               opts,
		customMarshalerMap: map[reflect.Type]func(any) ([]byte, error){},
		line:               1,
		column:             1,
		offset:             0,
		indentNum:          DefaultIndentSpaces,
		anchorRefToName:    make(map[uintptr]string),
		anchorNameMap:      make(map[string]struct{}),
		aliasRefToName:     make(map[uintptr]string),
	}
}

// Encode writes v to the stream as one document. Every document after the first opens with a "---" separator.
//
// See [Marshal] for how a Go value maps to YAML.
//
// Encode writes the document as it walks v, through an [ast.Stream], and keeps only the output in memory. It builds
// the whole tree first under [WithComment], [WithSmartAnchor], [MarshalAnchor], [Flow] and [JSON]. Either way an
// error writes nothing to the stream.
func (e *Encoder) Encode(v any) error {
	if err := e.prepare(); err != nil {
		return err
	}
	// Lay the document out from the tree, not from the positions held by the
	// nodes. The encoder's indentation options take effect here.
	renderer := ast.NewRenderer(
		ast.WithIndent(e.indentNum),
		ast.WithIndentSequence(e.indentSequence),
	)

	if e.streams() {
		saved := e.saveAnchors()
		var out bytes.Buffer
		err := e.encodeStream(renderer.Stream(&out), v)
		switch {
		case err == nil:
			if err := e.separate(); err != nil {
				return err
			}
			_, err = e.writer.Write(out.Bytes())

			return err
		case !errors.Is(err, errStreamFallback):
			return err
		default:
			e.restoreAnchors(saved)
		}
	}

	node, err := e.encodeTree(v)
	if err != nil {
		return err
	}
	if err := e.setCommentByCommentMap(node); err != nil {
		return err
	}
	if err := e.separate(); err != nil {
		return err
	}
	if err := renderer.Render(e.writer, node); err != nil {
		return err
	}
	_, err = io.WriteString(e.writer, "\n")

	return err
}

// EncodeToNode returns the AST node that [Encoder.Encode] would write for v, without writing it.
func (e *Encoder) EncodeToNode(v any) (ast.Node, error) {
	if err := e.prepare(); err != nil {
		return nil, err
	}

	return e.encodeTree(v)
}

// separate writes the "---" that opens every document after the first.
func (e *Encoder) separate() error {
	if e.written {
		if _, err := io.WriteString(e.writer, "---\n"); err != nil {
			return err
		}
	}
	e.written = true

	return nil
}

// prepare applies the options, once per call.
func (e *Encoder) prepare() error {
	for _, opt := range e.opts {
		if err := opt(e); err != nil {
			return err
		}
	}
	// Read once per call, never cached across calls: a RegisterCustomMarshaler between two calls takes effect.
	e.noCustomMarshalers = len(e.customMarshalerMap) == 0 && !hasCustomMarshalers()

	return nil
}

// encodeTree builds the node tree of v.
//
// The root is v's interface slot, as an element of a []any or a value of a map[string]any is: a time.Time there
// takes its "!!timestamp" tag, so a reader into any gets a time back and not a string.
func (e *Encoder) encodeTree(v any) (ast.Node, error) {
	root := reflect.ValueOf(&v).Elem()
	if e.enableSmartAnchor {
		// The first encoding stores every mapping from an alias address to its name.
		if _, err := e.encodeValue(root, 1); err != nil {
			return nil, err
		}
		e.clearSmartAnchorRef()
	}
	node, err := e.encodeValue(root, 1)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (e *Encoder) setCommentByCommentMap(node ast.Node) error {
	if e.commentMap == nil {
		return nil
	}

	for path, comments := range e.commentMap {
		n, err := path.FilterNode(node)
		if err != nil {
			return err
		}
		if n == nil {
			continue
		}
		for _, comment := range comments {
			commentTokens := []*token.Token{}
			for _, text := range comment.Texts {
				commentTokens = append(commentTokens, token.New(text, text, token.Position{}))
			}
			commentGroup := ast.CommentGroup(commentTokens)
			switch comment.Position {
			case CommentHeadPosition:
				if err := astutils.PlaceHeadComment(node, n, commentGroup); err != nil {
					return err
				}
			case CommentLinePosition:
				if err := astutils.PlaceLineComment(node, n, commentGroup); err != nil {
					return err
				}
			case CommentFootPosition:
				if err := astutils.PlaceFootComment(node, n, commentGroup); err != nil {
					return err
				}
			default:
				return yamlerrors.ErrUnknownCommentPositionType
			}
		}
	}

	return nil
}

func (e *Encoder) encodeDocument(doc []byte) (ast.Node, error) {
	f, err := parser.ParseBytes(doc)
	if err != nil {
		return nil, err
	}

	for _, docNode := range f.Docs {
		if docNode.Body != nil {
			return docNode.Body, nil
		}
	}

	return nil, nil
}

func (e *Encoder) isInvalidValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	kind := v.Type().Kind()
	if kind == reflect.Pointer && v.IsNil() {
		return true
	}
	if kind == reflect.Interface && v.IsNil() {
		return true
	}
	return false
}

func (e *Encoder) existsTypeInCustomMarshalerMap(t reflect.Type) bool {
	if e.noCustomMarshalers {
		return false
	}
	if _, exists := e.customMarshalerMap[t]; exists {
		return true
	}

	customMarshalerMx.RLock()
	defer customMarshalerMx.RUnlock()
	if _, exists := customMarshalerMap[t]; exists {
		return true
	}
	return false
}

func (e *Encoder) marshalerFromCustomMarshalerMap(t reflect.Type) (func(any) ([]byte, error), bool) {
	if e.noCustomMarshalers {
		return nil, false
	}
	if marshaler, exists := e.customMarshalerMap[t]; exists {
		return marshaler, exists
	}

	customMarshalerMx.RLock()
	defer customMarshalerMx.RUnlock()
	if marshaler, exists := customMarshalerMap[t]; exists {
		return marshaler, exists
	}
	return nil, false
}

// heldNode returns the node held by v, for a caller encoding a parsed or built tree.
//
// The encoder writes the node as it stands, so a document keeps its comments, its anchors and its tags.
func (e *Encoder) heldNode(v reflect.Value) (ast.Node, bool) {
	if !v.CanInterface() {
		return nil, false
	}
	return reflect.TypeAssert[ast.Node](v)
}

func (e *Encoder) canEncodeByMarshaler(v reflect.Value) bool {
	if !v.CanInterface() {
		return false
	}
	if e.existsTypeInCustomMarshalerMap(v.Type()) {
		return true
	}
	t := v.Type()
	if t.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		t = v.Elem().Type()
	}

	switch marshalerKindOf(t) {
	case marshalsItself:
		return true
	case marshalsAsJSON:
		return e.useJSONMarshaler
	default:
		return false
	}
}

// marshalerKind records how a Go type is written: by the encoder, by a method of its own, or by MarshalJSON when
// [UseJSONMarshaler] is set.
type marshalerKind uint8

const (
	encodedByTheEncoder marshalerKind = iota
	marshalsItself
	marshalsAsJSON
)

// marshalerKinds caches marshalerKindOf by type. Marshal builds a new Encoder on every call, so a cache held by the
// Encoder would start empty each time.
var marshalerKinds sync.Map // reflect.Type -> marshalerKind

// marshalerKindOf returns how the encoder writes a value of type t.
//
// It decides from the type, so that a value is not boxed into an interface only to be asked which interfaces it
// implements: on a document of strings, that boxing made one allocation in ten. The checks run in the order of the
// type switch they replace, and the first match wins.
func marshalerKindOf(t reflect.Type) marshalerKind {
	if t.NumMethod() == 0 {
		// A type without methods implements none of the interfaces below, and
		// none of the exact types has an empty method set. This skips the map
		// for a float64 or a string, the values a document holds most.
		return encodedByTheEncoder
	}
	if kind, ok := marshalerKinds.Load(t); ok {
		return kind.(marshalerKind) //nolint:forcetypeassert // only marshalerKindOf stores, and it stores a marshalerKind
	}

	kind := encodedByTheEncoder
	switch {
	case t.Implements(reflect.TypeFor[ast.Node]()):
		// encodeNode writes a parsed node as it stands. Through a marshaler,
		// the node went out as its own text and was parsed again. That cost
		// a round trip, and the node's comments had to be read back from the
		// rendered text.
	case t == reflect.TypeFor[MapSlice]() || t == reflect.TypeFor[MapSliceSeq]() ||
		t == reflect.TypeFor[MapItem]() || t == reflect.TypeFor[Base64]():
		// The encoder writes these four itself, ahead of any interface they
		// satisfy. [UseJSONMarshaler] is a last resort, for a type the encoder
		// cannot otherwise write. The encoder writes a MapSlice as a mapping, a
		// MapSliceSeq as an "!!omap", and a Base64 as a "!!binary". When the
		// interface came first, a Base64 went out through FromJSON as a plain
		// string without its tag, whenever the option was set.
	case t.Implements(reflect.TypeFor[Marshaler]()),
		t.Implements(reflect.TypeFor[GoYAMLMarshaler]()),
		t == reflect.TypeFor[time.Time](), t == reflect.TypeFor[*time.Time](),
		t == reflect.TypeFor[time.Duration](),
		t.Implements(reflect.TypeFor[encoding.TextMarshaler]()):
		kind = marshalsItself
	case t.Implements(reflect.TypeFor[jsonMarshaler]()):
		kind = marshalsAsJSON
	}
	marshalerKinds.Store(t, kind)

	return kind
}

// encodeBinary writes a [Base64] as the "!!binary" scalar it came from.
//
// It writes the text in canonical form, without the line breaks permitted by
// RFC 2045. A value carried through Go therefore comes back on one line,
// whatever the layout of the document it was read from.
func (e *Encoder) encodeBinary(b Base64, column int) ast.Node {
	value := e.encodeString(b.Canonical(), column)
	node := ast.Tag(token.New("!!binary", "!!binary", value.GetToken().Position))
	node.Value = value

	return node
}

func (e *Encoder) encodeByMarshaler(v reflect.Value, column int) (ast.Node, error) {
	iface := v.Interface()

	if marshaler, exists := e.marshalerFromCustomMarshalerMap(v.Type()); exists {
		doc, err := marshaler(iface)
		if err != nil {
			return nil, err
		}
		node, err := e.encodeDocument(doc)
		if err != nil {
			return nil, err
		}
		return node, nil
	}

	if marshaler, ok := iface.(Marshaler); ok {
		doc, err := marshaler.MarshalYAML()
		if err != nil {
			return nil, err
		}
		node, err := e.encodeDocument(doc)
		if err != nil {
			return nil, err
		}
		return node, nil
	}

	if marshaler, ok := iface.(GoYAMLMarshaler); ok {
		marshalV, err := marshaler.MarshalYAML()
		if err != nil {
			return nil, err
		}
		return e.encodeValue(reflect.ValueOf(marshalV), column)
	}

	if t, ok := iface.(time.Time); ok {
		return e.encodeTime(t, column), nil
	}
	// Handle *time.Time here: it implements TextMarshaler, and must not be written as plain text.
	if t, ok := iface.(*time.Time); ok && t != nil {
		return e.encodeTime(*t, column), nil
	}

	if t, ok := iface.(time.Duration); ok {
		return e.encodeDuration(t, column), nil
	}

	// A number keeps its type. *big.Int and *big.Float both carry MarshalText,
	// so the branch below wrote them as quoted text, and a round trip turned a
	// number into a string. The decoder builds these for a value too wide for a
	// machine word, and codec.ToJSON writes them as numbers.
	if n, ok := iface.(*big.Int); ok && n != nil {
		value := n.String()

		return ast.Integer(token.New(value, value, e.pos(column))), nil
	}
	if f, ok := iface.(*big.Float); ok && f != nil {
		return e.encodeBigFloat(f, column), nil
	}

	if marshaler, ok := iface.(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return nil, err
		}
		node := e.encodeString(string(text), column)
		return node, nil
	}

	if e.useJSONMarshaler {
		if marshaler, ok := iface.(jsonMarshaler); ok {
			return e.encodeJSON(marshaler)
		}
	}

	return nil, errors.New("does not implemented GoYAMLMarshaler")
}

// encodeJSON converts the JSON written by marshaler to a YAML document, and encodes it.
func (e *Encoder) encodeJSON(marshaler jsonMarshaler) (ast.Node, error) {
	jsonBytes, err := marshaler.MarshalJSON()
	if err != nil {
		return nil, err
	}
	doc, err := fromJSON(jsonBytes, false)
	if err != nil {
		return nil, err
	}

	return e.encodeDocument(doc)
}

func (e *Encoder) encodeValue(v reflect.Value, column int) (ast.Node, error) {
	if e.isInvalidValue(v) {
		return e.encodeNil(), nil
	}
	if v.Kind() == reflect.Interface {
		// Check before the marshalers, which write a time.Time plain.
		if t, held := e.heldTime(v.Elem()); held {
			return e.encodeTaggedTime(t, column), nil
		}
	}
	if node, isNode := e.heldNode(v); isNode {
		return node, nil
	}
	if node, layout := v3NodeValue(v); layout != nil {
		// A go.yaml.in/yaml/v3 Node, written as the YAML it describes.
		return e.encodeV3Node(node, layout, column)
	}
	if e.canEncodeByMarshaler(v) {
		node, err := e.encodeByMarshaler(v, column)
		if err != nil {
			return nil, err
		}
		return node, nil
	}
	// Compare the value's type instead of reading it through
	// reflect.Value.Interface. Interface panics on an unexported field, and the
	// encoder writes a struct holding one like any other.
	if v.Type() == reflect.TypeFor[Base64]() {
		b := Base64(v.String())
		// The tag goes back on, so a document read and written again keeps its
		// "!!binary". A plain string cannot carry the tag, and that is half the
		// reason the decoder builds a [Base64]. JSON has no tags and no binary
		// type, so JSON output holds the base64 string alone, as ToJSON writes
		// for the same document.
		if e.isJSONStyle {
			return e.encodeString(b.Canonical(), column), nil
		}

		return e.encodeBinary(b, column), nil
	}

	return e.encodeKind(v, column)
}

// encodeKind writes v by its kind, once encodeValue has ruled out a null, a tagged time, a node, a go.yaml.in/yaml/v3
// Node, a marshaler and a Base64. A streamed encode calls it for a scalar it has run those checks on.
func (e *Encoder) encodeKind(v reflect.Value, column int) (ast.Node, error) {
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.encodeInt(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.encodeUint(v.Uint()), nil
	case reflect.Float32:
		return e.encodeFloat(v.Float(), 32), nil
	case reflect.Float64:
		return e.encodeFloat(v.Float(), 64), nil
	case reflect.Pointer:
		if value := e.encodePtrAnchor(v, column); value != nil {
			return value, nil
		}
		return e.encodeValue(v.Elem(), column)
	case reflect.Interface:
		return e.encodeValue(v.Elem(), column)
	case reflect.String:
		return e.encodeString(v.String(), column), nil
	case reflect.Bool:
		return e.encodeBool(v.Bool()), nil
	case reflect.Slice:
		if value := e.encodePtrAnchor(v, column); value != nil {
			return value, nil
		}
		return e.encodeSlice(v)
	case reflect.Array:
		return e.encodeArray(v)
	case reflect.Struct:
		if v.CanInterface() {
			if mapSlice, ok := reflect.TypeAssert[MapSlice](v); ok {
				return e.encodeMapSlice(mapSlice, column)
			}
			if seq, ok := reflect.TypeAssert[MapSliceSeq](v); ok {
				return e.encodeOrderedMapSeq(seq, column)
			}
			if mapItem, ok := reflect.TypeAssert[MapItem](v); ok {
				return e.encodeMapItem(mapItem, column)
			}
			if t, ok := reflect.TypeAssert[time.Time](v); ok {
				return e.encodeTime(t, column), nil
			}
		}
		return e.encodeStruct(v, column)
	case reflect.Map:
		if value := e.encodePtrAnchor(v, column); value != nil {
			return value, nil
		}
		return e.encodeMap(v, column)
	default:
		return nil, fmt.Errorf("unknown value type %s", v.Type().String())
	}
}

func (e *Encoder) encodePtrAnchor(v reflect.Value, column int) ast.Node {
	anchorName, exists := e.getAnchor(v.Pointer())
	if !exists {
		return nil
	}
	aliasName := anchorName
	alias := ast.Alias(token.New("*", "*", e.pos(column)))
	alias.Value = ast.String(token.New(aliasName, aliasName, e.pos(column)))
	e.setSmartAlias(aliasName, v.Pointer())

	return alias
}

func (e *Encoder) pos(column int) token.Position {
	return token.At(e.line, column, e.offset, e.indentNum)
}

func (e *Encoder) encodeNil() *ast.NullNode {
	return e.nullNode(e.newToken("null", e.column))
}

func (e *Encoder) encodeInt(v int64) *ast.IntegerNode {
	return e.integerNode(e.newToken(strconv.FormatInt(v, 10), e.column))
}

func (e *Encoder) encodeUint(v uint64) *ast.IntegerNode {
	return e.integerNode(e.newToken(strconv.FormatUint(v, 10), e.column))
}

func (e *Encoder) encodeFloat(v float64, bitSize int) ast.Node {
	switch {
	case v == math.Inf(0):
		return e.infinityNode(e.newToken(".inf", e.column))
	case v == math.Inf(-1):
		return e.infinityNode(e.newToken("-.inf", e.column))
	case math.IsNaN(v):
		return e.nanNode(e.newToken(".nan", e.column))
	default:
		value := strconv.FormatFloat(v, 'g', -1, bitSize)
		if !strings.Contains(value, ".") && !strings.Contains(value, "e") {
			if e.autoInt {
				return e.integerNode(e.newToken(value, e.column))
			}
			// Append a ".0" suffix so that the value reads back as a float.
			value += ".0"
		}

		return e.floatNode(e.newToken(value, e.column))
	}
}

// encodeBigFloat writes a float too wide for a float64, as [Encoder.encodeFloat]
// writes one that fits. A bare number reads back as an integer, so the ".0"
// keeps the float.
func (e *Encoder) encodeBigFloat(v *big.Float, column int) ast.Node {
	if v.IsInf() {
		value := ".inf"
		if v.Signbit() {
			value = "-.inf"
		}

		return ast.Infinity(token.New(value, value, e.pos(column)))
	}

	value := v.Text('g', -1)
	if !strings.Contains(value, ".") && !strings.Contains(value, "e") {
		if e.autoInt {
			return ast.Integer(token.New(value, value, e.pos(column)))
		}
		value += ".0"
	}

	return ast.Float(token.New(value, value, e.pos(column)))
}

func (e *Encoder) isNeedQuoted(v string) bool {
	if e.isJSONStyle {
		return true
	}

	if e.isFlowStyle && strings.ContainsAny(v, "[]{},'\"\n") {
		// A plain scalar inside a flow collection holds no flow indicator: "[" or "{" opens a collection there, as
		// "]", "}" and "," close one or part its entries. A block scalar cannot stand inside a flow collection
		// either, so a line break takes quotes there, whatever UseLiteralStyleIfMultiline asks.
		return true
	}

	if e.useLiteralStyleIfMultiline && strings.Contains(v, "\n") && !token.NeedsQuotedSpelling(v) {
		// A value holding a carriage return or a byte order mark has no block
		// scalar spelling, so it never gets the literal style, whatever the
		// option.
		return false
	}

	if e.isFlowStyle {
		for i := range len(v) {
			if v[i] != ':' {
				continue
			}
			if i+1 < len(v) && v[i+1] == '/' {
				continue
			}
			return true
		}
	}

	if token.IsNeedQuoted(v) {
		return true
	}

	return false
}

func (e *Encoder) encodeString(v string, column int) *ast.StringNode {
	if e.isNeedQuoted(v) {
		// A single-quoted scalar has no escapes. A character that needs an escape takes double quotes even under
		// UseSingleQuote: written single-quoted, "\r" read back as a backslash and an "r".
		if e.singleQuote && !token.NeedsQuotedSpelling(v) {
			v = codecutils.QuoteWith(v, '\'')
		} else {
			v = strconv.Quote(v)
		}
	}
	return e.stringNode(e.newToken(v, column))
}

func (e *Encoder) encodeBool(v bool) *ast.BoolNode {
	return e.boolNode(e.newToken(strconv.FormatBool(v), e.column))
}

func (e *Encoder) encodeSlice(value reflect.Value) (*ast.SequenceNode, error) {
	if e.indentSequence {
		e.column += e.indentNum
		defer func() { e.column -= e.indentNum }()
	}
	column := e.column
	sequence := ast.Sequence(token.New("-", "-", e.pos(column)), e.isFlowStyle)
	for i := range value.Len() {
		node, err := e.encodeValue(value.Index(i), column)
		if err != nil {
			return nil, err
		}
		sequence.Values = append(sequence.Values, node)
	}
	return sequence, nil
}

func (e *Encoder) encodeArray(value reflect.Value) (*ast.SequenceNode, error) {
	if e.indentSequence {
		e.column += e.indentNum
		defer func() { e.column -= e.indentNum }()
	}
	column := e.column
	sequence := ast.Sequence(token.New("-", "-", e.pos(column)), e.isFlowStyle)
	for i := range value.Len() {
		node, err := e.encodeValue(value.Index(i), column)
		if err != nil {
			return nil, err
		}
		sequence.Values = append(sequence.Values, node)
	}
	return sequence, nil
}

// encodeMapItem writes one entry of a [MapSlice].
//
// encodeMapItem encodes the key as a value, as [Encoder.encodeMap] encodes a
// Go map's key. When that gives no node usable as a key, it falls back to the
// key's text. MapItem.Key is an interface{}, and a decode fills it with the
// resolved value of the key: an int for "1:", a float for "1.0:" and nil for
// "null:". Asserting a string here panicked on every one of those, and on a
// MapSlice built by hand with any key but a string.
func (e *Encoder) encodeMapItem(item MapItem, column int) (*ast.MappingValueNode, error) {
	// Both fields are interface slots and reach encodeValue as such, so a
	// time.Time in either takes its tag.
	value, err := e.encodeValue(reflect.ValueOf(&item.Value).Elem(), column)
	if err != nil {
		return nil, err
	}

	encoded, err := e.encodeValue(reflect.ValueOf(&item.Key).Elem(), column)
	key, isKey := encoded.(ast.MapKeyNode)
	if !isKey || err != nil {
		key = e.encodeString(fmt.Sprint(item.Key), column)
	}
	key = e.jsonKeyNode(key, column)

	return ast.MappingValue(
		token.New("", "", e.pos(column)),
		key,
		value,
	), nil
}

// encodeOrderedMapSeq writes a [MapSliceSeq] as an "!!omap": a flow sequence of
// one-entry mappings, "!!omap [{x: 1}, {y: 2}]".
//
// The tag goes back on, so a document read and written again keeps it. A
// MapSlice writes a plain mapping instead. The two types stay separate so that
// the destination declared by a caller decides the spelling.
func (e *Encoder) encodeOrderedMapSeq(value MapSliceSeq, column int) (ast.Node, error) {
	if e.isJSONStyle {
		// JSON has no ordered map and no tags. The encoder writes the entries
		// as an object, in sequence order, as ToJSON does for the same
		// document.
		return e.encodeMapSlice(MapSlice(value), column)
	}

	// The sequence and its mappings are flow collections, so every value inside them is written in flow style
	// too: a block mapping or sequence cannot stand inside a flow collection.
	flow := e
	if !e.isFlowStyle {
		flow = &Encoder{}
		*flow = *e
		flow.isFlowStyle = true
	}
	seq := ast.Sequence(token.New("", "", e.pos(column)), true)
	for _, item := range MapSlice(value).items {
		entry, err := flow.encodeMapItem(item, column)
		if err != nil {
			return nil, err
		}
		mapping := ast.Mapping(token.New("", "", e.pos(column)), true)
		mapping.Values = append(mapping.Values, entry)
		seq.Values = append(seq.Values, mapping)
	}

	node := ast.Tag(token.New("!!omap", "!!omap", e.pos(column)))
	node.Value = seq

	return node, nil
}

func (e *Encoder) encodeMapSlice(value MapSlice, column int) (*ast.MappingNode, error) {
	node := ast.Mapping(token.New("", "", e.pos(column)), e.isFlowStyle)
	for _, item := range value.items {
		encoded, err := e.encodeMapItem(item, column)
		if err != nil {
			return nil, err
		}
		node.Values = append(node.Values, encoded)
	}
	return node, nil
}

// jsonKeyNode returns key as a JSON member name, when the encoder writes JSON.
//
// A JSON member name is a string, and every other spelling is a YAML key, so
// "1: a" writes {"1":"a"} and not {1: "a"}. The encoder used to write the
// second form for every key that is not a string, and that is not JSON.
//
// The text comes from [ast.KeyName], which [ToJSON] also reads through
// astutils.KeyText. The one rule applies to the node encoded from the key, not
// to the Go value, so an integer writes in decimal and a float with a decimal
// point.
func (e *Encoder) jsonKeyNode(key ast.MapKeyNode, column int) ast.MapKeyNode {
	if !e.isJSONStyle {
		return key
	}
	if _, isString := key.(*ast.StringNode); isString {
		return key
	}
	name, kind := ast.KeyName(key)
	if kind == token.KeyOther {
		return key
	}

	return e.encodeString(name, column)
}

func (e *Encoder) encodeMap(value reflect.Value, column int) (ast.Node, error) {
	n := value.Len()
	names, order := make([]string, n), make([]int, n)
	keys, values, order, err := e.mapEntries(value, names, order)
	if err != nil {
		return nil, err
	}
	node := ast.Mapping(token.New("", "", e.pos(column)), e.isFlowStyle)
	node.Values = make([]*ast.MappingValueNode, 0, len(order))
	for _, idx := range order {
		v := values.Index(idx)
		encoded, err := e.encodeValue(v, column)
		if err != nil {
			return nil, err
		}
		keyText := names[idx]
		vRef := e.toPointer(v)

		// The second encoding assigns an anchor to a value used by an alias.
		if aliasName, exists := e.getSmartAlias(vRef); exists {
			anchorName := aliasName
			anchorNode := ast.Anchor(token.New("&", "&", e.pos(column)))
			anchorNode.Name = ast.String(token.New(anchorName, anchorName, e.pos(column)))
			anchorNode.Value = encoded
			encoded = anchorNode
		}

		node.Values = append(node.Values, ast.MappingValue(nil, e.mapKeyNode(keys.Index(idx), keyText, column), encoded))
		e.setSmartAnchor(vRef, keyText)
	}
	return node, nil
}

// mapEntries reads a Go map and sorts its entries by key name. It returns an error for two keys that are one YAML
// key, unless SkipDuplicateMapKey keeps the first.
//
// keys and values are slices of the map's key and value types, filled from one MapRange. names receives each key as
// fmt.Sprint prints it. The returned order lists the indexes to write, sorted by name, without the repeats
// SkipDuplicateMapKey drops.
//
// The caller makes names and order, value.Len() long each, and keeps them in variables of their own: escape analysis
// treats a struct as one value, and a struct holding them beside a reflect.Value moved a small map's slices off the
// caller's stack.
func (e *Encoder) mapEntries(value reflect.Value, names []string, order []int) (keys, values reflect.Value, _ []int, _ error) {
	n := value.Len()

	// MapKeys and MapIndex allocate a copy per key and per value; two slices
	// take one allocation each. Each key keeps its reflect.Value: a nil key in
	// a map[any]any does not come back from reflect.ValueOf(nil), which returns
	// the zero Value.
	keyType := value.Type().Key()
	keys = reflect.MakeSlice(reflect.SliceOf(keyType), n, n)
	values = reflect.MakeSlice(reflect.SliceOf(value.Type().Elem()), n, n)
	iter := value.MapRange()
	for i := 0; iter.Next(); i++ {
		keys.Index(i).SetIterKey(iter)
		values.Index(i).SetIterValue(iter)
	}

	// Each key is printed once: the sort compares the names n log n times,
	// and the entry writes them as the anchor name.
	plain := isPlainString(keyType)
	for i := range names {
		if plain {
			names[i] = keys.Index(i).String()
		} else {
			names[i] = fmt.Sprint(keys.Index(i).Interface())
		}
	}
	for i := range order {
		order[i] = i
	}
	// slices.SortFunc takes order as a slice. sort.Slice took it as an any, which moved it to the heap.
	slices.SortFunc(order, func(i, j int) int {
		if c := strings.Compare(names[i], names[j]); c != 0 {
			return c
		}

		// int(1) and uint64(1) print alike. Ordering them by Go type makes the
		// entry kept by SkipDuplicateMapKey independent of map iteration.
		return strings.Compare(fmt.Sprintf("%T", keys.Index(i).Interface()), fmt.Sprintf("%T", keys.Index(j).Interface()))
	})

	seen := repeatableKeys(keyType)
	if seen == nil {
		return keys, values, order, nil
	}
	kept := order[:0]
	for _, idx := range order {
		key := keys.Index(idx).Interface()
		first, repeated := seen.add(key)
		if repeated && e.skipDuplicateMapKey {
			continue
		}
		if repeated {
			return keys, values, nil, fmt.Errorf("map keys %#v and %#v are one YAML key: %w", first, key, yamlerrors.ErrDuplicateKey)
		}
		kept = append(kept, idx)
	}

	return keys, values, kept, nil
}

// mapKeyNode returns the node of a map key named name.
//
// The key has the map's key type, so a map[any]any key reaches encodeValue in an interface slot: a time.Time there
// takes its tag. A key that gives no node usable as a key is written as its name.
func (e *Encoder) mapKeyNode(key reflect.Value, name string, column int) ast.MapKeyNode {
	kn, err := e.encodeValue(key, column)
	keyNode, ok := kn.(ast.MapKeyNode)
	if !ok || err != nil {
		keyNode = e.encodeString(name, column)
	}

	return e.jsonKeyNode(keyNode, column)
}

// isPlainString reports whether fmt.Sprint prints a value of t as its string
// contents: a string kind with no String, Error or Format method.
func isPlainString(t reflect.Type) bool {
	return t.Kind() == reflect.String &&
		!t.Implements(reflect.TypeFor[fmt.Stringer]()) &&
		!t.Implements(reflect.TypeFor[error]()) &&
		!t.Implements(reflect.TypeFor[fmt.Formatter]())
}

// keySet records the YAML identity of each key written from a Go map, for a
// map where two Go keys can be one YAML key.
type keySet map[keyIdentity]any

// keyIdentity holds the kind and compared name given to a key by codecutils.KeyIDOf.
type keyIdentity struct {
	kind token.KeyKind
	name string
}

// repeatableKeys returns an empty keySet for a map whose key type can hold two
// Go keys that are one YAML key. It returns nil when == already tells the keys
// apart the way YAML does.
//
// An interface holds int(1) beside uint64(1); a float, one NaN beside another;
// a time.Time, one instant in two zones. A string, integer or bool key type
// holds one Go key per YAML key, and costs nothing.
func repeatableKeys(t reflect.Type) keySet {
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	default:
		return keySet{}
	}
}

// add records key, and returns the key already recorded under its identity and
// whether there was one. A key without a kind from codecutils.KeyIDOf is never a repeat.
func (s keySet) add(key any) (any, bool) {
	kind, name, named := codecutils.KeyIDOf(key)
	if !named {
		return nil, false
	}
	id := keyIdentity{kind: kind, name: name}
	if first, repeated := s[id]; repeated {
		return first, true
	}
	s[id] = key

	return nil, false
}

func (e *Encoder) isOmittedByOmitZero(v reflect.Value) bool {
	kind := v.Kind()
	if v.CanInterface() {
		if z, ok := reflect.TypeAssert[IsZeroer](v); ok {
			if (kind == reflect.Pointer || kind == reflect.Interface) && v.IsNil() {
				return true
			}
			return z.IsZero()
		}
	}
	switch kind {
	case reflect.String:
		return len(v.String()) == 0
	case reflect.Interface, reflect.Pointer, reflect.Slice, reflect.Map:
		return v.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Struct:
		vt := v.Type()
		for i := v.NumField() - 1; i >= 0; i-- {
			if vt.Field(i).PkgPath != "" {
				continue // private field
			}
			if !e.isOmittedByOmitZero(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// nothing to do
	}
	return false
}

func (e *Encoder) isOmittedByOmitEmptyOption(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return len(v.String()) == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Bool:
		return !v.Bool()
	default:
		// nothing to do
	}
	return false
}

// isOmittedByOmitEmptyTag reports whether the omitempty flag of a `yaml` tag omits v. The flag of a `json` tag
// follows encoding/json/v2 instead: see encodesEmpty.
//
// The rule is go.yaml.in/yaml/v3's isZero, line for line, and it stays so for compatibility with v3. It differs from
// both versions of encoding/json:
//
//   - Like json v1's omitempty, it omits false and 0, which json/v2's omitempty keeps.
//   - Like omitzero, it calls IsZero when the type implements it.
//   - It omits a struct whose exported fields are all omittable by this rule, whatever their own tags say.
func (e *Encoder) isOmittedByOmitEmptyTag(v reflect.Value) bool {
	kind := v.Kind()
	if z, ok := reflect.TypeAssert[IsZeroer](v); ok {
		if (kind == reflect.Pointer || kind == reflect.Interface) && v.IsNil() {
			return true
		}
		return z.IsZero()
	}
	switch kind {
	case reflect.String:
		return len(v.String()) == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Struct:
		vt := v.Type()
		for i := v.NumField() - 1; i >= 0; i-- {
			if vt.Field(i).PkgPath != "" {
				continue // private field
			}
			if !e.isOmittedByOmitEmptyTag(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// nothing to do
	}
	return false
}

func (e *Encoder) encodeTime(v time.Time, column int) *ast.StringNode {
	value := v.Format(time.RFC3339Nano)
	if e.isJSONStyle {
		value = strconv.Quote(value)
	}
	return ast.String(token.New(value, value, e.pos(column)))
}

// heldTime returns the time.Time held by an interface slot, and reports whether
// the encoder writes it itself. A custom marshaler registered for the type wins.
func (e *Encoder) heldTime(v reflect.Value) (time.Time, bool) {
	if !v.IsValid() || !v.CanInterface() || e.existsTypeInCustomMarshalerMap(v.Type()) {
		return time.Time{}, false
	}
	switch t := v.Interface().(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}

		return *t, true
	default:
		return time.Time{}, false
	}
}

// encodeTaggedTime writes a time.Time held in an interface slot (a value in an
// any, a map[any]any key, a MapSlice entry, the value handed to Marshal) as a
// "!!timestamp".
//
// The core schema resolves no timestamp, so the plain spelling reads back as a
// string wherever the destination is not a time.Time. A round trip through an
// any used to turn the value into text. A field or a map key typed time.Time
// stays plain, because its destination sets the type. JSON has no tags, so
// JSON output holds the RFC 3339 string alone, as ToJSON writes it.
func (e *Encoder) encodeTaggedTime(t time.Time, column int) ast.Node {
	value := e.encodeTime(t, column)
	if e.isJSONStyle {
		return value
	}
	node := ast.Tag(token.New("!!timestamp", "!!timestamp", value.GetToken().Position))
	node.Value = value

	return node
}

func (e *Encoder) encodeDuration(v time.Duration, column int) *ast.StringNode {
	value := v.String()
	if e.isJSONStyle {
		value = strconv.Quote(value)
	}
	return ast.String(token.New(value, value, e.pos(column)))
}

func (e *Encoder) encodeAnchor(anchorName string, value ast.Node, fieldValue reflect.Value, column int) (*ast.AnchorNode, error) {
	anchorNode := ast.Anchor(token.New("&", "&", e.pos(column)))
	anchorNode.Name = ast.String(token.New(anchorName, anchorName, e.pos(column)))
	anchorNode.Value = value
	if e.anchorCallback != nil {
		if err := e.anchorCallback(anchorNode, fieldValue.Interface()); err != nil {
			return nil, err
		}
		if snode, ok := anchorNode.Name.(*ast.StringNode); ok {
			anchorName = snode.Value
		}
	}
	if fieldValue.Kind() == reflect.Pointer {
		e.setAnchor(fieldValue.Pointer(), anchorName)
	}
	return anchorNode, nil
}

// tagMode returns the encoding/json rules that this encoder adds to the `yaml`
// tag. With neither option set, it follows go.yaml.in/yaml/v3 exactly. The pair
// mirrors [UseJSONTags] and [UseInferredNames] on the decoder.
func (e *Encoder) tagMode() structfields.Mode {
	mode := structfields.YAMLTags
	if e.writeJSONTags {
		mode |= structfields.JSONTags
	}
	if e.writeInferredNames {
		mode |= structfields.InferredNames
	}

	return mode
}

// omitsField reports whether the omitzero and omitempty tags, or the OmitZero and OmitEmpty options, leave a field
// out before it is encoded.
//
// A field with a `json` tag follows encoding/json/v2 for omitempty, flag and option alike, and v2 decides from the
// encoded value: encodeFieldValue drops it.
func (e *Encoder) omitsField(sf *structfields.Info, fieldValue reflect.Value) bool {
	switch {
	case (e.omitZero || sf.IsOmitZero) && e.isOmittedByOmitZero(fieldValue):
		return true
	case sf.IsJSONTag:
		return false
	case e.omitEmpty && e.isOmittedByOmitEmptyOption(fieldValue):
		return true
	case sf.IsOmitEmpty && e.isOmittedByOmitEmptyTag(fieldValue):
		return true
	default:
		return false
	}
}

// encodeFieldValue encodes the value of the field sf, through the `json` tag's `,string` option where it is set. It
// returns a nil node for a field with a `json` tag that `,omitempty` or the OmitEmpty option drops.
func (e *Encoder) encodeFieldValue(sf *structfields.Info, fieldValue reflect.Value, column int) (ast.Node, error) {
	var (
		encoded ast.Node
		err     error
	)
	if sf.IsString {
		encoded, err = e.encodeStringNumber(sf, fieldValue, column)
	} else {
		encoded, err = e.encodeValue(fieldValue, column)
	}
	if err != nil {
		return nil, err
	}
	if sf.OmitsEmptyEncoded(e.omitEmpty) && !sf.IsInline && encodesEmpty(encoded) {
		return nil, nil
	}

	return encoded, nil
}

// encodesEmpty reports whether n is null, an empty string, an empty mapping or an empty sequence, through any tag.
// `json:",omitempty"` drops a field whose value encodes as one, as encoding/json/v2 drops a field that encodes as
// null, "", {} or []. A zero number, false, and a type whose IsZero returns true all encode as something else, and
// stay.
func encodesEmpty(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.NullNode:
		return true
	case *ast.StringNode:
		// The encoder writes the quotes into Value, so an empty string is one of these three spellings.
		return n.Value == "" || n.Value == `""` || n.Value == `''`
	case *ast.MappingNode:
		return len(n.Values) == 0
	case *ast.SequenceNode:
		return len(n.Values) == 0
	case *ast.TagNode:
		return encodesEmpty(n.Value)
	default:
		return false
	}
}

// fieldKey returns the key node a field is written under.
func (e *Encoder) fieldKey(fp fieldPlan, column int) ast.MapKeyNode {
	if fp.plainKey && !e.isJSONStyle {
		// isNeedQuoted returns false for this name under every option but JSON.
		return e.stringNode(e.newToken(fp.info.RenderName, column))
	}

	return e.encodeString(fp.info.RenderName, column)
}

// structField encodes a field that is not omitted, and appends the entries it writes to out: one entry, none for an
// inline field that is null, or the entries an inline field brings. inlineAnchor reports an inline field tagged as
// an anchor, which anchors the whole mapping.
func (e *Encoder) structField(
	fields *structfields.Fields, fp fieldPlan, fieldValue reflect.Value, column int, out []*ast.MappingValueNode,
) (_ []*ast.MappingValueNode, inlineAnchor bool, _ error) {
	sf := fp.info
	ve := e
	if !e.isFlowStyle && sf.IsFlow {
		ve = &Encoder{}
		*ve = *e
		ve.isFlowStyle = true
	}
	encoded, err := ve.encodeFieldValue(sf, fieldValue, column)
	if err != nil || encoded == nil {
		return out, false, err
	}
	key := e.fieldKey(fp, column)
	switch {
	case encoded.Type() == ast.AliasType:
		if aliasName := sf.AliasName; aliasName != "" {
			alias, ok := encoded.(*ast.AliasNode)
			if !ok {
				return out, false, yamlerrors.NewUnexpectedNodeType(encoded.Type(), ast.AliasType, encoded.GetToken())
			}
			got := alias.Value.String()
			if aliasName != got {
				return out, false, fmt.Errorf("expected alias name is %q but got %q", aliasName, got)
			}
		}
		if sf.IsInline {
			// With both alias and inline, write `<<: *alias`.
			key = ast.MergeKey(token.New("<<", "<<", e.pos(column)))
		}
	case sf.AnchorName != "":
		anchorNode, err := e.encodeAnchor(sf.AnchorName, encoded, fieldValue, column)
		if err != nil {
			return out, false, err
		}
		encoded = anchorNode
	case sf.IsInline:
		inlineAnchor = sf.IsAutoAnchor
		mapNode, ok := encoded.(ast.MapNode)
		if !ok {
			// Skip an inline field that is null.
			if _, ok := encoded.(*ast.NullNode); ok {
				return out, inlineAnchor, nil
			}
			return out, false, errors.New("inline value is must be map or struct type")
		}
		// A map holds the document's own keys, so the encoder only checks
		// whether a field already writes the name. A struct holds fields,
		// and flat records the field reached by each name.
		fromMap := structfields.InlineTakesUnclaimedEntries(fieldValue.Type())
		for _, entry := range astutils.EntriesOf(mapNode) {
			mapKey, mapValue := entry.Key, entry.Value
			keyName := mapKey.GetToken().Value
			if fromMap {
				if fields.Claims(keyName) {
					continue
				}
			} else if !fields.WritesPromoted(keyName, sf.Index) {
				continue
			}
			out = append(out, ast.MappingValue(nil, mapKey, mapValue))
		}

		return out, inlineAnchor, nil
	case sf.IsAutoAnchor:
		anchorNode, err := e.encodeAnchor(sf.RenderName, encoded, fieldValue, column)
		if err != nil {
			return out, false, err
		}
		encoded = anchorNode
	}

	return append(out, ast.MappingValue(nil, key, encoded)), false, nil
}

func (e *Encoder) encodeStruct(value reflect.Value, column int) (ast.Node, error) {
	node := ast.Mapping(token.New("", "", e.pos(column)), e.isFlowStyle)
	plan, err := planOf(value.Type(), e.tagMode())
	if err != nil {
		return nil, err
	}
	hasInlineAnchorField := false
	var inlineAnchorValue reflect.Value
	for _, fp := range plan.write {
		fieldValue := value.Field(fp.index)
		if e.omitsField(fp.info, fieldValue) {
			continue
		}
		var inlineAnchor bool
		node.Values, inlineAnchor, err = e.structField(plan.fields, fp, fieldValue, column, node.Values)
		if err != nil {
			return nil, err
		}
		if inlineAnchor {
			hasInlineAnchorField = true
			inlineAnchorValue = fieldValue
		}
	}
	if !hasInlineAnchorField {
		return node, nil
	}
	anchorName := "anchor"
	anchorNode := ast.Anchor(token.New("&", "&", e.pos(column)))
	anchorNode.Name = ast.String(token.New(anchorName, anchorName, e.pos(column)))
	anchorNode.Value = node
	if e.anchorCallback != nil {
		if err := e.anchorCallback(anchorNode, value.Addr().Interface()); err != nil {
			return nil, err
		}
		if snode, ok := anchorNode.Name.(*ast.StringNode); ok {
			anchorName = snode.Value
		}
	}
	if inlineAnchorValue.Kind() == reflect.Pointer {
		e.setAnchor(inlineAnchorValue.Pointer(), anchorName)
	}

	return anchorNode, nil
}

func (e *Encoder) toPointer(v reflect.Value) uintptr {
	if e.isInvalidValue(v) {
		return 0
	}

	switch v.Type().Kind() {
	case reflect.Pointer:
		return v.Pointer()
	case reflect.Interface:
		return e.toPointer(v.Elem())
	case reflect.Slice:
		return v.Pointer()
	case reflect.Map:
		return v.Pointer()
	default:
		// nothing to do
	}
	return 0
}

func (e *Encoder) clearSmartAnchorRef() {
	if !e.enableSmartAnchor {
		return
	}
	e.anchorRefToName = make(map[uintptr]string)
	e.anchorNameMap = make(map[string]struct{})
}

func (e *Encoder) setSmartAnchor(ptr uintptr, name string) {
	if !e.enableSmartAnchor {
		return
	}
	e.setAnchor(ptr, e.generateAnchorName(name))
}

func (e *Encoder) setAnchor(ptr uintptr, name string) {
	if ptr == 0 {
		return
	}
	if name == "" {
		return
	}
	e.anchorRefToName[ptr] = name
	e.anchorNameMap[name] = struct{}{}
}

func (e *Encoder) generateAnchorName(base string) string {
	if _, exists := e.anchorNameMap[base]; !exists {
		return base
	}
	for i := 1; i < 100; i++ {
		name := base + strconv.Itoa(i)
		if _, exists := e.anchorNameMap[name]; exists {
			continue
		}
		return name
	}
	return ""
}

func (e *Encoder) getAnchor(ref uintptr) (string, bool) {
	anchorName, exists := e.anchorRefToName[ref]
	return anchorName, exists
}

func (e *Encoder) setSmartAlias(name string, ref uintptr) {
	if !e.enableSmartAnchor {
		return
	}
	e.aliasRefToName[ref] = name
}

func (e *Encoder) getSmartAlias(ref uintptr) (string, bool) {
	if !e.enableSmartAnchor {
		return "", false
	}
	aliasName, exists := e.aliasRefToName[ref]
	return aliasName, exists
}
