// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package key_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser/internal/key"
	"github.com/go-openapi/go-yaml/token"
)

// TestSetResetForgetsEveryKey checks that Set.Reset forgets the keys of a mapping past the spill,
// and turns UseJSONNames off.
func TestSetResetForgetsEveryKey(t *testing.T) {
	t.Parallel()

	var s key.Set
	s.UseJSONNames(true)
	for i := range padTo + 8 {
		record(&s, 0, fmt.Sprintf("k%d", i), token.KeyString, int32(i+1))
	}

	s.Reset()
	require.Equal(t, 0, s.Base())
	for i := range padTo + 8 {
		got := record(&s, 0, fmt.Sprintf("k%d", i), token.KeyString, int32(i+1))
		assert.Falsef(t, got.repeat, "k%d is remembered after Reset", i)
	}

	s.Reset()
	record(&s, 0, "1", token.KeyInt, 1)
	assert.False(t, record(&s, 0, "1", token.KeyString, 2).repeat,
		`the integer 1 and the string "1" are two keys once Reset turns UseJSONNames off`)
}

// TestLedgerResetClosesEveryMapping checks that Ledger.Reset drops the mappings a stopped parse left open,
// so it remembers none of their keys and notes no repeat on them.
func TestLedgerResetClosesEveryMapping(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	outer := &ast.MappingNode{}
	_ = l.Open(outer)
	l.RecordOnce(l.Base(), "a", token.KeyString, at(1))
	l.RecordBuilt("seq(a)", "[a]", at(2))

	l.Reset()
	require.False(t, l.InMapping())
	require.Equal(t, 0, l.Base())

	inner := &ast.MappingNode{}
	closeInner := l.Open(inner)
	l.RecordOnce(l.Base(), "a", token.KeyString, at(3))
	l.RecordBuilt("seq(a)", "[a]", at(4))
	closeInner()

	assert.Empty(t, outer.Duplicates)
	assert.Empty(t, inner.Duplicates)
}
