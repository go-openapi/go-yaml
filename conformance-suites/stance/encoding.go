// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stance

// The encoding properties, which belong to no language in particular.
//
// A byte order mark is a question about the bytes, asked before any grammar
// reads them, so the vocabulary and the shapes that exercise it are shared:
// every parser that takes bytes has to answer this, and answering it wrongly
// looks the same in all of them.
const (
	// TagBOM marks a document opening with a UTF-8 byte order mark. It declares
	// no change of encoding, which is what makes ignoring it a defensible
	// position and skipping the others not.
	TagBOM Tag = "encoding/bom"
	// TagUTF16 marks a document that announces or looks like UTF-16.
	TagUTF16 Tag = "encoding/utf16"
	// TagUTF32 marks a document that announces or looks like UTF-32.
	TagUTF32 Tag = "encoding/utf32"
	// TagNotUTF8 marks bytes that are not well-formed UTF-8 at all: lone
	// continuation bytes, overlong sequences, truncated sequences, surrogates
	// encoded directly, and code points beyond U+10FFFF.
	TagNotUTF8 Tag = "encoding/not-utf8"
)

// Mark is an encoding signature that can open a document.
type Mark struct {
	// Name identifies the mark in a shape's name.
	Name string
	// Bytes is the signature itself.
	Bytes []byte
	// Declares is the encoding a document opening with this mark claims to be
	// in, and is what makes skipping the mark right or wrong. Skipping a UTF-8
	// mark leaves the rest of the document meaning what it meant; skipping a
	// UTF-16 one leaves bytes that were never UTF-8 to begin with.
	Declares string
	// Exhibits is what a document carrying this mark is meant to exhibit. It is
	// the shape generator's *intent*, checked against what a tagger actually
	// detects rather than trusted -- see the marks test.
	Exhibits []Tag
}

// Marks is every byte order mark, including the two that are prefixes of
// others.
//
// UTF-32LE opens with the UTF-16LE mark, so a parser checking two bytes before
// four will diagnose a UTF-32 document as UTF-16. UTF-32BE opens with two NUL
// bytes and matches no shorter mark at all, so a parser that checks only the
// two-byte forms misses it entirely and falls through to whatever its ordinary
// error for a NUL is. Both are real and both were found this way.
var Marks = []Mark{
	{
		Name:     "utf-8",
		Bytes:    []byte{0xEF, 0xBB, 0xBF},
		Declares: "UTF-8",
		Exhibits: []Tag{TagBOM},
	},
	{
		Name:     "utf-16le",
		Bytes:    []byte{0xFF, 0xFE},
		Declares: "UTF-16LE",
		Exhibits: []Tag{TagUTF16, TagNotUTF8},
	},
	{
		Name:     "utf-16be",
		Bytes:    []byte{0xFE, 0xFF},
		Declares: "UTF-16BE",
		Exhibits: []Tag{TagUTF16, TagNotUTF8},
	},
	{
		Name:     "utf-32le",
		Bytes:    []byte{0xFF, 0xFE, 0x00, 0x00},
		Declares: "UTF-32LE",
		Exhibits: []Tag{TagUTF32, TagNotUTF8},
	},
	{
		Name:     "utf-32be",
		Bytes:    []byte{0x00, 0x00, 0xFE, 0xFF},
		Declares: "UTF-32BE",
		Exhibits: []Tag{TagUTF32, TagNotUTF8},
	},
}
