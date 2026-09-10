// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

var (
	ErrInvalidTokenType  = errors.New("invalid token type")
	ErrInvalidAnchorName = errors.New("invalid anchor name")
	ErrInvalidAliasName  = errors.New("invalid alias name")
)

// NodeType type identifier of node
type NodeType int

const (
	// UnknownNodeType type identifier for default
	UnknownNodeType NodeType = iota
	// DocumentType type identifier for document node
	DocumentType
	// NullType type identifier for null node
	NullType
	// BoolType type identifier for boolean node
	BoolType
	// IntegerType type identifier for integer node
	IntegerType
	// FloatType type identifier for float node
	FloatType
	// InfinityType type identifier for infinity node
	InfinityType
	// NanType type identifier for nan node
	NanType
	// StringType type identifier for string node
	StringType
	// MergeKeyType type identifier for merge key node
	MergeKeyType
	// LiteralType type identifier for literal node
	LiteralType
	// MappingType type identifier for mapping node
	MappingType
	// MappingKeyType type identifier for mapping key node
	MappingKeyType
	// MappingValueType type identifier for mapping value node
	MappingValueType
	// SequenceType type identifier for sequence node
	SequenceType
	// SequenceEntryType type identifier for sequence entry node
	SequenceEntryType
	// AnchorType type identifier for anchor node
	AnchorType
	// AliasType type identifier for alias node
	AliasType
	// DirectiveType type identifier for directive node
	DirectiveType
	// TagType type identifier for tag node
	TagType
	// CommentType type identifier for comment node
	CommentType
	// CommentGroupType type identifier for comment group node
	CommentGroupType
)

// String node type identifier to text
func (t NodeType) String() string {
	switch t {
	case UnknownNodeType:
		return "UnknownNode"
	case DocumentType:
		return "Document"
	case NullType:
		return "Null"
	case BoolType:
		return "Bool"
	case IntegerType:
		return "Integer"
	case FloatType:
		return "Float"
	case InfinityType:
		return "Infinity"
	case NanType:
		return "Nan"
	case StringType:
		return "String"
	case MergeKeyType:
		return "MergeKey"
	case LiteralType:
		return "Literal"
	case MappingType:
		return "Mapping"
	case MappingKeyType:
		return "MappingKey"
	case MappingValueType:
		return "MappingValue"
	case SequenceType:
		return "Sequence"
	case SequenceEntryType:
		return "SequenceEntry"
	case AnchorType:
		return "Anchor"
	case AliasType:
		return "Alias"
	case DirectiveType:
		return "Directive"
	case TagType:
		return "Tag"
	case CommentType:
		return "Comment"
	case CommentGroupType:
		return "CommentGroup"
	}
	return ""
}

// String node type identifier to YAML Structure name
// based on https://yaml.org/spec/1.2/spec.html
func (t NodeType) YAMLName() string {
	switch t {
	case UnknownNodeType:
		return "unknown"
	case DocumentType:
		return "document"
	case NullType:
		return "null"
	case BoolType:
		return "boolean"
	case IntegerType:
		return "int"
	case FloatType:
		return "float"
	case InfinityType:
		return "inf"
	case NanType:
		return "nan"
	case StringType:
		return "string"
	case MergeKeyType:
		return "merge key"
	case LiteralType:
		return "scalar"
	case MappingType:
		return "mapping"
	case MappingKeyType:
		return "key"
	case MappingValueType:
		return "value"
	case SequenceType:
		return "sequence"
	case SequenceEntryType:
		return "value"
	case AnchorType:
		return "anchor"
	case AliasType:
		return "alias"
	case DirectiveType:
		return "directive"
	case TagType:
		return "tag"
	case CommentType:
		return "comment"
	case CommentGroupType:
		return "comment"
	}
	return ""
}

// Node type of node
type Node interface {
	// String node to text
	String() string
	// GetToken returns token instance
	GetToken() *token.Token
	// Type returns type of node
	Type() NodeType
	// AddColumn add column number to child nodes recursively
	AddColumn(int)
	// SetComment set comment token to node
	SetComment(*CommentGroupNode) error
	// Comment returns comment token instance
	GetComment() *CommentGroupNode
	// GetPath returns YAMLPath for the current node
	GetPath() string
	// SetPath set YAMLPath for the current node
	SetPath(string)
	// GetPathNode returns the step of the path trie this node ends
	GetPathNode() *PathNode
	// SetPathNode records the step of the path trie this node ends
	SetPathNode(*PathNode)
	// MarshalYAML
	MarshalYAML() ([]byte, error)
}

// MapKeyNode type for map key node
type MapKeyNode interface {
	Node
	IsMergeKey() bool
	// String node to text without comment
	stringWithoutComment() string
}

// ScalarNode type for scalar node
type ScalarNode interface {
	MapKeyNode
	GetValue() interface{}
	// Text returns the scalar as text, unconverted: the digits of a number as
	// the document wrote them, a quoted string with its quotes and escapes
	// resolved, a block scalar folded and chomped. Deciding what a number
	// means is the caller's, and Text is where it starts.
	Text() string
	// Bytes returns Text as bytes, without copying it. See [token.TextBytes]
	// for what a caller may do with them.
	Bytes() []byte
}

type BaseNode struct {
	path    *PathNode
	Comment *CommentGroupNode
	// HeadComment is what stands above the node, on lines of its own, and it
	// may hold several: a run of comment lines above one node is one group.
	//
	// Comment is the node's other one and does not mean the same thing for
	// every node. On a MappingNode or a SequenceNode it is the head comment and
	// the renderer writes it above; on an AnchorNode or a TagNode it is the
	// comment written beside the property, which Renderer.withOwnComment puts
	// back at the end of that line; on a bare scalar it is whatever was
	// attached last. So a node outside a mapping or sequence entry had one
	// field for two comments and the second write took the first: "# c1" over
	// "831 # c2" rendered "831 # c1" with the property comment gone, and
	// "# c1" over "&a q # c2" rendered both onto one line, which the scanner
	// reads back as a single comment.
	//
	// This means one thing everywhere, and [Renderer] writes it above whatever
	// the node is.
	HeadComment *CommentGroupNode
}

// GetHeadComment returns what stands above the node. See [BaseNode.HeadComment].
func (n *BaseNode) GetHeadComment() *CommentGroupNode {
	if n == nil {
		return nil
	}

	return n.HeadComment
}

// SetHeadComment records what stands above the node.
func (n *BaseNode) SetHeadComment(node *CommentGroupNode) error {
	if n == nil {
		return nil
	}
	n.HeadComment = node

	return nil
}

func addCommentString(base string, node *CommentGroupNode) string {
	if node.Blank() {
		// Every String method tests its comment against nil before calling
		// here, and a group emptied by [CommentNode.Remove] is still there:
		// without this, "a: 1 # c" with the comment removed came back "a: 1 "
		// with the space that stood in front of it.
		return base
	}

	return fmt.Sprintf("%s %s", base, node.String())
}

// GetPath returns YAMLPath for the current node.
func (n *BaseNode) GetPath() string {
	if n == nil {
		return ""
	}
	return n.path.String()
}

// SetPath set YAMLPath for the current node.
func (n *BaseNode) SetPath(path string) {
	if n == nil {
		return
	}
	p := &PathNode{}
	p.Literal(path)
	n.path = p
}

// GetPathNode returns the step of the path trie this node ends.
func (n *BaseNode) GetPathNode() *PathNode {
	if n == nil {
		return nil
	}
	return n.path
}

// SetPathNode records the step of the path trie this node ends.
func (n *BaseNode) SetPathNode(p *PathNode) {
	if n == nil {
		return
	}
	n.path = p
}

// GetComment returns comment token instance
func (n *BaseNode) GetComment() *CommentGroupNode {
	return n.Comment
}

// SetComment set comment token
func (n *BaseNode) SetComment(node *CommentGroupNode) error {
	if err := refuseOverwritingWhatWasWritten(n.Comment, node); err != nil {
		return err
	}
	n.Comment = node

	return nil
}

// refuseOverwritingWhatWasWritten rejects putting a comment of a caller's own,
// or nothing at all, over one the document wrote.
//
// The comment being dropped is the only record of which bytes of the source it
// stands on, and [Renderer.VerbatimFile] needs them to take the old text out of
// the copy. Assigned over, the tree says one thing and the document another:
// "a: 1 # old" came back as "a: 1 # old" whatever was put in its place, and a
// comment removed this way came back as well.
//
// Use [CommentNode.Replace] and [CommentNode.Remove], which keep the token and
// say what happens to it. A parse building a tree reaches for [TakeComment],
// which is not an edit: the comment is being put on the node it belongs to and
// the document's own text is not changing.
func refuseOverwritingWhatWasWritten(old, node *CommentGroupNode) error {
	if !fromSourceGroup(old) || fromSourceGroup(node) {
		return nil
	}

	return fmt.Errorf(
		"%q was written by the document: edit it with CommentNode.Replace or CommentNode.Remove rather than assigning over it",
		old.String(),
	)
}

// fromSourceGroup reports whether every comment in g was cut from a document.
func fromSourceGroup(g *CommentGroupNode) bool {
	if g == nil || len(g.Comments) == 0 {
		return false
	}
	for _, comment := range g.Comments {
		if comment.Token == nil || !comment.Token.FromSource() {
			return false
		}
	}

	return true
}

// TakeComment hands over the comment standing on n and clears the slot, without
// recording that the document no longer holds it.
//
// It is for a parse building a tree, where a comment read from the document is
// being put on the node it belongs to. A caller editing a parsed tree wants
// [CommentNode.Replace] or [CommentNode.Remove] instead: those keep the token
// that says which bytes of the source the comment stands on, and this drops it.
func TakeComment(n Node) *CommentGroupNode {
	if n == nil {
		return nil
	}
	comment := n.GetComment()
	if carrier, ok := n.(interface{ clearComment() }); ok {
		carrier.clearComment()
	}

	return comment
}

func (n *BaseNode) clearComment() { n.Comment = nil }

func (n *SequenceEntryNode) clearComment() { n.LineComment = nil }

// Null create node for null value
func Null(tk *token.Token) *NullNode {
	return &NullNode{
		Token: tk,
	}
}

// Bool create node for boolean value
func Bool(tk *token.Token) *BoolNode {
	b, _ := token.ParseBool(tk.Value)

	return &BoolNode{
		Token: tk,
		Value: b,
	}
}

// Integer create node for integer value
func Integer(tk *token.Token) *IntegerNode {
	return &IntegerNode{
		Token: tk,
	}
}

// Float create node for float value
func Float(tk *token.Token) *FloatNode {
	return &FloatNode{
		Token: tk,
	}
}

// Infinity create node for .inf or -.inf value.
//
// The 1.2 core schema's float production is `[-+]? ( \.inf | \.Inf | \.INF )`,
// so a "+" spells the same value the bare form does. Reading the sign rather
// than matching all nine spellings keeps this in step with the scanner's
// reservedInfKeywords, which is where they are listed.
func Infinity(tk *token.Token) *InfinityNode {
	node := &InfinityNode{
		Token: tk,
	}
	if tk.Type != token.InfinityType {
		return node
	}
	if strings.HasPrefix(tk.Value, "-") {
		node.Value = math.Inf(-1)

		return node
	}
	node.Value = math.Inf(0)

	return node
}

// Nan create node for .nan value
func Nan(tk *token.Token) *NanNode {
	return &NanNode{
		Token: tk,
	}
}

// String create node for string value
func String(tk *token.Token) *StringNode {
	return &StringNode{
		Token: tk,
		Value: tk.Value,
	}
}

// Comment create node for comment
func Comment(tk *token.Token) *CommentNode {
	return &CommentNode{
		Token: tk,
	}
}

func CommentGroup(comments []*token.Token) *CommentGroupNode {
	nodes := []*CommentNode{}
	for _, comment := range comments {
		nodes = append(nodes, Comment(comment))
	}
	return &CommentGroupNode{
		Comments: nodes,
	}
}

// MergeKey create node for merge key ( << )
func MergeKey(tk *token.Token) *MergeKeyNode {
	return &MergeKeyNode{
		Token: tk,
	}
}

// Mapping create node for map
func Mapping(tk *token.Token, isFlowStyle bool, values ...*MappingValueNode) *MappingNode {
	node := &MappingNode{
		Start:       tk,
		IsFlowStyle: isFlowStyle,
		Values:      []*MappingValueNode{},
	}
	node.Values = append(node.Values, values...)
	return node
}

// MappingValue create node for mapping value
func MappingValue(tk *token.Token, key MapKeyNode, value Node) *MappingValueNode {
	return &MappingValueNode{
		Start: tk,
		Key:   key,
		Value: value,
	}
}

// MappingKey create node for map key ( '?' ).
func MappingKey(tk *token.Token) *MappingKeyNode {
	return &MappingKeyNode{
		Start: tk,
	}
}

// Sequence create node for sequence
func Sequence(tk *token.Token, isFlowStyle bool) *SequenceNode {
	return &SequenceNode{
		Start:       tk,
		IsFlowStyle: isFlowStyle,
		Values:      []Node{},
	}
}

func Anchor(tk *token.Token) *AnchorNode {
	return &AnchorNode{
		Start: tk,
	}
}

func Alias(tk *token.Token) *AliasNode {
	return &AliasNode{
		Start: tk,
	}
}

func Document(tk *token.Token, body Node) *DocumentNode {
	return &DocumentNode{
		Start: tk,
		Body:  body,
	}
}

func Directive(tk *token.Token) *DirectiveNode {
	return &DirectiveNode{
		Start: tk,
	}
}

func Literal(tk *token.Token) *LiteralNode {
	return &LiteralNode{
		Start: tk,
	}
}

func Tag(tk *token.Token) *TagNode {
	return &TagNode{
		Start: tk,
	}
}

// File contains all documents in YAML file
type File struct {
	Name string
	Docs []*DocumentNode
	// text and read hold what Read is handing out. They are the file's own
	// rather than each node's: a cursor on every node cost 8 bytes of every
	// node in the document to serve a reader nothing asks a node for.
	text []byte
	read int
}

// Read renders the file and reads the text out, so that a parsed document may
// be handed to anything taking an [io.Reader].
//
// The text is taken once, at the first Read, and the documents are not looked
// at again until it runs out: a node changed while a read is running does not
// change what is left to read. Reading to [io.EOF] starts the next read from
// the top of the file as it stands then.
func (f *File) Read(p []byte) (int, error) {
	if f.text == nil {
		f.text = []byte(f.String())
	}
	if f.read >= len(f.text) {
		f.text, f.read = nil, 0

		return 0, io.EOF
	}

	n := copy(p, f.text[f.read:])
	f.read += n

	return n, nil
}

// String all documents to text
func (f *File) String() string {
	return defaultRenderer.File(f)
}

// DocumentNode type of Document
type DocumentNode struct {
	BaseNode
	Start *token.Token // position of DocumentHeader ( `---` )
	End   *token.Token // position of DocumentEnd ( `...` )
	Body  Node
	// StartComment is the comment closing the "---" line, and EndComment the
	// one closing the "..." line. Both are the marker's own: a comment written
	// *above* a "---" introduces the document and reaches its body, and a
	// comment on the body's last line is the body's.
	//
	// They are named rather than taken from the inherited BaseNode.Comment
	// because a document may carry both at once -- "--- # a" over a body over
	// "... # b" -- and one slot holds one of them. Nothing claimed either
	// before, so "--- # c1" rendered as "---" and the comment was read and
	// dropped.
	StartComment *CommentGroupNode
	EndComment   *CommentGroupNode
	// Anchors holds the node each anchor of this document names, under the
	// anchor's name. The parser fills it as it reads, so a caller walking the
	// tree as it is built sees the anchors that stand before it; it is nil for
	// a document that declares none.
	//
	// An anchor belongs to the document it was written in, so each document of
	// a stream carries its own. Where a name is declared twice the last
	// declaration is the one kept here -- read [AliasNode.Target] to expand a
	// particular alias, which names the declaration that stood before it.
	Anchors map[string]Node
}

// Type returns DocumentNodeType
func (d *DocumentNode) Type() NodeType { return DocumentType }

// GetToken returns token instance.
//
// It returns nil for a document with no content, such as the one an empty
// source produces: an empty document has no token to point at.
func (d *DocumentNode) GetToken() *token.Token {
	if d.Body == nil {
		return nil
	}
	return d.Body.GetToken()
}

// AddColumn add column number to child nodes recursively
func (d *DocumentNode) AddColumn(col int) {
	if d.Body != nil {
		d.Body.AddColumn(col)
	}
}

// String document to text
func (d *DocumentNode) String() string {
	return defaultRenderer.String(d)
}

// MarshalYAML encodes to a YAML text
func (d *DocumentNode) MarshalYAML() ([]byte, error) {
	return []byte(d.String()), nil
}

// NullNode type of null node
type NullNode struct {
	BaseNode
	Token *token.Token
}

// Type returns NullType
func (n *NullNode) Type() NodeType { return NullType }

// GetToken returns token instance
func (n *NullNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *NullNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue returns nil value
func (n *NullNode) GetValue() interface{} {
	return nil
}

// String returns the null as the document spelled it.
func (n *NullNode) String() string {
	if n.Token.Type == token.ImplicitNullType {
		if n.Comment != nil {
			return n.Comment.String()
		}
		return ""
	}
	if n.Comment != nil {
		return addCommentString(n.stringWithoutComment(), n.Comment)
	}

	return n.stringWithoutComment()
}

func (n *NullNode) stringWithoutComment() string {
	if n.Token.Type == token.ImplicitNullType {
		// A null nobody wrote has no text. Writing "null" for it would put a
		// key where the document had none.
		return ""
	}

	// The spelling the document used, not the canonical one. YAML resolves
	// "Null", "NULL", "null" and "~" to the same node, so writing "null" for
	// all four kept the value and lost the document -- and under a "!!str" the
	// value went too, since the tag names the characters and "Null" is not
	// "null".
	//
	// Every other scalar node already renders from its token: "!!str True"
	// keeps its capital T and "!!str .INF" its capitals.
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *NullNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *NullNode) IsMergeKey() bool {
	return false
}

// IntegerNode type of integer node
type IntegerNode struct {
	BaseNode
	Token *token.Token
}

// Type returns IntegerType
func (n *IntegerNode) Type() NodeType { return IntegerType }

// GetToken returns token instance
func (n *IntegerNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *IntegerNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue reads the integer and returns it as an int64 where the document
// wrote a sign, as a uint64 where it did not, and as a [big.Int] where neither
// holds it. It returns nil where the text
// is not an integer after all.
//
// The parser types the scalar without converting it, so the conversion happens
// here, each time it is asked for. Use [IntegerNode.Text] to read the digits as
// the document wrote them.
func (n *IntegerNode) GetValue() interface{} {
	if n.Token == nil {
		return nil
	}
	if v, ok := token.ParseInteger(n.Token.Value, n.Token.Type); ok {
		return v
	}

	// The document wrote a whole number wider than int64 or uint64. YAML 1.2
	// puts no bound on an integer -- "arbitrary sized finite mathematical
	// integers" -- and the scanner types a scalar by its grammar, so the value
	// is read exactly here and what to do with it is the decoder's.
	if v, ok := token.ParseBigInteger(n.Token.Value, n.Token.Type); ok {
		return v
	}

	return nil
}

// String int64 to text
func (n *IntegerNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

func (n *IntegerNode) stringWithoutComment() string {
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *IntegerNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *IntegerNode) IsMergeKey() bool {
	return false
}

// FloatNode type of float node
type FloatNode struct {
	BaseNode
	Token *token.Token
}

// Type returns FloatType
func (n *FloatNode) Type() NodeType { return FloatType }

// GetToken returns token instance
func (n *FloatNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *FloatNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue reads the float and returns it as a float64, as a [big.Float] where
// the number reaches past what a float64 holds, or 0 where the text is not a
// float after all.
//
// The parser types the scalar without converting it, so the conversion happens
// here, each time it is asked for. Use [FloatNode.Text] to read the number as
// the document wrote it.
func (n *FloatNode) GetValue() interface{} {
	if n.Token == nil {
		return float64(0)
	}
	if v, ok := token.ParseFloat(n.Token.Value, n.Token.Type); ok {
		return v
	}

	// Past what a float64 reaches. See [IntegerNode.GetValue].
	if v, ok := token.ParseBigFloat(n.Token.Value, n.Token.Type); ok {
		return v
	}

	return float64(0)
}

// String float64 to text
func (n *FloatNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

func (n *FloatNode) stringWithoutComment() string {
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *FloatNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *FloatNode) IsMergeKey() bool {
	return false
}

// StringNode type of string node
type StringNode struct {
	BaseNode
	Token *token.Token
	Value string
}

// Type returns StringType
func (n *StringNode) Type() NodeType { return StringType }

// GetToken returns token instance
func (n *StringNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *StringNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue returns string value
func (n *StringNode) GetValue() interface{} {
	return n.Value
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *StringNode) IsMergeKey() bool {
	return false
}

// escapeSingleQuote escapes s to a single quoted scalar.
// https://yaml.org/spec/1.2.2/#732-single-quoted-style
func escapeSingleQuote(s string) string {
	var sb strings.Builder
	growLen := len(s) + // s includes also one ' from the doubled pair
		2 + // opening and closing '
		strings.Count(s, "'") // ' added by ReplaceAll
	sb.Grow(growLen)
	sb.WriteString("'")
	sb.WriteString(strings.ReplaceAll(s, "'", "''"))
	sb.WriteString("'")
	return sb.String()
}

// quotedString writes a quoted scalar so that reading it back gives the same
// value.
//
// A single-quoted scalar has no escapes: what it holds is what it says, and a
// line break written inside one is folded away -- one break becomes a space,
// and n+1 breaks become n breaks, with the whitespace around them dropped. Most
// values holding a break therefore have no single-quoted spelling at all, so
// they are written double-quoted, where a break is an escape and survives.
func quotedString(n *StringNode) string {
	if n.Token.Type == token.DoubleQuoteType || strings.ContainsAny(n.Value, "\n\r") {
		return strconv.Quote(n.Value)
	}

	return escapeSingleQuote(n.Value)
}

// String string value to text with quote or literal header if required
//
// A node built rather than parsed carries no token, and there is nothing then
// to say it was quoted or where it stood: it comes out as its plain value.
func (n *StringNode) String() string {
	if n.Token == nil {
		if n.Comment != nil {
			return addCommentString(n.Value, n.Comment)
		}

		return n.Value
	}

	switch n.Token.Type {
	case token.SingleQuoteType, token.DoubleQuoteType:
		quoted := quotedString(n)
		if n.Comment != nil {
			return addCommentString(quoted, n.Comment)
		}

		return quoted
	}

	if header := token.LiteralBlockHeader(n.Value); header != "" {
		// This block assumes that the line breaks in this inside scalar content and the Outside scalar content are the same.
		// It works mostly, but inconsistencies occur if line break characters are mixed.
		lbc := token.DetectLineBreakCharacter(n.Value)
		space := strings.Repeat(" ", int(n.Token.Position.Column)-1)
		indent := strings.Repeat(" ", int(n.Token.Position.IndentNum()))
		values := []string{}
		for _, v := range strings.Split(n.Value, lbc) {
			values = append(values, fmt.Sprintf("%s%s%s", space, indent, v))
		}
		block := strings.TrimSuffix(strings.TrimSuffix(strings.Join(values, lbc), fmt.Sprintf("%s%s%s", lbc, indent, space)), fmt.Sprintf("%s%s", indent, space))
		return fmt.Sprintf("%s%s%s", header, lbc, block)
	} else if token.NeedsQuotedSpelling(n.Value) {
		quoted := strconv.Quote(n.Value)
		if n.Comment != nil {
			return addCommentString(quoted, n.Comment)
		}

		return quoted
	} else if len(n.Value) > 0 && (n.Value[0] == '{' || n.Value[0] == '[') {
		return fmt.Sprintf(`'%s'`, n.Value)
	}
	if n.Comment != nil {
		return addCommentString(n.Value, n.Comment)
	}
	return n.Value
}

func (n *StringNode) stringWithoutComment() string {
	if n.Token == nil {
		return n.Value
	}

	switch n.Token.Type {
	case token.SingleQuoteType, token.DoubleQuoteType:
		return quotedString(n)
	}

	if header := token.LiteralBlockHeader(n.Value); header != "" {
		// This block assumes that the line breaks in this inside scalar content and the Outside scalar content are the same.
		// It works mostly, but inconsistencies occur if line break characters are mixed.
		lbc := token.DetectLineBreakCharacter(n.Value)
		space := strings.Repeat(" ", int(n.Token.Position.Column)-1)
		indent := strings.Repeat(" ", int(n.Token.Position.IndentNum()))
		values := []string{}
		for _, v := range strings.Split(n.Value, lbc) {
			values = append(values, fmt.Sprintf("%s%s%s", space, indent, v))
		}
		block := strings.TrimSuffix(strings.TrimSuffix(strings.Join(values, lbc), fmt.Sprintf("%s%s%s", lbc, indent, space)), fmt.Sprintf("  %s", space))
		return fmt.Sprintf("%s%s%s", header, lbc, block)
	} else if token.NeedsQuotedSpelling(n.Value) {
		return strconv.Quote(n.Value)
	} else if len(n.Value) > 0 && (n.Value[0] == '{' || n.Value[0] == '[') {
		return fmt.Sprintf(`'%s'`, n.Value)
	}
	return n.Value
}

// MarshalYAML encodes to a YAML text
func (n *StringNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// LiteralNode type of literal node
type LiteralNode struct {
	BaseNode
	Start *token.Token
	Value *StringNode
	// Source is the block as the document wrote it, indicators excluded: the
	// bytes from the end of Start to the end of Value, so the indentation of
	// the first content line is part of it.
	//
	// The parser fills it for a folded scalar and leaves it empty for a literal
	// one. Folding rewrites the line structure -- "a\nb" comes back as "a b" --
	// so a folded scalar can only be written out again from the text it was
	// read from; a literal scalar's value is its content exactly and needs no
	// source. A node built by hand rather than parsed has none either, and
	// renders as a literal.
	Source string
}

// Type returns LiteralType
func (n *LiteralNode) Type() NodeType { return LiteralType }

// GetToken returns token instance
func (n *LiteralNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *LiteralNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// GetValue returns string value
func (n *LiteralNode) GetValue() interface{} {
	return n.String()
}

// String literal to text
func (n *LiteralNode) String() string {
	return defaultRenderer.String(n)
}

func (n *LiteralNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// MarshalYAML encodes to a YAML text
func (n *LiteralNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *LiteralNode) IsMergeKey() bool {
	return false
}

// MergeKeyNode type of merge key node
type MergeKeyNode struct {
	BaseNode
	Token *token.Token
}

// Type returns MergeKeyType
func (n *MergeKeyNode) Type() NodeType { return MergeKeyType }

// GetToken returns token instance
func (n *MergeKeyNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns '<<' value
func (n *MergeKeyNode) GetValue() interface{} {
	return n.Token.Value
}

// String returns '<<' value
func (n *MergeKeyNode) String() string {
	return n.stringWithoutComment()
}

func (n *MergeKeyNode) stringWithoutComment() string {
	return n.Token.Value
}

// AddColumn add column number to child nodes recursively
func (n *MergeKeyNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// MarshalYAML encodes to a YAML text
func (n *MergeKeyNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *MergeKeyNode) IsMergeKey() bool {
	return true
}

// BoolNode type of boolean node
type BoolNode struct {
	BaseNode
	Token *token.Token
	Value bool
}

// Type returns BoolType
func (n *BoolNode) Type() NodeType { return BoolType }

// GetToken returns token instance
func (n *BoolNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *BoolNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue returns boolean value
func (n *BoolNode) GetValue() interface{} {
	return n.Value
}

// String boolean to text
func (n *BoolNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

func (n *BoolNode) stringWithoutComment() string {
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *BoolNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *BoolNode) IsMergeKey() bool {
	return false
}

// InfinityNode type of infinity node
type InfinityNode struct {
	BaseNode
	Token *token.Token
	Value float64
}

// Type returns InfinityType
func (n *InfinityNode) Type() NodeType { return InfinityType }

// GetToken returns token instance
func (n *InfinityNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *InfinityNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue returns math.Inf(0) or math.Inf(-1)
func (n *InfinityNode) GetValue() interface{} {
	return n.Value
}

// String infinity to text
func (n *InfinityNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

func (n *InfinityNode) stringWithoutComment() string {
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *InfinityNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *InfinityNode) IsMergeKey() bool {
	return false
}

// NanNode type of nan node
type NanNode struct {
	BaseNode
	Token *token.Token
}

// Type returns NanType
func (n *NanNode) Type() NodeType { return NanType }

// GetToken returns token instance
func (n *NanNode) GetToken() *token.Token {
	return n.Token
}

// AddColumn add column number to child nodes recursively
func (n *NanNode) AddColumn(col int) {
	n.Token.AddColumn(col)
}

// GetValue returns math.NaN()
func (n *NanNode) GetValue() interface{} {
	return math.NaN()
}

// String returns .nan
func (n *NanNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

func (n *NanNode) stringWithoutComment() string {
	return n.Token.Value
}

// MarshalYAML encodes to a YAML text
func (n *NanNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *NanNode) IsMergeKey() bool {
	return false
}

// MapNode interface of MappingValueNode / MappingNode
type MapNode interface {
	MapRange() *MapNodeIter
}

// MapNodeIter is an iterator for ranging over a MapNode
type MapNodeIter struct {
	values []*MappingValueNode
	idx    int
}

const (
	startRangeIndex = -1
)

// Next advances the map iterator and reports whether there is another entry.
// It returns false when the iterator is exhausted.
func (m *MapNodeIter) Next() bool {
	m.idx++
	next := m.idx < len(m.values)
	return next
}

// Key returns the key of the iterator's current map node entry.
func (m *MapNodeIter) Key() MapKeyNode {
	return m.values[m.idx].Key
}

// Value returns the value of the iterator's current map node entry.
func (m *MapNodeIter) Value() Node {
	return m.values[m.idx].Value
}

// KeyValue returns the MappingValueNode of the iterator's current map node entry.
func (m *MapNodeIter) KeyValue() *MappingValueNode {
	return m.values[m.idx]
}

// MappingNode type of mapping node
// DuplicateKey is one entry of a mapping whose key the mapping already held.
//
// YAML 1.2.2 §3.2.1.1: two keys are equal when they resolve to the same node,
// so "7" and "007" are one key written twice while "1" and "1.0" are two. The
// parser records the repeats it finds and refuses nothing; whoever loads the
// document decides what to do about them, which is what lets a linter read a
// document that a decoder will not.
type DuplicateKey struct {
	// Name is the key as its type spells it, which is what makes it the same
	// key: token.KeyName writes an integer in decimal whatever base the
	// document used.
	Name string
	// At is where the repeat stands and FirstAt where the key was first
	// written, so a complaint can name both.
	At, FirstAt token.Position
	// JSONNameOnly marks two keys that YAML tells apart and JSON does not: "1"
	// and "\"1\"" are an integer and a string, and both write the member "1".
	// Only a parse under parser.WithJSONCompatible records one.
	JSONNameOnly bool
}

type MappingNode struct {
	BaseNode
	Start       *token.Token
	End         *token.Token
	IsFlowStyle bool
	// Duplicates holds the entries whose key the mapping already held, in the
	// order they were written. It is nil for a mapping that repeats none, which
	// is nearly every mapping of nearly every document, so a document without
	// duplicates carries nothing for them.
	//
	// It is nil as well where the parse was told to allow them with
	// [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey]: nothing
	// is recorded, so tolerating a repeat costs no memory at all.
	Duplicates  []DuplicateKey
	Values      []*MappingValueNode
	FootComment *CommentGroupNode
	// StartComment is the comment closing the line the '{' stands on, as in
	// "{ # why\n  a: 1 }". Comment holds the one after the '}' instead, so a
	// flow mapping written with both keeps both.
	//
	// A block mapping opens on its first key and never fills this.
	StartComment *CommentGroupNode
}

func (n *MappingNode) startPos() token.Position {
	if len(n.Values) == 0 {
		return n.Start.Position
	}
	return n.Values[0].Key.GetToken().Position
}

// Merge merge key/value of map.
func (n *MappingNode) Merge(target *MappingNode) {
	keyToMapValueMap := map[string]*MappingValueNode{}
	for _, value := range n.Values {
		key := value.Key.String()
		keyToMapValueMap[key] = value
	}
	column := n.startPos().Column - target.startPos().Column
	target.AddColumn(int(column))
	for _, value := range target.Values {
		mapValue, exists := keyToMapValueMap[value.Key.String()]
		if exists {
			mapValue.Value = value.Value
		} else {
			n.Values = append(n.Values, value)
		}
	}
}

// SetIsFlowStyle set value to IsFlowStyle field recursively.
func (n *MappingNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	for _, value := range n.Values {
		value.SetIsFlowStyle(isFlow)
	}
}

// Type returns MappingType
func (n *MappingNode) Type() NodeType { return MappingType }

// GetToken returns token instance
func (n *MappingNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *MappingNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	n.End.AddColumn(col)
	for _, value := range n.Values {
		value.AddColumn(col)
	}
}

// String mapping values to text
// IsMergeKey returns whether it is a MergeKey node.
//
// A collection is never one: "<<" is a scalar.
func (n *MappingNode) IsMergeKey() bool { return false }

// stringWithoutComment renders the mapping for use as a key, where comments
// would be noise -- a key's identity is its content.
func (n *MappingNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

func (n *MappingNode) String() string {
	return defaultRenderer.String(n)
}

// MapRange implements MapNode protocol
func (n *MappingNode) MapRange() *MapNodeIter {
	return &MapNodeIter{
		idx:    startRangeIndex,
		values: n.Values,
	}
}

// MarshalYAML encodes to a YAML text
func (n *MappingNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// MappingKeyNode type of tag node
type MappingKeyNode struct {
	BaseNode
	Start *token.Token
	Value Node
}

// Type returns MappingKeyType
func (n *MappingKeyNode) Type() NodeType { return MappingKeyType }

// GetToken returns token instance
func (n *MappingKeyNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *MappingKeyNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// String tag to text
func (n *MappingKeyNode) String() string {
	return defaultRenderer.String(n)
}

func (n *MappingKeyNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// MarshalYAML encodes to a YAML text
func (n *MappingKeyNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *MappingKeyNode) IsMergeKey() bool {
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

// MappingValueNode type of mapping value
type MappingValueNode struct {
	BaseNode
	Start        *token.Token // delimiter token ':'.
	CollectEntry *token.Token // collect entry token ','.
	Key          MapKeyNode
	Value        Node
	// LineComment is the comment written on the entry's own ':' line, where
	// that line carries nothing else: "? k" over ": # c".
	//
	// An entry written the short way has nowhere to put such a comment but the
	// value, since "a: # c" over "  v" writes the comment on the key's line --
	// and that works, because the value node's own slot is free. An entry
	// written the long way has a ':' on a line of its own, and the comment
	// there is neither the key's nor the value's: on the value it becomes a
	// head comment and collides with one written under the ':', and on
	// BaseNode.Comment it collides with one written above the '?'.
	//
	// ⚠️ [SequenceEntryNode] names HeadComment and reads its *line* comment
	// through the inherited accessor; this type does the reverse, naming the
	// line comment and reading its head one through BaseNode.Comment. The two
	// are consistent in having two slots and inconsistent in which one is
	// named. Renaming BaseNode.Comment reaches every node type, so the
	// asymmetry stands rather than being worth that.
	LineComment *CommentGroupNode
	FootComment *CommentGroupNode
	IsFlowStyle bool
}

// Replace replace value node.
func (n *MappingValueNode) Replace(value Node) error {
	column := n.Value.GetToken().Position.Column - value.GetToken().Position.Column
	value.AddColumn(int(column))
	n.Value = value
	return nil
}

// Type returns MappingValueType
func (n *MappingValueNode) Type() NodeType { return MappingValueType }

// GetToken returns token instance
func (n *MappingValueNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *MappingValueNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Key != nil {
		n.Key.AddColumn(col)
	}
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// SetIsFlowStyle set value to IsFlowStyle field recursively.
func (n *MappingValueNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	switch value := n.Value.(type) {
	case *MappingNode:
		value.SetIsFlowStyle(isFlow)
	case *MappingValueNode:
		value.SetIsFlowStyle(isFlow)
	case *SequenceNode:
		value.SetIsFlowStyle(isFlow)
	}
}

// String mapping value to text
func (n *MappingValueNode) String() string {
	return defaultRenderer.String(n)
}

// MapRange implements MapNode protocol
func (n *MappingValueNode) MapRange() *MapNodeIter {
	return &MapNodeIter{
		idx:    startRangeIndex,
		values: []*MappingValueNode{n},
	}
}

// MarshalYAML encodes to a YAML text
func (n *MappingValueNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// ArrayNode interface of SequenceNode
type ArrayNode interface {
	ArrayRange() *ArrayNodeIter
}

// ArrayNodeIter is an iterator for ranging over a ArrayNode
type ArrayNodeIter struct {
	values []Node
	idx    int
}

// Next advances the array iterator and reports whether there is another entry.
// It returns false when the iterator is exhausted.
func (m *ArrayNodeIter) Next() bool {
	m.idx++
	next := m.idx < len(m.values)
	return next
}

// Value returns the value of the iterator's current array entry.
func (m *ArrayNodeIter) Value() Node {
	return m.values[m.idx]
}

// Len returns length of array
func (m *ArrayNodeIter) Len() int {
	return len(m.values)
}

// SequenceNode type of sequence node
type SequenceNode struct {
	BaseNode
	Start             *token.Token
	End               *token.Token
	IsFlowStyle       bool
	Values            []Node
	ValueHeadComments []*CommentGroupNode
	Entries           []*SequenceEntryNode
	FootComment       *CommentGroupNode
	// StartComment is the comment closing the line the '[' stands on, as in
	// "[ # why\n  a ]". Comment holds the one after the ']' instead, so a flow
	// sequence written with both keeps both.
	//
	// A block sequence opens on the '-' of its first entry, where a comment
	// belongs to that entry, and never fills this.
	StartComment *CommentGroupNode
}

// Replace replace value node.
func (n *SequenceNode) Replace(idx int, value Node) error {
	if len(n.Values) <= idx {
		return fmt.Errorf(
			"invalid index for sequence: sequence length is %d, but specified %d index",
			len(n.Values), idx,
		)
	}
	column := n.Values[idx].GetToken().Position.Column - value.GetToken().Position.Column
	value.AddColumn(int(column))
	n.Values[idx] = value
	return nil
}

// Merge merge sequence value.
func (n *SequenceNode) Merge(target *SequenceNode) {
	column := n.Start.Position.Column - target.Start.Position.Column
	target.AddColumn(int(column))
	n.Values = append(n.Values, target.Values...)
	if len(target.ValueHeadComments) == 0 {
		n.ValueHeadComments = append(n.ValueHeadComments, make([]*CommentGroupNode, len(target.Values))...)
		return
	}
	n.ValueHeadComments = append(n.ValueHeadComments, target.ValueHeadComments...)
}

// SetIsFlowStyle set value to IsFlowStyle field recursively.
func (n *SequenceNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	for _, value := range n.Values {
		switch value := value.(type) {
		case *MappingNode:
			value.SetIsFlowStyle(isFlow)
		case *MappingValueNode:
			value.SetIsFlowStyle(isFlow)
		case *SequenceNode:
			value.SetIsFlowStyle(isFlow)
		}
	}
}

// Type returns SequenceType
func (n *SequenceNode) Type() NodeType { return SequenceType }

// GetToken returns token instance
func (n *SequenceNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *SequenceNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	n.End.AddColumn(col)
	for _, value := range n.Values {
		value.AddColumn(col)
	}
}

// String sequence to text
// IsMergeKey returns whether it is a MergeKey node.
//
// A collection is never one: "<<" is a scalar.
func (n *SequenceNode) IsMergeKey() bool { return false }

// stringWithoutComment renders the sequence for use as a key. SequenceNode
// renders without comments already, so this is String.
func (n *SequenceNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

func (n *SequenceNode) String() string {
	return defaultRenderer.String(n)
}

// ArrayRange implements ArrayNode protocol
func (n *SequenceNode) ArrayRange() *ArrayNodeIter {
	return &ArrayNodeIter{
		idx:    startRangeIndex,
		values: n.Values,
	}
}

// MarshalYAML encodes to a YAML text
func (n *SequenceNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// SequenceEntryNode is the sequence entry.
type SequenceEntryNode struct {
	BaseNode
	HeadComment *CommentGroupNode // head comment.
	LineComment *CommentGroupNode // line comment e.g.) - # comment.
	Start       *token.Token      // entry token.
	Value       Node              // value node.
}

// String node to text
func (n *SequenceEntryNode) String() string {
	return "" // TODO
}

// GetToken returns token instance
func (n *SequenceEntryNode) GetToken() *token.Token {
	return n.Start
}

// Type returns type of node
func (n *SequenceEntryNode) Type() NodeType {
	return SequenceEntryType
}

// AddColumn add column number to child nodes recursively
func (n *SequenceEntryNode) AddColumn(col int) {
	n.Start.AddColumn(col)
}

// SetComment set line comment.
func (n *SequenceEntryNode) SetComment(cm *CommentGroupNode) error {
	n.LineComment = cm
	return nil
}

// Comment returns comment token instance
func (n *SequenceEntryNode) GetComment() *CommentGroupNode {
	return n.LineComment
}

// MarshalYAML
func (n *SequenceEntryNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// SequenceEntry creates SequenceEntryNode instance.
func SequenceEntry(start *token.Token, value Node, headComment *CommentGroupNode) *SequenceEntryNode {
	return &SequenceEntryNode{
		HeadComment: headComment,
		Start:       start,
		Value:       value,
	}
}

// SequenceMergeValue creates SequenceMergeValueNode instance.
func SequenceMergeValue(values ...MapNode) *SequenceMergeValueNode {
	return &SequenceMergeValueNode{
		values: values,
	}
}

// SequenceMergeValueNode is used to convert the Sequence node specified for the merge key into a MapNode format.
type SequenceMergeValueNode struct {
	values []MapNode
}

// MapRange returns MapNodeIter instance.
func (n *SequenceMergeValueNode) MapRange() *MapNodeIter {
	ret := &MapNodeIter{idx: startRangeIndex}
	for _, value := range n.values {
		iter := value.MapRange()
		ret.values = append(ret.values, iter.values...)
	}
	return ret
}

// AnchorNode type of anchor node
type AnchorNode struct {
	BaseNode
	Start *token.Token
	Name  Node
	Value Node
}

func (n *AnchorNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

func (n *AnchorNode) SetName(name string) error {
	if n.Name == nil {
		return ErrInvalidAnchorName
	}
	s, ok := n.Name.(*StringNode)
	if !ok {
		return ErrInvalidAnchorName
	}
	s.Value = name
	return nil
}

// Type returns AnchorType
func (n *AnchorNode) Type() NodeType { return AnchorType }

// GetToken returns token instance
func (n *AnchorNode) GetToken() *token.Token {
	return n.Start
}

func (n *AnchorNode) GetValue() any {
	return n.Value.GetToken().Value
}

// AddColumn add column number to child nodes recursively
func (n *AnchorNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Name != nil {
		n.Name.AddColumn(col)
	}
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// String anchor to text
func (n *AnchorNode) String() string {
	return defaultRenderer.String(n)
}

// MarshalYAML encodes to a YAML text
func (n *AnchorNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *AnchorNode) IsMergeKey() bool {
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

// AliasNode type of alias node
type AliasNode struct {
	BaseNode
	Start *token.Token
	Value Node
	// Target is the node the alias names, which the parser fills as it reads
	// the alias. Read it to expand an alias without collecting anchors of your
	// own; the parser resolves the name where the alias stands, so two aliases
	// of one name written either side of a redefined anchor point at the two
	// nodes and not both at the last.
	//
	// It is nil for an alias built by hand and for one the encoder writes,
	// neither of which went through a parse.
	//
	// ⚠️ Target may reach back to a node the alias stands inside: "&x [ *x ]"
	// is a document, and YAML's representation is a graph. Expand it with a
	// guard, as [github.com/go-openapi/go-yaml.Decoder] does -- it refuses a
	// cycle, because a Go value built by walking has nowhere to put one.
	//
	// Nothing follows Target on its own: [Walk] does not, and neither does the
	// renderer, which writes an alias as the "*name" it was written as.
	Target Node
}

func (n *AliasNode) stringWithoutComment() string {
	// An alias carries no comment of its own, and it is the '*' that makes it
	// one: dropping it here would write the name as a plain scalar.
	return n.String()
}

func (n *AliasNode) SetName(name string) error {
	if n.Value == nil {
		return ErrInvalidAliasName
	}
	s, ok := n.Value.(*StringNode)
	if !ok {
		return ErrInvalidAliasName
	}
	s.Value = name
	return nil
}

// Type returns AliasType
func (n *AliasNode) Type() NodeType { return AliasType }

// GetToken returns token instance
func (n *AliasNode) GetToken() *token.Token {
	return n.Start
}

func (n *AliasNode) GetValue() any {
	return n.Value.GetToken().Value
}

// AddColumn add column number to child nodes recursively
func (n *AliasNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// String alias to text
func (n *AliasNode) String() string {
	return fmt.Sprintf("*%s", n.Value.String())
}

// MarshalYAML encodes to a YAML text
func (n *AliasNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *AliasNode) IsMergeKey() bool {
	return false
}

// DirectiveNode type of directive node
type DirectiveNode struct {
	BaseNode
	// Start is '%' token.
	Start *token.Token
	// Name is directive name e.g.) "YAML" or "TAG".
	Name Node
	// Values is directive values e.g.) "1.2" or "!!" and "tag:clarkevans.com,2002:app/".
	Values []Node
}

// Type returns DirectiveType
func (n *DirectiveNode) Type() NodeType { return DirectiveType }

// GetToken returns token instance
func (n *DirectiveNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *DirectiveNode) AddColumn(col int) {
	if n.Name != nil {
		n.Name.AddColumn(col)
	}
	for _, value := range n.Values {
		value.AddColumn(col)
	}
}

// String directive to text
func (n *DirectiveNode) String() string {
	values := make([]string, 0, len(n.Values))
	for _, val := range n.Values {
		values = append(values, val.String())
	}
	return strings.Join(append([]string{"%" + n.Name.String()}, values...), " ")
}

// MarshalYAML encodes to a YAML text
func (n *DirectiveNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// TagNode type of tag node
type TagNode struct {
	BaseNode
	Start *token.Token
	Value Node
	// URI is the tag this node carries, expanded from the shorthand the
	// document wrote: "!!int" and "!<tag:yaml.org,2002:int>" both give
	// tag:yaml.org,2002:int, and "!thing" gives "!thing", since the primary
	// handle expands to "!". A "%TAG" directive changes what a handle expands
	// to, so read this rather than Start to find which tag the node carries.
	URI string
	// Schema is the scalar schema the document is read under, which decides
	// what spellings the tag's type has: "017" is octal under %YAML 1.1 and
	// decimal without it, and "0b101" is an integer under the first and no
	// number at all under the second.
	//
	// The scanner types a plain scalar under this already, so for one the
	// token carries the answer. A quoted or block scalar it never sniffs --
	// the quotes make it a string when nothing else types it -- and a tag does
	// type it, so its text is read under this instead. Both spellings of the
	// same content then get one answer, which is what §3.3.2 requires: the
	// "!" non-specific tag is given only to a node *lacking* an explicit tag,
	// so `!!int "017"` and `!!int 017` are one node and cannot differ.
	//
	// It is stamped on the node for the same reason LaxTags is: a subtree
	// handed to a consumer carries how to read it.
	Schema token.Schema
	// LaxTags says the document was read with
	// [github.com/go-openapi/go-yaml/parser.WithLaxTags], so a consumer that
	// can fall back to the text should, where it would otherwise refuse a tag
	// naming a type its scalar is not. [TagNode.Resolve] reports it as
	// [Resolution.Lax].
	//
	// It is stamped on the node rather than kept beside the document, so that a
	// node handed over on its own -- what DecodeFromNode and
	// expressions.Path.Read are given -- carries the policy it was read under.
	LaxTags bool
	// Implicit says the parse built this tag and the document did not write it.
	//
	// YAML 1.1 resolves types 1.2's core schema leaves out, and a timestamp is
	// one: under a "%YAML 1.1" directive, or
	// [github.com/go-openapi/go-yaml/parser.WithYAMLVersion] at 1.1,
	// "a: 2001-12-14" resolves to tag:yaml.org,2002:timestamp where under 1.2
	// it is the string. The tag records which type the scalar resolved to,
	// where the document wrote nothing.
	//
	// A renderer writing the document back leaves an implicit tag off, since
	// the source holds none; one reformatting to explicit tags writes it. A
	// consumer reading types -- the decoder, ToJSON -- reads URI and does not
	// care which way it arrived.
	Implicit bool
}

func (n *TagNode) GetValue() any {
	scalar, ok := n.Value.(ScalarNode)
	if !ok {
		return nil
	}
	return scalar.GetValue()
}

func (n *TagNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// Type returns TagType
func (n *TagNode) Type() NodeType { return TagType }

// GetToken returns token instance
func (n *TagNode) GetToken() *token.Token {
	return n.Start
}

// AddColumn add column number to child nodes recursively
func (n *TagNode) AddColumn(col int) {
	n.Start.AddColumn(col)
	if n.Value != nil {
		n.Value.AddColumn(col)
	}
}

// String tag to text
func (n *TagNode) String() string {
	return defaultRenderer.String(n)
}

// MarshalYAML encodes to a YAML text
func (n *TagNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// IsMergeKey returns whether it is a MergeKey node.
func (n *TagNode) IsMergeKey() bool {
	if name, known := token.ReservedTagOf(n.URI); known && name == token.MergeTag {
		// The tag names the type, and a written tag is not tied to a spec
		// version: "!!merge << : *a" folds under the core schema, where a bare
		// "<<" is an ordinary key. Reading through to the value would ask the
		// wrong question there, since the parser builds a string for the "<<"
		// it did not resolve.
		return true
	}
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

func (n *TagNode) ArrayRange() *ArrayNodeIter {
	arr, ok := n.Value.(ArrayNode)
	if !ok {
		return nil
	}
	return arr.ArrayRange()
}

// CommentNode type of comment node
type CommentNode struct {
	BaseNode
	Token *token.Token
	// edit is what a caller asked to happen to this comment, and text the words
	// to put in its place. The zero value leaves the document's own text alone,
	// so a parsed tree nobody has touched carries no edit at all.
	//
	// It is kept beside the token rather than written over it: the token says
	// which bytes of the source this comment stands on, and
	// [Renderer.VerbatimFile] needs them to take the old text out of the copy.
	// Assigning over a node's comment throws them away, which is why a comment
	// the document wrote is edited through [CommentNode.Replace] and
	// [CommentNode.Remove] and not by putting another group in its place.
	edit commentEdit
	text string
}

// commentEdit is what happens to a comment when it is rendered.
type commentEdit uint8

const (
	// commentAsWritten writes the comment the document wrote.
	commentAsWritten commentEdit = iota
	// commentReplaced writes CommentNode.text in its place.
	commentReplaced
	// commentRemoved writes nothing, and takes the line the comment stood on
	// with it where the comment was alone on it.
	commentRemoved
)

// Replace records that n renders as "# " + text instead of what the document
// wrote.
//
// text is the comment without its "#": [CommentNode.String] adds one, as it
// does for a parsed comment, whose token holds the text after the "#" as well.
// A line break in text would end the comment and put the rest of it into the
// document as content, so it is rejected here rather than at rendering time.
func (n *CommentNode) Replace(text string) error {
	if n == nil {
		return errors.New("no comment to replace")
	}
	if strings.ContainsAny(text, "\n\r") {
		return fmt.Errorf("a comment holds one line, and this holds a break: %q", text)
	}
	n.edit, n.text = commentReplaced, text

	return nil
}

// Remove records that n is not written at all.
//
// The comment stays in the tree so that [Renderer.VerbatimFile] knows which
// bytes of the source to leave out. Setting the node's comment field to nil
// instead loses that and the old text comes back.
func (n *CommentNode) Remove() {
	if n == nil {
		return
	}
	n.edit, n.text = commentRemoved, ""
}

// Removed reports whether n has been removed by [CommentNode.Remove].
func (n *CommentNode) Removed() bool {
	return n != nil && n.edit == commentRemoved
}

// Text is what n renders as, without the "#": the replacement where one was
// set, and the document's own words otherwise.
func (n *CommentNode) Text() string {
	if n == nil {
		return ""
	}
	if n.edit == commentReplaced {
		return n.text
	}
	if n.Token == nil {
		return ""
	}

	return n.Token.Value
}

// Type returns TagType
func (n *CommentNode) Type() NodeType { return CommentType }

// GetToken returns token instance
func (n *CommentNode) GetToken() *token.Token { return n.Token }

// AddColumn add column number to child nodes recursively
func (n *CommentNode) AddColumn(col int) {
	if n.Token == nil {
		return
	}
	n.Token.AddColumn(col)
}

// String comment to text
func (n *CommentNode) String() string {
	if n.Removed() {
		return ""
	}

	return "#" + n.Text()
}

// MarshalYAML encodes to a YAML text
func (n *CommentNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// CommentGroupNode type of comment node
type CommentGroupNode struct {
	BaseNode
	Comments []*CommentNode
}

// Type returns TagType
func (n *CommentGroupNode) Type() NodeType { return CommentType }

// GetToken returns token instance
func (n *CommentGroupNode) GetToken() *token.Token {
	for _, comment := range n.Comments {
		if !comment.Removed() {
			return comment.Token
		}
	}

	return nil
}

// Visible hands over the comments of n that are still written, in the order
// they stand. A comment [CommentNode.Remove] took out is skipped.
func (n *CommentGroupNode) Visible() []*CommentNode {
	if n == nil {
		return nil
	}

	out := make([]*CommentNode, 0, len(n.Comments))
	for _, comment := range n.Comments {
		if !comment.Removed() {
			out = append(out, comment)
		}
	}

	return out
}

// Blank reports whether n writes nothing: it is absent, holds no comment, or
// every comment in it has been removed.
//
// A renderer asks this rather than testing the group against nil, or a group
// emptied by [CommentNode.Remove] leaves the space that was to stand in front
// of it.
func (n *CommentGroupNode) Blank() bool {
	if n == nil {
		return true
	}
	for _, comment := range n.Comments {
		if !comment.Removed() {
			return false
		}
	}

	return true
}

// Replace records that the group renders as one comment reading "# " + text.
//
// It is [CommentNode.Replace] on a group holding one comment, which is what a
// group holds nearly everywhere: a run of comment lines above one node is the
// exception. It rejects a group holding several rather than guessing which one
// the caller meant.
func (n *CommentGroupNode) Replace(text string) error {
	if n == nil {
		return errors.New("no comment to replace")
	}
	visible := n.Visible()
	if len(visible) != 1 {
		return fmt.Errorf("this group holds %d comments, so replace the one you mean", len(visible))
	}

	return visible[0].Replace(text)
}

// Remove records that nothing in the group is written.
func (n *CommentGroupNode) Remove() {
	if n == nil {
		return
	}
	for _, comment := range n.Comments {
		comment.Remove()
	}
}

// AddColumn add column number to child nodes recursively
func (n *CommentGroupNode) AddColumn(col int) {
	for _, comment := range n.Comments {
		comment.AddColumn(col)
	}
}

// String comment to text
func (n *CommentGroupNode) String() string {
	values := []string{}
	for _, comment := range n.Visible() {
		values = append(values, comment.String())
	}

	return strings.Join(values, "\n")
}

func (n *CommentGroupNode) StringWithSpace(col int) string {
	values := []string{}
	space := strings.Repeat(" ", col)
	for _, comment := range n.Visible() {
		space := space
		if comment.Token != nil && comment.Token.BlankLineAbove() {
			space = fmt.Sprintf("%s%s", "\n", space)
		}
		values = append(values, space+comment.String())
	}
	return strings.Join(values, "\n")
}

// MarshalYAML encodes to a YAML text
func (n *CommentGroupNode) MarshalYAML() ([]byte, error) {
	return []byte(n.String()), nil
}

// Visitor has Visit method that is invokded for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children of node with the visitor w,
// followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(Node) Visitor
}

// Walk traverses an AST in depth-first order: It starts by calling v.Visit(node); node must not be nil.
// If the visitor w returned by v.Visit(node) is not nil,
// Walk is invoked recursively with visitor w for each of the non-nil children of node,
// followed by a call of w.Visit(nil).
func Walk(v Visitor, node Node) {
	if v = v.Visit(node); v == nil {
		return
	}

	switch n := node.(type) {
	case *CommentNode:
	case *NullNode:
		walkComment(v, &n.BaseNode)
	case *IntegerNode:
		walkComment(v, &n.BaseNode)
	case *FloatNode:
		walkComment(v, &n.BaseNode)
	case *StringNode:
		walkComment(v, &n.BaseNode)
	case *MergeKeyNode:
		walkComment(v, &n.BaseNode)
	case *BoolNode:
		walkComment(v, &n.BaseNode)
	case *InfinityNode:
		walkComment(v, &n.BaseNode)
	case *NanNode:
		walkComment(v, &n.BaseNode)
	case *LiteralNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Value)
	case *DirectiveNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Name)
		for _, value := range n.Values {
			Walk(v, value)
		}
	case *TagNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Value)
	case *DocumentNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Body)
	case *MappingNode:
		walkComment(v, &n.BaseNode)
		for _, value := range n.Values {
			Walk(v, value)
		}
	case *MappingKeyNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Value)
	case *MappingValueNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Key)
		Walk(v, n.Value)
	case *SequenceNode:
		walkComment(v, &n.BaseNode)
		for _, value := range n.Values {
			Walk(v, value)
		}
	case *AnchorNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Name)
		Walk(v, n.Value)
	case *AliasNode:
		walkComment(v, &n.BaseNode)
		Walk(v, n.Value)
	}
}

func walkComment(v Visitor, base *BaseNode) {
	if base == nil {
		return
	}
	if base.Comment == nil {
		return
	}
	Walk(v, base.Comment)
}

type filterWalker struct {
	typ     NodeType
	results []Node
}

func (v *filterWalker) Visit(n Node) Visitor {
	if v.typ == n.Type() {
		v.results = append(v.results, n)
	}
	return v
}

type parentFinder struct {
	target Node
}

func (f *parentFinder) walk(parent, node Node) Node {
	if f.target == node {
		return parent
	}
	switch n := node.(type) {
	case *CommentNode:
		return nil
	case *NullNode:
		return nil
	case *IntegerNode:
		return nil
	case *FloatNode:
		return nil
	case *StringNode:
		return nil
	case *MergeKeyNode:
		return nil
	case *BoolNode:
		return nil
	case *InfinityNode:
		return nil
	case *NanNode:
		return nil
	case *LiteralNode:
		return f.walk(node, n.Value)
	case *DirectiveNode:
		if found := f.walk(node, n.Name); found != nil {
			return found
		}
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *TagNode:
		return f.walk(node, n.Value)
	case *DocumentNode:
		return f.walk(node, n.Body)
	case *MappingNode:
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *MappingKeyNode:
		return f.walk(node, n.Value)
	case *MappingValueNode:
		if found := f.walk(node, n.Key); found != nil {
			return found
		}
		return f.walk(node, n.Value)
	case *SequenceNode:
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *AnchorNode:
		if found := f.walk(node, n.Name); found != nil {
			return found
		}
		return f.walk(node, n.Value)
	case *AliasNode:
		return f.walk(node, n.Value)
	}
	return nil
}

// Parent get parent node from child node.
func Parent(root, child Node) Node {
	finder := &parentFinder{target: child}
	return finder.walk(root, root)
}

// Filter returns a list of nodes that match the given type.
func Filter(typ NodeType, node Node) []Node {
	walker := &filterWalker{typ: typ}
	Walk(walker, node)
	return walker.results
}

// FilterFile returns a list of nodes that match the given type.
func FilterFile(typ NodeType, file *File) []Node {
	results := []Node{}
	for _, doc := range file.Docs {
		walker := &filterWalker{typ: typ}
		Walk(walker, doc)
		results = append(results, walker.results...)
	}
	return results
}

type ErrInvalidMergeType struct {
	dst Node
	src Node
}

func (e *ErrInvalidMergeType) Error() string {
	return fmt.Sprintf("cannot merge %s into %s", e.src.Type(), e.dst.Type())
}

// Merge merge document, map, sequence node.
func Merge(dst Node, src Node) error {
	if doc, ok := src.(*DocumentNode); ok {
		src = doc.Body
	}
	err := &ErrInvalidMergeType{dst: dst, src: src}
	switch dst.Type() {
	case DocumentType:
		node, _ := dst.(*DocumentNode)
		return Merge(node.Body, src)
	case MappingType:
		node, _ := dst.(*MappingNode)
		target, ok := src.(*MappingNode)
		if !ok {
			return err
		}
		node.Merge(target)
		return nil
	case SequenceType:
		node, _ := dst.(*SequenceNode)
		target, ok := src.(*SequenceNode)
		if !ok {
			return err
		}
		node.Merge(target)
		return nil
	}
	return err
}

// BlockSource is the block a folded scalar was written as: src from the end of
// its header token to the end of its content token, so that the indentation of
// the first content line stands at the front of it. A parser fills
// [LiteralNode.Source] with it.
//
// Nothing for a literal scalar, whose value is its content exactly, and nothing
// where either offset does not address its token -- a window on some other part
// of the document is worse than no source at all.
func BlockSource(src string, header, content *token.Token) string {
	if header == nil || content == nil || header.Type != token.FoldedType {
		return ""
	}

	start, end := int(header.EndOffset()), int(content.EndOffset())
	if start < 0 || end > len(src) || start >= end {
		return ""
	}

	// A header carrying a comment -- ">1 # indentation indicator" -- ends at
	// the comment rather than at the break, the comment standing as a token of
	// its own. The block begins on the next line either way, so a header that
	// did not end one is walked to the end of its line first. Without it the
	// block was read back with the comment text at the front of it.
	if start > 0 && !isLineBreak(src[start-1]) {
		for start < end && !isLineBreak(src[start]) {
			start++
		}
		if start < end && src[start] == '\r' {
			start++
		}
		if start < end && src[start] == '\n' {
			start++
		}
	}

	if start >= end {
		return ""
	}

	return src[start:end]
}

// isLineBreak reports whether c ends a line.
func isLineBreak(c byte) bool { return c == '\n' || c == '\r' }
