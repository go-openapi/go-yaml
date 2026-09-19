// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
)

// TestAFeatureNeverChangesAnOutcome keeps Expect blind to the feature list.
//
// The same document with every feature the vocabulary has and with none of them
// must score identically, or a feature has become a tag by the back door.
func TestAFeatureNeverChangesAnOutcome(t *testing.T) {
	quiet := tolerant("a consumer that has ruled on nothing")

	bare := verbatim()

	loaded := verbatim()
	loaded.Features = []stance.Feature{
		"presentation/flow-collection", "break/crlf", "node/anchor", "value/mapping",
	}

	bareOut, bareWhy := quiet.Expect(bare)

	loadedOut, loadedWhy := quiet.Expect(loaded)
	if bareOut != loadedOut || bareWhy != loadedWhy {
		t.Errorf("features changed the expectation: %s (%s) became %s (%s)",
			bareOut, bareWhy, loadedOut, loadedWhy)
	}
}

// TestFeaturesSelectRatherThanExcuse exercises the filters a consumer uses
// instead of declaring a position it does not hold.
func TestFeaturesSelectRatherThanExcuse(t *testing.T) {
	docs := []stance.Doc{
		{Name: "plain", Features: []stance.Feature{"value/string"}},
		{Name: "folded", Features: []stance.Feature{"presentation/block-folded", "value/string"}},
		{Name: "folded and CRLF", Features: []stance.Feature{"presentation/block-folded", "break/crlf"}},
	}

	if got := stance.With(docs, "presentation/block-folded"); len(got) != 2 {
		t.Errorf("With kept %d docs, want 2", len(got))
	}

	if got := stance.With(docs, "presentation/block-folded", "break/crlf"); len(got) != 1 {
		t.Errorf("With on two features kept %d docs, want 1", len(got))
	}

	if got := stance.Without(docs, "presentation/block-folded"); len(got) != 1 {
		t.Errorf("Without kept %d docs, want 1", len(got))
	}

	want := []stance.Feature{"break/crlf", "presentation/block-folded", "value/string"}

	got := stance.FeaturesOf(docs)
	if len(got) != len(want) {
		t.Fatalf("FeaturesOf reported %v, want %v", got, want)
	}

	for i, f := range want {
		if got[i] != f {
			t.Errorf("FeaturesOf is not sorted: %v, want %v", got, want)

			break
		}
	}
}
