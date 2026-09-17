// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// found is what an entry holds, and "" where the lookup returned none.
func found(entry *ast.MappingValueNode) string {
	if entry == nil {
		return ""
	}

	return entry.Value.String()
}

// TestLookupReadsAMappingsOwnKeys checks Lookup on the shapes a value comes wrapped in.
func TestLookupReadsAMappingsOwnKeys(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, src, key, want string
	}{
		{"a key", "a: 1\nb: 2\n", "b", "2"},
		{"a key it does not hold", "a: 1\n", "zzz", ""},
		{"a quoted key by its text", `"a b": 1`, "a b", "1"},
		{"an integer key by its name", "1: one\n", "1", "one"},
		{"a flow mapping", "{a: 1, b: 2}\n", "a", "1"},
		{"the first of two", "a: 1\na: 2\n", "a", "1"},
		{"a merged key is not read", "base: &b\n  a: 1\nm:\n  <<: *b\n", "a", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithMergeKeys(), parser.WithAllowDuplicateMapKey())
			require.NoError(t, err)

			node := file.Docs[0].Body
			if entry := ast.Lookup(node, "m"); entry != nil {
				node = entry.Value
			}
			assert.Equal(t, tc.want, found(ast.Lookup(node, tc.key)))
		})
	}

	assert.Nil(t, ast.Lookup(nil, "a"), "a nil node holds no key")
	assert.Nil(t, ast.LookupIn(nil, "a"), "a nil mapping holds no key")
	assert.Nil(t, ast.Lookup(ast.Text("scalar"), "a"), "a scalar holds no key")
}

// TestLookupReadsThroughAProperty checks that a value carrying an anchor, a tag or an alias is reached
// through it: a document writes those in front of the mapping a caller means.
func TestLookupReadsThroughAProperty(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src, want string }{
		{"an anchor", "m: &anchored\n  a: 1\n", "1"},
		{"a tag", "m: !!map\n  a: 1\n", "1"},
		{"an alias", "base: &b\n  a: 1\nm: *b\n", "1"},
		{"a tag on an anchor", "m: !!map &anchored\n  a: 1\n", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			assert.Equal(t, tc.want, found(ast.Lookup(ast.Lookup(file.Docs[0].Body, "m").Value, "a")))
		})
	}
}

// TestLookupMergedReadsWhatAMergeBringsIn holds the precedence the merge type requires: the mapping's own
// keys win, and among the keys a "<<" brings in an earlier source beats a later one.
func TestLookupMergedReadsWhatAMergeBringsIn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, src, key, want string
	}{
		{
			"the mapping's own key wins",
			"base: &b\n  a: merged\nm:\n  <<: *b\n  a: own\n", "a", "own",
		},
		{
			"a merged key",
			"base: &b\n  a: merged\nm:\n  <<: *b\n  b: own\n", "a", "merged",
		},
		{
			"an earlier source wins",
			"one: &x\n  a: first\ntwo: &y\n  a: second\nm:\n  <<: [*x, *y]\n", "a", "first",
		},
		{
			"a merge of a merge",
			"deep: &d\n  a: deep\nmid: &m\n  <<: *d\nm:\n  <<: *m\n", "a", "deep",
		},
		{
			"a key no source holds",
			"base: &b\n  a: 1\nm:\n  <<: *b\n", "zzz", "",
		},
		{
			"a mapping written out, with no alias",
			"m:\n  <<: {a: merged}\n", "a", "merged",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithMergeKeys())
			require.NoError(t, err)

			mapping := ast.Lookup(file.Docs[0].Body, "m").Value
			assert.Equal(t, tc.want, found(ast.LookupMerged(mapping, tc.key)))
		})
	}

	assert.Nil(t, ast.LookupMerged(nil, "a"))
}

// TestLookupInReadsAMergeSource checks LookupIn on what MergeEntry.Sources holds, which is a MapNode and
// not always a Node.
func TestLookupInReadsAMergeSource(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseBytes([]byte("base: &b\n  a: 1\nm:\n  <<: *b\n"), parser.WithMergeKeys())
	require.NoError(t, err)

	mapping, ok := ast.Lookup(file.Docs[0].Body, "m").Value.(*ast.MappingNode)
	require.True(t, ok)

	merge := ast.MergeOf(mapping.Values[0])
	require.Equal(t, ast.Folds, merge.Verdict)
	require.Len(t, merge.Sources, 1)

	assert.Equal(t, "1", found(ast.LookupIn(merge.Sources[0], "a")))
}
