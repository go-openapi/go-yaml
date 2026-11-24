// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
	"unsafe"

	"github.com/go-openapi/go-yaml/internal/scanner/swar"
	"github.com/go-openapi/go-yaml/token"
)

// byteOrderMark is YAML 1.2's c-byte-order-mark.
//
// nb-char is c-printable less b-char and less this, so no node may hold a byte order mark: it marks a document
// prefix and nothing else.
// Scanner.checkByteOrderMark refuses one anywhere a node may go, and the scan steps over the rest, so a file saved by
// an editor that writes a mark still reads.
//
// A mark carries no information about the encoding here.
//
// # YAML spec
//
// The spec lets a stream announce UTF-16 or UTF-32 with a BOM.
// This library only supports UTF-8, and departs from YAML 1.2.2 on purpose in that one place.
//
// A UTF-16 stream's mark is two bytes that form no character, and validateStream refuses them.
const byteOrderMark = '\ufeff'

// byteOrderMarkText is the mark's three bytes, for the prefix tests that step over a run of them.
const byteOrderMarkText = string(byteOrderMark)

// validateStream checks that the source is text a YAML stream may hold.
//
// c-printable admits every character a stream may contain, so the control characters below 0x20 apart from tab, line
// feed and carriage return are not YAML however they got there.
//
// A stream is also Unicode, and a byte belonging to no character is not a character.
// Converting the source to runes turns each such byte into U+FFFD, and by the time anything else looks, the byte has
// gone unreported.
// So this reads the string, where the rest of the scanner works on runes.
func validateStream(text string) error {
	if at := firstUnprintable(text); at >= 0 {
		return unprintableErr(text, at)
	}

	return nil
}

// validateJSONText checks that the source is text JSON may hold: UTF-8, with no control character below 0x20 other
// than tab, line feed and carriage return. RFC 8259 admits no other control character raw, inside a string or out.
func validateJSONText(text string) error {
	for i := 0; i < len(text); {
		c := text[i]
		if c < utf8.RuneSelf {
			if c < 0x20 && c != 0x09 && c != 0x0A && c != 0x0D {
				return ErrInvalidToken(fmt.Sprintf("found character %q that JSON text may not hold raw", rune(c)),
					token.Invalid(text[i:i+1], streamPosition(text, i)))
			}
			i++

			continue
		}

		r, width := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && width <= 1 {
			return ErrInvalidToken("found a byte that is part of no character",
				token.Invalid(text[i:i+1], streamPosition(text, i)))
		}
		i += width
	}

	return nil
}

// firstUnprintable returns the offset of the first byte the stream may not hold, or -1 where every one of them is a
// character c-printable admits.
//
// Eight bytes at a time while they are ASCII, which covers nearly all of them in nearly every document.
// One word answers whether any of the eight is DEL, or a control character other than tab, line feed and carriage
// return.
// A word carrying a byte over 0x7f is stepped through one character at a time, because above 0x7f c-printable admits
// or refuses a character and not a byte.
// The word loop resumes after the run.
func firstUnprintable(text string) int {
	// The same bytes under the type the word loads need, as Context.reset does for the scan proper.
	//
	// TODO: validateStream is called from Init before the scan holds anything, so it could take the caller's []byte
	// directly and drop this conversion. That means validateSource on []byte and utf8.DecodeRune below.
	raw := unsafe.Slice(unsafe.StringData(text), len(text))

	i := 0
	for i+8 <= len(text) {
		w := binary.LittleEndian.Uint64(raw[i:])
		if w&swar.HighBits == 0 {
			if m := swar.ControlMask(w) &^ swar.AllowedControlMask(w); m != 0 {
				return i + swar.FirstByte(m)
			}
			i += 8

			continue
		}

		next, bad := readNonASCII(text, w, i)
		if bad >= 0 {
			return bad
		}
		i = next
	}

	return firstUnprintableTail(text, i)
}

// readNonASCII steps over the characters of the word at i that carries a byte over 0x7f, and returns where the bytes
// are ASCII again, or where the stream holds a character it may not.
//
// It takes the whole run, not one character.
// A document written in a script of three bytes to the character loaded and tested a word for every one of them, and
// every test failed the same way.
// It is a call, and not the loop's own code, to keep the word loop above small.
// The run is the cold path: twitter_status, the most non-Latin of the workloads, reaches it for one word in five.
func readNonASCII(text string, w uint64, i int) (int, int) {
	// The ASCII bytes standing in front of the first one over 0x7f are read from the word as usual.
	k := swar.FirstByte(w & swar.HighBits)
	if m := (swar.ControlMask(w) &^ swar.AllowedControlMask(w)) & (1<<(8*k) - 1); m != 0 {
		return 0, i + swar.FirstByte(m)
	}
	i += k

	for i < len(text) && text[i] >= utf8.RuneSelf {
		r, width := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && width <= 1 {
			return 0, i
		}
		if !printable(r) {
			return 0, i
		}
		i += width
	}

	return i, -1
}

// firstUnprintableTail reads the last bytes of the source, fewer than a word of them, one character at a time.
func firstUnprintableTail(text string, i int) int {
	for i < len(text) {
		if c := text[i]; c < utf8.RuneSelf {
			if !printableASCII(c) {
				return i
			}
			i++

			continue
		}

		r, width := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && width <= 1 {
			// Either a byte that is not text, or a U+FFFD the author wrote: only the width tells them apart.
			return i
		}
		if !printable(r) {
			return i
		}
		i += width
	}

	return -1
}

// unprintableErr names the character at offset i, which firstUnprintable found the stream may not hold.
func unprintableErr(text string, i int) error {
	r, width := utf8.DecodeRuneInString(text[i:])
	if r == utf8.RuneError && width <= 1 {
		return ErrInvalidToken("found a byte that is part of no character",
			token.Invalid(text[i:i+1], streamPosition(text, i)))
	}

	return ErrInvalidToken(
		fmt.Sprintf("found character %q that a YAML stream may not hold", r),
		token.Invalid(string(r), streamPosition(text, i)),
	)
}

// printableASCII answers [printable] for the bytes below utf8.RuneSelf, where it comes to one range and the three line
// and tab characters outside it.
func printableASCII(c byte) bool {
	return c >= 0x20 && c <= 0x7E || c == 0x09 || c == 0x0A || c == 0x0D
}

// streamPosition counts the characters before byte i, for the error naming the character a stream may not hold.
//
// Line, column and offset all count from 1, and offset counts characters, not bytes.
// validateStream reported them that way back when it counted every character to report one.
// A line ends at a line feed, a carriage return and line feed, or a carriage return alone, as for the scan: the error
// token stands among the tokens, and has to stand after the ones in front of it.
func streamPosition(text string, i int) token.Position {
	line, column, offset := 1, 1, 1
	for j, r := range text[:i] {
		offset++
		if r == '\n' || r == '\r' && (j+1 == len(text) || text[j+1] != '\n') {
			line++
			column = 1

			continue
		}
		column++
	}

	return token.At(int32(line), int32(column), int32(offset), 0)
}

// printable is YAML 1.2's c-printable.
//
//nolint:mnd // we have a lot of runes to check and making them constants won't really improve readability.
func printable(r rune) bool {
	switch {
	case r == 0x09 || r == 0x0A || r == 0x0D:
		return true
	case r >= 0x20 && r <= 0x7E:
		return true
	case r == 0x85:
		return true
	case r >= 0xA0 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	default:
		return r >= 0x10000 && r <= 0x10FFFF
	}
}
