// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// extentLedger records the documents whose token extents do not tile the source.
//
// The extents are meant to follow one another with nothing between and nothing past the end, so
// src[previous end:this end] gives each token back as the document wrote it, indentation and all. That is the property
// TestOriginsTileTheSource asserts in internal/scanner, and the one github.com/go-openapi/go-yaml/transform stands on:
// its pieces tile the source, so a transform that changes nothing gives the document back byte for byte.
//
// Six documents of the 12,281 the parse accepts break it, and they break it in two ways:
//
//   - Five end a String token past the end of the document, by 9 to 45 bytes. Every one of them holds a multi-line
//     plain scalar, and "- single multiline\n - sequence entry\n" is the whole of the smallest: 37 bytes, and the
//     second token reports Offset() 21 and EndOffset() 56. Both ends come from the scalar's last line: the start is
//     wrong too, since the scalar begins at offset 2 and 21 is the "-" on the second line.
//   - One starts a token before the one before it ended, by one byte, writing its line breaks as "\r\n" and "\n"
//     together.
//
// It was seven. "---\r\n&a1 \nFalse\t# c1\r\n" put the Comment one byte inside the anchor before it, because
// removeRightSpaceFromBuf cut the space closing "&a1 " off the origin and the extent was taken from what was left.
// cursor.originTrimmed counts those bytes back into the end.
//
// None of the six is recorded by offsetMissLedger, and the overlap was measured rather than assumed. That ledger
// reads a token's text back through the extents, so a token whose extent breaks gets an empty origin and is skipped.
// Six of the 402 test suite documents break the extents, and in every one the break falls on the last token, so no
// token downstream of a break has its origin shifted. Six tokens are invisible to offsetMissLedger for that reason,
// and this ledger records them.
//
// The three String misses offsetMissLedger does record sit in three other documents: a block scalar each in
// spec-example-8-1-block-scalar-header and the two spec-example-8-2-block-indentation-indicator cases. One document
// reaches both corpora,
// sequence-entry-that-looks-like-two-with-wrong-indentation, keyed 1ae84972 here, and it contributes no offset miss.
//
// A scanner fix has to move both ledgers together. Clamping the extent on 1ae84972 makes its Offset visible, and
// Offset 21 does not address the token, so offsetMissLedger's String count rises as this entry goes.
//
// The documents are keyed by the first four bytes of the SHA-256 of their text. jsonLedger, jsonTokenLedger and
// decodeLedger key a document by its test suite case name, which is the convention here, and it runs out at the five
// entries that are fuzz seeds alone: a seed's index moves whenever the corpus is generated again. A hash keys the
// text itself, so growing the corpus adds an entry only when a document that breaks the tiling is genuinely new.
//
// Three things move when the scanner is fixed, not two. transform.Walk clamps an extent into the document rather
// than refusing it, in walker.extent, so the six still rebuild byte for byte: the bytes past the end are not there
// to write. Emptying this ledger raises offsetMissLedger's String count and leaves that clamp guarding nothing, so
// the entry, the count and the clamp go in one window. A clamp still standing after this ledger empties is dead code
// hiding a fixed bug.
var extentLedger = map[string]string{
	// seed/1795: a mapping under a byte order mark, ending on a multi-line plain scalar.
	"1319f7b1": "a String token ends 45 bytes past the document",
	// seed/0202, suite/sequence-entry-that-looks-like-two-with-wrong-indentation:
	// "- single multiline\n - sequence entry\n", the smallest of the five.
	"1ae84972": "a String token ends 19 bytes past the document",
	// seed/9615: a sequence entry holding a verbatim tag.
	"59cf8381": "a String token ends 9 bytes past the document",
	// seed/9354: "- {}\r\n- null\r\n    - '-1'\r\n".
	"a1258ce0": "a String token ends 11 bytes past the document",
	// seed/17802: a flow mapping after a "%TAG" directive, with "\r\n" breaks.
	"c49d6b9d": "a MappingValue token starts 1 bytes before the one before it",
}

// TestExtentsTileTheAcceptedCorpus scans every document the parse accepts and holds what fails to tile against
// extentLedger.
//
// Only the documents the parse accepts are scored. A document it refuses has no walk to tile, and 464 of the corpus
// break the extents on the way to being refused, almost all of them on the Invalid token an error carries, which is
// built from more of the source than one token's worth. Scoring those needs a second ledger: offsetMissLedger already
// counts 13 Invalid misses over refused documents, and the two would collide there.
func TestExtentsTileTheAcceptedCorpus(t *testing.T) {
	t.Parallel()

	measured := make(map[string]string)
	var accepted int
	for text := range acceptedDocuments(t) {
		accepted++
		if defect := firstExtentDefect(text); defect != "" {
			measured[documentKey(text)] = defect
		}
	}

	t.Logf("scanned %d accepted documents, %d do not tile", accepted, len(measured))
	require.Positive(t, accepted)
	ledgers.Compare(t, "extent defects", measured, extentLedger)
}

// firstExtentDefect names the first token of text whose extent leaves the document or falls behind the token before
// it, and returns "" where the extents tile.
func firstExtentDefect(text string) string {
	var s scanner.Scanner
	s.Init([]byte(text))

	prev := 0
	for {
		tk, ok := s.NextToken()
		if !ok {
			return ""
		}
		at, end := int(tk.Position.Offset()), int(tk.EndOffset())
		switch {
		case end > len(text):
			return fmt.Sprintf("a %s token ends %d bytes past the document", tk.Type, end-len(text))
		case at < prev:
			return fmt.Sprintf("a %s token starts %d bytes before the one before it", tk.Type, prev-at)
		}
		prev = end
	}
}

// documentKey names a document by the first four bytes of the SHA-256 of its text.
func documentKey(text string) string {
	sum := sha256.Sum256([]byte(text))

	return hex.EncodeToString(sum[:4])
}

// acceptedDocuments yields the text of every document of the test suite and the fuzz seeds that the parse accepts,
// each text once however many names carry it.
func acceptedDocuments(t *testing.T) map[string]struct{} {
	t.Helper()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	texts := make(map[string]struct{}, len(suites)+len(seeds))
	for _, s := range suites {
		texts[string(s.InYAML)] = struct{}{}
	}
	for _, s := range seeds {
		texts[s] = struct{}{}
	}

	for text := range texts {
		if _, err := parser.ParseBytes([]byte(text)); err != nil {
			delete(texts, text)
		}
	}

	return texts
}
