// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/suite"
	yamloracle "github.com/go-openapi/conformance-suites/yaml"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

const stored = "testdata/yaml-smoke.jsonl.gz"

var writeCorpus = flag.Bool("yamlcorpus.write", false, "rewrite the stored corpus")

// TestTheStoredCorpusIsWhatTheOracleWouldSayNow is regenerate-and-diff, and it
// is the guard that keeps a cached oracle from going quietly wrong.
//
// An artifact is a cache of verdicts and labels. A cache nobody compares against
// the thing it caches is a set of claims nobody is checking, and the expensive
// failure is a frozen corpus freezing an oracle *bug* -- fixtures accusing a
// parser of defects it does not have, which a live oracle would have corrected
// the day the bug was fixed.
func TestTheStoredCorpusIsWhatTheOracleWouldSayNow(t *testing.T) {
	stored, err := os.ReadFile(stored)
	if err != nil {
		t.Fatalf("no stored corpus: %v", err)
	}

	var built bytes.Buffer
	if err := yamlcorpus.Smoke().Write(&built); err != nil {
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
		"Either the grammar changed, the generator did, or a label did. Look at which before regenerating:\n"+
		"    go test -run TestRegenerate ./internal/testintegration/yamlcorpus/ -args -yamlcorpus.write",
		firstDifference(want, got))
}

// TestRegenerate rewrites the stored corpus, behind a flag so it cannot happen
// by accident.
//
// A corpus that rewrote itself whenever it disagreed would make the test above
// incapable of ever failing.
func TestRegenerate(t *testing.T) {
	if !*writeCorpus {
		t.Skip("pass -yamlcorpus.write to rewrite the stored corpus")
	}

	var buf bytes.Buffer
	if err := yamlcorpus.Smoke().Write(&buf); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Clean(stored), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Logf("wrote %s, %d bytes", stored, buf.Len())
}

// TestTheCorpusIsUsableWithNothingElse is the claim that makes this an artifact
// rather than a generator: read it and score against it with no oracle, no
// generation and no fuzzing in the loop.
func TestTheCorpusIsUsableWithNothingElse(t *testing.T) {
	header, cases, err := yamlcorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	switch {
	case header.Grammar != yamloracle.YAML.Name():
		t.Errorf("built against %q", header.Grammar)
	case header.Digest != yamloracle.YAML.Digest():
		t.Errorf("built against a different grammar file:\n  stored %s\n  now    %s",
			header.Digest, yamloracle.YAML.Digest())
	case len(cases) == 0:
		t.Fatal("the corpus is empty")
	}

	var valid, refused, opaque, meaning, cyclic, enumerated int

	for _, c := range cases {
		if c.WellFormed {
			valid++
		} else {
			refused++
		}

		if c.Opaque {
			opaque++
		}

		if c.Meaning != nil {
			meaning++

			if c.Meaning.Cyclic {
				cyclic++
			}
		}

		if strings.HasPrefix(c.Name, "shape/") {
			enumerated++
		}
	}

	t.Logf("%d cases: %d valid, %d refused, %d opaque", len(cases), valid, refused, opaque)
	t.Logf("%d carry a meaning, %d of them cyclic; %d are enumerated shapes", meaning, cyclic, enumerated)

	if valid == 0 || refused == 0 {
		t.Error("a corpus of only one kind cannot test both directions")
	}

	if meaning == 0 {
		t.Error("no case says what it means, so the value families are not in here")
	}
}

// TestTheEnumeratedFamiliesSurviveMinimizing checks they were not selected away.
//
// They are the part no coverage signal argues for: an alias reaches the same
// productions whether it resolves or not, and every plain scalar reaches the
// same ones whatever it denotes. A minimizer consulting only what the grammar
// saw would keep one of each family and drop the rest as duplicates.
func TestTheEnumeratedFamiliesSurviveMinimizing(t *testing.T) {
	_, cases, err := yamlcorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	var found int

	for _, c := range cases {
		if strings.HasPrefix(c.Name, "shape/") {
			found++
		}
	}

	if want := len(yamlcorpus.Cases()); found != want {
		t.Errorf("%d of %d enumerated shapes are in the corpus", found, want)
	}
}

// TestTheCorpusReachesMostOfTheGrammar is the measurement that says whether any
// of this was worth generating.
//
// The denominator is computed rather than guessed -- 605 production-by-context
// buckets reachable from a stream, see yamloracle.Reach -- so the number here means
// something. A corpus that reached a third of it would be a corpus that had
// explored one corner and called it a language.
func TestTheCorpusReachesMostOfTheGrammar(t *testing.T) {
	_, cases, err := yamlcorpus.SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}

	reach := yamloracle.YAML.Reach("l-yaml-stream", "block-in")
	cover := yamloracle.YAML.NewCoverage()
	rec := yamloracle.NewRecognizer(4096)
	rec.Cover(cover)

	for _, c := range cases {
		rec.Stream(c.Src)
	}

	reached, matched, total := cover.Against(reach)
	t.Logf("%d of %d buckets entered, %d matched", reached, total, matched)

	// All of them, and asserted exactly rather than as a floor.
	//
	// A floor was right while the last few were open; now that they are closed
	// the exact number is the stronger guard, and the failure it catches is a
	// change that quietly narrows the generator -- a style axis dropped, an
	// alphabet trimmed -- which shows up here long before it shows up as a
	// missed defect.
	//
	// Reaching every bucket is not the same as covering the grammar and should
	// not be read as it. Entry is easy to saturate; matching is not, and the
	// gap between the two numbers is the part of the language the corpus makes
	// the grammar consider and never satisfies.
	if reached != total {
		t.Errorf("the corpus enters %d of %d buckets", reached, total)
	}

	// The unmatched buckets are logged rather than counted against a ceiling.
	// They move by a few whenever the generator changes, because each is
	// reached by a handful of documents and a changed draw sequence hands them
	// to different ones -- see the sweep recorded on yamlgen.drawInt. Log them
	// so a real narrowing can be told from that jitter by reading the names.
	if unmatched := cover.Unmatched(reach); len(unmatched) > 0 {
		t.Logf("entered and never matched (%d):\n  %s", len(unmatched), strings.Join(unmatched, "\n  "))
	}

	// Nothing is left out any more, so this logs nothing. It used to name
	// thirteen productions nothing entered -- nine of them tags, which the
	// emitter did not write until yamlgen.Tagged, one a %YAML directive, one an
	// explicit key and one the keep chomping indicator -- and the enumerated
	// shapes in yamlcorpus closed the rest. Kept because a narrowed generator
	// shows up here first.
	if missing := cover.Missing(reach); len(missing) > 0 {
		t.Logf("never entered (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
}

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
