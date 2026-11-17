// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package printer

import (
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// Property additional property set for each the token.
type Property struct {
	Prefix string
	Suffix string
}

// PrintFunc returns property instance.
type PrintFunc func() *Property

// Printer create text from token collection or ast.
type Printer struct {
	LineNumber       bool
	LineNumberFormat func(num int) string
	MapKey           PrintFunc
	Anchor           PrintFunc
	Alias            PrintFunc
	Bool             PrintFunc
	String           PrintFunc
	Number           PrintFunc
	Comment          PrintFunc
}

// PrintNode create text from ast.Node.
func (p *Printer) PrintNode(node ast.Node) []byte {
	return []byte(fmt.Sprintf("%+v\n", node))
}

func (p *Printer) setDefaultColorSet() {
	p.Bool = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiMagenta),
			Suffix: format(ColorReset),
		}
	}
	p.Number = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiMagenta),
			Suffix: format(ColorReset),
		}
	}
	p.MapKey = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiCyan),
			Suffix: format(ColorReset),
		}
	}
	p.Anchor = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiYellow),
			Suffix: format(ColorReset),
		}
	}
	p.Alias = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiYellow),
			Suffix: format(ColorReset),
		}
	}
	p.String = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiGreen),
			Suffix: format(ColorReset),
		}
	}
	p.Comment = func() *Property {
		return &Property{
			Prefix: format(ColorFgHiBlack),
			Suffix: format(ColorReset),
		}
	}
}

func (p *Printer) PrintErrorMessage(msg string, isColored bool) string {
	if isColored {
		return fmt.Sprintf("%s%s%s",
			format(ColorFgHiRed),
			msg,
			format(ColorReset),
		)
	}
	return msg
}

func (p *Printer) setupErrorTokenFormat(annotateLine int, isColored bool) {
	prefix := func(annotateLine, num int) string {
		if annotateLine == num {
			return fmt.Sprintf("> %2d | ", num)
		}

		return fmt.Sprintf("  %2d | ", num)
	}
	p.LineNumber = true
	p.LineNumberFormat = func(num int) string {
		if isColored {
			return colorize(prefix(annotateLine, num), ColorBold, ColorFgHiWhite)
		}

		return prefix(annotateLine, num)
	}
	if isColored {
		p.setDefaultColorSet()
	}
}

// PrintErrorSource draws the lines of src around tk, with tk's line marked and
// a caret under its column.
//
// src is the document the token was read from, or a window of it; firstLine is
// the line number src starts at. The lines come from the document itself rather
// than from the tokens read out of it, so what a reader sees under an error is
// what they wrote.
func (p *Printer) PrintErrorSource(src string, firstLine int, tk *token.Token, isColored bool) string {
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
	p.setupErrorTokenFormat(errLine, isColored)

	var out strings.Builder
	prefixLen := len(fmt.Sprintf("  %2d | ", errLine))
	for num := from; num <= to; num++ {
		line := lines[num-firstLine]
		if num == to {
			// Trailing space on the last line of the window draws nothing and
			// leaves the caret hanging past the text.
			line = strings.TrimRight(line, " \t")
		}
		out.WriteString(p.LineNumberFormat(num))
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
