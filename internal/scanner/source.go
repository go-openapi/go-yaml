// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"
	"math"

	"github.com/go-openapi/go-yaml/token"
)

// maxSourceLen is the longest source the scanner reads.
//
// [token.Position] stores lines and columns as int32, which keeps a token to 56 bytes.
// The scan counts them in int and narrows at every position it builds.
// The length of the source bounds all of them: an offset addresses a byte of it,
// and a column stands at most one past the last byte of its line.
// Refusing a longer source here, once, makes every one of those narrowings safe.
//
// The alternative was a checked conversion at each of the eleven sites.
// That spends a compare per token to report a document too large to hold in memory.
const maxSourceLen = math.MaxInt32 - 1

// validateSource checks the source before the scan reads a byte of it, and returns the offsets of the characters only
// a quoted scalar may hold.
func validateSource(text string) ([]int, error) {
	if sourceTooLong(len(text)) {
		return nil, ErrInvalidToken(
			fmt.Sprintf("a source must be under %d bytes, and this one is %d", maxSourceLen, len(text)),
			token.Invalid("", token.At(1, 1, 0, 0)),
		)
	}

	return validateStream(text)
}

// validateJSONSource checks a JSON source before the scan reads a byte of it.
func validateJSONSource(text string) error {
	if sourceTooLong(len(text)) {
		return ErrInvalidToken(
			fmt.Sprintf("a source must be under %d bytes, and this one is %d", maxSourceLen, len(text)),
			token.Invalid("", token.At(1, 1, 0, 0)),
		)
	}

	return validateJSONText(text)
}

// sourceTooLong reports whether the scanner refuses a source of n bytes.
//
// It stands apart from validateSource so that a test can check the bound at its edge.
// No test allocates two gigabytes to reach the refusal itself.
func sourceTooLong(n int) bool { return n > maxSourceLen }

// posInt narrows an int count to the int32 that [token.Position] stores for a line or a column.
//
// The scanner counts in int, and only Line and Column are narrower.
// See maxSourceLen for why the conversion cannot wrap.
func posInt(n int) int32 {
	return int32(n)
}
