// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"errors"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
	"github.com/go-openapi/testify/v2/require"

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
