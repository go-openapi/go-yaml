// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"

	"github.com/go-openapi/go-yaml/token"
)

// WithSource gives a renderer the document a tree was parsed from, which is what
// [Renderer.Verbatim] writes back out.
//
// Rendering without it lays a tree out by its depth, which is what every other
// method here does and what an encoder wants. Rendering with it puts a document
// back the way it was written.
func WithSource(src []byte) RenderOption {
	return func(r *Renderer) { r.src = src }
}

// extent is the stretch of source a node covers: from the first token any leaf
// under it was cut from, to the end of the last.
//
// A node does not record it and does not need to. [Node.GetToken] is no help --
// a block mapping's token is the ":" of its first entry, which stands after its
// first key -- but the leaves have everything, so the extent is the union of
// theirs.
type extent struct{ from, to int32 }

var noExtent = extent{from: -1, to: -1}

func (e extent) found() bool { return e.from >= 0 }

func (e extent) union(other extent) extent {
	if !other.found() {
		return e
	}
	if !e.found() {
		return other
	}
	if other.from < e.from {
		e.from = other.from
	}
	if other.to > e.to {
		e.to = other.to
	}

	return e
}

// sourceExtent returns what n covers in the document it was parsed from, and
// reports nothing for a node built rather than read: a synthetic token carries
// a position the parser chose, which addresses no text.
func sourceExtent(n Node) extent {
	if n == nil {
		return noExtent
	}

	out := noExtent
	walkSourceTokens(n, func(tk *token.Token) {
		out = out.union(extent{from: tk.Position.Offset(), to: tk.EndOffset()})
	})

	return out
}

// walkSourceTokens hands over every token under n that was cut from a document,
// in the order the document wrote them.
//
// The scanner mints tokens in order and the parse builds nodes as it reads, so a
// descent follows the document. Two things it must not do: read a collection's
// own token, which is a delimiter from inside it, and follow a token the parser
// made up, whose position points wherever the parser put it -- an anchored empty
// node's implicit null points back at the "&" that opened it.
//
// It does not have to reach every token. A flow collection's commas hang on no
// node at all, and the comments hang beside one rather than in the descent;
// both are written out with the token that follows them, since the extents tile
// the document and nothing between two of them is lost.
func walkSourceTokens(n Node, fn func(*token.Token)) {
	if n == nil {
		return
	}

	hand := func(tk *token.Token) {
		if tk != nil && tk.FromSource() {
			fn(tk)
		}
	}

	switch v := n.(type) {
	case *DocumentNode:
		hand(v.Start)
		walkSourceTokens(v.Body, fn)
		hand(v.End)
	case *MappingNode:
		// Only a flow mapping has brackets of its own. A block mapping's Start
		// is the ":" its first entry hands over itself.
		if v.IsFlowStyle {
			hand(v.Start)
		}
		for _, entry := range v.Values {
			walkSourceTokens(entry, fn)
		}
		if v.IsFlowStyle {
			hand(v.End)
		}
	case *MappingValueNode:
		hand(v.CollectEntry)
		walkSourceTokens(v.Key, fn)
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *MappingKeyNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *SequenceNode:
		if v.IsFlowStyle {
			hand(v.Start)
		}
		for i, value := range v.Values {
			if !v.IsFlowStyle && i < len(v.Entries) && v.Entries[i] != nil {
				hand(v.Entries[i].Start)
			}
			walkSourceTokens(value, fn)
		}
		if v.IsFlowStyle {
			hand(v.End)
		}
	case *AnchorNode:
		hand(v.Start)
		walkSourceTokens(v.Name, fn)
		walkSourceTokens(v.Value, fn)
	case *AliasNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *TagNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *LiteralNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *DirectiveNode:
		hand(v.Start)
		walkSourceTokens(v.Name, fn)
		for _, value := range v.Values {
			walkSourceTokens(value, fn)
		}
	default:
		hand(n.GetToken())
	}
}

// verbatimWriter copies the source forward, never back.
type verbatimWriter struct {
	w      io.Writer
	src    []byte
	cursor int
	err    error
}

// upTo writes the source from where the last write stopped to end.
//
// Everything between two tokens goes out with the second of them: the
// indentation, the comments, a flow collection's commas, a "---". The extents
// tile the document, so copying forward to each token's end copies all of it.
func (vw *verbatimWriter) upTo(end int) {
	if vw.err != nil || end <= vw.cursor {
		return
	}
	if end > len(vw.src) {
		end = len(vw.src)
	}
	if vw.cursor < 0 {
		vw.cursor = 0
	}

	_, vw.err = vw.w.Write(vw.src[vw.cursor:end])
	vw.cursor = end
}

// Verbatim writes n as the document it was parsed from wrote it.
//
// It needs [WithSource]. A node the source does not reach -- one the encoder
// built, or a caller inserted -- writes nothing here yet; laying those out
// beside the text around them is the next piece of this.
//
// ⚠️ Provisional, and it is not what [Renderer.Render] does: that lays a tree
// out by its depth and is what an encoder wants.
func (r *Renderer) Verbatim(w io.Writer, n Node) error {
	if r.src == nil || n == nil {
		return nil
	}

	span := sourceExtent(n)
	if !span.found() {
		return nil
	}

	vw := &verbatimWriter{w: w, src: r.src, cursor: int(span.from)}
	walkSourceTokens(n, func(tk *token.Token) { vw.upTo(int(tk.EndOffset())) })
	vw.upTo(int(span.to))

	return vw.err
}

// VerbatimFile writes a whole file back as it was read, the text around its
// documents included.
func (r *Renderer) VerbatimFile(w io.Writer, f *File) error {
	if r.src == nil || f == nil {
		return nil
	}

	vw := &verbatimWriter{w: w, src: r.src}
	for _, doc := range f.Docs {
		walkSourceTokens(doc, func(tk *token.Token) { vw.upTo(int(tk.EndOffset())) })
	}
	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch has no token to be written with.
	vw.upTo(len(r.src))

	return vw.err
}
