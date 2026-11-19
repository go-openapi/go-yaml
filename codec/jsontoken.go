// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"iter"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// JSONTokenKind says what a [JSONToken] carries.
//
// The nine kinds are what a JSON document is made of once the separators are
// left out. A reader that colors by kind, indexes by key or records a position
// needs no more, and nothing about YAML crosses this boundary: no tags, no
// anchors, no styles, no comments.
type JSONTokenKind uint8

const (
	// JSONInvalid is the zero value, and the kind of no token this package
	// hands over.
	JSONInvalid JSONTokenKind = iota
	// JSONObjectStart is the "{" of a mapping.
	JSONObjectStart
	// JSONObjectEnd is the "}" of a mapping.
	JSONObjectEnd
	// JSONArrayStart is the "[" of a sequence.
	JSONArrayStart
	// JSONArrayEnd is the "]" of a sequence.
	JSONArrayEnd
	// JSONKey is a mapping key, as the string JSON names a member with.
	JSONKey
	// JSONString is a string value, decoded.
	JSONString
	// JSONNumber is a number value, carrying the digits JSON spells it with.
	JSONNumber
	// JSONBool is true or false, read from [JSONToken.Bool].
	JSONBool
	// JSONNull is null.
	JSONNull
)

func (k JSONTokenKind) String() string {
	switch k {
	case JSONObjectStart:
		return "{"
	case JSONObjectEnd:
		return "}"
	case JSONArrayStart:
		return "["
	case JSONArrayEnd:
		return "]"
	case JSONKey:
		return "key"
	case JSONString:
		return "string"
	case JSONNumber:
		return "number"
	case JSONBool:
		return "bool"
	case JSONNull:
		return "null"
	default:
		return "invalid"
	}
}

// JSONToken is one piece of the JSON a YAML document holds the same values as.
//
// Value carries the text of a key, a string or a number, and is empty for
// everything else; a number keeps the digits rather than a parsed float, so a
// value JSON can carry whole is not rounded on the way through. Bool carries
// true or false and is read only when Kind is [JSONBool].
//
// A token's Value is safe to keep for as long as the source is: the scanner
// hands back either a window into the document or a string of its own, and
// never a view of a buffer it writes again. That is a stronger promise than the
// nodes of a walk carry, which are good only until Leave returns.
//
// No token stands for a "," or a ":". An object is a [JSONObjectStart], its
// members as a [JSONKey] and a value each, then a [JSONObjectEnd].
type JSONToken struct {
	// Value is the text of a key, a string or a number, and empty otherwise.
	Value string
	// At is where the token's first byte stands in the source. Offset is a
	// 0-based byte index, so src[At.Offset():] begins at the token, and Line
	// and Column count from 1 with Column counting characters.
	At token.Position
	// Kind says what the token is.
	Kind JSONTokenKind
	// Bool is the value of a [JSONBool].
	Bool bool
}

// JSONTokens converts a YAML document to JSON tokens, and answers for where the
// conversion stands while it reads.
//
// Build one with [ToJSONTokens] and range over [JSONTokens.Tokens]. [JSONTokens.Depth]
// and [JSONTokens.Path] describe the token the range is on, and
// [JSONTokens.Err] reports what stopped it.
type JSONTokens struct {
	src  []byte
	opts []parser.Option
	err  error

	// budget bounds how many tokens one document may make, and is 0 for no
	// bound. A later document of the stream is converted and thrown away, and
	// counts against it too. An alias writes what its anchor names again, so a document a
	// few hundred bytes long can name more values than there is memory for.
	budget int
	// oneDocument says a stream holding more than one document is refused
	// rather than converted down to its first.
	oneDocument bool

	// depth counts the objects and arrays open around the token in hand. A
	// closing token reports the depth it returns to.
	depth int
	// frames name the path to the token in hand, one per open collection.
	frames []jsonPathFrame
}

// jsonPathFrame is one collection on the way to the token in hand: the key of
// the member being read, or the index of the element.
//
// named says the frame has been given one. A collection's opening token stands
// at the collection's own pointer and not at a member of it, so an unnamed
// frame writes no segment: "{" at the root is "" and the "[" of "tags" is
// "/tags".
type jsonPathFrame struct {
	key   string
	index int
	array bool
	named bool
	// wantsKey says the next thing an object frame expects is a member name.
	// An object alternates key and value, and the parse can hand two values
	// over for one entry -- "&!" is an anchor the parse reads as two nodes --
	// so the alternation is checked rather than assumed.
	wantsKey bool
}

// ToJSONTokens reads src as the JSON tokens holding the same values, handing
// each over as the parse reaches it.
//
// It converts what [ToJSON] writes, token by token instead of into one buffer,
// and refuses the same documents: a value JSON has no spelling for -- an
// infinity, a NaN, a collection as a mapping key -- stops the conversion rather
// than being given one this package invented.
//
// A stream of several documents converts its first, which is the one
// [Unmarshal] reads. The rest are converted too and none of them is handed
// over, so a stream whose later documents cannot be converted is refused, as
// [ToJSON] refuses it.
//
// opts are passed to the parse. [github.com/go-openapi/go-yaml/parser.WithJSONCompatible]
// is always on and cannot be turned off.
//
// A document that repeats a mapping key is refused, as [Unmarshal] refuses it.
// Under [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey] the conversion keeps the first entry and
// drops the later one, where a decoder under [AllowDuplicateMapKey] keeps the last value.
// The first entry's tokens are handed over before the repeat is read, and cannot be taken back.
//
// Nothing is read until [JSONTokens.Tokens] is ranged over, and ranging twice
// converts twice.
func ToJSONTokens(src []byte, opts ...parser.Option) *JSONTokens {
	return &JSONTokens{src: src, opts: opts}
}

// Budget bounds how many tokens one document may hand over, and returns the
// receiver so that it reads in the range statement.
//
// An alias hands over everything its anchor names, again, each time it is
// named, so a document of a few hundred bytes can name more values than there
// is memory to hold. Neither the nesting depth nor the size of one scalar
// bounds that; this does. Set it for input you did not write.
//
// A budget of 0, which is the default, does not bound it.
func (s *JSONTokens) Budget(tokens int) *JSONTokens {
	s.budget = tokens

	return s
}

// OneDocument refuses a stream holding more than one document, and returns the
// receiver so that it reads in the range statement.
//
// Without it a stream converts its first document, which is the one [Unmarshal]
// reads, and the rest are read without being handed over. A caller building one
// JSON value has no room for the second and should say so here rather than
// converting a document the author did not put first.
//
// A "%YAML" or "%TAG" line does not count: it opens a document of its own ahead
// of the one it applies to.
func (s *JSONTokens) OneDocument() *JSONTokens {
	s.oneDocument = true

	return s
}

// Err reports what stopped the conversion, and nil where nothing did.
//
// A range over [JSONTokens.Tokens] ends silently on an error, as a lexer does,
// so read this after the loop.
func (s *JSONTokens) Err() error { return s.err }

// Depth counts the objects and arrays open around the token just handed over.
//
// A document's outermost value stands at 1, and what it holds at 2. A closing
// [JSONObjectEnd] or [JSONArrayEnd] reports the depth it returns to rather than
// the one it closes, so the opener and the closer of one collection report
// different numbers.
func (s *JSONTokens) Depth() int { return s.depth }

// Path is the RFC 6901 JSON pointer to the token just handed over.
//
// A key and the value it names share a pointer -- the key names the member the
// value fills -- and the document's outermost value is "". The pointer is built
// as the conversion reads, so this costs a walk of the open collections and no
// bookkeeping between calls.
func (s *JSONTokens) Path() string { return string(s.AppendPath(nil)) }

// AppendPath writes [JSONTokens.Path] to dst and returns it, for a caller that
// keeps a buffer rather than taking a string each time.
func (s *JSONTokens) AppendPath(dst []byte) []byte {
	for _, f := range s.frames {
		if !f.named {
			continue
		}
		dst = append(dst, '/')
		if f.array {
			dst = strconv.AppendInt(dst, int64(f.index), 10)

			continue
		}
		dst = appendPointerToken(dst, f.key)
	}

	return dst
}

// appendPointerToken writes one segment of a JSON pointer, escaping the two
// characters RFC 6901 §3 gives a meaning to.
func appendPointerToken(dst []byte, s string) []byte {
	if !strings.ContainsAny(s, "~/") {
		return append(dst, s...)
	}
	for i := range len(s) {
		switch s[i] {
		case '~':
			dst = append(dst, '~', '0')
		case '/':
			dst = append(dst, '~', '1')
		default:
			dst = append(dst, s[i])
		}
	}

	return dst
}

// Tokens hands over the tokens of the first document, in the order JSON writes
// them, and stops at the first one the conversion refuses.
//
// Breaking out of the range stops the parse. [JSONTokens.Depth] and
// [JSONTokens.Path] describe the token the body is running on, so read them
// there rather than after the loop.
func (s *JSONTokens) Tokens() iter.Seq[JSONToken] {
	return func(yield func(JSONToken) bool) {
		s.err = nil
		s.depth = 0
		s.frames = s.frames[:0]

		t := &jsonTokener{state: s, yield: yield, peek: -1}
		opts := append(append([]parser.Option{}, s.opts...),
			parser.WithOmitNodePaths(), parser.WithJSONCompatible())

		if _, err := parser.New(opts...).Walk(s.src, t); err != nil && s.err == nil {
			s.err = err
		}
		if s.err == nil && !t.stopped && t.handed == 0 {
			// The first document holds no node: an empty stream, or a document
			// written as nothing between its markers. Both read as a null, as
			// they do through [ToJSON]. It stands at the start of the source,
			// which is the only place a document that wrote nothing can point
			// at, and keeps every token of a stream carrying a position.
			//
			// The walk may stand in a later document, which emit hands nothing
			// over from, and this null belongs to the first.
			t.ended = false
			t.emit(JSONToken{Kind: JSONNull, At: token.Position{Line: 1, Column: 1}})
		}
	}
}
