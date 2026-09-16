// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAMergedKeyLosesToTheMappingsOwn pins which member a "<<" writes and which it leaves out.
//
// A mapping's own key beats one a merge brings in, whatever their order in the document, and an earlier
// merge beats a later one. ToJSON answers the first from the parse's key ledger, through
// parser.Closing.HoldsKey, and keeps its own record only of the names earlier merges brought in.
//
// ⚠️ The two questions are not the same one. The ledger compares a key by the name and the node it resolves
// to, since 3.2.1.1 makes "7" and "007" one key and "1" and "1.0" two. HoldsKey compares the name alone,
// which is what a JSON member is named by: "1: a" and "\"1\": b" are two YAML keys writing one member.
// The cases below are the ones that tell the two readings apart, so a swap back to comparing nodes fails here.
func TestAMergedKeyLosesToTheMappingsOwn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src, want string }{
		{
			"an own key written after the merge",
			"base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			`{"base":{"a":1,"b":2},"m":{"b":"own","a":1}}`,
		}, {
			"an integer own key against a quoted merged one, which JSON names alike",
			"base: &b\n  \"1\": merged\nm:\n  <<: *b\n  1: own\n",
			`{"base":{"1":"merged"},"m":{"1":"own"}}`,
		}, {
			"a quoted own key against an integer merged one",
			"base: &b\n  1: merged\nm:\n  <<: *b\n  \"1\": own\n",
			`{"base":{"1":"merged"},"m":{"1":"own"}}`,
		}, {
			"a float own key against an integer merged one, which JSON names apart",
			"base: &b\n  1: merged\nm:\n  <<: *b\n  1.0: own\n",
			`{"base":{"1":"merged"},"m":{"1.0":"own","1":"merged"}}`,
		}, {
			"007 owns the member 7",
			"base: &b\n  7: merged\nm:\n  <<: *b\n  007: own\n",
			`{"base":{"7":"merged"},"m":{"7":"own"}}`,
		}, {
			"a bool own key against the quoted merged one",
			"base: &b\n  \"true\": merged\nm:\n  <<: *b\n  true: own\n",
			`{"base":{"true":"merged"},"m":{"true":"own"}}`,
		}, {
			"the earlier of two merge sources wins",
			"one: &x {a: 1}\ntwo: &y {a: 2, b: 3}\nm:\n  <<: [*x, *y]\n",
			`{"one":{"a":1},"two":{"a":2,"b":3},"m":{"a":1,"b":3}}`,
		}, {
			"an own key beats both merge sources",
			"one: &x {a: 1}\ntwo: &y {a: 2, b: 3}\nm:\n  <<: [*y, *x]\n  a: own\n",
			`{"one":{"a":1},"two":{"a":2,"b":3},"m":{"a":"own","b":3}}`,
		}, {
			"a mapping written in place as the merge source",
			"m:\n  <<: {a: 1, b: 2}\n  a: own\n",
			`{"m":{"a":"own","b":2}}`,
		}, {
			"an anchored own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  &k a: own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an explicit own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  ? a\n  : own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"a tagged own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  !!str a: own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an alias as the own key",
			"n: &n a\nbase: &b {a: 1}\nm:\n  <<: *b\n  *n : own\n",
			`{"n":"a","base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an own key replaces the whole merged value and does not fold into it",
			"base: &b\n  nest: {x: 1}\nm:\n  <<: *b\n  nest: {y: 2}\n",
			`{"base":{"nest":{"x":1}},"m":{"nest":{"y":2}}}`,
		}, {
			"a merge source that merges in its turn",
			"deep: &d\n  <<: {a: 1}\n  b: 2\nm:\n  <<: *d\n  a: own\n",
			`{"deep":{"b":2,"a":1},"m":{"a":"own","b":2}}`,
		}, {
			// The mapping inside writes "x" and the merge brings "x" to the mapping around it.
			// The parse pops a mapping's keys as it closes, so the inner ones are out of range by the
			// time the outer mapping is left; leaked, they would drop the merged member.
			"a name an inner mapping writes does not answer for the outer one",
			"base: &b {x: merged}\nm:\n  <<: *b\n  inner:\n    x: 1\n",
			`{"base":{"x":"merged"},"m":{"inner":{"x":1},"x":"merged"}}`,
		}, {
			"a name the outer mapping writes does not answer for a merge inside it",
			"base: &b {x: merged}\nm:\n  x: own\n  inner:\n    <<: *b\n",
			`{"base":{"x":"merged"},"m":{"x":"own","inner":{"x":"merged"}}}`,
		}, {
			"a merge in flow style",
			"m: {<<: {a: 1}, a: own, b: 2}\n",
			`{"m":{"a":"own","b":2}}`,
		}, {
			"an empty merge source",
			"base: &b {}\nm:\n  <<: *b\n  a: 1\n",
			`{"base":{},"m":{"a":1}}`,
		}, {
			"a merge into a mapping that writes nothing itself",
			"base: &b {a: 1}\nm:\n  <<: *b\n",
			`{"base":{"a":1},"m":{"a":1}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.ToJSON([]byte(tc.src), parser.WithMergeKeys())
			require.NoErrorf(t, err, "%q", tc.src)

			assert.JSONEq(t, tc.want, string(out))
			assert.Equalf(t, tc.want, string(out), "%q: the members are written in this order", tc.src)
		})
	}

	t.Run("a repeated merge key is a repeat like any other", func(t *testing.T) {
		t.Parallel()

		_, err := codec.ToJSON([]byte("m:\n  <<: {a: 1}\n  <<: {a: 2, b: 3}\n"), parser.WithMergeKeys())

		require.ErrorContains(t, err, `mapping key "<<" already defined`)
	})
}

// TestEveryRouteToAMergeAnswersAlike checks that how a "<<" comes to merge does not change what it writes.
//
// Parser.mergeKeysInForce resolves a bare "<<" under YAML 1.1 or under WithMergeKeys, and a written
// "!!merge" tag is honored at any version. isMergeKey reads three node shapes for those routes --
// ast.MergeKeyNode, a TagNode over the merge tag, and MappingKeyNode.IsMergeKey for the "? <<" spelling --
// and the cases above reach only the first.
//
// ⚠️ Under 1.2 with no option a bare "<<" is an ordinary key spelled "<<", and the last case pins that.
// It is the documented reading, not a defect: the merge key is a 1.1 type.
func TestEveryRouteToAMergeAnswersAlike(t *testing.T) {
	t.Parallel()

	const merged = `{"base":{"a":1,"b":2},"m":{"b":"own","a":1}}`

	for _, tc := range []struct {
		name string
		src  string
		opts []parser.Option
		want string
	}{
		{
			name: "a bare << under WithMergeKeys",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			opts: []parser.Option{parser.WithMergeKeys()},
			want: merged,
		}, {
			name: "a bare << under a %YAML 1.1 directive",
			src:  "%YAML 1.1\n---\nbase: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			want: merged,
		}, {
			name: "a bare << under WithYAMLVersion(YAML11)",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			opts: []parser.Option{parser.WithYAMLVersion(parser.YAML11)},
			want: merged,
		}, {
			name: "a written !!merge tag, at 1.2 and with no option",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  !!merge <<: *b\n  b: own\n",
			want: merged,
		}, {
			name: "the \"? <<\" spelling",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  ? <<\n  : *b\n  b: own\n",
			opts: []parser.Option{parser.WithMergeKeys()},
			want: merged,
		}, {
			name: "a bare << at 1.2 is an ordinary key, and merges nothing",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			want: `{"base":{"a":1,"b":2},"m":{"<<":{"a":1,"b":2},"b":"own"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.ToJSON([]byte(tc.src), tc.opts...)
			require.NoErrorf(t, err, "%q", tc.src)

			assert.Equalf(t, tc.want, string(out), "%q", tc.src)
		})
	}

	// The reading HoldsKey answers on -- a name shared by two keys YAML tells apart -- has to be the
	// same whichever route the merge took, since each reaches collectMerge by a different node shape.
	t.Run("the name a merged key loses on does not depend on the route", func(t *testing.T) {
		t.Parallel()

		const want = `{"base":{"1":"merged"},"m":{"1":"own"}}`

		byDirective, err := codec.ToJSON([]byte("%YAML 1.1\n---\nbase: &b\n  \"1\": merged\nm:\n  <<: *b\n  1: own\n"))
		require.NoError(t, err)

		byTag, err := codec.ToJSON([]byte("base: &b\n  \"1\": merged\nm:\n  !!merge <<: *b\n  1: own\n"))
		require.NoError(t, err)

		assert.Equal(t, want, string(byDirective))
		assert.Equal(t, want, string(byTag))
	})
}
