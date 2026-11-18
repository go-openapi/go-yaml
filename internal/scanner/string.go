// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

// TODO(perf): should be folded in the main loop as we are making a redundant byte compare here.
func (s *Scanner) scanQuote(ctx *Context, ch rune) (bool, error) {
	if ctx.existsBuffer() || s.inAnchorName(ch) {
		return false, nil
	}

	if ch == '\'' {
		tk, err := s.scanSingleQuote(ctx)
		if err != nil {
			return false, err
		}
		ctx.addTokenValue(tk)
	} else {
		tk, err := s.scanDoubleQuote(ctx)
		if err != nil {
			return false, err
		}
		ctx.addTokenValue(tk)
	}

	ctx.clear()

	return true, nil
}

// TODO(perf): rechallenge with SVAR - we discarded it only because our current corpus doesn't quote.
// produce a micro-benchmark with quoted scalars and compare.
func (s *Scanner) scanSingleQuote(ctx *Context) (token.Token, error) {
	ctx.addOriginBuf('\'')
	baseIndent := s.contentIndent()
	srcpos := s.pos()
	startIndex := ctx.idx + 1
	src := ctx.src
	size := int32(len(src))

	// A single-quoted scalar reads back as the source between its quotes unless a line break is folded or
	// a "''" stands for one quote.
	// Until one of those happens the value is a window on src and nothing is built: value stays nil and copied stays
	// false.
	//
	// The first rewrite copies what has been passed over so far, and the rest of the scalar is appended as before.
	value := s.quoted[:0]
	copied := false
	keep := func(upto int32) {
		if !copied {
			value = append(value, src[startIndex:upto]...)
			copied = true
		}
	}

	isFirstLineChar := false
	isNewLine := false

	var width int // TODO: should be int32
	for idx := startIndex; idx < size; idx += int32(width) {
		var c rune
		c, width = utf8.DecodeRuneInString(src[idx:]) // TODO: var w int ; width = int32(w)
		if !isNewLine {
			s.progressColumn(ctx, 1)
		} else {
			isNewLine = false
		}
		ctx.addOriginBuf(c)

		switch {
		case isNewLineChar(c):
			keep(idx)
			var err error
			if value, err = s.foldQuotedLineBreak(ctx, value, isFirstLineChar, src[idx+int32(width):], baseIndent); err != nil {
				return token.Token{}, err
			}
			isFirstLineChar = true
			isNewLine = true

			continue
		case isFirstLineChar && c == ' ':
			continue
		case isFirstLineChar && c == '\t':
			if s.lastDelimColumn >= s.column {
				return token.Token{}, ErrInvalidToken("tab character cannot be used for indentation in single-quoted text", token.Invalid(ctx.origin(), s.pos()))
			}

			continue
		case c != '\'':
			if copied {
				value = utf8.AppendRune(value, c)
			}
			isFirstLineChar = false

			continue
		case idx+int32(width) < int32(len(ctx.src)) && ctx.src[idx+int32(width)] == '\'':
			// '' handle as ' character
			keep(idx)
			value = utf8.AppendRune(value, c)
			ctx.addOriginBuf(c)
			idx++
			s.progressColumn(ctx, 1)

			continue
		default:
			// pick token

			s.progressColumn(ctx, 1)
			text := src[startIndex:idx]
			if copied {
				text = string(value)
				s.quoted = value[:0]
			}

			return token.MakeSingleQuote(text, ctx.origin(), srcpos), nil
		}
	}

	s.progressColumn(ctx, 1)

	return token.Token{}, ErrInvalidToken("could not find end character of single-quoted text", token.Invalid(ctx.origin(), srcpos))
}

// foldQuotedLineBreak folds the line break a quoted scalar reads into value: it drops the spaces ending the line,
// and appends a space, or a line feed where the line held nothing but the break. It then checks rest, the text after
// the break, for a document marker and for a continuation line not indented past baseIndent.
func (s *Scanner) foldQuotedLineBreak(ctx *Context, value []byte, emptyLine bool, rest string, baseIndent int32) ([]byte, error) {
	value = bytes.TrimRight(value, " ")
	if emptyLine {
		value = append(value, '\n')
	} else {
		value = append(value, ' ')
	}
	s.progressLine(ctx)
	if rest == "" {
		return value, nil
	}
	if err := s.validateDocumentSeparatorMarker(ctx, rest); err != nil {
		return nil, err
	}
	if err := s.checkContinuationIndent(ctx, rest, baseIndent); err != nil {
		return nil, err
	}

	return value, nil
}

// TODO(perf): same remark as above.
//
//nolint:mnd,gocyclo,cyclop,maintidx,gocognit // a case per rune to check and per escape sequence; named constants would not read better
func (s *Scanner) scanDoubleQuote(ctx *Context) (token.Token, error) {
	ctx.addOriginBuf('"')
	baseIndent := s.contentIndent()
	srcpos := s.pos()
	startIndex := ctx.idx + 1
	src := ctx.src
	size := int32(len(src))
	// As in scanSingleQuote: the value stays a window on src until a folded line break, an escape, or a tab dropped
	// before one rewrites it.
	// The first time that happens, keep copies everything passed over so far, and the rest appends as before.
	// A scalar holding none of the three, which is most of them, is never built.
	value := s.quoted[:0]
	copied := false
	keep := func(upto int32) {
		if !copied {
			value = append(value, src[startIndex:upto]...)
			copied = true
		}
	}
	isFirstLineChar := false
	isNewLine := false

	var width int
	for idx := startIndex; idx < size; idx += int32(width) {
		var c rune
		c, width = utf8.DecodeRuneInString(src[idx:])
		if !isNewLine {
			s.progressColumn(ctx, 1)
		} else {
			isNewLine = false
		}
		ctx.addOriginBuf(c)
		switch {
		case isNewLineChar(c):
			keep(idx)
			var err error
			if value, err = s.foldQuotedLineBreak(ctx, value, isFirstLineChar, src[idx+int32(width):], baseIndent); err != nil {
				return token.Token{}, err
			}
			isFirstLineChar = true
			isNewLine = true

			continue
		case isFirstLineChar && c == ' ':
			continue
		case isFirstLineChar && c == '\t':
			if s.lastDelimColumn >= s.column {
				return token.Token{}, ErrInvalidToken("tab character cannot be used for indentation in double-quoted text", token.Invalid(ctx.origin(), s.pos()))
			}
			continue
		case c == '\\':
			keep(idx)
			isFirstLineChar = false
			if idx+1 >= size {
				value = utf8.AppendRune(value, c)

				continue
			}
			nextChar, _ := utf8.DecodeRuneInString(src[idx+1:])
			var progress int32
			// Each escape standing for one character repeats the same three statements. That reads badly, and it is the
			// fastest shape measured: this switch compiles to a jump table whose constants sit in the instruction stream.
			//
			// Two rewrites were tried against BenchmarkScannerNextToken, interleaved in one window, and both cost. A
			// [utf8.RuneSelf]-wide lookup table behind a five-case switch is +3.1% per token on escaped-dense-1000
			// (p=0.047, n=12), an indexed load where there was none.
			// One switch whose arms set only the character, with the three statements after it, is +1.2% geomean and
			// +4.6% on escaped-100 (p=0.003, n=10), a branch where there was none.
			switch nextChar {
			case '0':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x00)
			case 'a':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x07)
			case 'b':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x08)
			case 't':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x09)
			case 'n':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x0A)
			case 'v':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x0B)
			case 'f':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x0C)
			case 'r':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x0D)
			case 'e':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x1B)
			case ' ':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x20)
			case '"':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x22)
			case '/':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x2F)
			case '\\':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x5C)
			case 'N':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x85)
			case '_':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0xA0)
			case 'L':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x2028)
			case 'P':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, 0x2029)
			case 'x':
				// \x00 style must have 3 characters at least.
				if idx+3 >= size {
					return token.Token{}, ErrInvalidToken("not enough length for escaped 8-bit character", token.Invalid(ctx.origin(), s.pos()))
				}
				progress = 3
				codeNum, isHex := hexDigitsToInt(src[idx+2 : idx+progress+1])
				if !isHex {
					return token.Token{}, ErrInvalidToken("found a character that is not a hexadecimal digit in escaped 8-bit character", token.Invalid(ctx.origin(), s.pos()))
				}
				// Two hex digits, so codeNum is at most 0xFF and the narrowing cannot wrap.
				value = utf8.AppendRune(value, rune(codeNum))
			case 'u':
				// \u0000 style must have 5 characters at least.
				if idx+5 >= size {
					return token.Token{}, ErrInvalidToken("not enough length for escaped UTF-16 character", token.Invalid(ctx.origin(), s.pos()))
				}
				progress = 5
				codeNum, isHex := hexDigitsToInt(src[idx+2 : idx+6])
				if !isHex {
					return token.Token{}, ErrInvalidToken("found a character that is not a hexadecimal digit in escaped UTF-16 character", token.Invalid(ctx.origin(), s.pos()))
				}

				// handle surrogate pairs.
				if codeNum >= 0xD800 && codeNum <= 0xDBFF {
					high := codeNum

					// \u0000\u0000 style must have 11 characters at least.
					if idx+11 >= size {
						return token.Token{}, ErrInvalidToken("not enough length for escaped UTF-16 surrogate pair", token.Invalid(ctx.origin(), s.pos()))
					}

					if src[idx+6] != '\\' || src[idx+7] != 'u' {
						return token.Token{}, ErrInvalidToken("found unexpected character after high surrogate for UTF-16 surrogate pair", token.Invalid(ctx.origin(), s.pos()))
					}

					low, isHex := hexDigitsToInt(src[idx+8 : idx+12])
					if !isHex {
						return token.Token{}, ErrInvalidToken("found a character that is not a hexadecimal digit in the low surrogate", token.Invalid(ctx.origin(), s.pos()))
					}
					if low < 0xDC00 || low > 0xDFFF {
						return token.Token{}, ErrInvalidToken("found unexpected low surrogate after high surrogate", token.Invalid(ctx.origin(), s.pos()))
					}
					codeNum = ((high - 0xD800) * 0x400) + (low - 0xDC00) + 0x10000
					progress += 6
				}
				escaped, isChar := escapedRune(codeNum)
				if !isChar {
					return token.Token{}, ErrInvalidToken(escapeNamesNoCharacter, token.Invalid(ctx.origin(), s.pos()))
				}
				value = utf8.AppendRune(value, escaped)
			case 'U':
				// \U00000000 style must have 9 characters at least.
				if idx+9 >= size {
					return token.Token{}, ErrInvalidToken("not enough length for escaped UTF-32 character", token.Invalid(ctx.origin(), s.pos()))
				}
				progress = 9
				codeNum, isHex := hexDigitsToInt(src[idx+2 : idx+10])
				if !isHex {
					return token.Token{}, ErrInvalidToken("found a character that is not a hexadecimal digit in escaped UTF-32 character", token.Invalid(ctx.origin(), s.pos()))
				}
				escaped, isChar := escapedRune(codeNum)
				if !isChar {
					return token.Token{}, ErrInvalidToken(escapeNamesNoCharacter, token.Invalid(ctx.origin(), s.pos()))
				}
				value = utf8.AppendRune(value, escaped)
			case '\n':
				isFirstLineChar = true
				isNewLine = true
				ctx.addOriginBuf(nextChar)
				s.progressColumn(ctx, 1)
				s.progressLine(ctx)
				idx++
				continue
			case '\r':
				isFirstLineChar = true
				isNewLine = true
				ctx.addOriginBuf(nextChar)
				s.progressLine(ctx)
				progress = 1
				// Skip \n after \r in CRLF sequences
				if idx+2 < size && src[idx+2] == '\n' {
					ctx.addOriginBuf('\n')
					progress = 2
				}
			case '\t':
				progress = 1
				ctx.addOriginBuf(nextChar)
				value = utf8.AppendRune(value, nextChar)
			default:
				s.progressColumn(ctx, 1)
				return token.Token{}, ErrInvalidToken(fmt.Sprintf("found unknown escape character %q", nextChar), token.Invalid(ctx.origin(), s.pos()))
			}
			// The escapes naming a code point, \xXX, \uXXXX and \UXXXXXXXX, leave the marker and its digits to be
			// recorded here.
			// Every other case records what it consumed as it goes.
			// These three cannot: a surrogate pair settles how far it reaches only once the low half is read.
			if isCodePointEscape(nextChar) {
				for i := idx + 1; i <= idx+progress && i < size; i++ {
					ctx.addOriginBuf(rune(src[i]))
				}
			}
			idx += progress
			s.progressColumn(ctx, progress)
			continue
		case c == '\t':
			var (
				foundNotSpaceChar bool
				progress          int32
			)
			for i := idx + 1; i < size; i++ {
				if src[i] == ' ' || src[i] == '\t' {
					progress++
					continue
				}
				if isNewLineChar(rune(src[i])) {
					break
				}
				foundNotSpaceChar = true
			}
			if foundNotSpaceChar {
				// The tab stands in the text, so it is the source's own byte and the window still holds.
				if copied {
					value = utf8.AppendRune(value, c)
				}
				if src[idx+1] != '"' {
					s.progressColumn(ctx, 1)
				}
			} else {
				// Dropped, with the whitespace after it, so the value parts company with the source here.
				// The origin still takes that whitespace: the document wrote it, and the token's end is counted from
				// the origin. Left out, the end fell short by it and a verbatim copy cut the closing quote.
				keep(idx)
				for i := idx + 1; i <= idx+progress; i++ {
					ctx.addOriginBuf(rune(src[i]))
				}
				idx += progress
				s.progressColumn(ctx, progress)
			}
			continue
		case c != '"':
			if copied {
				value = utf8.AppendRune(value, c)
			}
			isFirstLineChar = false
			continue
		default:
			// The '"' that closes the scalar.
			s.progressColumn(ctx, 1)
			text := src[startIndex:idx]
			if copied {
				text = string(value)
				s.quoted = value[:0]
			}

			return token.MakeDoubleQuote(text, ctx.origin(), srcpos), nil
		}
	}
	s.progressColumn(ctx, 1)

	return token.Token{}, ErrInvalidToken("could not find end character of double-quoted text", token.Invalid(ctx.origin(), srcpos))
}

// isCodePointEscape reports whether an escape names a code point by its digits, and so consumes more of the source than
// the backslash and one marker.
func isCodePointEscape(marker rune) bool {
	return marker == 'x' || marker == 'u' || marker == 'U'
}

// hexToInt returns the value of one hexadecimal digit, and whether the rune is one at all.
//
// Answering that is the point: subtracting '0' from whatever turned up gave a number for every rune, so an escape whose
// digits were not digits decoded to some other character instead of being refused.
//
//nolint:mnd // hex to int is fine, no need to redefine constants for these symbols
func hexToInt(b rune) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b) - '0', true
	case b >= 'a' && b <= 'f':
		return int(b) - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return int(b) - 'A' + 10, true
	default:
		return 0, false
	}
}

// hexRunesToInt returns the value a run of hexadecimal digits spells, and whether every rune in it was a digit.
// hexDigitsToInt reads b as hexadecimal digits, and reports whether every one of them is a digit at all.
func hexDigitsToInt(b string) (int, bool) {
	sum := 0
	for i := range len(b) {
		digit, isHex := hexToInt(rune(b[i]))
		if !isHex {
			return 0, false
		}
		sum += digit << (uint(len(b)-i-1) * 4) //nolint:mnd // the shift is fine
	}

	return sum, true
}

// escapeNamesNoCharacter refuses a "\uXXXX" or "\UXXXXXXXX" whose digits name no character.
const escapeNamesNoCharacter = "found an escaped code point that is not a character"

// escapedRune returns the character an escape's digits name, and false where they name none.
//
// "\U" takes eight hexadecimal digits, reaching 0xFFFFFFFF, past the largest code point and past what an int32 holds.
// rune is int32, so "\UFFFFFFFF" converted to -1, and utf8.AppendRune writes U+FFFD for any rune it cannot encode.
// The scalar then came back carrying a replacement character the document never wrote, and nothing reported it.
//
// D800 to DFFF are the other half.
// Each is one half of a UTF-16 pair and denotes no character alone, so utf8.ValidRune refuses them.
// A pair written as "\uD83D\uDE00" is combined before escapedRune sees it.
//
// The range is tested before the conversion, not after, so the narrowing below cannot be the thing that decides.
func escapedRune(codeNum int) (rune, bool) {
	if codeNum < 0 || codeNum > utf8.MaxRune {
		return 0, false
	}

	r := rune(codeNum)

	return r, utf8.ValidRune(r)
}
