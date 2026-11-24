// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"
	"testing"
	"unsafe"

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

func TestAJoinKeepsOnlyThePartsThatWrite(t *testing.T) {
	head, inline, trailing, foot := rendered{}, rendered{}, rendered{}, rendered{}
	key, colon, value := leaf("k"), leaf(":"), leaf(" v")

	var p rendered
	allocs := testing.AllocsPerRun(100, func() {
		p = join(sepNone, head, key, colon, inline, value, trailing, foot)
	})

	assert.Equal(t, 1.0, allocs, "the kept parts, and not the variadic slice")
	assert.Len(t, p.parts, 3)
	assert.Equal(t, "k: v", p.string())
}

func TestAJoinOfOnePartIsThatPart(t *testing.T) {
	text := leaf("v").indentedBy(2)

	var p rendered
	allocs := testing.AllocsPerRun(100, func() { p = join(sepNone, leaf(""), text, rendered{}) })

	assert.Zero(t, allocs)
	assert.Equal(t, text, p)
}

// TestAPieceTakesFiftySixBytes guards the field order of rendered: join copies a piece per part it
// keeps, and a field moved between two one-byte fields pads it back to 64.
func TestAPieceTakesFiftySixBytes(t *testing.T) {
	assert.Equal(t, uintptr(56), unsafe.Sizeof(rendered{}))
}

// TestShiftingAPieceWritesLikeAWrapper checks the shortcut in rendered.shifted against the wrapper it replaces, for
// a leaf and a join, hanging or not, and for a piece that already carries an indent.
func TestShiftingAPieceWritesLikeAWrapper(t *testing.T) {
	pieces := []rendered{
		leaf("a"), leaf("a\nb"), leaf("\na\n\nb"),
		join(sepBreak, leaf("a"), leaf("b: c")),
		leaf("a\nb").indentedBy(2),
	}
	for _, p := range pieces {
		for _, hanging := range []bool{false, true} {
			wrapped := rendered{parts: []rendered{p}, indent: 4, hanging: hanging, size: p.size, spans: p.spans, leads: p.leads}
			shifted := p.shifted(4, hanging)

			assert.Equalf(t, wrapped.string(), shifted.string(), "%q, hanging %v", p.string(), hanging)
			assert.Equalf(t, wrapped.withoutLead().string(), shifted.withoutLead().string(), "%q without its lead", p.string())

			outer := join(sepNone, leaf("- "), shifted).indentedBy(2)
			assert.Equalf(t, join(sepNone, leaf("- "), wrapped).indentedBy(2).string(), outer.string(), "%q nested", p.string())
		}
	}

	p := leaf("a\nb")
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = p.shifted(3, true) }))
}
