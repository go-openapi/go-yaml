// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
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
	for tc := range anAllowedRepeatKeepsOneEntryOnEveryPathCases() {
		t.Run(tc.src, func(t *testing.T) {
			allow := parser.WithAllowDuplicateMapKey()

			js, err := ToJSON([]byte(tc.src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, tc.first, string(js), "ToJSON keeps the first entry")

			toks, err := collectJSONTokens([]byte(tc.src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, tc.first, string(rebuildJSON(toks)), "the tokens keep the first entry")

			if tc.last == "" {
				return
			}
			for path := range walkAndOrderedMapPaths(AllowDuplicateMapKey()) {
				var v any
				require.NoErrorf(t, Unmarshal([]byte(tc.src), &v, path.opts...), "%s", path.name)
				got, err := Marshal(v, JSON())
				require.NoError(t, err)
				assert.JSONEqf(t, tc.last, string(got), "the %s keeps the last entry", path.name)
			}
		})
	}
}

// anAllowedRepeatKeepsOneEntryOnEveryPathCase holds a document and the JSON a converter and a decoder give for it.
type anAllowedRepeatKeepsOneEntryOnEveryPathCase struct {
	src   string
	first string
	last  string
}

func anAllowedRepeatKeepsOneEntryOnEveryPathCases() iter.Seq[anAllowedRepeatKeepsOneEntryOnEveryPathCase] {
	return slices.Values([]anAllowedRepeatKeepsOneEntryOnEveryPathCase{
		{src: "a: 1\nb: 2\na: 3\n", first: `{"a":1,"b":2}`, last: `{"a":3,"b":2}`},
		{src: "{a: 1, b: 2, a: 3}\n", first: `{"a":1,"b":2}`, last: `{"a":3,"b":2}`},
		{src: "a: 1\n? a\n: 3\n", first: `{"a":1}`, last: `{"a":3}`},
		{src: "a: 1\n&x a: 3\n", first: `{"a":1}`, last: `{"a":3}`},
		{src: "a: 1\n!!str a: 3\n", first: `{"a":1}`, last: `{"a":3}`},
		{src: "a: 1\nb: {c: [1, 2]}\na: {d: 3}\n", first: `{"a":1,"b":{"c":[1,2]}}`, last: `{"a":{"d":3},"b":{"c":[1,2]}}`},
		{src: "a: 1\na: &y 2\nb: *y\n", first: `{"a":1,"b":2}`, last: `{"a":2,"b":2}`},
		// A "<<" written twice is a repeat as well, and the entry after it
		// is not one. YAML 1.1 defines the merge key.
		{src: "%YAML 1.1\n---\na: &a {x: 1}\nb:\n  <<: *a\n  <<: *a\n  c: 2\n", first: `{"a":{"x":1},"b":{"c":2,"x":1}}`, last: `{"a":{"x":1},"b":{"c":2,"x":1}}`},
		// An alias to a mapping that repeats a key, which the tokens read off
		// the tree.
		{src: "a: &m {x: 1, x: 2}\nb: *m\n", first: `{"a":{"x":1},"b":{"x":1}}`, last: `{"a":{"x":2},"b":{"x":2}}`},
		// Two keys to YAML and one member to JSON: the decoders hold both.
		{src: "1: a\n\"1\": b\n", first: `{"1":"a"}`, last: ``},
	})
}

// keyIdentityCases are keys of every kind a YAML key can be, for the kinds the
// corpus writes rarely or never as a key.
var keyIdentityCases = []string{
	"-0: a\n-0.0: b\n0x10: c\n0o17: d\n1e400: e\n-1e400: f\n.inf: g\n.NaN: h\n" +
		"123456789012345678901: i\n-9223372036854775808: j\n18446744073709551615: k\n" +
		"null: l\n~: m\ntrue: n\n\"q\\u0041\": o\n? |\n  lit\n: p\n<<: q\n",
	"!!float 1: a\n!!int 0x10: b\n!!str 1: c\n!!null : d\n!!bool true: e\n" +
		"!!binary AA==: f\n? !!binary |\n  AA\n  AA\n: g\n" +
		"!!timestamp 2001-12-14t21:59:43.10-05:00: h\n!!timestamp 2001-12-14: i\n" +
		"!!timestamp 2001-12-15T02:59:43.1+00:00: j\n",
	"%YAML 1.1\n---\n2001-12-14: a\n2001-12-15 2:59:43.10: b\n0b101: c\n017: d\n" +
		"1_000: e\n190:20:30: f\nyes: g\nOff: h\n1.0e+400: i\n",
}

// TestAKeyNodeAndItsValueHaveOneIdentity holds the two sides of a key's
// identity together.
//
// The parser compares a key by [ast.ComparedKeyName], read off the node. A
// merge, a MapSlice and the encoder compare a Go value by codecutils.KeyIDOf. Every key
// defect of this kind sat between two such rules, so for every scalar key the
// corpus writes, the value the key decodes to must have the identity the key's
// node has.
func TestAKeyNodeAndItsValueHaveOneIdentity(t *testing.T) {
	srcs := corpusSources()
	for i, text := range keyIdentityCases {
		srcs = append(srcs, corpusSource{name: "cases/" + string(rune('0'+i)), text: text})
	}

	var compared int
	for _, src := range srcs {
		file, err := parser.ParseBytes([]byte(src.text))
		if err != nil {
			if strings.HasPrefix(src.name, "cases/") {
				assert.NoErrorf(t, err, "%s does not parse, so its keys are compared nowhere", src.name)
			}

			continue
		}
		for _, doc := range file.Docs {
			if doc == nil || doc.Body == nil {
				continue
			}
			for _, n := range ast.Filter(ast.MappingValueType, doc.Body) {
				entry, isEntry := n.(*ast.MappingValueNode)
				if !isEntry || entry.Key == nil || entry.Key.IsMergeKey() {
					continue
				}
				name, kind, named := ast.ComparedKeyName(entry.Key, nil)
				if !named {
					continue
				}

				d := NewDecoder(bytes.NewReader(nil))
				value, err := d.mapKeyNodeToValue(entry.Key)
				if err != nil || !codecutils.HashableKey(value) {
					continue
				}
				gotKind, gotName, gotNamed := codecutils.KeyIDOf(value)
				if assert.Truef(t, gotNamed, "%s: key %q has no name as the value %#v", src.name, entry.Key.String(), value) {
					assert.Equalf(t, kind, gotKind, "%s: key %q as the value %#v", src.name, entry.Key.String(), value)
					assert.Equalf(t, name, gotName, "%s: key %q as the value %#v", src.name, entry.Key.String(), value)
				}
				compared++
			}
		}
	}

	// 11,191 on 2026-09-11. A floor near it fails when the loop stops reaching
	// the corpus, which a floor of a few keys would not notice.
	t.Logf("%d keys compared", compared)
	assert.GreaterOrEqualf(t, compared, 10000, "the loop has stopped reaching the corpus")
}
