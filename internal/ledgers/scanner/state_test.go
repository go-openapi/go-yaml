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
// The seeds come from three sources and any of them moves the counts. yamlcorpus/testdata/yaml-compact.jsonl.gz is
// generated in another module and was regenerated five times on 2026-09-07, so a baseline measured in the morning was
// stale by the evening. Nothing tied the two together, and the ledger went red carrying numbers that had never
// matched the tree they shipped in.
//
// TestStateLedger checks this before it compares anything, so a regenerated corpus fails saying to re-baseline
// instead of accusing the scanner. It earned that on 2026-09-10: 014db89 regenerated the corpus for two 1.1
// number spellings, seven counts moved at once, and the check named the corpus instead of the scanner.
const corpusFingerprint = "c335bdf21dab01b8"

// stateLedger records how often two pieces of the scanner's state that look like the same number disagree, over the
// fuzz corpus.
//
// It decides whether a field can go.
// Four went: Scanner.source, sourceSize, sourcePos and offset each agreed with the Context's src, size and idx over
// 395,323 checks without one disagreement, so they held the Context's numbers under other names.
// The pairs below disagree, and measuring them turns that from a guess into a count.
// A fifth went on 2026-11-20: indent.lastIndentLevel==indentLevel compared two indent levels nothing had read since
// the token stopped carrying one (e0cba5c4), and Scanner.indentLevel, lastIndentLevel and prevLineIndentNum went with
// it. Its last reading was 5,444 of 156,628.
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
	//
	// 0 of 2,764 since 8158f652, which keeps a block scalar's final line feed at the end of the stream: over the
	// same corpus it took both disagreements away. The corpus now holds no
	// read where the mark and the trimmed length differ inside a block scalar, so nothing here tests the paragraph
	// above any more; a seed that brings one back raises the count.
	//
	// Re-measured on 2026-11-23 over yamlcorpus/48, which draws properties on merges and keys and the 1.1 types: every
	// denominator grew with the corpus, and no entry that stood at 0 moved. On the corpus before it, the same day,
	// the plain and whole-buffer reads had risen by 49 and the extents by 9 when a '}' or a ']' inside a flow
	// collection began to end a plain scalar wherever it stands: "[b}]" scans "b" and then the '}'.
	//
	// Re-baselined 2026-11-24 after four scanner fixes, measured commit by commit; every entry is still at 0, and
	// only the denominators moved (plain / whole buffer / block / spaces / extents):
	//
	//   - e1097b2f, a tab before a key's properties rejected: -16 / -16 / 0 / 0 / -1;
	//   - 2ccac9a0, a '-' before a blank in a flow collection rejected: +14,944 / +14,943 / -1 / -71 / -54;
	//   - ba6b3161, a '-' continuing a plain scalar scanned as plain, the headerless folded block gone:
	//     -33,083 / -33,106 / -23 / -258 / +3;
	//   - 0a4a9742, a token cut at a CR LF ended before the '\r': -1 / -1 / 0 / 0 / 0.
	//
	// Re-baselined the same day, still all at 0, when a directive's name and parameters began to read as text with no
	// indicator's scan: -12,333 / -12,327 / +6 / +129 / +332. The words skip the arm the plain and whole-buffer probes
	// sit in, and the seeds holding a reserved directive such as "%%YAML 1.2" now scan past their first line.
	//
	// Again the same day, still all at 0, when a comment line indented less than a block scalar's content began to
	// end the scalar: +28 / +36 / +8 / +1 / +1. Six seeds such as "|2-" over "#c" now scan to their end.
	//
	// Still all at 0 when "<<::" began to read as the key "<<:" instead of a merge key: -4 / -4 / 0 / 0 / 0.
	//
	// And once more, still all at 0, when a flow indicator began to close an anchor or an alias: -1,437 / -1,437 / 0 /
	// 0 / -1. The likely cause, not measured: while isAlias outlived the ',' of "[*m, b]", scanWhiteSpace took every
	// later space for the end of a property and cut the buffer there, and each cut ran the plain and whole-buffer probes.
	//
	// Still all at 0 when a tab in front of a comment began to read as separation: +103 / +103 / 0 / +16 / +12, and
	// the tab entry 0 of 8. Three seeds holding "\t# c" now scan to their end. scanTab counts the tab it takes in
	// indentNum: before it did, the tab entry read 10 of 17, the spaces after the tab in "\t  # c" one column short.
	"buf.notSpaceCharPos==trimmed/plain": {0, 755932},
	"buf.notSpaceCharPos==trimmed/block": {0, 3795},

	// A mark past the end of the buffer made bufferedSrc slice a byte the last token wrote.
	// Fixed; nothing may raise this.
	"buf.notSpaceCharPos<=len(buf)": {0, 759727},

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
	"indent.indentNum==column-1/tab":    {0, 8},
	"indent.indentNum==column-1/spaces": {0, 99952},

	// bufferedToken assembles a token's extent from what the scanner already holds: where the origin began, how long
	// it is, and the line the text ends on. It does not read the origin back to work the extent out.
	// This compares that extent against token.MeasureOrigin's, which token.Make used.
	// Nothing may raise it: a disagreement is a token pointing at the wrong stretch of source.
	"token.extentMatchesTheOrigin": {0, 116401},
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
