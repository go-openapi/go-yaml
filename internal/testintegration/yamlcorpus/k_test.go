// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/parser"
)

var measureK = flag.Bool("yamlcorpus.k", false, "rebuild the corpus at several quotas and report what each finds")

// TestMeasureK is where PerSignature's value comes from.
//
// Behind a flag because it rebuilds the corpus seven times, and the number it
// produces is meant to be read by a person and written into Smoke rather than
// asserted. Asserting it would fix the answer to whatever this library happens
// to do today, and the quota is a property of the corpus rather than of any
// parser.
//
//	go test -run TestMeasureK ./internal/testintegration/yamlcorpus/ -v -args -yamlcorpus.k
//
// # What is being counted
//
// Not disagreements. Fifty documents failing the same way are one finding, and
// a quota chosen to maximize raw disagreements would just be a quota chosen to
// maximize duplicates. So the complaints are grouped by their text with the
// position stripped, and what is counted is how many distinct ones survive.
//
// # The quota is not the interesting axis
//
// It stops paying at sixteen: raising it to thirty-two buys one complaint for
// 37KB. Drawing more documents does better -- keeping everything at 1500
// documents finds 27 complaints in 535KB, where twice the mutants at a quota of
// sixteen finds 29 in 395KB. Most complaints are singletons, one document in
// twenty thousand, so whether a quota keeps one is luck, and more documents
// beats more of each.
//
// # And not all of those either
//
// Some complaints are the corpus's own fault and have to come out before the
// number means anything -- see [Mislabelled]. The smoke tier's 29 include three
// duplicate-key refusals, one per key spelling, where a byte mutation wrote a
// key twice and the grammar cannot see it. The two columns were equal for a
// while, so the separation is what notices when a change to the generator makes
// them differ again.
func TestMeasureK(t *testing.T) {
	if !*measureK {
		t.Skip("pass -yamlcorpus.k to measure the quota")
	}

	// Both axes, because the quota turned out to be the less useful one. Read
	// down the quota column and it saturates at sixteen; read across to the
	// rows that draw more and the number keeps climbing, so the corpus is
	// short of documents rather than short of quota.
	for _, r := range []struct{ documents, mutants, quota int }{
		{1500, 12, 1},
		{1500, 12, 4},
		{1500, 12, 16},
		{1500, 12, 32},
		{1500, 12, 0},
		{1500, 24, 16},
		{3000, 12, 16},
		{6000, 12, 0},
	} {
		build := yamlcorpus.Build{
			Tier: "measure", Seed: 1,
			Documents: r.documents, MutantsEach: r.mutants, PerSignature: r.quota,
		}

		var buf bytes.Buffer
		if err := build.Write(&buf); err != nil {
			t.Fatal(err)
		}

		_, cases, err := suite.FromBytes(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}

		all, clean := complaints(cases)

		quota := "none"
		if r.quota > 0 {
			quota = strconv.Itoa(r.quota)
		}

		t.Logf("%5d docs x%-3d quota=%-5s %6d cases %8d bytes   %2d complaints, %2d of them the library's",
			r.documents, r.mutants, quota, len(cases), buf.Len(), len(all), len(clean))
	}
}

// TestTheCorpusNoLongerAccusesACorrectParser is the assertion the stage-bound
// verdict was added for.
//
// A mutation that breaks an anchor leaves a document the grammar accepts and a
// conforming parser must refuse: the alias resolves to nothing. The corpus
// recorded the grammar's verdict and no tag, so it expected the document to be
// read, and a library that correctly refused it was scored wrong. Thirty-six
// stored cases did this.
//
// A mutant now claims its acceptance for parsing and no further, so a consumer
// that composes scores it on its refusals and leaves its acceptances alone. The
// refusals are most of what a mutant is worth and they still count everywhere,
// because a document that does not parse does not compose either.
func TestTheCorpusNoLongerAccusesACorrectParser(t *testing.T) {
	_, cases, err := yamlcorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	var accused, unscored, scored int

	for _, c := range cases {
		// An enumerated shape is exempt, and the exemption is the point rather
		// than a convenience. Its label was put on by construction and is
		// right, so a correct-looking refusal there is a *finding* -- the key
		// family carries a document with two keys alike in text and different
		// once resolved, and this library refuses it. Counting that as the
		// corpus misbehaving would count a defect as a mistake.
		if strings.HasPrefix(c.Name, "shape/") {
			continue
		}

		// A tab between an anchor and its node is not a mislabel either way:
		// the corpus is right that the document should be read, and the library
		// loses the anchor. Mislabelled counts "could not find alias" as the
		// corpus's fault because a mutation can break one, and here nothing was
		// mutated. Held out until the parser is fixed -- see
		// yamlgen_test.TestDefectATabAfterANodesPropertiesIsMishandled and the
		// ledger entry decode/a-tab-after-an-anchor-loses-the-value.
		if anchorFollowedByTab(string(c.Src)) {
			continue
		}

		doc := stance.Doc{
			Name: c.Name, Src: c.Src, WellFormed: c.WellFormed,
			Opaque: c.Opaque, Tags: asTags(c.Tags),
			VerdictAt: suite.StageOf(c.VerdictAt),
		}

		want, _ := yamlcorpus.GoYAML.Expect(doc)
		if want == stance.Undecided {
			unscored++

			continue
		}

		scored++

		if _, err := readStream(c.Src); err != nil && want == stance.Accept && Mislabelled(err.Error()) {
			accused++

			if accused < 4 {
				t.Errorf("%s: expected to be read, and a correct parser refuses it: %v", c.Name, summarize(err))
			}
		}
	}

	t.Logf("%d cases scored at construct, %d left unscored, %d accusations", scored, unscored, accused)

	if accused > 0 {
		t.Errorf("%d cases still accuse a correct parser", accused)
	}
}

// anchorFollowedByTab reports whether an anchor name ends at a tab.
//
// Crude on purpose: it wants "&" then a name then a tab, anywhere. A document
// wrongly held out here is one the rest of the suite still scores, where one let
// through reports a defect of ours as a fault of the corpus.
func anchorFollowedByTab(src string) bool {
	for i := 0; i+1 < len(src); i++ {
		if src[i] != '&' {
			continue
		}

		for j := i + 1; j < len(src); j++ {
			if src[j] == '\t' {
				return true
			}

			if src[j] == ' ' || src[j] == '\n' || src[j] == '\r' {
				break
			}
		}
	}

	return false
}

// Mislabelled reports whether a complaint is the corpus's fault rather than the
// library's.
//
// All three are rules a grammar cannot express and a mutation can break: an
// alias with no anchor, a duplicate key, a key that resolves to nothing. The
// corpus records what the grammar said, which for these documents is not the
// whole answer.
func Mislabelled(complaint string) bool {
	for _, s := range []string{"could not find alias", "already defined", "undefined map key"} {
		if strings.Contains(complaint, s) {
			return true
		}
	}

	return false
}

// complaints groups a replay's disagreements by what the library said, and
// separates the ones that are the corpus's fault.
func complaints(cases []suite.Case) (all, clean map[string]int) {
	all, clean = map[string]int{}, map[string]int{}

	for _, c := range cases {
		doc := stance.Doc{
			Name: c.Name, Src: c.Src, WellFormed: c.WellFormed,
			Opaque: c.Opaque, Tags: asTags(c.Tags),
			VerdictAt: suite.StageOf(c.VerdictAt),
		}

		// The parser is asked about the verdict and the decoder about the
		// value, because those are the questions each of them answers. A
		// verdict scored against the decoder cannot tell a syntax refusal from
		// a resolution refusal, and a corpus that recorded the grammar's
		// verdict is talking about syntax.
		want, _ := yamlcorpus.GoYAMLParser.Expect(doc)
		if want == stance.Undecided {
			continue
		}

		_, perr := parser.ParseBytes(c.Src, parser.WithComments())

		if (want == stance.Accept) == (perr == nil) {
			if perr != nil || c.Meaning == nil || len(c.Meaning.JSON) == 0 {
				continue
			}

			value, derr := readStream(c.Src)
			if derr != nil {
				all["the value cannot be read: "+summarize(derr)]++
				clean["the value cannot be read: "+summarize(derr)]++

				continue
			}

			if got, merr := json.Marshal(value); merr != nil || !bytes.Equal(got, c.Meaning.JSON) {
				all["the value differs"]++
				clean["the value differs"]++
			}

			continue
		}

		err := perr

		kind := "accepts a document the grammar refuses"
		if want == stance.Accept {
			kind = "refuses: " + summarize(err)
		}

		all[kind]++

		if !Mislabelled(kind) {
			clean[kind]++
		}
	}

	return all, clean
}

// summarize reduces an error to its first line without the position, so that
// the same complaint about different documents groups.
func summarize(err error) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	if i := strings.IndexByte(msg, ']'); i >= 0 && i < 12 {
		msg = msg[i+1:]
	}

	return strings.TrimSpace(msg)
}

// readStream decodes every document in a stream, returning the first one's
// value.
//
// The whole stream, because a decode of one document says nothing about a
// pattern whose anchor and alias are in different documents.
func readStream(src []byte) (any, error) {
	dec := codec.NewDecoder(bytes.NewReader(src))

	var (
		first any
		got   bool
	)

	for {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return first, nil
		}

		if err != nil {
			return nil, err
		}

		if !got {
			first, got = v, true
		}
	}
}

func asTags(names []string) []stance.Tag {
	out := make([]stance.Tag, 0, len(names))
	for _, n := range names {
		out = append(out, stance.Tag(n))
	}

	slices.Sort(out)

	return out
}
