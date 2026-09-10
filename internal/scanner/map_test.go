// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenizeBlockMappings checks the ":" map.go scans, and the "-" scanner.scanSequence scans under it.
//
// The cases cover a tab after the colon, nesting, blank lines inside a value, and trailing spaces on a key line.
func TestTokenizeBlockMappings(t *testing.T) {
	t.Parallel()

	runCases(t, []testscanner.Case{
		{
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
		{
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
