// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// File holds the documents of one YAML stream, in order.
type File struct {
	Name string
	Docs []*DocumentNode
	// text and read hold what Read is handing out. They belong to the file
	// instead of each node: a cursor on every node would cost 8 bytes of every
	// node in the document, and no caller needs a reader on a single node.
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

// String returns every document as text.
func (f *File) String() string {
	return defaultRenderer.File(f)
}

// DocumentNode is one document of a stream: its "---", its body and its "...".
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
	// FootComment holds the comments written after the body, where the body has
	// no slot of its own for them: a scalar, a block scalar or a flow collection
	// at the root. "x" over "# note" holds "# note" here, and the renderer writes
	// it below the body, before any "...". A block mapping or sequence keeps the
	// comments after it in its own FootComment.
	//
	// They went to the body's own comment before, the slot of a comment on its
	// line, so "x" over "# note" rendered as "x # note", and "|" over "  text"
	// over "# note" as "| # note" over "  text".
	FootComment *CommentGroupNode
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
	// Schema is the resolution the document's plain scalars were read against:
	// the version its "%YAML" line declares, or the parser's WithYAMLVersion
	// where it declares none. The zero value is [token.Schema12].
	//
	// A node carries no version of its own, so a reader that renders part of
	// the document for another parse reads it here: under 1.1, "yes" is true
	// and "<<" merges, and a fragment parsed again under 1.2 reads neither.
	Schema token.Schema
}

// Type returns [DocumentType].
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

// String renders the document with the default [Renderer].
func (d *DocumentNode) String() string {
	return defaultRenderer.String(d)
}

// DirectiveNode is a directive, such as "%YAML 1.2", above a document.
type DirectiveNode struct {
	BaseNode

	// Start is '%' token.
	Start *token.Token
	// Name is directive name e.g.) "YAML" or "TAG".
	Name Node
	// Values is directive values e.g.) "1.2" or "!!" and "tag:clarkevans.com,2002:app/".
	Values []Node
}

// Type returns [DirectiveType].
func (n *DirectiveNode) Type() NodeType { return DirectiveType }

// GetToken returns n.Start, the "%".
func (n *DirectiveNode) GetToken() *token.Token {
	return n.Start
}

// String returns the directive line.
func (n *DirectiveNode) String() string {
	values := make([]string, 0, len(n.Values))
	for _, val := range n.Values {
		values = append(values, directiveWord(val))
	}
	return strings.Join(append([]string{"%" + directiveWord(n.Name)}, values...), " ")
}

// directiveWord returns the text of a directive's name or parameter, as written, with the comment closing the line
// when the word is the last one.
//
// 6.8 reads each as ns-char+ and has no quoting, so the word is written as its token holds it. Rendered as a
// scalar, it was quoted wherever it opened with an indicator, and "%{YAML 1.2" came back as "%'{YAML' 1.2", a
// directive with another name.
func directiveWord(n Node) string {
	tk := n.GetToken()
	if tk == nil {
		return n.String()
	}

	if comment := n.GetComment(); comment != nil {
		return addCommentString(tk.Value, comment)
	}

	return tk.Value
}
