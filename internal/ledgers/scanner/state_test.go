// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package scanner_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"maps"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner"
)

// stateLedger records how often two pieces of the scanner's state that look like the same number disagree, over the
// fuzz corpus.
//
// It decides whether a field can go.
// Four went: Scanner.source, sourceSize, sourcePos and offset each agreed with the Context's src, size and idx over
// 395,323 checks without one disagreement, so they held the Context's numbers under other names.
// The pairs below disagree, and measuring them turns that from a guess into a count.
// A fifth went on 2026-11-20: indent.lastIndentLevel==indentLevel compared two indent levels nothing had read since
// the token stopped carrying one, and Scanner.indentLevel, lastIndentLevel and prevLineIndentNum went with it.
// Its last reading was 5,444 of 156,628.
//
// A ratchet in both directions, as the offset ledger is.
// A pair that starts disagreeing more has lost an invariant; one that starts disagreeing less may have become
// removable, and the entry comes down to say so.
//
// An entry records the number of disagreements, and nothing else fails the test. The number of checks moves with the
// corpus and with every scanner fix that changes what gets scanned, so it is logged next to the count and not compared.
// A check the ledger names that ran zero times fails: the probe stopped being reached.
var stateLedger = map[string]int64{ //nolint:gochecknoglobals // ok to store an immutable map as a global
	// Outside a block scalar the mark is the buffer's length less the whitespace and the fold break it ends with,
	// exactly, over every read.
	// It was three until bufferedToken stopped clearing the value buffer and leaving the mark past the end of it.
	//
	// So it could be worked out at the read, a scan back over the buffer once per token, instead of a compare and a
	// store for every character written.
	// Inside a block scalar it could not: the two sites that rewrite the buffer set it outright, keeping the space that
	// folds a line and dropping the tab that ends one, and no scan of the bytes tells those apart from content.
	// The block entry read 2 of 2,738 until the scanner began to keep a block scalar's final line feed at the end of
	// the stream, which took both disagreements away. The corpus now holds no read where the mark and the trimmed length
	// differ inside a block scalar, so nothing here tests the paragraph above any more; a seed that brings one back
	// raises the count.
	"buf.notSpaceCharPos==trimmed/plain": 0,
	"buf.notSpaceCharPos==trimmed/block": 0,

	// A mark past the end of the buffer made bufferedSrc slice a byte the last token wrote.
	// Fixed; nothing may raise this.
	"buf.notSpaceCharPos<=len(buf)": 0,

	// Both entries count a space opening a line where indentNum has stopped tracking the column, the second where a
	// tab stands in the run before it. Nothing may raise either.
	//
	// The /tab bucket read 3 of 7 until a tab that is content, in the leading whitespace of a block scalar's line,
	// began to count its column: the scan loop's tab arm counts one there, and scanMultiLine moved the column over it
	// and left indentNum alone, so "  \t bar" under ">" read indentNum 2 at column 4.
	//
	// The /spaces bucket read 8 of 100,143 until a quoted scalar running over several lines stopped leaving the line
	// marked as opening: past the closing quote, the space before "# lala" in "a: \"double" over "  quotes\" # lala"
	// counted as indentation.
	"indent.indentNum==column-1/tab":    0,
	"indent.indentNum==column-1/spaces": 0,

	// bufferedToken assembles a token's extent from what the scanner already holds: where the origin began, how long
	// it is, and the line the text ends on. It does not read the origin back to work the extent out.
	// This compares that extent against token.MeasureOrigin's, which token.Make used.
	// Nothing may raise it: a disagreement is a token pointing at the wrong stretch of source.
	"token.extentMatchesTheOrigin": 0,
}

// TestStateLedger holds the scanner's state pairs to what they were measured at.
//
//	go test -tags yamlprobe -run TestStateLedger ./internal/ledgers/scanner/
func TestStateLedger(t *testing.T) {
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	require.NotEmpty(t, seeds)

	probe.Reset()
	for _, src := range seeds {
		var s scanner.Scanner
		s.Init([]byte(src))
		for {
			if _, ok := s.NextToken(); !ok {
				break
			}
		}
	}

	// probe.Checks holds every invariant the run touched, the ones that never disagreed included, and most of them
	// never disagree. An invariant is measured when it disagreed, so a clean one the ledger says nothing about is not
	// reported as a new defect.
	//
	// A clean invariant the ledger does name is measured too, at 0. Those entries are the ones recording that a pair
	// must never disagree, and dropping them would read as a defect that had gone away.
	checks := probe.Checks()
	measured := make(map[string]int64, len(checks))
	for name, inv := range checks {
		if _, recorded := stateLedger[name]; inv.Failed > 0 || recorded {
			measured[name] = inv.Failed
		}
	}

	// The number of checks moves with the corpus, so a count is only readable next to it. Compare reports the count;
	// these lines carry the number of checks and the ratio.
	t.Logf("seed corpus %s, %d seeds", fingerprintOf(seeds), len(seeds))
	for _, name := range slices.Sorted(maps.Keys(measured)) {
		inv := checks[name]
		var ratio float64
		if inv.Tested > 0 {
			ratio = 100 * float64(inv.Failed) / float64(inv.Tested)
		}
		t.Logf("%s: %d of %d (%.2f%%)", name, inv.Failed, inv.Tested, ratio)
	}

	ledgers.Compare(t, "disagreements", measured, stateLedger)

	for _, name := range slices.Sorted(maps.Keys(stateLedger)) {
		assert.Positivef(t, checks[name].Tested, "%s is in the ledger and nothing checked it", name)
	}
}

// fingerprintOf hashes the seeds in the order they were scanned, each prefixed by its length so that two corpora
// cannot collide by splitting the same bytes differently. The log carries it, so two runs show whether they read the
// same corpus.
func fingerprintOf(seeds []string) string {
	h := sha256.New()
	var size [8]byte

	for _, seed := range seeds {
		binary.LittleEndian.PutUint64(size[:], uint64(len(seed)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(seed))
	}

	return hex.EncodeToString(h.Sum(nil)[:8])
}
