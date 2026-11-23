// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"sort"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"

	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/parser"
)

var (
	blindReport = flag.String("yamlcorpus.blind", "",
		"replay the corpus against this library and write a JSON report to this path")
	blindFull = flag.Bool("yamlcorpus.blind.full", false,
		"replay the full tier rather than the stored smoke artifact")
)

// Finding is one class of disagreement between the corpus and a library.
//
// The class is the grouping key and the count is how many documents fell into
// it, because fifty documents failing the same way are one finding.
//
// Names carries every document in the class, and is what two reports are
// actually diffed on: a class name is a string built from an error message, and
// two builds of a library that reject the same document for the same reason
// routinely word it differently. The case names come from the corpus, which is
// the same artifact in both runs, so they join where the messages do not.
//
// Examples are bytes rather than names, for the reverse reason: a name is only
// resolvable by somebody holding this corpus, and the whole point of a
// reduction is to hand the document to something else.
type Finding struct {
	Class    string   `json:"class"`
	Count    int      `json:"count"`
	Examples []string `json:"examples"`
	Names    []string `json:"names"`
}

// Report is a whole replay, in a form two builds of the library can be diffed
// on.
type Report struct {
	Tier     string    `json:"tier"`
	Cases    int       `json:"cases"`
	Scored   int       `json:"scored"`
	Unscored int       `json:"unscored"`
	Valued   int       `json:"valued"`
	Findings []Finding `json:"findings"`
}

// TestBlindReplay is the blind test's instrument, and is deliberately not an
// assertion.
//
// It replays a corpus against whatever library the module resolves to and
// writes down what it found. Run against this library it reports the defects
// that are still open; run in a worktree of an older commit, where the module's
// replace directive points at that older library, it reports what the corpus
// would have said had it existed then. The difference between the two reports
// is the measurement.
//
//	go test -run TestBlindReplay ./yamlcorpus/ -args -yamlcorpus.blind=/tmp/new.json
//
// # Which consumer is asked what
//
// The verdict goes to [yamlcorpus.GoYAMLParser] and the value to
// [yamlcorpus.GoYAML], for the reason spelled out on those tables: a decoder
// reads a whole stream into Go values and cannot say at which stage it stopped,
// so a syntax refusal and a resolution refusal look the same to it. A corpus
// that recorded the grammar's verdict is talking about syntax.
//
// # What it measured, against the parser as it was before this branch
//
// Replayed against the library at the commit the branch's fixes are parented
// at, and diffed against the same replay here. Written down rather than
// asserted, for the reason [TestMeasureK] gives: an assertion would fix the
// answer to whatever this library happens to do today.
//
//	tier    cases    documents the parser got wrong   distinct complaints
//	smoke   10,102   777 then, 136 now                24 then, 21 now
//	full    78,044   5,531 then, 464 now              48 then, 42 now
//
// Sixteen of the thirty-six fixes are rediscovered, each attributed to the
// commit that flipped the document, by replaying the differing documents
// against every commit in between. What the corpus cannot reach divides in
// two, and the interesting half is small: nine are documents the generator
// cannot write -- a byte order mark, an explicit key, a flow collection as a
// key, a tag, a directive -- and every one of them the grammar labels
// correctly, so they are a generator's gap and not an oracle's. The other
// eleven are outside what a verdict and a value can see at all: eight are
// about the text a document is written back as, one is a stream whose later
// documents are dropped where only the first one's value is compared, and two
// are API surfaces a corpus never calls.
func TestBlindReplay(t *testing.T) {
	if *blindReport == "" {
		t.Skip("pass -yamlcorpus.blind=<path> to replay")
	}

	cases := corpus(t)

	report := Report{Tier: "compact", Cases: len(cases)}
	if *blindFull {
		report.Tier = "full"
	}

	byClass := map[string]*Finding{}

	note := func(class string, c suite.Case) {
		f := byClass[class]
		if f == nil {
			f = &Finding{Class: class}
			byClass[class] = f
		}

		f.Count++
		f.Names = append(f.Names, c.Name)

		// Three, because one example can be an accident of a single document
		// and a whole class is too much to read.
		if len(f.Examples) < 3 {
			f.Examples = append(f.Examples, string(c.Src))
		}
	}

	for _, c := range cases {
		doc := stance.Doc{
			Name: c.Name, Src: c.Src, WellFormed: c.WellFormed,
			Opaque: c.Opaque, Tags: asTags(c.Tags),
			VerdictAt: suite.StageOf(c.VerdictAt),
		}

		want, _ := yamlcorpus.GoYAMLParser.Expect(doc)
		if want == stance.Undecided {
			report.Unscored++

			continue
		}

		report.Scored++

		_, perr := parser.ParseBytes(c.Src, parser.WithComments())

		if (want == stance.Accept) != (perr == nil) {
			class := "accepts a document the grammar refuses"
			if want == stance.Accept {
				class = "refuses: " + summarize(perr)
			}

			note(class, c)

			continue
		}

		// A refused document denotes nothing, so there is no value to score.
		if perr != nil || c.Meaning == nil || len(c.Meaning.JSON) == 0 {
			continue
		}

		report.Valued++

		value, derr := readStream(c.Src)
		if derr != nil {
			note("the value cannot be read: "+summarize(derr), c)

			continue
		}

		if got, merr := yamlgen.MeaningJSON(value); merr != nil || !bytes.Equal(got, c.Meaning.JSON) {
			note("the value differs", c)
		}
	}

	for _, f := range byClass {
		report.Findings = append(report.Findings, *f)
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		return report.Findings[i].Class < report.Findings[j].Class
	})

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(*blindReport, out, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Logf("%s tier: %d cases, %d scored, %d unscored, %d valued, %d distinct complaints",
		report.Tier, report.Cases, report.Scored, report.Unscored, report.Valued, len(report.Findings))

	for _, f := range report.Findings {
		t.Logf("  %4d  %s", f.Count, f.Class)
	}
}

func corpus(t *testing.T) []suite.Case {
	t.Helper()

	if !*blindFull {
		_, cases, err := yamlembed.CompactSuite()
		if err != nil {
			t.Fatal(err)
		}

		return cases
	}

	var buf bytes.Buffer

	build := yamlbuild.Full()
	if err := build.Write(&buf); err != nil {
		t.Fatal(err)
	}

	_, cases, err := suite.FromBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	return cases
}
