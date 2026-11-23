// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// TestEveryBreakIsScoredAsARejection checks the labels reach an expectation.
//
// Being invisible to the grammar was what made these documents worthless
// before. What makes them worth generating now is that the corpus can say what
// they break, so a consumer that reads one is wrong and can be told so.
func TestEveryBreakIsScoredAsARejection(t *testing.T) {
	for _, e := range yamlbuild.Generate(1, 400, 0) {
		if len(e.Tags) == 0 {
			continue
		}

		doc := stance.Doc{
			Name: e.Name, Src: e.Src, WellFormed: true,
			VerdictAt: stance.Construct, Tags: e.Tags,
		}

		if out, why := yamlcorpus.GoYAML.Expect(doc); out != stance.Reject {
			t.Fatalf("%s is expected to be %s (%s), and it breaks %v", e.Name, out, why, e.Tags)
		}
	}
}

// TestTheLibraryCatchesTheBreaks is the payoff, and the reason a mutation
// producing no refusals from the oracle was worth keeping.
//
// These are documents the grammar cannot fault and a conforming consumer must
// refuse. Whether a parser catches them is not a question a corpus of grammar
// verdicts could have asked at all.
func TestTheLibraryCatchesTheBreaks(t *testing.T) {
	_, cases, err := yamlembed.CompactSuite()
	if err != nil {
		t.Fatal(err)
	}

	var caught, missed int

	for _, c := range cases {
		if !strings.HasPrefix(c.Name, "generated/") || len(c.Tags) == 0 {
			continue
		}

		doc := stance.Doc{
			Name: c.Name, Src: c.Src, WellFormed: c.WellFormed,
			VerdictAt: stance.Construct, Tags: asTags(c.Tags),
		}

		if want, _ := yamlcorpus.GoYAML.Expect(doc); want != stance.Reject {
			continue
		}

		if _, err := readStream(c.Src); err != nil {
			caught++

			continue
		}

		missed++

		if missed < 4 {
			t.Errorf("%s: read without complaint, and it breaks %v", c.Name, c.Tags)
		}
	}

	t.Logf("of the documents broken on purpose, the library catches %d and misses %d", caught, missed)

	if caught == 0 {
		t.Error("none were caught, which would make the family untested rather than passing")
	}
}
