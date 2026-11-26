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
// Line and Column are int32. Offset takes 40 bits and IndentNum 24, packed in one word, so Offset reaches 1 TiB
// and IndentNum 16 M columns. A token holds its Position by value, so the width of each number adds to the
// token's width.
type Position struct {
	Line   int32
	Column int32
	// where packs Offset in its low 40 bits and IndentNum in its high 24, so the two take one register where two
	// fields would take two. See README.md in this directory.
	//
	// Read them with [Position.Offset] and [Position.IndentNum].
	where uint64
}

const (
	// OffsetBits is the width of Position.Offset. A source must be shorter than 1<<OffsetBits bytes.
	OffsetBits = 40
	// IndentBits is the width of Position.IndentNum. A line must be indented by fewer than 1<<IndentBits columns.
	IndentBits = 64 - OffsetBits

	// offsetMask selects the low OffsetBits of Position.where, which hold Offset.
	offsetMask = 1<<OffsetBits - 1
)

// Offset returns the byte the token starts at, counting from 0.
func (p Position) Offset() int { return int(p.where & offsetMask) }

// IndentNum returns the number of spaces the token's line is indented by.
func (p Position) IndentNum() int { return int(p.where >> OffsetBits) }

// SetOffset records the byte the token starts at.
func (p *Position) SetOffset(offset int) {
	p.where = p.where&^offsetMask | uint64(offset)&offsetMask
}

// At returns the position at line, column, offset and indentNum. A composite literal cannot set Offset and
// IndentNum, which share a packed field.
//
// Line and Column narrow to int32. The scanner refuses a source with more lines than that holds.
func At(line, column, offset, indentNum int) Position {
	return Position{
		Line:   int32(line),
		Column: int32(column),
		where:  uint64(offset)&offsetMask | uint64(indentNum)<<OffsetBits,
	}
}

// String returns p as "[line:L,column:C,offset:O]".
func (p *Position) String() string {
	return fmt.Sprintf("[line:%d,column:%d,offset:%d]", p.Line, p.Column, p.Offset())
}
