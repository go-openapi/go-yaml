// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "fmt"

// Position records where a token starts in the source.
//
// Line and Column count from 1, in characters, as YAML measures indentation. Offset counts from 0, in bytes, so
// src[Offset:] starts with the token, and a caret drawn from Line and Column lands on its first character.
//
// Offset can start earlier than the token's text for two kinds of token: block scalar content, and the Invalid
// token an error carries. README.md in this directory gives the counts.
//
// The four numbers are int32. A token holds its Position by value, so the width of each number adds to the
// token's width.
type Position struct {
	Line   int32
	Column int32
	// where packs Offset in its low 32 bits and IndentNum in its high 32, so the two take one register where two
	// fields would take two. See README.md in this directory.
	//
	// Read them with [Position.Offset] and [Position.IndentNum].
	where uint64
}

// offsetMask selects the low 32 bits of Position.where, which hold Offset.
const offsetMask = 1<<32 - 1

// Offset returns the byte the token starts at, counting from 0.
func (p Position) Offset() int32 { return int32(p.where & offsetMask) }

// IndentNum returns the number of spaces the token's line is indented by.
func (p Position) IndentNum() int32 { return int32(p.where >> 32) }

// SetOffset records the byte the token starts at.
func (p *Position) SetOffset(offset int32) {
	p.where = p.where&^offsetMask | uint64(uint32(offset))
}

// At returns the position at line, column, offset and indentNum. A composite literal cannot set Offset and
// IndentNum, which share a packed field.
func At(line, column, offset, indentNum int32) Position {
	return Position{
		Line:   line,
		Column: column,
		where:  uint64(uint32(offset)) | uint64(uint32(indentNum))<<32,
	}
}

// String returns p as "[line:L,column:C,offset:O]".
func (p *Position) String() string {
	return fmt.Sprintf("[line:%d,column:%d,offset:%d]", p.Line, p.Column, p.Offset())
}
