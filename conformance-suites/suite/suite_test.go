// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package suite_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/conformance-suites/suite"
)

func sample() (suite.Header, []suite.Case) {
	return suite.Header{
		Grammar:   "RFC 8259",
		Digest:    "sha256:deadbeef",
		Generator: "jsonspike/1",
		Seed:      1,
		Tier:      "smoke",
		Cases:     3,
		Vocabulary: []suite.TagSpec{
			{Tag: "number/out-of-range", Stage: "construct"},
			{Tag: "encoding/bom", Stage: "parse"},
		},
	}, []suite.Case{
		{Name: "a", Src: []byte(`{"a":1}`), WellFormed: true, Origin: suite.Origin{Document: 0}},
		{
			Name: "b", Src: []byte("\xff\xfe{}"), Opaque: true,
			Tags: []string{"encoding/not-utf8", "encoding/utf16"}, Origin: suite.Origin{Document: 1},
		},
		{Name: "c", Src: []byte(`{"a":}`), Origin: suite.Origin{Document: 2, Mutation: "delete a run", Signature: "xyz"}},
	}
}

func write(t *testing.T) []byte {
	t.Helper()

	h, cases := sample()

	var buf bytes.Buffer

	w, err := suite.NewWriter(&buf, h)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		if err := w.Add(c); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// TestAnArtifactSurvivesTheRoundTrip is the basic claim: nothing a case carries
// is lost on the way to disk and back, including bytes that are not text.
func TestAnArtifactSurvivesTheRoundTrip(t *testing.T) {
	_, want := sample()

	gotHeader, got, err := suite.FromBytes(write(t))
	if err != nil {
		t.Fatal(err)
	}

	if gotHeader.Cases != len(want) || len(got) != len(want) {
		t.Fatalf("wrote %d cases, header says %d, read %d", len(want), gotHeader.Cases, len(got))
	}

	for i := range want {
		switch {
		case got[i].Name != want[i].Name:
			t.Errorf("case %d is named %q, was %q", i, got[i].Name, want[i].Name)
		case !bytes.Equal(got[i].Src, want[i].Src):
			t.Errorf("%s is %q, was %q", want[i].Name, got[i].Src, want[i].Src)
		case got[i].WellFormed != want[i].WellFormed:
			t.Errorf("%s came back with a different verdict", want[i].Name)
		case got[i].Opaque != want[i].Opaque:
			t.Errorf("%s came back with a different opacity", want[i].Name)
		case got[i].Origin != want[i].Origin:
			t.Errorf("%s lost its origin: %+v", want[i].Name, got[i].Origin)
		}
	}
}

// TestWritingTwiceGivesTheSameBytes is what regenerate-and-diff rests on.
//
// If the same corpus compressed differently on two runs, a diff could never
// distinguish a corpus that drifted from one that was edited, and the artifact
// would have to be trusted rather than checked.
func TestWritingTwiceGivesTheSameBytes(t *testing.T) {
	if first, again := write(t), write(t); !bytes.Equal(first, again) {
		t.Errorf("the same corpus compressed to %d bytes and then %d", len(first), len(again))
	}
}

// TestTagOrderDoesNotChangeTheBytes pins the other half of that: two runs that
// discovered the same tags in a different order are the same corpus.
func TestTagOrderDoesNotChangeTheBytes(t *testing.T) {
	h, _ := sample()
	h.Cases = 1

	render := func(tags []string) []byte {
		var buf bytes.Buffer

		w, err := suite.NewWriter(&buf, h)
		if err != nil {
			t.Fatal(err)
		}

		if err := w.Add(suite.Case{Name: "a", Src: []byte("{}"), Tags: tags}); err != nil {
			t.Fatal(err)
		}

		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		return buf.Bytes()
	}

	if a, b := render([]string{"x/one", "y/two"}), render([]string{"y/two", "x/one"}); !bytes.Equal(a, b) {
		t.Error("the order tags were found in changed the artifact")
	}
}

// TestATruncatedArtifactIsAnError checks the count is verified rather than
// trusted, so a corpus that lost cases fails instead of passing with fewer.
func TestATruncatedArtifactIsAnError(t *testing.T) {
	full := write(t)

	if _, _, err := suite.FromBytes(full[:len(full)-12]); err == nil {
		t.Error("a truncated artifact read cleanly")
	}
}

// TestAMiscountedArtifactIsRefusedOnClose catches the same thing at the writing
// end, where it is cheaper to notice.
func TestAMiscountedArtifactIsRefusedOnClose(t *testing.T) {
	h, _ := sample()
	h.Cases = 99

	var buf bytes.Buffer

	w, err := suite.NewWriter(&buf, h)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Add(suite.Case{Name: "a", Src: []byte("{}")}); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err == nil {
		t.Error("an artifact holding one case of a promised ninety-nine closed cleanly")
	}
}

// TestAnArtifactMustSayWhereItCameFrom pins the header fields that make a
// corpus checkable rather than merely readable.
func TestAnArtifactMustSayWhereItCameFrom(t *testing.T) {
	for _, missing := range []struct {
		what string
		h    suite.Header
	}{
		{"the grammar", suite.Header{Digest: "d", Tier: "smoke"}},
		{"the grammar digest", suite.Header{Grammar: "g", Tier: "smoke"}},
		{"the tier", suite.Header{Grammar: "g", Digest: "d"}},
	} {
		if _, err := suite.NewWriter(&bytes.Buffer{}, missing.h); err == nil {
			t.Errorf("an artifact naming no %s was accepted", missing.what)
		}
	}
}

// TestAConsumerCanTellWhenATagIsNewToIt is the guard that keeps a shared corpus
// honest across parsers.
func TestAConsumerCanTellWhenATagIsNewToIt(t *testing.T) {
	h, _, err := suite.FromBytes(write(t))
	if err != nil {
		t.Fatal(err)
	}

	if got := h.Unknown([]string{"encoding/bom", "number/out-of-range"}); len(got) != 0 {
		t.Errorf("a stance covering the vocabulary was told it was missing %v", got)
	}

	got := h.Unknown([]string{"encoding/bom"})
	if len(got) != 1 || got[0] != "number/out-of-range" {
		t.Errorf("a stance missing a tag was told %v", got)
	}
}

// TestTheVocabularyOrderDoesNotReachTheBytes guards a determinism hole the
// vocabulary opened when it stopped being a list of strings.
//
// Two builds that discovered the same tags in a different order have to produce
// the same artifact, or regenerate-and-diff reports a difference that is not
// one. Sorting a slice of strings was obviously right and easy to keep right; a
// slice of structs needs a comparator, and a comparator is something that can
// quietly go missing.
func TestTheVocabularyOrderDoesNotReachTheBytes(t *testing.T) {
	forward := suite.Header{
		Grammar: "RFC 8259", Digest: "sha256:deadbeef", Generator: "g", Tier: "smoke",
		Vocabulary: []suite.TagSpec{
			{Tag: "a/one", Stage: "parse"},
			{Tag: "b/two", Stage: "construct", Settled: "reject", Because: "the spec says so"},
		},
	}

	backward := forward
	backward.Vocabulary = []suite.TagSpec{forward.Vocabulary[1], forward.Vocabulary[0]}

	if a, b := headerBytes(t, forward), headerBytes(t, backward); !bytes.Equal(a, b) {
		t.Errorf("the vocabulary's order reached the bytes:\n  %s\n  %s", a, b)
	}

	// And the caller's slice is left alone, since a builder may well be holding
	// the language's vocabulary rather than a copy of it.
	if forward.Vocabulary[0].Tag != "a/one" {
		t.Error("writing reordered the caller's vocabulary in place")
	}
}

func headerBytes(t *testing.T, h suite.Header) []byte {
	t.Helper()

	var buf bytes.Buffer

	w, err := suite.NewWriter(&buf, h)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}
