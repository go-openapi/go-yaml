// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"sort"
	"strings"
	"testing"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

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
	oracle := yamloracle.NewRecognizer(1024)

	for _, r := range yamlbuild.Refusals() {
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
// Replacing `'@' is a reserved character` with `found text the scanner could not read as a token` in
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
// [Generator] is bumped on the same commit. Four seeds of the compact tier draw
// 53 to 55 signatures and agree on 50 of them, so the handful at the bottom are
// reachable rather than reliable.
func TestTheCorpusDrawsEveryComplaint(t *testing.T) {
	_, cases, err := yamlembed.CompactSuite()
	require.NoError(t, err)

	seen := map[string]int{}
	refused := 0

	for _, c := range cases {
		_, perr := parser.ParseBytes(c.Src, parser.WithComments())
		if perr == nil {
			continue
		}
		refused++
		seen[yamlbuild.RefusalSignature(perr)]++
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

		return yamlbuild.RefusalSignature(err)
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
//
// It came back on 2026-09-10, and nothing about the parser changed: narrowing
// the "!!omap" draw to the shape the tag names reshuffled every seed, and the
// reshuffled corpus draws the complaint again. One added, none gone.
//
// "a node takes at most one tag" arrived on 2026-09-11, when the parser began
// refusing a second tag on one node. 8 documents of the corpus draw it, and the
// grammar refuses all 8. 4 of them were read before, and the other 4 were
// refused with "a document holds one root node, and this stands past it".
//
// "an implicit key must stand on one line" arrived on 2026-11-18,
// when a pair inside a flow sequence began to be refused for a key that starts
// on a line above its ':', as in "[a" over " b: c]". The check read only the
// line the key ends on, so such a key was read as "a b".
//
// "a directive must be named right after its _" arrived on 2026-11-18, when the
// scanner began refusing a "%" with nothing but a blank, a break or the end of
// the stream after it, and "a directive must be named right after its '%'" left with it: the
// grouping still refuses a "%" that never got a name, and no document reaches it
// now that the scanner refuses the same "%" first.
//
// "found a byte order mark inside a comment" arrived the same day, when a
// comment holding U+FEFF began to be refused: c-nb-comment-text is '#'
// nb-char*, and nb-char leaves the mark out.
//
// "a block mapping cannot start on the line of its tag" and its anchor
// twin arrived the same day, when a node property standing before a "?" on its
// line began to be refused as one before a "-" was: "!!null ?" was read as a
// tagged mapping, and "k: &m ? a" as {k: {a: null}}.
//
// Three arrived on 2026-11-23, when conformance-suites' generator began drawing
// properties on the "<<", "!!omap" families and tagged keys, and nothing about
// the parser changed. "a !!merge tag must stand on '<<'" is drawn by mutants alone,
// each "!!merge" left standing on something that is not a "<<"; the same draws
// found "? !!merge <<" refused with it, which is now fixed.
// "a flow sequence entry is missing before _" and "a leading empty line of a block
// scalar holds more spaces than its first content line" come from the
// reshuffled seeds.
//
// Re-baselined on 2026-11-26 when the parser and scanner messages were reworded
// to the spec's nouns. The corpus refuses the same 9,414 cases; 79 signatures
// became 76, mostly because the length and the digit escapes merged. The same
// day "an alias cannot carry an anchor or a tag" split from "found _ where a
// scalar is expected", and the two mismatched-closer messages from the scanner's
// new check, which refuses "[{]}" at the ']'.
var parserComplaints = []string{
	"\\U takes eight hexadecimal digits",
	"\\u takes four hexadecimal digits",
	"\\x takes two hexadecimal digits",
	"_ is a reserved character",
	"_ is not a scalar, and a flow collection has no sequence entries",
	"a _ in a tag must be followed by two hexadecimal digits",
	"a _ tag must stand on _",
	"a block mapping cannot start on the _ line",
	"a block mapping cannot start on the line of its anchor",
	"a block mapping cannot start on the line of its tag",
	"a block scalar header cannot take this option",
	"a block sequence cannot start on the line of its anchor",
	"a block sequence cannot start on the line of its key",
	"a block sequence cannot start on the line of its tag",
	"a comment must be preceded by a space, and a scalar cannot begin with _",
	"a comment must be separated from a block scalar header by a space",
	"a directive must be followed by a _ line",
	"a directive must be named right after its _",
	"a document holds one root node, and this stands past it",
	"a document takes at most one YAML directive",
	"a double-quoted scalar is not closed by _",
	"a flow collection continues on a line that is not indented past the one it started on",
	"a flow mapping entry is missing before _",
	"a flow mapping entry must be followed by _ _ or _",
	"a flow mapping is not closed by _",
	"a flow mapping needs _ or _ after an entry",
	"a flow sequence entry is missing before _",
	"a flow sequence is not closed by _",
	"a flow sequence needs _ or _ after an entry",
	"a leading empty line of a block scalar holds more spaces than its first content line",
	"a line at the column of a mapping must open an entry",
	"a mapping value cannot open a mapping on the line of its key",
	"a node takes at most one anchor",
	"a node takes at most one tag",
	"a plain scalar cannot begin with _",
	"a quoted scalar cannot hold a _ or _ line",
	"a scalar continues on a line that is not indented past the one it started on",
	"a single-quoted scalar is not closed by a single quote",
	"a tab cannot stand for the indentation a mapping entry needs",
	"a tab cannot stand for the indentation a sequence entry needs",
	"a tag cannot hold _",
	"a tag handle takes letters, digits and _ between its _ characters",
	"a tag must name a suffix after its handle",
	"a verbatim tag must end with _",
	"an alias cannot carry an anchor or a tag",
	"an alias must be followed by a name",
	"an alias must be separated from the node that follows it",
	"an anchor must be followed by a name",
	"an anchor must be separated from the node that follows it",
	"an explicit key names one node, and this stands past it",
	"an implicit key must stand on one line",
	"anchor is not indented past its _",
	"anchor is not indented past its key",
	"could not find alias _",
	"found _ where _ closes a flow mapping",
	"found _ where _ closes a flow sequence",
	"found _ where a mapping key is expected",
	"found _ where a scalar is expected",
	"found _ with no _ to close",
	"found a _ with no mapping key before it",
	"found a byte order mark inside a comment",
	"found a byte order mark inside a line, where a node may not hold one",
	"found a byte that is part of no character",
	"found a tab character where an indentation space is expected",
	"found an escaped code point that is not a character",
	"found character _ that a YAML stream may not hold",
	"found character _ that cannot start any token",
	"found unknown escape character _",
	"tag handle _ has already been declared by a TAG directive",
	"tag handle _ is not defined by a TAG directive",
	"tag is not indented past its _",
	"tag is not indented past its key",
	"the TAG directive takes a handle and a prefix",
	"the YAML directive takes one version number",
	"the content of a block scalar is indented less than the indicator in its header states",
	"unknown YAML version _",
	"value is not allowed in this context. map key-value is pre-defined",
	"value is not indented past its _",
	"value is not indented past its key",
}
