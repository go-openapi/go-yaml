// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestEveryTypeHasAnIndicator checks the derivation covers every type, so a
// type added later cannot quietly fall through to NotIndicator.
func TestEveryTypeHasAnIndicator(t *testing.T) {
	indicators := map[token.Type]token.Indicator{
		token.SequenceEntryType: token.BlockStructureIndicator,
		token.MappingKeyType:    token.BlockStructureIndicator,
		token.MappingValueType:  token.BlockStructureIndicator,
		token.CollectEntryType:  token.FlowCollectionIndicator,
		token.SequenceStartType: token.FlowCollectionIndicator,
		token.SequenceEndType:   token.FlowCollectionIndicator,
		token.MappingStartType:  token.FlowCollectionIndicator,
		token.MappingEndType:    token.FlowCollectionIndicator,
		token.CommentType:       token.CommentIndicator,
		token.AnchorType:        token.NodePropertyIndicator,
		token.AliasType:         token.NodePropertyIndicator,
		token.TagType:           token.NodePropertyIndicator,
		token.LiteralType:       token.BlockScalarIndicator,
		token.FoldedType:        token.BlockScalarIndicator,
		token.SingleQuoteType:   token.QuotedScalarIndicator,
		token.DoubleQuoteType:   token.QuotedScalarIndicator,
		token.DirectiveType:     token.DirectiveIndicator,
	}

	for typ, want := range indicators {
		assert.Equalf(t, want, typ.Indicator(), "%s", typ)
		assert.Equalf(t, token.CharacterTypeIndicator, typ.CharacterType(), "%s", typ)
	}

	assert.Equal(t, token.CharacterTypeWhiteSpace, token.SpaceType.CharacterType())
	assert.Equal(t, token.CharacterTypeInvalid, token.InvalidType.CharacterType())
	assert.Equal(t, token.CharacterTypeMiscellaneous, token.StringType.CharacterType())
	assert.Equal(t, token.NotIndicator, token.StringType.Indicator())
}

// TestOnlyTheFourIntegerTypesAreIntegers holds Type.IsInteger to the types
// ScalarType hands back for a whole number, so a type added later cannot join
// them by accident.
//
// ast.integerKeyType reads it to pick the base an "!!int" key's digits are in,
// and a type wrongly counted an integer there would send text to
// ParseWholeNumber under a base it was not written in.
func TestOnlyTheFourIntegerTypesAreIntegers(t *testing.T) {
	integers := []token.Type{
		token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType, token.HexIntegerType,
	}
	for _, typ := range integers {
		assert.Truef(t, typ.IsInteger(), "%s", typ)
	}

	for typ := token.UnknownType; typ <= token.InvalidType; typ++ {
		if slices.Contains(integers, typ) {
			continue
		}
		assert.Falsef(t, typ.IsInteger(), "%s", typ)
	}
}
