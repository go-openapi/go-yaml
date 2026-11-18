// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package excerpt draws the lines of a document around a token, which is what
// an error prints under its message.
package excerpt

import (
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// The ANSI escapes a colored excerpt uses. The line number is bold bright
// white, and the message bright red.
const (
	lineNumberOn  = "\x1b[1;97m"
	lineNumberOff = "\x1b[22;0m"
	messageOn     = "\x1b[91m"
	messageOff    = "\x1b[0m"
)

// Message returns msg, in red when colored is set.
func Message(msg string, colored bool) string {
	if !colored {
		return msg
	}

	return messageOn + msg + messageOff
}

// Source draws the lines of src around tk, with tk's line marked and a caret
// under its column.
//
// src is the document the token was read from, or a window of it; firstLine is
// the line number src starts at. The lines come from the document itself rather
// than from the tokens read out of it, so what a reader sees under an error is
// what they wrote.
func Source(src string, firstLine int, tk *token.Token, colored bool) string {
	const context = 3

	lines := splitLines(src)
	if n := len(lines); n > 0 && lines[n-1] == "" {
		// A document ending in a line break has no line after it.
		lines = lines[:n-1]
	}

	errLine := int(tk.Position.Line)
	// The token carries how far it reaches: the breaks from its first
	// character to the end of the gap after it, and whether that gap opens on
	// a break. Both were counted from its source text until the text stopped
	// being kept.
	lastLine := errLine + int(tk.BreaksAfterLeading())
	if tk.TrailingBreaks() > 0 {
		lastLine--
	}

	from := max(errLine-context, firstLine)
	to := min(lastLine+context, firstLine+len(lines)-1)
	if from > to {
		return ""
	}
	// A window opening on blank lines shows nothing of the document. Start it
	// where the text does. The index is checked as well as errLine: a token
	// carrying a line past the end of the text would otherwise walk off it.
	for from < errLine && from-firstLine < len(lines) && strings.TrimSpace(lines[from-firstLine]) == "" {
		from++
	}

	var out strings.Builder
	prefixLen := len(fmt.Sprintf("  %2d | ", errLine))
	for num := from; num <= to; num++ {
		line := lines[num-firstLine]
		if num == to {
			// Trailing space on the last line of the window draws nothing and
			// leaves the caret hanging past the text.
			line = strings.TrimRight(line, " \t")
		}
		out.WriteString(lineNumber(num, errLine, colored))
		out.WriteString(line)
		out.WriteString("\n")
		// The caret follows the last line the token covers, not the first: a
		// token spanning lines is read to its end before anything is wrong with
		// it, and what is wrong is usually what should have come next.
		if num == lastLine {
			out.WriteString(strings.Repeat(" ", prefixLen+int(tk.Position.Column)-1))
			out.WriteString("^\n")
		}
	}

	// The line break after the caret is kept where the caret ends the window:
	// it separates the mark from whatever the caller writes next.
	if lastLine >= to {
		return out.String()
	}

	return strings.TrimSuffix(out.String(), "\n")
}

// lineNumber is the margin of line num, marked when it is the line in error.
func lineNumber(num, errLine int, colored bool) string {
	margin := fmt.Sprintf("  %2d | ", num)
	if num == errLine {
		margin = fmt.Sprintf("> %2d | ", num)
	}
	if !colored {
		return margin
	}

	return lineNumberOn + margin + lineNumberOff
}

// splitLines cuts src where the scanner counts a line break: "\r\n", "\r" or
// "\n".
//
// Splitting on "\n" alone gives fewer lines than the scanner counted, so a
// token's Position.Line points past the end of the result and the window drawn
// around it reads a line that is not there. "\r\r\r\r0\n " is five lines to the
// scanner and two to strings.Split.
func splitLines(src string) []string {
	lines := make([]string, 0, strings.Count(src, "\n")+strings.Count(src, "\r")+1)

	var start int
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			lines = append(lines, src[start:i])
			start = i + 1
		case '\r':
			lines = append(lines, src[start:i])
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}

	return append(lines, src[start:])
}
