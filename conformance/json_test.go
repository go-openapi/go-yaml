// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// jsonLedger records every case whose JSON does not match what the suite says.
//
// Both entries are the two the decoder harness at the repository root also
// carries, for the same reasons, which is the useful part: two converters
// sharing a parser and disagreeing with the suite in exactly the same two
// places says the disagreement is the parser's reading and not a converter's.
//
// The ledger is a two-way ratchet, as the others in this package are. A case
// that starts diverging fails as a regression, and one that stops diverging
// fails too, so a fix is recorded by deleting its entry.
var jsonLedger = map[string]string{
	"construct-binary": "!!binary resolves to the bytes it encodes, where the fixture records the text",

	"spec-example-2-26-ordered-mappings": "an !!omap writes the ordered map the tag names, where the fixture writes the array that spells it",

	"trailing-line-of-spaces/01": "the fixture keeps a trailing line of spaces that 7.4.1 folds away",
}

// TestSuiteToJSON converts every case of the YAML Test Suite and compares the
// JSON with the fixture's.
//
// It sits between the two harnesses either side of it. TestSuiteAcceptance asks
// only whether a document is taken or refused, which says nothing about what it
// was read as; the harness at the repository root reads values through the
// decoder, so a parser defect reaches it only where the decoder exposes one.
// codec.ToJSON runs on parser.Walk and touches no reflection, so this scores
// what the parse made of a document with the least between the two.
//
// It is also the only thing that walks all 402 cases. The workloads the
// performance work measures are six documents; the shapes that broke Walk this
// far -- a tag on a collection, a "?" key, an anchor standing as a key -- are
// here and not there.
//
// ToJSON converts the first document of a stream, so a case with several is
// scored on its first alone.
func TestSuiteToJSON(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	var scored, agreed, refused int
	diverged := make(map[string]string)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			got, err := codec.ToJSON(test.InYAML)

			if test.Error {
				// Parsed anyway, so a panic here still fails the run.
				if assert.Errorf(t, err, "invalid document was converted to %s", got) {
					refused++
				}

				return
			}
			if len(test.InJSON) == 0 {
				return
			}
			scored++

			why := disagreement(t, test, got, err)
			if why == "" {
				agreed++
				assert.NotContainsf(t, jsonLedger, test.Name,
					"%s: now converts as the suite says -- if that is a fix, delete the ledger entry", test.Name)

				return
			}

			diverged[test.Name] = why
			_, known := jsonLedger[test.Name]
			assert.Truef(t, known, "%s: not in the ledger: %s", test.Name, why)
		})
	}

	reportJSON(t, scored, agreed, refused, diverged)
}

// disagreement says how the JSON differs from the fixture's, or "" where it
// does not.
func disagreement(t *testing.T, test *yamltestsuite.TestSuite, got []byte, err error) string {
	t.Helper()

	if err != nil {
		return "refused: " + err.Error()
	}

	want, err := json.Marshal(test.InJSON[0])
	if err != nil {
		return "the fixture's own value does not encode: " + err.Error()
	}

	// Both sides go through the same encoder, so key order and spacing are the
	// encoder's either way and only the values are compared.
	wantText, err := reencode(want)
	if err != nil {
		return "the fixture's own JSON does not parse: " + err.Error()
	}
	gotText, err := reencode(got)
	if err != nil {
		return "wrote " + string(got) + ", which is not JSON"
	}
	if wantText != gotText {
		return "wrote " + gotText + ", and the fixture says " + wantText
	}

	return ""
}

// reencode reads JSON and writes it again, so that two texts holding the same
// values compare equal.
func reencode(text []byte) (string, error) {
	var v any
	if err := json.Unmarshal(text, &v); err != nil {
		return "", err
	}
	out, err := json.Marshal(v)

	return string(out), err
}

func reportJSON(t *testing.T, scored, agreed, refused int, diverged map[string]string) {
	t.Helper()

	t.Logf("YAML Test Suite through ToJSON: %d cases scored, %d agree (%.1f%%), %d diverge; %d invalid documents refused",
		scored, agreed, 100*float64(agreed)/float64(scored), len(diverged), refused)

	names := make([]string, 0, len(diverged))
	for name := range diverged {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Logf("  %s: %s", name, diverged[name])
	}
}
