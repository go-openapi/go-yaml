// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenizeBlockMappings checks the ":" map.go scans, and the "-" scanner.scanSequence scans under it.
//
// The cases cover a tab after the colon, nesting, blank lines inside a value, and trailing spaces on a key line.
func TestTokenizeBlockMappings(t *testing.T) {
	t.Parallel()

	runCases(t, blockMappingTestCases())
}

// TestAMergeKeyTakesATabAsSeparation checks that a tab around a merge key's ':' separates as a space does.
//
// s-separate-in-line is s-white+, and s-white is a space or a tab.
// cursor.isMergeKey accepted a space alone, so "<<:\t{a: 1}" and "<<\t: {a: 1}" scanned "<<" as a plain string,
// and the parser read an ordinary key where "<<: {a: 1}" merges.
func TestAMergeKeyTakesATabAsSeparation(t *testing.T) {
	t.Parallel()

	isMergeKey := func(tk token.Token) bool { return tk.Type == token.MergeKeyType }

	t.Run("a tab before or after the ':' leaves a merge key", func(t *testing.T) {
		for _, src := range []string{
			"<<: {a: 1}\n",
			"<<:\t{a: 1}\n",
			"<<\t: {a: 1}\n",
			"<< \t : {a: 1}\n",
			"<<\t:\t*a\n",
			"<<:\t\n  a: 1\n",
			"{<<:\t{a: 1}}\n",
			"k:\n  <<\t: {a: 1}\n",
			"- <<\t: {a: 1}\n",
		} {
			tokens, err := scanTokens(src)
			require.NoErrorf(t, err, "%q", src)
			assert.Truef(t, slices.ContainsFunc(tokens, isMergeKey), "%q", src)
		}
	})

	t.Run("text in front of the '<<' leaves a plain scalar", func(t *testing.T) {
		// Scanner.scanMergeKey runs at every '<', and cursor.isMergeKey reads only what follows it.
		// Without a look at the buffer, "a<<: 1" was cut into "a" and a merge key, and the parser lost the "a".
		for _, tc := range []struct{ src, key string }{
			{"a<<: 1\n", "a<<"},
			{"<<<: 1\n", "<<<"},
			{"x <<: {a: 1}\n", "x <<"},
			{"x\t<<: 1\n", "x\t<<"},
			{"[a<<: 1]\n", "a<<"},
		} {
			tokens, err := scanTokens(tc.src)
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Falsef(t, slices.ContainsFunc(tokens, isMergeKey), "%q", tc.src)
			assert.Truef(t, slices.ContainsFunc(tokens, func(tk token.Token) bool { return tk.Value == tc.key }),
				"no key %q in %q", tc.key, tc.src)
		}
	})

	t.Run("a merge key after another entry is still one", func(t *testing.T) {
		for _, src := range []string{
			"k: v\n<<: {a: 1}\n",
			"k:\n  a: 1\n  <<: {b: 2}\n",
			"- a\n- <<: {b: 1}\n",
			"{a: 1, <<: {b: 2}}\n",
			"k: v\n\n<<: {a: 1}\n",
			"k: v # c\n<<: {a: 1}\n",
		} {
			tokens, err := scanTokens(src)
			require.NoErrorf(t, err, "%q", src)
			assert.Truef(t, slices.ContainsFunc(tokens, isMergeKey), "%q", src)
		}
	})

	t.Run("anything else after the '<<' leaves a plain scalar", func(t *testing.T) {
		// A ':' followed by anything but a blank is ns-plain-char, and only the first ':' is asked: "<<:: 1" is the
		// key "<<:". Read on to the second ':', the scan cut a merge key, and "[<<:: y]" was refused.
		for _, src := range []string{"<<x: 1\n", "<<:x\n", "<<:: 1\n", "[<<:: y]\n", "{<<:: y}\n"} {
			tokens, err := scanTokens(src)
			require.NoErrorf(t, err, "%q", src)
			assert.Falsef(t, slices.ContainsFunc(tokens, isMergeKey), "%q", src)
		}
	})
}

func blockMappingTestCases() iter.Seq[testscanner.Case] {
	return testscanner.Concat(blockMappingEntryCases(), blockMappingValueCases())
}

// blockMappingEntryCases reads a key and its value on one line, and mappings nested under a key.
func blockMappingEntryCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			Name: "entry/plain-value",
			YAML: `v: hi`,
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
					Type:   token.StringType,
					Value:  "hi",
					Origin: " hi",
				},
			},
		},
		{
			Name: "entry/tab-after-colon",
			YAML: `v:	a`,
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
					Type:  token.StringType,
					Value: "a",
					//nolint: gci
					Origin: "	a",
				},
			},
		},
		{
			Name: "entry/with-line-break",
			YAML: `hello: world
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "hello",
					Origin: "hello",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "world",
					Origin: " world",
				},
			},
		},
		{
			Name: "nested/one-space-indent",
			YAML: `
a:
 b: c
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "\n b",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: " c",
				},
			},
		},
		{
			Name: "nested/keys-out-of-order-with-submapping",
			YAML: `
b: 2
a: 1
d: 4
c: 3
sub:
  e: 5
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "\nb",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2\n",
				},
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
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1\n",
				},
				{
					Type:   token.StringType,
					Value:  "d",
					Origin: "d",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "4",
					Origin: " 4\n",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: "c",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "3",
					Origin: " 3\n",
				},
				{
					Type:   token.StringType,
					Value:  "sub",
					Origin: "sub",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "\n  e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "5",
					Origin: " 5",
				},
			},
		},
	})
}

// blockMappingValueCases reads a value written on the lines below its key: a block sequence, or a plain scalar running over several lines.
func blockMappingValueCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			Name: "sequence-value/two-strings",
			YAML: `
v:
- A
- B
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "\nv",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n-",
				},
				{
					Type:   token.StringType,
					Value:  "A",
					Origin: " A\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.StringType,
					Value:  "B",
					Origin: " B",
				},
			},
		},
		{
			Name: "sequence-value/mixed-with-nested-mapping",
			YAML: `
v:
- A
- 1
- B:
 - 2
 - 3
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "\nv",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n-",
				},
				{
					Type:   token.StringType,
					Value:  "A",
					Origin: " A\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.StringType,
					Value:  "B",
					Origin: " B",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n -",
				},
				{
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2\n ",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.IntegerType,
					Value:  "3",
					Origin: " 3",
				},
			},
		},
		{
			Name: "multi-line-plain/blank-line-inside",
			YAML: `
a:
 b

 c
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "b\nc",
					Origin: "\n b\n\n c",
				},
			},
		},
		{
			Name: "multi-line-plain/trailing-spaces-and-blank-lines",
			YAML: `
a:   
 b   

  
 c
 d 
e: f
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "b\nc d",
					Origin: "   \n b   \n\n  \n c\n d \n",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "f",
					Origin: " f",
				},
			},
		},
	})
}
