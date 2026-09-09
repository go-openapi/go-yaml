// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"
	"strings"

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
// an alias stands for rather than a reference it has no way to look up.
func WithAliasTargets(anchors map[string]Node) RenderOption {
	return func(r *Renderer) {
		r.aliasTargets = anchors
		r.resolving = make(map[string]bool, len(anchors))
	}
}

// Renderer turns an AST back into YAML text.
//
// It exists as a type rather than as a String method so that callers can
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
	// was passed, and a Renderer carrying them is built per call rather than
	// shared: bare() copies the struct, so the two maps are the same maps in
	// the copy, which is what lets the recursion below see its own progress.
	aliasTargets map[string]Node
	resolving    map[string]bool
	// src is the document a tree was parsed from, for [Renderer.Verbatim]. It
	// is nil unless [WithSource] was passed.
	src []byte
	// transform is handed each stretch of a verbatim rendering, and is nil
	// unless [WithTransform] was passed.
	transform TransformFunc
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

// Render writes n to w.
func (r *Renderer) Render(w io.Writer, n Node) error {
	lw := lineWriter{w: w}
	r.render(n).writeTo(&lw, 0)

	return lw.err
}

// String renders n and returns the text.
//
// The result is relative to nothing: the node's own first line carries no
// indentation, and everything nested under it is indented from there. A caller
// placing the result somewhere indented adds that indentation itself.
func (r *Renderer) String(n Node) string {
	return r.render(n).string()
}

// render builds what n writes, without writing it.
//
// It is String's dispatch, returning the pieces rather than the text so that a
// parent can indent a child by a number and ask whether it spans lines without
// building it twice. A node type still composing strings arrives here as one
// leaf, which is correct and costs what it always did.
func (r *Renderer) render(n Node) rendered {
	if n == nil {
		return rendered{}
	}

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

// File renders a whole file. It is separate from String because *File is not a
// Node -- it holds documents rather than being one.
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

func (r *Renderer) document(n *DocumentNode) rendered {
	parts := make([]rendered, 0, 3)
	if n.Start != nil {
		parts = append(parts, leaf(n.Start.Value))
	}
	if n.Body != nil {
		parts = append(parts, r.documentBody(n.Body))
	}
	if n.End != nil {
		parts = append(parts, leaf(n.End.Value))
	}

	return join(sepBreak, parts...)
}

func (r *Renderer) mapping(n *MappingNode) rendered {
	if len(n.Values) == 0 {
		return r.withComment(leaf("{}"), n.Comment)
	}
	if n.IsFlowStyle {
		values := make([]Node, 0, len(n.Values))
		for _, value := range n.Values {
			values = append(values, value)
		}
		if r.flowCarriesComments(values, nil, n.FootComment) {
			return r.withComment(leaf(r.flowBlock("{", "}", values, nil, n.FootComment)), n.Comment)
		}

		entries := make([]string, 0, len(n.Values))
		for _, value := range n.Values {
			entries = append(entries, r.inline(value))
		}

		return r.withComment(leaf("{"+strings.Join(entries, ", ")+"}"), n.Comment)
	}

	lines := make([]rendered, 0, len(n.Values)+1)
	if r.comments && n.Comment != nil {
		lines = append(lines, r.render(n.Comment))
	}
	for _, value := range n.Values {
		lines = append(lines, r.render(value))
	}
	if r.comments && n.FootComment != nil {
		lines = append(lines, r.render(n.FootComment))
	}

	return join(sepBreak, lines...)
}

func (r *Renderer) mappingValue(n *MappingValueNode) rendered {
	key := leaf(r.bare().inline(n.Key))

	// A blank line before an entry is the author's, not the layout's: it groups
	// entries, and no amount of re-rendering should lose it. Unlike a column, it
	// does not compound when a document is read and written repeatedly.
	var head rendered
	if r.comments && n.Comment != nil {
		// The gap is above the comment, which is what now leads the entry.
		head = join(sepNone, leaf(blankLineBefore(n.Comment)), r.render(n.Comment), leaf("\n"))
	} else {
		head = leaf(blankLineBefore(n.Key))
	}

	if _, explicit := n.Key.(*MappingKeyNode); explicit {
		// The ':' goes on its own line. Written inline as "? a: b", YAML reads
		// the whole of "a: b" as the key.
		body := join(sepNone, r.render(n.Key), leaf("\n:"))
		if value := r.value(n.Value, false); !value.empty() {
			body = join(sepNone, body, value)
		}

		return join(sepNone, head, body, r.footComment(n.FootComment))
	}

	// A comment on the key belongs after the ':', not before it: written where
	// the key sits, it would be read back as part of the key.
	comment := r.keyComment(n.Key)
	value := r.value(n.Value, comment != "")
	if comment == "" {
		// A comment written on the key's line, above a block, is recorded on the
		// block rather than on the key. It goes back where it was written.
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

	return join(sepNone, head, key, leaf(r.colonAfter(n.Key)), inline, value, trailing,
		r.footComment(n.FootComment))
}

// colonAfter returns the ':' that closes a key, with the separating space the
// key needs in front of it.
//
// An anchor name, an alias name and a tag shorthand may all contain ':', so a
// key that ends on one absorbs the ':' written straight after it: "&a: v"
// anchors "a:" over the scalar v, where "&a : v" anchors the empty key of a
// mapping. The space is what tells them apart.
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
	return r.bare().String(n) == ""
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
		if node.IsFlowStyle || len(node.Values) == 0 {
			return "", value
		}
		comment = node.Comment
	case *SequenceNode:
		if node.IsFlowStyle || len(node.Values) == 0 {
			return "", value
		}
		comment = node.Comment
	}
	if comment == nil || !sameLine(comment, key) {
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

	return r.String(comment), leaf("\n" + rest)
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

	return r.String(comment)
}

// value renders what follows a "key:", including the space or newline that
// separates it. It returns "" for an absent value.
//
// keyCommented says the key carries a comment, which claims the rest of the
// line: a collection that would otherwise sit beside its key goes below it so
// that the comment stays next to the key it belongs to.
// alias renders n as the node its anchor names.
//
// An anchor whose node holds the alias would render forever, so a name already
// being rendered falls back to the reference: "&a1 [*a1]" renders the inner
// alias as "*a1" rather than exhausting the stack.
func (r *Renderer) alias(n *AliasNode) string {
	name := n.Value.GetToken().Value
	target := r.aliasTarget(n, name)
	if target == nil || r.resolving[name] {
		return n.String()
	}

	r.resolving[name] = true
	defer delete(r.resolving, name)

	return r.String(target)
}

// deref returns the node an alias names, for the layout questions below that
// ask what shape a value has -- whether it fits on the key's line, whether it
// is a block sequence, whether it carries its own indentation. An alias renders
// as its target, so the target is what those questions are about.
//
// It resolves one step and does not guard against a cycle: rendering goes
// through [Renderer.alias], which does, and one step answers the shape.
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
	// whatever kind of node stands at the top of them. fitsOnKeyLine says yes to
	// an anchor and a tag, since they carry their own value and decide their own
	// shape -- but the comment claims the rest of the key's line, so what
	// follows cannot start there. Kept on it, the comment was pushed past the
	// whole value and came back on the last line of it: "k: # c1" over "&a2"
	// over "- 1" rendered as "k: &a2" over "- 1 # c1", and where the last entry
	// was a block scalar the comment landed inside its content and changed the
	// value.
	if r.fitsOnKeyLine(shape) && (!collection || !spansLines) &&
		(!keyCommented || (!collection && !spansLines)) {
		return join(sepNone, leaf(" "), text)
	}
	if sequence, ok := shape.(*SequenceNode); ok && !sequence.IsFlowStyle && !r.indentSequence {
		// A block sequence under a mapping key sits at the key's own
		// indentation unless asked otherwise: "key:" then "- item" in column
		// one of the key's level. Both layouts are legal; this is the one YAML
		// is usually written in. A flow sequence is not laid out this way: it
		// is a value like any other and indents under its key.
		return join(sepNone, leaf("\n"), text)
	}

	return join(sepNone, leaf("\n"), text.indentedBy(r.indent))
}

// fitsOnKeyLine reports whether a value belongs after its key on the same line.
// Everything that renders as a block of its own goes below it instead.
func (r *Renderer) fitsOnKeyLine(n Node) bool {
	switch node := n.(type) {
	case *MappingNode:
		return node.IsFlowStyle || len(node.Values) == 0
	case *SequenceNode:
		return node.IsFlowStyle || len(node.Values) == 0
	case *AnchorNode, *TagNode, *LiteralNode:
		// These carry their own value, which may itself be a block; they decide
		// their own shape, and start on the key's line either way.
		return true
	default:
		return true
	}
}

func (r *Renderer) mappingKey(n *MappingKeyNode) rendered {
	value := r.entry(n.Value)
	if value.empty() {
		return leaf(n.Start.Value)
	}
	if value.leads {
		// The key opens below its "?" with a blank line between, and a blank
		// line is two breaks: one ends the "?"'s own line and one is the gap.
		// Written with the single break the entry carries, the gap was lost and
		// the next rendering pulled the key back up onto the "?" line, so the
		// document moved on every pass.
		return join(sepNone, leaf(n.Start.Value+" "), leaf("\n"), value)
	}

	return join(sepNone, leaf(n.Start.Value+" "), value)
}

// entry renders a node placed after a marker that occupies the start of its
// line -- "- " or "? " -- indenting its continuation lines to sit under it.
func (r *Renderer) entry(n Node) rendered {
	piece := r.render(n)
	if carriesOwnIndent(r.deref(n)) {
		return piece
	}

	if piece.leads {
		// A blank line ends the marker's line, so what follows no longer stands
		// after the marker: it opens a line of its own and takes the
		// indentation every other line under the marker takes. Written as a
		// hanging indent it landed in column one, outside the key it belongs
		// to -- "?" over a blank line over a mapping came back as two entries
		// of the document rather than one key, and the rendered text no longer
		// read as the document that was parsed.
		return join(sepNone, leaf("\n"), piece.withoutLead().indentedBy(r.indent))
	}

	// The marker already holds the first line, so the piece is written where it
	// left off and only the lines under it take the indentation. That is what
	// hangingIndent did by cutting the first line off; the writer does it by
	// putting the padding in at each line break instead.
	return piece.hangingBy(r.indent)
}

func (r *Renderer) sequence(n *SequenceNode) rendered {
	if len(n.Values) == 0 {
		return r.withComment(leaf("[]"), n.Comment)
	}
	if n.IsFlowStyle {
		if r.flowCarriesComments(n.Values, n.ValueHeadComments, n.FootComment) {
			return r.withComment(
				leaf(r.flowBlock("[", "]", n.Values, n.ValueHeadComments, n.FootComment)), n.Comment)
		}

		entries := make([]string, 0, len(n.Values))
		for _, value := range n.Values {
			entries = append(entries, r.inline(value))
		}

		return r.withComment(leaf("["+strings.Join(entries, ", ")+"]"), n.Comment)
	}

	lines := make([]rendered, 0, len(n.Values)+1)
	if r.comments && n.Comment != nil {
		lines = append(lines, r.render(n.Comment))
	}
	for i, value := range n.Values {
		// A blank line inside an entry surfaces as a leading break on the
		// entry's own text. It belongs above the "- ", not after it.
		entry := r.entry(value)
		var blank string
		if entry.leads {
			blank, entry = "\n", entry.withoutLead()
		}
		if r.comments && i < len(n.ValueHeadComments) && n.ValueHeadComments[i] != nil {
			comment := n.ValueHeadComments[i]
			if blank == "" {
				// The entry's own token follows the comment, so the gap the
				// author left shows up above the comment instead.
				blank = blankLineBefore(comment)
			}
			lines = append(lines, join(sepNone, leaf(blank), r.render(comment)))
			blank = ""
		} else if blank == "" {
			// Only a block collection reports a gap of its own. For anything
			// else the sequence reads it off the entry's first token.
			blank = blankLineBefore(value)
		}
		comment := r.entryLineComment(n, i)
		if comment != "" && !carriesOwnIndent(value) &&
			(!r.fitsOnKeyLine(value) || entry.spans) {
			// Everything after the '#' is commented out, so a value that would
			// share the dash's line goes below it instead. A block scalar is
			// exempt: its header is all that shares the line, and a comment
			// after the header is where YAML puts one.
			//
			// fitsOnKeyLine says yes to an anchor and a tag, which carry their
			// own value and decide their own shape, so a property standing on a
			// block kept the dash's line and the comment was written after the
			// whole entry -- on the last line of it. "- # c" over "&a2" over a
			// mapping holding a folded scalar came back with the comment inside
			// the scalar's content. The same shape under a mapping key is
			// Renderer.value's to place.
			lines = append(lines,
				leaf(blank+"-"+comment),
				r.render(value).indentedBy(r.indent))

			continue
		}
		lines = append(lines, join(sepNone, leaf(blank+"- "), entry, leaf(comment)))
	}
	if r.comments && n.FootComment != nil {
		lines = append(lines, r.render(n.FootComment))
	}

	return join(sepBreak, lines...)
}

// entryLineComment returns the comment written on the entry's own line.
//
// It is recorded on the entry rather than on its value, because an entry whose
// value is written below it -- or is not written at all -- has nothing on that
// line to carry it. Reading the entries is the only way to find it, and not
// reading them is how such a comment used to be dropped.
func (r *Renderer) entryLineComment(n *SequenceNode, i int) string {
	if !r.comments || i >= len(n.Entries) || n.Entries[i] == nil {
		return ""
	}

	comment := n.Entries[i].LineComment
	if comment == nil {
		return ""
	}

	return " " + r.String(comment)
}

func (r *Renderer) anchor(n *AnchorNode) string {
	return r.withOwnComment(n.Comment, r.prefixed("&"+r.String(n.Name), n.Value))
}

func (r *Renderer) tag(n *TagNode) string {
	if n.Implicit {
		// The parse resolved a type the document left implicit, so the document
		// holds no tag to write back. See TagNode.Implicit.
		return r.withOwnComment(n.Comment, r.String(n.Value))
	}

	return r.withOwnComment(n.Comment, r.prefixed(n.Start.Value, n.Value))
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
	if !r.comments || comment == nil {
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
func (r *Renderer) prefixed(marker string, value Node) string {
	return r.prefixedAt(marker, value, false)
}

func (r *Renderer) prefixedAt(marker string, value Node, atDocumentRoot bool) string {
	if value == nil {
		return marker
	}

	text := r.String(value)
	if atDocumentRoot {
		// A marker does not enclose what it names: "&a |2" is still the
		// document's own node, and the width its header states is counted from
		// the same place.
		text = r.documentBody(value).string()
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
		return !node.IsFlowStyle && len(node.Values) > 0
	case *SequenceNode:
		return !node.IsFlowStyle && len(node.Values) > 0
	default:
		return false
	}
}

// documentBody renders what a document holds.
//
// It differs from String in one respect, and only for a block scalar that
// states its own indentation. That width is counted from the indentation of
// whatever encloses the scalar, and a document encloses nothing: the spec gives
// its node an indentation of -1, so content written one column in is stated as
// two. Everywhere else the enclosing level is the start of the header's own
// line and the two numbers agree.
//
// A property may stand between the document and the scalar -- "&a |2" is a
// document whose node is an anchored block scalar -- so those are unwrapped
// rather than handed to String.
func (r *Renderer) documentBody(n Node) rendered {
	switch node := n.(type) {
	case *LiteralNode:
		return leaf(r.literalAt(node, true))
	case *AnchorNode:
		return leaf(r.withOwnComment(node.Comment, r.prefixedAt("&"+r.String(node.Name), node.Value, true)))
	case *TagNode:
		return leaf(r.withOwnComment(node.Comment, r.prefixedAt(node.Start.Value, node.Value, true)))
	default:
		return r.render(n)
	}
}

func (r *Renderer) literal(n *LiteralNode) string {
	return r.literalAt(n, false)
}

func (r *Renderer) literalAt(n *LiteralNode, atDocumentRoot bool) string {
	header := n.Start.Value

	// The content is written at the renderer's own width, so a header that
	// states a width has to say that one -- carrying the source's over left it
	// describing a layout that is no longer there, and the value gained a
	// column on every cycle.
	if statedIndent(header) > 0 {
		stated := r.indent
		if atDocumentRoot {
			stated++
		}
		header = restateIndent(header, stated)
	}

	if r.comments && n.Comment != nil {
		header += " " + r.String(n.Comment)
	}

	value := n.Value.Value
	if value == "" {
		// An empty block scalar is its header. Writing the line break that
		// would introduce content leaves a blank line the parser reads as
		// content indented differently from what the header announced.
		//
		// '+' comes off: it keeps every trailing break, and the break that ends
		// the header line is one. A document written with a final line break --
		// every document is -- would read "|+" back as "\n" rather than as the
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

	// A literal scalar's value is its content exactly, and the header says how
	// to write it: each line indented and closed by a break, with the last
	// break left to whatever follows the node. The chomping indicator needs no
	// arithmetic here -- it is what decided how many trailing breaks the value
	// has, and writing them all back is what makes "|+" keep the blank lines it
	// exists for.
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
// The blank lines the source ends on are not read back from it. They are what
// the chomping indicator decides, and the value is where that decision has
// already been made: the source ends the same way whether the header says ">",
// ">-" or ">+", so writing its tail back would keep blank lines for the two
// styles that discard them.
func (r *Renderer) foldedFromSource(n *LiteralNode, indent int) string {
	value := n.Value.Value
	origin := n.Source

	// srcBreak is how the source wrote a line break, and is what the origin has
	// to be read with. It is not what is written back: the renderer writes "\n"
	// whatever the document used, as it does everywhere else. Copying the
	// source's break here left a CRLF document rendering to a mixture of both,
	// so it never settled; with a lone CR the content lines drifted a column
	// right, a fold stopped folding, and in a nested position the result was
	// not a block scalar at all.
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
		// rather than standing for a blank line of its own.
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
// token.DetectLineBreakCharacter answers "\r\n" for that case, which is right
// for deciding what a file uses and wrong for deciding what to write here.
func lineBreakOf(value string) string {
	if !strings.ContainsAny(value, "\r\n") {
		return "\n"
	}

	return token.DetectLineBreakCharacter(value)
}

// statedIndent returns the indentation a block scalar header asks for, or 0
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
		if !r.comments {
			return n.stringWithoutComment()
		}

		return n.String()
	}

	// One trailing break belongs to the block structure rather than to the
	// content: it is the break that ends the last line. The header says what to
	// do with the rest -- "|" clips them, "|-" strips them, "|+" keeps them.
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

// directive renders a directive and the comment lines that followed it.
//
// Those sit between the directive and the '---' below, which is where they were
// written and the only place they can go: a directive is one line, so there is
// nothing to append them to.
func (r *Renderer) directive(n *DirectiveNode) string {
	if !r.comments || n.Comment == nil {
		return n.String()
	}

	comment := r.String(n.Comment)
	if commentTk := n.Comment.GetToken(); commentTk != nil && commentTk.Position.Line != 0 &&
		n.Start != nil && commentTk.Position.Line < n.Start.Position.Line {
		// Written above the directive rather than below it.
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
	if !r.comments || c == nil {
		return rendered{}
	}

	return join(sepNone, leaf("\n"), r.render(c))
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

// splitLeadingBlank separates a leading blank line from the text it precedes.
func splitLeadingBlank(text string) (string, string) {
	if rest, found := strings.CutPrefix(text, "\n"); found {
		return "\n", rest
	}

	return "", text
}

// flowCarriesComments reports whether anything in a flow collection has a
// comment on it, which is what stops it fitting on one line.
func (r *Renderer) flowCarriesComments(values []Node, heads []*CommentGroupNode, foot *CommentGroupNode) bool {
	if !r.comments {
		return false
	}
	if foot != nil {
		return true
	}
	for _, head := range heads {
		if head != nil {
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
func (r *Renderer) flowBlock(open, closing string, values []Node, heads []*CommentGroupNode, foot *CommentGroupNode) string {
	lines := make([]string, 0, len(values)+2)
	lines = append(lines, open)

	bare := r.bare()
	for i, value := range values {
		if i < len(heads) && heads[i] != nil {
			lines = append(lines, r.indented(r.String(heads[i])))
		}
		if head := headCommentOf(value); head != nil {
			lines = append(lines, r.indented(r.String(head)))
		}

		// The ',' separates the entries, so it goes before the comment: after
		// it, it would be commented out along with the rest of the line.
		entry := bare.inline(value)
		if i < len(values)-1 {
			entry += ","
		}
		if comment := lineCommentOf(value); comment != nil {
			entry += " " + r.String(comment)
		}
		lines = append(lines, r.indented(entry))
	}
	if foot != nil {
		lines = append(lines, r.indented(r.String(foot)))
	}

	return strings.Join(append(lines, closing), "\n")
}

// headCommentOf returns the comment written above an entry, or nil.
func headCommentOf(n Node) *CommentGroupNode {
	if entry, ok := n.(*MappingValueNode); ok {
		return entry.Comment
	}

	return nil
}

// lineCommentOf returns the comment written at the end of an entry's line, or
// nil. For a mapping entry that is a comment on its value or on its key.
func lineCommentOf(n Node) *CommentGroupNode {
	entry, ok := n.(*MappingValueNode)
	if !ok {
		return n.GetComment()
	}
	if entry.Value != nil {
		if comment := entry.Value.GetComment(); comment != nil {
			return comment
		}
	}

	return entry.Key.GetComment()
}

// inline renders a node for a context that cannot hold a line break.
func (r *Renderer) inline(n Node) string {
	return strings.TrimLeft(strings.ReplaceAll(r.String(n), "\n", " "), " ")
}

func (r *Renderer) withComment(text rendered, c *CommentGroupNode) rendered {
	if !r.comments || c == nil {
		return text
	}

	return join(sepSpace, text, leaf(c.String()))
}

// indented shifts every line of text one level deeper.
func (r *Renderer) indented(text string) string {
	return indentLines(text, r.indent)
}

// hangingIndent shifts every line but the first, for text placed after a marker
// that already occupies the first line -- "- " or "? ".
func (r *Renderer) hangingIndent(text string) string {
	first, rest, found := strings.Cut(text, "\n")
	if !found {
		return first
	}

	return first + "\n" + indentLines(rest, r.indent)
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
	for _, line := range strings.Split(text, lbc) {
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
