// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseReturnsTheDocumentsBeforeTheOneThatFailed parses a stream whose third document does not parse. Parse
// returns the syntax error with the two documents before it, each complete. Walk returns no file: the nodes it handed
// over are recycled.
func TestParseReturnsTheDocumentsBeforeTheOneThatFailed(t *testing.T) {
	src := []byte("a: 1\n---\nb: [2, 3]\n---\nc: [1, 2\n---\nd: 4\n")

	file, err := parser.ParseBytes(src)
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
	require.NotNil(t, file)
	require.Len(t, file.Docs, 2)
	assert.Equal(t, "a: 1", file.Docs[0].Body.String())
	assert.Equal(t, "b: [2, 3]", file.Docs[1].Body.String())

	file, err = parser.ParseBytes([]byte("[1, 2\n"))
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
	require.NotNil(t, file)
	assert.Empty(t, file.Docs)

	walked, err := parser.New().Walk(src, discardVisitor{})
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
	assert.Nil(t, walked)
}
