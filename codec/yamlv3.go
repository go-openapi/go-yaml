// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/go-openapi/go-yaml/ast"
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
