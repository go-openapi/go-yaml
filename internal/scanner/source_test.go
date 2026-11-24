// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"math"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestSourceLengthBound holds maxSourceLen at its edge.
//
// The refusal itself is out of a test's reach: it takes a source of two gigabytes. That is why the bound is a
// predicate of its own, and not a comparison written inline in validateSource.
// An off-by-one is how this check fails, and a predicate can be read at any size.
//
// A column stands one past the last byte of its line, so a source of exactly math.MaxInt32 bytes would put a column at
// math.MaxInt32+1 and wrap it. maxSourceLen leaves room for that byte.
func TestSourceLengthBound(t *testing.T) {
	assert.Equal(t, math.MaxInt32-1, maxSourceLen)
	assert.False(t, sourceTooLong(0))
	assert.False(t, sourceTooLong(1<<20))
	assert.False(t, sourceTooLong(maxSourceLen), "a source of exactly maxSourceLen bytes is read")
	assert.True(t, sourceTooLong(maxSourceLen+1))
	assert.True(t, sourceTooLong(math.MaxInt32))
}

// TestValidateSourceReadsAnOrdinarySource checks that the length gate hands an ordinary document on to the printable
// check instead of standing in front of it.
func TestValidateSourceReadsAnOrdinarySource(t *testing.T) {
	_, err := validateSource("a: 1\nb: [c, d]\n")
	require.NoError(t, err)
	_, err = validateSource("a: \x00\n")
	assert.ErrorContains(t, err, "a YAML stream may not hold")
}
