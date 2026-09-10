// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenGroupTypeNamesEveryType walks the whole enum, so a type added
// without a name here prints as "none" and this fails.
func TestTokenGroupTypeNamesEveryType(t *testing.T) {
	t.Parallel()

	for named := range groupTypeNames() {
		assert.Equalf(t, named.want, named.typ.String(), "type %d", uint8(named.typ))
	}
}

// TestAnUnnamedTokenGroupTypePrintsAsNone pins what String does with a value
// outside the enum.
//
// ⚠️ It answers "none", which is what TokenGroupNone answers, so a dump cannot
// tell an ungrouped token from a group of a type that does not exist.
func TestAnUnnamedTokenGroupTypePrintsAsNone(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "none", group.TokenGroupType(200).String())
}

type namedGroupType struct {
	typ  group.TokenGroupType
	want string
}

func groupTypeNames() iter.Seq[namedGroupType] {
	return slices.Values([]namedGroupType{
		{group.TokenGroupNone, "none"},
		{group.TokenGroupDirective, "directive"},
		{group.TokenGroupDirectiveName, "directive_name"},
		{group.TokenGroupDocument, "document"},
		{group.TokenGroupDocumentBody, "document_body"},
		{group.TokenGroupAnchor, "anchor"},
		{group.TokenGroupAnchorName, "anchor_name"},
		{group.TokenGroupAlias, "alias"},
		{group.TokenGroupLiteral, "literal"},
		{group.TokenGroupFolded, "folded"},
		{group.TokenGroupScalarTag, "scalar_tag"},
		{group.TokenGroupMapKey, "map_key"},
		{group.TokenGroupMapKeyValue, "map_key_value"},
	})
}

// TestATokenGroupHoldsItsMembersInOrder checks Len, At, First and Last at each
// size the group is built for.
//
// A group of one or two keeps its members in the group itself and a longer one
// keeps a slice, so the sizes either side of two are the ones to write.
func TestATokenGroupHoldsItsMembersInOrder(t *testing.T) {
	t.Parallel()

	members := []*group.TapeToken{
		tapeToken("a", token.StringType, 1, 1, 1),
		tapeToken("b", token.StringType, 1, 2, 2),
		tapeToken("c", token.StringType, 1, 3, 3),
		tapeToken("d", token.StringType, 1, 4, 4),
	}

	for _, size := range []int{0, 1, 2, 3, 4} {
		t.Run(sizeName(size), func(t *testing.T) {
			t.Parallel()

			g := group.NewTokenGroup(group.TokenGroupMapKey, members[:size])

			require.Equal(t, size, g.Len())
			for i := range size {
				assert.Samef(t, members[i], g.At(i), "member %d", i)
			}

			if size == 0 {
				assert.Nil(t, g.First())
				assert.Nil(t, g.Last())

				return
			}
			assert.Same(t, members[0], g.First())
			assert.Same(t, members[size-1], g.Last())
		})
	}
}

// TestMembersWritesTheInlinePairIntoTheCallersArray checks that reading the
// members of a short group allocates nothing: the caller owns the array.
func TestMembersWritesTheInlinePairIntoTheCallersArray(t *testing.T) {
	t.Parallel()

	first := tapeToken("a", token.StringType, 1, 1, 1)
	second := tapeToken("b", token.StringType, 1, 2, 2)
	third := tapeToken("c", token.StringType, 1, 3, 3)

	var pair [2]*group.TapeToken
	inline := group.NewTokenGroup(group.TokenGroupMapKey, []*group.TapeToken{first, second})
	got := inline.Members(&pair)

	require.Len(t, got, 2)
	assert.Same(t, first, pair[0], "the pair was written into the caller's array")
	assert.Same(t, second, pair[1])

	var untouched [2]*group.TapeToken
	long := group.NewTokenGroup(group.TokenGroupDocument, []*group.TapeToken{first, second, third})
	got = long.Members(&untouched)

	require.Len(t, got, 3)
	assert.Nil(t, untouched[0], "a group of three keeps a slice, so the array stays empty")
}

// TestATokenGroupReadsItsFirstMember checks that a group answers for the token
// it begins at, which is what the parser reads when it holds a group.
func TestATokenGroupReadsItsFirstMember(t *testing.T) {
	t.Parallel()

	first := tapeToken("k", token.StringType, 6, 2, 30)
	second := tapeToken(":", token.MappingValueType, 6, 3, 31)
	g := group.NewTokenGroup(group.TokenGroupMapKey, []*group.TapeToken{first, second})

	assert.Equal(t, token.StringType, g.TokenType())
	assert.Equal(t, 6, g.Line())
	assert.Equal(t, 2, g.Column())
	require.NotNil(t, g.RawToken())
	assert.Equal(t, "k", g.RawToken().Value)
}

// TestAnEmptyTokenGroupAnswersZero checks the other end of the same accessors.
// The grouper builds a group before its members are read.
func TestAnEmptyTokenGroupAnswersZero(t *testing.T) {
	t.Parallel()

	g := group.NewTokenGroup(group.TokenGroupDocument, nil)

	assert.Zero(t, g.Len())
	assert.Nil(t, g.RawToken())
	assert.Zero(t, g.Line())
	assert.Zero(t, g.Column())
	assert.Equal(t, token.Type(0), g.TokenType())
}

func sizeName(n int) string {
	switch n {
	case 0:
		return "no member"
	case 1:
		return "one member"
	default:
		return string(rune('0'+n)) + " members"
	}
}
