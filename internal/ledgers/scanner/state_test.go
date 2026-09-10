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

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner"
)

// disagreement is how often a pair disagreed and how often it was checked.
//
// Both are recorded because they answer different questions. A count moves when the corpus moves; a ratio moves when
// the scanner moves. Comparing counts alone reports a regenerated corpus as a scanner regression, which is what this
// ledger did between 2026-09-07 and 2026-09-08.
type disagreement struct {
	Failed int64
	Tested int64
}

// corpusFingerprint names the seed corpus the counts below were measured over: the SHA-256 of every seed
// [fuzzseeds.All] returns, in order, each prefixed by its length.
//
// The seeds come from three sources and any of them moves the counts. yamlcorpus/testdata/yaml-smoke.jsonl.gz is
// generated in another module and was regenerated five times on 2026-09-07, so a baseline measured in the morning was
// stale by the evening. Nothing tied the two together, and the ledger went red carrying numbers that had never
// matched the tree they shipped in.
//
// TestStateLedger checks this before it compares anything, so a regenerated corpus fails saying to re-baseline
// instead of accusing the scanner. It earned that on 2026-09-10: 014db89 regenerated the corpus for two 1.1
// number spellings, seven counts moved at once, and the check named the corpus instead of the scanner.
const corpusFingerprint = "5fe5ec9674b1c75d"

// stateLedger records how often two pieces of the scanner's state that look like the same number disagree, over the
// fuzz corpus.
//
// It decides whether a field can go.
// Four went: Scanner.source, sourceSize, sourcePos and offset each agreed with the Context's src, size and idx over
// 395,323 checks without one disagreement, so they held the Context's numbers under other names.
// The three below disagree, and measuring them turns that from a guess into a count.
//
// A ratchet in both directions, as the offset ledger is.
// A pair that starts disagreeing more has lost an invariant; one that starts disagreeing less may have become
// removable, and the entry comes down to say so.
//
// Each entry carries the denominator it was measured over, so the ratio is on the page next to the count. The counts
// follow the corpus and the ratios follow the scanner, and an entry recording only a count cannot tell those apart.
var stateLedger = map[string]disagreement{ //nolint:gochecknoglobals // ok to store and immutable map as a global
	// Outside a block scalar the mark is the buffer's length less the whitespace and the fold break it ends with,
	// exactly, over every read.
	// It was three until bufferedToken stopped clearing the value buffer and leaving the mark past the end of it.
	//
	// So it could be worked out at the read, a scan back over the buffer once per token, instead of a compare and a
	// store for every character written.
	// Inside a block scalar it could not: the two sites that rewrite the buffer set it outright, keeping the space that
	// folds a line and dropping the tab that ends one, and no scan of the bytes tells those apart from content.
	// 2 of 2,738 re-baselined 2026-09-10 after 16dd5be, from 2 of 2,744 the same day, 2 of 2,743 and 2 of 2,742 on
	// 2026-09-09 and 3 of 1,348 on 2026-09-07. The ratio has held at 0.07% across all four.
	"buf.notSpaceCharPos==trimmed/plain": {0, 317518},
	"buf.notSpaceCharPos==trimmed/block": {2, 2734},

	// A mark past the end of the buffer made bufferedSrc slice a byte the last token wrote.
	// Fixed; nothing may raise this.
	"buf.notSpaceCharPos<=len(buf)": {0, 320252},

	// Both entries count a space opening a line where indentNum has stopped tracking the column. They have different
	// causes, and only the second is a surprise.
	//
	// The /tab bucket is 100% by construction, and reading it as a count of anything else is a mistake made once
	// already. updateIndent takes a tab in leading whitespace, sets indentHasTab and returns without counting it,
	// because s-indent(n) is s-space x n and a tab is separation and not indentation. The main loop advances the column
	// for it regardless, so from that tab to the end of the line indentNum lags column-1 and every following space
	// trips the probe. 11 of 11, from 12 of 12 and 5 of 5 before that: yamlgen began drawing tab separators, so the
	// corpus holds spaces standing after a tab and a regeneration moves how many. The number counts those and nothing
	// else, and it always reads 100%. It would catch a tab starting to count as indentation, which breaks
	// s-indent(n).
	//
	// The /spaces bucket is the one worth watching, and it holds one cause: a quoted scalar spanning a line break.
	// The quote scanners call progressLine, marking the next character as opening a line, then read the rest of the
	// scalar with progressColumn, which never reaches updateIndent. So isFirstCharAtLine is still true after the line
	// has been read into. 6 of 30,174, 0.02%, from 6 of 30,360, 6 of 30,348, 8 of 19,953, 0.04% and 7 of 11,748, 0.06%:
	// the corpus keeps growing faster than the cause, and the count has now fallen while the denominator rose by half.
	// The denominator fell for the first time on 2026-09-10, by 186, and the corpus did not move: 16dd5be ends a plain
	// scalar at a comment whatever its column, so fewer lines open inside one.
	"indent.indentNum==column-1/tab":    {11, 11},
	"indent.indentNum==column-1/spaces": {7, 29870},

	// The indent level a token was given and the level the scanner stands at part company where a block opens, so the
	// two are not a redundant pair. 5,403 of 158,781, from 5,398 of 158,695, 5,399 of 158,692, 4,213 of 129,685,
	// 2,729 of 83,807, 2,793 of 69,177 and 3,000 of 76,279 before that.
	//
	// Read the ratio, not the count. 3.40%, against 3.25% over a corpus 22% smaller, and 4.0% when the corpus was
	// 69,177 pos() calls. The ratio has moved over a range of 0.75 points across five regenerations while the count
	// has nearly doubled, so the count follows the corpus and the ratio is the number that would report a scanner
	// change. 3.25% -> 3.40% is inside that range and no scanner change accounts for it: the corpus artifact is
	// byte-identical across every commit from b120b7b to master.
	//
	// This is the first move of the four that a scanner change does account for, and it is one disagreement fewer
	// on the same corpus: scanDocumentEnd clears the indentation state now, as scanDocumentStart always has, so the
	// level a token is given no longer carries across a "...". 3.4022% -> 3.4013%, which is the ratio holding while
	// the count falls -- the direction the ledger says an entry may come down for.
	//
	// Re-baselined again on 2026-09-10 after 014db89 regenerated the corpus for two new 1.1 number spellings.
	// 3.4013% -> 3.4028%, a move of 0.0015 points where the documented range is 0.75, and every other entry
	// held its ratio to four figures while its denominator grew by about 0.05%. So the corpus moved and the
	// scanner did not, which is the reading the fingerprint check exists to make available: it failed saying
	// the corpus had moved rather than reporting seven regressions.
	//
	// Re-baselined again the same day onto master's own regeneration -- the four ordered-map commits landed
	// while this was in flight -- which moved four denominators by about 0.02% and left every count alone.
	//
	// Re-baselined a fourth time on 2026-09-10, and the fingerprint moved: yamlgen stopped offering "!!omap" on
	// any sequence and offers it only on the sequence of one-entry mappings the tag names, so every seed
	// reshuffled. 3.4039% -> 3.4013%, a move of 0.0026 points where the documented range is 0.75. The /spaces
	// bucket rose 6 -> 7 on a denominator that fell by 304, the first time that count has moved since
	// 2026-09-09; a quoted scalar spanning a line break is the one cause, and the corpus now holds one more.
	//
	// Re-baselined a third time on 2026-09-10, and that one was the opposite reading: the fingerprint held, so
	// the corpus stood still and 16dd5be moved the scanner. Six denominators moved and four of them fell --
	// ending a plain scalar at a comment cuts 186 line openings, 108 indent levels and 6 block-trimmed reads,
	// while plain-trimmed reads rise by 89. 5,403 of 158,781 -> 5,401 of 158,673, 3.4028% -> 3.4039%. Two
	// disagreements fewer on a smaller denominator, which is a ratio holding rather than a scanner regressing.
	//
	// A count re-baselined without the ratio beside it says nothing about whether the scanner changed, which is why
	// the ledger records the denominator.
	"indent.lastIndentLevel==indentLevel": {5385, 158353},

	// bufferedToken assembles a token's extent from what the scanner already holds: where the origin began, how long
	// it is, and the line the text ends on. It does not read the origin back to work the extent out.
	// This compares that extent against token.MeasureOrigin's, which token.Make used.
	// Nothing may raise it: a disagreement is a token pointing at the wrong stretch of source.
	"token.extentMatchesTheOrigin": {0, 48488},
}

// TestStateLedger holds the scanner's state pairs to what they were measured at.
//
//	go test -tags yamlprobe -run TestStateLedger ./internal/scanner/
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
	measured := make(map[string]disagreement, len(checks))
	for name, inv := range checks {
		if _, recorded := stateLedger[name]; inv.Failed > 0 || recorded {
			measured[name] = disagreement{Failed: inv.Failed, Tested: inv.Tested}
		}
	}

	// The counts move with the corpus, so a failure is only readable next to what each was measured over. Compare
	// reports the count; these lines carry the denominator and the ratio.
	for _, name := range slices.Sorted(maps.Keys(measured)) {
		inv := checks[name]
		var ratio float64
		if inv.Tested > 0 {
			ratio = 100 * float64(inv.Failed) / float64(inv.Tested)
		}
		t.Logf("%s: %d of %d (%.2f%%)", name, inv.Failed, inv.Tested, ratio)
	}

	// The corpus is checked before the counts. A seed corpus that has moved makes every count below wrong at once,
	// and reporting that as seven scanner regressions buries the one fact that explains them.
	if got := fingerprintOf(seeds); got != corpusFingerprint {
		t.Fatalf("the seed corpus has moved: measured %s, and the ledger records %s.\n"+
			"The counts above were taken over a different corpus, so nothing here says whether the scanner changed.\n"+
			"Re-baseline: set corpusFingerprint to %s and copy the counts logged above into stateLedger.",
			got, corpusFingerprint, got)
	}

	ledgers.Compare(t, "disagreements", measured, stateLedger)

	for name := range stateLedger {
		require.Containsf(t, checks, name, "%s is in the ledger and nothing checks it", name)
	}
}

// fingerprintOf hashes the seeds in the order they were scanned, each prefixed by its length so that two corpora
// cannot collide by splitting the same bytes differently.
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
