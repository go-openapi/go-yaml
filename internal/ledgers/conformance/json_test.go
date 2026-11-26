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
	"github.com/go-openapi/go-yaml/internal/ledgers"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// jsonLedger records the cases whose JSON departs from the fixture's on purpose, with why.
//
// Both are settled, not defects. The fixture's in.json ignores the tag, and ToJSON honours it:
//
//   - construct-binary: the fixture keeps the line breaks of a "!!binary" written as a literal block, and ToJSON writes
//     the canonical base64 through [token.CanonicalBase64]. Both texts encode the same bytes.
//   - spec-example-2-26-ordered-mappings: the fixture writes an "!!omap" as the array of one-entry objects the document
//     spells it with, and ToJSON writes the ordered object the tag names. An "!!omap" holds no repeated key, so the
//     object keeps everything the array held.
//
// The decoder answers both cases the same way (reasonBinaryCanonical and reasonOrderedMap in decode_test.go), and
// jsonTokenLedger carries the same two for ToJSONTokens.
//
// The ledger is a two-way ratchet, as the others in this package are. A case that starts departing fails as a
// regression, and one that stops departing fails too, so a change of answer is recorded by editing its entry.
var jsonLedger = map[string]string{
	"construct-binary": "!!binary resolves to the bytes it encodes, where the fixture records the text",

	"spec-example-2-26-ordered-mappings": "an !!omap writes the ordered map the tag names, where the fixture writes the array that spells it",
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

			if why := disagreement(t, test, got, err); why != "" {
				diverged[test.Name] = why

				return
			}
			agreed++
		})
	}

	ledgers.CompareNames(t, "departure", diverged, jsonLedger)

	reportJSON(t, "ToJSON", scored, agreed, refused, diverged, jsonLedger)
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

// reportJSON logs a JSON harness's score: the cases that agree with the fixture, the ones ledger records as answered
// on purpose, and any other departure, which the harness has already failed on.
func reportJSON(t *testing.T, harness string, scored, agreed, refused int, diverged, ledger map[string]string) {
	t.Helper()

	var settled int
	for name := range diverged {
		if _, ok := ledger[name]; ok {
			settled++
		}
	}
	t.Logf("YAML Test Suite through %s: %d cases scored, %d agree, %d answered on purpose (a tag the fixture ignores), "+
		"%d unexplained; %d invalid documents refused",
		harness, scored, agreed, settled, len(diverged)-settled, refused)

	names := make([]string, 0, len(diverged))
	for name := range diverged {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Logf("  %s: %s", name, diverged[name])
	}
}
