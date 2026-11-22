// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"iter"
	"slices"
	"testing"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAnAllowedOrderedMapRepeatKeepsOneEntryOnEveryPath covers an "!!omap" read
// under the option that allows a key to repeat.
//
// The parser records the repeat on the sequence with the index of the entry
// that repeats the key, and every reader keeps one entry through that record. A
// converter writes JSON as it goes, so it keeps the first entry and drops the
// later ones. A decoder keeps the last value where the first entry stood, as it
// does for a mapping.
func TestAnAllowedOrderedMapRepeatKeepsOneEntryOnEveryPath(t *testing.T) {
	for tc := range anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCases() {
		t.Run(tc.src, func(t *testing.T) {
			allow := parser.WithAllowDuplicateMapKey()

			js, err := ToJSON([]byte(tc.src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, tc.first, string(js), "ToJSON keeps the first entry")

			toks, err := collectJSONTokens([]byte(tc.src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, tc.first, string(rebuildJSON(toks)), "the tokens keep the first entry")

			for path := range walkAndOrderedMapPaths(AllowDuplicateMapKey()) {
				var v any
				require.NoErrorf(t, Unmarshal([]byte(tc.src), &v, path.opts...), "%s", path.name)
				got, err := Marshal(v, JSON())
				require.NoError(t, err)
				assert.JSONEqf(t, tc.last, string(got), "the %s keeps the last value", path.name)
			}
		})
	}
}

// anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCase holds a document and the JSON a converter and a decoder give for it.
type anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCase struct {
	src   string
	first string
	last  string
}

func anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCases() iter.Seq[anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCase] {
	return slices.Values([]anAllowedOrderedMapRepeatKeepsOneEntryOnEveryPathCase{
		{src: "k: !!omap [{a: 1}, {b: 2}, {a: 3}]\n", first: `{"k":{"a":1,"b":2}}`, last: `{"k":{"a":3,"b":2}}`},
		{src: "k: !!omap\n- a: 1\n- a: 2\n", first: `{"k":{"a":1}}`, last: `{"k":{"a":2}}`},
		{src: "!!omap [{x: 1}, {x: 3}]\n", first: `{"x":1}`, last: `{"x":3}`},
		{src: "k: !!omap [{1: a}, {0x1: b}]\n", first: `{"k":{"1":"a"}}`, last: `{"k":{"1":"b"}}`},
		// An entry written as an alias, which opens no mapping of its own.
		{src: "m: &m {x: 2}\nk: !!omap [{x: 1}, *m]\n", first: `{"m":{"x":2},"k":{"x":1}}`, last: `{"m":{"x":2},"k":{"x":2}}`},
		// An alias to an anchored "!!omap", which the tokens read off the tree.
		{src: "a: &o !!omap [{x: 1}, {x: 2}]\nb: *o\n", first: `{"a":{"x":1},"b":{"x":1}}`, last: `{"a":{"x":2},"b":{"x":2}}`},
		// An entry that repeats its own key is one key written twice, and the
		// tree read it as the wrong shape.
		{src: "k: !!omap [{a: 1, a: 2}]\n", first: `{"k":{"a":1}}`, last: `{"k":{"a":2}}`},
		{src: "k: !!omap [{a: 1, a: 2}, {b: 3}]\n", first: `{"k":{"a":1,"b":3}}`, last: `{"k":{"a":2,"b":3}}`},
		{src: "o: &o !!omap [{a: 1, a: 2}]\np: *o\n", first: `{"o":{"a":1},"p":{"a":1}}`, last: `{"o":{"a":2},"p":{"a":2}}`},
	})
}

// TestAnOrderedMapKeyIsUniqueOnEveryPath holds the four readers of a document to
// one answer about the keys of an "!!omap".
//
// Section 3.2.1.1 holds an ordered map's keys to being unique as it holds a
// mapping's. Each entry of an "!!omap" is a mapping of its own, so the parser
// records a repeat across entries on the sequence, with the entry's index, and
// every reader refuses it through that record. Each reader used to compare the
// entries itself: ToJSON and the tokens compared the written JSON member names,
// so they refused "1" beside "\"1\"" as a repeat, and the tree refused an entry
// written as an alias as no entry at all.
func TestAnOrderedMapKeyIsUniqueOnEveryPath(t *testing.T) {
	for tc := range anOrderedMapKeyIsUniqueOnEveryPathCases() {
		src, want := tc.src, tc.want
		var walked any
		walkErr := Unmarshal([]byte(src), &walked)
		_, treeErr := decodedStream(src)
		_, jsonErr := ToJSON([]byte(src))
		_, tokenErr := collectJSONTokens([]byte(src))

		switch want {
		case keyKept:
			require.NoErrorf(t, walkErr, "walk: %q", src)
			require.NoErrorf(t, treeErr, "tree: %q", src)
			require.NoErrorf(t, jsonErr, "ToJSON: %q", src)
			require.NoErrorf(t, tokenErr, "tokens: %q", src)
		case keyRepeated:
			for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
				assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%s: %q", name, src)
			}
		case keyNotJSON:
			// Two keys, so the decoders hold both; one JSON member, so the
			// converters refuse it as they do for a mapping.
			require.NoErrorf(t, walkErr, "walk: %q", src)
			require.NoErrorf(t, treeErr, "tree: %q", src)
			assert.ErrorIsf(t, jsonErr, yamlerrors.ErrNotJSON, "ToJSON: %q", src)
			assert.ErrorIsf(t, tokenErr, yamlerrors.ErrNotJSON, "tokens: %q", src)
		default:
			require.FailNowf(t, "unknown expectation", "%v", want)
		}
	}
}

// orderedMapVerdict records how every reader treats an ordered map: it keeps the entries, rejects a repeated key,
// or, for a JSON reader, rejects two keys that write one JSON member.
type orderedMapVerdict int

const (
	keyKept orderedMapVerdict = iota
	keyRepeated
	keyNotJSON // two YAML keys that write one JSON member
)

// anOrderedMapKeyIsUniqueOnEveryPathCase holds an "!!omap" document and what every reader does with it.
type anOrderedMapKeyIsUniqueOnEveryPathCase struct {
	src  string
	want orderedMapVerdict
}

func anOrderedMapKeyIsUniqueOnEveryPathCases() iter.Seq[anOrderedMapKeyIsUniqueOnEveryPathCase] {
	return slices.Values([]anOrderedMapKeyIsUniqueOnEveryPathCase{
		{src: "k: !!omap [{a: 1}, {a: 2}]\n", want: keyRepeated},
		{src: "k: !!omap [a: 1, a: 2]\n", want: keyRepeated},
		{src: "k: !!omap\n- a: 1\n- a: 2\n", want: keyRepeated},
		{src: "k: !!omap [{1: a}, {0x1: b}]\n", want: keyRepeated},
		{src: "k: !!omap [{-0: a}, {0: b}]\n", want: keyRepeated},
		{src: "k: !!omap [{null: a}, {~: b}]\n", want: keyRepeated},
		{src: "a: &m {x: 1}\nk: !!omap [{x: 2}, *m]\n", want: keyRepeated},
		{src: "k: &o !!omap [{a: 1}, {a: 2}]\n", want: keyRepeated},
		{src: "k: !!omap [{a: 1, a: 2}]\n", want: keyRepeated},
		{src: "k: !!omap [{b: 0}, {a: 1, a: 2}]\n", want: keyRepeated},
		{src: "k: !!omap &o [{a: 1}, {a: 2}]\n", want: keyRepeated},
		{src: "k: !!omap &o [{a: 1}, {b: 2}]\n", want: keyKept},
		{src: "k: !!omap &o [{a: 1}, {b: 2}]\nl: *o\n", want: keyKept},
		{src: "k: !!omap [{1: a}, {\"1\": b}]\n", want: keyNotJSON},
		{src: "k: !!omap [{!!binary AA==: a}, {\"AA==\": b}]\n", want: keyNotJSON},
		{src: "k: !!omap [{1: a}, {1.0: b}]\n", want: keyKept},
		{src: "k: !!omap [{a: [{a: 1}]}, {b: 2}]\n", want: keyKept},
		{src: "k: !!omap [{a: !!omap [{a: 1}]}, {b: 2}]\n", want: keyKept},
		{src: "k: !!omap [{a: 1}, {b: 2}]\nl: !!omap [{a: 1}]\n", want: keyKept},
		{src: "k: [{a: 1}, {a: 2}]\n", want: keyKept},
		{src: "a: &m {x: 1}\nk: !!omap [{y: 2}, *m]\n", want: keyKept},
		{src: "k: !!omap [{!!timestamp 2001-12-14t21:59:43.10-05:00: a}, " +
			"{!!timestamp 2001-12-15 02:59:43.10: b}]\n", want: keyRepeated},
	})
}
