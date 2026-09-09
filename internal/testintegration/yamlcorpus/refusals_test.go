// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/parser"
)

// TestEveryRefusalSaysWhy holds each named document to the reason it is
// refused for, not merely to being refused.
//
// The grammar's verdict is checked alongside, because the two say different
// things about a failure. Where the grammar refuses too, a lost message means
// the parser stopped explaining a syntax error it still catches. Where the
// grammar accepts, the message is the only statement anywhere of a rule the
// library applies and the language does not.
func TestEveryRefusalSaysWhy(t *testing.T) {
	oracle := grammar.NewRecognizer(1024)

	for _, r := range yamlcorpus.Refusals() {
		t.Run(r.Name, func(t *testing.T) {
			assert.Equal(t, r.WellFormed, oracle.Stream([]byte(r.Src)).OK,
				"the document is %q and WellFormed says otherwise", r.Src)

			_, err := parser.ParseBytes([]byte(r.Src), parser.WithComments())
			require.Error(t, err, "%q is read, so there is no message to hold", r.Src)

			assert.Contains(t, err.Error(), r.Says,
				"%q is refused and no longer says why:\n  want a message containing: %s\n  got: %s",
				r.Src, r.Says, err)
		})
	}
}

// TestTheCorpusDrawsEveryComplaint is the wide net: every distinct thing the
// parser says while refusing a document in the stored corpus.
//
// Asserted as a set rather than as a count, and the difference is not academic.
// Replacing `'@' is a reserved character` with `found an invalid token` in
// scanner.go -- the regression this exists for, reproduced to check the test
// catches it -- left the count at 54, because one complaint went and one
// arrived. The set reports `gone: [_ is a reserved character]` and names it.
//
// Both directions are worth failing on. A complaint that leaves means either
// the parser stopped making it or the generator stopped reaching it. One that
// arrives means the parser learned to say something new, or the generator
// learned to provoke it, and either is worth a line in a review rather than a
// silent pass.
//
// The list moves when the corpus is regenerated, which is deliberate and rare:
// [Generator] is bumped on the same commit. Four seeds of the smoke tier draw
// 53 to 55 signatures and agree on 50 of them, so the handful at the bottom are
// reachable rather than reliable.
func TestTheCorpusDrawsEveryComplaint(t *testing.T) {
	_, cases, err := yamlcorpus.SmokeSuite()
	require.NoError(t, err)

	seen := map[string]int{}
	refused := 0

	for _, c := range cases {
		_, perr := parser.ParseBytes(c.Src, parser.WithComments())
		if perr == nil {
			continue
		}
		refused++
		seen[yamlcorpus.RefusalSignature(perr)]++
	}

	got := make([]string, 0, len(seen))
	for sig := range seen {
		got = append(got, sig)
	}
	sort.Strings(got)

	// The template count is measured rather than written down here. See
	// TestTheParserVocabularyGapIsMeasured, which reads them out of the source
	// and names the ones nothing reaches.
	t.Logf("%d of %d cases refused, %d distinct complaints",
		refused, len(cases), len(got))

	if !assert.Equal(t, parserComplaints, got, "the parser's vocabulary over this corpus has changed") {
		t.Logf("gone:  %v", missingFrom(got, parserComplaints))
		t.Logf("added: %v", missingFrom(parserComplaints, got))
	}
}

// missingFrom returns the entries of want that have are not in got.
func missingFrom(got, want []string) []string {
	have := make(map[string]bool, len(got))
	for _, s := range got {
		have[s] = true
	}

	var out []string
	for _, s := range want {
		if !have[s] {
			out = append(out, s)
		}
	}

	return out
}

// TestARefusalSignatureKeepsTheParserAndDropsTheDocument is what makes the set
// above readable: the same complaint about two documents has to be one entry,
// and two different complaints must not collapse into one.
func TestARefusalSignatureKeepsTheParserAndDropsTheDocument(t *testing.T) {
	sig := func(src string) string {
		_, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.Error(t, err, "%q", src)

		return yamlcorpus.RefusalSignature(err)
	}

	t.Run("the document falls out", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"@ x\n", "` x\n"},
			{"a: [1\n", "b:\n  - [2\n"},
			{"a: \"\\q\"\n", "a: \"\\z\"\n"},
		} {
			assert.Equal(t, sig(pair[0]), sig(pair[1]),
				"%q and %q are the same complaint and must group", pair[0], pair[1])
		}
	})

	t.Run("the parser's words stay", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"@ x\n", "& x\n"},
			{"& x\n", "* x\n"},
			{"a: [1\n", "a: {b: 1\n"},
		} {
			assert.NotEqual(t, sig(pair[0]), sig(pair[1]),
				"%q and %q are different complaints and must not group", pair[0], pair[1])
		}
	})

	t.Run("nothing of the document survives", func(t *testing.T) {
		// A signature that still held a position, a quoted literal or a number
		// would be one per document rather than one per complaint.
		for _, src := range []string{"@ x\n", "a: [1\n", "a: \"\\u00zz\"\n", "\x00\n"} {
			got := sig(src)
			assert.NotContains(t, got, "[")
			assert.NotContains(t, got, "'")
			assert.False(t, strings.ContainsAny(got, "0123456789"), "%q -> %q", src, got)
		}
	})
}

// parserComplaints is every distinct thing this library says while refusing a
// document in the stored corpus, as [yamlcorpus.RefusalSignature] reduces it.
//
// Generated by running TestTheCorpusDrawsEveryComplaint and pasting what it
// reports. Regenerate it on the same commit that regenerates the corpus, and
// read the diff: a line that leaves is a complaint nothing provokes any more.
//
// "found an invalid key for this map" left on 2026-09-09, when the explicit
// key's ':' was held to the '?'s own column. One document in the corpus drew
// it, and that document is valid -- the grammar accepts it -- so it now parses.
// The complaint is still live: the enumerated shape "]: 1" in refusals.go draws
// it, and TestTheParserVocabularyGapIsMeasured does not count it unreached.
var parserComplaints = []string{
	"YAML version has already been specified",
	"_ is a reserved character",
	"_ is not a scalar, and a flow collection has no sequence entries",
	"_ or _ must be specified",
	"a _ in a tag must be followed by two hexadecimal digits",
	"a comment must be preceded by a space, and a scalar cannot begin with _",
	"a flow collection continues on a line that is not indented past the one it started on",
	"a plain scalar cannot begin with _",
	"a scalar continues on a line that is not indented past the one it started on",
	"a tag handle takes letters, digits and _ between its _ characters",
	"a tag must name a suffix after its handle",
	"a verbatim tag must end with _",
	"an alias must be followed by a name",
	"an alias must be separated from the node that follows it",
	"an anchor must be followed by a name",
	"an anchor must be separated from the node that follows it",
	"an explicit key names one node, and this stands past it",
	"anchor is not allowed in this context",
	"anchor is not allowed in this sequence context",
	"anchors cannot be used consecutively",
	"block sequence entries are not allowed in this context",
	"cannot use this node as a map key",
	"comment must be separated from the block scalar header by a space",
	"could not find _ character corresponding to _",
	"could not find alias _",
	"could not find end character of double-quoted text",
	"could not find end character of single-quoted text",
	"could not find flow map content",
	"could not find flow mapping end token _",
	"could not find map value",
	"found a byte order mark inside a line, where a node may not hold one",
	"found a byte that is part of no character",
	"found a character that is not a hexadecimal digit in escaped N-bit character",
	"found a character that is not a hexadecimal digit in escaped UTF-N character",
	"found a tab character where an indentation space is expected",
	"found an escaped code point that is not a character",
	"found character _ that a YAML stream may not hold",
	"found character _ that cannot start any token",
	"found invalid tag character _",
	"found unexpected document separator",
	"found unknown escape character _",
	"invalid header option",
	"mapping value is not allowed in this context",
	"non-map value is specified",
	"not enough length for escaped N-bit character",
	"not enough length for escaped UTF-N character",
	"sequence end token _ not found",
	"sequence entries are not allowed after a tag on the same line",
	"sequence entries are not allowed after anchor on the same line",
	"tab character cannot stand for the indentation a mapping entry needs",
	"tab character cannot use as a sequence delimiter",
	"tag handle _ has already been declared by a TAG directive",
	"tag handle _ is not defined by a TAG directive",
	"tag is not allowed in this context",
	"tag is not allowed in this sequence context",
	"the content of a block scalar is indented less than the indicator in its header states",
	"undefined directive value",
	"unexpected directive value. document not started",
	"unexpected format TAG directive",
	"unexpected format YAML directive",
	"unexpected key name",
	"unexpected map key",
	"unexpected scalar value",
	"unexpected scalar value type",
	"unknown YAML version _",
	"value cannot be placed after document separator",
	"value is not allowed in this context",
	"value is not allowed in this context. map key-value is pre-defined",
	"value is not indented past its key",
}
