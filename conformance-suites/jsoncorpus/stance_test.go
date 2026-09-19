// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/jsoncorpus"
	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"
)

// measuredDefaultLexer is what the default lexer actually does with the
// suite's implementation-defined cases.
//
// It was produced by running the lexer over them, not by reading its
// documentation. It exists so that TestTheStanceDescribesTheRealLexer can check
// the declared stance against behavior rather than against intent: a stance is
// a claim about a parser, and an unchecked claim is how a corpus starts lying.
//
// Only the implementation-defined cases are listed. The other 283 are settled
// by the suite itself, and the lexer passes all of them -- core's
// conformanceXFail() is empty -- so the suite's own label is the measurement.
var measuredDefaultLexer = map[string]stance.Outcome{
	"i_number_double_huge_neg_exp":                   stance.Accept,
	"i_number_huge_exp":                              stance.Accept,
	"i_number_neg_int_huge_exp":                      stance.Accept,
	"i_number_pos_double_huge_exp":                   stance.Accept,
	"i_number_real_neg_overflow":                     stance.Accept,
	"i_number_real_pos_overflow":                     stance.Accept,
	"i_number_real_underflow":                        stance.Accept,
	"i_number_too_big_neg_int":                       stance.Accept,
	"i_number_too_big_pos_int":                       stance.Accept,
	"i_number_very_big_negative_int":                 stance.Accept,
	"i_structure_500_nested_arrays":                  stance.Accept,
	"i_structure_UTF-8_BOM_empty_object":             stance.Accept,
	"i_object_key_lone_2nd_surrogate":                stance.Reject,
	"i_string_1st_surrogate_but_2nd_missing":         stance.Reject,
	"i_string_1st_valid_surrogate_2nd_invalid":       stance.Reject,
	"i_string_UTF-16LE_with_BOM":                     stance.Reject,
	"i_string_UTF-8_invalid_sequence":                stance.Reject,
	"i_string_UTF8_surrogate_U+D800":                 stance.Reject,
	"i_string_incomplete_surrogate_and_escape_valid": stance.Reject,
	"i_string_incomplete_surrogate_pair":             stance.Reject,
	"i_string_incomplete_surrogates_escape_valid":    stance.Reject,
	"i_string_invalid_lonely_surrogate":              stance.Reject,
	"i_string_invalid_surrogate":                     stance.Reject,
	"i_string_invalid_utf-8":                         stance.Reject,
	"i_string_inverted_surrogates_U+1D11E":           stance.Reject,
	"i_string_iso_latin_1":                           stance.Reject,
	"i_string_lone_second_surrogate":                 stance.Reject,
	"i_string_lone_utf8_continuation_byte":           stance.Reject,
	"i_string_not_in_unicode_range":                  stance.Reject,
	"i_string_overlong_sequence_2_bytes":             stance.Reject,
	"i_string_overlong_sequence_6_bytes":             stance.Reject,
	"i_string_overlong_sequence_6_bytes_null":        stance.Reject,
	"i_string_truncated-utf-8":                       stance.Reject,
	"i_string_utf16BE_no_BOM":                        stance.Reject,
	"i_string_utf16LE_no_BOM":                        stance.Reject,
}

// TestTheStanceDescribesTheRealLexer checks that the grammar plus the declared
// stance predict what the lexer does, over every document in the suite.
//
// This is the claim the corpus format rests on. A corpus stores the grammar's
// verdict and a set of tags, never a verdict about a parser, so that one corpus
// can score several parsers holding different positions. That only works if
// tags plus a stance really do reconstruct a parser's behavior -- and this is
// where that gets checked against a parser we have.
func TestTheStanceDescribesTheRealLexer(t *testing.T) {
	dir := suiteDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	var (
		docs           []stance.Doc
		wrong, unknown []string
		scored         int
	)

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		src, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			t.Fatalf("cannot read %s: %v", name, rerr)
		}

		short := strings.TrimSuffix(name, ".json")
		doc := jsoncorpus.Describe(short, src)
		docs = append(docs, doc)

		want, ok := realBehaviour(short)
		if !ok {
			t.Fatalf("no measurement for %s", short)
		}

		got, why := jsoncorpus.DefaultLexer.Expect(doc)
		switch {
		case got == stance.Undecided:
			unknown = append(unknown, short+" -- "+why)
		case got != want:
			wrong = append(wrong, short+": the lexer says "+want.String()+
				", the stance predicts "+got.String()+" -- "+why)
		default:
			scored++
		}
	}

	t.Logf("%d of %d documents predicted, %d undecidable under this stance", scored, len(docs), len(unknown))

	if missing := jsoncorpus.DefaultLexer.Undeclared(docs); len(missing) > 0 {
		t.Errorf("the stance says nothing about %v, so those documents go unscored", missing)
	}

	sort.Strings(unknown)
	for _, u := range unknown {
		t.Logf("undecidable -- %s", u)
	}

	sort.Strings(wrong)
	for _, w := range wrong {
		t.Errorf("%s", w)
	}
}

// realBehaviour is what the lexer does with a document: the measurement for an
// implementation-defined case, and the suite's own label otherwise.
func realBehaviour(short string) (stance.Outcome, bool) {
	switch {
	case strings.HasPrefix(short, "y_"):
		return stance.Accept, true
	case strings.HasPrefix(short, "n_"):
		return stance.Reject, true
	default:
		got, ok := measuredDefaultLexer[short]

		return got, ok
	}
}

// TestOneCorpusScoresTwoStages is the demonstration that the axis pays for
// itself, on the artifact that is actually checked in.
//
// The lexer stops at parsing and never converts a number, so the corpus's
// out-of-range cases are not its question and it is scored on the rest. A
// consumer that does convert reaches them, and the same bytes become evidence
// about it. Before the stages this needed the lexer to declare Accepts on a
// property it has no way to have an opinion about.
//
// The converting table below is illustrative and says so: unlike DefaultLexer
// it is not measured against anything, because there is nothing here to measure
// it against. What is being demonstrated is the mechanism, not a parser.
func TestOneCorpusScoresTwoStages(t *testing.T) {
	_, cases, err := jsoncorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	converting := stance.Table{
		Name:    "an illustrative consumer that reads numbers into float64",
		Because: "demonstrates a construct-stage position; not measured against any parser",
		At:      stance.Construct,
		Speaks:  jsoncorpus.Vocabulary(),
		Stands: map[stance.Tag]stance.Stand{
			stance.TagNotUTF8:              stance.Refuses,
			stance.TagUTF16:                stance.Refuses,
			stance.TagUTF32:                stance.Refuses,
			stance.TagBOM:                  stance.Accepts,
			jsoncorpus.TagLoneSurrogate:    stance.Refuses,
			jsoncorpus.TagDeepNesting:      stance.Accepts,
			jsoncorpus.TagNumberOutOfRange: stance.Refuses,
		},
	}

	var reachesLexer, reachesConverter int

	for _, c := range cases {
		doc := stance.Doc{
			Name:       c.Name,
			Src:        c.Src,
			WellFormed: c.WellFormed,
			Opaque:     c.Opaque,
			Tags:       tagsOf(c),
			VerdictAt:  suite.StageOf(c.VerdictAt),
		}

		if !hasTag(doc.Tags, jsoncorpus.TagNumberOutOfRange) {
			continue
		}

		if out, _ := jsoncorpus.DefaultLexer.Expect(doc); out == stance.Accept {
			reachesLexer++
		}

		if out, _ := converting.Expect(doc); out == stance.Reject {
			reachesConverter++
		}
	}

	if reachesLexer == 0 {
		t.Fatal("no out-of-range case in the corpus, so this proves nothing")
	}

	if reachesConverter != reachesLexer {
		t.Errorf("%d cases the lexer reads, %d the converter refuses; the same cases should be both",
			reachesLexer, reachesConverter)
	}

	t.Logf("%d cases are accepted by a lexer and refused by a converter, from one corpus and one set of bytes",
		reachesLexer)
}

func hasTag(tags []stance.Tag, want stance.Tag) bool {
	return slices.Contains(tags, want)
}
