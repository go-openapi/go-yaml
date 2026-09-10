// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// scanBoth reads src with the bulk skip on and off, and returns the two token
// streams rendered so a difference names itself.
func scanBoth(src string) (fast, slow []string, fastErr, slowErr error) {
	read := func() ([]string, error) {
		var s Scanner
		s.Init([]byte(src))
		var out []string
		for {
			tk, ok := s.NextToken()
			if !ok {
				break
			}
			out = append(out, renderToken(src, &tk))
		}

		return out, s.Err()
	}

	alnumFastPath = true
	fast, fastErr = read()
	alnumFastPath = false
	slow, slowErr = read()
	alnumFastPath = true

	return fast, slow, fastErr, slowErr
}

// renderToken writes everything about a token the bulk skip could get wrong:
// its type, its value, the stretch of source it was written as, and where it
// stands. The extent is the thing the first attempt at a bulk skip lost, so it
// is read back off the document rather than trusted.
func renderToken(src string, tk *token.Token) string {
	start, end := tk.Position.Offset(), tk.EndOffset()
	origin := "<out of range>"
	if start >= 0 && end >= start && int(end) <= len(src) {
		origin = src[start:end]
	}

	return string(rune(tk.Type)) + "|" + tk.Value + "|" + origin + "|" +
		itoa32(tk.Position.Line) + ":" + itoa32(tk.Position.Column) + ":" +
		itoa32(start) + ":" + itoa32(end)
}

func itoa32(i int32) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}

	return string(b)
}

// TestAlnumRunReadsWhatTheByteLoopReads decides whether the bulk skip is right.
//
// Stepping over a run of characters in one go has broken something different
// each of the three times it was tried before -- four origin recordings lost, a
// mask that borrowed across lanes, a token boundary that moved on a blank line
// -- and each was a piece of scanner state that is consistent only because it
// is advanced a character at a time. None of those was caught by a test of the
// scanner's output on a document anybody would write.
//
// So the two paths are run over the same source and every token compared: the
// type, the value, the text it was written as, and all four numbers of its
// position. A disagreement names the document and the token.
func TestAlnumRunReadsWhatTheByteLoopReads(t *testing.T) {
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	require.NotEmpty(t, seeds)

	for i, src := range seeds {
		fast, slow, fastErr, slowErr := scanBoth(src)

		if slowErr != nil {
			assert.Errorf(t, fastErr, "seed/%d: the byte loop refused %q and the bulk skip did not", i, src)
		} else {
			assert.NoErrorf(t, fastErr, "seed/%d: the bulk skip refused %q and the byte loop did not", i, src)
		}
		require.Equalf(t, slow, fast, "seed/%d: the two paths read %q differently", i, src)
	}
}

// FuzzAlnumRunMatchesTheByteLoop is the same comparison, over whatever the
// fuzzer reaches.
func FuzzAlnumRunMatchesTheByteLoop(f *testing.F) {
	seeds, err := fuzzseeds.All()
	require.NoError(f, err)
	for _, src := range seeds {
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src string) {
		fast, slow, fastErr, slowErr := scanBoth(src)
		require.Equalf(t, slowErr == nil, fastErr == nil, "the two paths disagree on whether %q reads", src)
		require.Equal(t, slow, fast)
	})
}

// scanPlain drives a Scanner from inside the package, so the tokenize cases run beside the differential test above.
//
// scanner_test.go carries the same adapter for package scanner_test. [testscanner.RunCases] takes the scan as a
// parameter because testscanner must not import the scanner: this file imports testscanner, so that would be a cycle.
func scanPlain(src string) ([]token.Token, error) {
	var s Scanner
	s.Init([]byte(src))

	tokens := make([]token.Token, 0, testscanner.EstimateTokens(src))
	for tk := range s.Tokens() {
		held := tk
		tokens = append(tokens, held)
	}

	return tokens, s.Err()
}

// TestTokenizePlainScalars checks the plain scalars plain.go steps over, and the type each one resolves to.
//
// The resolution itself happens in token.New against the current schema; these cases pin the answer the scanner
// hands back for the spellings YAML 1.1 and YAML 1.2 read differently -- "0100", "0o10", "0x_1A_2B_3C", "+0b1010".
func TestTokenizePlainScalars(t *testing.T) {
	t.Parallel()

	testscanner.RunCases(t, scanPlain, plainScalarTestCases())
}

func plainScalarTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: `null
  `,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.NullType,
					Value:  "null",
					Origin: "null\n  ",
				},
			},
		},
		{
			// The "_" digit separator is YAML 1.1's; the 1.2 core schema has none.
			YAML: `0_`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "0_",
					Origin: "0_",
				},
			},
		},
		{
			YAML: `0x_1A_2B_3C`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "0x_1A_2B_3C",
					Origin: "0x_1A_2B_3C",
				},
			},
		},
		{
			// YAML 1.1 wrote a binary integer as "0b..."; 1.2 has no such form.
			YAML: `+0b1010`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "+0b1010",
					Origin: "+0b1010",
				},
			},
		},
		{
			// A leading zero made a number octal in YAML 1.1. The 1.2 decimal form is "[-+]?
			// [0-9]+", which reads the zero and nothing into it.
			YAML: `0100`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.IntegerType,
					Value:  "0100",
					Origin: "0100",
				},
			},
		},
		{
			YAML: `0o10`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.OctetIntegerType,
					Value:  "0o10",
					Origin: "0o10",
				},
			},
		},
		{
			YAML: `0.123e+123`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.FloatType,
					Value:  "0.123e+123",
					Origin: "0.123e+123",
				},
			},
		},
		{
			YAML: `v: true`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.BoolType,
					Value:  "true",
					Origin: " true",
				},
			},
		},
		{
			YAML: `v: false`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.BoolType,
					Value:  "false",
					Origin: " false",
				},
			},
		},
		{
			YAML: `v: 10`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "10",
					Origin: " 10",
				},
			},
		},
		{
			YAML: `v: -10`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "-10",
					Origin: " -10",
				},
			},
		},
		{
			YAML: `v: 42`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "42",
					Origin: " 42",
				},
			},
		},
		{
			YAML: `v: 4294967296`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "4294967296",
					Origin: " 4294967296",
				},
			},
		},
		{
			YAML: `v: 0.1`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FloatType,
					Value:  "0.1",
					Origin: " 0.1",
				},
			},
		},
		{
			YAML: `v: 0.99`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FloatType,
					Value:  "0.99",
					Origin: " 0.99",
				},
			},
		},
		{
			YAML: `v: -0.1`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FloatType,
					Value:  "-0.1",
					Origin: " -0.1",
				},
			},
		},
		{
			YAML: `v: .inf`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.InfinityType,
					Value:  ".inf",
					Origin: " .inf",
				},
			},
		},
		{
			YAML: `v: -.inf`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.InfinityType,
					Value:  "-.inf",
					Origin: " -.inf",
				},
			},
		},
		{
			YAML: `v: .nan`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.NanType,
					Value:  ".nan",
					Origin: " .nan",
				},
			},
		},
		{
			YAML: `v: null`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.NullType,
					Value:  "null",
					Origin: " null",
				},
			},
		},
		{
			YAML: `123`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.IntegerType,
					Value:  "123",
					Origin: "123",
				},
			},
		},
		{
			YAML: `a: null`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.NullType,
					Value:  "null",
					Origin: " null",
				},
			},
		},
		{
			YAML: `
t2: 2018-01-09T10:40:47Z
t4: 2098-01-09T10:40:47Z
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "t2",
					Origin: "\nt2",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "2018-01-09T10:40:47Z",
					Origin: " 2018-01-09T10:40:47Z\n",
				},
				{
					Type:   token.StringType,
					Value:  "t4",
					Origin: "t4",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "2098-01-09T10:40:47Z",
					Origin: " 2098-01-09T10:40:47Z",
				},
			},
		},
		{
			YAML: `a: 3s`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "3s",
					Origin: " 3s",
				},
			},
		},
		{
			YAML: `a: 1.2.3.4`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "1.2.3.4",
					Origin: " 1.2.3.4",
				},
			},
		},
		{
			YAML: `a: 100.5`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FloatType,
					Value:  "100.5",
					Origin: " 100.5",
				},
			},
		},
		{
			YAML: `a: bogus`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "bogus",
					Origin: " bogus",
				},
			},
		},
		{
			YAML: `1x0`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "1x0",
					Origin: "1x0",
				},
			},
		},
		{
			YAML: `0b98765`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "0b98765",
					Origin: "0b98765",
				},
			},
		},
		{
			// 9 and 8 are not octal digits, so this was a string under YAML 1.1. Under 1.2 it is a decimal number that opens
			// with a zero.
			YAML: `098765`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.IntegerType,
					Value:  "098765",
					Origin: "098765",
				},
			},
		},
		{
			YAML: `0o98765`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "0o98765",
					Origin: "0o98765",
				},
			},
		},
	})
}
