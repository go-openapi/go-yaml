// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
	"github.com/go-openapi/go-yaml/token"
)

// v3Packages are the import paths go.yaml.in/yaml/v3 is published under.
//
// The decoder recognises that library's Node type by its package path and name, and builds one through reflection.
// So a type written for v3 keeps working, and this module does not depend on v3: a program holding such a type
// already links v3 itself.
var v3Packages = []string{"go.yaml.in/yaml/v3", "gopkg.in/yaml.v3"}

// The values of v3's Kind and Style, both declared there as 1 << iota.
const (
	v3DocumentNode uint64 = 1 << iota
	v3SequenceNode
	v3MappingNode
	v3ScalarNode
	v3AliasNode
)

const (
	v3TaggedStyle uint64 = 1 << iota
	v3DoubleQuotedStyle
	v3SingleQuotedStyle
	v3LiteralStyle
	v3FoldedStyle
	v3FlowStyle
)

// v3NodeLayout holds the indexes of the yaml.Node fields the decoder fills.
type v3NodeLayout struct {
	typ                                             reflect.Type
	kind, style, tag, value, anchor, alias, content int
	line, column                                    int
}

var (
	v3Layouts sync.Map // reflect.Type -> *v3NodeLayout, or nil for a type that is not v3's Node
	v3Hooks   sync.Map // reflect.Type of a pointer -> *v3NodeLayout of its hook's parameter, or nil
)

// v3NodeLayoutOf returns the layout of t when t is go.yaml.in/yaml/v3's Node, and nil otherwise.
//
// A struct named Node in one of [v3Packages] whose fields do not have the expected kinds is not taken for v3's.
func v3NodeLayoutOf(t reflect.Type) *v3NodeLayout {
	if t.Kind() != reflect.Struct || t.Name() != "Node" || !slices.Contains(v3Packages, t.PkgPath()) {
		return nil
	}
	if cached, ok := v3Layouts.Load(t); ok {
		layout, _ := cached.(*v3NodeLayout)

		return layout
	}
	layout := readV3NodeLayout(t)
	v3Layouts.Store(t, layout)

	return layout
}

func readV3NodeLayout(t reflect.Type) *v3NodeLayout {
	layout := &v3NodeLayout{typ: t}
	for _, f := range []struct {
		name  string
		kind  reflect.Kind
		index *int
	}{
		{"Kind", reflect.Uint32, &layout.kind},
		{"Style", reflect.Uint32, &layout.style},
		{"Tag", reflect.String, &layout.tag},
		{"Value", reflect.String, &layout.value},
		{"Anchor", reflect.String, &layout.anchor},
		{"Alias", reflect.Pointer, &layout.alias},
		{"Content", reflect.Slice, &layout.content},
		{"Line", reflect.Int, &layout.line},
		{"Column", reflect.Int, &layout.column},
	} {
		field, ok := t.FieldByName(f.name)
		if !ok || field.Type.Kind() != f.kind || len(field.Index) != 1 {
			return nil
		}
		*f.index = field.Index[0]
	}

	return layout
}

// v3NodeHook returns the layout of the yaml.Node that ptr's UnmarshalYAML method takes, when ptr has v3's
// UnmarshalYAML(*yaml.Node) error, and nil otherwise.
func v3NodeHook(ptr reflect.Type) *v3NodeLayout {
	if cached, ok := v3Hooks.Load(ptr); ok {
		layout, _ := cached.(*v3NodeLayout)

		return layout
	}
	layout := readV3NodeHook(ptr)
	v3Hooks.Store(ptr, layout)

	return layout
}

func readV3NodeHook(ptr reflect.Type) *v3NodeLayout {
	method, ok := ptr.MethodByName("UnmarshalYAML")
	if !ok {
		return nil
	}
	// The method's type holds the receiver first.
	fn := method.Type
	if fn.NumIn() != 2 || fn.NumOut() != 1 || fn.Out(0) != reflect.TypeFor[error]() {
		return nil
	}
	param := fn.In(1)
	if param.Kind() != reflect.Pointer {
		return nil
	}

	return v3NodeLayoutOf(param.Elem())
}

// callV3NodeHook builds the yaml.Node of src and passes it to the UnmarshalYAML method of the value ptr points to.
//
// v3 does not call the method for a null, and leaves the value as it was. So neither does this.
func (d *Decoder) callV3NodeHook(ptr reflect.Value, layout *v3NodeLayout, src ast.Node) error {
	if ast.IsNull(src) {
		return nil
	}
	node := d.newV3NodeBuilder(layout).build(src)
	out := ptr.MethodByName("UnmarshalYAML").Call([]reflect.Value{node})
	if err, ok := reflect.TypeAssert[error](out[0]); ok && err != nil {
		return err
	}

	return nil
}

// v3NodeBuilder builds a tree of go.yaml.in/yaml/v3 Node values from an AST.
type v3NodeBuilder struct {
	layout *v3NodeLayout
	// built holds the *Node built from each AST node, so an alias points at the Node of its anchor, as in v3.
	built       map[ast.Node]reflect.Value
	aliasTarget func(*ast.AliasNode) (ast.Node, string)
}

func (d *Decoder) newV3NodeBuilder(layout *v3NodeLayout) *v3NodeBuilder {
	return &v3NodeBuilder{layout: layout, built: map[ast.Node]reflect.Value{}, aliasTarget: d.aliasTarget}
}

// document returns a *Node of kind DocumentNode holding body, as v3 gives for a yaml.Node at the root.
func (b *v3NodeBuilder) document(doc *ast.DocumentNode, body ast.Node) reflect.Value {
	ptr := b.node(v3DocumentNode, "", "")
	root := b.build(body)
	if doc != nil && doc.Start != nil {
		b.place(ptr, doc.Start)
	} else {
		// A document with no "---" starts where its first node does.
		b.copyPlace(ptr, root)
	}
	b.append(ptr, root)

	return ptr
}

// build returns the *Node v3 builds for n.
func (b *v3NodeBuilder) build(n ast.Node) reflect.Value {
	if n == nil {
		return b.node(v3ScalarNode, "!!null", "")
	}
	if ptr, ok := b.built[n]; ok {
		return ptr
	}

	switch n := n.(type) {
	case *ast.AnchorNode:
		ptr := b.build(n.Value)
		b.set(ptr, b.layout.anchor, n.Name.GetToken().Value)
		b.placeProperty(ptr, n.Start)

		return ptr
	case *ast.TagNode:
		return b.tagged(n)
	case *ast.AliasNode:
		target, name := b.aliasTarget(n)
		ptr := b.node(v3AliasNode, "", name)
		b.place(ptr, n.Start)
		b.built[n] = ptr
		if target != nil {
			to := b.build(target)
			if to.Elem().Field(b.layout.anchor).String() == "" {
				// The anchor stands outside the node handed over, and the build reached its value directly.
				b.set(to, b.layout.anchor, name)
			}
			ptr.Elem().Field(b.layout.alias).Set(to)
		}

		return ptr
	case *ast.MappingKeyNode:
		return b.build(n.Value)
	case *ast.MappingNode:
		ptr := b.collection(n, v3MappingNode, "!!map", n.IsFlowStyle, n.Start)
		for _, entry := range n.Values {
			b.entry(ptr, entry)
		}
		if !n.IsFlowStyle && len(n.Values) > 0 {
			// v3 places a block mapping at its first key. The parser's Start is the first ":".
			b.copyPlace(ptr, ptr.Elem().Field(b.layout.content).Index(0))
		}

		return ptr
	case *ast.MappingValueNode:
		// A block mapping of one entry, which the parser does not wrap in a MappingNode.
		ptr := b.collection(n, v3MappingNode, "!!map", n.IsFlowStyle, n.Key.GetToken())
		b.entry(ptr, n)

		return ptr
	case *ast.SequenceNode:
		ptr := b.collection(n, v3SequenceNode, "!!seq", n.IsFlowStyle, n.Start)
		for _, value := range n.Values {
			b.append(ptr, b.build(value))
		}

		return ptr
	}

	ptr := b.scalar(n)
	b.built[n] = ptr

	return ptr
}

// tagged builds the node a tag stands on, with the tag v3 gives it.
//
// v3 writes a tag in its short form, "!!int" for tag:yaml.org,2002:int, and marks the node TaggedStyle. A tag the
// parser added to a plain timestamp under YAML 1.1 is implicit: v3 resolves that tag and does not mark the style.
func (b *v3NodeBuilder) tagged(n *ast.TagNode) reflect.Value {
	ptr := b.build(n.Value)
	if n.URI == "" || n.URI == "!" {
		return ptr
	}
	b.set(ptr, b.layout.tag, v3ShortTag(n.URI))
	if !n.Implicit {
		style := ptr.Elem().Field(b.layout.style)
		style.SetUint(style.Uint() | v3TaggedStyle)
		b.placeProperty(ptr, n.Start)
	}

	return ptr
}

// v3ShortTag returns tag as v3's shortTag writes it.
func v3ShortTag(tag string) string {
	if rest, ok := strings.CutPrefix(tag, token.YAMLTagPrefix); ok {
		return "!!" + rest
	}

	return tag
}

func (b *v3NodeBuilder) collection(n ast.Node, kind uint64, tag string, flow bool, at *token.Token) reflect.Value {
	ptr := b.node(kind, tag, "")
	if flow {
		b.layoutStyle(ptr, v3FlowStyle)
	}
	b.place(ptr, at)
	// Registered before the children, so an alias inside the collection to its own anchor finds it.
	b.built[n] = ptr

	return ptr
}

// entry appends a mapping entry's key and value to the Node of its mapping.
//
// An implicit null, the value of "a:" with nothing after it, stands just after the ":" in v3.
func (b *v3NodeBuilder) entry(mapping reflect.Value, entry *ast.MappingValueNode) {
	b.append(mapping, b.build(entry.Key))
	value := b.build(entry.Value)
	if null, ok := entry.Value.(*ast.NullNode); ok && null.String() == "" && entry.Start != nil {
		elem := value.Elem()
		elem.Field(b.layout.line).SetInt(int64(entry.Start.Position.Line))
		elem.Field(b.layout.column).SetInt(int64(entry.Start.Position.Column) + 1)
	}
	b.append(mapping, value)
}

// scalar builds the Node of a scalar: its tag is the one this library resolves the scalar to.
func (b *v3NodeBuilder) scalar(n ast.Node) reflect.Value {
	switch n := n.(type) {
	case *ast.StringNode:
		ptr := b.node(v3ScalarNode, "!!str", n.Value)
		if n.Token != nil {
			switch n.Token.Type {
			case token.DoubleQuoteType:
				b.layoutStyle(ptr, v3DoubleQuotedStyle)
			case token.SingleQuoteType:
				b.layoutStyle(ptr, v3SingleQuotedStyle)
			default:
				// A plain string carries no style.
			}
		}
		b.place(ptr, n.Token)

		return ptr
	case *ast.LiteralNode:
		value := ""
		if n.Value != nil {
			value = n.Value.Value
		}
		ptr := b.node(v3ScalarNode, "!!str", value)
		if n.Start != nil && strings.HasPrefix(n.Start.Value, ">") {
			b.layoutStyle(ptr, v3FoldedStyle)
		} else {
			b.layoutStyle(ptr, v3LiteralStyle)
		}
		b.place(ptr, n.Start)

		return ptr
	case *ast.MergeKeyNode:
		ptr := b.node(v3ScalarNode, "!!merge", "<<")
		b.place(ptr, n.Token)

		return ptr
	}

	tk := n.GetToken()
	text := ""
	if tk != nil {
		text = tk.Value
	}
	tag := "!!str"
	switch n := n.(type) {
	case *ast.NullNode:
		tag = "!!null"
		// "null", "~", or nothing where the document left the value out.
		text = n.String()
	case *ast.BoolNode:
		tag = "!!bool"
	case *ast.IntegerNode:
		tag = "!!int"
	case *ast.FloatNode, *ast.InfinityNode, *ast.NanNode:
		tag = "!!float"
	}
	ptr := b.node(v3ScalarNode, tag, text)
	b.place(ptr, tk)

	return ptr
}

// node returns a new *Node of the given kind, tag and value.
func (b *v3NodeBuilder) node(kind uint64, tag, value string) reflect.Value {
	ptr := reflect.New(b.layout.typ)
	elem := ptr.Elem()
	elem.Field(b.layout.kind).SetUint(kind)
	elem.Field(b.layout.tag).SetString(tag)
	elem.Field(b.layout.value).SetString(value)

	return ptr
}

func (b *v3NodeBuilder) set(ptr reflect.Value, field int, value string) {
	ptr.Elem().Field(field).SetString(value)
}

func (b *v3NodeBuilder) layoutStyle(ptr reflect.Value, style uint64) {
	field := ptr.Elem().Field(b.layout.style)
	field.SetUint(field.Uint() | style)
}

// place sets the Node's Line and Column from tk, both counted from 1 as in v3.
func (b *v3NodeBuilder) place(ptr reflect.Value, tk *token.Token) {
	if tk == nil {
		return
	}
	elem := ptr.Elem()
	elem.Field(b.layout.line).SetInt(int64(tk.Position.Line))
	elem.Field(b.layout.column).SetInt(int64(tk.Position.Column))
}

// placeProperty places the Node at tk, an anchor or a tag written before it, unless another property already
// placed it earlier. v3 places a node at its first property, and the AST nests a tag and an anchor in one order
// whatever order the document wrote them in.
func (b *v3NodeBuilder) placeProperty(ptr reflect.Value, tk *token.Token) {
	if tk == nil {
		return
	}
	elem := ptr.Elem()
	line, column := int(elem.Field(b.layout.line).Int()), int(elem.Field(b.layout.column).Int())
	at, col := int(tk.Position.Line), int(tk.Position.Column)
	if line == 0 || at < line || (at == line && col < column) {
		b.place(ptr, tk)
	}
}

// copyPlace sets the Line and Column of to from those of from.
func (b *v3NodeBuilder) copyPlace(to, from reflect.Value) {
	to.Elem().Field(b.layout.line).Set(from.Elem().Field(b.layout.line))
	to.Elem().Field(b.layout.column).Set(from.Elem().Field(b.layout.column))
}

func (b *v3NodeBuilder) append(parent, child reflect.Value) {
	content := parent.Elem().Field(b.layout.content)
	content.Set(reflect.Append(content, child))
}

// v3NodeValue returns the yaml.Node v holds, directly or behind one pointer, with its layout. A nil *yaml.Node holds
// none, and encodes as a nil pointer does.
func v3NodeValue(v reflect.Value) (reflect.Value, *v3NodeLayout) {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem()
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, nil
		}
		v = v.Elem()
	}
	layout := v3NodeLayoutOf(v.Type())
	if layout == nil {
		return reflect.Value{}, nil
	}

	return v, layout
}

// encodeV3Node writes a go.yaml.in/yaml/v3 Node as the YAML it describes, as v3 does.
//
// A Node reaches the encoder as a field of type yaml.Node or *yaml.Node, or as the value MarshalYAML returns.
// The encoder writes its kind, style, tag, anchor and alias. It does not write its comments.
func (e *Encoder) encodeV3Node(node reflect.Value, layout *v3NodeLayout, column int) (ast.Node, error) {
	if e.isJSONStyle {
		return e.encodeV3NodeAsJSON(node, layout, column)
	}
	kind := node.Field(layout.kind).Uint()
	switch kind {
	case 0:
		// A zero Node encodes as a null in v3.
		return e.encodeNil(), nil
	case v3DocumentNode:
		content := node.Field(layout.content)
		if content.Len() == 0 {
			return e.encodeNil(), nil
		}

		return e.encodeV3Child(content.Index(0), layout, column)
	case v3AliasNode:
		name := node.Field(layout.value).String()
		if name == "" && !node.Field(layout.alias).IsNil() {
			name = node.Field(layout.alias).Elem().Field(layout.anchor).String()
		}
		alias := ast.Alias(token.New("*", "*", e.pos(column)))
		alias.Value = ast.String(token.New(name, name, e.pos(column)))

		return alias, nil
	}

	var (
		value ast.Node
		err   error
	)
	// A Node built by hand may hold the long form of its tag.
	tag := v3ShortTag(node.Field(layout.tag).String())
	style := node.Field(layout.style).Uint()
	explicit := style&v3TaggedStyle != 0
	switch kind {
	case v3ScalarNode:
		value, tag = e.encodeV3Scalar(node.Field(layout.value).String(), tag, style, explicit, column)
	case v3MappingNode:
		value, err = e.encodeV3Mapping(node, layout, style&v3FlowStyle != 0, column)
		if !explicit && tag == "!!map" {
			tag = ""
		}
	case v3SequenceNode:
		value, err = e.encodeV3Sequence(node, layout, style&v3FlowStyle != 0, column)
		if !explicit && tag == "!!seq" {
			tag = ""
		}
	default:
		return nil, fmt.Errorf("cannot encode a yaml.Node of kind %d", kind)
	}
	if err != nil {
		return nil, err
	}

	return e.v3Properties(value, tag, node.Field(layout.anchor).String(), column), nil
}

// encodeV3NodeAsJSON writes a Node under [JSON]. JSON has no tags, anchors or aliases, and a scalar's JSON type
// follows from its YAML one, so the encoder writes the Node as YAML, decodes that into an any, and encodes the value.
func (e *Encoder) encodeV3NodeAsJSON(node reflect.Value, layout *v3NodeLayout, column int) (ast.Node, error) {
	e.isJSONStyle = false
	written, err := e.encodeV3Node(node, layout, column)
	e.isJSONStyle = true
	if err != nil {
		return nil, err
	}
	var value any
	if err := Unmarshal([]byte(written.String()), &value); err != nil {
		return nil, err
	}

	return e.encodeValue(reflect.ValueOf(&value).Elem(), column)
}

func (e *Encoder) encodeV3Child(ptr reflect.Value, layout *v3NodeLayout, column int) (ast.Node, error) {
	if ptr.IsNil() {
		return e.encodeNil(), nil
	}

	return e.encodeV3Node(ptr.Elem(), layout, column)
}

// encodeV3Scalar writes a scalar Node, and returns the tag left to write in front of it.
//
// It follows v3's encoder. A tag the Node did not mark TaggedStyle is dropped where the scalar reads back with that
// tag anyway: a quoted or block string for !!str, and a plain scalar that resolves to the tag, as "12" does to !!int.
// A !!str whose text resolves to another type is quoted instead.
func (e *Encoder) encodeV3Scalar(text, tag string, style uint64, explicit bool, column int) (ast.Node, string) {
	quoted := style&(v3DoubleQuotedStyle|v3SingleQuotedStyle) != 0
	block := style&(v3LiteralStyle|v3FoldedStyle) != 0 || strings.Contains(text, "\n")
	forceQuote := false
	if !explicit && tag != "" {
		switch {
		case tag == "!!str" && (quoted || block):
			tag = ""
		case v3PlainTag(text) == tag:
			tag = ""
		case tag == "!!str":
			tag = ""
			forceQuote = true
		}
	}

	switch {
	case style&v3DoubleQuotedStyle != 0 || forceQuote:
		quotedText := strconv.Quote(text)

		return ast.String(token.New(quotedText, quotedText, e.pos(column))), tag
	case style&v3SingleQuotedStyle != 0 && !token.NeedsQuotedSpelling(text):
		quotedText := codecutils.QuoteWith(text, '\'')

		return ast.String(token.New(quotedText, quotedText, e.pos(column))), tag
	case style&(v3LiteralStyle|v3FoldedStyle) != 0 && !e.isFlowStyle && v3BlockSafe(text):
		return e.encodeV3Block(text, style&v3FoldedStyle != 0, column), tag
	case block || tag == "" && v3PlainTag(text) == "!!str":
		// A string: the encoder quotes it where its plain spelling would read as something else.
		return e.encodeString(text, column), tag
	}

	// A plain scalar of another type, or one under a tag: written as it is.
	return ast.String(token.New(text, text, e.pos(column))), tag
}

// v3BlockSafe reports whether a block scalar can hold text as it is: one that needs an escape cannot, and one whose
// first line starts with a space would need an indentation indicator.
func v3BlockSafe(text string) bool {
	return text != "" && !token.NeedsQuotedSpelling(text) && !strings.HasPrefix(text, " ")
}

// encodeV3Block writes text as a literal block, or as a folded one when folded is set.
//
// A folded block reads a single line break between two lines as a space, so each run of line breaks inside the text
// takes one more break when written. A line starting with a space or a tab is read with its breaks kept, so a text
// holding one stays literal.
//
// The header carries the chomping indicator text needs: none for one final line break, "-" for none, "+" for more.
func (e *Encoder) encodeV3Block(text string, folded bool, column int) ast.Node {
	body := strings.TrimRight(text, "\n")
	trailing := text[len(body):]
	header := "|"
	if folded && v3FoldSafe(body) {
		header = ">"
		body = v3FoldBreaks(body)
	}
	switch len(trailing) {
	case 0:
		header += "-"
	case 1:
	default:
		header += "+"
	}
	written := body + trailing
	block := ast.Literal(token.New(header, header, e.pos(column)))
	block.Value = ast.String(token.New(written, written, e.pos(column)))

	return block
}

// v3FoldSafe reports whether every line of body after the first starts with neither a space nor a tab.
func v3FoldSafe(body string) bool {
	for i := range len(body) - 1 {
		if body[i] == '\n' && (body[i+1] == ' ' || body[i+1] == '\t') {
			return false
		}
	}

	return true
}

// v3FoldBreaks returns body with one more line break in each run of breaks, as a folded block writes them.
func v3FoldBreaks(body string) string {
	var b strings.Builder
	b.Grow(len(body) + strings.Count(body, "\n"))
	for i := range len(body) {
		if body[i] == '\n' && (i == 0 || body[i-1] != '\n') {
			b.WriteByte('\n')
		}
		b.WriteByte(body[i])
	}

	return b.String()
}

// v3PlainTag returns the tag this library resolves plain text to, in the short form v3 writes. Empty text is a null.
func v3PlainTag(text string) string {
	if text == "" {
		return "!!null"
	}
	switch token.ScalarType(text, token.Schema12) {
	case token.NullType, token.ImplicitNullType:
		return "!!null"
	case token.BoolType:
		return "!!bool"
	case token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType, token.HexIntegerType:
		return "!!int"
	case token.FloatType, token.InfinityType, token.NanType:
		return "!!float"
	default:
		return "!!str"
	}
}

func (e *Encoder) encodeV3Mapping(node reflect.Value, layout *v3NodeLayout, flow bool, column int) (ast.Node, error) {
	mapping := ast.Mapping(token.New("", "", e.pos(column)), flow || e.isFlowStyle)
	content := node.Field(layout.content)
	if content.Len()%2 != 0 {
		return nil, fmt.Errorf("a yaml.Node mapping holds %d nodes, not key and value pairs", content.Len())
	}
	for i := 0; i < content.Len(); i += 2 {
		keyNode, err := e.encodeV3Child(content.Index(i), layout, column)
		if err != nil {
			return nil, err
		}
		key, isKey := keyNode.(ast.MapKeyNode)
		if !isKey {
			return nil, fmt.Errorf("a yaml.Node mapping key of type %s cannot be written as a key", keyNode.Type())
		}
		value, err := e.encodeV3Child(content.Index(i+1), layout, column)
		if err != nil {
			return nil, err
		}
		mapping.Values = append(mapping.Values, ast.MappingValue(token.New("", "", e.pos(column)), e.jsonKeyNode(key, column), value))
	}

	return mapping, nil
}

func (e *Encoder) encodeV3Sequence(node reflect.Value, layout *v3NodeLayout, flow bool, column int) (ast.Node, error) {
	sequence := ast.Sequence(token.New("-", "-", e.pos(column)), flow || e.isFlowStyle)
	content := node.Field(layout.content)
	for i := range content.Len() {
		value, err := e.encodeV3Child(content.Index(i), layout, column)
		if err != nil {
			return nil, err
		}
		sequence.Values = append(sequence.Values, value)
	}

	return sequence, nil
}

// v3Properties puts the tag and the anchor of a Node on its encoded value.
func (e *Encoder) v3Properties(value ast.Node, tag, anchor string, column int) ast.Node {
	if tag != "" {
		text := tag
		if !strings.HasPrefix(text, "!") {
			// A URI v3 cannot shorten: the verbatim form.
			text = "!<" + text + ">"
		}
		tagged := ast.Tag(token.New(text, text, e.pos(column)))
		tagged.Value = value
		value = tagged
	}
	if anchor != "" {
		anchored := ast.Anchor(token.New("&", "&", e.pos(column)))
		anchored.Name = ast.String(token.New(anchor, anchor, e.pos(column)))
		anchored.Value = value
		value = anchored
	}

	return value
}
