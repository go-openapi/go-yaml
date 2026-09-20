// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// exponentNumber matches a number written with an exponent, which is how a
// float past float64's range is spelled.
var exponentNumber = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?[eE][-+]?[0-9]+$`)

// TestAWideFloatIsAJSONNumberInTheMeaning covers what the corpus publishes for
// a float past float64's range.
//
// A *big.Float carries MarshalText, so encoding/json wrote one as a quoted
// string and the artifact stated that "-9.948e-328" denotes text where the
// document holds a number. JSON numbers are digits of unbounded length, so
// nothing about the value asked for that.
func TestAWideFloatIsAJSONNumberInTheMeaning(t *testing.T) {
	t.Parallel()

	over, _, err := big.ParseFloat("1e400", 10, 64, big.ToNearestEven)
	require.NoError(t, err)
	under, _, err := big.ParseFloat("-9.948e-328", 10, 64, big.ToNearestEven)
	require.NoError(t, err)
	wide, ok := new(big.Int).SetString("12345678901234567890123456789", 10)
	require.True(t, ok)

	for name, tc := range map[string]struct {
		value any
		want  string
	}{
		"a float past the range":  {value: over, want: `{"k":1e+400}`},
		"a float under it":        {value: under, want: `{"k":-9.948e-328}`},
		"an integer past a word":  {value: wide, want: `{"k":12345678901234567890123456789}`},
		"a float that fits":       {value: 1.5, want: `{"k":1.5}`},
		"a float inside a list":   {value: []any{over}, want: `{"k":[1e+400]}`},
		"a float under a mapping": {value: map[string]any{"in": under}, want: `{"k":{"in":-9.948e-328}}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Compared as text, because encoding/json reads a JSON number into
			// a float64 and 1e400 does not fit one, so JSONEq cannot read the
			// very values under test.
			encoded, err := yamlgen.MeaningJSON(map[string]any{"k": tc.value})
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(encoded))
		})
	}
}

// TestTheStoredCorpusStatesAWideFloatAsANumber reads the cases the corpus
// labeled for a float past float64's range and checks where the digits landed.
//
// A meaning is scanned rather than compared, because the value sits anywhere in
// the document the generator drew. The scan looks only at these cases on
// purpose: the generator's string alphabet holds "1e3" and ".inf", so a
// document whose *string* spells a number is not evidence of anything.
func TestTheStoredCorpusStatesAWideFloatAsANumber(t *testing.T) {
	t.Parallel()

	_, cases, err := yamlcorpus.SmokeSuite()
	require.NoError(t, err)

	checked := 0
	for _, c := range cases {
		if c.Meaning == nil || len(c.Meaning.JSON) == 0 {
			continue
		}
		if !slices.Contains(c.Tags, string(yamlcorpus.TagFloatOutOfRange)) {
			continue
		}

		decoder := json.NewDecoder(bytes.NewReader(c.Meaning.JSON))
		decoder.UseNumber()
		var value any
		require.NoErrorf(t, decoder.Decode(&value), "%s: %s", c.Name, c.Meaning.JSON)

		checked++
		assert.Truef(t, holdsExponentNumber(value),
			"%s carries a float past the range and states %s, with the digits in no number",
			c.Name, c.Meaning.JSON)
	}

	assert.NotZero(t, checked, "no case states a meaning for a float past the range")
	t.Logf("%d cases state a float past float64's range", checked)
}

// holdsExponentNumber reports whether a decoded meaning carries a JSON number
// written with an exponent, which is how a float past the range is spelled.
func holdsExponentNumber(v any) bool {
	switch n := v.(type) {
	case json.Number:
		return exponentNumber.MatchString(n.String())
	case []any:
		return slices.ContainsFunc(n, holdsExponentNumber)
	case map[string]any:
		for _, value := range n {
			if holdsExponentNumber(value) {
				return true
			}
		}
	}

	return false
}
