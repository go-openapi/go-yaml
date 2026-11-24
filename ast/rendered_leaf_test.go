// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
)

// TestALeafFlattensLikeItsWrite checks the shortcut in rendered.string against
// the full write, for every leaf shape the shortcut takes or declines.
func TestALeafFlattensLikeItsWrite(t *testing.T) {
	for _, text := range []string{"", "v", "a b", "\n", "a\nb", "\na\n\nb\n"} {
		for _, indent := range []int32{0, 2} {
			for _, hanging := range []bool{false, true} {
				p := leaf(text)
				p.indent = indent
				p.hanging = hanging

				var b strings.Builder
				p.writeTo(&lineWriter{w: &b}, 0)

				assert.Equalf(t, b.String(), p.string(), "leaf %q, indent %d, hanging %v", text, indent, hanging)
			}
		}
	}
}

func TestALeafFlattensWithoutAllocating(t *testing.T) {
	p := leaf("key")
	p.indent = 4

	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = p.string() }))
}
