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
const corpusFingerprint = "427fbac8b10d916a"

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
	//
	// 0 of 2,764 since 8158f652, which keeps a block scalar's final line feed at the end of the stream: over the
	// same corpus it took both disagreements away. The corpus now holds no
	// read where the mark and the trimmed length differ inside a block scalar, so nothing here tests the paragraph
	// above any more; a seed that brings one back raises the count.
	"buf.notSpaceCharPos==trimmed/plain": {0, 312943},
	"buf.notSpaceCharPos==trimmed/block": {0, 2764},

	// A mark past the end of the buffer made bufferedSrc slice a byte the last token wrote.
	// Fixed; nothing may raise this.
	"buf.notSpaceCharPos<=len(buf)": {0, 315707},

	// Both entries count a space opening a line where indentNum has stopped tracking the column. They have different
	// causes, and only the second is a surprise.
	//
	// The /tab bucket counts the spaces that follow a tab among a line's leading whitespace. updateIndent takes such
	// a tab, sets indentHasTab and returns without counting it, because s-indent(n) is s-space x n and a tab is
	// separation and not indentation.
	//
	// 4 of 11, from 11 of 11 on 2026-09-11, when a tab began to move the column. The scan loop's tab arm counts a
	// tab opening a line in indentNum, and it left the column alone, so indentNum stood one ahead of column-1 on
	// every such line: "\t # c1" read indentNum 1 at column 1. Moving the column puts the two back in step, and 7 of
	// the 11 agree now. The 4 left stand inside a block scalar's content, "  \t bar" under ">" among them, where
	// scanMultiLine moves the column over the tab and leaves indentNum alone. Before that the bucket read 100%
	// throughout -- 11 of 11, 12 of 12, 5 of 5 -- and its comment had the direction of the gap the wrong way round.
	//
	// The /spaces bucket is the one worth watching, and it holds one cause: a quoted scalar spanning a line break.
	// The quote scanners call progressLine, marking the next character as opening a line, then read the rest of the
	// scalar with progressColumn, which never reaches updateIndent. So isFirstCharAtLine is still true after the line
	// has been read into. 6 of 30,174, 0.02%, from 6 of 30,360, 6 of 30,348, 8 of 19,953, 0.04% and 7 of 11,748, 0.06%:
	// the corpus keeps growing faster than the cause, and the count has now fallen while the denominator rose by half.
	// The denominator fell for the first time on 2026-09-10, by 186, and the corpus did not move: 16dd5be ends a plain
	// scalar at a comment whatever its column, so fewer lines open inside one.
	"indent.indentNum==column-1/tab": {4, 11},
	//
	// 8 of 30,818 on 2026-11-20 from 7 of 29,756: the regenerations of 2026-09-20 and 2026-11-18 moved it. Over the
	// corpus of 2026-09-11 the scanner of 2026-11-20 still reads 7 of 29,720.
	"indent.indentNum==column-1/spaces": {8, 30818},

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
	// Re-baselined again on 2026-09-11, fingerprint moved, and the cause is eleven documents rather than a
	// generator change: the kind-mismatch census went into yamlcorpus as written-out shapes. Four denominators
	// rose by about 0.03% and every count held, which is what adding a handful of short documents looks like.
	//
	// Re-baselined on 2026-09-11 with the fingerprint HOLDING, which is the reading that says the scanner moved.
	// be17078 ends a block scalar on a last line holding only spaces, where the source ends there; a document
	// that used to be refused now completes. Three counts moved and they are exactly the buffer family the fix
	// touches -- block-trimmed reads 2,734 -> 2,767, plain-trimmed 317,518 -> 317,551, buffer-length checks
	// 320,252 -> 320,318 -- while indent.* and token.* held to the digit. A scanner change confined to the
	// entries the changed code reaches is the shape a fix should have; one that moved the indent counts as well
	// would have been reaching further than its diff.
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
	// Re-baselined on 2026-09-11 with the fingerprint holding, for a "<<" separated from its ":" by a tab. The
	// corpus holds 111 such keys, and they scan as a merge key now instead of a plain scalar. Scanner.scanMergeKey
	// calls pos() for each, where bufferedToken worked the plain scalar's position out without it: indent levels
	// 158,394 -> 158,505 and extents 48,513 -> 48,402, both by 111, with 103 more buffer reads. 17 of the 111
	// disagree, so 5,385 -> 5,402 and 3.3997% -> 3.4081%, a move of 0.0084 points where the documented range is
	// 0.75. The same keys written with a space took that path already.
	//
	// And again the same day, for a "<<" after a plain scalar's text, which stays in that text now. 13 such keys
	// stop being cut as merge keys: indent levels 158,505 -> 158,492 and extents 48,402 -> 48,415, and none of the
	// 13 disagreed, so 3.4081% -> 3.4084%. scanMergeKey reads the buffer for every merge key it cuts, which adds
	// 944 buffer reads.
	//
	// And once more for a tab between a key cut as a token of its own and its ":". One seed,
	// "\"\\n\"\t: !\t|2-" over "  null", read that tab as indentation and was refused; it now reads to its end, as
	// go.yaml.in/yaml/v3 reads it. That one document adds 8 pos() calls and one disagreement, 5,402 of 158,492 ->
	// 5,403 of 158,500 and 3.4084% -> 3.4088%, and between 1 and 5 buffer reads and line openings to four other
	// entries, every one of them with its count unchanged.
	//
	// And for a tab in a directive line, which now separates the name and the parameters as a space does. Each
	// such tab cuts a token where the tab arm read on, so 20 more extents are measured, and 2 fewer buffer reads and
	// 2 fewer pos() calls are made. No count moved: 5,403 of 158,498 is 3.4088%, as 5,403 of 158,500 was.
	//
	// Re-baselined on 2026-09-11 over four moves, three of them the scanner's with the fingerprint holding. The fix
	// that starts a block scalar of spaces at its first content space has addIndent report a content space, and its
	// caller records s.pos() for it: 329 more pos() calls, 2 of them disagreeing, so 5,403 of 158,498 -> 5,405 of
	// 158,827, 3.4089% -> 3.4031%, and no other entry moved. The regeneration that capped yamlgen's float exponents at
	// 995 then moved the fingerprint to 3318075c314e70a4 and every denominator with it, by at most 0.34%, and every
	// count held but this one: 5,388 of 158,906, 3.3907%. Last, the fix that reads a '&' or '*' in a directive line
	// as a character lets the seeds holding one scan further: 22 more pos() calls with one disagreement, 39 more
	// buffer-length checks, 3 more line openings and 7 more extents: 5,389 of 158,928, 3.3908%. Then the fix that reads
	// an anchor name opening with '!', '|', '>', '&' or '*' scans such a name as one name where a tag, a block scalar
	// header or a second anchor used to begin: 61 fewer pos() calls with the disagreements unchanged, 17 fewer line
	// openings, 13 more extents, 5 more buffer-length checks, 6 more plain-trimmed reads and one fewer block-trimmed
	// read. 5,389 of 158,867 is 3.3921%.
	//
	// Re-baselined on 2026-11-20 over six scanner fixes and four regenerations, measured apart by running each fix
	// over the corpus of 2026-09-11. 386a8b0b folds a flow scalar onto a line left of the last colon: 26 more pos()
	// calls, 11 of them disagreeing, 5,400 of 158,893. 3f727cd2 refuses '% 1', a BOM before "---" and '|' in a %TAG
	// prefix, so the seeds holding one stop early: 165 fewer pos() calls, 5,397 of 158,726. da0a2c34 measures an
	// entry from the property in front of its key, and moves this entry most: 57 more disagreements on 38 more
	// pos() calls, 5,454 of 158,761, 3.3921% -> 3.4353%. 43 of the 57 come from seeds whose tokens did not change:
	// lastDelimColumn now stands at the '&' left of the key, so a line indented under "&a1 '0o17':" reads as an
	// indent where it read as a dedent. The dedent flushed an empty buffer, and bufferedToken writes
	// lastIndentLevel whether or not it cuts a token, so the flush hid the level change from the next pos(). The
	// other 14 come from seeds the fix scans differently, ">2-" over "dup" read as one folded scalar among them.
	// 386a8b0b moved 12 the same way, over seeds of flow collections whose tokens held. The rest moved a
	// denominator by 15 or less. The
	// regenerations then moved the fingerprint to 427fbac8b10d916a: 5,444 of 156,628, 3.4757%, a move of 0.04
	// points where the documented range is 0.75.
	//
	// A count re-baselined without the ratio beside it says nothing about whether the scanner changed, which is why
	// the ledger records the denominator.
	"indent.lastIndentLevel==indentLevel": {5444, 156628},

	// bufferedToken assembles a token's extent from what the scanner already holds: where the origin began, how long
	// it is, and the line the text ends on. It does not read the origin back to work the extent out.
	// This compares that extent against token.MeasureOrigin's, which token.Make used.
	// Nothing may raise it: a disagreement is a token pointing at the wrong stretch of source.
	"token.extentMatchesTheOrigin": {0, 47457},
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
