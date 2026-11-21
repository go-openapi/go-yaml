// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// jsonState is what a JSON text accepts next, in the grammar of RFC 8259.
type jsonState uint8

const (
	// jsonValue expects a value: the text's own, a member's after its ':', or an element after a ','.
	jsonValue jsonState = iota
	// jsonValueOrClose expects the first element of an array, or the ']' of an empty one.
	jsonValueOrClose
	// jsonKeyOrClose expects the first member name of an object, or the '}' of an empty one.
	jsonKeyOrClose
	// jsonKey expects a member name after a ','.
	jsonKey
	// jsonColon expects the ':' after a member name.
	jsonColon
	// jsonAfterValue expects a ',' or the end of the array or object the value stands in.
	jsonAfterValue
	// jsonDone has read the text's value, and expects nothing more.
	jsonDone
)

// jsonSyntax checks the scanner's tokens against the grammar of JSON as they are cut, under [WithJSONSyntax].
//
// JSON's grammar needs one token to decide each step, so the check keeps a state and the brackets open around it,
// and nothing else. A YAML scanner cuts every JSON text into tokens that map one to one onto JSON's own:
// '{', '}', '[', ']', ',', ':', a double-quoted string, and a plain scalar for a number, true, false or null.
// Any other token is YAML that JSON does not have.
type jsonSyntax struct {
	src   string
	open  []token.Type
	state jsonState
	// last is the last token read, for the error at the end of the text, and read says there is one.
	last token.Token
	read bool
}

func (j *jsonSyntax) reset(src string) {
	*j = jsonSyntax{src: src, open: j.open[:0]}
}

// next checks tk, the next token of the text.
func (j *jsonSyntax) next(tk token.Token) error {
	j.last, j.read = tk, true

	switch tk.Type {
	case token.MappingStartType, token.SequenceStartType:
		if !j.expectsValue() {
			return j.unexpected(tk)
		}
		j.open = append(j.open, tk.Type)
		j.state = jsonValueOrClose
		if tk.Type == token.MappingStartType {
			j.state = jsonKeyOrClose
		}

		return nil

	case token.MappingEndType, token.SequenceEndType:
		return j.close(tk)

	case token.InvalidType:
		// The scanner failed here and reports why, which names the fault better than JSON's grammar can.
		return nil

	case token.CollectEntryType:
		if j.state != jsonAfterValue {
			return j.unexpected(tk)
		}
		j.state = jsonValue
		if j.inside() == token.MappingStartType {
			j.state = jsonKey
		}

		return nil

	case token.MappingValueType:
		if j.state != jsonColon {
			return j.unexpected(tk)
		}
		j.state = jsonValue

		return nil

	case token.DoubleQuoteType:
		if err := j.checkString(tk); err != nil {
			return err
		}
		if j.state == jsonKeyOrClose || j.state == jsonKey {
			j.state = jsonColon

			return nil
		}
		if !j.expectsValue() {
			return j.unexpected(tk)
		}
		j.valueDone()

		return nil

	case token.NullType, token.BoolType, token.IntegerType, token.FloatType, token.StringType,
		token.BinaryIntegerType, token.OctetIntegerType, token.HexIntegerType, token.InfinityType, token.NanType:
		if !j.expectsValue() {
			return j.unexpected(tk)
		}
		if !isJSONLiteral(tk.Value) {
			return notJSON(fmt.Sprintf("%q is not a JSON value", tk.Value), tk)
		}
		j.valueDone()

		return nil

	default:
		return j.unexpected(tk)
	}
}

// end checks that the text holds one whole value, once the scanner has no more tokens.
func (j *jsonSyntax) end() error {
	if j.state == jsonDone {
		return nil
	}
	if !j.read {
		return yamlerrors.NewNotJSONSyntax("a JSON text holds a value, and this one is empty", token.Invalid("", token.At(1, 1, 0, 0)))
	}

	return notJSON("the JSON text ends where it expects "+j.expected(), j.last)
}

func (j *jsonSyntax) expectsValue() bool {
	return j.state == jsonValue || j.state == jsonValueOrClose
}

// inside returns the bracket the next token stands in.
func (j *jsonSyntax) inside() token.Type {
	if len(j.open) == 0 {
		return token.UnknownType
	}

	return j.open[len(j.open)-1]
}

func (j *jsonSyntax) valueDone() {
	j.state = jsonAfterValue
	if len(j.open) == 0 {
		j.state = jsonDone
	}
}

func (j *jsonSyntax) close(tk token.Token) error {
	want := token.MappingStartType
	if tk.Type == token.SequenceEndType {
		want = token.SequenceStartType
	}
	if j.inside() != want {
		return j.unexpected(tk)
	}

	// The bracket closes after a value, or right after it opened. Anywhere else it follows a ',' or a member name
	// with no value. A bracket of the wrong kind, as in "{]", failed the check above.
	switch j.state {
	case jsonAfterValue, jsonKeyOrClose, jsonValueOrClose:
	default:
		return j.unexpected(tk)
	}
	j.open = j.open[:len(j.open)-1]
	j.valueDone()

	return nil
}

func (j *jsonSyntax) unexpected(tk token.Token) error {
	return notJSON(fmt.Sprintf("found %s where JSON expects %s", describeJSONToken(tk), j.expected()), tk)
}

// notJSON returns the error for tk. It takes the token by value and copies it here, on the error path, so that the
// check of a token that passes allocates nothing.
func notJSON(msg string, tk token.Token) error {
	at := tk

	return yamlerrors.NewNotJSONSyntax(msg, &at)
}

// expected names what the state accepts, for an error.
func (j *jsonSyntax) expected() string {
	switch j.state {
	case jsonValue:
		return "a value"
	case jsonValueOrClose:
		return "a value or ']'"
	case jsonKeyOrClose:
		return "a member name or '}'"
	case jsonKey:
		return "a member name"
	case jsonColon:
		return "':'"
	case jsonAfterValue:
		if j.inside() == token.MappingStartType {
			return "',' or '}'"
		}

		return "',' or ']'"
	default:
		return "the end of the text"
	}
}

// describeJSONToken names tk for an error, by the YAML it is when JSON has nothing like it.
func describeJSONToken(tk token.Token) string {
	switch tk.Type {
	case token.CommentType:
		return "a comment"
	case token.AnchorType:
		return "an anchor"
	case token.AliasType:
		return "an alias"
	case token.TagType:
		return "a tag"
	case token.DirectiveType:
		return "a directive"
	case token.DocumentHeaderType:
		return `"---"`
	case token.DocumentEndType:
		return `"..."`
	case token.SingleQuoteType:
		return "a single-quoted string"
	case token.LiteralType, token.FoldedType:
		return "a block scalar"
	case token.SequenceEntryType:
		return `a block sequence entry "-"`
	case token.MappingKeyType:
		return `an explicit key "?"`
	case token.DoubleQuoteType:
		return "a string"
	default:
		return fmt.Sprintf("%q", tk.Value)
	}
}

// checkString checks the text of a double-quoted string as JSON writes one:
// no raw control character, line break or tab, and only the escapes of RFC 8259 section 7.
func (j *jsonSyntax) checkString(tk token.Token) error {
	start, end := int(tk.Position.Offset()), int(tk.EndOffset())
	if start < 0 || end > len(j.src) || end-start < 2 {
		// A string token from the source spans both its quotes. This guards the slice below if one ever does not.
		return nil
	}
	text := j.src[start+1 : end-1]

	for i := 0; i < len(text); i++ {
		c := text[i]
		if c < ' ' {
			return notJSON(fmt.Sprintf("a JSON string cannot hold %q raw", rune(c)), tk)
		}
		if c != '\\' {
			continue
		}
		i++
		if i >= len(text) {
			break
		}
		switch text[i] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			if !isHex4(text[i+1:]) {
				return notJSON(`a JSON "\u" escape takes four hexadecimal digits`, tk)
			}
			i += 4
		default:
			return notJSON(fmt.Sprintf(`JSON has no "\%c" escape`, text[i]), tk)
		}
	}

	return nil
}

func isHex4(s string) bool {
	if len(s) < 4 {
		return false
	}
	for _, c := range []byte(s[:4]) {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}

	return true
}

// isJSONLiteral reports whether s is true, false, null or a number as RFC 8259 section 6 writes one:
// an optional '-', an integer with no leading zero, then an optional fraction and an optional exponent.
func isJSONLiteral(s string) bool {
	switch s {
	case "true", "false", "null":
		return true
	default:
	}

	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case i < len(s) && s[i] >= '1' && s[i] <= '9':
		i = digitsFrom(s, i)
	default:
		return false
	}
	if i < len(s) && s[i] == '.' {
		j := digitsFrom(s, i+1)
		if j == i+1 {
			return false
		}
		i = j
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		j := digitsFrom(s, i)
		if j == i {
			return false
		}
		i = j
	}

	return i == len(s)
}

func digitsFrom(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	return i
}
