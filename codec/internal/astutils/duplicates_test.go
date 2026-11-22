// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestRefuseDuplicateKeys(t *testing.T) {
	t.Run("returns an error for a repeated key", func(t *testing.T) {
		err := RefuseDuplicateKeys(mappingOf(t, "a: 1\nb: 2\na: 3\n"))

		require.Error(t, err)
		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
		assert.Contains(t, err.Error(), `"a"`)
	})

	t.Run("returns nothing for distinct keys", func(t *testing.T) {
		assert.NoError(t, RefuseDuplicateKeys(mappingOf(t, "a: 1\nb: 2\n")))
	})

	t.Run("returns nothing for a repeat the parser allowed", func(t *testing.T) {
		m := mappingOf(t, "a: 1\na: 2\n", parser.WithAllowDuplicateMapKey())

		assert.NoError(t, RefuseDuplicateKeys(m))
	})

	t.Run("returns nothing for a node that is not a mapping", func(t *testing.T) {
		assert.NoError(t, RefuseDuplicateKeys(bodyOf(t, "[a, a]")))
	})
}

func TestRefuseRepeats(t *testing.T) {
	// "1" and 1 are two YAML keys, and one name.
	const src = "1: a\n\"1\": b\n"

	t.Run("a reader keyed by name rejects two keys of one name", func(t *testing.T) {
		assert.Error(t, RefuseRepeats(mappingOf(t, src), true))
	})

	t.Run("a reader keyed by value holds both", func(t *testing.T) {
		assert.NoError(t, RefuseRepeats(mappingOf(t, src), false))
	})
}

func TestNewAllowedRepeat(t *testing.T) {
	m := mappingOf(t, "a: 1\na: 2\nb: 3\n", parser.WithAllowDuplicateMapKey())
	require.Len(t, m.Duplicates, 1)

	var seen int
	assert.True(t, NewAllowedRepeat(m, &seen), "the first look finds the repeat")
	assert.Equal(t, 1, seen)
	assert.False(t, NewAllowedRepeat(m, &seen), "a second look finds nothing new")

	assert.False(t, NewAllowedRepeat(nil, &seen))
}

func TestOrderedMapDuplicates(t *testing.T) {
	const src = "!!omap\n- a: 1\n- b: 2\n- a: 3\n"

	sequenceOf := func(t *testing.T, opts ...parser.Option) *ast.SequenceNode {
		t.Helper()
		tag, ok := bodyOf(t, src, opts...).(*ast.TagNode)
		require.True(t, ok)
		seq, ok := tag.Value.(*ast.SequenceNode)
		require.True(t, ok)

		return seq
	}

	t.Run("returns an error for a key repeated across entries", func(t *testing.T) {
		err := RefuseOrderedMapDuplicates(sequenceOf(t))

		require.Error(t, err)
		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
		assert.Empty(t, AllowedRepeatEntries(sequenceOf(t)))
	})

	t.Run("lists the entries of a repeat the parser allowed", func(t *testing.T) {
		seq := sequenceOf(t, parser.WithAllowDuplicateMapKey())

		require.NoError(t, RefuseOrderedMapDuplicates(seq))
		assert.Equal(t, []int{2}, AllowedRepeatEntries(seq))
	})

	t.Run("accepts a nil sequence", func(t *testing.T) {
		require.NoError(t, RefuseOrderedMapDuplicates(nil))
		assert.Nil(t, AllowedRepeatEntries(nil))
	})
}
