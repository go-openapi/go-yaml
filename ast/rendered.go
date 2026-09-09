// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"
	"strings"
)

// rendered is a piece of output that has not been written out yet: a leaf's
// text, or several pieces to be joined, with an indentation that applies to
// every line of it.
//
// It exists because joining rendered children into a string, and then indenting
// that string by splitting it into lines again, copies every byte once per level
// of nesting it sits under. A document nested d deep cost O(bytes x d), which
// for a nested document is the square of its length: 1.6 MB rendered in 0.98 s
// and allocated 2.9 GB. Holding the pieces and writing them once costs the
// output, and indenting a subtree is a number rather than a pass over its text.
//
// spans and leads are what the layout decisions used to ask the finished string:
// whether a value renders over more than one line, and whether it opens with a
// line break. They are kept as pieces are put together, so the question is
// answered before anything is written rather than after. zz_rendered_test.go
// holds both against the text the piece flattens to.
type rendered struct {
	// text is a leaf's own text. It is empty for a piece holding parts.
	text string
	// parts are written one after another with sep between them, exactly as
	// strings.Join would.
	parts []rendered
	sep   sepKind
	// indent is added to every line of this piece that has content. A blank
	// line takes none, so nothing gains trailing spaces.
	indent int32
	// hanging says the first line of this piece takes no indentation, wherever
	// it falls. A sequence entry and a "?" key are written after a marker that
	// already holds their first line, and the marker's own line break is not
	// theirs to indent under.
	hanging bool
	// size is what this piece flattens to, indentation excluded, so that an
	// empty piece can be told from one holding a line break.
	size int32
	// spans says the piece covers more than one line, leads that it opens on a
	// line break.
	spans bool
	leads bool
}

// sepKind is what goes between the parts of a join. The renderer uses three
// separators and nothing else, so one byte says which rather than the sixteen a
// string header costs on every piece.
type sepKind uint8

const (
	// sepNone puts nothing between the parts.
	sepNone sepKind = iota
	// sepBreak puts a line break between them, which is how a mapping's entries
	// and a document's parts are laid out.
	sepBreak
	// sepSpace puts a space between them, for a comment written after a value.
	sepSpace
)

func (s sepKind) text() string {
	switch s {
	case sepBreak:
		return "\n"
	case sepSpace:
		return " "
	default:
		return ""
	}
}

// leaf holds text that is already final.
func leaf(text string) rendered {
	return rendered{
		text:  text,
		size:  int32(len(text)),
		spans: strings.Contains(text, "\n"),
		leads: strings.HasPrefix(text, "\n"),
	}
}

// join writes the parts one after another with sep between them.
func join(sep sepKind, parts ...rendered) rendered {
	out := rendered{parts: parts, sep: sep}

	if sep == sepNone {
		// Nothing separates the parts, so a part that writes nothing changes
		// nothing. Dropping them here is most of what a piece holds: a plain
		// "k: v" entry is built from seven parts and four of them are the
		// comments and the blank line it does not have.
		parts = withoutEmpty(parts)
		out.parts = parts
	}

	isBreak := sep == sepBreak
	var leadSet bool
	for i, part := range parts {
		if i > 0 {
			out.size += int32(len(sep.text()))
			out.spans = out.spans || isBreak
			if !leadSet && sep != sepNone {
				out.leads = isBreak
				leadSet = true
			}
		}
		if !leadSet && part.size > 0 {
			out.leads = part.leads
			leadSet = true
		}
		out.spans = out.spans || part.spans
		out.size += part.size
	}
	return out
}

// withoutEmpty returns the parts that write something, and parts itself where
// they all do.
func withoutEmpty(parts []rendered) []rendered {
	// Filtered in place: parts is the variadic slice, which belongs to this
	// call and nothing else holds. Building a second one traded a wide slice
	// for a narrow slice and an allocation, which is worse than the width.
	out := parts[:0]
	for _, part := range parts {
		if part.size > 0 {
			out = append(out, part)
		}
	}

	return out
}

// indentedBy shifts every line of a piece one or more levels deeper, without
// looking at its text.
func (p rendered) indentedBy(spaces int) rendered {
	return p.shifted(spaces, false)
}

// hangingBy shifts every line but the first, for a piece written after a marker
// that already occupies the line it starts on.
func (p rendered) hangingBy(spaces int) rendered {
	return p.shifted(spaces, true)
}

func (p rendered) shifted(spaces int, hanging bool) rendered {
	if spaces <= 0 || p.size == 0 {
		return p
	}
	// Wrapped rather than added to, so that a piece already carrying an indent
	// keeps it and the two accumulate as the write descends.
	return rendered{
		parts:   []rendered{p},
		indent:  int32(spaces),
		hanging: hanging,
		size:    p.size,
		spans:   p.spans,
		leads:   p.leads,
	}
}

// withoutLead returns the piece without the line break it opens with.
//
// A blank line an author left above an entry surfaces as a leading break on the
// entry's own text, and a sequence writes it above the "-" rather than after it.
// Taking it off is a structural cut where it can be -- the piece holding the
// break is found and only that one is rebuilt -- and a flatten where the break
// is a separator between two pieces, which a mapping and a sequence never
// produce as their first line.
func (p rendered) withoutLead() rendered {
	if !p.leads {
		return p
	}
	if p.parts == nil {
		out := leaf(strings.TrimPrefix(p.text, "\n"))
		out.indent = p.indent

		return out
	}

	for i, part := range p.parts {
		if part.size == 0 {
			if i > 0 && p.sep == sepBreak {
				break
			}

			continue
		}
		parts := make([]rendered, len(p.parts))
		copy(parts, p.parts)
		parts[i] = part.withoutLead()
		out := join(p.sep, parts...)
		out.indent = p.indent

		return out
	}

	return leaf(strings.TrimPrefix(p.string(), "\n"))
}

// empty reports whether the piece writes nothing at all.
func (p rendered) empty() bool { return p.size == 0 }

// string flattens the piece.
func (p rendered) string() string {
	var b strings.Builder
	b.Grow(int(p.size))
	w := lineWriter{w: &b}
	p.writeTo(&w, 0)

	return b.String()
}

// writeTo writes the piece, putting indent in after each line break rather than
// splitting a finished block into lines.
func (p rendered) writeTo(w *lineWriter, indent int) {
	indent += int(p.indent)
	if p.hanging {
		// The marker that introduced this piece holds its first line, so that
		// line takes no indentation however it was reached.
		w.atLineStart = false
	}
	if p.parts == nil {
		w.writeString(p.text, indent)

		return
	}
	for i, part := range p.parts {
		if i > 0 {
			w.writeString(p.sep.text(), indent)
		}
		part.writeTo(w, indent)
	}
}

// lineWriter writes text with an indentation that is put in lazily, at the
// first character of a line that has content.
//
// Lazily is what makes it match indentLines, which pads every line it is given
// except the empty ones: a blank line gains no trailing spaces, and a piece
// written after "- " keeps its first line where the marker left it.
type lineWriter struct {
	w io.Writer
	// atLineStart says a line break was written and nothing has followed it.
	atLineStart bool
	err         error
}

func (w *lineWriter) writeString(s string, indent int) {
	if w.err != nil || s == "" {
		return
	}

	for len(s) > 0 {
		cut := strings.IndexByte(s, '\n')
		if cut < 0 {
			w.pad(indent)
			w.write(s)

			return
		}
		if cut > 0 {
			w.pad(indent)
			w.write(s[:cut])
		}
		w.write("\n")
		w.atLineStart = true
		s = s[cut+1:]
	}
}

// pad writes the indentation a line opens with, once.
func (w *lineWriter) pad(indent int) {
	if !w.atLineStart || indent <= 0 {
		w.atLineStart = false

		return
	}
	w.atLineStart = false
	w.write(strings.Repeat(" ", indent))
}

func (w *lineWriter) write(s string) {
	if w.err != nil {
		return
	}
	_, w.err = io.WriteString(w.w, s)
}
