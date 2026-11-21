// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestWithJSONSyntaxAgreesWithRFC8259 reads every document of the JSON smoke corpus under parser.WithJSONSyntax and
// checks the verdict against the RFC 8259 grammar's. A document carrying an implementation-defined property (an
// encoding question, an out-of-range number, a lone surrogate escape, deep nesting) is read but not scored, and nor
// is one the oracle could not read as text.
func TestWithJSONSyntaxAgreesWithRFC8259(t *testing.T) {
	_, cases, err := jsoncorpus.SmokeSuite()
	require.NoError(t, err)

	var scored, accepted int
	for _, c := range cases {
		_, err := parser.ParseBytes(c.Src, parser.WithJSONSyntax())
		if c.Opaque || len(c.Tags) > 0 {
			continue
		}
		scored++

		if c.WellFormed {
			accepted++
			if err != nil {
				t.Errorf("%s: JSON rejected: %v\n%q", c.Name, err, c.Src)
			}

			continue
		}
		if err == nil {
			t.Errorf("%s: not JSON, accepted:\n%q", c.Name, c.Src)

			continue
		}
		if !errors.Is(err, yamlerrors.ErrNotJSONSyntax) {
			t.Errorf("%s: rejected as %v, want ErrNotJSONSyntax", c.Name, err)
		}
	}

	t.Logf("%d of %d documents scored, %d of them JSON", scored, len(cases), accepted)
	require.NotZero(t, accepted)
	require.NotZero(t, scored-accepted)
}

// TestFromJSONRoundTripsTheJSONCorpus converts every JSON document of the smoke corpus to YAML with codec.FromJSON,
// back with codec.ToJSON, and compares the two JSON texts as values, numbers by their text. A document carrying an
// implementation-defined property is converted and its outcome counted, not scored.
func TestFromJSONRoundTripsTheJSONCorpus(t *testing.T) {
	_, cases, err := jsoncorpus.SmokeSuite()
	require.NoError(t, err)

	var scored, untagged int
	for _, c := range cases {
		if !c.WellFormed || c.Opaque {
			continue
		}
		yml, err := codec.FromJSON(c.Src)
		if len(c.Tags) > 0 {
			continue
		}
		untagged++
		if err != nil {
			t.Errorf("%s: %v\n%q", c.Name, err, c.Src)

			continue
		}
		back, err := codec.ToJSON(yml)
		if err != nil {
			t.Errorf("%s: the YAML does not convert back: %v\n%s", c.Name, err, yml)

			continue
		}
		scored++
		assert.Equal(t, decodeJSON(t, c.Src), decodeJSON(t, back), "%s via\n%s", c.Name, yml)
	}

	t.Logf("%d of %d JSON documents round-tripped", scored, untagged)
	require.NotZero(t, scored)
}

func decodeJSON(t *testing.T, src []byte) any {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var v any
	require.NoError(t, dec.Decode(&v), "%q", src)

	return v
}
