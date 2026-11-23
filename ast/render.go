// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

// DefaultIndent is the number of spaces one level of nesting adds.
const DefaultIndent = 2

// RenderOption configures a Renderer.
type RenderOption func(*Renderer)

// WithIndent sets how many spaces one level of nesting adds. Values below one
// are ignored: YAML block structure needs at least one space to exist.
func WithIndent(spaces int) RenderOption {
	return func(r *Renderer) {
		if spaces >= 1 {
			r.indent = spaces
		}
	}
}

// WithIndentSequence controls whether a block sequence under a mapping key is
// indented beneath it. It is not, by default, which is the customary YAML
// layout and what this library has always emitted.
func WithIndentSequence(on bool) RenderOption {
	return func(r *Renderer) { r.indentSequence = on }
}

// WithComments controls whether comments are written. They are, by default.
func WithComments(on bool) RenderOption {
	return func(r *Renderer) { r.comments = on }
}

// WithAliasTargets renders each alias as the node its anchor names, taking the
// nodes from anchors keyed by anchor name and otherwise from
// [AliasNode.Target], which the parser fills. Without this option an alias
// renders as the reference it was written as, "*name", which is also what an
// alias with neither falls back to.
//
// Pass an empty map to resolve from the tree alone. The map is for a tree built
// by hand, and for anchors that came from somewhere else: the decoder passes
// the ones it has collected so that a custom UnmarshalYAML receives the value
// an alias stands for instead of a reference it has no way to look up.
func WithAliasTargets(anchors map[string]Node) RenderOption {
	return func(r *Renderer) {
		r.aliasTargets = anchors
		r.resolving = make(map[string]bool, len(anchors))
	}
}

// Renderer turns an AST back into YAML text.
//
// It is a type, not a String method, so that callers can
// configure it and drive it -- the encoder uses this same renderer instead of
// building a tree and then shifting every token's column to fake indentation.
//
// Indentation comes from depth in the tree, not from the positions recorded
// when the document was read. Those positions describe where a node was, which
// stops being true the moment anything is edited, and re-reading text laid out
// from stale positions produced a document that drifted a little further on
// every cycle. Rendering here reaches a fixed point after one pass.
type Renderer struct {
	indent         int
	comments       bool
	indentSequence bool
	// aliasTargets holds the node each anchor names, and resolving the anchor
	// names being rendered right now. Both are nil unless [WithAliasTargets]
	// was passed, and a Renderer carrying them is built per call, not shared:
	// bare() copies the struct, so the copy holds the same two maps and the
	// recursion below sees its own progress through them.
	aliasTargets map[string]Node
	resolving    map[string]bool
	// src is the document a tree was parsed from, for [Renderer.Verbatim]. It
	// is nil unless [WithSource] was passed.
	src []byte
	// transform is handed each stretch of a verbatim rendering, and is nil
	// unless [WithTransform] was passed.
	transform TransformFunc
	// forceFlow lays every collection out as "{a: 1}" and "[1, 2]" whatever
	// the node's own style, for a node being written into a flow collection where a
	// line break would end the collection. See [Renderer.flowing].
	forceFlow bool
	// hoisted is the block scalar whose comment group the node opening its
	// line has already written above that line. See [Renderer.unhoisted].
	hoisted *LiteralNode
	// atMargin reports whether the node being rendered starts in column 0: a document's own
	// scalar, or a key of the block mapping that is a document's own node. A
	// plain scalar there reading "--- x" or "..." opens or closes a document.
	atMargin bool
}

// defaultRenderer and bareRenderer back the String methods of the composite
// node types. They hold no state, so one of each serves the whole package.
var (
	defaultRenderer = NewRenderer()
	bareRenderer    = NewRenderer(WithComments(false))
)

// NewRenderer returns a Renderer with two-space indentation and comments on.
func NewRenderer(opts ...RenderOption) *Renderer {
	r := &Renderer{indent: DefaultIndent, comments: true}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Render writes n to w.
func (r *Renderer) Render(w io.Writer, n Node) error {
	lw := lineWriter{w: w}
	r.fromMargin(n).render(n).writeTo(&lw, 0)

	return lw.err
}

// String renders n and returns the text.
//
// The result is relative to nothing: the node's own first line carries no
// indentation, and everything nested under it is indented from there. A caller
// placing the result somewhere indented adds that indentation itself.
//
// The first line starts in column 0, so a plain scalar there opening with "---"
// or "..." comes out quoted, as it does at the top of a document: written plain,
// it reads back as a document marker.
func (r *Renderer) String(n Node) string {
	return r.fromMargin(n).render(n).string()
}

// File renders a whole file. It is separate from String because *File is not a
// Node -- it holds documents and is not one.
func (r *Renderer) File(n *File) string {
	docs := make([]rendered, 0, len(n.Docs))
	for _, doc := range n.Docs {
		// A document with nothing in it contributes nothing, not a blank line.
		if piece := r.render(doc); !piece.empty() {
			docs = append(docs, piece)
		}
	}
	if len(docs) == 0 {
		return ""
	}

	// The final line break belongs to the file: a node's rendering never ends
	// in one, so that it can be placed anywhere.
	return join(sepNone, join(sepBreak, docs...), leaf("\n")).string()
}

// text renders n with the margin the caller stands at, for the renderer's own
// use. String is the entry point, and starts in column 0.
func (r *Renderer) text(n Node) string {
	return r.render(n).string()
}

// fromMargin returns a Renderer for n standing in column 0: a document's own
// node, or the root of what String and Render write. A scalar starts there, and
// so do the keys of a block mapping. See [Renderer.atMargin].
func (r *Renderer) fromMargin(n Node) *Renderer {
	switch node := n.(type) {
	case *StringNode:
		return r.withMargin(true)
	case *MappingNode:
		if !r.flowsInline(node.IsFlowStyle) {
			return r.withMargin(true)
		}
	}

	return r
}

// bare returns a Renderer that writes no comments, for the places a comment
// cannot go -- inside a key, where it would be read back as part of the key.
func (r *Renderer) bare() *Renderer {
	if !r.comments {
		return r
	}

	bare := *r
	bare.comments = false

	return &bare
}

// flowing is a Renderer that lays every collection out in flow style.
//
// A node inserted into "{...}" or "[...]" has to be written on one line: the
// block layout its own fields ask for would close the collection at the first
// break. The copy leaves the caller's tree alone, which a walk marking
// IsFlowStyle on the way past would not.
func (r *Renderer) flowing() *Renderer {
	if r.forceFlow {
		return r
	}

	flowing := *r
	flowing.forceFlow = true

	return &flowing
}

// flowsInline reports whether a collection is written on one line.
func (r *Renderer) flowsInline(flowStyle bool) bool {
	return r.forceFlow || flowStyle
}

// endsOnAComment reports whether n renders with a comment at the end of its
// last line, so that anything written after it would be read as part of it.
//
// It looks through the properties standing in front of a node, since an anchor
// and a tag render their value on their own line.
func endsOnAComment(n Node) bool {
	for range maxPropertyDepth {
		if n == nil {
			return false
		}
		if !n.GetComment().Blank() {
			return true
		}
		if _, isComment := n.(*CommentGroupNode); isComment {
			// The parse hung a comment on a property with nothing after it, so
			// the comment *is* the node: "&a2" over "# x" gives an anchor whose
			// value is the comment group.
			return true
		}
		switch node := n.(type) {
		case *AnchorNode:
			// Renderer.anchor writes the name's comment where the anchor's own
			// would go, so it ends the line just the same. "- # c2" over "&a2"
			// over "# " came back as "- &a2 #  # c2".
			if node.Name != nil && !node.Name.GetComment().Blank() {
				return true
			}
			n = node.Value
		case *TagNode:
			n = node.Value
		case *LiteralNode:
			// A comment set on the content is written on the header's line too.
			return !blockComment(node).Blank()
		default:
			return false
		}
	}

	return false
}

// maxPropertyDepth bounds the walk through the properties in front of a node.
const maxPropertyDepth = 64

// standsAbove returns what [BaseNode.HeadComment] holds for n, or nil. Distinct
// from headCommentOf, which reads a mapping entry's own Comment.
func standsAbove(n Node) *CommentGroupNode {
	carrier, ok := n.(headCommented)
	if !ok {
		return nil
	}

	return present(carrier.GetHeadComment())
}

// headCommented is a node carrying [BaseNode.HeadComment]. Every node type in
// this package embeds BaseNode and so satisfies it; the assertion is here
// and not a method on [Node], so that the interface does not grow.
type headCommented interface {
	GetHeadComment() *CommentGroupNode
}

// render builds what n writes, without writing it.
//
// It is String's dispatch, returning the pieces instead of the text so that a
// parent can indent a child by a number and check whether it spans lines
// without building it twice. A node type still composing strings arrives here as one
// leaf, which is correct and costs what it always did.
func (r *Renderer) render(n Node) rendered {
	if n == nil {
		return rendered{}
	}
	if lit, group := r.unhoisted(n); lit != nil {
		// Nothing wrote the group above the line, so n opens it.
		return join(sepBreak, leaf(r.commentGroup(group)), r.hoisting(lit).render(n))
	}

	return r.withHeadComment(n, r.renderNode(n))
}

// withHeadComment puts what stands above a node back above it.
//
// [BaseNode.HeadComment] is set only where the node's other comment field is
// already taken, which is why hoisting is safe here: a node that reaches this
// carries two comments, and without withHeadComment only one would render.
// Hoisting every head comment would move comments the older field already
// places correctly, and a dozen Test Suite documents would stop round-tripping.
func (r *Renderer) withHeadComment(n Node, body rendered) rendered {
	if !r.comments {
		return body
	}
	carrier, ok := n.(headCommented)
	if !ok {
		return body
	}
	cm := carrier.GetHeadComment()
	if cm.Blank() {
		return body
	}

	return join(sepBreak, leaf(r.commentGroup(cm)), body)
}

func (r *Renderer) renderNode(n Node) rendered {
	if alias, ok := n.(*AliasNode); ok && r.aliasTargets != nil {
		return leaf(r.alias(alias))
	}

	switch node := n.(type) {
	case *DocumentNode:
		return r.document(node)
	case *MappingNode:
		return r.mapping(node)
	case *MappingValueNode:
		return r.mappingValue(node)
	case *MappingKeyNode:
		return r.mappingKey(node)
	case *SequenceNode:
		return r.sequence(node)
	case *AnchorNode:
		return leaf(r.anchor(node))
	case *TagNode:
		return leaf(r.tag(node))
	case *LiteralNode:
		return leaf(r.literal(node))
	case *StringNode:
		return leaf(r.stringNode(node))
	case *DirectiveNode:
		return leaf(r.directive(node))
	case *CommentGroupNode:
		return leaf(r.commentGroup(node))
	default:
		// Scalars, aliases and everything else that occupies one line and
		// contains no nested node: their own rendering is already relative.
		if key, ok := n.(MapKeyNode); ok && !r.comments {
			return leaf(key.stringWithoutComment())
		}

		return leaf(n.String())
	}
}

// documentParts counts what a document renders: its "---", its body and its "...".
const documentParts = 3

func (r *Renderer) document(n *DocumentNode) rendered {
	parts := make([]rendered, 0, documentParts)
	if n.Start != nil {
		parts = append(parts, r.withComment(leaf(n.Start.Value), n.StartComment))
	}
	if n.Body != nil {
		parts = append(parts, r.documentBody(n.Body))
	}
	if n.End != nil {
		parts = append(parts, r.withComment(leaf(n.End.Value), n.EndComment))
	}

	return join(sepBreak, parts...)
}

func (r *Renderer) mapping(n *MappingNode) rendered {
	if len(n.Values) == 0 {
		return r.withComment(leaf("{}"), n.Comment)
	}
	if r.flowsInline(n.IsFlowStyle) {
		values := make([]Node, 0, len(n.Values))
		for _, value := range n.Values {
			values = append(values, value)
		}
		comments := flowComments{start: n.StartComment, foot: n.FootComment}
		if r.flowCarriesComments(values, comments) {
			return r.withComment(leaf(r.flowBlock("{", "}", values, comments)), n.Comment)
		}

		entries := make([]string, 0, len(n.Values))
		for _, value := range n.Values {
			entries = append(entries, r.inline(value))
		}

		return r.withComment(leaf("{"+strings.Join(entries, ", ")+"}"), n.Comment)
	}

	lines := make([]rendered, 0, len(n.Values)+1)
	if r.comments && !n.Comment.Blank() {
		lines = append(lines, r.render(n.Comment))
	}
	for _, value := range n.Values {
		lines = append(lines, r.render(value))
	}
	if r.comments && !n.FootComment.Blank() {
		lines = append(lines, join(sepNone, leaf(blankLineBefore(n.FootComment)), r.render(n.FootComment)))
	}

	return join(sepBreak, lines...)
}

func (r *Renderer) mappingValue(n *MappingValueNode) rendered {
	key := leaf(r.implicitKey(n.Key))
	// The key may stand in column 0; nothing after it does.
	r = r.withMargin(false)

	// A blank line before an entry is the author's, not the layout's: it groups
	// entries, and no amount of re-rendering should lose it. Unlike a column, it
	// does not compound when a document is read and written repeatedly.
	var head rendered
	if r.comments && !n.Comment.Blank() {
		// The gap goes above the comment, because the comment now leads the entry.
		head = join(sepNone, leaf(blankLineBefore(n.Comment)), r.render(n.Comment), leaf("\n"))
	} else {
		head = leaf(blankLineBefore(n.Key))
	}
	if lit, group := r.unhoisted(n.Value); lit != nil {
		head = join(sepNone, head, leaf(r.commentGroup(group)+"\n"))
		r = r.hoisting(lit)
	}

	if _, explicit := n.Key.(*MappingKeyNode); explicit {
		// The ':' goes on its own line. Written inline as "? a: b", YAML reads
		// the whole of "a: b" as the key.
		//
		// A comment written on that ':' line goes back on it. It has nowhere
		// else: on the value it becomes a head comment and takes the slot a
		// comment written under the ':' needs, and above the entry it collides
		// with one written above the '?'. The line comment claims the rest of
		// the line, so the value goes below whatever shape it is.
		var lineComment string
		if r.comments && !n.LineComment.Blank() {
			lineComment = " " + r.text(n.LineComment)
		}

		value := r.value(n.Value, lineComment != "")
		if r.comments && standsAbove(n.Value) != nil && !value.empty() {
			// As below: a value with a comment above it opens a line of its own.
			value = r.below(n.Value)
		} else if lineComment != "" && !value.empty() && !value.leads {
			// The comment claims the ':' line, so the value opens one of its
			// own. Renderer.value lets a plain scalar share a commented key's
			// line, which is right where the comment follows the value and
			// wrong here, where it stands in front of it: the value would be
			// written after a '#' and read back as part of the comment.
			value = r.below(n.Value)
		}

		body := join(sepNone, r.render(n.Key), leaf("\n:"), leaf(lineComment), value)

		return join(sepNone, head, body, r.footComment(n.FootComment))
	}

	// A comment on the key belongs after the ':', not before it: written where
	// the key sits, it would be read back as part of the key.
	comment := r.keyComment(n.Key)
	value := r.value(n.Value, comment != "")
	if r.comments && !value.empty() &&
		(standsAbove(n.Value) != nil || (comment != "" && endsOnAComment(n.Value))) {
		// The value opens a line of its own, for one of two reasons.
		//
		// It carries a comment above it, which cannot be written after the ':':
		// the value would then stand below the comment in column one, outside
		// the mapping, and the document would not read back. "k:" over "  # c1"
		// over "  v # c2" rendered "k: # c1" over "v # c2".
		//
		// Or the key carries a comment and the value already ends its line with
		// one. Written after that, the key's comment met the value's, and two
		// comments on one line come back as a single comment: "key: # Comment"
		// over "        # lines" over "  value" rendered
		// "key: value # lines # Comment".
		value = r.below(n.Value)
	}
	if comment == "" {
		// A comment written on the key's line, above a block, is recorded on the
		// block instead of on the key. It goes back where it was written.
		comment, value = r.hoistBlockComment(n.Key, n.Value, value)
	}

	var inline, trailing rendered
	switch {
	case comment == "":
	case value.leads:
		inline = leaf(" " + comment)
	default:
		trailing = leaf(" " + comment)
	}

	colon := r.colonAfter(n.Key)
	if implicitKeyTooLong(key.text, colon) {
		// Written as an explicit key, "? k" over ": v", which has no length limit. In a flow collection the line
		// break becomes a space: "{? k : v}".
		return join(sepNone, head, leaf("? "), key, leaf("\n:"), inline, value, trailing,
			r.footComment(n.FootComment))
	}

	return join(sepNone, head, key, leaf(colon), inline, value, trailing,
		r.footComment(n.FootComment))
}

// implicitKey renders a key written before its ':' on one line.
//
// An implicit key cannot span lines, so a string that would render as a "|" block is written double-quoted, where a
// line break is an escape. Flattened onto the key's line, "a\nb" came out as "|-   a   b: 1", which does not parse.
// An explicit "? " key may still be a block: it takes the MappingKeyNode path.
func (r *Renderer) implicitKey(key Node) string {
	if s, ok := key.(*StringNode); ok && blockScalarHeader(s) != "" {
		return strconv.Quote(s.Value)
	}

	return r.bare().inline(key)
}

// maxImplicitKey is the most characters YAML 1.2 lets an implicit key take, the key as written and the space before
// its ':' together (productions 154 and 155). libyaml and the readers built on it, go.yaml.in/yaml/v3 among them,
// reject a longer one, in a block mapping and in a flow mapping alike.
const maxImplicitKey = 1024

// implicitKeyTooLong reports whether a key written as key, followed by colon, takes more than maxImplicitKey
// characters before its ':'.
func implicitKeyTooLong(key, colon string) bool {
	if len(key)+len(colon)-1 <= maxImplicitKey {
		// A character takes at least one byte, so a key this short in bytes is short enough.
		return false
	}

	return utf8.RuneCountInString(key)+len(colon)-1 > maxImplicitKey
}

// colonAfter returns the ':' that closes a key, with the separating space the
// key needs in front of it.
//
// An anchor name, an alias name and a tag shorthand may all contain ':', so a
// key that ends on one absorbs the ':' written straight after it: "&a: v"
// anchors "a:" over the scalar v, where "&a : v" anchors the empty key of a
// mapping. Only the space separates the two readings.
func (r *Renderer) colonAfter(key Node) string {
	if r.absorbsColon(key) {
		return " :"
	}
	return ":"
}

func (r *Renderer) absorbsColon(n Node) bool {
	switch nn := n.(type) {
	case *AliasNode:
		// An alias is its name and nothing else.
		return true
	case *AnchorNode:
		return r.endsOnProperty(nn.Value)
	case *TagNode:
		return r.endsOnProperty(nn.Value)
	}
	return false
}

// endsOnProperty reports whether a property's node leaves the property itself
// last on the line -- because the node is the empty scalar, or because it is
// another property in the same position.
func (r *Renderer) endsOnProperty(n Node) bool {
	if n == nil {
		return true
	}
	if r.absorbsColon(n) {
		return true
	}
	return r.bare().text(n) == ""
}

// hoistBlockComment takes a block collection's own leading comment off the
// front of its rendered value, so that the caller can put it back on the key's
// line. It returns the comment and what is left of the value.
func (r *Renderer) hoistBlockComment(key, n Node, value rendered) (string, rendered) {
	if !r.comments || !value.leads {
		return "", value
	}

	var comment *CommentGroupNode
	switch node := n.(type) {
	case *MappingNode:
		if r.flowsInline(node.IsFlowStyle) || len(node.Values) == 0 {
			return "", value
		}
		comment = node.Comment
	case *SequenceNode:
		if r.flowsInline(node.IsFlowStyle) || len(node.Values) == 0 {
			return "", value
		}
		comment = node.Comment
	}
	if comment.Blank() || !sameLine(comment, key) {
		// Written on its own line above the block, it is a comment on the block
		// and stays there.
		return "", value
	}

	// The comment is the block's first line, wherever value() indented it to.
	// Dropping a line means reading the text, which is the one place the pieces
	// have to be flattened early. It costs the subtree, and only for a block
	// whose own comment was written on the key's line.
	_, rest, found := strings.Cut(value.string()[1:], "\n")
	if !found {
		return "", value
	}

	return r.text(comment), leaf("\n" + rest)
}

// sameLine reports whether two nodes were written on the same source line.
//
// A node built in code rather than read from a document has no line. Nothing
// separates it from its neighbors, so it counts as sharing theirs: a comment
// attached by a caller was attached to that entry, not to a line of its own.
func sameLine(a, b Node) bool {
	ta, tb := a.GetToken(), b.GetToken()
	// Line counts from 1, so a zero line is a token the encoder built rather
	// than one read from a document. Nothing is known about where it stands, so
	// it is taken to stand where the other one does.
	if ta == nil || tb == nil || ta.Position.Line == 0 || tb.Position.Line == 0 {
		return true
	}

	return ta.Position.Line == tb.Position.Line
}

// keyComment returns the comment carried by a mapping key, rendered, or "".
func (r *Renderer) keyComment(key Node) string {
	if !r.comments {
		return ""
	}
	comment := key.GetComment()
	if comment == nil {
		return ""
	}

	return r.text(comment)
}

// value renders what follows a "key:", including the space or newline that
// separates it. It returns "" for an absent value.
//
// keyCommented reports whether the key carries a comment. A key comment takes
// the rest of the line: a collection that would otherwise sit beside its key goes below it so
// that the comment stays next to the key it belongs to.
// alias renders n as the node its anchor names.
//
// An anchor whose node holds the alias would render forever, so a name already
// being rendered falls back to the reference: "&a1 [*a1]" renders the inner
// alias as "*a1" instead of exhausting the stack.
func (r *Renderer) alias(n *AliasNode) string {
	name := n.Value.GetToken().Value
	target := r.aliasTarget(n, name)
	if target == nil || r.resolving[name] {
		return n.String()
	}

	r.resolving[name] = true
	defer delete(r.resolving, name)

	return r.text(target)
}

// deref returns the node an alias names, for the layout checks below that
// test the shape of a value -- whether it fits on the key's line, whether it
// is a block sequence, whether it carries its own indentation. An alias renders
// as its target, so those checks examine the target.
//
// It resolves one step and does not guard against a cycle: rendering goes
// through [Renderer.alias], which does, and one step is enough to find the shape.
func (r *Renderer) deref(n Node) Node {
	alias, ok := n.(*AliasNode)
	if !ok || r.aliasTargets == nil {
		return n
	}

	if target := r.aliasTarget(alias, alias.Value.GetToken().Value); target != nil {
		return target
	}

	return n
}

// aliasTarget returns the node an alias names, from the anchors the caller
// passed or from the alias itself.
//
// The parser fills [AliasNode.Target] where the alias stands, so a node
// rendered on its own carries what its aliases name even when the caller's
// table was built from something else -- the decoder hands a custom
// UnmarshalYAML one node and the anchors of the whole file, and an alias whose
// anchor stood outside that node was in neither.
func (r *Renderer) aliasTarget(n *AliasNode, name string) Node {
	if target := r.aliasTargets[name]; target != nil {
		return target
	}

	return n.Target
}

func (r *Renderer) value(n Node, keyCommented bool) rendered {
	if n == nil {
		return rendered{}
	}

	text := r.render(n)
	if text.empty() {
		return rendered{}
	}

	shape := r.deref(n)
	spansLines := text.spans
	collection := isCollection(shape)

	// A flow collection fits on the key's line only while it stays on one line.
	// A comment forces it onto several, and then the lines below it carry no
	// indentation of their own: its closing bracket would land in column one,
	// outside the mapping it belongs to, and the document it wrote would not
	// read back.
	//
	// A commented key also sends a value written over several lines below it,
	// whatever kind of node stands at the top of them. fitsOnKeyLine returns
	// true for an anchor and a tag, since they carry their own value and set
	// their own shape -- but the comment takes the rest of the key's line, so what
	// follows cannot start there. Kept on it, the comment was pushed past the
	// whole value and came back on the last line of it: "k: # c1" over "&a2"
	// over "- 1" rendered as "k: &a2" over "- 1 # c1", and where the last entry
	// was a block scalar the comment landed inside its content and changed the
	// value.
	if r.fitsOnKeyLine(shape) && (!collection || !spansLines) &&
		(!keyCommented || (!collection && !spansLines)) {
		return join(sepNone, leaf(" "), text)
	}
	if sequence, ok := shape.(*SequenceNode); ok && !r.flowsInline(sequence.IsFlowStyle) && !r.indentSequence {
		// A block sequence under a mapping key sits at the key's own
		// indentation unless asked otherwise: "key:" then "- item" in column
		// one of the key's level. Both layouts are legal; this is the one YAML
		// is usually written in. A flow sequence is not laid out this way: it
		// is a value like any other and indents under its key.
		return join(sepNone, leaf("\n"), text)
	}
	if statesWidth(n) {
		// The header opens a line of its own, one level in from the key.
		text = r.lifted(n, r.indent)
	}

	return join(sepNone, leaf("\n"), text.indentedBy(r.indent))
}

// below renders a value on a line of its own, indented under whatever
// introduced it.
//
// A block sequence sits at its key's own indentation unless asked otherwise,
// which is the one shape that does not take the extra level.
func (r *Renderer) below(n Node) rendered {
	text := r.lifted(n, r.indent)
	if sequence, ok := r.deref(n).(*SequenceNode); ok && !r.flowsInline(sequence.IsFlowStyle) && !r.indentSequence {
		return join(sepNone, leaf("\n"), text)
	}

	return join(sepNone, leaf("\n"), text.indentedBy(r.indent))
}

// fitsOnKeyLine reports whether a value belongs after its key on the same line.
// Everything that renders as a block of its own goes below it instead.
func (r *Renderer) fitsOnKeyLine(n Node) bool {
	switch node := n.(type) {
	case *MappingNode:
		return r.flowsInline(node.IsFlowStyle) || len(node.Values) == 0
	case *SequenceNode:
		return r.flowsInline(node.IsFlowStyle) || len(node.Values) == 0
	case *AnchorNode, *TagNode, *LiteralNode:
		// These carry their own value, which may itself be a block; they decide
		// their own shape, and start on the key's line either way.
		return true
	default:
		return true
	}
}

func (r *Renderer) mappingKey(n *MappingKeyNode) rendered {
	r = r.withMargin(false)

	var above string
	if lit, group := r.unhoisted(n.Value); lit != nil {
		above = r.commentGroup(group) + "\n"
		r = r.hoisting(lit)
	}

	value := r.entry(n.Value)
	if value.empty() {
		return leaf(above + n.Start.Value)
	}
	if value.leads {
		// The key opens below its "?" with a blank line between, and a blank
		// line is two breaks: one ends the "?"'s own line and one is the gap.
		// Written with the single break the entry carries, the gap was lost and
		// the next rendering pulled the key back up onto the "?" line, so the
		// document moved on every pass.
		return join(sepNone, leaf(above+n.Start.Value+" "), leaf("\n"), value)
	}

	return join(sepNone, leaf(above+n.Start.Value+" "), value)
}

// entry renders a node placed after a marker that occupies the start of its
// line -- "- " or "? " -- indenting its continuation lines to sit under it.
func (r *Renderer) entry(n Node) rendered {
	piece := r.render(n)
	if carriesOwnIndent(r.deref(n)) {
		return piece
	}

	if piece.leads {
		// A blank line ends the marker's line, so what follows does not stand
		// after the marker: it opens a line of its own and takes the
		// indentation every other line under the marker takes. Written as a
		// hanging indent it would land in column one, outside the key it
		// belongs to -- "?" over a blank line over a mapping would come back as
		// two entries of the document instead of one key, and the rendered text
		// would not read as the document that was parsed.
		return join(sepNone, leaf("\n"), piece.withoutLead().indentedBy(r.indent))
	}

	// The marker already holds the first line, so the piece is written where it
	// left off and only the lines under it take the indentation: the writer
	// puts the padding in at each line break.
	//
	// The lines under it hang by the marker's width, not by the indent: the
	// first line starts after "- ", and a mapping or a sequence written there
	// has its later lines in that same column. Hung by an indent of 4, "- a: 1"
	// over "    b: 2" put b deeper than a, and the document did not read back.
	return piece.hangingBy(markerWidth)
}

// markerWidth is the width of the "- " and "? " markers, after which an entry
// starts on the marker's line.
const markerWidth = 2

func (r *Renderer) sequence(n *SequenceNode) rendered {
	if len(n.Values) == 0 {
		return r.withComment(leaf("[]"), n.Comment)
	}
	if r.flowsInline(n.IsFlowStyle) {
		comments := flowComments{start: n.StartComment, heads: n.ValueHeadComments, foot: n.FootComment}
		if r.flowCarriesComments(n.Values, comments) {
			return r.withComment(leaf(r.flowBlock("[", "]", n.Values, comments)), n.Comment)
		}

		entries := make([]string, 0, len(n.Values))
		for _, value := range n.Values {
			entries = append(entries, r.inline(value))
		}

		return r.withComment(leaf("["+strings.Join(entries, ", ")+"]"), n.Comment)
	}

	lines := make([]rendered, 0, len(n.Values)+1)
	if r.comments && !n.Comment.Blank() {
		lines = append(lines, r.render(n.Comment))
	}
	for i, value := range n.Values {
		// A comment group on a block scalar has no room on the "- " line.
		er, above := r, ""
		if lit, group := r.unhoisted(value); lit != nil {
			er, above = r.hoisting(lit), r.commentGroup(group)+"\n"
		}

		// A blank line inside an entry surfaces as a leading break on the
		// entry's own text. It belongs above the "- ", not after it.
		entry := er.entry(value)
		var blank string
		if entry.leads {
			blank, entry = "\n", entry.withoutLead()
		}
		if head := headOf(n, i); r.comments && !head.Blank() {
			if blank == "" {
				// The entry's own token follows the comment, so the gap the
				// author left shows up above the comment instead.
				blank = blankLineBefore(head)
			}
			lines = append(lines, join(sepNone, leaf(blank), r.render(head)))
			blank = ""
		} else if blank == "" {
			blank = r.entryGap(n, i, value, head)
		}
		comment := r.entryLineComment(n, i)
		if comment != "" && (carriesOwnIndent(value) ||
			!r.fitsOnKeyLine(value) || entry.spans || endsOnAComment(value)) {
			// Everything after the '#' is commented out, so a value that would
			// share the dash's line goes below it instead.
			//
			// A block scalar goes below it too. Kept on the dash's line, it had
			// the comment written after the whole entry, which is after its last
			// line of content: "- # c" over "  |2-" over "   x" came back as
			// "- |2-" over "   x # c" and read " x # c". Below the dash, a width
			// its header states counts from the sequence, as Renderer.lifted
			// has it.
			//
			// fitsOnKeyLine returns true for an anchor and a tag, which carry
			// their own value and set their own shape, so a property standing on a
			// block kept the dash's line and the comment was written after the
			// whole entry -- on the last line of it. "- # c" over "&a2" over a
			// mapping holding a folded scalar came back with the comment inside
			// the scalar's content. The same shape under a mapping key is
			// Renderer.value's to place.
			//
			// endsOnAComment is the other way it goes wrong: a value carrying a
			// comment of its own ends its line with one, and the entry's
			// comment written after that met it. Two comments on one line come
			// back as a single comment, "#" inside one being ordinary text, so
			// "- # c1" over "  &a q # c2" lost one to
			// "- &a q # c2 # c1".
			lines = append(lines,
				leaf(blank+above+"-"+comment),
				er.lifted(value, r.indent).indentedBy(r.indent))

			continue
		}
		lines = append(lines, join(sepNone, leaf(blank+above+"- "), entry, leaf(comment)))
	}
	if r.comments && !n.FootComment.Blank() {
		lines = append(lines, join(sepNone, leaf(blankLineBefore(n.FootComment)), r.render(n.FootComment)))
	}

	return join(sepBreak, lines...)
}

// entryGap returns the blank line the author left above entry i of n, value,
// where no head comment is rendered to carry it.
func (r *Renderer) entryGap(n *SequenceNode, i int, value Node, head *CommentGroupNode) string {
	// A head comment the caller removed still records where the author left
	// a gap, and nothing else records it: the entry's own token stands
	// under the comment, not under the blank line.
	if r.comments && head != nil {
		if blank := blankAboveComment(head); blank != "" {
			return blank
		}
	}
	// Only a block collection reports a gap of its own. For anything else the
	// sequence reads it off the entry's first token, falling back to the '-'.
	// An empty entry has no token of its own to read: ": &1" over a blank over
	// "-" over "? \"\"" wrote the '-' with nothing above it and lost the
	// author's blank line.
	if blank := blankLineBefore(value); blank != "" {
		return blank
	}

	return entryBlankLine(n, i)
}

// headOf returns the comment written above the entry at index i, or nil.
func headOf(n *SequenceNode, i int) *CommentGroupNode {
	if i >= len(n.ValueHeadComments) {
		return nil
	}

	return n.ValueHeadComments[i]
}

// blankAboveComment returns the blank line an author left above a comment group,
// reading the group's first comment whether or not it is still written.
//
// CommentGroupNode.GetToken reads only the comments still written, which every
// other renderer path needs. Here the group has been emptied by
// [CommentNode.Remove] and the gap the author left above it is still the
// document's: "- a" over a blank over "# head" over "- b" keeps its blank line
// when the comment goes.
func blankAboveComment(c *CommentGroupNode) string {
	if c == nil || len(c.Comments) == 0 {
		return ""
	}
	if tk := c.Comments[0].Token; tk != nil && tk.BlankLineAbove() {
		return "\n"
	}

	return ""
}

// entryBlankLine returns the blank line an author left above the '-' of the
// entry at index i, or "".
func entryBlankLine(n *SequenceNode, i int) string {
	if i >= len(n.Entries) || n.Entries[i] == nil || n.Entries[i].Start == nil {
		return ""
	}
	if n.Entries[i].Start.BlankLineAbove() {
		return "\n"
	}

	return ""
}

// entryLineComment returns the comment written on the entry's own line.
//
// It is recorded on the entry instead of on its value, because an entry whose
// value is written below it -- or is not written at all -- has nothing on that
// line to carry it. Only the entries hold it, so a renderer that does not read
// them drops the comment.
func (r *Renderer) entryLineComment(n *SequenceNode, i int) string {
	if !r.comments || i >= len(n.Entries) || n.Entries[i] == nil {
		return ""
	}

	comment := n.Entries[i].LineComment
	if comment.Blank() {
		return ""
	}

	return " " + r.text(comment)
}

func (r *Renderer) anchor(n *AnchorNode) string {
	// The name is rendered without its comment, and the comment is put back at
	// the end of the line with the anchor's own.
	//
	// A comment written beside an anchor is hung on the name, and rendering the
	// name with it wrote the comment into the marker: "&a # beside" over "  q"
	// became "&a # beside q", so the value stood inside the comment and the
	// document read back as null. The tag below never had this: its comment
	// reaches it through the value, not through a name.
	own := firstComment(n.Comment, n.Name.GetComment())

	return r.withOwnComment(own, r.prefixed("&"+r.bare().text(n.Name), n.Value, own))
}

// firstComment returns whichever of two comment groups is present, preferring
// the first. Two comments cannot share the end of one line: written there they
// come back as a single comment, "#" inside one being ordinary text.
// blockComment returns the comment a block scalar writes on its header's line: its own, then the one set on its
// content.
//
// A block scalar's content is text with no line of its own, so a comment set on [LiteralNode.Value] is written
// as the block scalar's. The parser never puts one there; a caller may. Where both hold a comment, both are
// written, one line each, which is a group too long for the header's line.
func blockComment(n *LiteralNode) *CommentGroupNode {
	var content *CommentGroupNode
	if n.Value != nil {
		content = n.Value.GetComment()
	}

	switch {
	case content.Blank():
		return n.Comment
	case n.Comment.Blank():
		return content
	}

	return &CommentGroupNode{Comments: append(append([]*CommentNode{}, n.Comment.Comments...), content.Comments...)}
}

func firstComment(own, borrowed *CommentGroupNode) *CommentGroupNode {
	if !own.Blank() {
		return own
	}

	return present(borrowed)
}

func (r *Renderer) tag(n *TagNode) string {
	if n.Implicit {
		// The parse resolved a type the document left implicit, so the document
		// holds no tag to write back. See TagNode.Implicit.
		return r.withOwnComment(n.Comment, r.text(n.Value))
	}

	return r.withOwnComment(n.Comment, r.prefixed(n.Start.Value, n.Value, n.Comment))
}

// withOwnComment puts a property's own comment back at the end of the line the
// property ends.
//
// An anchor and a tag both reach a comment written beside them through the node
// they render -- the name for an anchor, the value for a tag -- and both may
// also carry one on the property node itself. Rendering only the marker dropped
// that one: "!!null # c1" and "- &a2 !!seq # c2" came back without theirs,
// while "!!null null # c1" kept one because the comment had been hung on the
// scalar instead. An anchor lost the comment newMappingValueNode hangs there
// for an explicit key, so "? a" over ": # c3" over "  &a2 v" wrote no comment
// at all.
//
// The first line is the property's, whatever follows: a property ending its
// line has either no value or a block written underneath, and one with a value
// beside it carries no comment of its own.
func (r *Renderer) withOwnComment(comment *CommentGroupNode, text string) string {
	if !r.comments || comment.Blank() {
		return text
	}

	head, rest, wrapped := strings.Cut(text, "\n")
	head = addCommentString(head, comment)
	if !wrapped {
		return head
	}

	return head + "\n" + rest
}

// prefixed renders a node introduced by a marker -- an anchor name or a tag --
// which sits on its own line when what follows is a block.
// own is the comment withOwnComment will put back at the end of the marker's
// line, and controls whether the value may share that line.
func (r *Renderer) prefixed(marker string, value Node, own *CommentGroupNode) string {
	return r.prefixedAt(marker, value, own, 0)
}

// prefixedAt is [Renderer.prefixed] for a marker whose line starts lift
// columns in from the node enclosing it. See [Renderer.lifted].
func (r *Renderer) prefixedAt(marker string, value Node, own *CommentGroupNode, lift int) string {
	if value == nil {
		return marker
	}
	r = r.withMargin(false)

	text := r.text(value)
	if lift > 0 {
		// A marker does not enclose what it names: "&a |2" is still the
		// enclosing node's child, and the width its header states is counted
		// from the same place.
		text = r.lifted(value, lift).string()
	}
	if text == "" {
		return marker
	}
	if r.startsBlock(value) {
		if _, isSequence := value.(*SequenceNode); isSequence && !r.indentSequence {
			return marker + "\n" + text
		}

		return marker + "\n" + r.indented(text)
	}
	if r.comments && (standsAbove(value) != nil || (own != nil && endsOnAComment(value))) {
		// The value cannot share the marker's line, for one of two reasons.
		// Either text opens with the comment written above the value, which
		// beside the marker lands on the marker's line and swallows the
		// marker's own comment -- "- !foo # c2" over "# c3" over "\"rqSb\"" came
		// back as "- !foo # c3 # c2". Or the value ends on a comment and the
		// marker carries one too, which put both onto one line the other way
		// round: "- &a1 # c2" over "K # c3" came back as "- &a1 K # c3 # c2".
		// A comment runs to the end of its line, so either way the next read
		// takes the two for one. A property may stand on its own line with its
		// node underneath.
		if statesWidth(value) {
			text = r.lifted(value, lift+r.indent).string()
		}

		return marker + "\n" + r.indented(text)
	}

	return marker + " " + text
}

// isCollection reports whether n is a mapping or a sequence, in either style.
func isCollection(n Node) bool {
	switch n.(type) {
	case *MappingNode, *SequenceNode:
		return true
	default:
		return false
	}
}

func (r *Renderer) startsBlock(n Node) bool {
	switch node := n.(type) {
	case *MappingNode:
		return !r.flowsInline(node.IsFlowStyle) && len(node.Values) > 0
	case *SequenceNode:
		return !r.flowsInline(node.IsFlowStyle) && len(node.Values) > 0
	default:
		return false
	}
}

// documentBody renders what a document holds.
//
// A document encloses nothing: the spec gives its node an indentation of -1,
// so a block scalar written one column in states a width of two. See
// [Renderer.lifted].
func (r *Renderer) documentBody(n Node) rendered {
	return r.fromMargin(n).lifted(n, 1)
}

// withMargin returns a Renderer whose next node starts in column 0 or not. See
// [Renderer.atMargin].
func (r *Renderer) withMargin(on bool) *Renderer {
	if r.atMargin == on {
		return r
	}

	margin := *r
	margin.atMargin = on

	return &margin
}

// isQuoted reports whether n was read from a quoted scalar.
func isQuoted(n *StringNode) bool {
	return n.Token != nil &&
		(n.Token.Type == token.SingleQuoteType || n.Token.Type == token.DoubleQuoteType)
}

// readsAsMarker reports whether n is a plain one-line scalar whose text opens
// with "---" or "...", which written plain in column 0 reads back as a document
// marker.
func readsAsMarker(n *StringNode) bool {
	return blockScalarHeader(n) == "" && !isQuoted(n) && startsWithDocumentMarker(n.Value)
}

// quotedMarker returns n single-quoted, with its comment where comments is set.
// Single quotes keep the value and its type: a scalar opening with "---" or
// "..." resolves to a string either way.
func quotedMarker(n *StringNode, comments bool) string {
	quoted := "'" + strings.ReplaceAll(n.Value, "'", "''") + "'"
	if comments && !n.Comment.Blank() {
		return addCommentString(quoted, n.Comment)
	}

	return quoted
}

// startsWithDocumentMarker reports whether value, written plain at the start of
// a line, reads as a "---" or "..." marker: three dashes or three dots followed
// by a blank or by nothing.
//
// An indented "    ---" over "false" is the plain scalar "--- false", which
// written back in column 0 read as a document holding false, and a root key
// "  --- x" came back as "--- x: 1", which does not parse.
func startsWithDocumentMarker(value string) bool {
	if !strings.HasPrefix(value, "---") && !strings.HasPrefix(value, "...") {
		return false
	}
	if len(value) == len("---") {
		return true
	}

	switch value[len("---")] {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

// lifted renders n with the start of its line lift columns in from the
// indentation of the node enclosing it.
//
// It differs from render in one respect, and only for a block scalar that
// states its own indentation. That width is counted from the indentation of
// the enclosing node, not from the line the header stands on. The two agree
// for "k: |2", where the header shares its key's line. They differ at the
// document root, which is lifted by one, and for a header written on a line
// below its key: "k: # c" over "  |2-" counts from the mapping in column 0, so
// the content written four columns in states four.
//
// A property may stand between the enclosing node and the scalar -- "&a |2"
// is still the node its header counts from -- so those are unwrapped rather
// than handed to render.
func (r *Renderer) lifted(n Node, lift int) rendered {
	if lit, group := r.unhoisted(n); lit != nil {
		return join(sepBreak, leaf(r.commentGroup(group)), r.hoisting(lit).lifted(n, lift))
	}

	switch node := n.(type) {
	case *LiteralNode:
		return r.withHeadComment(n, leaf(r.literalAt(node, lift)))
	case *AnchorNode:
		// The name without its comment, as Renderer.anchor does and for the
		// same reason.
		own := firstComment(node.Comment, node.Name.GetComment())

		return r.withHeadComment(n,
			leaf(r.withOwnComment(own,
				r.prefixedAt("&"+r.bare().text(node.Name), node.Value, own, lift))))
	case *TagNode:
		if node.Implicit {
			// No tag to write back, as in Renderer.tag.
			return r.withHeadComment(n, leaf(r.withOwnComment(node.Comment, r.lifted(node.Value, lift).string())))
		}

		return r.withHeadComment(n,
			leaf(r.withOwnComment(node.Comment, r.prefixedAt(node.Start.Value, node.Value, node.Comment, lift))))
	default:
		// render wraps it already.
		return r.render(n)
	}
}

// statesWidth reports whether n is a block scalar whose header states its
// indentation, looking through the properties in front of it.
func statesWidth(n Node) bool {
	for range maxPropertyDepth {
		switch node := n.(type) {
		case *LiteralNode:
			return statedIndent(node.Start.Value) > 0
		case *AnchorNode:
			n = node.Value
		case *TagNode:
			n = node.Value
		default:
			return false
		}
	}

	return false
}

// unhoisted returns the block scalar standing at n, looking through the
// properties in front of it, when its comment runs over more than one line and
// nothing has written the comment yet.
//
// A block scalar's header has room for one comment: the content starts on the
// next line, and a second comment line there ends the scalar before it. "k: |2-
// #c" over "#d" over "  x" does not parse. The whole group goes above the line
// the header stands on instead, written by whatever opens that line -- the
// mapping entry, the "- " of a sequence entry, the "?" of an explicit key, or
// the scalar itself.
func (r *Renderer) unhoisted(n Node) (*LiteralNode, *CommentGroupNode) {
	if !r.comments {
		return nil, nil
	}

	for range maxPropertyDepth {
		switch node := n.(type) {
		case *LiteralNode:
			comment := blockComment(node)
			if node == r.hoisted || commentLines(comment) < 2 {
				return nil, nil
			}

			return node, comment
		case *AnchorNode:
			n = node.Value
		case *TagNode:
			n = node.Value
		default:
			return nil, nil
		}
	}

	return nil, nil
}

// hoisting returns a Renderer that leaves lit's comment out, the caller having
// written it above the line.
func (r *Renderer) hoisting(lit *LiteralNode) *Renderer {
	hoisting := *r
	hoisting.hoisted = lit

	return &hoisting
}

// commentLines counts the comments of a group that are still written.
func commentLines(c *CommentGroupNode) int {
	if c == nil {
		return 0
	}

	var lines int
	for _, comment := range c.Comments {
		if !comment.Removed() {
			lines++
		}
	}

	return lines
}

func (r *Renderer) literal(n *LiteralNode) string {
	return r.literalAt(n, 0)
}

func (r *Renderer) literalAt(n *LiteralNode, lift int) string {
	header := n.Start.Value

	// The content is written at the renderer's own width, so a header that
	// states a width must state the renderer's: "a: |1" over "  x" comes back
	// as "a: |2" over "   x", holding the same " x". Carrying the source's
	// digit over would leave it describing a layout that is gone, and the value
	// would gain a column on every cycle. [Renderer.VerbatimFile] keeps the digit
	// the document wrote.
	if statedIndent(header) > 0 {
		header = restateIndent(header, r.indent+lift)
	}

	if comment := blockComment(n); r.comments && !comment.Blank() && n != r.hoisted {
		header += " " + r.text(comment)
	}

	value := n.Value.Value
	if value == "" {
		// An empty block scalar is its header. Writing the line break that
		// would introduce content leaves a blank line the parser reads as
		// content indented differently from what the header announced.
		//
		// '+' comes off: it keeps every trailing break, and the break that ends
		// the header line is one. A document written with a final line break --
		// every document is -- would read "|+" back as "\n" instead of as the
		// empty value written here. Clipping an empty value leaves it empty, so
		// dropping the indicator keeps the value and gains a document that
		// survives being read back.
		return withoutKeepChomping(header)
	}

	indent := r.indent

	if isFolded(n.Start) {
		// A folded scalar's value has lost its line structure -- that is what
		// folding is -- so it can only be written from the text it was read
		// from.
		return header + r.foldedFromSource(n, indent)
	}

	// A literal scalar's value is its content exactly, and the header sets how
	// to write it: each line indented and closed by a break, with the last
	// break left to whatever follows the node. The chomping indicator needs no
	// arithmetic here: it already set how many trailing breaks the value has,
	// and writing them all back keeps the blank lines of "|+".
	lbc := lineBreakOf(value)
	body := blockScalarBody(value, indent, lbc)

	return header + lbc + strings.TrimSuffix(body, lbc)
}

// withoutKeepChomping returns header with its '+' removed, leaving any width
// indicator where it stands.
func withoutKeepChomping(header string) string {
	return strings.ReplaceAll(header, "+", "")
}

func isFolded(tk *token.Token) bool {
	return tk.Type == token.FoldedType
}

// foldedFromSource writes a folded scalar's content back from the source text,
// stripping the indentation that introduced it by the width the decoder
// stripped -- not by the least indented line. The two differ exactly when the
// header states a width and every content line starts with spaces of its own,
// and those spaces are part of the value.
//
// The blank lines the source ends on are not read back from it. The chomping
// indicator sets them, and the value already holds its result: the source ends
// the same way whether the header reads ">", ">-" or ">+", so writing its tail back would keep blank lines for the two
// styles that discard them.
func (r *Renderer) foldedFromSource(n *LiteralNode, indent int) string {
	value := n.Value.Value
	origin := n.Source

	// srcBreak holds the line break the source wrote, and foldedFromSource
	// reads the origin with it. The renderer does not write it back: it writes
	// "\n" whatever the document used, as it does everywhere else. Copying the
	// source's break here would render a CRLF document to a mixture of both,
	// so it would never settle; with a lone CR the content lines would drift a
	// column right, a fold would stop folding, and in a nested position the
	// result would not be a block scalar at all.
	srcBreak := lineBreakOf(origin)

	var content string
	if strings.Trim(value, "\r\n") != "" {
		// A value that is nothing but line breaks has no content lines to write
		// back, whatever the source looks like: every one of its lines was
		// blank, and the tail below is the whole of it.
		lead := introducedIndent(origin, value, srcBreak)
		content = trimTrailingBlankLines(origin, srcBreak, lead)

		// Before dedenting, not after: dedentBy takes the lines apart at "\n",
		// so a document written with lone carriage returns is one line to it
		// and only the first loses its indentation.
		content = strings.ReplaceAll(content, srcBreak, "\n")
		content = dedentBy(content, lead)
	}

	blanks := trailingBreaks(value, lineBreakOf(value))
	if content != "" {
		// One of the value's trailing breaks ends its last line of content
		// instead of standing for a blank line of its own.
		blanks--
	}

	var body strings.Builder
	if content != "" {
		body.WriteString(indentLinesWith(content, indent, "\n"))
		body.WriteString("\n")
	}
	for range max(blanks, 0) {
		body.WriteString("\n")
	}

	// The last break belongs to whatever follows the node, the same way the
	// literal spelling leaves it.
	return "\n" + strings.TrimSuffix(body.String(), "\n")
}

// trimTrailingBlankLines removes the lines with nothing on them that a block
// scalar's source ends on, and the break closing its last line of content.
//
// A line of spaces is blank only up to the width that introduced the block.
// Past that width the spaces are content -- a folded scalar treats a line
// indented further than its neighbors literally, so those are the one kind of
// trailing whitespace that has to be written back.
func trimTrailingBlankLines(text, lbc string, lead int) string {
	lines := strings.Split(text, lbc)

	end := len(lines)
	for end > 0 && strings.TrimLeft(lines[end-1], " \t") == "" && len(lines[end-1]) <= lead {
		end--
	}

	return strings.Join(lines[:end], lbc)
}

// trailingBreaks counts the line breaks a value ends on.
func trailingBreaks(value, lbc string) int {
	count := 0
	for strings.HasSuffix(value, lbc) {
		value = strings.TrimSuffix(value, lbc)
		count++
	}

	return count
}

// blockScalarBody writes a block scalar's content: every line indented and
// closed by a line break. A line with nothing on it stays empty -- indenting it
// would leave trailing spaces on a blank line.
func blockScalarBody(value string, indent int, lbc string) string {
	lines := strings.Split(value, lbc)
	if strings.HasSuffix(value, lbc) {
		// The split leaves an empty element past the final break, which is not
		// a line of the content.
		lines = lines[:len(lines)-1]
	}

	pad := strings.Repeat(" ", indent)

	var body strings.Builder
	for _, line := range lines {
		if line != "" {
			body.WriteString(pad)
			body.WriteString(line)
		}
		body.WriteString(lbc)
	}

	return body.String()
}

// lineBreakOf returns the line break a scalar's content is written with.
//
// A value holding no break at all is written with the ordinary one:
// token.DetectLineBreakCharacter returns "\r\n" for that case, which is right
// for deciding what a file uses and wrong for deciding what to write here.
func lineBreakOf(value string) string {
	if !strings.ContainsAny(value, "\r\n") {
		return "\n"
	}

	return token.DetectLineBreakCharacter(value)
}

// statedIndent returns the indentation a block scalar header states, or 0
// when it leaves the width to be inferred from the content.
func statedIndent(header string) int {
	for _, c := range header {
		if c >= '1' && c <= '9' {
			return int(c - '0')
		}
	}

	return 0
}

// restateIndent rewrites the width a block scalar header states, leaving the
// style and the chomping indicator around it alone. YAML allows one digit, so a
// width outside that range cannot be written and the header is left as it was.
func restateIndent(header string, width int) string {
	if width < 1 || width > 9 {
		return header
	}

	var out strings.Builder
	for _, c := range header {
		if c >= '1' && c <= '9' {
			out.WriteByte(byte('0' + width))

			continue
		}
		out.WriteRune(c)
	}

	return out.String()
}

// stringNode renders a scalar string.
//
// A string holding line breaks has no one-line form: it comes out as a block
// scalar, and that makes it the one scalar whose rendering spans lines and so
// needs the same relative treatment as a block. The node's own String would lay
// it out from the column it was recorded at.
func (r *Renderer) stringNode(n *StringNode) string {
	header := blockScalarHeader(n)
	if header == "" {
		if r.atMargin && readsAsMarker(n) {
			return quotedMarker(n, r.comments)
		}
		if !r.comments {
			return n.stringWithoutComment()
		}

		return n.plainString()
	}

	// One trailing break belongs to the block structure, not to the content:
	// it is the break that ends the last line. The header sets what happens to
	// the rest -- "|" clips them, "|-" strips them, "|+" keeps them.
	lbc := lineBreakOf(n.Value)
	content := strings.TrimSuffix(n.Value, lbc)

	return header + lbc + indentLinesWith(content, r.indent, lbc)
}

// blockScalarHeader returns the block header a string needs, or "" when the
// string fits on one line or is quoted -- a quoted scalar keeps its quotes.
func blockScalarHeader(n *StringNode) string {
	if n.Token == nil {
		return token.LiteralBlockHeader(n.Value)
	}

	switch n.Token.Type {
	case token.SingleQuoteType, token.DoubleQuoteType:
		return ""
	default:
		return token.LiteralBlockHeader(n.Value)
	}
}

// carriesOwnIndent reports whether a node already indents its own continuation
// lines relative to the start of the line it begins on.
//
// A block scalar does: its header shares a line with whatever introduces it --
// "key:" or "- " -- and its content is indented from that line's start, not
// from where the header happens to sit. Indenting it again would push the
// content one level too deep for every level of nesting.
func carriesOwnIndent(n Node) bool {
	switch node := n.(type) {
	case *LiteralNode:
		return true
	case *StringNode:
		return blockScalarHeader(node) != ""
	case *AnchorNode:
		// A property stays on the line the header ends, so what it names is
		// still indented from the start of that line and not from the property.
		return carriesOwnIndent(node.Value)
	case *TagNode:
		return carriesOwnIndent(node.Value)
	default:
		return false
	}
}

// directive renders a directive and its comment lines, on the side of the
// directive where the document wrote them.
//
// Comments written after the directive go between it and the '---' below: a
// directive is one line, so there is nothing to append them to. Comments written
// above it, by their line number, go above it.
func (r *Renderer) directive(n *DirectiveNode) string {
	if !r.comments || n.Comment.Blank() {
		return n.String()
	}

	comment := r.text(n.Comment)
	if commentTk := n.Comment.GetToken(); commentTk != nil && commentTk.Position.Line != 0 &&
		n.Start != nil && commentTk.Position.Line < n.Start.Position.Line {
		// Written above the directive, not below it.
		return comment + "\n" + n.String()
	}

	return n.String() + "\n" + comment
}

func (r *Renderer) commentGroup(n *CommentGroupNode) string {
	if !r.comments {
		return ""
	}

	return n.String()
}

func (r *Renderer) footComment(c *CommentGroupNode) rendered {
	if !r.comments || c.Blank() {
		return rendered{}
	}

	return join(sepNone, leaf("\n"+blankLineBefore(c)), r.render(c))
}

// blankLineBefore returns the blank line an author left above n, or "".
func blankLineBefore(n Node) string {
	if n == nil {
		return ""
	}
	if tk := n.GetToken(); tk != nil && tk.BlankLineAbove() {
		return "\n"
	}

	return ""
}

/*
// splitLeadingBlank separates a leading blank line from the text it precedes.
func splitLeadingBlank(text string) (string, string) {
	if rest, found := strings.CutPrefix(text, "\n"); found {
		return "\n", rest
	}

	return "", text
}
*/

// flowComments gathers the comment slots a flow collection carries outside its
// entries: the one on the opening bracket's line, the ones written above an
// element, and the one on the line before the closing bracket.
type flowComments struct {
	start *CommentGroupNode
	heads []*CommentGroupNode
	foot  *CommentGroupNode
}

// flowCarriesComments reports whether anything in a flow collection has a
// comment on it. A comment keeps the collection from fitting on one line.
func (r *Renderer) flowCarriesComments(values []Node, comments flowComments) bool {
	if !r.comments {
		return false
	}
	if !comments.start.Blank() || !comments.foot.Blank() {
		return true
	}
	for _, head := range comments.heads {
		if !head.Blank() {
			return true
		}
	}
	for _, value := range values {
		if headCommentOf(value) != nil || lineCommentOf(value) != nil {
			return true
		}
	}

	return false
}

// flowBlock writes a flow collection across lines, one entry to a line.
//
// A flow collection is normally written on one line, but a comment cannot go
// there: everything after it is commented out, including the bracket that
// closes the collection. Several lines is the only layout that holds both, and
// it is still a flow collection.
func (r *Renderer) flowBlock(open, closing string, values []Node, comments flowComments) string {
	lines := make([]string, 0, len(values)+2)
	if !comments.start.Blank() {
		open += " " + r.text(comments.start)
	}
	lines = append(lines, open)

	heads := comments.heads
	bare := r.bare()
	for i, value := range values {
		if i < len(heads) && !heads[i].Blank() {
			lines = append(lines, r.indented(r.text(heads[i])))
		}
		if head := headCommentOf(value); head != nil {
			lines = append(lines, r.indented(r.text(head)))
		}

		// The ',' separates the entries, so it goes before the comment: after
		// it, it would be commented out along with the rest of the line.
		entry := bare.inline(value)
		if i < len(values)-1 {
			entry += ","
		}
		if comment := lineCommentOf(value); comment != nil {
			entry += " " + r.text(comment)
		}
		lines = append(lines, r.indented(entry))
	}
	if !comments.foot.Blank() {
		lines = append(lines, r.indented(r.text(comments.foot)))
	}

	return strings.Join(append(lines, closing), "\n")
}

// headCommentOf returns the comment written above an entry, or nil.
func headCommentOf(n Node) *CommentGroupNode {
	if entry, ok := n.(*MappingValueNode); ok {
		return present(entry.Comment)
	}

	return nil
}

// lineCommentOf returns the comment written at the end of an entry's line, or
// nil. For a mapping entry that is a comment on its value or on its key.
func lineCommentOf(n Node) *CommentGroupNode {
	entry, ok := n.(*MappingValueNode)
	if !ok {
		return present(n.GetComment())
	}
	if entry.Value != nil {
		if comment := present(entry.Value.GetComment()); comment != nil {
			return comment
		}
	}

	return present(entry.Key.GetComment())
}

// present returns c, or nil where it writes nothing. A caller testing a group
// against nil sees one emptied by [CommentNode.Remove] as still there.
func present(c *CommentGroupNode) *CommentGroupNode {
	if c.Blank() {
		return nil
	}

	return c
}

// inline renders a node for a context that cannot hold a line break.
func (r *Renderer) inline(n Node) string {
	return strings.TrimLeft(strings.ReplaceAll(r.text(n), "\n", " "), " ")
}

func (r *Renderer) withComment(text rendered, c *CommentGroupNode) rendered {
	if !r.comments || c.Blank() {
		return text
	}

	return join(sepSpace, text, leaf(c.String()))
}

// indented shifts every line of text one level deeper.
func (r *Renderer) indented(text string) string {
	return indentLines(text, r.indent)
}

func indentLines(text string, spaces int) string {
	return indentLinesWith(text, spaces, "\n")
}

// indentLinesWith indents text whose lines are separated by lbc. A block scalar
// keeps whatever line break its content was written with, which is not always
// the "\n" the rest of the document is laid out in.
func indentLinesWith(text string, spaces int, lbc string) string {
	if spaces <= 0 || text == "" {
		return text
	}

	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(text, lbc)
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = pad + line
	}

	return strings.Join(lines, lbc)
}

// introducedIndent returns how much indentation the source text carries that
// the value does not: the width that introduced the block.
//
// It is measured on the first line that has content, by comparing the two.
// Falling back to the least indented line is right whenever the value cannot
// answer, and wrong only for the case this exists for.
func introducedIndent(origin, value, lbc string) int {
	first, _, _ := strings.Cut(origin, lbc)
	valueFirst, _, _ := strings.Cut(value, lbc)

	if trimmed := strings.TrimLeft(first, " "); trimmed != "" && strings.HasSuffix(trimmed, strings.TrimLeft(valueFirst, " ")) {
		if lead := len(first) - len(valueFirst); lead >= 0 {
			return lead
		}
	}

	return commonIndent(origin, lbc)
}

// commonIndent returns the indentation shared by every line that has content.
func commonIndent(text, lbc string) int {
	common := -1
	for line := range strings.SplitSeq(text, lbc) {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		if lead := len(line) - len(trimmed); common < 0 || lead < common {
			common = lead
		}
	}
	if common < 0 {
		return 0
	}

	return common
}

// dedentBy removes n columns of indentation from every line, leaving the
// relative shape that is part of a block scalar's value.
func dedentBy(text string, n int) string {
	if n <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if len(line) >= n {
			lines[i] = line[n:]

			continue
		}
		lines[i] = strings.TrimLeft(line, " ")
	}

	return strings.Join(lines, "\n")
}
