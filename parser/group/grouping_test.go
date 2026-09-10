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

// fed is one token to hand the Grouper: a type, the text it was read as, and
// the line it stands on.
type fed struct {
	typ   token.Type
	value string
	line  int32
}

// feed walks tokens through a Grouper the way the reader does -- one Feed per
// token, then one Finish -- and returns what comes out the far end.
//
// Tokens are written here by type instead of scanned, so a case says which
// token types the grouping joins and nothing about how a document spells them.
func feed(t *testing.T, tokens ...fed) (*group.Grouper, []*group.TapeToken) {
	t.Helper()

	g := group.NewGrouper(len(tokens))
	out := make([]*group.TapeToken, 0, len(tokens))
	for i, f := range tokens {
		line := f.line
		if line == 0 {
			line = 1
		}
		out = g.Feed(tapeToken(f.value, f.typ, line, int32(i+1), i+1), out)
	}

	out = g.Finish(out)

	return &g, out
}

// fedOK feeds tokens and requires the Grouper refused nothing.
func fedOK(t *testing.T, tokens ...fed) (*group.Grouper, []*group.TapeToken) {
	t.Helper()

	g, out := feed(t, tokens...)
	require.NoError(t, g.Err)

	return g, out
}

// TestFeedHandsOnATokenNoStageReads checks the common case: a scalar means
// nothing to any stage and comes straight out, ungrouped.
func TestFeedHandsOnATokenNoStageReads(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.StringType, "a", 1},
		fed{token.IntegerType, "1", 1},
	)

	require.Len(t, out, 2)
	for i, tk := range out {
		assert.Equalf(t, group.TokenGroupNone, tk.GroupType(), "token %d became a group", i)
	}
	assert.Equal(t, "a", out[0].RawToken().Value)
	assert.Equal(t, "1", out[1].RawToken().Value)
}

// TestABlockHeaderTakesTheTokenAfterIt checks that "|" and ">" join with their
// content as one token.
func TestABlockHeaderTakesTheTokenAfterIt(t *testing.T) {
	t.Parallel()

	for _, block := range []struct {
		name string
		typ  token.Type
		want group.TokenGroupType
	}{
		{"a literal", token.LiteralType, group.TokenGroupLiteral},
		{"a folded", token.FoldedType, group.TokenGroupFolded},
	} {
		t.Run(block.name, func(t *testing.T) {
			t.Parallel()

			_, out := fedOK(t,
				fed{block.typ, "|", 1},
				fed{token.StringType, "content", 2},
			)

			require.Len(t, out, 1, "the header and its content are one token")
			assert.Equal(t, block.want, out[0].GroupType())
			require.Equal(t, 2, out[0].Group.Len())
			assert.Equal(t, "content", out[0].Group.At(1).RawToken().Value)
		})
	}
}

// TestASecondBlockHeaderIsContent checks the rule stageBlockScalars states:
// whatever follows a header is its content, read as it stands.
func TestASecondBlockHeaderIsContent(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.LiteralType, "|", 1},
		fed{token.LiteralType, "|", 2},
		fed{token.StringType, "after", 3},
	)

	require.Len(t, out, 2)
	assert.Equal(t, group.TokenGroupLiteral, out[0].GroupType())
	assert.Equal(t, "|", out[0].Group.At(1).RawToken().Value, "the second header is the first one's content")
	assert.Equal(t, group.TokenGroupNone, out[1].GroupType())
}

// TestFinishHandsOverAHeaderWithNoContent checks that a stage still holding
// something at the end of the stream hands it over instead of dropping it.
//
// "a: |" at the end of a document is the shape that gets here.
func TestFinishHandsOverAHeaderWithNoContent(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t, fed{token.LiteralType, "|", 1})

	require.Len(t, out, 1, "the header comes out even though nothing followed it")
	assert.Equal(t, "|", out[0].RawToken().Value)
}

// TestAnAnchorTakesItsNameThenWhatItNames checks the two groups an anchor
// builds: the "&" with its name, and that with the node standing after it.
func TestAnAnchorTakesItsNameThenWhatItNames(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.AnchorType, "&", 1},
		fed{token.StringType, "x", 1},
		fed{token.IntegerType, "1", 1},
	)

	require.Len(t, out, 1, "the anchor, its name and the node it names are one token")
	assert.Equal(t, group.TokenGroupAnchor, out[0].GroupType())

	named := out[0].Group
	require.Equal(t, 2, named.Len())
	assert.Equal(t, group.TokenGroupAnchorName, named.At(0).GroupType())
	assert.Equal(t, "1", named.At(1).RawToken().Value)
}

// TestAnAliasTakesItsName checks that "*" joins with the name after it.
func TestAnAliasTakesItsName(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.AliasType, "*", 1},
		fed{token.StringType, "x", 1},
	)

	require.Len(t, out, 1)
	assert.Equal(t, group.TokenGroupAlias, out[0].GroupType())
	assert.Equal(t, "x", out[0].Group.At(1).RawToken().Value)
}

// TestACommentClosingALineAttachesToIt checks that a comment written after a
// token on the same line leaves the stream and is recorded against that token.
func TestACommentClosingALineAttachesToIt(t *testing.T) {
	t.Parallel()

	g, out := fedOK(t,
		fed{token.StringType, "a", 1},
		fed{token.CommentType, "# trailing", 1},
	)

	require.Len(t, out, 1, "the comment closes the line, so it does not stand on its own")
	require.Len(t, g.LineComments, 1)

	comment, ok := g.LineComments[out[0]]
	require.True(t, ok, "the comment is recorded against the token whose line it closes")
	assert.Equal(t, "# trailing", comment.Value)
}

// TestACommentOnItsOwnLineStaysInTheStream checks the other half of the same
// stage: a comment standing on a line of its own is a token like any other.
func TestACommentOnItsOwnLineStaysInTheStream(t *testing.T) {
	t.Parallel()

	g, out := fedOK(t,
		fed{token.StringType, "a", 1},
		fed{token.CommentType, "# above", 2},
	)

	require.Len(t, out, 2)
	assert.Equal(t, token.CommentType, out[1].Type())
	assert.Empty(t, g.LineComments, "nothing was closed, so nothing is recorded")
}

// TestADirectiveTakesItsLine checks that "%YAML 1.2" groups as one directive,
// closed by the "---" that starts the document.
func TestADirectiveTakesItsLine(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.DirectiveType, "%", 1},
		fed{token.StringType, "YAML", 1},
		fed{token.FloatType, "1.2", 1},
		fed{token.DocumentHeaderType, "---", 2},
	)

	require.Len(t, out, 2, "the directive, then the header that ends it")
	assert.Equal(t, group.TokenGroupDirective, out[0].GroupType())
	assert.Equal(t, token.DocumentHeaderType, out[1].Type())
}

// TestADirectiveIsRefusedWhereNoDocumentFollows checks the refusal a stream
// ending on a directive gets: a directive names what follows it.
func TestADirectiveIsRefusedWhereNoDocumentFollows(t *testing.T) {
	t.Parallel()

	g, _ := feed(t,
		fed{token.DirectiveType, "%", 1},
		fed{token.StringType, "YAML", 1},
		fed{token.FloatType, "1.2", 1},
	)

	require.Error(t, g.Err)
	assert.Contains(t, g.Err.Error(), "document not started")
}

// TestADirectiveWithNoNameIsRefused checks the other refusal: a "%" that never
// got a name.
func TestADirectiveWithNoNameIsRefused(t *testing.T) {
	t.Parallel()

	g, _ := feed(t, fed{token.DirectiveType, "%", 1})

	require.Error(t, g.Err)
	assert.Contains(t, g.Err.Error(), "undefined directive value")
}

// TestAKeyJoinsTheColonAfterIt checks the group a mapping entry's key becomes.
//
// The key stays where it stands in the stream and the group hangs on it, so the
// comments written between a key and its ':' keep their place after it.
func TestAKeyJoinsTheColonAfterIt(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.StringType, "a", 1},
		fed{token.MappingValueType, ":", 1},
		fed{token.IntegerType, "1", 1},
	)

	require.Len(t, out, 2, "the key and its ':' are one token, the value another")
	assert.Equal(t, group.TokenGroupMapKey, out[0].GroupType())
	require.Equal(t, 2, out[0].Group.Len())
	assert.Equal(t, "a", out[0].Group.At(0).RawToken().Value)
	assert.Equal(t, token.MappingValueType, out[0].Group.At(1).Type())
	assert.Equal(t, token.IntegerType, out[1].Type())
}

// TestAColonWithNothingBeforeItGetsANullKey checks that an absent key is
// supplied instead of refused: 1.2 reads ": value" as the null node keyed on
// nothing.
func TestAColonWithNothingBeforeItGetsANullKey(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.MappingValueType, ":", 1},
		fed{token.IntegerType, "1", 1},
	)

	require.Len(t, out, 2)
	require.Equal(t, group.TokenGroupMapKey, out[0].GroupType())
	assert.Equal(t, token.ImplicitNullType, out[0].Group.At(0).Type(),
		"the key is absent, so one is supplied",
	)
}

// TestAKeyOnAnEarlierLineIsNotTheKey checks the line rule for a block mapping:
// an implicit key shares its line with its ':', so a token on the line above is
// not a candidate and the ':' gets a null key.
func TestAKeyOnAnEarlierLineIsNotTheKey(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.StringType, "a", 1},
		fed{token.MappingValueType, ":", 2},
		fed{token.IntegerType, "1", 2},
	)

	require.Len(t, out, 3, "the token above stands on its own")
	assert.Equal(t, group.TokenGroupNone, out[0].GroupType())
	assert.Equal(t, group.TokenGroupMapKey, out[1].GroupType())
	assert.Equal(t, token.ImplicitNullType, out[1].Group.At(0).Type())
}

// TestPunctuationBeforeAColonGetsANullKey checks what "- :" groups as.
//
// A '-' cannot itself be a key, and precedesAbsentKey lists it, so the entry is
// keyed on the null node instead of being refused.
func TestPunctuationBeforeAColonGetsANullKey(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.SequenceEntryType, "-", 1},
		fed{token.MappingValueType, ":", 1},
		fed{token.IntegerType, "1", 1},
	)

	require.Len(t, out, 3, "the '-' stands on its own, then the keyed ':', then the value")
	assert.Equal(t, token.SequenceEntryType, out[0].Type())
	require.Equal(t, group.TokenGroupMapKey, out[1].GroupType())
	assert.Equal(t, token.ImplicitNullType, out[1].Group.At(0).Type())
}

// TestATagJoinsTheScalarAfterIt checks the group a tag and its node become.
func TestATagJoinsTheScalarAfterIt(t *testing.T) {
	t.Parallel()

	_, out := fedOK(t,
		fed{token.TagType, "!!str", 1},
		fed{token.StringType, "a", 1},
	)

	require.Len(t, out, 1)
	assert.Equal(t, group.TokenGroupScalarTag, out[0].GroupType())
	assert.Equal(t, "!!str", out[0].Group.At(0).RawToken().Value)
	assert.Equal(t, "a", out[0].Group.At(1).RawToken().Value)
}
