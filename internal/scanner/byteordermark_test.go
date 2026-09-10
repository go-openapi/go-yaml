// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
)

// bom is written as an escape because a byte order mark in Go source is one the compiler refuses: "illegal byte order
// mark".
const bom = "\ufeff"

// TestByteOrderMarkStandsOnlyInADocumentPrefix checks where U+FEFF may appear.
//
// nb-char excludes the mark, so no node may hold one. l-document-prefix ::= c-byte-order-mark? l-comment* is the only
// production that admits one, and l-yaml-stream places those prefixes at the start of the stream, after a document
// suffix, and before an explicit document.
//
// Every verdict below was taken from the recognizer compiled from yaml-spec-1.2.json, in
// internal/testintegration/grammar, and not read off the specification by hand.
// The two that the ledgers named are the first pair: "a: <mark>b" was read and is now refused, and "a: 1 / ... /
// <mark>--- / b: 2" was refused and is now read.
func TestByteOrderMarkStandsOnlyInADocumentPrefix(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ok   bool
	}{
		{name: "inside a line, holding content", src: "a: " + bom + "b\n"},
		{name: "after a document suffix, before ---", src: "a: 1\n...\n" + bom + "---\nb: 2\n", ok: true},

		{name: "opening the stream", src: bom + "a: 1\n", ok: true},
		{name: "opening the stream, twice", src: bom + bom + "---\na: 1\n", ok: true},
		{name: "opening the stream, before a comment", src: bom + "# c\n---\na: 1\n", ok: true},
		{name: "opening the stream, before a directive", src: bom + "%YAML 1.2\n---\na: 1\n", ok: true},

		{name: "before an explicit document, with no suffix", src: "a: 1\n" + bom + "---\nb: 2\n", ok: true},
		{name: "before a comment that stands before ---", src: "a: 1\n" + bom + "# c\n---\nb: 2\n", ok: true},
		{name: "before a blank line that stands before ---", src: "a: 1\n" + bom + "   \n---\nb: 2\n", ok: true},
		{name: "before a document suffix", src: "a: 1\n" + bom + "...\n", ok: true},
		{name: "after a document suffix, before a bare document", src: "a: 1\n...\n" + bom + "b: 2\n", ok: true},

		{name: "opening a mapping entry", src: "a: 1\n" + bom + "b: 2\n"},
		{name: "opening a sequence entry", src: "- a\n" + bom + "- b\n"},
		{name: "opening a line inside an explicit document", src: "---\n" + bom + "a: 1\n"},
		{name: "opening a line that ends a block scalar", src: "a: |\n  x\n" + bom + "b: 2\n"},
		{name: "alone on a line inside a document", src: "a: 1\n" + bom + "\nb: 2\n"},
		{name: "before a directive that may not stand there", src: "a: 1\n" + bom + "%YAML 1.2\n---\nb: 2\n"},
		{name: "inside a flow sequence", src: "[" + bom + "a]\n"},

		// nb-double-char and nb-single-char are built from nb-json, which is #x9 | [#x20-#x10FFFF] and takes the mark like
		// any other character.
		// So a quoted scalar holds one where a plain or block scalar may not.
		{name: "inside a double-quoted scalar", src: "a: \"x" + bom + "y\"\n", ok: true},
		{name: "inside a single-quoted scalar", src: "a: 'x" + bom + "y'\n", ok: true},
		{name: "the whole of a double-quoted scalar", src: "a: \"" + bom + "\"\n", ok: true},
		{name: "inside a quoted scalar in a flow sequence", src: "[\"a" + bom + "b\"]\n", ok: true},
		{name: "opening a quoted scalar's second line", src: "a: \"x\n" + bom + "y\"\n"},
		{name: "inside a block scalar", src: "a: |\n  x" + bom + "y\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := scanErr(test.src)
			if test.ok {
				assert.NoErrorf(t, err, "%q is YAML 1.2", test.src)

				return
			}
			assert.Errorf(t, err, "%q is not YAML 1.2", test.src)
		})
	}
}

// TestByteOrderMarkIsDroppedRatherThanRead checks that a mark a document prefix may carry leaves no trace in the
// tokens, so a file saved by an editor that writes a mark still reads.
func TestByteOrderMarkIsDroppedRatherThanRead(t *testing.T) {
	plain := scanAll(t, "a: 1\n...\n---\nb: 2\n")
	marked := scanAll(t, "a: 1\n...\n"+bom+"---\nb: 2\n")

	assert.Equal(t, len(plain), len(marked))
	for i := range plain {
		if i >= len(marked) {
			break
		}
		assert.Equalf(t, plain[i].Value, marked[i].Value, "token %d", i)
		assert.Equalf(t, plain[i].Position.Column, marked[i].Position.Column, "token %d column", i)
	}
}

// TestOffsetsCountAByteOrderMark checks that a mark the scanner steps over is still counted in the offsets after it.
//
// Init used to delete every mark before scanning, so a document carrying one was tokenized against a text the caller
// never wrote and every offset after the mark was three bytes short.
func TestOffsetsCountAByteOrderMark(t *testing.T) {
	tests := map[string]struct {
		src  string
		want map[string]int // token value -> byte offset in src
	}{
		"opening the stream": {
			src:  bom + "a: 1\n",
			want: map[string]int{"a": 3, ":": 4, "1": 6},
		},
		"opening a document after a suffix": {
			src:  "a: 1\n...\n" + bom + "---\nb: 2\n",
			want: map[string]int{"...": 5, "---": 12, "b": 16, "2": 19},
		},
		"none at all": {
			src:  "a: 1\n",
			want: map[string]int{"a": 0, ":": 1, "1": 3},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			seen := make(map[string]bool)
			for _, tk := range tokenize(t, test.src) {
				want, ok := test.want[tk.Value]
				if !ok {
					continue
				}
				seen[tk.Value] = true
				assert.Equalf(t, want, int(tk.Position.Offset()), "%q", tk.Value)

				// The offset addresses the source as it was handed in.
				require.LessOrEqual(t, int(tk.Position.Offset()), len(test.src))
				assert.Truef(t, strings.HasPrefix(test.src[tk.Position.Offset():], tk.Value),
					"offset %d does not address %q", tk.Position.Offset(), tk.Value)
			}
			for value := range test.want {
				assert.Truef(t, seen[value], "no token for %q", value)
			}
		})
	}
}

// scanErr drives the scanner to exhaustion and returns the first error that is not io.EOF.
func scanErr(src string) error {
	var s scanner.Scanner
	s.Init([]byte(src))

	for range maxScanCalls {
		if _, ok := s.NextToken(); !ok {
			return s.Err()
		}
	}

	return nil
}
