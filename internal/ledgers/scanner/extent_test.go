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
// It holds nothing: all 12,375 documents the parse accepts tile, measured 2026-09-11.
//
// It held four until scanRawFoldedChar started a plain scalar continued by a "- " line where its text does. Each of
// the four ended a String token 9 to 45 bytes past the end of the document, and "- single multiline\n - sequence
// entry\n" was the smallest: 37 bytes, with the scalar reported at Offset() 21 and EndOffset() 56. The start was wrong
// too, since the scalar begins at offset 2. The fix corrected the Offset with the end, so offsetMissLedger's String
// count stayed where it was, and transform's walker.extent stopped clamping an extent into the document.
//
// It held six before a "<<" cut out of a plain scalar as a merge key stopped starting a MappingValue token inside
// the one before it: seed/17860 writes "{<<<: ...}" and seed/10138 " i    <<:". It held seven before that, while
// removeRightSpaceFromBuf cut the space closing "&a1 " off the origin and put the Comment after it one byte inside the
// anchor. cursor.originTrimmed counts those bytes back into the end.
//
// offsetMissLedger reads a token's text back through the extents, so a token whose extent breaks gets an empty
// origin there and is counted nowhere. A document that stops tiling has to be recorded here.
//
// The documents are keyed by the first four bytes of the SHA-256 of their text. jsonLedger, jsonTokenLedger and
// decodeLedger key a document by its test suite case name, which is the convention here, and it runs out at the fuzz
// seeds: a seed's index moves whenever the corpus is generated again. A hash keys the text itself, so growing the
// corpus adds an entry only when a document that breaks the tiling is genuinely new.
var extentLedger = map[string]string{}

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
		at, end := tk.Position.Offset(), tk.EndOffset()
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
