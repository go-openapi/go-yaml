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

// bodyOf returns the root node of a one-document source.
func bodyOf(t *testing.T, src string) ast.Node {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src))
	require.NoErrorf(t, err, "%q", src)
	require.Lenf(t, f.Docs, 1, "%q", src)

	return f.Docs[0].Body
}

// keysOf returns the key text of each entry MapRange yields, and checks that Entries yields the same entries.
func keysOf(t *testing.T, n ast.MapNode) []string {
	t.Helper()

	var keys []string
	for key, value := range n.MapRange() {
		keys = append(keys, key.GetToken().Value+"="+value.GetToken().Value)
	}

	i := 0
	for entry := range n.Entries() {
		require.Lessf(t, i, len(keys), "Entries yields more entries than MapRange")
		assert.Equal(t, keys[i], entry.Key.GetToken().Value+"="+entry.Value.GetToken().Value)
		i++
	}
	assert.Equal(t, len(keys), i, "Entries yields as many entries as MapRange")
	assert.Equal(t, len(keys), n.Len(), "Len counts the entries the iterators yield")

	return keys
}

// TestAMapNodeYieldsItsEntriesInOrder checks MapRange, Entries and Len on the three kinds of MapNode.
func TestAMapNodeYieldsItsEntriesInOrder(t *testing.T) {
	first, ok := bodyOf(t, "a: 1\nb: 2\n").(*ast.MappingNode)
	require.True(t, ok)
	second, ok := bodyOf(t, "{c: 3}").(*ast.MappingNode)
	require.True(t, ok)

	t.Run("a mapping", func(t *testing.T) {
		assert.Equal(t, []string{"a=1", "b=2"}, keysOf(t, first))
	})

	t.Run("an empty mapping", func(t *testing.T) {
		empty, ok := bodyOf(t, "{}").(*ast.MappingNode)
		require.True(t, ok)
		assert.Empty(t, keysOf(t, empty))
	})

	t.Run("a single entry", func(t *testing.T) {
		assert.Equal(t, []string{"a=1"}, keysOf(t, first.Values[0]))
	})

	t.Run("a merge sequence reads each mapping in turn", func(t *testing.T) {
		merged := ast.SequenceMergeValue(first, second, first.Values[1])
		assert.Equal(t, []string{"a=1", "b=2", "c=3", "b=2"}, keysOf(t, merged))
	})

	t.Run("breaking out stops the merge sequence", func(t *testing.T) {
		merged := ast.SequenceMergeValue(first, second)

		var seen []string
		for entry := range merged.Entries() {
			seen = append(seen, entry.Key.GetToken().Value)
			if len(seen) == 2 {
				break
			}
		}
		assert.Equal(t, []string{"a", "b"}, seen)

		seen = nil
		for key := range merged.MapRange() {
			seen = append(seen, key.GetToken().Value)

			break
		}
		assert.Equal(t, []string{"a"}, seen)
	})
}

// TestASequenceYieldsItsValuesInOrder checks ArrayRange and Len on a SequenceNode.
func TestASequenceYieldsItsValuesInOrder(t *testing.T) {
	for _, src := range []string{"- x\n- y\n- z\n", "[x, y, z]", "[]"} {
		seq, ok := bodyOf(t, src).(*ast.SequenceNode)
		require.Truef(t, ok, "%q", src)

		var values []string
		for value := range seq.ArrayRange() {
			values = append(values, value.GetToken().Value)
		}
		assert.Equalf(t, len(values), seq.Len(), "%q", src)
		if seq.Len() > 0 {
			assert.Equalf(t, []string{"x", "y", "z"}, values, "%q", src)
		}
	}
}
