// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import (
	"math/rand/v2"
	"strconv"
	"strings"
)

// Value is what a document means, generated before any bytes exist.
//
// Generating the meaning first is what makes the expectation free: a document
// written from a Value has a known reading, where a document generated as text
// needs an oracle to say what it should decode to. It is the same argument as
// yamlgen's, and it holds for the same reason.
type Value interface {
	// Decoded returns what unmarshalling into an any should produce.
	Decoded() any
}

type (
	// Null is JSON's null.
	Null struct{}
	// Bool is true or false.
	Bool struct{ V bool }
	// Num is a number, held as the text of one.
	//
	// JSON numbers are text: the grammar admits magnitudes and precisions no
	// float64 carries, and a generator that held them as float64 could not
	// produce those at all -- which are exactly the documents the
	// number/out-of-range tag is about.
	Num struct{ Text string }
	// Str is a string.
	Str struct{ V string }
	// Arr is an array.
	Arr struct{ Items []Value }
	// Obj is an object. Order is kept so emitting is deterministic; JSON gives
	// it no meaning.
	Obj struct{ Members []Member }
)

// Member is one name/value pair of an object.
type Member struct {
	Name string
	Val  Value
}

func (Null) Decoded() any   { return nil }
func (b Bool) Decoded() any { return b.V }
func (s Str) Decoded() any  { return s.V }

// Decoded parses the number the way a conventional decoder would, and reports
// the text when that loses information. A caller comparing against it has to
// decide which it wanted, which is the point: this is where the
// implementation-defined question lives.
func (n Num) Decoded() any {
	f, err := strconv.ParseFloat(n.Text, 64)
	if err != nil {
		return n.Text
	}

	return f
}

func (a Arr) Decoded() any {
	out := make([]any, 0, len(a.Items))
	for _, item := range a.Items {
		out = append(out, item.Decoded())
	}

	return out
}

func (o Obj) Decoded() any {
	out := make(map[string]any, len(o.Members))
	for _, m := range o.Members {
		out[m.Name] = m.Val.Decoded()
	}

	return out
}

// Style is one way of writing a Value down.
//
// JSON's presentation freedom is small next to YAML's -- there is no
// indentation to get wrong and no block scalars -- so the axes here are
// whitespace, how a number is spelled, and how much of a string is escaped.
type Style struct {
	// Indent is how many spaces each nesting level adds. Zero writes the
	// document with no insignificant whitespace at all.
	Indent int
	// SpaceAfterName puts a space after the name separator.
	SpaceAfterName bool
	// SpaceAfterComma puts a space after a value separator, where the layout
	// has not already put a newline there.
	SpaceAfterComma bool
	// EscapeNonASCII writes every character above U+007F as a \u escape, which
	// is the presentation difference most likely to be got wrong.
	EscapeNonASCII bool
	// EscapeSolidus writes "/" as "\/", which JSON permits and nothing
	// requires.
	EscapeSolidus bool
}

// Emit writes a Value in a Style.
//
// It is an independent writer, not a call into any library under test: one
// style cannot demonstrate that several presentations read alike, and a
// generator that emitted through the implementation it is testing would agree
// with that implementation by construction.
func Emit(v Value, s Style) []byte {
	var b strings.Builder

	emitValue(&b, v, s, 0)

	return []byte(b.String())
}

func emitValue(b *strings.Builder, v Value, s Style, depth int) {
	switch val := v.(type) {
	case Null:
		b.WriteString("null")
	case Bool:
		b.WriteString(strconv.FormatBool(val.V))
	case Num:
		b.WriteString(val.Text)
	case Str:
		writeString(b, val.V, s)
	case Arr:
		emitSeq(b, s, depth, len(val.Items), '[', ']', func(i int) {
			emitValue(b, val.Items[i], s, depth+1)
		})
	case Obj:
		emitSeq(b, s, depth, len(val.Members), '{', '}', func(i int) {
			writeString(b, val.Members[i].Name, s)
			b.WriteByte(':')

			if s.SpaceAfterName {
				b.WriteByte(' ')
			}

			emitValue(b, val.Members[i].Val, s, depth+1)
		})
	}
}

// emitSeq writes a bracketed, comma-separated run, which is the only layout
// JSON has and is shared by both collections.
func emitSeq(b *strings.Builder, s Style, depth, n int, open, shut byte, item func(int)) {
	b.WriteByte(open)

	if n == 0 {
		b.WriteByte(shut)

		return
	}

	for i := range n {
		if i > 0 {
			b.WriteByte(',')

			if s.SpaceAfterComma && s.Indent == 0 {
				b.WriteByte(' ')
			}
		}

		newline(b, s, depth+1)
		item(i)
	}

	newline(b, s, depth)
	b.WriteByte(shut)
}

func newline(b *strings.Builder, s Style, depth int) {
	if s.Indent == 0 {
		return
	}

	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", s.Indent*depth))
}

// writeString writes a JSON string literal.
//
// The escapes that are mandatory are the quote, the reverse solidus and
// everything below U+0020. The rest are the Style's business, and being able to
// write the same string several ways is most of what makes the presentation
// axis worth having.
func writeString(b *strings.Builder, v string, s Style) {
	b.WriteByte('"')

	for _, r := range v {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '/' && s.EscapeSolidus:
			b.WriteString(`\/`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20, r > 0x7E && s.EscapeNonASCII:
			writeEscape(b, r)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')
}

// writeEscape writes a code point as one \u escape, or as the surrogate pair
// JSON requires above the basic plane.
func writeEscape(b *strings.Builder, r rune) {
	if r <= 0xFFFF {
		b.WriteString(`\u` + pad4(uint32(r)))

		return
	}

	r -= 0x10000
	b.WriteString(`\u` + pad4(0xD800+uint32(r>>10)))
	b.WriteString(`\u` + pad4(0xDC00+uint32(r&0x3FF)))
}

func pad4(v uint32) string {
	out := strconv.FormatUint(uint64(v), 16)
	for len(out) < 4 {
		out = "0" + out
	}

	return out
}

// RandomValue draws a Value.
//
// It is written from the grammar and knows nothing about any parser: the shapes
// it can produce are the shapes RFC 8259 defines, weighted so that a document
// of a useful size comes out rather than one scalar or a thousand.
func RandomValue(rng *rand.Rand, depth int) Value {
	// Past a few levels the collections stop being drawn, so recursion ends.
	kinds := 4
	if depth < 3 {
		kinds = 6
	}

	switch rng.IntN(kinds) {
	case 0:
		return Null{}
	case 1:
		return Bool{V: rng.IntN(2) == 0}
	case 2:
		return Num{Text: randomNumber(rng)}
	case 3:
		return Str{V: randomString(rng)}
	case 4:
		items := make([]Value, rng.IntN(4))
		for i := range items {
			items[i] = RandomValue(rng, depth+1)
		}

		return Arr{Items: items}
	default:
		members := make([]Member, rng.IntN(4))
		for i := range members {
			members[i] = Member{Name: randomString(rng), Val: RandomValue(rng, depth+1)}
		}

		return dedupe(members)
	}
}

// dedupe drops repeated names, because two members of one name is a document
// whose meaning is implementation-defined and this generator claims to know
// what its documents mean.
func dedupe(members []Member) Obj {
	seen := make(map[string]bool, len(members))
	out := members[:0]

	for _, m := range members {
		if seen[m.Name] {
			continue
		}

		seen[m.Name] = true

		out = append(out, m)
	}

	return Obj{Members: out}
}

// randomNumber spells a number every way the grammar allows, including the
// magnitudes float64 cannot carry.
func randomNumber(rng *rand.Rand) string {
	var b strings.Builder

	if rng.IntN(3) == 0 {
		b.WriteByte('-')
	}

	if rng.IntN(4) == 0 {
		b.WriteByte('0')
	} else {
		b.WriteByte(byte('1' + rng.IntN(9)))

		for range rng.IntN(4) {
			b.WriteByte(byte('0' + rng.IntN(10)))
		}
	}

	if rng.IntN(3) == 0 {
		b.WriteByte('.')

		for range 1 + rng.IntN(3) {
			b.WriteByte(byte('0' + rng.IntN(10)))
		}
	}

	if rng.IntN(4) == 0 {
		b.WriteString([]string{"e", "E"}[rng.IntN(2)])
		b.WriteString([]string{"", "+", "-"}[rng.IntN(3)])

		for range 1 + rng.IntN(4) {
			b.WriteByte(byte('0' + rng.IntN(10)))
		}
	}

	return b.String()
}

// randomString draws string content, reaching for the characters that make a
// writer work: the mandatory escapes, characters outside ASCII, and the
// astral plane that needs a surrogate pair.
func randomString(rng *rand.Rand) string {
	alphabet := []rune{
		'a', 'b', 'z', 'A', 'Z', '0', '9', ' ', '-', '_', '/',
		'"', '\\', '\n', '\t', '\r', 0x00, 0x1F,
		'é', 'ß', '☃', 0xFEFF, 0x1D11E, 0x10FFFF,
	}

	out := make([]rune, rng.IntN(6))
	for i := range out {
		out[i] = alphabet[rng.IntN(len(alphabet))]
	}

	return string(out)
}

// RandomStyle draws a presentation.
func RandomStyle(rng *rand.Rand) Style {
	return Style{
		Indent:          []int{0, 0, 1, 2, 4}[rng.IntN(5)],
		SpaceAfterName:  rng.IntN(2) == 0,
		SpaceAfterComma: rng.IntN(2) == 0,
		EscapeNonASCII:  rng.IntN(2) == 0,
		EscapeSolidus:   rng.IntN(4) == 0,
	}
}
