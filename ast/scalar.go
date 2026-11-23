// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// SetComment records a comment on n, and rejects a comment of a caller's own on
// a null the document did not write.
//
// An implicit null stands for a node that is not in the source: the value of
// "a:" with nothing after it, the entry of a bare "-". A comment the parser
// hangs there is a comment the document did write -- "k: # note" holds the note
// on the null -- and it is copied back from the bytes it was read from. One a
// caller adds has no bytes and nowhere to go: [Renderer.Verbatim] laid the null
// out as though it had been inserted, so "- " over "- 1" came back with an entry
// that was never there, reading [nil nil 1] where the document holds [nil 1].
//
// A null a caller built, and one the document spells out, both take a comment:
// only an implicit one is refused, and only for a comment with no source of its
// own.
func (n *NullNode) SetComment(node *CommentGroupNode) error {
	if node != nil && !fromSourceGroup(node) && n.Token != nil && n.Token.Type == token.ImplicitNullType {
		return errors.New("this null stands for a node the document does not hold, so a comment on it has nowhere to go")
	}

	return n.BaseNode.SetComment(node)
}

// NullNode is a null: "null", "~", "Null", "NULL", or nothing at all where a value is due.
type NullNode struct {
	BaseNode

	Token *token.Token
}

// Type returns [NullType].
func (n *NullNode) Type() NodeType { return NullType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *NullNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns nil.
func (n *NullNode) GetValue() any {
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

// IsMergeKey returns false.
func (n *NullNode) IsMergeKey() bool {
	return false
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

// IntegerNode is an integer scalar, such as 42, 0x2a or 0o52.
type IntegerNode struct {
	BaseNode

	Token *token.Token
}

// Type returns [IntegerType].
func (n *IntegerNode) Type() NodeType { return IntegerType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *IntegerNode) GetToken() *token.Token {
	return n.Token
}

// GetValue reads the integer and returns it as an int64 where the document
// wrote a sign, as a uint64 where it did not, and as a [big.Int] where neither
// holds it. It returns nil where the text
// is not an integer after all.
//
// The parser types the scalar without converting it, so the conversion happens
// here, each time it is asked for. Use [IntegerNode.Text] to read the digits as
// the document wrote them.
func (n *IntegerNode) GetValue() any {
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

// String returns the integer as the document wrote it, with its comment.
func (n *IntegerNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

// IsMergeKey returns false.
func (n *IntegerNode) IsMergeKey() bool {
	return false
}

func (n *IntegerNode) stringWithoutComment() string {
	return n.Token.Value
}

// FloatNode is a floating-point scalar, such as 1.5 or 6.02e23.
type FloatNode struct {
	BaseNode

	Token *token.Token
}

// Type returns [FloatType].
func (n *FloatNode) Type() NodeType { return FloatType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *FloatNode) GetToken() *token.Token {
	return n.Token
}

// GetValue reads the float and returns it as a float64, as a [big.Float] where
// the number reaches past what a float64 holds, or 0 where the text is not a
// float after all. A float whose decimal exponent is past ±1000 is an infinity
// of its sign, or zero: see [token.FloatPastRange].
//
// The parser types the scalar without converting it, so the conversion happens
// here, each time it is asked for. Use [FloatNode.Text] to read the number as
// the document wrote it.
func (n *FloatNode) GetValue() any {
	if n.Token == nil {
		return float64(0)
	}
	if v, ok := token.ParseFloat(n.Token.Value, n.Token.Type); ok {
		return v
	}
	if v, ok := token.FloatPastRange(n.Token.Value, n.Token.Type); ok {
		return v
	}

	// Past what a float64 reaches. See [IntegerNode.GetValue].
	if v, ok := token.ParseBigFloat(n.Token.Value, n.Token.Type); ok {
		return v
	}

	return float64(0)
}

// String returns the number as the document wrote it, with its comment.
func (n *FloatNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

// IsMergeKey returns false.
func (n *FloatNode) IsMergeKey() bool {
	return false
}

func (n *FloatNode) stringWithoutComment() string {
	return n.Token.Value
}

// StringNode is a string scalar: plain, single-quoted or double-quoted.
type StringNode struct {
	BaseNode

	Token *token.Token
	Value string
}

// Type returns [StringType].
func (n *StringNode) Type() NodeType { return StringType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *StringNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns the string, with its quotes removed and its escapes read.
func (n *StringNode) GetValue() any {
	return n.Value
}

// IsMergeKey returns false: a quoted "<<", or one under a tag, is a [StringNode] and not a merge key.
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
//
// A plain value opening with "---" or "..." comes out single-quoted: the text
// starts in column 0, where written plain it reads back as a document marker.
func (n *StringNode) String() string {
	if readsAsMarker(n) {
		return quotedMarker(n, true)
	}

	return n.plainString()
}

// plainString renders n as String does, without quoting a value that opens with
// a document marker. The renderer uses it wherever n does not start a line.
func (n *StringNode) plainString() string {
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
	default:
		// nothing to do
	}

	if header := token.LiteralBlockHeader(n.Value); header != "" {
		// This block assumes that the line breaks in this inside scalar content and the Outside scalar content are the same.
		// It works mostly, but inconsistencies occur if line break characters are mixed.
		lbc := token.DetectLineBreakCharacter(n.Value)
		space := strings.Repeat(" ", int(n.Token.Position.Column)-1)
		indent := strings.Repeat(" ", int(n.Token.Position.IndentNum()))
		values := []string{}
		for v := range strings.SplitSeq(n.Value, lbc) {
			values = append(values, fmt.Sprintf("%s%s%s", space, indent, v))
		}
		block := strings.TrimSuffix(strings.TrimSuffix(strings.Join(values, lbc), fmt.Sprintf("%s%s%s", lbc, indent, space)), fmt.Sprintf("%s%s", indent, space))
		return fmt.Sprintf("%s%s%s", header, lbc, block)
	} else if token.NeedsQuotedSpelling(n.Value) || strings.Contains(n.Value, "\n") {
		// A line break with no block scalar spelling. See token.LiteralBlockHeader.
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
	default:
		// nothing to do
	}

	if header := token.LiteralBlockHeader(n.Value); header != "" {
		// This block assumes that the line breaks in this inside scalar content and the Outside scalar content are the same.
		// It works mostly, but inconsistencies occur if line break characters are mixed.
		lbc := token.DetectLineBreakCharacter(n.Value)
		space := strings.Repeat(" ", int(n.Token.Position.Column)-1)
		indent := strings.Repeat(" ", int(n.Token.Position.IndentNum()))
		values := []string{}
		for v := range strings.SplitSeq(n.Value, lbc) {
			values = append(values, fmt.Sprintf("%s%s%s", space, indent, v))
		}
		block := strings.TrimSuffix(strings.TrimSuffix(strings.Join(values, lbc), fmt.Sprintf("%s%s%s", lbc, indent, space)), "  "+space)
		return fmt.Sprintf("%s%s%s", header, lbc, block)
	} else if token.NeedsQuotedSpelling(n.Value) || strings.Contains(n.Value, "\n") {
		// A line break with no block scalar spelling. See token.LiteralBlockHeader.
		return strconv.Quote(n.Value)
	} else if len(n.Value) > 0 && (n.Value[0] == '{' || n.Value[0] == '[') {
		return fmt.Sprintf(`'%s'`, n.Value)
	}
	return n.Value
}

// LiteralNode is a block scalar, literal ("|") or folded (">").
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

// Type returns [LiteralType].
func (n *LiteralNode) Type() NodeType { return LiteralType }

// GetToken returns n.Start, the block's header.
func (n *LiteralNode) GetToken() *token.Token {
	return n.Start
}

// GetValue returns the block scalar's content, folded and chomped as its header says: "|" over "  x" over "  y" is
// "x\ny\n". Use [LiteralNode.String] for the block as it is written, header included.
func (n *LiteralNode) GetValue() any {
	if n.Value == nil {
		return ""
	}

	return n.Value.GetValue()
}

// String returns the block scalar as written: its header, then its content.
func (n *LiteralNode) String() string {
	return defaultRenderer.String(n)
}

// IsMergeKey returns false.
func (n *LiteralNode) IsMergeKey() bool {
	return false
}

func (n *LiteralNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// MergeKeyNode is the "<<" key of a merge.
type MergeKeyNode struct {
	BaseNode

	Token *token.Token
}

// Type returns [MergeKeyType].
func (n *MergeKeyNode) Type() NodeType { return MergeKeyType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *MergeKeyNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns the key's text, "<<".
func (n *MergeKeyNode) GetValue() any {
	return n.Token.Value
}

// String returns "<<".
func (n *MergeKeyNode) String() string {
	return n.stringWithoutComment()
}

// IsMergeKey returns true.
func (n *MergeKeyNode) IsMergeKey() bool {
	return true
}

func (n *MergeKeyNode) stringWithoutComment() string {
	return n.Token.Value
}

// BoolNode is a boolean scalar, such as true or False.
type BoolNode struct {
	BaseNode

	Token *token.Token
	Value bool
}

// Type returns [BoolType].
func (n *BoolNode) Type() NodeType { return BoolType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *BoolNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns the bool.
func (n *BoolNode) GetValue() any {
	return n.Value
}

// String returns the boolean as the document wrote it, with its comment.
func (n *BoolNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

// IsMergeKey returns false.
func (n *BoolNode) IsMergeKey() bool {
	return false
}

func (n *BoolNode) stringWithoutComment() string {
	return n.Token.Value
}

// InfinityNode is an infinity: ".inf", "+.inf" or "-.inf", in any of the spellings [Infinity] accepts.
type InfinityNode struct {
	BaseNode

	Token *token.Token
	Value float64
}

// Type returns [InfinityType].
func (n *InfinityNode) Type() NodeType { return InfinityType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *InfinityNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns math.Inf(0) or math.Inf(-1).
func (n *InfinityNode) GetValue() any {
	return n.Value
}

// String returns the infinity as the document wrote it, with its comment.
func (n *InfinityNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

// IsMergeKey returns false.
func (n *InfinityNode) IsMergeKey() bool {
	return false
}

func (n *InfinityNode) stringWithoutComment() string {
	return n.Token.Value
}

// NanNode is a not-a-number: ".nan", ".NaN" or ".NAN".
type NanNode struct {
	BaseNode

	Token *token.Token
}

// Type returns [NanType].
func (n *NanNode) Type() NodeType { return NanType }

// GetToken returns n.Token, the scalar as the document wrote it.
func (n *NanNode) GetToken() *token.Token {
	return n.Token
}

// GetValue returns math.NaN().
func (n *NanNode) GetValue() any {
	return math.NaN()
}

// String returns the value as the document wrote it, with its comment.
func (n *NanNode) String() string {
	if n.Comment != nil {
		return addCommentString(n.Token.Value, n.Comment)
	}
	return n.stringWithoutComment()
}

// IsMergeKey returns false.
func (n *NanNode) IsMergeKey() bool {
	return false
}

func (n *NanNode) stringWithoutComment() string {
	return n.Token.Value
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
