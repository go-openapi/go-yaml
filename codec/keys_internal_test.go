// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"testing"

	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAnAllowedRepeatKeepsOneEntryOnEveryPath covers a mapping read under the
// option that allows a key to repeat.
//
// Every reader keeps one entry of a repeated key, and which one depends on how
// it reads. A decoder fills a map, so the last entry written stands. A
// converter writes JSON as it goes, and JSON names a member once, so the first
// entry stands and the later ones are dropped -- as the encoder writes a map
// under SkipDuplicateMapKey. TestAnAllowedOrderedMapRepeatKeepsOneEntryOnEveryPath
// covers an "!!omap".
func TestAnAllowedRepeatKeepsOneEntryOnEveryPath(t *testing.T) {
	for src, want := range map[string]struct{ first, last string }{
		"a: 1\nb: 2\na: 3\n":                {`{"a":1,"b":2}`, `{"a":3,"b":2}`},
		"{a: 1, b: 2, a: 3}\n":              {`{"a":1,"b":2}`, `{"a":3,"b":2}`},
		"a: 1\n? a\n: 3\n":                  {`{"a":1}`, `{"a":3}`},
		"a: 1\n&x a: 3\n":                   {`{"a":1}`, `{"a":3}`},
		"a: 1\n!!str a: 3\n":                {`{"a":1}`, `{"a":3}`},
		"a: 1\nb: {c: [1, 2]}\na: {d: 3}\n": {`{"a":1,"b":{"c":[1,2]}}`, `{"a":{"d":3},"b":{"c":[1,2]}}`},
		"a: 1\na: &y 2\nb: *y\n":            {`{"a":1,"b":2}`, `{"a":2,"b":2}`},
		// A "<<" written twice is a repeat as well, and the entry after it
		// is not one. YAML 1.1 defines the merge key.
		"%YAML 1.1\n---\na: &a {x: 1}\nb:\n  <<: *a\n  <<: *a\n  c: 2\n": {
			`{"a":{"x":1},"b":{"c":2,"x":1}}`, `{"a":{"x":1},"b":{"c":2,"x":1}}`,
		},
		// An alias to a mapping that repeats a key, which the tokens read off
		// the tree.
		"a: &m {x: 1, x: 2}\nb: *m\n": {`{"a":{"x":1},"b":{"x":1}}`, `{"a":{"x":2},"b":{"x":2}}`},
		// Two keys to YAML and one member to JSON: the decoders hold both.
		"1: a\n\"1\": b\n": {`{"1":"a"}`, ``},
	} {
		t.Run(src, func(t *testing.T) {
			allow := parser.WithAllowDuplicateMapKey()

			js, err := ToJSON([]byte(src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, want.first, string(js), "ToJSON keeps the first entry")

			toks, err := collectJSONTokens([]byte(src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, want.first, string(rebuildJSON(toks)), "the tokens keep the first entry")

			if want.last == "" {
				return
			}
			for name, opts := range map[string][]DecodeOption{
				"walk": {AllowDuplicateMapKey()},
				"tree": {AllowDuplicateMapKey(), UseOrderedMap()},
			} {
				var v any
				require.NoErrorf(t, Unmarshal([]byte(src), &v, opts...), "%s", name)
				got, err := Marshal(v, JSON())
				require.NoError(t, err)
				assert.JSONEqf(t, want.last, string(got), "the %s keeps the last entry", name)
			}
		})
	}
}
