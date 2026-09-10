// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
)

// TestFirstUnprintableMatchesTheByteLoop holds the word-at-a-time scan to the
// byte loop, on the documents people write and on text built to sit on the
// seams: a bad byte at each offset of a word, at the last word's edge, and past
// a multi-byte character, which sends the scan back to a byte at a time and
// then into the word loop again.
//
//nolint:gosmopolitan // using non-latin runes is the purpose of this test.
func TestFirstUnprintableMatchesTheByteLoop(t *testing.T) {
	t.Run("both versions of firstUnprintable", func(t *testing.T) {
		t.Run("should agree on the analysis workload", func(t *testing.T) {
			for _, doc := range testscanner.WorkloadDocs(t) {
				same(t, doc.Text)
			}
		})

		t.Run("should agree to detect an unprintable byte with and without multi-byte char in front", func(t *testing.T) {
			// A byte a stream may not hold, at every offset of a run long enough to cross several words, with and without a
			// multi-byte character in front of it so that the scan reaches it both ways.
			for _, bad := range []string{"\x00", "\x01", "\x0b", "\x0c", "\x1f", "\x7f", "\x80", "\xff", "\xe0\xa0"} {
				for _, prefix := range []string{"", "é", "aé", "日本語 "} {
					for at := range 24 {
						text := prefix + strings.Repeat("a", at) + bad + strings.Repeat("b", 24-at)
						same(t, text)
					}
				}
			}
		})

		t.Run("should agree and report proper offset", func(t *testing.T) {
			// Text a stream may hold, at the same offsets, so that a false alarm shows up as loudly as a missed one.
			for _, good := range []string{"\t", "\n", "\r", "\r\n", " ", "~", "é", "日", "\U0001F600"} {
				for at := range 24 {
					text := strings.Repeat("a", at) + good + strings.Repeat("b", 24-at)
					same(t, text)
					require.Equalf(t, -1, firstUnprintable(text), "%q holds nothing a stream may not", text)
				}
			}
		})

		t.Run("should agree on edge cases", func(t *testing.T) {
			same(t, "")
			same(t, "\x00")
			same(t, strings.Repeat("a", 7)+"\x00")
			same(t, strings.Repeat("a", 8)+"\x00")
		})
	})
}

// firstUnprintableByByte is the loop firstUnprintable replaced: one byte at a time, decoding a rune wherever the byte
// is not ASCII.
//
// It is kept as the reference the word-at-a-time scan is held to.
func firstUnprintableByByte(text string) int {
	for i := 0; i < len(text); {
		if c := text[i]; c < utf8.RuneSelf {
			if !printableASCII(c) {
				return i
			}
			i++

			continue
		}

		r, width := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && width <= 1 {
			return i
		}
		if !printable(r) {
			return i
		}
		i += width
	}

	return -1
}

func same(t *testing.T, text string) {
	t.Helper()
	assert.Equalf(t, firstUnprintableByByte(text), firstUnprintable(text),
		"the two disagree on %q",
		text,
	)
}
