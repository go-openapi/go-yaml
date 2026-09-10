// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// TestATaggedIntegerDropsALeadingZeroItsDigitsCannotCarry holds the one
// combination that made the generator write a document no reader accepts.
//
// [yamlgen.NumberLeadingZero] writes an integer's decimal digits behind a "0"
// so that the two schemas read different numbers -- "0511" is 511 under core
// and 329 under YAML 1.1. That only works while every digit is an octal one.
// The decimal 8 written that way is "08", which 1.1 reads as neither an octal
// nor a decimal integer, so it is a string; put "!!int" in front of it and the
// tag names a type its scalar is not.
//
// TestEmitParses drew exactly that at 20,000 checks on 2026-09-10 and reported
// it as a parser defect, because it consults grammar.NewRecognizer and a tag
// that fails to resolve leaves a syntactically clean document. Both yardsticks
// refuse it -- go.yaml.in/yaml/v3 with *cannot decode !!float `08` as a !!int*
// and libfyaml 1.0.0b1 with a parse failure -- so the refusal is right and the
// document was the generator's fault.
//
// The tag stays and the leading zero goes: "!!int 8" is an integer under both
// schemas. The untagged spelling is untouched too, which is the half worth
// keeping -- "09" alone is the string "09" under 1.1 and the decimal 9 under
// core, a disagreement TestTheNumberFormsMeanUnder11WhatTheLibraryReads
// states.
func TestATaggedIntegerDropsALeadingZeroItsDigitsCannotCarry(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value   int
		tag     string
		version string
		want    string
		reads   any
	}{
		"a non-octal digit drops the leading zero under a tag": {value: 8, tag: yamlgen.TagInt, version: "1.1", want: "%YAML 1.1\n---\n\"k\": !!int 8\n", reads: 8},
		"and so does the digit above it":                       {value: 9, tag: yamlgen.TagInt, version: "1.1", want: "%YAML 1.1\n---\n\"k\": !!int 9\n", reads: 9},
		"octal digits keep it under a tag":                     {value: 511, tag: yamlgen.TagInt, version: "1.1", want: "%YAML 1.1\n---\n\"k\": !!int 0511\n", reads: 329},
		"a non-octal digit keeps it untagged":                  {value: 9, tag: "", version: "1.1", want: "%YAML 1.1\n---\n\"k\": 09\n", reads: "09"},
		"octal digits keep it untagged":                        {value: 511, tag: "", version: "1.1", want: "%YAML 1.1\n---\n\"k\": 0511\n", reads: uint64(329)},
		"core keeps it under a tag, whatever the digits":       {value: 878, tag: yamlgen.TagInt, version: "", want: "\"k\": !!int 0878\n", reads: 878},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var val yamlgen.Value = yamlgen.Int{V: tc.value}
			if tc.tag != "" {
				val = yamlgen.Tagged{Tag: tc.tag, V: val}
			}

			style := yamlgen.Style{NullSpelling: "null", NumberForm: yamlgen.NumberLeadingZero, Version: tc.version}
			w := yamlgen.Write(yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "k"}, Val: val}}}, style)

			assert.Equal(t, tc.want, w.Text)

			// The point of the whole guard: whatever was written, the library
			// reads it, and reads it as what Write says it means.
			var got any
			require.NoError(t, codec.Unmarshal([]byte(w.Text), &got))
			assert.Equal(t, map[string]any{"k": tc.reads}, got)
		})
	}
}
