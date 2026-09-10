// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"sort"
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// jsonTokenLedger records every case whose tokens do not rebuild the JSON the
// suite says, and carries the same two entries jsonLedger does.
//
// It is a two-way ratchet, as jsonLedger is: a case that starts diverging fails
// as a regression, and one that stops diverging fails too.
//
// ⚠️ Both converters read one parse, so a third entry appearing here and not in
// jsonLedger is the token converter's own -- which is the question this harness
// is here to answer.
var jsonTokenLedger = map[string]string{
	"construct-binary": "!!binary resolves to the bytes it encodes, where the fixture records the text",

	"spec-example-2-26-ordered-mappings": "an !!omap writes the ordered map the tag names, where the fixture writes the array that spells it",

	"trailing-line-of-spaces/01": "the fixture keeps a trailing line of spaces that 7.4.1 folds away",
}

// TestSuiteToJSONTokens converts every case of the YAML Test Suite through
// [codec.ToJSONTokens] and compares the JSON its tokens rebuild with the
// fixture's.
//
// It scores the same documents TestSuiteToJSON scores and against the same
// fixtures, so the two ledgers are comparable line for line. What it adds is
// the token converter's own reading of an alias and a merge: ToJSON records the
// text each anchor wrote and answers a merge by reading its own output back,
// where this follows ast.AliasNode.Target and asks ast.MergeOf. A case that
// diverges here and not there is that difference showing.
func TestSuiteToJSONTokens(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	var scored, agreed, refused int
	diverged := make(map[string]string)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			got, err := jsonFromTokens(test.InYAML)

			if test.Error {
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
				assert.NotContainsf(t, jsonTokenLedger, test.Name,
					"%s: now converts as the suite says -- if that is a fix, delete the ledger entry", test.Name)

				return
			}

			diverged[test.Name] = why
			_, known := jsonTokenLedger[test.Name]
			assert.Truef(t, known, "%s: not in the ledger: %s", test.Name, why)
		})
	}

	t.Logf("YAML Test Suite through ToJSONTokens: %d cases scored, %d agree (%.1f%%), %d diverge; %d invalid documents refused",
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

// jsonFromTokens converts src and writes the JSON its tokens stand for.
//
// It is what a caller rebuilding a document from the stream does, and it is
// written out here rather than shipped so that the harness scores the tokens
// and not a helper's reading of them: the separators a token stream leaves out
// are put back from the kinds alone.
func jsonFromTokens(src []byte) ([]byte, error) {
	tokens := codec.ToJSONTokens(src)

	var (
		out   []byte
		comma bool
	)
	for tk := range tokens.Tokens() {
		if comma && tk.Kind != codec.JSONObjectEnd && tk.Kind != codec.JSONArrayEnd {
			out = append(out, ',')
		}

		switch tk.Kind {
		case codec.JSONObjectStart:
			out, comma = append(out, '{'), false
		case codec.JSONArrayStart:
			out, comma = append(out, '['), false
		case codec.JSONObjectEnd:
			out, comma = append(out, '}'), true
		case codec.JSONArrayEnd:
			out, comma = append(out, ']'), true
		case codec.JSONKey:
			out = append(strconv.AppendQuote(out, tk.Value), ':')
			comma = false
		case codec.JSONString:
			out, comma = strconv.AppendQuote(out, tk.Value), true
		case codec.JSONNumber:
			out, comma = append(out, tk.Value...), true
		case codec.JSONBool:
			out, comma = strconv.AppendBool(out, tk.Bool), true
		default:
			out, comma = append(out, "null"...), true
		}
	}

	return out, tokens.Err()
}
