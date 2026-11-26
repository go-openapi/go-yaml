// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package conformance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/testify/v2/require"
)

// Why a case of the YAML Test Suite does not decode to its expected JSON.
//
// This measures the decoder. The parser is measured separately, by
// TestSuiteAcceptance in acceptance_test.go, which is at 100% on the same fixtures -- so nothing
// below is a document the parser refuses, and the two lists no longer overlap
// at all.
//
// None of these is a decoder defect. Twenty-nine are cases this harness cannot
// score and two are answers this library gives on purpose, and they are worth
// separating from a defect because a count that mixes them says the decoder is
// at 92% when the cases it can actually decide put it at 100.0%.
const (
	// The fixture states its expectation as out.yaml -- a canonical YAML
	// document -- and carries no in.json for this harness to compare against.
	//
	// Every one of the twenty is a document whose keys JSON cannot spell: a
	// sequence key, a mapping key, or an empty key. That is why the suite
	// records no in.json for them, and why no harness comparing JSON will ever
	// score them. Scoring them needs a comparison against out.yaml, which means
	// composing both sides and comparing node trees.
	reasonStatedAsOutYAML = "expectation stated as out.yaml, which this harness does not read"
	// The fixture carries in.yaml alone: no in.json, no out.yaml, no error
	// marker. There is nothing to agree or disagree with.
	//
	// The parser harness excludes these by name, through
	// yamltestsuite.HasExpectation. This one counts them, because one of the
	// nine is a document the decoder refuses -- ": a\n: b\n", which holds one
	// empty key twice -- and dropping them silently would hide that.
	//
	// noExpectationPins in noexpectation_test.go states what each of the
	// nine composes to, decoded into codec.MapSlice, and
	// TestEveryFixtureStatingNoExpectationHasAPin holds the two lists to the
	// same names.
	reasonNoExpectation = "the fixture states no expectation"
	// The document decodes, to a value other than the expected JSON.
	reasonWrongValue = "decodes to a value other than the expected JSON"

	// The fixture records the scalar's text and the decoder resolves the tag on
	// it, so the two sides describe different things rather than disagreeing.
	// reasonTagResolved = "the fixture records the text and the decoder resolves the tag".

	// The fixture writes an "!!omap" as the array of one-entry objects the
	// document spells it with, and the decoder reads it as the ordered map the
	// tag names.
	reasonOrderedMap = "an !!omap decodes to an ordered map, not to the array that spells it"
	// The fixture keeps the line breaks RFC 2045 permits inside an encoded
	// stream, and a Base64 writes the canonical text.
	reasonBinaryCanonical = "a !!binary writes canonical base64, where the fixture keeps its line breaks"
)

// The two fixtures the decoder does not match, neither of them a defect. ToJSON
// and ToJSONTokens diverge on the same two, scored by jsonLedger and
// jsonTokenLedger.
//
//  1. construct-binary. A "!!binary" reads into a [codec.Base64] -- the
//     encoded text, canonical -- where the fixture's in.json keeps the line
//     breaks RFC 2045 permits inside an encoded stream. The characters are the
//     same and the breaks are not, so the bytes agree and the JSON text does
//     not. It resolved to []byte before, and json.Marshal wrote those bytes
//     back as base64 with no breaks either, so the case has never matched.
//
//  2. spec-example-2-26-ordered-mappings. The fixture writes an "!!omap" as
//     the array of one-entry objects the document spells it with, and the
//     decoder reads it as the ordered map the tag names.

// decodeLedger records every case that does not decode to its expected JSON,
// with why.
//
// TestYAMLTestSuite checks it with [ledgers.CompareNames], in both directions: a case
// that starts failing fails because it is missing from here, and one that starts
// passing fails because it is still listed.
var decodeLedger = map[string]string{
	// A key JSON cannot spell. The suite states what these should compose to
	// in out.yaml and records no in.json at all.
	"aliases-in-explicit-block-mapping":                reasonStatedAsOutYAML,
	"aliases-in-flow-objects":                          reasonStatedAsOutYAML,
	"anchors-on-empty-scalars":                         reasonStatedAsOutYAML,
	"empty-implicit-key-in-single-pair-flow-sequences": reasonStatedAsOutYAML,
	"flow-mapping-separate-values":                     reasonStatedAsOutYAML,
	"flow-sequence-in-flow-mapping":                    reasonStatedAsOutYAML,
	"implicit-flow-mapping-key-on-one-line":            reasonStatedAsOutYAML,
	"mapping-key-and-flow-sequence-item-anchors":       reasonStatedAsOutYAML,
	"nested-implicit-complex-keys":                     reasonStatedAsOutYAML,
	"question-mark-edge-cases/00":                      reasonStatedAsOutYAML,
	"question-mark-edge-cases/01":                      reasonStatedAsOutYAML,
	"single-character-streams/01":                      reasonStatedAsOutYAML,
	"single-pair-implicit-entries":                     reasonStatedAsOutYAML,
	"spec-example-2-11-mapping-between-sequences":      reasonStatedAsOutYAML,
	"spec-example-6-12-separation-spaces":              reasonStatedAsOutYAML,
	"spec-example-7-16-flow-mapping-entries":           reasonStatedAsOutYAML,
	"tags-on-empty-scalars":                            reasonStatedAsOutYAML,
	"various-combinations-of-explicit-block-mappings":  reasonStatedAsOutYAML,
	"various-trailing-comments":                        reasonStatedAsOutYAML,
	"various-trailing-comments-1-3":                    reasonStatedAsOutYAML,

	// in.yaml and nothing else. The decoder refuses the first of these --
	// ": a\n: b\n" holds one null key twice, which the load reports -- and
	// reads the other eight. It refused the zero-indented sequence too, with
	// "[5:1] a document holds one root node, and this stands past it", until 8.2.2's seq-space was
	// admitted as an explicit key's body.
	//
	// syntax-character-edge-cases/02 is "!", the non-specific tag on the empty
	// node, which "!" makes the empty string. It was read as no document at
	// all until the decoder stopped folding every document into a value to
	// decide whether it held one.
	//
	// What each of the nine composes to is pinned in noexpectation_test.go.
	"block-mapping-with-missing-keys":                  reasonNoExpectation,
	"empty-keys-in-block-and-flow-mapping":             reasonNoExpectation,
	"empty-lines-at-end-of-document":                   reasonNoExpectation,
	"spec-example-7-3-completely-empty-flow-nodes":     reasonNoExpectation,
	"spec-example-8-18-implicit-block-mapping-entries": reasonNoExpectation,
	"spec-example-8-19-compact-block-mappings":         reasonNoExpectation,
	"syntax-character-edge-cases/00":                   reasonNoExpectation,
	"syntax-character-edge-cases/02":                   reasonNoExpectation,
	"zero-indented-sequences-in-explicit-mapping-keys": reasonNoExpectation,

	// "!!omap" reads into a codec.MapSliceSeq, which holds the entries of the
	// sequence's one-entry mappings under the order they stood in. in.json
	// states the array the document is written as -- [{"Mark McGwire":65},...]
	// -- where ToJSON and the encoder both write the object {"Mark
	// McGwire":65,...}. JSON specifies no ordering, so neither spelling is the
	// JSON of an ordered map and the tag's meaning decides which we write.
	"spec-example-2-26-ordered-mappings": reasonOrderedMap,

	// The value decodes to a codec.Base64, which holds the base64 the document
	// wrote and writes it back with the line breaks taken out. in.json keeps
	// them, so the two texts differ by four newlines and stand for the same
	// bytes. ToJSON has written the canonical form all along --
	// json_test.go records the same departure -- and this reads it
	// through Base64.MarshalJSON, so the two agree with each other now.
	"construct-binary": reasonBinaryCanonical,
}

// scoredReasons are the reasons that mean the decoder got something wrong. The
// rest mean the fixture and this harness cannot be compared.
var scoredReasons = map[string]bool{reasonWrongValue: true}

func TestYAMLTestSuite(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	failed := make(map[string]error, len(decodeLedger))

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			if err := decodeAsExpected(t, test); err != nil {
				failed[test.Name] = err
			}
		})
	}

	ledgers.CompareNames(t, "decode failure", failed, decodeLedger)
	reportDecode(t, len(tests), failed)
}

// decodeAsExpected reports why a suite case does not decode to its expected
// JSON, or nil when it does.
func decodeAsExpected(t *testing.T, test *yamltestsuite.TestSuite) (err error) {
	t.Helper()

	defer func() {
		if e := recover(); e != nil {
			// A panic is a failure like any other here, but it is worth its own
			// message: the stack is the only useful part of it.
			err = fmt.Errorf("panic decoding %q: %v\n%s", string(test.InYAML), e, debug.Stack())
		}
	}()

	if test.Error {
		var v any
		if err := codec.Unmarshal(test.InYAML, &v); err == nil {
			return errors.New("invalid document was accepted")
		}

		return nil
	}

	dec := codec.NewDecoder(bytes.NewReader(test.InYAML))
	for idx := 0; ; idx++ {
		var v any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("decoding document %d: %w", idx, err)
		}

		if len(test.InJSON) <= idx {
			return fmt.Errorf("document %d decoded to %v, and the fixture expects nothing", idx, v)
		}

		expected, err := json.Marshal(test.InJSON[idx])
		if err != nil {
			return fmt.Errorf("encoding the expected value: %w", err)
		}
		got, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encoding the decoded value: %w", err)
		}
		if !bytes.Equal(expected, got) {
			return fmt.Errorf("document %d decoded to %s, expected %s", idx, got, expected)
		}
	}
}

func reportDecode(t *testing.T, total int, failed map[string]error) {
	t.Helper()

	passed := total - len(failed)
	t.Logf("YAML Test Suite through the decoder: %d cases, %d decode as expected (%.1f%%), %d do not",
		total, passed, 100*float64(passed)/float64(total), len(failed))

	byReason := make(map[string][]string)
	var wrong int
	for name := range failed {
		reason := decodeLedger[name]
		byReason[reason] = append(byReason[reason], name)
		if scoredReasons[reason] {
			wrong++
		}
	}

	// The number that means something about the decoder. The cases this
	// harness cannot compare are not failures, and counting them as such
	// understates the decoder by seven points.
	scored := total - (len(failed) - wrong)
	t.Logf("  of the %d cases this harness can score, %d decode as expected (%.1f%%)",
		scored, scored-wrong, 100*float64(scored-wrong)/float64(scored))

	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool { return len(byReason[reasons[i]]) > len(byReason[reasons[j]]) })

	for _, reason := range reasons {
		t.Logf("  %2d %s: %s", len(byReason[reason]), plural(len(byReason[reason]), "case"), reason)
	}
}

// plural adds an "s" to word when n is not one.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}

	return word + "s"
}
