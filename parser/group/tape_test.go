// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// tapeToken returns a token off the tape: filled from what the scanner read,
// standing at seq.
//
// It goes through [token.Assemble] and [token.MeasureOrigin] instead of writing
// the struct out, so the token carries the end offset and the end line the
// scanner would give it. A token built by hand has EndLine 0, and keyEndLine
// then reads every key as standing on an earlier line than its ':' -- which
// turns "a: 1" into a mapping with an implicit null key and no defect anywhere.
func tapeToken(value string, typ token.Type, line, column int32, seq int) *group.TapeToken {
	pos := token.Position{Line: line, Column: column}
	tk := new(group.TapeToken)
	tk.Raw(token.Assemble(typ, value, pos, token.MeasureOrigin(value, pos)), seq)

	return tk
}

// TestTapeTokenAnswersOnANilReceiver pins the nil contract.
//
// The parser reads ahead past the end of a document and past the end of a flow
// collection, so a nil token reaches these accessors on every document. Each
// answers the zero value for its type instead of panicking.
func TestTapeTokenAnswersOnANilReceiver(t *testing.T) {
	t.Parallel()

	var nilToken *group.TapeToken

	assert.Equal(t, int32(0), nilToken.Seq())
	assert.Nil(t, nilToken.RawToken())
	assert.Equal(t, token.Type(0), nilToken.Type())
	assert.Equal(t, group.TokenGroupNone, nilToken.GroupType())
	assert.Zero(t, nilToken.Line())
	assert.Zero(t, nilToken.Column())
}

// TestTapeTokenReadsWhatTheScannerRead checks the plain case: no group, so
// every accessor answers from the raw token.
func TestTapeTokenReadsWhatTheScannerRead(t *testing.T) {
	t.Parallel()

	tk := tapeToken("a", token.StringType, 3, 5, 7)

	assert.Equal(t, int32(7), tk.Seq())
	assert.Equal(t, token.StringType, tk.Type())
	assert.Equal(t, group.TokenGroupNone, tk.GroupType())
	assert.Equal(t, 3, tk.Line())
	assert.Equal(t, 5, tk.Column())

	raw := tk.RawToken()
	require.NotNil(t, raw)
	assert.Equal(t, "a", raw.Value)
}

// TestNewSyntheticCopiesTheToken checks that a token the parse makes is off the
// tape: it holds its own copy, so it outlives whatever the tail does.
func TestNewSyntheticCopiesTheToken(t *testing.T) {
	t.Parallel()

	assert.Nil(t, group.NewSynthetic(nil), "nothing to stand for, so no token")

	source := token.Token{Value: "null", Type: token.NullType}
	tk := group.NewSynthetic(&source)
	require.NotNil(t, tk)

	source.Value = "overwritten"

	assert.Equal(t, "null", tk.RawToken().Value, "the token was copied, not pointed at")
	assert.Equal(t, int32(0), tk.Seq(), "a synthetic token stands nowhere on the tape")
}

// TestRawClearsTheGroup checks that filling a token in from the scanner drops
// whatever group a previous fill hung on it.
//
// A chunk of tape is filled again once nothing reads the tokens it holds, so a
// cell reaches Raw carrying the last document's group.
func TestRawClearsTheGroup(t *testing.T) {
	t.Parallel()

	tk := tapeToken("a", token.StringType, 1, 1, 1)
	tk.Group = group.NewTokenGroup(group.TokenGroupAlias, []*group.TapeToken{
		tapeToken("*x", token.AliasType, 9, 9, 9),
	})
	require.Equal(t, group.TokenGroupAlias, tk.GroupType())

	tk.Raw(token.Token{Value: "b", Type: token.IntegerType}, 4)

	assert.Nil(t, tk.Group)
	assert.Equal(t, group.TokenGroupNone, tk.GroupType())
	assert.Equal(t, token.IntegerType, tk.Type())
	assert.Equal(t, int32(4), tk.Seq())
}

// TestAGroupAnswersBeforeTheRawToken checks the precedence [group.TapeToken]
// documents: raw holds what the scanner read, Group what a pass made of it.
func TestAGroupAnswersBeforeTheRawToken(t *testing.T) {
	t.Parallel()

	member := tapeToken("*x", token.AliasType, 9, 4, 12)
	tk := tapeToken("a", token.StringType, 3, 5, 7)
	tk.Group = group.NewTokenGroup(group.TokenGroupAlias, []*group.TapeToken{member})

	assert.Equal(t, token.AliasType, tk.Type(), "the type comes from the group's first member")
	assert.Equal(t, group.TokenGroupAlias, tk.GroupType())
	assert.Equal(t, 9, tk.Line())
	assert.Equal(t, 4, tk.Column())
	assert.Equal(t, "*x", tk.RawToken().Value)
}

// TestSeqOfAGroupTokenIsWhereTheGroupBegins checks the fallback in Seq.
//
// A pass turns a token into a group by hanging the group on it, and a token the
// grouping made instead of drawing from the stream has no sequence of its own.
// Its group's first member has one, and the group begins there.
func TestSeqOfAGroupTokenIsWhereTheGroupBegins(t *testing.T) {
	t.Parallel()

	first := tapeToken("k", token.StringType, 1, 1, 11)
	second := tapeToken(":", token.MappingValueType, 1, 2, 12)

	made := new(group.TapeToken) // no Raw call, so seq stays 0
	made.Group = group.NewTokenGroup(group.TokenGroupMapKey, []*group.TapeToken{first, second})

	assert.Equal(t, int32(11), made.Seq())

	drawn := tapeToken("k", token.StringType, 1, 1, 3)
	drawn.Group = made.Group
	assert.Equal(t, int32(3), drawn.Seq(), "a token drawn from the stream keeps its own place")
}

// TestSetGroupTypeNeedsAGroup checks that retyping a token that never became a
// group is a no-op instead of a panic.
func TestSetGroupTypeNeedsAGroup(t *testing.T) {
	t.Parallel()

	tk := tapeToken("a", token.StringType, 1, 1, 1)
	tk.SetGroupType(group.TokenGroupAlias)
	assert.Equal(t, group.TokenGroupNone, tk.GroupType(), "no group, so nothing to retype")

	tk.Group = group.NewTokenGroup(group.TokenGroupMapKey, []*group.TapeToken{
		tapeToken("k", token.StringType, 1, 1, 2),
	})
	tk.SetGroupType(group.TokenGroupMapKeyValue)
	assert.Equal(t, group.TokenGroupMapKeyValue, tk.GroupType())
}
