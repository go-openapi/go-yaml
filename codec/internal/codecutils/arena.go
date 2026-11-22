// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"strings"
	"unsafe"

	"github.com/go-openapi/go-yaml/internal/nocopy"
)

// Arena holds the strings a decode hands back, copied out of the document and
// boxed in bulk.
//
// The parse never copies: nocopy.String turns the document into a string once
// and every token's Value is a slice of it, so a decoded string used to point
// into the caller's bytes. One key kept out of a 100 MB file then held all
// 100 MB, and a Decoder reading from an io.Reader held the buffer it read.
// Clone gives the caller memory the document does not own.
//
// Copying each string on its own would cost two allocations where there was
// one: the bytes, then the interface header. The Arena allocates both in
// chunks, so it hands back owned strings for fewer allocations than pointing
// into the document took. A live string then pins a chunk rather than the
// document: 8 KB at worst, and a string longer than 2 KB gets its own
// allocation instead.
type Arena struct {
	text []byte
	strs []string
}

const (
	// arenaChunk is how much text one allocation copies into.
	arenaChunk = 8 << 10
	// arenaBig is the length above which a string is copied on its own, rather
	// than filling a chunk it would leave little of.
	arenaBig = arenaChunk / 4
	// arenaSlab is how many string headers one allocation boxes.
	arenaSlab = 256
)

// Clone returns s copied into memory the arena owns.
func (a *Arena) Clone(s string) string {
	switch {
	case len(s) == 0:
		return ""
	case len(s) > arenaBig:
		return strings.Clone(s)
	}

	if cap(a.text)-len(a.text) < len(s) {
		a.text = make([]byte, 0, arenaChunk)
	}
	at := len(a.text)
	a.text = append(a.text, s...)

	// The chunk is never appended past its capacity, so the bytes just written
	// stay where they are and the string keeps reading them.
	return nocopy.String(a.text[at:len(a.text)])
}

// stringProto carries the type word of an any holding a string. Box copies that
// word rather than fabricating one.
var stringProto any = "proto"

// eface is the layout of an interface value holding no methods: the type of
// what it holds, then a pointer to it.
type eface struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

// Box returns s as an any.
//
// Writing "any(s)" calls runtime.convTstring, which allocates 16 bytes for the
// string header every time -- 31,458 of them for citm_catalog. This appends to
// a slab of headers instead and points the interface at the one just written,
// so 256 strings are boxed by one allocation. The result is an ordinary
// interface value: it type-asserts, compares and reflects like any other.
//
// The slab is kept alive by the interfaces pointing into it, so a slab whose
// strings are all dropped is collected like anything else.
func (a *Arena) Box(s string) any {
	if len(a.strs) == cap(a.strs) {
		a.strs = make([]string, 0, arenaSlab)
	}
	a.strs = append(a.strs, s)

	v := stringProto
	(*eface)(unsafe.Pointer(&v)).data = unsafe.Pointer(&a.strs[len(a.strs)-1])

	return v
}

// Value returns s as an any the document does not own.
func (a *Arena) Value(s string) any {
	return a.Box(a.Clone(s))
}
