// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"
)

const stored = "testdata/json-smoke.jsonl.gz"

// TestTheStoredCorpusIsWhatTheOracleWouldSayNow is regenerate-and-diff, and it
// is the guard that keeps this whole idea from going quietly wrong.
//
// An artifact is a cache of an oracle's verdicts. A cache that is never
// compared against the thing it caches is a set of claims nobody is checking,
// and the way this design fails expensively is a frozen corpus freezing an
// oracle *bug* -- fixtures accusing a parser of defects it does not have, which
// a live oracle would have corrected the day the bug was fixed.
//
// So the corpus is rebuilt from its seed and compared byte for byte. A
// difference means either the grammar moved or the generator did, and both are
// things to look at rather than to absorb.
func TestTheStoredCorpusIsWhatTheOracleWouldSayNow(t *testing.T) {
	stored, err := os.ReadFile(stored)
	if err != nil {
		t.Fatalf("no stored corpus: %v", err)
	}

	var built bytes.Buffer
	if err := jsoncorpus.Smoke().Write(&built); err != nil {
		t.Fatal(err)
	}

	want, err := suite.Content(stored)
	if err != nil {
		t.Fatal(err)
	}

	got, err := suite.Content(built.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(want, got) {
		return
	}

	t.Errorf("the stored corpus is not what the oracle says now (%s).\n"+
		"Either the grammar changed, or the generator did. Look at which before regenerating:\n"+
		"    go test -run TestRegenerate ./json/ -args -jsoncorpus.write",
		firstDifference(want, got))
}

// TestRegenerate rewrites the stored corpus, behind a flag so it cannot happen
// by accident.
//
// Regenerating is how a real change is absorbed, and it has to be deliberate:
// a corpus that rewrote itself whenever it disagreed would make
// TestTheStoredCorpusIsWhatTheOracleWouldSayNow incapable of ever failing.
func TestRegenerate(t *testing.T) {
	if !*writeCorpus {
		t.Skip("pass -jsoncorpus.write to rewrite the stored corpus")
	}

	var buf bytes.Buffer
	if err := jsoncorpus.Smoke().Write(&buf); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Clean(stored), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Logf("wrote %s, %d bytes", stored, buf.Len())
}

// TestTheEmbeddedCorpusIsUsableWithNothingElse is the claim that makes this an
// artifact rather than a generator: a consumer reads it and scores against it
// with no oracle, no generation and no fuzzing in the loop.
func TestTheEmbeddedCorpusIsUsableWithNothingElse(t *testing.T) {
	header, cases, err := jsoncorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	switch {
	case header.Grammar != jsoncorpus.JSON.Name():
		t.Errorf("the corpus was built against %q", header.Grammar)
	case header.Digest != jsoncorpus.JSON.Digest():
		t.Errorf("the corpus was built against a different grammar file:\n  stored %s\n  now    %s",
			header.Digest, jsoncorpus.JSON.Digest())
	case header.Generator != jsoncorpus.Generator:
		t.Errorf("the corpus was built by %q, this is %q", header.Generator, jsoncorpus.Generator)
	case len(cases) == 0:
		t.Fatal("the corpus is empty")
	}

	var valid, refused, opaque, tagged int

	for _, c := range cases {
		switch {
		case c.WellFormed:
			valid++
		default:
			refused++
		}

		if c.Opaque {
			opaque++
		}

		if len(c.Tags) > 0 {
			tagged++
		}
	}

	t.Logf("%d cases: %d valid, %d refused, %d opaque, %d carrying a tag",
		len(cases), valid, refused, opaque, tagged)

	if valid == 0 || refused == 0 {
		t.Error("a corpus of only one kind cannot test both directions")
	}
}

// TestTheStanceCanScoreEveryStoredCase holds the corpus to the standard the
// suite itself is held to: a case nobody can score is not a test.
func TestTheStanceCanScoreEveryStoredCase(t *testing.T) {
	header, cases, err := jsoncorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	// The vocabulary check a consumer is expected to run first. A stance that
	// has never heard of a tag would read it as absent and score a document as
	// though a question it does not understand had been settled.
	if unknown := header.Unknown(known()); len(unknown) > 0 {
		t.Errorf("the corpus uses tags this stance has not ruled on: %v", unknown)
	}

	var undecided int

	for _, c := range cases {
		doc := stance.Doc{
			Name:       c.Name,
			Src:        c.Src,
			WellFormed: c.WellFormed,
			Opaque:     c.Opaque,
			Tags:       tagsOf(c),
			VerdictAt:  suite.StageOf(c.VerdictAt),
		}

		if out, why := jsoncorpus.DefaultLexer.Expect(doc); out == stance.Undecided {
			undecided++

			if undecided < 5 {
				t.Errorf("%s cannot be scored: %s", c.Name, why)
			}
		}
	}

	if undecided > 0 {
		t.Errorf("%d of %d cases are undecidable", undecided, len(cases))
	}
}

// TestTheEnumeratedShapesSurviveMinimizing checks the shapes were not selected
// away.
//
// They are the part of the corpus no coverage signal argues for -- a byte order
// mark appears in no grammar -- so a minimizer that consulted only what the
// grammar saw would drop every one of them, and the corpus would silently stop
// saying anything about encoding.
func TestTheEnumeratedShapesSurviveMinimizing(t *testing.T) {
	_, cases, err := jsoncorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	var found int

	for _, c := range cases {
		if c.Origin.Mutation == "enumerated" {
			found++
		}
	}

	if want := len(jsoncorpus.EncodingShapes()); found != want {
		t.Errorf("%d of %d enumerated shapes are in the corpus", found, want)
	}
}

func tagsOf(c suite.Case) []stance.Tag {
	out := make([]stance.Tag, 0, len(c.Tags))
	for _, t := range c.Tags {
		out = append(out, stance.Tag(t))
	}

	return out
}

// known is the language's tag vocabulary.
//
// Not the table's declarations: a consumer legitimately declares nothing about
// a tag belonging to a stage it never reaches, and reading that silence as
// ignorance would report the lexer as incomplete for not having an opinion
// about numbers it never converts.
func known() []string { return jsoncorpus.Vocabulary().Tags() }

// firstDifference names the line two artifacts first disagree on, so that a
// failure points at a case rather than at a byte count.
func firstDifference(want, got []byte) string {
	a := bytes.Split(want, []byte("\n"))
	b := bytes.Split(got, []byte("\n"))

	for i := range min(len(a), len(b)) {
		if bytes.Equal(a[i], b[i]) {
			continue
		}

		return fmt.Sprintf("line %d of %d differs:\n  stored: %s\n  now:    %s",
			i+1, len(a), truncate(a[i]), truncate(b[i]))
	}

	return fmt.Sprintf("%d lines stored, %d regenerated", len(a), len(b))
}

// truncate keeps a differing line short enough to read.
func truncate(line []byte) []byte {
	const width = 240
	if len(line) <= width {
		return line
	}

	return append(line[:width:width], "..."...)
}
