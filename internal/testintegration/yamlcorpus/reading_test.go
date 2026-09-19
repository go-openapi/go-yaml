// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"slices"
	"testing"

	"github.com/go-openapi/conformance-suites/suite"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// Claiming a reading, over the stored artifact.
//
// A verdict is the same under every reading -- "0777" is a valid document
// whichever schema is asked -- so none of this changes what a consumer is
// expected to accept. It changes what it is held to when the corpus says what
// a document denotes, which is the half of the corpus a verdict is blind to.

func storedCases(t *testing.T) (suite.Header, []suite.Case) {
	t.Helper()

	header, cases, err := yamlcorpus.SmokeSuite()
	if err != nil {
		t.Fatalf("reading the stored corpus: %v", err)
	}

	return header, cases
}

// TestEachReadingGetsItsOwnAnswer is the case the field exists for.
func TestEachReadingGetsItsOwnAnswer(t *testing.T) {
	want := map[string][2]string{
		"shape/schema/0777":  {`{"k":777}`, `{"k":511}`},
		"shape/schema/1_000": {`{"k":"1_000"}`, `{"k":1000}`},
		"shape/schema/1:30":  {`{"k":"1:30"}`, `{"k":90}`},
		"shape/schema/yes":   {`{"k":"yes"}`, `{"k":true}`},
		"shape/schema/1e3":   {`{"k":1000}`, `{"k":"1e3"}`},
		"shape/schema/0o17":  {`{"k":15}`, `{"k":"0o17"}`},
	}

	found := 0

	header, cases := storedCases(t)

	for _, c := range cases {
		answers, named := want[c.Name]
		if !named {
			continue
		}

		found++

		core, ok := header.MeaningFor(c, yamlgen.ReadingCore)
		if !ok || string(core.JSON) != answers[0] {
			t.Errorf("%s under the core schema: %q, want %q", c.Name, core.JSON, answers[0])
		}

		legacy, ok := header.MeaningFor(c, yamlgen.Reading11)
		if !ok || string(legacy.JSON) != answers[1] {
			t.Errorf("%s under YAML 1.1: %q, want %q", c.Name, legacy.JSON, answers[1])
		}
	}

	if found != len(want) {
		t.Errorf("found %d of the %d named scalars in the stored corpus", found, len(want))
	}
}

// TestAReadingTheCorpusDoesNotStateIsUnscored is the honest half.
//
// Nothing here states an answer under the JSON schema, so a consumer
// implementing one is left unscored on values rather than measured against the
// core schema's answer. Silence is the right result and the false return is
// what carries it.
func TestAReadingTheCorpusDoesNotStateIsUnscored(t *testing.T) {
	var stated int

	header, cases := storedCases(t)

	for _, c := range cases {
		if _, ok := header.MeaningFor(c, yamlgen.ReadingJSON); ok {
			stated++
		}
	}

	if stated != 0 {
		t.Errorf("%d cases claim an answer under the JSON schema, and none should", stated)
	}
}

// TestAgreementNeedsNoList checks the cheap path.
//
// Most documents mean the same thing under every reading, carry one Meaning and
// no list at all, and every consumer is entitled to that one answer. Storing
// three copies of it would triple the artifact to say nothing.
func TestAgreementNeedsNoList(t *testing.T) {
	var agreed, listed int

	header, cases := storedCases(t)

	for _, c := range cases {
		if c.Meaning == nil {
			continue
		}

		if len(c.Meanings) > 0 {
			listed++

			continue
		}

		agreed++

		if _, ok := header.MeaningFor(c, yamlgen.Reading11); !ok {
			t.Fatalf("%s agrees under every reading and YAML 1.1 got nothing", c.Name)
		}
	}

	if listed == 0 || agreed == 0 {
		t.Errorf("the corpus has %d cases that agree and %d that do not; both should be non-zero",
			agreed, listed)
	}

	t.Logf("%d cases agree under every reading, %d state one answer per reading", agreed, listed)
}

// TestAnUnweighedReadingGetsNothingEvenWhereTheOthersAgree pins the trap.
//
// The failsafe schema reads every scalar as a string, so it disagrees with the
// core schema about "k: 1" and about most of this corpus. Almost none of those
// cases carries a list of meanings, because no reading the corpus weighed
// disagrees about them -- and reading that as "every reading agrees" would hand
// a failsafe consumer the integer and mark it broken for saying "1".
//
// Header.Readings is what separates "they agree" from "nobody asked", and this
// is the failure it prevents.
func TestAnUnweighedReadingGetsNothingEvenWhereTheOthersAgree(t *testing.T) {
	header, cases := storedCases(t)

	const failsafe = "yaml-1.2-failsafe"

	if slices.Contains(header.Readings, failsafe) {
		t.Fatalf("this corpus now weighs %s, so the test needs a reading it does not", failsafe)
	}

	for _, c := range cases {
		if c.Meaning == nil {
			continue
		}

		if _, ok := header.MeaningFor(c, failsafe); ok {
			t.Fatalf("%s answered for a reading this corpus never weighed", c.Name)
		}
	}

	// The same case under a reading it does weigh still answers, or the check
	// above would pass for the wrong reason.
	var answered int

	for _, c := range cases {
		if _, ok := header.MeaningFor(c, yamlgen.Reading11); ok {
			answered++
		}
	}

	if answered == 0 {
		t.Error("no case answers under YAML 1.1, so nothing above was actually exercised")
	}
}
