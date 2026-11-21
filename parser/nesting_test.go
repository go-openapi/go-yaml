// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestTheParseRefusesADocumentNestedPast10000Levels holds the nesting cap. A collection, an anchor, a tag and a "?"
// key each open a level, as Cursor.Depth counts them on a walk. A document of 10,000 levels parses, and one more
// is refused with ErrExceededMaxDepth at the token that opens it.
func TestTheParseRefusesADocumentNestedPast10000Levels(t *testing.T) {
	const limit = 10000

	for name, nested := range map[string]func(n int) string{
		"flow sequences":  func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) },
		"flow mappings":   func(n int) string { return strings.Repeat("{c: ", n) + "null" + strings.Repeat("}", n) },
		"block sequences": func(n int) string { return strings.Repeat("- ", n) + "x\n" },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(nested(limit)))
			require.NoError(t, err, "at the limit")

			_, err = parser.ParseBytes([]byte(nested(limit + 1)))
			require.ErrorIs(t, err, yamlerrors.ErrExceededMaxDepth)

			var yerr *yamlerrors.Error
			require.True(t, errors.As(err, &yerr))
			assert.Equal(t, int32(1), yerr.GetToken().Position.Line, "the error points into the document")
		})
	}

	// Anchors, tags and "?" keys each add a level of their own, so these reach the cap in fewer repetitions.
	for name, src := range map[string]string{
		"anchored sequences": strings.Repeat("[&a ", limit/2+1) + strings.Repeat("]", limit/2+1),
		"tagged sequences":   strings.Repeat("[!!seq ", limit/2+1) + strings.Repeat("]", limit/2+1),
		"explicit keys":      strings.Repeat("? ", limit+1) + "x\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(src))
			require.ErrorIs(t, err, yamlerrors.ErrExceededMaxDepth)
		})
	}

	t.Run("a walk stops at the same level", func(t *testing.T) {
		src := strings.Repeat("[", limit+1) + strings.Repeat("]", limit+1)
		_, err := parser.New().Walk([]byte(src), enterAll{})
		require.ErrorIs(t, err, yamlerrors.ErrExceededMaxDepth)
	})
}

// enterAll receives every node of a walk and keeps nothing.
type enterAll struct{}

func (enterAll) Enter(ast.Node, parser.Cursor) error  { return nil }
func (enterAll) Leave(ast.Node, parser.Closing) error { return nil }
