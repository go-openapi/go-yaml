// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// isTimestamp reports whether value reads as a timestamp, or as a clock time such as "12:30".
//
// A clock time is no timestamp, and YAML 1.1 reads it as a base 60 integer. It is quoted to keep the
// quoting go.yaml.in/yaml/v3 applies.
func isTimestamp(value string) bool {
	if _, ok := ParseTimestamp(value); ok {
		return true
	}

	return isClockTime(value)
}

// isClockTime reports whether value is an hour of one or two digits below 24, a colon,
// and a minute of one or two digits below 60.
func isClockTime(value string) bool {
	c := timestampCursor{text: value}
	hour, hourDigits := c.number(2)
	if hourDigits == 0 || hour > 23 || !c.skip(':') {
		return false
	}
	minute, minuteDigits := c.number(2)

	return minuteDigits > 0 && minute < 60 && c.done()
}

// NeedsQuotedSpelling reports whether value holds a character that only a
// double-quoted scalar can carry, as an escape.
//
// A carriage return is one: YAML normalizes a stream's line breaks on read, so
// "\r\n" and a lone "\r" both arrive as "\n" and no plain, single-quoted or
// block scalar keeps one. U+FEFF, the byte order mark, is another: nb-char
// excludes it, so no plain or block scalar may hold it.
//
// The rest are the characters c-printable leaves out, which a YAML stream may
// not hold raw at all: the C0 controls but tab and line feed, DEL, the C1
// controls but NEL, the surrogates, U+FFFE and U+FFFF, and a byte that is part
// of no character. Written raw, the document they are in does not read back.
//
// Written as escapes inside a double-quoted scalar, all of them survive.
func NeedsQuotedSpelling(value string) bool {
	for i := 0; i < len(value); i++ {
		if c := value[i]; c < utf8.RuneSelf {
			if c < ' ' && c != '\t' && c != '\n' || c == 0x7F {
				return true
			}

			continue
		}

		r, width := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && width <= 1 || r == '\ufeff' || !printableAbove7F(r) {
			return true
		}
		i += width - 1
	}

	return false
}

// printableAbove7F is YAML 1.2's c-printable for a character above 0x7F.
//
//nolint:mnd // the ranges are the spec's, and naming each bound adds nothing.
func printableAbove7F(r rune) bool {
	switch {
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

// isLeadingZeroDecimal reports whether value is a run of decimal digits written
// with a leading zero, such as "088253".
//
// This library resolves integers as YAML 1.1 does, where "0" followed by digits
// is octal and "088253" is not octal at all -- so it reads a string, as PyYAML
// does. YAML 1.2's core schema resolves [-+]?[0-9]+ and reads the integer
// 88253. The two schemas also disagree on the value of "0777": 511 here and in
// go.yaml.in/yaml/v3, 777 under 1.2 core.
//
// Encoding such a value quoted means a reader following either schema reads
// back the string that was written. This is why the encoder already quotes the
// 1.1 bool keywords -- "y", "yes", "on" -- that this library does not resolve.
func isLeadingZeroDecimal(value string) bool {
	digits := value
	if digits != "" && (digits[0] == '+' || digits[0] == '-') {
		digits = digits[1:]
	}
	if len(digits) < 2 || digits[0] != '0' {
		return false
	}

	for i := range len(digits) {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}

	return true
}

// opensADocumentMarker reports whether value, written plain at the start of a
// line, reads as a "---" or "..." marker: three dashes or three dots followed by
// a blank or by nothing.
//
// A document's own scalar and a key of its top mapping start their line, so
// "---" encoded plain read back as a document holding null, "..." as an empty
// stream, and "... x" did not parse. "--- x" was quoted already, only through
// the "- " rule below.
func opensADocumentMarker(value string) bool {
	if !strings.HasPrefix(value, "---") && !strings.HasPrefix(value, "...") {
		return false
	}
	if len(value) == len("---") {
		return true
	}

	return value[len("---")] == ' ' || value[len("---")] == '\t'
}

// IsNeedQuoted checks whether the value needs quote for passed string or not.
func IsNeedQuoted(value string) bool {
	if value == "" {
		return true
	}
	if _, exists := reservedEncKeywordTypes[value]; exists {
		return true
	}
	if isNumber(value) {
		return true
	}
	if isLeadingZeroDecimal(value) {
		return true
	}
	if value == "-" {
		return true
	}
	if opensADocumentMarker(value) {
		return true
	}
	first := value[0]
	switch first {
	case '*', '&', '[', '{', '}', ']', ',', '!', '|', '>', '%', '\'', '"', '@', ' ', '\t', '`', ':':
		return true
	}
	last := value[len(value)-1]
	switch last {
	case ':', ' ', '\t':
		return true
	}
	if isTimestamp(value) {
		return true
	}
	if NeedsQuotedSpelling(value) {
		return true
	}
	for i, c := range value {
		switch c {
		case '#', '\\':
			return true
		case ':', '-':
			if i+1 < len(value) && value[i+1] == ' ' {
				return true
			}
		}
	}
	return false
}

// LiteralBlockHeader returns the block scalar header value needs, or "" where
// value has no block scalar spelling.
//
// A value holding a carriage return or a byte order mark has none, for two
// different reasons. YAML normalizes a stream's line breaks on read -- "\r\n"
// and a lone "\r" both become "\n" -- so a block scalar cannot carry a CR
// whatever it is written with. And nb-char excludes U+FEFF, so no block or
// plain scalar may hold one at all. Both have to be double-quoted, where each
// is an escape.
//
// A value whose first line holding text starts with a space has none either. A reader takes the block's
// indentation from that line, so it would read "\n 0" written as "|-" over an empty line and "   0" as "\n0".
// Writing it would need an indentation indicator, which no writer here emits.
//
// A value made only of line breaks keeps them with "|+": with no line of text, "|" and "|-" both read "".
func LiteralBlockHeader(value string) string {
	if NeedsQuotedSpelling(value) {
		return ""
	}

	lbc := DetectLineBreakCharacter(value)

	switch {
	case !strings.Contains(value, lbc):
		return ""
	case firstTextStartsWithSpace(value, lbc):
		return ""
	case strings.Trim(value, lbc) == "":
		return "|+"
	case strings.HasSuffix(value, fmt.Sprintf("%s%s", lbc, lbc)):
		return "|+"
	case strings.HasSuffix(value, lbc):
		return "|"
	default:
		return "|-"
	}
}

// firstTextStartsWithSpace reports whether the first line of value holding anything but a line break starts with
// a space.
func firstTextStartsWithSpace(value, lbc string) bool {
	for line := range strings.SplitSeq(value, lbc) {
		if line != "" {
			return line[0] == ' '
		}
	}

	return false
}

// DetectLineBreakCharacter detect line break character in only one inside scalar content scope.
func DetectLineBreakCharacter(src string) string {
	nc := strings.Count(src, "\n")
	rc := strings.Count(src, "\r")
	rnc := strings.Count(src, "\r\n")
	switch {
	case nc == rnc && rc == rnc:
		return "\r\n"
	case rc > nc:
		return "\r"
	default:
		return "\n"
	}
}
