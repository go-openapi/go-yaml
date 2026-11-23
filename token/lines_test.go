// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestLookbackResetStartsAFreshStream derives a blank line above a token from the one before it, then resets the
// Lookback: the same token, first of a new stream, has nothing above it.
func TestLookbackResetStartsAFreshStream(t *testing.T) {
	first := token.MakeString("a", "a", token.At(1, 1, 0, 0))
	second := token.MakeString("b", "b", token.At(3, 1, 3, 0))

	var l token.Lookback
	l.Derive(&first)
	l.Derive(&second)
	assert.True(t, second.BlankLineAbove(), "line 2 is empty")

	l.Reset()
	again := token.MakeString("b", "b", token.At(3, 1, 3, 0))
	l.Derive(&again)
	assert.False(t, again.BlankLineAbove(), "the first token of a stream has nothing above it")
	assert.Zero(t, again.CommentBreaksAbove())
}
