// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"

	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/token"
)

// IndentState state for indent.
type IndentState int

const (
	// IndentStateEqual equals previous indent.
	IndentStateEqual IndentState = iota
	// IndentStateUp more indent than previous.
	IndentStateUp
	// IndentStateDown less indent than previous.
	IndentStateDown
	// IndentStateKeep uses not indent token.
	IndentStateKeep
)

func (s *Scanner) updateIndentState() {
	if s.lastDelimColumn == 0 || s.isFlowMode() {
		return
	}

	if s.lastDelimColumn < s.column {
		s.indentState = IndentStateUp

		return
	}

	// If lastDelimColumn and s.column are the same, treat as Down state since it is the same column as delimiter.
	s.indentState = IndentStateDown
}

func (s *Scanner) updateIndent(ctx *Context, c rune) {
	if s.isFirstCharAtLine && isNewLineChar(c) {
		return
	}
	if s.isFirstCharAtLine && c == ' ' {
		if probe.Enabled {
			name := "indent.indentNum==column-1/spaces"
			if s.indentHasTab {
				name = "indent.indentNum==column-1/tab"
			}
			probe.Check(name, s.indentNum == s.column-1, func() string {
				// The source around the cursor, for the probe report.
				const probeBytesBefore, probeBytesAfter = 24, 16
				from := max(ctx.idx-probeBytesBefore, 0)
				to := min(ctx.idx+probeBytesAfter, int32(len(ctx.src)))

				return fmt.Sprintf(
					"indentNum=%d column=%d line=%d idx=%d flow=%d/%d anchor=%v alias=%v directive=%v tab=%v around=%q",
					s.indentNum, s.column, s.line, ctx.idx,
					s.startedFlowSequenceNum, s.startedFlowMapNum,
					s.isAnchor, s.isAlias, s.isDirective, s.indentHasTab,
					ctx.src[from:to])
			})
		}
		s.indentNum++

		return
	}

	if s.isFirstCharAtLine && c == '\t' {
		// found tab indent.
		// In this case, scanTab returns error.
		s.indentHasTab = true
		return
	}

	if !s.isFirstCharAtLine {
		s.indentState = IndentStateKeep
		return
	}

	s.updateIndentState()
	s.isFirstCharAtLine = false
}

// indentHoldsATab reports whether a tab stood in this line's leading whitespace.
//
// s-indent(n) is spaces and nothing else, so a tab among a line's indentation
// leaves a block entry with nothing to sit on. A tab is separation rather than
// indentation, and is admitted where separation is: in front of a flow node or
// a scalar, and inside a flow collection.
//
// updateIndent sets the flag for a tab anywhere in the run, since it leaves
// isFirstCharAtLine standing, so "  \ta" and "\ta" both set it. Three callers
// each asked this question their own way before -- two by cutting the origin
// with strings.TrimPrefix(origin, " "), which trims one space and so read
// " \ta: 1" and "  \ta: 1" differently, and one by reading the flag. Ask here.
func (s *Scanner) indentHoldsATab() bool { return s.indentHasTab }

// tabStandsWhereAnEntryNeedsIndent reports whether a tab stands where a block
// entry needs s-indent(n).
//
// Two runs can hold it and they are not the same run. A tab among the line's
// own indentation is [Scanner.indentHoldsATab]; a tab after a token already cut
// on this line -- "- \ta: 1", where the '-' left isFirstCharAtLine false -- is
// in the separation since that token and is [Context.leadingBlanksHoldATab].
// Reading only the first admits "- \ta: 1", which the grammar refuses and
// go.yaml.in/yaml/v3 refuses with it; reading only the second admits
// "\t\"a\": 1", since a quoted key resets the origin buffer.
//
// A flow collection admits a tab in either place, so callers ask this together
// with [Scanner.isFlowMode]: "{\ta: 1}" is read by the grammar, by v3 and by
// libfyaml.
//
// The second run stands where an entry needs s-indent(n) only after a block
// indicator: "-", "?" or ":". After a key cut as a token of its own -- a quoted
// scalar, an alias, a flow collection or a merge key, as in "\"a\"\t: 1" -- the
// run holds only the blanks between the key and its ':', and a tab there
// separates, as it does after the plain key "a\t: 1", which keeps the tab in its
// own buffer. go.yaml.in/yaml/v3 reads "\"a\"\t: 1" as {a: 1}.
func (s *Scanner) tabStandsWhereAnEntryNeedsIndent(ctx *Context) bool {
	if s.indentHoldsATab() {
		return true
	}
	if !ctx.leadingBlanksHoldATab() {
		return false
	}

	last := ctx.lastToken()

	return last == nil || last.Position.Line != s.line || isBlockIndicator(last.Type)
}

// isBlockIndicator reports whether t is one of the indicators a block entry follows: "-", "?" or ":".
func isBlockIndicator(t token.Type) bool {
	switch t {
	case token.SequenceEntryType, token.MappingKeyType, token.MappingValueType:
		return true
	default:
		return false
	}
}

func (s *Scanner) isChangedToIndentStateDown() bool {
	return s.indentState == IndentStateDown
}

func (s *Scanner) isChangedToIndentStateUp() bool {
	return s.indentState == IndentStateUp
}

// checkFlowIndent rejects the line about to start when it is not indented past the line its flow collection opened on.
//
// This refuses "flow: [a,\nb]": "b" sits in the same column as the key owning the collection, so nothing marks it as
// part of that collection.
// Indentation is spaces, so a line led by a tab clears nothing.
//
// It runs from scanNewLine.
// A quoted or literal scalar spanning lines never reaches scanNewLine, its line breaks belonging to the scalar and not
// to the collection.
func (s *Scanner) checkFlowIndent(ctx *Context) error {
	if !s.isFlowMode() {
		return nil
	}

	indent, blank := lineIndent(ctx.src[ctx.idx+1:])
	if blank || indent > s.flowIndent {
		return nil
	}

	s.progressLine(ctx)

	return ErrInvalidToken("a flow collection continues on a line that is not indented past the one it started on", token.Invalid(ctx.origin(), s.pos()))
}

// contentIndent is the indentation a further line of the construct now being scanned has to clear.
func (s *Scanner) contentIndent() int32 {
	if s.isFlowMode() {
		return s.flowIndent
	}

	// The indentation of the block node this belongs to: the key or the '-' that introduced it.
	// Not the line the construct starts on, which may already be indented under that key.
	//
	// Zero means nothing introduced it: the construct is the document's own root, and its further lines have nothing to be
	// indented past.
	return s.lastDelimColumn - 1
}

// checkContinuationIndent rejects a further line of a quoted scalar that is not indented past the line the scalar
// started on.
//
// A scalar spanning lines is one value, and what marks its later lines as part of it is that they are indented under
// it.
// Without that, "quoted: \"a\nb\"" reads as a scalar and then a second, unrelated line.
func (s *Scanner) checkContinuationIndent(ctx *Context, rest string, base int32) error {
	indent, blank := lineIndent(rest)
	if blank || indent > base {
		return nil
	}

	return ErrInvalidToken("a scalar continues on a line that is not indented past the one it started on", token.Invalid(ctx.origin(), s.pos()))
}

// lineIndent returns how many spaces begin the line, and whether the line holds nothing else.
//
// A blank line is part of no indentation.
func lineIndent(src string) (int32, bool) {
	var indent int32
	for _, c := range src {
		switch c {
		case ' ':
			indent++
		case '\t':
			// A tab is whitespace but not indentation: it neither adds to the count nor ends the line.
		case '\n', '\r':
			return indent, true
		default:
			return indent, false
		}
	}

	return indent, true
}
