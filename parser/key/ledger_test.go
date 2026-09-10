// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package key_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser/key"
	"github.com/go-openapi/go-yaml/token"
)

// TestLedgerOpensAndClosesWithTheMapping checks the pairing InMapping reports
// on. RecordBuilt writes to the innermost mapping's map and needs one open.
func TestLedgerOpensAndClosesWithTheMapping(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	assert.False(t, l.InMapping())

	outer := new(ast.MappingNode)
	closeOuter := l.Open(outer)
	assert.True(t, l.InMapping())

	inner := new(ast.MappingNode)
	closeInner := l.Open(inner)
	assert.True(t, l.InMapping())

	closeInner()
	assert.True(t, l.InMapping(), "the outer mapping is still open")

	closeOuter()
	assert.False(t, l.InMapping())
}

// TestLedgerNotesARepeatOnTheMappingHoldingIt checks that a repeated key reaches
// the tree: the parse reads the document to the end and the mapping carries what
// was wrong with it, so a linter or a renderer can read a document a decoder
// refuses.
func TestLedgerNotesARepeatOnTheMappingHoldingIt(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	node := new(ast.MappingNode)
	defer l.Open(node)()

	base := l.Base()
	l.RecordOnce(base, "a", token.KeyString, at(1))
	assert.Empty(t, node.Duplicates, "one key is no repeat")

	l.RecordOnce(base, "a", token.KeyString, at(3))

	require.Len(t, node.Duplicates, 1)
	assert.Equal(t, ast.DuplicateKey{Name: "a", At: at(3), FirstAt: at(1)}, node.Duplicates[0])
}

// TestLedgerNotesARepeatOnTheInnermostMapping checks which mapping a repeat is
// recorded on. The mapping being read is the innermost one open, because an
// outer mapping's next key waits for the inner one to close.
func TestLedgerNotesARepeatOnTheInnermostMapping(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	outer := new(ast.MappingNode)
	defer l.Open(outer)()
	l.RecordOnce(l.Base(), "a", token.KeyString, at(1))

	inner := new(ast.MappingNode)
	innerBase := l.Base()
	closeInner := l.Open(inner)
	l.RecordOnce(innerBase, "b", token.KeyString, at(2))
	l.RecordOnce(innerBase, "b", token.KeyString, at(3))
	closeInner()
	l.Close(innerBase)

	assert.Empty(t, outer.Duplicates, "the repeat stands in the inner mapping")
	require.Len(t, inner.Duplicates, 1)
	assert.Equal(t, "b", inner.Duplicates[0].Name)
}

// TestLedgerUnderJSONNamesMarksTheMemberName checks that JSONNameOnly reaches
// the tree, so a load can tell a repeated YAML key from two YAML keys that
// write one JSON member.
func TestLedgerUnderJSONNamesMarksTheMemberName(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	l.UseJSONNames(true)
	node := new(ast.MappingNode)
	defer l.Open(node)()

	base := l.Base()
	l.RecordOnce(base, "1", token.KeyInt, at(1))
	l.RecordOnce(base, "1", token.KeyString, at(3))

	require.Len(t, node.Duplicates, 1)
	assert.True(t, node.Duplicates[0].JSONNameOnly)
	assert.Equal(t, at(1), node.Duplicates[0].FirstAt)
}

// TestLedgerRecordsNothingWithNoMappingOpen checks the guard on noteDuplicate.
// The parser records a flow key before it has built the mapping node, so a
// repeat can be found with nothing open to record it on.
func TestLedgerRecordsNothingWithNoMappingOpen(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	base := l.Base()

	assert.NotPanics(t, func() {
		l.RecordOnce(base, "a", token.KeyString, at(1))
		l.RecordOnce(base, "a", token.KeyString, at(3))
	})
}

// TestLedgerRecordReportsWhatTheSetFound checks the pass-through: Record answers
// where the key was first written and leaves the mapping untouched, where
// RecordOnce notes the repeat.
func TestLedgerRecordReportsWhatTheSetFound(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	node := new(ast.MappingNode)
	defer l.Open(node)()

	base := l.Base()
	_, _, defined := l.Record(base, "a", token.KeyString, at(1))
	require.False(t, defined)

	first, jsonOnly, defined := l.Record(base, "a", token.KeyString, at(3))
	assert.True(t, defined)
	assert.False(t, jsonOnly)
	assert.Equal(t, at(1), first)
	assert.Empty(t, node.Duplicates, "Record reports; RecordOnce records")
}

// TestLedgerCloseDropsTheMappingsKeys checks that the keys go with the mapping,
// so the next mapping opening at the same base starts empty.
func TestLedgerCloseDropsTheMappingsKeys(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	node := new(ast.MappingNode)
	defer l.Open(node)()

	base := l.Base()
	l.RecordOnce(base, "a", token.KeyString, at(1))
	require.Equal(t, base+1, l.Base())

	l.Close(base)
	assert.Equal(t, base, l.Base())

	l.RecordOnce(base, "a", token.KeyString, at(9))
	assert.Empty(t, node.Duplicates, "the mapping that used \"a\" has closed")
}

// TestLedgerRecordBuiltNotesARepeatUnderTheDisplayName checks the second half of
// the ledger: a key no single token names -- a sequence, a mapping -- is
// compared by the identity the parser builds from the finished node, and the
// repeat is named by what the document wrote.
func TestLedgerRecordBuiltNotesARepeatUnderTheDisplayName(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	node := new(ast.MappingNode)
	defer l.Open(node)()

	l.RecordBuilt("seq(string/a)", "[a]", at(1))
	assert.Empty(t, node.Duplicates)

	l.RecordBuilt("seq(string/a)", "[ a ]", at(3))

	require.Len(t, node.Duplicates, 1)
	assert.Equal(t, ast.DuplicateKey{Name: "[ a ]", At: at(3), FirstAt: at(1)}, node.Duplicates[0],
		"the repeat names itself as the document wrote it")
}

// TestLedgerRecordBuiltKeepsOneMappingsKeysToItself checks that built keys are
// scoped the way scalar keys are: builtKeys stands beside openMaps and is pushed
// and popped with it, so an identity an outer mapping used is no repeat for an
// inner one.
func TestLedgerRecordBuiltKeepsOneMappingsKeysToItself(t *testing.T) {
	t.Parallel()

	var l key.Ledger
	outer := new(ast.MappingNode)
	defer l.Open(outer)()
	l.RecordBuilt("seq(string/a)", "[a]", at(1))

	inner := new(ast.MappingNode)
	closeInner := l.Open(inner)
	l.RecordBuilt("seq(string/a)", "[a]", at(2))
	assert.Empty(t, inner.Duplicates, "the inner mapping has not used [a]")
	closeInner()

	l.RecordBuilt("seq(string/a)", "[a]", at(5))
	require.Len(t, outer.Duplicates, 1)
	assert.Equal(t, at(1), outer.Duplicates[0].FirstAt)
}
