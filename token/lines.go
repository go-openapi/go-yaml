// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// Lookback derives two facts about each token from the tokens read before it: whether the author left a blank
// line above it, and how many line breaks the comments above it take up.
//
// Both depend only on the last two tokens, on whether the scalar before those is a block header's content, and on
// a running count of the breaks in the comments just read. Lookback keeps those four and updates them as each
// token goes past, so a stream is read once, forward, and no token already emitted is held or read again.
type Lookback struct {
	// prev and prev2 are copies, not the tokens themselves: a scanner holding
	// its tokens in a slice of values reuses the room a token stood in as soon
	// as the token has been handed over.
	prev     Token
	prev2    Token
	hasPrev  bool
	hasPrev2 bool

	// blockHeader records, for the prefix ending one and two tokens back,
	// whether it ends on a literal or folded header once the comments between
	// are skipped. Index 0 is the prefix through prev, 1 the prefix before
	// prev, 2 the prefix before prev2.
	blockHeader [3]bool

	// commentBreaks counts the line breaks taken by the run of comments read
	// since the last token that was not a comment.
	commentBreaks int32
}

// Derive sets tk.BlankLineAbove() and tk.CommentBreaksAbove() from the tokens l has
// read, then reads tk itself.
//
// The first token of a stream keeps the zero values: nothing stands above it.
func (l *Lookback) Derive(tk *Token) {
	if tk == nil {
		return
	}
	if l.hasPrev {
		tk.SetBlankLineAbove(l.blankLineAbove(tk))
		tk.SetCommentBreaksAbove(l.commentBreaksAbove(tk))
	}
	l.read(tk)
}

// Reset drops what l has read, so it starts again on a fresh stream.
func (l *Lookback) Reset() {
	*l = Lookback{}
}

// read moves tk into l's state, shifting out the token that falls off the end.
func (l *Lookback) read(tk *Token) {
	l.blockHeader[2] = l.blockHeader[1]
	l.blockHeader[1] = l.blockHeader[0]
	if tk.Type == CommentType {
		// A comment stands between a header and its content without separating
		// them, so it neither makes nor breaks a block header, and its breaks
		// add to the run above whatever comes next.
		l.commentBreaks += l.originBreaksOf(tk)
	} else {
		l.blockHeader[0] = tk.Type == LiteralType || tk.Type == FoldedType
		l.commentBreaks = 0
	}
	l.prev2, l.hasPrev2 = l.prev, l.hasPrev
	l.prev, l.hasPrev = *tk, true
}

// blankLineAbove reports whether the author left an empty line above t.
func (l *Lookback) blankLineAbove(t *Token) bool {
	prev := &l.prev
	// blockHeader for the prefix before prev, which records whether prev is block scalar content.
	header := l.blockHeader[1]

	var adjustment int32
	// A sequence entry's '-' opens no gap of its own: the gap the author left is above the '-', so the comparison
	// steps back past it. The lines between
	// the '-' and t are then the entry's own layout --
	// -
	//   b: c
	// -- and not part of that gap.
	//
	// It steps back only where t stands inside the entry. An empty entry is followed by a token belonging to
	// whatever encloses it, and stepping back would give that token the gap written above the '-': ": &1" over a
	// blank over "-" over "? \"\"" would report a blank line above the '?', which has none, and none above the
	// second '-' of "-" over a blank over "- a", which has one.
	if prev.Type == SequenceEntryType && standsInsideEntry(prev, t) {
		adjustment = t.Position.Line - prev.Position.Line
		if l.hasPrev2 {
			prev = &l.prev2
			header = l.blockHeader[2]
		}
	}

	lineDiff := t.Position.Line - prev.Position.Line - 1
	if lineDiff <= 0 {
		return false
	}

	switch prev.Type {
	case StringType, SingleQuoteType, DoubleQuoteType:
		// A scalar may span lines: a quoted one written across two, a block one
		// whose content is several, and the blank lines a "|+" keeps. Those
		// lines are the scalar's own, and none of them is a gap the author left
		// above t.
		adjustment += int32(linesSpannedBy(prev, header))
	case NullType, ImplicitNullType:
		// A null counts a gap only where its own source text holds a line break. Otherwise a comment between the
		// null and t is written back on the null's line, and the lines it took are no blank line:
		// foo:
		//  bar:
		//  # comment
		//  baz: 1
		// becomes
		// foo:
		//  bar: null # comment
		//
		//  baz: 1
		return l.originBreaksBefore(prev) > 0
	default:
		// nothing to do
	}

	return lineDiff-adjustment > 0
}

// standsInsideEntry reports whether t belongs to the sequence entry dash opens.
// An entry's content shares the dash's line or is indented past it; anything at
// the dash's own column or to the left of it closes the entry.
func standsInsideEntry(dash, t *Token) bool {
	return t.Position.Line == dash.Position.Line || t.Position.Column > dash.Position.Column
}

// commentBreaksAbove counts the line breaks the comments written immediately
// above tk take up.
//
// A comment token carries its own line break in its origin. Where the comments
// are dropped -- a node rendered without them -- those breaks have to be
// written back, or the line after a comment runs into the line before it.
func (l *Lookback) commentBreaksAbove(tk *Token) int32 {
	if tk.Type == CommentType {
		return 0
	}

	return l.commentBreaks
}

// linesSpannedBy returns how many lines past its first the scalar tk occupies.
// isContent reports whether tk holds the content of a literal or folded block.
//
// EndLine counts the lines the scalar has content on, recorded when the token was built: the breaks inside the
// token, without the whitespace around it.
//
// A block scalar reaches past EndLine by the blank lines its chomping indicator
// keeps, and its value records those. Every trailing break past the one ending
// the last line of content stands for a blank line the scalar is written on, so
// ">+" and "|+" reach that many lines further down the document.
//
// The value's breaks measure the source only for a literal block. Folding rewrites the line structure --
// "  x\n\n  y\n" folds to "x\ny\n" -- so a folded scalar's value holds fewer breaks than its source has lines.
func linesSpannedBy(tk *Token, isContent bool) int {
	lines := int(tk.EndLine() - tk.Position.Line)
	if !isContent {
		return lines
	}

	return lines + max(TrailingBreaksIn(tk.Value)-1, 0)
}

// originBreaksBefore counts the breaks in the whole of prev's source text. It reads the token before prev, where
// prev's text begins.
func (l *Lookback) originBreaksBefore(prev *Token) int32 {
	if !l.hasPrev2 {
		return prev.BreaksAfterLeading()
	}

	return prev.EndLine() - l.prev2.EndLine() - l.prev2.TrailingBreaks() + prev.TrailingBreaks()
}

// originBreaksOf counts the line breaks in the whole of tk's source text, the
// whitespace before and after it included.
//
// The tokens' texts follow one another with nothing between them, so the breaks
// from where the token before it ended to where it ends are
// EndLine(tk) - EndLine(prev); take off the breaks that belonged to the gap
// after prev and add the ones in tk's own trailing gap.
func (l *Lookback) originBreaksOf(tk *Token) int32 {
	if !l.hasPrev {
		return tk.BreaksAfterLeading()
	}

	return tk.EndLine() - l.prev.EndLine() - l.prev.TrailingBreaks() + tk.TrailingBreaks()
}
