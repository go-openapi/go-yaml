// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsonspike

import (
	"bytes"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/go-openapi/conformance-suites/stance"
)

// The implementation-defined properties of a JSON document that are peculiar to
// JSON.
//
// The encoding properties are not here: a byte order mark is the same question
// in every language, so its vocabulary and the shapes exercising it live in
// [stance]. What remains is what RFC 8259 specifically leaves open, and all of
// it sits on the far side of the grammar -- these are questions about what a
// document means, asked only once it has been agreed that it is one.
const (
	// TagNumberOutOfRange marks a number the grammar admits and float64 cannot
	// carry: it overflows, underflows to zero, or is an integer too large to
	// survive as one. RFC 8259 section 6 lets an implementation limit range and
	// precision, so a parser that converts is exposed where a parser that keeps
	// the text is not.
	TagNumberOutOfRange stance.Tag = "number/out-of-range"
	// TagLoneSurrogate marks a \uXXXX escape naming a surrogate that is not
	// part of a well-formed pair. RFC 8259 section 8.2 calls the behavior
	// unpredictable. The escape itself is grammatical -- four hex digits -- so
	// this is a question about the value, never about the syntax.
	TagLoneSurrogate stance.Tag = "string/lone-surrogate"
	// TagDeepNesting marks a document nested past deepEnough. No specification
	// sets a limit and every parser has one, whether it says so or not.
	TagDeepNesting stance.Tag = "structure/deep-nesting"
)

// deepEnough is where nesting stops being a shape and starts being a resource
// question. It is a declared threshold rather than a discovered one: the point
// is to tag the documents whose acceptance depends on a limit, and any value
// well above ordinary data and well below a stack overflow does that.
const deepEnough = 128

// Tags reports which implementation-defined properties a document exhibits.
//
// wellFormed is the grammar's verdict, and it is a parameter rather than
// something recomputed here because the two families of property sit on
// opposite sides of it. Encoding is asked of any bytes at all. The rest are
// only meaningful once there is a parse: "the number is out of range" says
// nothing about a document that has no numbers in it, only text that resembles
// one, and tagging it anyway would quietly excuse the parser from a case the
// suite settled.
//
// That distinction is not fussiness. A spurious tag does not fail anything --
// it makes a document undecidable, so it silently leaves the scored set. An
// over-eager tagger produces a corpus that looks complete and is not.
func Tags(src []byte, wellFormed bool) []stance.Tag {
	tags := EncodingTags(src)

	if !wellFormed {
		return tags
	}

	return append(tags, ValueTags(src)...)
}

// EncodingTags reports the properties that are decided before any production
// runs, and that determine what code points the bytes denote at all.
//
// A declared mark is recognized from the longest signature down, because the
// UTF-32LE mark opens with the whole UTF-16LE one and a shorter-first check
// would call every UTF-32 document UTF-16.
func EncodingTags(src []byte) []stance.Tag {
	var tags []stance.Tag

	if mark, ok := declaredMark(src); ok {
		tags = append(tags, mark.Exhibits...)
	} else if looksWide(src) {
		tags = append(tags, stance.TagUTF16, stance.TagNotUTF8)
	}

	if !utf8.Valid(src) && !slices.Contains(tags, stance.TagNotUTF8) {
		tags = append(tags, stance.TagNotUTF8)
	}

	return tags
}

// declaredMark returns the byte order mark the document opens with, preferring
// the longest match.
func declaredMark(src []byte) (stance.Mark, bool) {
	var (
		found stance.Mark
		ok    bool
	)

	for _, mark := range stance.Marks {
		if bytes.HasPrefix(src, mark.Bytes) && len(mark.Bytes) > len(found.Bytes) {
			found, ok = mark, true
		}
	}

	return found, ok
}

// ValueTags reports the properties that are decided after a parse succeeds, and
// that determine what the document means rather than whether it is one.
//
// It assumes src is grammatical. On anything else its answers are noise, which
// is why [Tags] will not call it otherwise.
func ValueTags(src []byte) []stance.Tag {
	var tags []stance.Tag

	if hasLoneSurrogate(src) {
		tags = append(tags, TagLoneSurrogate)
	}

	if hasWideNumber(src) {
		tags = append(tags, TagNumberOutOfRange)
	}

	if depth(src) > deepEnough {
		tags = append(tags, TagDeepNesting)
	}

	return tags
}

// looksWide recognizes an unmarked wide encoding by its shape rather than by
// decoding it: ASCII interleaved with the NUL bytes that UTF-16 and UTF-32 put
// around it.
//
// The interleaving is what is checked, not the mere presence of a NUL. A
// document that simply has a stray NUL in it -- which the suite contains -- is
// ill-formed UTF-8 JSON and nothing more, and calling it UTF-16 would excuse a
// parser from a case that is actually settled.
func looksWide(src []byte) bool {
	var at [2]int

	for i, b := range src {
		if b == 0 {
			at[i%2]++
		}
	}

	nuls := at[0] + at[1]

	// Every code unit of ASCII text in UTF-16 carries exactly one NUL, always
	// on the same side, so the NULs are numerous and all of one parity.
	return nuls >= 2 && (at[0] == 0 || at[1] == 0) && nuls*3 >= len(src)
}

// hasLoneSurrogate scans the \uXXXX escapes in order and reports whether any
// surrogate stands outside a well-formed high-then-low pair.
//
// The scan is over the whole document rather than over string bodies, which is
// sound because \u appears nowhere else in JSON: outside a string the sequence
// is not grammatical at all, and a document carrying it is refused for that
// reason before this tag matters.
func hasLoneSurrogate(src []byte) bool {
	s := string(src)

	for i := 0; i+1 < len(s); {
		if s[i] == '\\' && s[i+1] == '\\' {
			i += 2 // an escaped backslash, so the next character is literal

			continue
		}

		if s[i] != '\\' || s[i+1] != 'u' {
			i++

			continue
		}

		r, ok := hexEscape(s, i)
		if !ok {
			i += 2

			continue
		}

		switch {
		case utf16.IsSurrogate(r) && r >= 0xDC00:
			return true // a low surrogate reached before any high one
		case utf16.IsSurrogate(r):
			low, ok := hexEscape(s, i+6)
			if !ok || utf16.DecodeRune(r, low) == utf8.RuneError {
				return true
			}
			i += 12

			continue
		}

		i += 6
	}

	return false
}

// hexEscape decodes the \uXXXX at position i, if that is what is there.
func hexEscape(s string, i int) (rune, bool) {
	if i+6 > len(s) || s[i] != '\\' || s[i+1] != 'u' {
		return 0, false
	}

	n, err := strconv.ParseUint(s[i+2:i+6], 16, 16)
	if err != nil {
		return 0, false
	}

	return rune(n), true
}

// hasWideNumber reports whether any number outside a string is one float64
// cannot carry.
//
// Out of range means the magnitude overflows to infinity, underflows to zero
// from a non-zero literal, or is an integer past what an int64 or a uint64
// holds. Those are the three ways a parser that converts loses information a
// parser that keeps the text does not.
func hasWideNumber(src []byte) bool {
	for _, lit := range numberLiterals(string(src)) {
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return true // ParseFloat only refuses a grammatical number for range
		}

		if math.IsInf(f, 0) {
			return true
		}

		// Underflow is a zero result from a mantissa that was not zero. The
		// exponent's own digits have to be cut away first: "0e+1" is zero
		// because its mantissa is, which loses nothing at all.
		mantissa, _, _ := strings.Cut(strings.ToLower(lit), "e")
		if f == 0 && strings.Trim(mantissa, "-+0.") != "" {
			return true
		}

		if isIntegral(lit) {
			if _, err := strconv.ParseInt(lit, 10, 64); err != nil {
				if _, err := strconv.ParseUint(lit, 10, 64); err != nil {
					return true
				}
			}
		}
	}

	return false
}

func isIntegral(lit string) bool {
	return !strings.ContainsAny(lit, ".eE")
}

// numberLiterals returns the number tokens that fall outside string bodies.
//
// A token has to *begin* the way a JSON number begins, with a minus or a digit.
// Continuing on any of the number characters would be enough for a grammatical
// document were it not for "true" and "false", whose 'e' is a number character
// and would otherwise be read as a number of its own.
func numberLiterals(s string) []string {
	const (
		starts = "0123456789-"
		body   = "0123456789+-.eE"
	)

	var (
		out      []string
		inString bool
	)

	for i := 0; i < len(s); i++ {
		switch {
		case inString && s[i] == '\\':
			i++ // whatever follows is escaped, including a quote

			continue
		case s[i] == '"':
			inString = !inString

			continue
		case inString:
			continue
		}

		if strings.IndexByte(starts, s[i]) < 0 {
			continue
		}

		end := i
		for end < len(s) && strings.IndexByte(body, s[end]) >= 0 {
			end++
		}

		out = append(out, s[i:end])
		i = end - 1
	}

	return out
}

// depth reports the deepest nesting of brackets outside string bodies.
func depth(src []byte) int {
	var (
		at, deepest int
		inString    bool
	)

	s := string(src)
	for i := 0; i < len(s); i++ {
		switch {
		case inString && s[i] == '\\':
			i++

			continue
		case s[i] == '"':
			inString = !inString
		case inString:
			continue
		case s[i] == '[' || s[i] == '{':
			at++
			deepest = max(deepest, at)
		case s[i] == ']' || s[i] == '}':
			at--
		}
	}

	return deepest
}
