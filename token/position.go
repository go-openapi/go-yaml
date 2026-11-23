// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "fmt"

// Position type for position in YAML document
// Position is where a token stands in the source.
//
// Line and Column count from 1 and count characters, which is what YAML
// measures indentation in. Offset counts from 0 and counts bytes, so
// src[Offset:] is the token: it addresses the source a caller handed in, and a
// caret drawn from it lands on the right character.
//
// Offset addresses the token for 97.1% of the YAML Test Suite's tokens; Line
// and Column for 94.2%. Two kinds of token are still reported early: block
// scalar content, whose offset is counted back from the cursor by the length
// of the folded value, and the Invalid token an error carries.
// scanner/offset_test.go holds the count of each.
//
// The four are int32. A document large enough to overflow one does not fit in
// memory to begin with, and a token holds this by value rather than pointing at
// it, so its width is the token's width.
type Position struct {
	Line   int32
	Column int32
	// where packs Offset in its low 32 bits and IndentNum in its high 32,
	// which keeps a token inside the nine registers an argument or a result
	// may use: Go counts a struct's fields rather than its words, so four
	// int32 here cost four registers and two of them cost one.
	//
	// Read them with [Position.Offset] and [Position.IndentNum].
	where uint64
}

// offsetMask selects the low 32 bits of Position.where, which hold Offset.
const offsetMask = 1<<32 - 1

// Offset is the byte the token starts at, counting from 0, so that src[Offset:]
// is the token.
func (p Position) Offset() int32 { return int32(p.where & offsetMask) }

// IndentNum is the number of spaces the line the token stands on is indented
// by.
func (p Position) IndentNum() int32 { return int32(p.where >> 32) }

// SetOffset records the byte the token starts at.
func (p *Position) SetOffset(offset int32) {
	p.where = p.where&^offsetMask | uint64(uint32(offset))
}

// SetIndentNum records how far the token's line is indented.
func (p *Position) SetIndentNum(indent int32) {
	p.where = p.where&offsetMask | uint64(uint32(indent))<<32
}

// At builds a position, which the packed fields keep a literal from doing.
func At(line, column, offset, indentNum int32) Position {
	return Position{
		Line:   line,
		Column: column,
		where:  uint64(uint32(offset)) | uint64(uint32(indentNum))<<32,
	}
}

// String position to text.
func (p *Position) String() string {
	return fmt.Sprintf("[line:%d,column:%d,offset:%d]", p.Line, p.Column, p.Offset())
}
