// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestMapKeyString(t *testing.T) {
	for tc := range mapKeyStringCases() {
		t.Run(tc.name, func(t *testing.T) {
			key := mappingOf(t, tc.src).Values[0].Key

			got, err := MapKeyString(key, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	for tc := range collectionKeyCases() {
		t.Run("rejects a collection key: "+tc.src, func(t *testing.T) {
			key := mappingOf(t, tc.src).Values[0].Key

			_, err := MapKeyString(key, tc.decoded)
			require.Error(t, err)
		})
	}
}

// collectionKeyCase holds a mapping keyed by a collection, and the value the key decodes to.
type collectionKeyCase struct {
	src     string
	decoded any
}

func collectionKeyCases() iter.Seq[collectionKeyCase] {
	return slices.Values([]collectionKeyCase{
		{"? {k: v}\n: a", map[string]any{"k": "v"}},
		{"? [k]\n: a", []any{"k"}},
	})
}

// mapKeyStringCase holds a one-entry mapping and the name MapKeyString gives its key.
type mapKeyStringCase struct {
	name, src, want string
}

func mapKeyStringCases() iter.Seq[mapKeyStringCase] {
	return slices.Values([]mapKeyStringCase{
		{"an integer", "1: a", "1"},
		{"a float, apart from the integer", "1.0: a", "1.0"},
		{"a string", "abc: a", "abc"},
		{"a quoted string", `"1": a`, "1"},
		{"a null", "~: a", "null"},
		{"the empty string, apart from the null", `"": a`, ""},
	})
}

func TestKeyText(t *testing.T) {
	for tc := range keyTextCases() {
		t.Run(tc.src, func(t *testing.T) {
			assert.Equal(t, tc.want, KeyText(mappingOf(t, tc.src).Values[0].Key))
		})
	}
}

func TestEachEntryOwnFirst(t *testing.T) {
	m := mappingOf(t, "%YAML 1.1\n---\n<<: {a: 1}\nb: 2\nc: 3\n")

	names, merged := keyNames(t, func(fn func(ast.Node, bool) error) error { return EachEntryOwnFirst(m, fn) })

	assert.Equal(t, []string{"b", "c", "<<"}, names, "own entries in document order, then the merge")
	assert.Equal(t, []bool{false, false, true}, merged)
}

func TestMapEntriesOwnFirst(t *testing.T) {
	m := mappingOf(t, "%YAML 1.1\n---\na: 1\n<<: {x: 1}\nb: 2\n")

	entries := MapEntriesOwnFirst(m)
	require.Len(t, entries, 3)

	var names []string
	for _, e := range entries {
		names = append(names, e.Key.GetToken().Value)
	}
	assert.Equal(t, []string{"a", "b", "<<"}, names)
	assert.False(t, entries[0].Merged)
	assert.True(t, entries[2].Merged)
}

func TestEachMergedEntry(t *testing.T) {
	m := mappingOf(t, "a: 1\nb: 2\n")

	var names []string
	require.NoError(t, EachMergedEntry(m, func(v ast.Node) error {
		names = append(names, v.(*ast.MappingValueNode).Key.GetToken().Value)

		return nil
	}))
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestCountNodes(t *testing.T) {
	aliasCost := func(target string) int {
		base := "a: &x " + target + "\n"

		return CountNodes(bodyOf(t, base+"c: *x\n")) - CountNodes(bodyOf(t, base))
	}

	// An alias adds the same count whatever it names: the walk does not follow it to its target.
	assert.Equal(t, aliasCost("1"), aliasCost("[1, 2, 3, 4, 5, 6, {k: [v, w]}]"))
}

func TestValidationErrorToken(t *testing.T) {
	m := mappingOf(t, "scalar: 1\ncollection:\n  k: v\n")
	scalar, collection := m.Values[0], m.Values[1]

	assert.Same(t, scalar.Value.GetToken(), ValidationErrorToken(scalar.Value, scalar),
		"a scalar is reported at itself")
	assert.Same(t, collection.GetToken(), ValidationErrorToken(collection.Value, collection),
		"a collection is reported at its entry")
	assert.Same(t, collection.Value.GetToken(), ValidationErrorToken(collection.Value, nil),
		"a collection without its entry is reported at itself")
}

func TestSequenceEntryNode(t *testing.T) {
	// The parser keeps the entry nodes only under WithComments.
	seq, ok := bodyOf(t, "- a\n- b\n", parser.WithComments()).(*ast.SequenceNode)
	require.True(t, ok)

	assert.Same(t, seq.Entries[1], SequenceEntryNode(seq, 1))
	assert.Nil(t, SequenceEntryNode(seq, 2))
	assert.Nil(t, SequenceEntryNode(seq, -1))
}

func TestStandsOnNothing(t *testing.T) {
	scalar := bodyOf(t, "x")

	assert.True(t, StandsOnNothing(nil))
	assert.True(t, StandsOnNothing(&ast.MappingKeyNode{}))
	assert.True(t, StandsOnNothing(&ast.MappingKeyNode{Value: &ast.AnchorNode{}}))
	assert.True(t, StandsOnNothing(&ast.TagNode{}))
	assert.False(t, StandsOnNothing(scalar))
	assert.False(t, StandsOnNothing(&ast.AnchorNode{Value: scalar}))
}

func TestAnchorName(t *testing.T) {
	m := mappingOf(t, "a: &x 1\nb: *x\n")
	anchor, ok := m.Values[0].Value.(*ast.AnchorNode)
	require.True(t, ok)
	alias, ok := m.Values[1].Value.(*ast.AliasNode)
	require.True(t, ok)

	assert.Equal(t, "x", AnchorName(anchor.Name))
	assert.Equal(t, "x", AnchorName(alias.Value))
	assert.Empty(t, AnchorName(nil))
}

func TestNodeAt(t *testing.T) {
	m := mappingOf(t, "a: 1\nb: 2\n")

	at := NodeAt(m.Values[1].Value)
	assert.Equal(t, 2, int(at.Line))
	assert.Equal(t, 4, int(at.Column))
	assert.Zero(t, NodeAt(nil))
}

func TestHeadCommentPath(t *testing.T) {
	m := mappingOf(t, "a: 1\nb: 2\n")

	assert.Equal(t, m.Values[0].Key.GetPath(), HeadCommentPath(m), "a mapping's head comment goes to its first key")
	assert.Equal(t, m.Values[1].Key.GetPath(), HeadCommentPath(m.Values[1]), "an entry's goes to its key")
}

// ************** map keys test cases **********************

// keyTextCase holds a one-entry mapping and the text KeyText gives its key.
type keyTextCase struct {
	src, want string
}

func keyTextCases() iter.Seq[keyTextCase] {
	return slices.Values([]keyTextCase{
		{"4: a", "4"},
		{"4.0: a", "4.0"},
		{"abc: a", "abc"},
		{"~: a", "null"},
		{"true: a", "true"},
	})
}

// bodyOf parses a one-document source and returns the document's body.
func bodyOf(t *testing.T, src string, opts ...parser.Option) ast.Node {
	t.Helper()
	file, err := parser.ParseBytes([]byte(src), opts...)
	require.NoError(t, err)
	require.NotEmpty(t, file.Docs)

	return file.Docs[len(file.Docs)-1].Body
}

// mappingOf parses a one-document source whose body is a block mapping.
func mappingOf(t *testing.T, src string, opts ...parser.Option) *ast.MappingNode {
	t.Helper()
	m, ok := bodyOf(t, src, opts...).(*ast.MappingNode)
	require.Truef(t, ok, "the body of %q is not a mapping", src)

	return m
}

// keyNames returns the text of each entry key, in the order fn is called.
func keyNames(t *testing.T, each func(fn func(ast.Node, bool) error) error) (names []string, merged []bool) {
	t.Helper()
	require.NoError(t, each(func(v ast.Node, m bool) error {
		entry, ok := v.(*ast.MappingValueNode)
		require.True(t, ok)
		names = append(names, entry.Key.GetToken().Value)
		merged = append(merged, m)

		return nil
	}))

	return names, merged
}
