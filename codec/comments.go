// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

// CommentPosition gives the place of a comment relative to its value: above it, on its line, or below it.
//
// An [ast.CommentGroupNode] holds the text of a run of comments, but not its place. The place follows from the node
// the parser attaches the group to, and the tree does not record it. A caller that reads comments out of a document,
// or writes them into one, uses CommentPosition to give the place.
type CommentPosition int

const (
	// CommentHeadPosition puts the comment on the lines above its value.
	CommentHeadPosition CommentPosition = CommentPosition(iota)
	// CommentLinePosition puts the comment on its value's line, after the value.
	CommentLinePosition
	// CommentFootPosition puts the comment on the lines below its value.
	CommentFootPosition
)

// String returns the name of the position: "Head", "Line" or "Foot".
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

// Comment holds the text of a comment and its place relative to its value, outside any document.
//
// Texts holds what an [ast.CommentGroupNode] holds, one line of text per entry, and Position adds the
// [CommentPosition] that the tree does not record. Build one with [HeadComment], [LineComment] or [FootComment], so
// that the texts and the position agree.
//
// A Comment does not address a value. A [CommentMap] files each Comment under the path of its value.
type Comment struct {
	// Texts is one line of comment per entry, written without the '#'.
	Texts []string
	// Position places the comment relative to its value.
	Position CommentPosition
}

// CommentMap holds the comments of a document, keyed by the path of the value each belongs to.
//
// A key is a YAML path, such as "$.foo.bar" or "$.baz[1]". [CommentToMap] fills a CommentMap while decoding, with each
// key taken from the node's own path, and [WithComment] reads one while encoding. So the encoder can write back a map
// the decoder filled.
//
// A value can carry several comments. A head comment and a line comment stand in different places, and are two
// entries under the same key.
type CommentMap map[string][]*Comment
