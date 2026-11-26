// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// TestABlockOfSpacesStartsAtItsFirstContentSpace checks the offset of a block
// scalar whose content is nothing but spaces past the indentation, cut where a
// less indented line opens.
//
// The offset was counted back from the cursor, which had read that line's
// indentation by then, and the value -- a run of spaces -- matched inside it.
// "|6-" over a 13-space line over "      # c" put its content on the comment's
// line, so a rendering that removed the comment could not take the line whole
// and the next entry moved two columns.
func TestABlockOfSpacesStartsAtItsFirstContentSpace(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int32
	}{
		// The 13th space of line 3 stands at offset 27, past the 12 columns of
		// indentation "- |6-" at column 7 asks for.
		{src: "k:\n      - |6-\n" + strings.Repeat(" ", 13) + "\n\n\n      # c\n      - x\n", want: 27},
		{src: "k:\n      - |6-\n" + strings.Repeat(" ", 13) + "\n      # c\n      - x\n", want: 27},
		{src: "k:\n  - |6-\n" + strings.Repeat(" ", 9) + "\n\n\n  # c\n  - y\n", want: 19},
		// A line that opens at column 1 never moved it.
		{src: "- |1-\n  \n\n- x\n", want: 7},
	} {
		tokens, err := scanTokens(tc.src)
		require.NoErrorf(t, err, "%q", tc.src)

		var content *token.Token
		for i := range tokens {
			if tokens[i].Type == token.StringType && tokens[i].Value == " " {
				content = &tokens[i]

				break
			}
		}
		require.NotNilf(t, content, "%q holds a block of spaces", tc.src)
		assert.Equalf(t, tc.want, content.Position.Offset(), "%q", tc.src)
		assert.Equalf(t, " ", tc.src[tc.want:tc.want+1], "the offset names a space of %q", tc.src)
	}
}

// TestABlockOfEmptyLinesCoversThem checks the extent of a block scalar's content when its value is blank: the empty
// lines a "|+" keeps, or the spaces past the indentation. The token covers those lines, from the first byte after the
// header's line, or from the first content space, to past the last line break the value keeps.
//
// Measured from its origin, such content covered nothing, and at the end of the stream it stood on the last break:
// "- |+" over two empty lines read "\n\n" from a token of no width. Where the document breaks its lines with "\r\n"
// or "\r", the value was not found in the source and the token fell to column 0. A transform saw the lines as filler.
func TestABlockOfEmptyLinesCoversThem(t *testing.T) {
	for _, tc := range []struct {
		src          string
		value        string
		offset, end  int32
		line, column int32
	}{
		{src: "- |+\n\n\n", value: "\n\n", offset: 5, end: 7, line: 2, column: 1},
		{src: "a: |+\n\n\n", value: "\n\n", offset: 6, end: 8, line: 2, column: 1},
		{src: "a: |+\n\n\nb: 1\n", value: "\n\n", offset: 6, end: 8, line: 2, column: 1},
		{src: "a: |+\r\n\r\n\r\nb: 1\n", value: "\n\n", offset: 7, end: 11, line: 2, column: 1},
		{src: "a: |+\r\rb: 1\r", value: "\n", offset: 6, end: 7, line: 2, column: 1},
		{src: "x:\n  a: |+\n\n  b: 1\n", value: "\n", offset: 11, end: 12, line: 3, column: 1},
		// A tab at column 1 is content of a root block scalar, and a blank one begins at it as at a space. Where no break
		// ends the source, the token runs to its end: "|" over an empty line and "\t " ended at offset 3 and began at 4,
		// and slicing its extent panicked. With no space, or with a break ending the source, the token begins at the
		// first byte after the header's line.
		{src: "|\n\n\t ", value: "\n\t \n", offset: 3, end: 5, line: 3, column: 1},
		{src: ">\n\n\t ", value: "\n\t \n", offset: 3, end: 5, line: 3, column: 1},
		{src: "|\n\n\t", value: "\n\t\n", offset: 2, end: 4, line: 2, column: 1},
		{src: "|\n\t ", value: "\t \n", offset: 2, end: 4, line: 2, column: 1},
		{src: "|\n\n\t \n", value: "\n\t \n", offset: 2, end: 6, line: 2, column: 1},
	} {
		tokens, err := scanTokens(tc.src)
		require.NoErrorf(t, err, "%q", tc.src)

		var content *token.Token
		for i := 1; i < len(tokens); i++ {
			if header := tokens[i-1].Type; tokens[i].Type == token.StringType && (header == token.LiteralType || header == token.FoldedType) {
				content = &tokens[i]

				break
			}
		}
		require.NotNilf(t, content, "%q holds a block scalar", tc.src)
		assert.Equalf(t, tc.value, content.Value, "%q", tc.src)
		assert.Equalf(t, []int32{tc.offset, tc.end, tc.line, tc.column},
			[]int32{content.Position.Offset(), content.EndOffset(), content.Position.Line, content.Position.Column}, "%q", tc.src)
	}
}
