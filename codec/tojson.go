// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/astutils"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

func toJSON(src []byte, opts ...parser.Option) ([]byte, error) {
	// On the workload corpus the JSON is between half the source and a little
	// under it: 0.51x on golang_source, 0.93x on twitter_status. One allocation
	// of the source's length therefore holds all of it. Growing from nothing
	// cost more than the text itself: appendJSONString made a quarter of the
	// conversion's allocations, almost all of them from doubling.
	out := make([]byte, 0, len(src))

	tokens := ToJSONTokens(src, opts...)
	comma := false
	for tok := range tokens.Tokens() {
		out, comma = appendJSONToken(out, tok, comma)
	}
	if err := tokens.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// appendJSONToken writes one token as JSON text, with the separator in front of
// it, and reports whether the next token takes a comma.
//
// No token stands for a "," or a ":", so appendJSONToken writes them back. It
// writes a comma in front of anything that follows a value or a closer, and a
// colon after a key.
func appendJSONToken(out []byte, tok JSONToken, comma bool) ([]byte, bool) {
	switch tok.Kind {
	case JSONObjectEnd:
		return append(out, '}'), true
	case JSONArrayEnd:
		return append(out, ']'), true
	default:
		// nothing to do
	}
	if comma {
		out = append(out, ',')
	}

	switch tok.Kind {
	case JSONObjectStart:
		return append(out, '{'), false
	case JSONArrayStart:
		return append(out, '['), false
	case JSONKey:
		return append(appendJSONString(out, tok.Value), ':'), false
	case JSONString:
		return appendJSONString(out, tok.Value), true
	case JSONNumber:
		return append(out, tok.Value...), true
	case JSONBool:
		return strconv.AppendBool(out, tok.Bool), true
	default:
		return append(out, "null"...), true
	}
}

// tagReader reads the node under a tag. err holds the error when the tag names a
// kind its node is not.
//
// tagReader is all that is left of a second reading of the document. ToJSON
// writes the tokens handed over by ToJSONTokens, so the two converters share
// this reading as code and everything else as tokens.
type tagReader struct {
	err error
}

// scalarToken returns the JSON token for one scalar node.
//
// It reads the node's own fields instead of the any returned by GetValue.
// Boxing a string costs one allocation per string, and a document is mostly
// strings: ast.StringNode.GetValue made 9% of a conversion's allocations.
func scalarToken(n ast.Node) JSONToken {
	switch t := n.(type) {
	case *ast.StringNode:
		return JSONToken{Kind: JSONString, Value: t.Value}
	case *ast.NullNode:
		return JSONToken{Kind: JSONNull}
	case *ast.BoolNode:
		return JSONToken{Kind: JSONBool, Bool: t.Value}
	case *ast.InfinityNode, *ast.NanNode:
		return JSONToken{Kind: JSONNull}
	case *ast.FloatNode:
		if tk := t.GetToken(); tk != nil {
			// Keep the digits the document wrote, where JSON spells the number
			// the same way. Reading them into a float64 and writing them back
			// rounds them to float64 precision: "0.1234567890123456789012345"
			// came out as 0.12345678901234568, seventeen digits of twenty-five,
			// for a value JSON can carry whole. The token takes the source's own
			// string, so nothing is parsed and nothing is copied.
			if isJSONNumber(tk.Value) {
				return numberToken(tk.Value)
			}
			if text, ok := decimalJSON(tk.Value, tk.Type); ok {
				return numberToken(text)
			}
			if f, ok := token.ParseFloat(tk.Value, tk.Type); ok {
				return float64Token(f)
			}
		}

		return floatToken(astutils.JSONScalarOf(t))
	case *ast.IntegerNode:
		if tk := t.GetToken(); tk != nil {
			if isJSONNumber(tk.Value) {
				// Keep the digits the document wrote, where JSON spells the
				// integer the same way. "-0" is a JSON number, and reading it
				// through strconv writes it back as "0", a different literal.
				return numberToken(tk.Value)
			}
			if u, negative, ok := token.ParseWholeNumber(tk.Value, tk.Type); ok {
				digits := strconv.FormatUint(u, 10)
				if negative && u != 0 {
					digits = "-" + digits
				}

				return numberToken(digits)
			}
		}

		return valueToken(astutils.JSONScalarOf(t))
	case *ast.LiteralNode:
		if t.Value == nil {
			return JSONToken{Kind: JSONNull}
		}

		return JSONToken{Kind: JSONString, Value: t.Value.Value}
	default:
		return valueToken(astutils.JSONScalarOf(n))
	}
}

// numberToken returns a number token for digits already spelled as JSON spells them.
func numberToken(digits string) JSONToken {
	return JSONToken{Kind: JSONNumber, Value: digits}
}

// name returns the string that names a mapping entry when tok is its key. JSON
// names a member with a string, so a number is named by its digits and a bool
// by its word.
func (tok JSONToken) name() string {
	switch tok.Kind {
	case JSONKey, JSONString, JSONNumber:
		return tok.Value
	case JSONBool:
		return strconv.FormatBool(tok.Bool)
	default:
		return "null"
	}
}

// taggedValue returns the JSON token for a tagged scalar, and reports whether the
// tag names a scalar type at all.
//
// [ast.TagNode.Resolve] makes the decision, not taggedValue. Before Resolve, the
// converter and the decoder each read the tag on their own and gave different
// answers: one rejected "!!timestamp not-a-date" and the other wrote it as
// "not-a-date". taggedValue now only converts. The converter writes the date
// as the document wrote it, where the decoder builds a time.Time, and both call
// Resolve first.
//
// A "%TAG" line can give the handle a prefix of the document's own. "!!int"
// under such a line names the document's type, not YAML's. Resolve reads the
// URI and returns it unresolved, and the value is written as it stands.
//
// key marks a tag that stands as a mapping key. A key is a string named after
// the value, as a bare key is: "!!float 1e3" names its entry 1000.0, as "1e3"
// does. A float key is therefore read into a value first, while a float value
// is written from its digits.
func (w *tagReader) taggedValue(t *ast.TagNode, key bool) (JSONToken, bool) {
	res := t.Resolve()

	switch res.Verdict {
	case ast.TagUnresolved:
		// A local tag, a foreign one, or a name YAML's repository does not
		// define. The node is written by its kind.
		return JSONToken{}, false
	case ast.TagKindMismatch:
		w.fail(yamlerrors.NewSyntax(
			fmt.Sprintf("%s does not support this kind of node", res.Tag), t.GetToken()))

		return JSONToken{}, false
	case ast.TagValueMismatch:
		if res.Lax {
			// parser.WithLaxTags: the scalar's text replaces the value the tag
			// could not read from it.
			return JSONToken{Kind: JSONString, Value: res.Text}, true
		}

		w.fail(yamlerrors.NewSyntax(
			fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), t.Value.GetToken()))

		return JSONToken{}, false
	default:
		// nothing to do
	}

	if res.Empty {
		// The tag has no value under it and takes its type's default. The
		// decoder gives the same value for the same document.
		return tagZeroToken(res.Tag), true
	}

	switch res.Tag {
	case token.StringTag:
		return JSONToken{Kind: JSONString, Value: res.Text}, true
	case token.IntegerTag:
		return valueToken(taggedInteger(res.Text, res.Schema)), true
	case token.FloatTag:
		if base, ok := token.FloatBase(res.Text, res.Schema); ok && !key {
			if text, ok := decimalJSON(res.Text, base); ok {
				return numberToken(text), true
			}
		}

		return floatToken(taggedFloat(res.Text, res.Schema)), true
	case token.BooleanTag:
		b, _ := token.ParseBool(strings.ToLower(res.Text))

		return JSONToken{Kind: JSONBool, Bool: b}, true
	case token.NullTag:
		return JSONToken{Kind: JSONNull}, true
	case token.BinaryTag:
		return binaryToken(res.Text), true
	case token.TimestampTag:
		// JSON has no date, so a timestamp is written as a string. The string
		// holds the instant, not the text that spelled it.
		//
		// yaml.org/type/timestamp.html admits a "t" for the "T", a space for
		// it, a date with no time at all and a zone as short as "-5". RFC 3339
		// spells none of these, so writing the text converted one instant
		// written two ways into two strings. The value converter writes the
		// time.Time built by the decoder, in RFC 3339, and tagZeroToken already
		// writes RFC 3339 for a "!!timestamp" with no value. Writing the text
		// was therefore the one inconsistency inside this library.
		//
		// Resolve has read the timestamp already, so this cannot fail.
		stamp, _ := token.ParseTimestamp(res.Text)

		return JSONToken{Kind: JSONString, Value: stamp.Format(time.RFC3339Nano)}, true
	default:
		// A tag naming a kind: !!seq, !!map, !!set, !!omap, !!merge. The node
		// writes itself.
		return JSONToken{}, false
	}
}

// tagZeroToken returns the JSON token for a tag with no value under it. It gives
// the zero value of the tag's type, written as the decoder writes its zero.
func tagZeroToken(tag token.ReservedTagKeyword) JSONToken {
	switch tag {
	case token.IntegerTag:
		return numberToken("0")
	case token.FloatTag:
		return numberToken("0.0")
	case token.BooleanTag:
		return JSONToken{Kind: JSONBool}
	case token.StringTag:
		return JSONToken{Kind: JSONString}
	case token.BinaryTag:
		// "!!binary" with nothing after it gives the empty byte string. Its
		// base64 spelling has no characters at all.
		return JSONToken{Kind: JSONString}
	case token.TimestampTag:
		// The zero time, as encoding/json writes a time.Time.
		return JSONToken{Kind: JSONString, Value: time.Time{}.Format(time.RFC3339Nano)}
	default:
		return JSONToken{Kind: JSONNull}
	}
}

func (w *tagReader) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// taggedInteger reads the whole number under a "!!int". A text that is not a
// number at all reads as zero, and one written as a float keeps its whole part:
// "!!int 3.7" gives 3.
func taggedInteger(text string, schema token.Schema) any {
	base, ok := token.IntegerBase(text, schema)
	if !ok {
		// ast.Resolve reported a mismatch, and the conversion returned an
		// error before this. No text with unreadable digits reaches here.
		return int64(0)
	}
	if n, parsed := token.ParseInteger(text, base); parsed {
		return n
	}
	if b, big := token.ParseBigInteger(text, base); big {
		// A number wider than a machine word keeps its digits, as the same
		// number does untagged. strconv.AppendInt on an int64 wrote
		// "!!int 123456789012345678901" out as math.MinInt64.
		return b
	}

	return int64(0)
}

// taggedFloat reads the number under a "!!float", keeping the width the number
// needs.
//
// strconv.ParseFloat returns ErrRange for a number outside float64, with an
// infinity or a zero. Writing that value gave "0.0" for "!!float 1e+310" and for
// "!!float 1e-400" alike. token.ParseBigFloat reads both, and the untagged
// spellings already convert through it.
func taggedFloat(text string, schema token.Schema) any {
	base, ok := token.FloatBase(text, schema)
	if !ok {
		return float64(0)
	}
	switch base {
	case token.NanType:
		return math.NaN()
	case token.InfinityType:
		if strings.HasPrefix(text, "-") {
			return math.Inf(-1)
		}

		return math.Inf(0)
	default:
		// nothing to do
	}
	if base.IsInteger() {
		// A whole number under "!!float". The digits are read in the base set
		// by the schema, so "!!float 017" gives 15 under 1.1, and then widened.
		// Sniffing the text again under 1.2 wrote 0 for every base but decimal.
		return integerAsFloat(text, base)
	}
	if f, parsed := token.ParseFloat(text, base); parsed {
		return f
	}
	if f, past := token.FloatPastRange(text, base); past {
		return f
	}
	if b, big := token.ParseBigFloat(text, base); big {
		return b
	}

	return float64(0)
}

// integerAsFloat widens a whole number written in any base to a real number. It
// returns a *big.Float for a number that no float64 holds.
func integerAsFloat(text string, base token.Type) any {
	if u, negative, ok := token.ParseWholeNumber(text, base); ok {
		f := float64(u)
		if negative {
			return -f
		}

		return f
	}
	if b, ok := token.ParseBigInteger(text, base); ok {
		return new(big.Float).SetInt(b)
	}

	return float64(0)
}

// valueToken returns the JSON token for one Go value.
//
// A number keeps its digits whatever its width. JSON bounds neither integers nor
// floats: RFC 8259 §6 leaves the range to the reader. A value too wide for a
// machine word arrives as a *big.Int or a *big.Float, and valueToken writes it
// with its digits intact. The reader decides what to do with it. encoding/json
// rejects a number past 1e308 into a float64 and rounds one below 1e-324 to
// zero. A reader that needs either uses json.Number.
//
// Infinity and NaN are the exception, and not because of width: JSON has no
// spelling for them at all. They become null, where encoding/json returns an
// error instead of writing them.
//
// v holds one of the closed set of types handed over by a scalar node or a tag:
// those returned by ast.ScalarNode.GetValue, and by taggedInteger and
// taggedFloat. Any other type gives null.
func valueToken(v any) JSONToken {
	switch t := v.(type) {
	case nil:
		return JSONToken{Kind: JSONNull}
	case string:
		return JSONToken{Kind: JSONString, Value: t}
	case bool:
		return JSONToken{Kind: JSONBool, Bool: t}
	case int:
		return numberToken(strconv.FormatInt(int64(t), 10))
	case int64:
		return numberToken(strconv.FormatInt(t, 10))
	case uint64:
		return numberToken(strconv.FormatUint(t, 10))
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return JSONToken{Kind: JSONNull}
		}

		return numberToken(strconv.FormatFloat(t, 'g', -1, 64))
	case *big.Int:
		return numberToken(t.String())
	case *big.Float:
		if t.IsInf() {
			return JSONToken{Kind: JSONNull}
		}

		return numberToken(t.Text('g', -1))
	default:
		return JSONToken{Kind: JSONNull}
	}
}

// floatToken calls valueToken for a value that YAML read as a float.
//
// JSON has one number type, so 1.0 and 1 are the same value. But a document that
// wrote a float should still hold one when converted back to YAML, and a bare
// "1" reads as an integer. floatToken keeps the fractional part for that.
func floatToken(v any) JSONToken {
	tok := valueToken(v)
	if tok.Kind == JSONNumber {
		tok.Value = withFraction(tok.Value)
	}

	return tok
}

// float64Token does the work of floatToken for a value already read as a
// float64. Every float of a document that fits in a float64 takes this path.
func float64Token(f float64) JSONToken {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return JSONToken{Kind: JSONNull}
	}

	return numberToken(withFraction(strconv.FormatFloat(f, 'g', -1, 64)))
}

// isJSONNumber reports whether text is a number JSON spells the same way.
//
// YAML writes several floats JSON does not: ".5" and "5." leave a side of the
// point empty, "+1.0" carries a sign JSON has no place for, "007.5" leads with
// a zero, ".inf" and ".nan" are words, and YAML 1.1 puts "_" between digits.
// Each of those is converted instead of copied.
func isJSONNumber(text string) bool {
	i := 0
	if i < len(text) && text[i] == '-' {
		i++
	}

	// An integer part of one digit, or several not opening with a zero.
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start || (i-start > 1 && text[start] == '0') {
		return false
	}

	if i < len(text) && text[i] == '.' {
		i++
		if i = digitsFrom(text, i); i < 0 {
			return false
		}
	}

	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		if i = digitsFrom(text, i); i < 0 {
			return false
		}
	}

	return i == len(text)
}

// decimalJSON rewrites the text of a decimal float, or of a decimal integer
// under "!!float", as JSON spells a number. It reports false for any other text.
//
// JSON's number grammar is narrower than YAML's: no "+", no leading zero, a
// digit on each side of a point, no "_". Rewriting the text keeps every digit
// the document wrote. When the number was read into a Go value first, it was
// rounded to the value's precision: "+0.12345678901234567890123" came out as
// 0.12345678901234568. It was also respelled, ".5e10" as 5e+09. A number past
// 1e±1000, which the decoder reads as an infinity or zero, was written as null
// or 0.0. YAML 1.1's base-60 floats and its octal, hex and binary integers still
// go through their value.
func decimalJSON(text string, typ token.Type) (string, bool) {
	if typ != token.FloatType && typ != token.IntegerType {
		return "", false
	}
	spelled, ok := jsonNumberText(text)
	if !ok {
		return "", false
	}

	return withFraction(spelled), true
}

// jsonNumberText rewrites a YAML decimal float as JSON spells it: no "+", no
// "_", one digit at least and no leading zero before the point, and no point
// with nothing after it. It reports false when the result is not a JSON
// number.
func jsonNumberText(text string) (string, bool) {
	s := strings.ReplaceAll(text, "_", "")

	var b strings.Builder
	b.Grow(len(s) + 1)
	if s != "" && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			b.WriteByte('-')
		}
		s = s[1:]
	}

	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	whole := strings.TrimLeft(s[:i], "0")
	if whole == "" {
		whole = "0"
	}
	b.WriteString(whole)
	s = s[i:]

	if s != "" && s[0] == '.' {
		j := 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > 1 {
			b.WriteString(s[:j])
		}
		s = s[j:]
	}
	b.WriteString(s)

	out := b.String()

	return out, isJSONNumber(out)
}

// digitsFrom reads one or more digits from text[i:] and returns the index where
// they end, or -1 when there are none.
func digitsFrom(text string, i int) int {
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start {
		return -1
	}

	return i
}

// withFraction appends ".0" to digits that hold no point and no exponent, so they
// read as a float.
func withFraction(digits string) string {
	for i := range len(digits) {
		switch digits[i] {
		case '.', 'e', 'E':
			return digits
		}
	}

	return digits + ".0"
}

// binaryToken returns a "!!binary" scalar as the document's base64 string, in
// canonical form: the same characters, without the line breaks allowed by
// RFC 2045.
//
// JSON has no binary type and no way to spell arbitrary bytes, because a JSON
// string is UTF-8. The token therefore carries the encoded text. encoding/json
// writes the same for a []byte, and JSON APIs commonly use it for binary data. The bytes
// are not decoded and re-encoded. The document already holds the canonical
// spelling apart from the breaks, and ast.TagNode.Resolve has already read it as
// base64, so writing the text back cannot invent a spelling.
//
// binaryToken used to write a sequence of the byte values: "!!binary aGVsbG8="
// gave [104,101,108,108,111]. No other JSON writer produces that.
func binaryToken(text string) JSONToken {
	canonical := withoutBase64Breaks(text)
	if _, err := base64.StdEncoding.DecodeString(canonical); err != nil {
		return JSONToken{Kind: JSONString, Value: text}
	}

	return JSONToken{Kind: JSONString, Value: canonical}
}

// withoutBase64Breaks returns text without the line breaks and spacing that
// RFC 2045 permits inside an encoded stream. The result is base64's canonical
// form.
func withoutBase64Breaks(text string) string {
	if !strings.ContainsAny(text, " \t\r\n") {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))
	for i := range len(text) {
		switch c := text[i]; c {
		case ' ', '\t', '\r', '\n':
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

// appendJSONString writes s as a JSON string. A string that needs an escape goes
// through encoding/json, so the escape rules are the standard library's.
func appendJSONString(out []byte, s string) []byte {
	if plainJSONString(s) {
		out = append(out, '"')
		out = append(out, s...)

		return append(out, '"')
	}

	text, err := json.Marshal(s)
	if err != nil {
		return append(out, `""`...)
	}

	return append(out, text...)
}

// plainJSONString reports whether s may be written between quotes as it stands.
func plainJSONString(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
			return false
		}
	}

	return true
}
