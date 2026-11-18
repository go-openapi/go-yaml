// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// entryOf is the first mapping entry of the document's root mapping, which is
// what MergeOf is asked about.
//
// It reads under YAML 1.1, where a bare "<<" is the merge key. The parser
// decides that, and under the core schema it builds an ordinary key instead --
// see the subtest that asserts it.
func entryOf(t *testing.T, src string) *ast.MappingValueNode {
	t.Helper()

	return entryUnder(t, "%YAML 1.1\n---\n"+src)
}

// entryUnder is entryOf without a version prepended, for asking what a document
// means as written.
func entryUnder(t *testing.T, src string) *ast.MappingValueNode {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src))
	require.NoErrorf(t, err, "%q", src)
	require.NotEmptyf(t, f.Docs, "%q", src)

	// A "%YAML" line is a document of the parse in its own right, ahead of the
	// one it applies to, so the body is the last document and not the first.
	switch body := f.Docs[len(f.Docs)-1].Body.(type) {
	case *ast.MappingNode:
		require.NotEmptyf(t, body.Values, "%q", src)

		return body.Values[0]
	case *ast.MappingValueNode:
		return body
	}
	t.Fatalf("%q: the document holds no mapping entry", src)

	return nil
}

// TestMergeOfAnswersForEveryEntry holds the one rule thirteen readers used to
// infer for themselves.
//
// Each of them asked MapKeyNode.IsMergeKey and drew its own conclusion from a
// bare yes -- the decoder folded, ToJSON deferred to the mapping close, the
// typed walk gave up on the document, and none agreed about a "<<" whose value
// is not a mapping. MergeOf answers the question itself.
func TestMergeOfAnswersForEveryEntry(t *testing.T) {
	t.Run("an ordinary key is not a merge", func(t *testing.T) {
		for _, src := range []string{"a: 1\n", "{a: 1}\n"} {
			assert.Equalf(t, ast.NotAMerge, ast.MergeOf(entryOf(t, src)).Verdict, "%q", src)
		}
	})

	t.Run("and under the core schema a bare << is an ordinary key too", func(t *testing.T) {
		// The merge key is tag:yaml.org,2002:merge, a YAML 1.1 type. Under 1.2
		// the parser builds a string for it, so nothing reaches here as a
		// merge -- which is what libfyaml 1.0.0b1 reads in its own 1.2 mode.
		for _, src := range []string{"<<: {x: 1}\n", "<<: [{x: 1}]\n", "<<: 1\n"} {
			got := ast.MergeOf(entryUnder(t, src))
			assert.Equalf(t, ast.NotAMerge, got.Verdict, "%q", src)
		}

		// A written tag names the type at any version, so this one does fold.
		assert.Equal(t, ast.Folds, ast.MergeOf(entryUnder(t, "!!merge << : {x: 1}\n")).Verdict)
	})

	t.Run("a key the parser built as a merge folds", func(t *testing.T) {
		// Which spellings reach here as a MergeKeyNode is the parser's to
		// settle, and it holds the version and the tag handles to settle it
		// with. This asks only what the entry then contributes.
		for _, src := range []string{
			"<<: {x: 1}\n",
			"!!merge << : {x: 1}\n",
			"!<tag:yaml.org,2002:merge> << : {x: 1}\n",
		} {
			got := ast.MergeOf(entryOf(t, src))
			assert.Equalf(t, ast.Folds, got.Verdict, "%q", src)
			assert.Lenf(t, got.Sources, 1, "%q", src)
		}
	})

	t.Run("a sequence folds earliest first", func(t *testing.T) {
		got := ast.MergeOf(entryOf(t, "<<: [{x: 1}, {x: 2}]\n"))
		require.Equal(t, ast.Folds, got.Verdict)
		require.Len(t, got.Sources, 2)

		// The order is the rule: reading them in turn and keeping the first
		// writer gives an earlier mapping precedence over a later one.
		first := got.Sources[0].MapRange()
		require.True(t, first.Next())
		assert.Equal(t, "1", first.Value().GetToken().Value)
	})

	t.Run("an anchor, an alias and a tag are read through", func(t *testing.T) {
		for _, src := range []string{
			"a: &a {x: 1}\n<<: *a\n",
			"<<: &a {x: 1}\n",
			"<<: !!map {x: 1}\n",
		} {
			entry := entryOf(t, src)
			if entry.Key == nil || !entry.Key.IsMergeKey() {
				// The first document's first entry is "a", so take the last.
				f, err := parser.ParseBytes([]byte("%YAML 1.1\n---\n" + src))
				require.NoError(t, err)
				body, ok := f.Docs[len(f.Docs)-1].Body.(*ast.MappingNode)
				require.True(t, ok)
				entry = body.Values[len(body.Values)-1]
			}
			got := ast.MergeOf(entry)
			assert.Equalf(t, ast.Folds, got.Verdict, "%q", src)
			assert.Lenf(t, got.Sources, 1, "%q", src)
		}
	})

	t.Run("a value that is not a mapping is refused, and names the node", func(t *testing.T) {
		for _, src := range []string{
			"<<: 1\n",
			"<<: [1]\n",
			"<<: [{x: 1}, 2]\n",
			"<<:\n",
		} {
			got := ast.MergeOf(entryOf(t, src))
			assert.Equalf(t, ast.Refused, got.Verdict, "%q", src)
			assert.NotNilf(t, got.Offender, "%q: the refusal names no node", src)
		}
	})

	t.Run("an alias naming nothing is refused rather than followed", func(t *testing.T) {
		// AliasNode.Target is nil where the parse could not fill it, and a
		// merge has nothing to fold.
		entry := &ast.MappingValueNode{
			Key:   ast.MergeKey(&token.Token{Type: token.MergeKeyType, Value: "<<"}),
			Value: &ast.AliasNode{},
		}
		got := ast.MergeOf(entry)
		assert.Equal(t, ast.Refused, got.Verdict)
	})

	t.Run("a nil entry is not a merge", func(t *testing.T) {
		assert.Equal(t, ast.NotAMerge, ast.MergeOf(nil).Verdict)
	})
}
