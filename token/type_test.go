// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestEveryConstantHasAName requires each constant of Type, CharacterType and Indicator to have a name of its own,
// and a value past the last constant to have none. CharacterType.String returned "" for CharacterTypeInvalid.
func TestEveryConstantHasAName(t *testing.T) {
	names := func(name string, values []fmt.Stringer, past fmt.Stringer) {
		t.Helper()
		seen := map[string]bool{}
		for _, v := range values {
			s := v.String()
			assert.NotEmptyf(t, s, "%s %d has no name", name, v)
			assert.Falsef(t, seen[s], "%s %q names two constants", name, s)
			seen[s] = true
		}
		assert.Emptyf(t, past.String(), "%s past the last constant has a name", name)
	}

	var types []fmt.Stringer
	for typ := token.UnknownType; typ <= token.InvalidType; typ++ {
		types = append(types, typ)
	}
	names("Type", types, token.InvalidType+1)

	var characters []fmt.Stringer
	for c := token.CharacterTypeIndicator; c <= token.CharacterTypeInvalid; c++ {
		characters = append(characters, c)
	}
	names("CharacterType", characters, token.CharacterTypeInvalid+1)

	var indicators []fmt.Stringer
	for i := token.NotIndicator; i <= token.InvalidUseOfReservedIndicator; i++ {
		indicators = append(indicators, i)
	}
	names("Indicator", indicators, token.InvalidUseOfReservedIndicator+1)
}
