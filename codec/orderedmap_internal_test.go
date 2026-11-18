// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
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
	for src, want := range map[string]struct{ first, last string }{
		"k: !!omap [{a: 1}, {b: 2}, {a: 3}]\n": {`{"k":{"a":1,"b":2}}`, `{"k":{"a":3,"b":2}}`},
		"k: !!omap\n- a: 1\n- a: 2\n":          {`{"k":{"a":1}}`, `{"k":{"a":2}}`},
		"!!omap [{x: 1}, {x: 3}]\n":            {`{"x":1}`, `{"x":3}`},
		"k: !!omap [{1: a}, {0x1: b}]\n":       {`{"k":{"1":"a"}}`, `{"k":{"1":"b"}}`},
		// An entry written as an alias, which opens no mapping of its own.
		"m: &m {x: 2}\nk: !!omap [{x: 1}, *m]\n": {`{"m":{"x":2},"k":{"x":1}}`, `{"m":{"x":2},"k":{"x":2}}`},
		// An alias to an anchored "!!omap", which the tokens read off the tree.
		"a: &o !!omap [{x: 1}, {x: 2}]\nb: *o\n": {`{"a":{"x":1},"b":{"x":1}}`, `{"a":{"x":2},"b":{"x":2}}`},
		// An entry that repeats its own key is one key written twice, and the
		// tree read it as the wrong shape.
		"k: !!omap [{a: 1, a: 2}]\n":           {`{"k":{"a":1}}`, `{"k":{"a":2}}`},
		"k: !!omap [{a: 1, a: 2}, {b: 3}]\n":   {`{"k":{"a":1,"b":3}}`, `{"k":{"a":2,"b":3}}`},
		"o: &o !!omap [{a: 1, a: 2}]\np: *o\n": {`{"o":{"a":1},"p":{"a":1}}`, `{"o":{"a":2},"p":{"a":2}}`},
	} {
		t.Run(src, func(t *testing.T) {
			allow := parser.WithAllowDuplicateMapKey()

			js, err := ToJSON([]byte(src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, want.first, string(js), "ToJSON keeps the first entry")

			toks, err := collectJSONTokens([]byte(src), allow)
			require.NoError(t, err)
			assert.JSONEq(t, want.first, string(rebuildJSON(toks)), "the tokens keep the first entry")

			for name, opts := range map[string][]DecodeOption{
				"walk": {AllowDuplicateMapKey()},
				"tree": {AllowDuplicateMapKey(), UseOrderedMap()},
			} {
				var v any
				require.NoErrorf(t, UnmarshalWithOptions([]byte(src), &v, opts...), "%s", name)
				got, err := MarshalWithOptions(v, JSON())
				require.NoError(t, err)
				assert.JSONEqf(t, want.last, string(got), "the %s keeps the last value", name)
			}
		})
	}
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
	type verdict int
	const (
		kept verdict = iota
		repeat
		notJSON // two YAML keys that write one JSON member
	)

	for src, want := range map[string]verdict{
		"k: !!omap [{a: 1}, {a: 2}]\n":                     repeat,
		"k: !!omap [a: 1, a: 2]\n":                         repeat,
		"k: !!omap\n- a: 1\n- a: 2\n":                      repeat,
		"k: !!omap [{1: a}, {0x1: b}]\n":                   repeat,
		"k: !!omap [{-0: a}, {0: b}]\n":                    repeat,
		"k: !!omap [{null: a}, {~: b}]\n":                  repeat,
		"a: &m {x: 1}\nk: !!omap [{x: 2}, *m]\n":           repeat,
		"k: &o !!omap [{a: 1}, {a: 2}]\n":                  repeat,
		"k: !!omap [{a: 1, a: 2}]\n":                       repeat,
		"k: !!omap [{b: 0}, {a: 1, a: 2}]\n":               repeat,
		"k: !!omap &o [{a: 1}, {a: 2}]\n":                  repeat,
		"k: !!omap &o [{a: 1}, {b: 2}]\n":                  kept,
		"k: !!omap &o [{a: 1}, {b: 2}]\nl: *o\n":           kept,
		"k: !!omap [{1: a}, {\"1\": b}]\n":                 notJSON,
		"k: !!omap [{!!binary AA==: a}, {\"AA==\": b}]\n":  notJSON,
		"k: !!omap [{1: a}, {1.0: b}]\n":                   kept,
		"k: !!omap [{a: [{a: 1}]}, {b: 2}]\n":              kept,
		"k: !!omap [{a: !!omap [{a: 1}]}, {b: 2}]\n":       kept,
		"k: !!omap [{a: 1}, {b: 2}]\nl: !!omap [{a: 1}]\n": kept,
		"k: [{a: 1}, {a: 2}]\n":                            kept,
		"a: &m {x: 1}\nk: !!omap [{y: 2}, *m]\n":           kept,
		"k: !!omap [{!!timestamp 2001-12-14t21:59:43.10-05:00: a}, " +
			"{!!timestamp 2001-12-15 02:59:43.10: b}]\n": repeat,
	} {
		var walked any
		walkErr := Unmarshal([]byte(src), &walked)
		_, treeErr := decodedStream(src)
		_, jsonErr := ToJSON([]byte(src))
		_, tokenErr := collectJSONTokens([]byte(src))

		switch want {
		case kept:
			require.NoErrorf(t, walkErr, "walk: %q", src)
			require.NoErrorf(t, treeErr, "tree: %q", src)
			require.NoErrorf(t, jsonErr, "ToJSON: %q", src)
			require.NoErrorf(t, tokenErr, "tokens: %q", src)
		case repeat:
			for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
				assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%s: %q", name, src)
			}
		case notJSON:
			// Two keys, so the decoders hold both; one JSON member, so the
			// converters refuse it as they do for a mapping.
			require.NoErrorf(t, walkErr, "walk: %q", src)
			require.NoErrorf(t, treeErr, "tree: %q", src)
			assert.ErrorIsf(t, jsonErr, yamlerrors.ErrNotJSON, "ToJSON: %q", src)
			assert.ErrorIsf(t, tokenErr, yamlerrors.ErrNotJSON, "tokens: %q", src)
		default:
			t.Fatalf("unknown expectation %v", want)
		}
	}
}
