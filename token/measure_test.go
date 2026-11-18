// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// The four helpers MeasureOrigin replaced. Every Make call ran all of them, so
// each walked the origin over again -- three times over the leading blanks
// alone. They are kept here as the reference the fused version is held to.

func trimBlanks[T Text](org T) T {
	i, j := 0, len(org)
	for i < j && isBlank(org[i]) {
		i++
	}
	for j > i && isBlank(org[j-1]) {
		j--
	}

	return org[i:j]
}

func breaksIn[T Text](org T) int {
	body := trimBlanks(org)

	var n int
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\n':
			n++
		case '\r':
			n++
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
		}
	}

	return n
}

func trailingBreaksIn[T Text](org T) int {
	i := len(org)
	for i > 0 {
		switch org[i-1] {
		case ' ', '\t', '\r', '\n':
			i--
		default:
			return breaksInRaw(org[i:])
		}
	}

	return breaksInRaw(org)
}

func breaksInRaw[T Text](s T) int {
	var n int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			n++
		case '\r':
			n++
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		}
	}

	return n
}

func extentOfBlanks[T Text](org T, pos Position) int32 {
	i := 0
	for i < len(org) && (org[i] == ' ' || org[i] == '\t' || org[i] == '\n' || org[i] == '\r') {
		i++
	}

	return pos.Offset() + int32(len(org)-i)
}

// TestMeasureOriginMatchesTheHelpersItReplaced runs MeasureOrigin against the
// four helpers above, over every origin of up to five characters drawn from a
// space, a tab, a CR, an LF and a letter -- 3,906 of them, which covers a lone
// CR, a CR LF, a break inside the text, an origin that is nothing but blanks,
// and the empty origin.
//
// The alphabet has one letter because the helpers only ever ask whether a byte
// is blank, a break, or neither.
func TestMeasureOriginMatchesTheHelpersItReplaced(t *testing.T) {
	pos := Position{Line: 7, Column: 3}
	pos.SetOffset(41)

	const alphabet = " \t\r\na"

	var origins []string
	var grow func(prefix string, left int)
	grow = func(prefix string, left int) {
		origins = append(origins, prefix)
		if left == 0 {
			return
		}
		for i := range len(alphabet) {
			grow(prefix+alphabet[i:i+1], left-1)
		}
	}
	grow("", 5)
	require.Len(t, origins, 3906)

	origins = append(origins,
		"  key",
		"\n    key",
		"\r\n\r\n  key",
		"key\n  ",
		"'a\n b'\n",
		"\n \t \n",
	)

	for _, org := range origins {
		wantEnd := extentOfBlanks(org, pos)
		wantSpans := uint64(pos.Line+int32(breaksIn(org))) | uint64(trailingBreaksIn(org))&trailingMask<<trailingShift

		ext := MeasureOrigin(org, pos)
		assert.Equalf(t, wantEnd, ext.End, "end offset for %q", org)
		assert.Equalf(t, wantSpans, ext.spans(), "spans for %q: end line %d, trailing breaks %d",
			org, breaksIn(org), trailingBreaksIn(org))

		// The []byte instantiation reads the same origin the same way.
		assert.Equalf(t, ext, MeasureOrigin([]byte(org), pos), "extent for []byte(%q)", org)
	}
}
