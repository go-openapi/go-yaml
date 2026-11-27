// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"math"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// TestSourceLengthBound holds maxSourceLen at its edge.
//
// The refusal itself is out of a test's reach: it takes a source of a terabyte. That is why the bound is a
// predicate of its own, and not a comparison written inline in validateSource.
// An off-by-one is how this check fails, and a predicate can be read at any size.
//
// The stream end stands one past the last byte, at the length of the source, so the longest source puts an offset
// at maxSourceLen. A Position must give it back, with the widest indent beside it.
func TestSourceLengthBound(t *testing.T) {
	if math.MaxInt > math.MaxInt32 {
		assert.Equal(t, 1<<40-1, maxSourceLen)
	} else {
		assert.Equal(t, math.MaxInt, maxSourceLen)
	}
	assert.False(t, sourceTooLong(0))
	assert.False(t, sourceTooLong(1<<20))
	assert.False(t, sourceTooLong(maxSourceLen), "a source of exactly maxSourceLen bytes is read")
	assert.True(t, sourceTooLong(maxSourceLen+1))

	end := token.At(1, 1, maxSourceLen, sourceLines.indent)
	assert.Equal(t, maxSourceLen, end.Offset())
	assert.Equal(t, sourceLines.indent, end.IndentNum())
}

// TestSourceLinesFitAPosition checks that the production limits keep every line, column and indent inside the
// fields of a [token.Position].
func TestSourceLinesFitAPosition(t *testing.T) {
	assert.Equal(t, 1<<token.IndentBits-1, sourceLines.indent)
	// A column stands one past the last byte of its line.
	assert.Equal(t, math.MaxInt32, sourceLines.width+1)
	// The line after the last break is numbered breaks+1.
	assert.Equal(t, math.MaxInt32, sourceLines.breaks+1)

	pos := token.At(sourceLines.breaks+1, sourceLines.width+1, 0, sourceLines.indent)
	assert.EqualValues(t, math.MaxInt32, pos.Line)
	assert.EqualValues(t, math.MaxInt32, pos.Column)
	assert.Equal(t, sourceLines.indent, pos.IndentNum())
}

// TestLineLimitsAtTheirEdges reads each refusal of lineLimits one byte either side of its limit, under limits small
// enough to write the sources out.
func TestLineLimitsAtTheirEdges(t *testing.T) {
	limits := lineLimits{indent: 3, width: 5, breaks: 2}

	for _, c := range []struct {
		name, src, refusal string
	}{
		{name: "empty", src: ""},
		{name: "shorter than every limit", src: "ab"},

		{name: "a line at the width", src: "abcde\nx"},
		{name: "a line past the width", src: "abcdef\nx", refusal: "a line must be at most 5 bytes"},
		{name: "a last line past the width", src: "x\nabcdef", refusal: "the one at byte 2 is 6"},
		{name: "a CR LF left out of the width", src: "abcde\r\nx"},
		{name: "a lone CR ends a line", src: "abcd\rabcd"},
		{name: "a lone CR before a long line", src: "abcd\rabcdef", refusal: "the one at byte 5 is 6"},

		{name: "an indent at the limit", src: "   a"},
		{name: "an indent past the limit", src: "    a", refusal: "a line must open with at most 3 blanks"},
		{name: "tabs count in the indent", src: "\t \ta\n \t\t a", refusal: "the one at byte 5 opens with 4"},
		{name: "a blank line past the limit", src: "a\n    ", refusal: "opens with 4"},

		{name: "breaks at the limit", src: "a\nb\nc"},
		{name: "breaks past the limit", src: "a\nb\nc\n", refusal: "at most 2 line breaks, and this one holds 3"},
		{name: "a CR LF is one break", src: "a\r\nb\r\nc"},
		{name: "a lone CR is one break", src: "a\rb\rc\r", refusal: "holds 3"},
		{name: "an LF then a CR LF", src: "\n\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := limits.check(c.src)
			if c.refusal == "" {
				assert.NoError(t, err)

				return
			}
			assert.ErrorContains(t, err, c.refusal)
		})
	}
}

// TestValidateSourceRefusesAnIndentPast24Bits reaches the indent refusal through validateSource at the production
// limit: 16 MiB of spaces, the one refusal of the three a test can afford.
func TestValidateSourceRefusesAnIndentPast24Bits(t *testing.T) {
	indent := strings.Repeat(" ", sourceLines.indent)

	_, err := validateSource("a:\n" + indent + "b\n")
	require.NoError(t, err)

	_, err = validateSource("a:\n " + indent + "b\n")
	require.ErrorContains(t, err, "a line must open with at most 16777215 blanks")

	require.ErrorContains(t, validateJSONSource(" "+indent+"1"), "opens with 16777216")
}

// TestValidateSourceReadsAnOrdinarySource checks that the length gate hands an ordinary document on to the printable
// check instead of standing in front of it.
func TestValidateSourceReadsAnOrdinarySource(t *testing.T) {
	_, err := validateSource("a: 1\nb: [c, d]\n")
	require.NoError(t, err)
	_, err = validateSource("a: \x00\n")
	assert.ErrorContains(t, err, "a YAML stream may not hold")
}
