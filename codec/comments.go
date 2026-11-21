// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

// CommentPosition says where a comment stands relative to the value it belongs
// to.
//
// This is the one thing [ast] does not record. An [ast.CommentGroupNode] holds
// the text of a run of comments and nothing about its placement: a node has a
// single comment slot, and which of the three a comment is follows from which
// node the group was attached to. The parser works the placement out and the
// tree does not keep it, so a caller reading comments out of a document or
// writing them into one needs this to say what the tree cannot.
type CommentPosition int

const (
	// CommentHeadPosition is a comment on the lines above its value.
	CommentHeadPosition CommentPosition = CommentPosition(iota)
	// CommentLinePosition is a comment sharing a line with its value, after it.
	CommentLinePosition
	// CommentFootPosition is a comment on the lines below its value.
	CommentFootPosition
)

func (p CommentPosition) String() string {
	switch p {
	case CommentHeadPosition:
		return "Head"
	case CommentLinePosition:
		return "Line"
	case CommentFootPosition:
		return "Foot"
	default:
		return ""
	}
}

// LineComment returns a comment to write after its value, on the same line.
func LineComment(text string) *Comment {
	return &Comment{
		Texts:    []string{text},
		Position: CommentLinePosition,
	}
}

// HeadComment returns a comment to write above its value, one line per text.
func HeadComment(texts ...string) *Comment {
	return &Comment{
		Texts:    texts,
		Position: CommentHeadPosition,
	}
}

// FootComment returns a comment to write below its value, one line per text.
func FootComment(texts ...string) *Comment {
	return &Comment{
		Texts:    texts,
		Position: CommentFootPosition,
	}
}

// Comment is the text of a comment and where it goes, apart from any document.
//
// It carries what [ast.CommentGroupNode] carries -- one line of text per entry
// in Texts -- plus the [CommentPosition] the tree drops. Build one with
// [HeadComment], [LineComment] or [FootComment] rather than by hand, so that
// the texts and the position agree.
//
// Nothing here addresses a document. A Comment says what to write and whether
// it goes above, beside or below; [CommentMap] says which value it belongs to.
type Comment struct {
	// Texts is one line of comment per entry, written without the '#'.
	Texts []string
	// Position places the comment relative to its value.
	Position CommentPosition
}

// CommentMap holds the comments of a document, against the path of the value
// each belongs to.
//
// A key is a YAML path as [PathString] parses it -- "$.foo.bar", "$.baz[1]".
// [CommentToMap] fills one in while decoding, taking each key from the node's
// own path, and [WithComment] reads one while encoding. So a map produced by
// decoding is a map the encoder can write back.
//
// A value may carry more than one comment: a head comment and a line comment
// stand in different places and are two entries under the same key.
type CommentMap map[string][]*Comment
