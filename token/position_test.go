// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestPositionKeepsItsPackedFieldsApart sets Offset and IndentNum, which share one field, and reads each back.
func TestPositionKeepsItsPackedFieldsApart(t *testing.T) {
	p := token.At(3, 7, 1<<31-1, 1<<31-1)
	assert.Equal(t, int32(3), p.Line)
	assert.Equal(t, int32(7), p.Column)
	assert.Equal(t, int32(1<<31-1), p.Offset())
	assert.Equal(t, int32(1<<31-1), p.IndentNum())

	p.SetOffset(42)
	assert.Equal(t, int32(42), p.Offset())
	assert.Equal(t, int32(1<<31-1), p.IndentNum(), "SetOffset wrote over IndentNum")
	assert.Equal(t, "[line:3,column:7,offset:42]", p.String())
}
