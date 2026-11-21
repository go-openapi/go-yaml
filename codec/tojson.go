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

	"github.com/go-openapi/go-yaml/codec/internal/astutils"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// ToJSON converts a YAML document to the JSON that holds the same values.
//
// It writes the tokens [ToJSONTokens] hands over into one buffer, putting back
// the commas and colons the tokens leave out. The two converters are one reading
// of a document -- the same aliases, merges, tags and refusals -- and differ only
// in what they hand the caller.
//
// A stream of several documents converts its first, which is the one
// [Unmarshal] reads. The rest are still read, so a stream whose later documents
// cannot be converted is refused rather than half-answered.
//
// opts are passed to the parse. The one that changes what is written is
// [github.com/go-openapi/go-yaml/parser.WithLaxTags], which reads a tag naming
// a type its scalar is not as the text rather than refusing it, so
// "k: !!int abc" converts to {"k":"abc"} instead of failing. Pass it here and
// to whatever else reads the same document, or the two disagree about it.
//
// [github.com/go-openapi/go-yaml/parser.WithJSONCompatible] is always on and
// cannot be turned off: a document JSON has no spelling for is refused rather
// than given one this converter invented.
//
// Under [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey] a
// repeated key keeps its first value, as in [ToJSONTokens], where a decoder
// keeps the last.
func ToJSON(src []byte, opts ...parser.Option) ([]byte, error) {
	// The JSON runs from half the source to a little under it on the workload
	// corpus -- 0.51x on golang_source, 0.93x on twitter_status -- so the
	// source's length is one allocation that holds all of it. Growing from
	// nothing cost more than the text itself: appendJSONString was a quarter of
	// what the conversion allocated, almost all of it doubling.
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
// No token stands for a "," or a ":", so they are put back here: a comma in
// front of anything that follows a value or a closer, a colon after a key.
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

// tagReader reads what a tag stands over, and holds the refusal where it names
// a kind its node is not.
//
// Only this much of a second reading of the document is left: ToJSON writes the
// tokens ToJSONTokens hands over, so the two converters share this reading as
// code and everything else as tokens.
type tagReader struct {
	err error
}

// scalarToken is the JSON one scalar node is worth, as the token that carries
// it.
//
// It reads the node's own fields rather than the any GetValue boxes them into.
// Boxing a string costs an allocation apiece, and a document is mostly strings:
// ast.StringNode.GetValue was 9% of everything a conversion allocated.
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
			// The digits the document wrote, where JSON spells a number the
			// same way. Reading them into a float64 and writing them back
			// rounds to what one holds: "0.1234567890123456789012345" came out
			// as 0.12345678901234568, seventeen digits of twenty-five, on a
			// value JSON can carry whole. The token takes the source's own
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
				// The digits the document wrote, where JSON spells the integer
				// the same way. "-0" is a JSON number and reading it through
				// strconv writes it back as "0", which is a different literal.
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

// numberToken is a number carrying digits already spelled as JSON spells them.
func numberToken(digits string) JSONToken {
	return JSONToken{Kind: JSONNumber, Value: digits}
}

// name is the string a mapping holds an entry under where this token stands as
// its key. JSON names a member with a string, so a number is named by the
// digits it is written with and a bool by its word.
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

// taggedValue is the JSON a tagged scalar is written as, and whether the tag
// names a scalar type at all.
//
// The decision is [ast.TagNode.Resolve]'s and not this function's. Before it,
// the converter and the decoder each read the tag for themselves and answered
// differently: "!!timestamp not-a-date" was refused by one and written as
// "not-a-date" by the other. Here the conversion is all that is left -- the
// converter writes the date as the document wrote it where the decoder wants a
// time.Time, and both ask the same question first.
//
// A "%TAG" line gives the handle a prefix of the document's own, so "!!int"
// under one names the document's type and not YAML's; Resolve reads the URI and
// reports it unresolved, and the value stands as it is written.
//
// key says the tag stands as a mapping key. A key is a string named after the
// value, as a bare key is -- "!!float 1e3" names its entry 1000.0, as "1e3"
// does -- so a float there is read into a value first, where a float standing
// as a value is written from its digits.
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
			// parser.WithLaxTags: the characters the scalar was written with
			// stand in for the value the tag could not make of them.
			return JSONToken{Kind: JSONString, Value: res.Text}, true
		}

		w.fail(yamlerrors.NewSyntax(
			fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), t.Value.GetToken()))

		return JSONToken{}, false
	default:
		// nothing to do
	}

	if res.Empty {
		// The tag stands on no value and takes its own default, which is what
		// the decoder gives for the same document.
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
		// JSON has no date, so a timestamp is written as a string -- and as the
		// instant it names rather than as the text that spelled it.
		//
		// yaml.org/type/timestamp.html admits a "t" for the "T", a space for
		// it, a date with no time at all and a zone as short as "-5", none of
		// which RFC 3339 spells, so one instant written two ways converted two
		// ways. The value converter writes the time.Time the decoder builds,
		// which is RFC 3339, and tagZeroToken already writes that for a
		// "!!timestamp" standing on no value -- so the text was the odd one
		// out inside this library before it was a question about the field.
		//
		// Resolve has read it already, so this cannot fail.
		stamp, _ := ast.ParseTimestamp(res.Text)

		return JSONToken{Kind: JSONString, Value: stamp.Format(time.RFC3339Nano)}, true
	default:
		// A tag naming a kind -- !!seq, !!map, !!set, !!omap, !!merge. The node
		// writes itself.
		return JSONToken{}, false
	}
}

// tagZeroToken is the JSON for a tag standing on no value: the value its type
// starts at, written as the decoder's own zero would be.
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
		// "!!binary" with nothing after it is the empty byte string, which
		// base64 spells as no characters at all.
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

// taggedInteger reads the whole number a "!!int" stands on. A text that is not
// a number at all counts as zero, and one written as a float keeps its whole
// part: "!!int 3.7" is 3.
func taggedInteger(text string, schema token.Schema) any {
	base, ok := token.IntegerBase(text, schema)
	if !ok {
		// ast.Resolve reported a mismatch and the document was refused before
		// this, so nothing reaches here with digits it cannot read.
		return int64(0)
	}
	if n, parsed := token.ParseInteger(text, base); parsed {
		return n
	}
	if b, big := token.ParseBigInteger(text, base); big {
		// A number wider than a machine word keeps its digits, as the same
		// number untagged already does. strconv.AppendInt on an int64 wrote
		// "!!int 123456789012345678901" out as math.MinInt64.
		return b
	}

	return int64(0)
}

// taggedFloat reads what "!!float" was written over, keeping the width the
// number needs.
//
// strconv.ParseFloat reports ErrRange for a number outside float64 and hands
// back an infinity or a zero, and writing that gave "0.0" for "!!float 1e+310"
// and for "!!float 1e-400" alike. token.ParseBigFloat reads both, which is what
// the untagged spellings already convert through.
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
		// A whole number under "!!float". The digits are read in the base the
		// schema gave them -- "!!float 017" is 15 under 1.1 -- and widened,
		// where sniffing the text again under 1.2 wrote 0 for every base but
		// decimal.
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

// integerAsFloat widens a whole number written in any base to the real number
// it names, keeping a *big.Float for one no float64 holds.
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

// valueToken is the JSON one Go value is worth.
//
// A number keeps its digits whatever width it takes. JSON bounds neither
// integers nor floats -- RFC 8259 §6 leaves the range to the reader -- so a
// value too wide for a machine word arrives as a *big.Int or a *big.Float and
// goes out with its digits intact. What a reader makes of it is the reader's:
// encoding/json refuses a number past 1e308 into a float64 and rounds one below
// 1e-324 to zero, and a reader that wants either uses json.Number.
//
// Infinity and NaN are the exception, and not because of width: JSON has no
// spelling for them at all. They become null, which is what encoding/json
// refuses to write.
//
// The types are the closed set a scalar node and a tag hand over -- what
// ast.ScalarNode.GetValue returns, and what taggedInteger and taggedFloat read.
// Anything else is null.
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

// floatToken is valueToken for a value YAML read as a float.
//
// JSON has one number type, so 1.0 and 1 are the same value -- but a document
// that wrote a float and converts back to YAML should still hold one, and a
// bare "1" reads as an integer. The fractional part is kept for that.
func floatToken(v any) JSONToken {
	tok := valueToken(v)
	if tok.Kind == JSONNumber {
		tok.Value = withFraction(tok.Value)
	}

	return tok
}

// float64Token is floatToken for a value already read as a float64, which is
// every float a document writes that one can hold.
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
// Each of those is converted rather than copied.
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

// decimalJSON writes a decimal float -- or a decimal integer standing under
// "!!float" -- from its text, spelled as JSON spells a number, and reports false
// for any other text.
//
// JSON's number grammar is narrower than YAML's: no "+", no leading zero, a
// digit on each side of a point, no "_". Rewriting the text keeps every digit
// the document wrote. Read into a Go value first, a number was rounded to what
// one holds -- "+0.12345678901234567890123" came out as 0.12345678901234568 --
// and respelled -- ".5e10" as 5e+09 -- and one past 1e±1000, which the decoder
// reads as an infinity or zero, was written as null or 0.0. YAML 1.1's base-60
// floats and its octal, hex and binary integers still go through their value.
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
// with nothing after it. It reports false where the result is not a JSON
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

// digitsFrom reads one or more digits and returns where they end, or -1 where
// there are none.
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

// withFraction keeps digits looking like a float.
func withFraction(digits string) string {
	for i := range len(digits) {
		switch digits[i] {
		case '.', 'e', 'E':
			return digits
		}
	}

	return digits + ".0"
}

// binaryToken is a "!!binary" scalar as the base64 string the document carries,
// in canonical form: the same characters with the line breaks RFC 2045 allows
// taken out.
//
// JSON has no binary type and no way to spell arbitrary bytes -- a JSON string
// is UTF-8 -- so the encoded text is what travels, which is what
// encoding/json writes for a []byte and what every JSON API means by binary.
// The bytes are not decoded and re-encoded: the document already holds the
// canonical spelling bar the breaks, and ast.TagNode.Resolve has already read
// it as base64, so writing the text back cannot invent one.
//
// It was a sequence of the numbers before -- "!!binary aGVsbG8=" gave
// [104,101,108,108,111] -- which no other JSON writer produces.
func binaryToken(text string) JSONToken {
	canonical := withoutBase64Breaks(text)
	if _, err := base64.StdEncoding.DecodeString(canonical); err != nil {
		return JSONToken{Kind: JSONString, Value: text}
	}

	return JSONToken{Kind: JSONString, Value: canonical}
}

// withoutBase64Breaks returns text with the line breaks and spacing RFC 2045
// permits inside an encoded stream taken out, which is base64's canonical form.
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

// appendJSONString writes a JSON string. Anything needing an escape goes
// through encoding/json rather than being escaped here, so the rules are the
// standard library's and not a second set of them.
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
