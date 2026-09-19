// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import "testing"

// TestHexPointRejectsCodePointsAboveMaxRune pins the bound hexPoint applies
// before converting to a rune.
//
// literalBytesOfTheGrammar calls hexPoint and keeps the result when it is
// below 0x80. Without the bound, "xFFFFFFFF" parses to 4294967295, converts to
// -1, passes that test and adds byte 255 to the alphabet the mutation catalog
// draws from.
func TestHexPointRejectsCodePointsAboveMaxRune(t *testing.T) {
	tests := []struct {
		spelling string
		want     rune
		ok       bool
	}{
		{spelling: "x41", want: 'A', ok: true},
		{spelling: "x10FFFF", want: 0x10FFFF, ok: true},
		{spelling: "x110000"},
		{spelling: "xFFFFFFFF"},
		{spelling: "xZZ"},
		{spelling: "x4"},
		{spelling: "41"},
	}
	for _, test := range tests {
		t.Run(test.spelling, func(t *testing.T) {
			got, ok := hexPoint(test.spelling)
			if ok != test.ok {
				t.Fatalf("expected hexPoint(%q) to report %v, but it reports %v", test.spelling, test.ok, ok)
			}
			if got != test.want {
				t.Fatalf("expected hexPoint(%q) to decode to %U, but it decodes to %U", test.spelling, test.want, got)
			}
		})
	}
}

// TestHexEscapeDecodesFourHexDigits covers the width hexEscape parses at. A
// \uXXXX escape is exactly four digits, so every value it accepts fits in a
// rune without a further check.
func TestHexEscapeDecodesFourHexDigits(t *testing.T) {
	tests := []struct {
		name string
		s    string
		i    int
		want rune
		ok   bool
	}{
		{name: "ascii", s: `\u0041`, want: 'A', ok: true},
		{name: "the widest value four digits hold", s: `\uFFFF`, want: 0xFFFF, ok: true},
		{name: "a lone surrogate, which the caller pairs up", s: `\uD83D`, want: 0xD83D, ok: true},
		{name: "at an offset", s: `ab\u0041`, i: 2, want: 'A', ok: true},
		{name: "not hex", s: `\uZZZZ`},
		{name: "too short", s: `\u00`},
		{name: "not an escape", s: `u0041ab`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := hexEscape(test.s, test.i)
			if ok != test.ok {
				t.Fatalf("expected hexEscape(%q, %d) to report %v, but it reports %v", test.s, test.i, test.ok, ok)
			}
			if got != test.want {
				t.Fatalf("expected hexEscape(%q, %d) to decode to %U, but it decodes to %U", test.s, test.i, test.want, got)
			}
		})
	}
}
