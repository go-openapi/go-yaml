// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"encoding/binary"

	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner/swar"
)

// alnumFastPath turns the bulk skip off, for the differential test that reads a document both ways.
//
// It is a variable and not a constant so that TestAlnumRunReadsWhatTheByteLoopReads can compare the two paths over the
// corpus. Nothing else writes it.
var alnumFastPath = true //nolint:gochecknoglobals // a test-only switch, read once per run

// isAlnum reports whether b is a letter or a digit, which is the continue-set the bulk skip steps over.
func isAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// alnumRun returns how many letters and digits stand at the cursor.
//
// A letter or a digit is none of the 24 characters scan has a case for, so every byte of the run would have fallen to
// the default arm and been appended one at a time. The run may therefore be taken in one go, which is what
// [Scanner.takeAlnumRun] does.
//
// A word holding any byte over 0x7f ends the word-at-a-time part: the masks are only exact under 0x80, and a
// multi-byte character is not a letter or a digit anyway, so the byte loop below finishes the run.
func (s *Scanner) alnumRun(ctx *Context) int {
	raw := ctx.raw
	size := len(raw)
	i := ctx.idx

	for i+8 <= size {
		w := binary.LittleEndian.Uint64(raw[i:])
		if w&swar.HighBits != 0 {
			break
		}
		if m := ^(swar.LetterMask(w) | swar.DigitMask(w)) & swar.HighBits; m != 0 {
			return i + swar.FirstByte(m) - ctx.idx
		}
		i += 8
	}

	for i < size && isAlnum(raw[i]) {
		i++
	}

	return i - ctx.idx
}

// takeAlnumRun reads the n letters and digits at the cursor, as n turns of the character loop would.
//
// Those turns would each have called addBuf, addOriginBuf and progressColumn, and updateIndent before them. This
// stands in for all four:
//
//   - addBuf appends the byte and moves notSpaceCharPos to the end of the buffer. Its rule about leading whitespace
//     cannot apply, the run holding no space or tab, so the whole run is appended and the mark goes to the end.
//   - addOriginBuf moves originEnd by one byte per character, which is what skipOrigin does for the run at once. It is
//     one byte per character because the run is ASCII by construction.
//   - progressColumn moves the column and the index by one apiece, likewise.
//   - updateIndent, reached with isFirstCharAtLine already false, sets the indent state to Keep and returns. Setting it
//     once stands for setting it n times. The caller guarantees the flag is false: a tab in the leading whitespace
//     falls to the default arm and leaves it standing, so after "\t" the run waits for the byte loop.
func (s *Scanner) takeAlnumRun(ctx *Context, n int) {
	if probe.Enabled {
		probe.Count("scan.alnum.runs", 1)
		probe.Count("scan.alnum.bytes", int64(n))
		probe.Max("scan.alnum.longest", int64(n))
	}

	ctx.buf = append(ctx.buf, ctx.raw[ctx.idx:ctx.idx+n]...)
	ctx.notSpaceCharPos = len(ctx.buf)
	ctx.skipOrigin(n)
	s.progressASCII(ctx, n)
	s.indentState = IndentStateKeep
}
