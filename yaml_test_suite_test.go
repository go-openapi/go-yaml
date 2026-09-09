// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package yaml_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// Why a case of the YAML Test Suite does not decode to its expected JSON.
//
// This measures the decoder. The parser is measured separately, in the
// conformance package, which is at 100% on the same fixtures -- so nothing
// below is a document the parser refuses, and the two lists no longer overlap
// at all.
//
// Three of these are decoder defects. The other twenty-nine are cases this
// harness cannot score, and they are worth separating because a count that
// mixes them says the decoder is at 92% when the cases it can actually decide
// put it at 99.2%.
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
	// noExpectationPins in zz_noexpectation_test.go states what each of the
	// nine composes to, decoded into codec.MapSlice, and
	// TestEveryFixtureStatingNoExpectationHasAPin holds the two lists to the
	// same names.
	reasonNoExpectation = "the fixture states no expectation"
	// The document decodes, to a value other than the expected JSON.
	reasonWrongValue = "decodes to a value other than the expected JSON"
	// The fixture records the scalar's text and the decoder resolves the tag on
	// it, so the two sides describe different things rather than disagreeing.
	reasonTagResolved = "the fixture records the text and the decoder resolves the tag"
	// The fixture expects a value the specification's own grammar does not
	// produce, and the implementations agree with the grammar.
	reasonFixtureDiffersFromSpec = "the fixture expects a value the grammar does not produce"
)

// The two fixtures the decoder does not match, neither of them a defect.
//
//  1. trailing-line-of-spaces/01 is not a defect. "foo: |\n  x\n   ", which
//     ends without a line break, decodes to "x\n " here and the fixture's
//     in.json records "x\n \n". The specification's grammar produces "x\n ":
//     b-chomped-last(clip) ::= b-as-line-feed | <end-of-stream>, so a literal
//     scalar running to the end of the stream ends without the break clipping
//     appends to a line that has one. go.yaml.in/yaml/v3 and PyYAML both read
//     "x\n " as well, and the sibling fixture 00 -- the same document with a
//     final break -- decodes to "x\n \n" here and everywhere.
//
//  2. construct-binary is not a defect either. "!!binary" resolves to []byte,
//     json.Marshal writes those bytes back as base64 without the line breaks
//     the literal block carried, and the fixture's in.json records the scalar
//     text with its line breaks intact. The decoder is right and the
//     comparison is the wrong one.

// decodeLedger records every case that does not decode to its expected JSON,
// with why.
//
// TestYAMLTestSuite ratchets it in both directions: a case that starts failing
// fails because it is missing from here, and one that starts passing fails
// because it is still listed.
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
	// "[5:1] value is not allowed in this context", until 8.2.2's seq-space was
	// admitted as an explicit key's body.
	//
	// syntax-character-edge-cases/02 is "!", the non-specific tag on the empty
	// node. It denotes null and was read as no document at all until the
	// decoder stopped folding every document into a value to decide whether it
	// held one; go.yaml.in/yaml/v3 hands back one document holding nil.
	//
	// What each of the nine composes to is pinned in zz_noexpectation_test.go.
	"block-mapping-with-missing-keys":                  reasonNoExpectation,
	"empty-keys-in-block-and-flow-mapping":             reasonNoExpectation,
	"empty-lines-at-end-of-document":                   reasonNoExpectation,
	"spec-example-7-3-completely-empty-flow-nodes":     reasonNoExpectation,
	"spec-example-8-18-implicit-block-mapping-entries": reasonNoExpectation,
	"spec-example-8-19-compact-block-mappings":         reasonNoExpectation,
	"syntax-character-edge-cases/00":                   reasonNoExpectation,
	"syntax-character-edge-cases/02":                   reasonNoExpectation,
	"zero-indented-sequences-in-explicit-mapping-keys": reasonNoExpectation,

	"trailing-line-of-spaces/01": reasonFixtureDiffersFromSpec,
	"construct-binary":           reasonTagResolved,
}

// scoredReasons are the reasons that mean the decoder got something wrong. The
// rest mean the fixture and this harness cannot be compared.
var scoredReasons = map[string]bool{reasonWrongValue: true}

func TestYAMLTestSuite(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	failed := make(map[string]struct{}, len(decodeLedger))

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			if err := decodeAsExpected(t, test); err != nil {
				failed[test.Name] = struct{}{}
				_, known := decodeLedger[test.Name]
				assert.Truef(t, known, "%s: not in the ledger: %v", test.Name, err)

				return
			}

			assert.NotContainsf(t, decodeLedger, test.Name,
				"%s: now decodes as expected -- if that is a fix, delete the ledger entry", test.Name)
		})
	}

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
		if err := yaml.Unmarshal(test.InYAML, &v); err == nil {
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

func reportDecode(t *testing.T, total int, failed map[string]struct{}) {
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
