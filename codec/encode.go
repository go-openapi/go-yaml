// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
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
	writer                     io.Writer
	opts                       []EncodeOption
	singleQuote                bool
	isFlowStyle                bool
	isJSONStyle                bool
	useJSONMarshaler           bool
	enableSmartAnchor          bool
	aliasRefToName             map[uintptr]string
	anchorRefToName            map[uintptr]string
	anchorNameMap              map[string]struct{}
	anchorCallback             func(*ast.AnchorNode, any) error
	customMarshalerMap         map[reflect.Type]func(any) ([]byte, error)
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
func (e *Encoder) Encode(v any) error {
	node, err := e.EncodeToNode(v)
	if err != nil {
		return err
	}
	if err := e.setCommentByCommentMap(node); err != nil {
		return err
	}
	if e.written {
		// Every document after the first opens with a separator.
		if _, err := io.WriteString(e.writer, "---\n"); err != nil {
			return err
		}
	}
	e.written = true

	// Lay the document out from the tree, not from the positions held by the
	// nodes. The encoder's indentation options take effect here.
	renderer := ast.NewRenderer(
		ast.WithIndent(e.indentNum),
		ast.WithIndentSequence(e.indentSequence),
	)
	if err := renderer.Render(e.writer, node); err != nil {
		return err
	}
	_, err = io.WriteString(e.writer, "\n")

	return err
}

// EncodeToNode returns the AST node that [Encoder.Encode] would write for v, without writing it.
func (e *Encoder) EncodeToNode(v any) (ast.Node, error) {
	for _, opt := range e.opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	if e.enableSmartAnchor {
		// The first encoding stores every mapping from an alias address to its name.
		if _, err := e.encodeValue(reflect.ValueOf(v), 1); err != nil {
			return nil, err
		}
		e.clearSmartAnchorRef()
	}
	node, err := e.encodeValue(reflect.ValueOf(v), 1)
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
	iface := v.Interface()
	switch iface.(type) {
	case ast.Node:
		// encodeNode writes a parsed node as it stands. Through a marshaler,
		// the node went out as its own text and was parsed again. That cost
		// a round trip, and the node's comments had to be read back from the
		// rendered text.
		return false
	case MapSlice, MapSliceSeq, MapItem, Base64:
		// The encoder writes these four itself, ahead of any interface they
		// satisfy. [UseJSONMarshaler] is a last resort, for a type the encoder
		// cannot otherwise write. The encoder writes a MapSlice as a mapping, a
		// MapSliceSeq as an "!!omap", and a Base64 as a "!!binary". When the
		// interface came first, a Base64 went out through FromJSON as a plain
		// string without its tag, whenever the option was set.
		return false
	case Marshaler:
		return true
	case GoYAMLMarshaler:
		return true
	case time.Time, *time.Time:
		return true
	case time.Duration:
		return true
	case encoding.TextMarshaler:
		return true
	case jsonMarshaler:
		return e.useJSONMarshaler
	}
	return false
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
	return token.At(int32(e.line), int32(column), int32(e.offset), int32(e.indentNum))
}

func (e *Encoder) encodeNil() *ast.NullNode {
	value := "null"
	return ast.Null(token.New(value, value, e.pos(e.column)))
}

func (e *Encoder) encodeInt(v int64) *ast.IntegerNode {
	value := strconv.FormatInt(v, 10)
	return ast.Integer(token.New(value, value, e.pos(e.column)))
}

func (e *Encoder) encodeUint(v uint64) *ast.IntegerNode {
	value := strconv.FormatUint(v, 10)
	return ast.Integer(token.New(value, value, e.pos(e.column)))
}

func (e *Encoder) encodeFloat(v float64, bitSize int) ast.Node {
	switch {
	case v == math.Inf(0):
		value := ".inf"
		return ast.Infinity(token.New(value, value, e.pos(e.column)))
	case v == math.Inf(-1):
		value := "-.inf"
		return ast.Infinity(token.New(value, value, e.pos(e.column)))
	case math.IsNaN(v):
		value := ".nan"
		return ast.Nan(token.New(value, value, e.pos(e.column)))
	default:
		value := strconv.FormatFloat(v, 'g', -1, bitSize)
		if !strings.Contains(value, ".") && !strings.Contains(value, "e") {
			if e.autoInt {
				return ast.Integer(token.New(value, value, e.pos(e.column)))
			}
			// Append a ".0" suffix so that the value reads back as a float.
			value += ".0"
		}

		return ast.Float(token.New(value, value, e.pos(e.column)))
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

	if e.useLiteralStyleIfMultiline && strings.Contains(v, "\n") && !token.NeedsQuotedSpelling(v) {
		// A value holding a carriage return or a byte order mark has no block
		// scalar spelling, so it never gets the literal style, whatever the
		// option.
		return false
	}

	if e.isFlowStyle && strings.ContainsAny(v, `]},'"`) {
		return true
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
	return ast.String(token.New(v, v, e.pos(column)))
}

func (e *Encoder) encodeBool(v bool) *ast.BoolNode {
	value := strconv.FormatBool(v)
	return ast.Bool(token.New(value, value, e.pos(e.column)))
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

	seq := ast.Sequence(token.New("", "", e.pos(column)), true)
	for _, item := range MapSlice(value).items {
		entry, err := e.encodeMapItem(item, column)
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
	node := ast.Mapping(token.New("", "", e.pos(column)), e.isFlowStyle)
	keys := make([]any, len(value.MapKeys()))
	for i, k := range value.MapKeys() {
		keys[i] = k.Interface()
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := fmt.Sprint(keys[i]), fmt.Sprint(keys[j])
		if a != b {
			return a < b
		}

		// int(1) and uint64(1) print alike. Ordering them by Go type makes the
		// entry kept by SkipDuplicateMapKey independent of map iteration.
		return fmt.Sprintf("%T", keys[i]) < fmt.Sprintf("%T", keys[j])
	})
	seen := repeatableKeys(value.Type().Key())
	for _, key := range keys {
		if seen != nil {
			first, repeated := seen.add(key)
			if repeated && e.skipDuplicateMapKey {
				continue
			}
			if repeated {
				return nil, fmt.Errorf("map keys %#v and %#v are one YAML key: %w", first, key, yamlerrors.ErrDuplicateKey)
			}
		}

		k := reflect.ValueOf(key)
		v := value.MapIndex(k)
		encoded, err := e.encodeValue(v, column)
		if err != nil {
			return nil, err
		}
		keyText := fmt.Sprint(key)
		vRef := e.toPointer(v)

		// The second encoding assigns an anchor to a value used by an alias.
		if aliasName, exists := e.getSmartAlias(vRef); exists {
			anchorName := aliasName
			anchorNode := ast.Anchor(token.New("&", "&", e.pos(column)))
			anchorNode.Name = ast.String(token.New(anchorName, anchorName, e.pos(column)))
			anchorNode.Value = encoded
			encoded = anchorNode
		}

		keyValue := reflect.ValueOf(key)
		if value.Type().Key().Kind() == reflect.Interface {
			// A map[any]any key stands in an interface slot, and reaches
			// encodeValue as one: a time.Time there takes its tag.
			keyValue = reflect.ValueOf(&key).Elem()
		}
		kn, err := e.encodeValue(keyValue, column)
		keyNode, ok := kn.(ast.MapKeyNode)
		if !ok || err != nil {
			keyNode = e.encodeString(fmt.Sprint(key), column)
		}
		keyNode = e.jsonKeyNode(keyNode, column)
		node.Values = append(node.Values, ast.MappingValue(
			nil,
			keyNode,
			encoded,
		))
		e.setSmartAnchor(vRef, keyText)
	}
	return node, nil
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

// isOmittedByOmitEmptyTag reports whether the omitempty tag omits v.
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
// any, a map[any]any key, a MapSlice entry) as a "!!timestamp".
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

//nolint:gocyclo,cyclop,gocognit // one branch per tag flag (omitempty, omitzero, flow, anchor, alias, inline)
func (e *Encoder) encodeStruct(value reflect.Value, column int) (ast.Node, error) {
	node := ast.Mapping(token.New("", "", e.pos(column)), e.isFlowStyle)
	structType := value.Type()
	mode := e.tagMode()
	fields, err := structfields.Of(structType, mode)
	if err != nil {
		return nil, err
	}
	fieldMap := fields.ByGoName
	hasInlineAnchorField := false
	var inlineAnchorValue reflect.Value
	for i := range value.NumField() {
		field := structType.Field(i)
		if structfields.Ignored(field, mode) {
			continue
		}
		fieldValue := value.FieldByName(field.Name)
		sf := fieldMap[field.Name]
		if (e.omitZero || sf.IsOmitZero) && e.isOmittedByOmitZero(fieldValue) {
			// omit encoding by omitzero tag or OmitZero option.
			continue
		}
		if e.omitEmpty && e.isOmittedByOmitEmptyOption(fieldValue) {
			// omit encoding by OmitEmpty option.
			continue
		}
		if sf.IsOmitEmpty && e.isOmittedByOmitEmptyTag(fieldValue) {
			// omit encoding by omitempty tag.
			continue
		}
		ve := e
		if !e.isFlowStyle && sf.IsFlow {
			ve = &Encoder{}
			*ve = *e
			ve.isFlowStyle = true
		}
		encoded, err := ve.encodeValue(fieldValue, column)
		if err != nil {
			return nil, err
		}
		var key ast.MapKeyNode = e.encodeString(sf.RenderName, column)
		switch {
		case encoded.Type() == ast.AliasType:
			if aliasName := sf.AliasName; aliasName != "" {
				alias, ok := encoded.(*ast.AliasNode)
				if !ok {
					return nil, yamlerrors.NewUnexpectedNodeType(encoded.Type(), ast.AliasType, encoded.GetToken())
				}
				got := alias.Value.String()
				if aliasName != got {
					return nil, fmt.Errorf("expected alias name is %q but got %q", aliasName, got)
				}
			}
			if sf.IsInline {
				// With both alias and inline, write `<<: *alias`.
				key = ast.MergeKey(token.New("<<", "<<", e.pos(column)))
			}
		case sf.AnchorName != "":
			anchorNode, err := e.encodeAnchor(sf.AnchorName, encoded, fieldValue, column)
			if err != nil {
				return nil, err
			}
			encoded = anchorNode
		case sf.IsInline:
			isAutoAnchor := sf.IsAutoAnchor
			if !hasInlineAnchorField {
				hasInlineAnchorField = isAutoAnchor
			}
			if isAutoAnchor {
				inlineAnchorValue = fieldValue
			}
			mapNode, ok := encoded.(ast.MapNode)
			if !ok {
				// Skip an inline field that is null.
				if _, ok := encoded.(*ast.NullNode); ok {
					continue
				}
				return nil, errors.New("inline value is must be map or struct type")
			}
			// A map holds the document's own keys, so the encoder only checks
			// whether a field already writes the name. A struct holds fields,
			// and flat records the field reached by each name.
			fromMap := structfields.InlineTakesUnclaimedEntries(fieldValue.Type())
			mapIter := mapNode.MapRange()
			for mapIter.Next() {
				mapKey := mapIter.Key()
				mapValue := mapIter.Value()
				keyName := mapKey.GetToken().Value
				if fromMap {
					if fields.Claims(keyName) {
						continue
					}
				} else if !fields.WritesPromoted(keyName, sf.Index) {
					continue
				}
				node.Values = append(node.Values, ast.MappingValue(nil, mapKey, mapValue))
			}
			continue
		case sf.IsAutoAnchor:
			anchorNode, err := e.encodeAnchor(sf.RenderName, encoded, fieldValue, column)
			if err != nil {
				return nil, err
			}
			encoded = anchorNode
		}
		node.Values = append(node.Values, ast.MappingValue(nil, key, encoded))
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
