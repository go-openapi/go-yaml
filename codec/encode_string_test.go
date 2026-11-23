// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"iter"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestMarshalEscapesWhatAStreamCannotHoldRaw marshals strings holding each kind of character c-printable leaves out,
// and the two a quoted scalar alone can carry, as a value and as a key, with and without UseSingleQuote. Each one is
// double-quoted with an escape, and reads back as it was. Written raw, or single-quoted where there are no escapes,
// it did not.
func TestMarshalEscapesWhatAStreamCannotHoldRaw(t *testing.T) {
	for tc := range marshalEscapesWhatAStreamCannotHoldRawCases() {
		name, s := tc.name, tc.value
		for _, single := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				out, err := codec.Marshal(map[string]string{"value": s, s: "key"}, codec.UseSingleQuote(single))
				require.NoError(t, err)

				var back map[string]string
				require.NoError(t, codec.Unmarshal(out, &back), "%q", out)
				assert.Equal(t, s, back["value"], "%q", out)
				assert.Equal(t, "key", back[s], "%q", out)
			})
		}
	}
}

// marshalEscapesWhatAStreamCannotHoldRawCase holds a string with a character that must be escaped.
type marshalEscapesWhatAStreamCannotHoldRawCase struct {
	name  string
	value string
}

func marshalEscapesWhatAStreamCannotHoldRawCases() iter.Seq[marshalEscapesWhatAStreamCannotHoldRawCase] {
	return slices.Values([]marshalEscapesWhatAStreamCannotHoldRawCase{
		{name: "DEL", value: "a" + string(rune(0x7f)) + "b"},
		{name: "NEL, which may stand", value: "a" + string(rune(0x85)) + "b"},
		{name: "U+FFFE", value: "a" + string(rune(0xfffe)) + "b"},
		{name: "U+FFFF", value: "a" + string(rune(0xffff)) + "b"},
		{name: "a C0 control", value: "a" + string(rune(0x01)) + "b"},
		{name: "a C1 control", value: "a" + string(rune(0x9b)) + "b"},
		{name: "a byte order mark", value: "a" + string(rune(0xfeff)) + "b"},
		{name: "a carriage return", value: "a\rb"},
		{name: "a tab, which may too", value: "a\tb"},
		{name: "an escape", value: "a" + string(rune(0x1b)) + "b"},
		{name: "over several lines", value: "a\n" + string(rune(0x01)) + "\nb"},
	})
}

// TestMarshalQuotesAStringThatStartsOrEndsWithATab marshals strings whose plain spelling would lose a tab: YAML
// strips the white space around a plain scalar, tabs included. Each is quoted, as a value and as a key, and reads
// back as it was.
func TestMarshalQuotesAStringThatStartsOrEndsWithATab(t *testing.T) {
	for _, s := range []string{"\ta", "a\t", "\t", "\t a \t"} {
		out, err := codec.Marshal(map[string]string{"value": s, s: "key"})
		require.NoError(t, err)

		var back map[string]string
		require.NoError(t, codec.Unmarshal(out, &back), "%q", out)
		assert.Equal(t, s, back["value"], "%q", out)
		assert.Equal(t, "key", back[s], "%q", out)
	}
}

// TestMarshalQuotesAStringThatStartsWithAnIndicator marshals strings that open with "-" or "?" followed by
// whitespace or nothing, and one where ":" is followed by a tab. Written plain, each read back as a sequence or a
// mapping: "-\tx" as a sequence holding "x", "? x" as a mapping keyed by "x", "a:\tb" as {a: b}. Each is quoted, as
// an entry of a sequence and as a key, and reads back as it was.
//
// The JSON corpus of github.com/go-openapi/conformance-suites found the first through codec.FromJSON, which writes
// a string as Marshal does.
func TestMarshalQuotesAStringThatStartsWithAnIndicator(t *testing.T) {
	for _, s := range []string{"-\tx", "-\t\t_", "?", "? x", "?\tx", "a:\tb", "- x", "-"} {
		out, err := codec.Marshal([]any{s, map[string]string{s: "key"}})
		require.NoError(t, err)

		var back []any
		require.NoError(t, codec.Unmarshal(out, &back), "%q", out)
		require.Len(t, back, 2, "%q", out)
		assert.Equal(t, s, back[0], "%q", out)
		assert.Equal(t, map[string]any{s: "key"}, back[1], "%q", out)
	}
}

// TestMarshalWritesAKeyHoldingALineBreak marshals map keys holding line breaks. An implicit key cannot span lines, so
// the key is double-quoted; written as a "|" block, it did not parse.
// TestMarshalAStringSpelledAsATimestampReadsBackAsAString marshals strings that look like timestamps,
// and reads each back under YAML 1.1 into an any. A 1.1 reader resolves a plain timestamp to a time.Time,
// so the encoder quotes each string the yaml.org expression accepts, and leaves the rest plain.
func TestMarshalAStringSpelledAsATimestampReadsBackAsAString(t *testing.T) {
	for _, s := range []string{
		"2002-12-14",
		"2001-1-2",
		"2001-12-14 21:59:43.10 -5",
		"2001-12-14t21:59:43.10-05:00",
		"2015-2-3T3:04:05Z",
		"2015-2-3T3:4:5Z",
	} {
		t.Run(s, func(t *testing.T) {
			out, err := codec.Marshal(map[string]string{"a": s})
			require.NoError(t, err)

			var got any
			require.NoError(t, codec.Unmarshal(append([]byte("%YAML 1.1\n---\n"), out...), &got))
			assert.Equal(t, map[string]any{"a": s}, got, "written as %q", out)
		})
	}
}

func TestMarshalWritesAKeyHoldingALineBreak(t *testing.T) {
	for _, k := range []string{"a\nb", "a\n", "\n"} {
		out, err := codec.Marshal(map[string]any{k: 1, "nested": map[string]int{k: 2}, "seq": []map[string]int{{k: 3}}})
		require.NoError(t, err)

		var back map[string]any
		require.NoError(t, codec.Unmarshal(out, &back), "%q", out)
		assert.Equal(t, uint64(1), back[k], "%q", out)
	}
}

// encodeStringCases covers when a string is written plain and when it is quoted, and the quote style options.
func encodeStringCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"string/dash-quoted",
			"a: \"-\"\n",
			map[string]string{"a": "-"},
			nil,
		},
		{
			"string/timestamp-quoted",
			"t2: \"2018-01-09T10:40:47Z\"\nt4: \"2098-01-09T10:40:47Z\"\n",
			map[string]string{
				"t2": "2018-01-09T10:40:47Z",
				"t4": "2098-01-09T10:40:47Z",
			},
			nil,
		},
		{
			"string/duration-plain",
			"a: 3s\n",
			map[string]string{
				"a": "3s",
			},
			nil,
		},
		{
			"string/angle-brackets-plain",
			"a: <foo>\n",
			map[string]string{"a": "<foo>"},
			nil,
		},
		{
			"string/colon-without-space-quoted",
			"a: \"1:1\"\n",
			map[string]string{"a": "1:1"},
			nil,
		},
		{
			"string/dotted-quad-plain",
			"a: 1.2.3.4\n",
			map[string]string{"a": "1.2.3.4"},
			nil,
		},
		{
			"string/colon-space-quoted",
			"a: \"b: c\"\n",
			map[string]string{"a": "b: c"},
			nil,
		},
		{
			"string/hash-after-space-quoted",
			"a: \"Hello #comment\"\n",
			map[string]string{"a": "Hello #comment"},
			nil,
		},
		{
			"string/leading-space-quoted",
			"a: \" b\"\n",
			map[string]string{"a": " b"},
			nil,
		},
		{
			"string/trailing-space-quoted",
			"a: \"b \"\n",
			map[string]string{"a": "b "},
			nil,
		},
		{
			"string/surrounding-spaces-quoted",
			"a: \" b \"\n",
			map[string]string{"a": " b "},
			nil,
		},
		{
			"string/leading-backquote-quoted",
			"a: \"`b` c\"\n",
			map[string]string{"a": "`b` c"},
			nil,
		},
		{
			"string/backslash-quoted",
			"a: \"\\\\0\"\n",
			map[string]string{"a": "\\0"},
			nil,
		},
		{
			"string/multibyte-plain",
			"v: あいうえお\nv2: かきくけこ\n",
			map[string]string{"v": "あいうえお", "v2": "かきくけこ"},
			nil,
		},
		{
			"quote-style/single-doubles-inner-quotes",
			`v: '''a''b'` + "\n",
			map[string]string{"v": `'a'b`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(true),
			},
		},
		{
			"quote-style/double-keeps-inner-single-quotes",
			`v: "'a'b"` + "\n",
			map[string]string{"v": `'a'b`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"quote-style/single-keeps-backslash",
			`a: '\.yaml'` + "\n",
			map[string]string{"a": `\.yaml`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(true),
			},
		},
	})
}

// encodeMultiLineCases covers strings holding line breaks: literal block scalars and their chomping, carriage returns, and UseLiteralStyleIfMultiline.
func encodeMultiLineCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"multi-line/literal-clip",
			"hello: |\n  hello\n  world\n",
			map[string]string{"hello": "hello\nworld\n"},
			nil,
		},
		{
			"multi-line/literal-strip",
			"hello: |-\n  hello\n  world\n",
			map[string]string{"hello": "hello\nworld"},
			nil,
		},
		{
			"multi-line/literal-keep",
			"hello: |+\n  hello\n  world\n\n",
			map[string]string{"hello": "hello\nworld\n\n"},
			nil,
		},
		// With no line of text, "|" and "|-" both read "", so a value made only of line breaks keeps them with "|+".
		{
			"multi-line/only-a-line-break",
			"hello: |+\n\n",
			map[string]string{"hello": "\n"},
			nil,
		},
		// A reader takes a block's indentation from its first line of text, so one that starts with a space
		// would lose that space. The value takes double quotes.
		{
			"multi-line/first-text-indented-double-quoted",
			"hello: \"\\n 0\"\n",
			map[string]string{"hello": "\n 0"},
			nil,
		},
		{
			"multi-line/literal-nested",
			"hello:\n  hello: |\n    hello\n    world\n",
			map[string]map[string]string{"hello": {"hello": "hello\nworld\n"}},
			nil,
		},
		// A carriage return has no block scalar spelling: YAML normalizes
		// "\r\n" and a lone "\r" to "\n" on read, so the block these used to
		// be written as read back without the CRs. See
		// TestMarshalStringHoldingCarriageReturn.
		{
			"multi-line/carriage-return-double-quoted",
			"hello: \"hello\\rworld\\r\"\n",
			map[string]string{"hello": "hello\rworld\r"},
			nil,
		},
		{
			"multi-line/crlf-double-quoted",
			"hello: \"hello\\r\\nworld\\r\\n\"\n",
			map[string]string{"hello": "hello\r\nworld\r\n"},
			nil,
		},
		{
			"multi-line/literal-style-option",
			"v: |-\n  username: hello\n  password: hello123\n",
			map[string]any{"v": "username: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(true),
			},
		},
		{
			"multi-line/literal-style-option-with-hash-line",
			"v: |-\n  # comment\n  username: hello\n  password: hello123\n",
			map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(true),
			},
		},
		{
			"multi-line/literal-style-option-off",
			"v: \"# comment\\nusername: hello\\npassword: hello123\"\n",
			map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(false),
			},
		},
	})
}

func TestEncodeWithNestedYAML(t *testing.T) {
	// Represents objects containing stringified YAML, and special chars
	for test := range encodeWithNestedYAMLCases() {
		yamlBytesForced, err := codec.Marshal(test.value, codec.UseLiteralStyleIfMultiline(true))
		require.NoError(t, err)

		// Convert it back for proper equality testing
		var unmarshaled any

		require.NoError(t, codec.Unmarshal(yamlBytesForced, &unmarshaled))

		require.Equal(t, test.value, unmarshaled)

		if test.expectDifferent {
			yamlBytesNotForced, err := codec.Marshal(test.value)
			require.NoError(t, err)

			require.NotEqualf(t, string(yamlBytesNotForced), string(yamlBytesForced),
				"the output does not change when literal style is not forced")
		}
	}
}

// encodeWithNestedYAMLCase holds a value holding stringified YAML, and whether forcing literal style changes its encoding.
type encodeWithNestedYAMLCase struct {
	value any
	// If true, expects a different result between when using forced literal style or not
	expectDifferent bool
}

func encodeWithNestedYAMLCases() iter.Seq[encodeWithNestedYAMLCase] {
	return slices.Values([]encodeWithNestedYAMLCase{
		{
			value:           map[string]any{"v": "# comment\nname: hello\npassword: hello123\nspecial: \":ghost:\"\ntext: |\n  nested multiline!"},
			expectDifferent: true,
		},
		{
			value:           map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			expectDifferent: true,
		},
		{
			value:           map[string]any{"v": "# comment\n"},
			expectDifferent: true,
		},
	})
}

// TestIssue356 covers a block scalar inside a sequence, whose content used to
// come back mangled.
//
// The sequence indentation of the source is not part of what is checked:
// rendering lays a document out in the library's own layout rather than
// replaying the columns it was read at. What must survive is the scalar --
// every line of it, including the blank one, at the relative indentation that
// makes it one value rather than a nested mapping.
func TestIssue356(t *testing.T) {
	for test := range issue356Cases() {
		t.Run(test.name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.in))
			require.NoError(t, err)
			got := f.String()
			require.Equal(t, test.want, got)

			// And it settles: reading the result back gives the same text.
			again, err := parser.ParseBytes([]byte(got))
			require.NoError(t, err)
			require.Equalf(t, got, again.String(), "a second render changed the document")
		})
	}
}

// issue356Case holds a document with a block scalar in a sequence, and the text it renders to.
type issue356Case struct {
	name string
	in   string
	want string
}

func issue356Cases() iter.Seq[issue356Case] {
	const scalar = `

  key:
    nest1: something
    nest2:
      nest2a: b`

	return slices.Values([]issue356Case{
		{
			name: "content on first line",
			in: `args:
  - |

    key:
      nest1: something
      nest2:
        nest2a: b
`,
			want: "args:\n- |" + scalar + "\n",
		},
		{
			name: "empty first line",
			in: `args:
  - |

    key:
      nest1: something
      nest2:
        nest2a: b
`,
			want: "args:\n- |" + scalar + "\n",
		},
	})
}

func TestMarshalIndentWithMultipleText(t *testing.T) {
	for tt := range marshalIndentWithMultipleTextCases() {
		t.Run(tt.name, func(t *testing.T) {
			b, err := codec.Marshal(tt.input, tt.indent)
			require.NoError(t, err)
			got := string(b)
			require.Equal(t, tt.want, got)
		})
	}
}

// marshalIndentWithMultipleTextCase holds a value, an indent option, and the YAML it encodes to.
type marshalIndentWithMultipleTextCase struct {
	name   string
	input  map[string]any
	indent codec.EncodeOption
	want   string
}

func marshalIndentWithMultipleTextCases() iter.Seq[marshalIndentWithMultipleTextCase] {
	return slices.Values([]marshalIndentWithMultipleTextCase{
		{
			name: "depth1",
			input: map[string]any{
				"key": []string{`line1
line2
line3`},
			},
			indent: codec.Indent(2),
			want: `key:
- |-
  line1
  line2
  line3
`,
		},
		{
			name: "depth2",
			input: map[string]any{
				"key": map[string]any{
					"key2": []string{`line1
line2
line3`},
				},
			},
			indent: codec.Indent(2),
			want: `key:
  key2:
  - |-
    line1
    line2
    line3
`,
		},
		{
			name: "raw string new lines",
			input: map[string]any{
				"key": "line1\nline2\nline3",
			},
			indent: codec.Indent(4),
			want: `key: |-
    line1
    line2
    line3
`,
		},
		{
			// The later lines of an entry stand under its first one, after "- ", whatever the indent.
			name: "a mapping and a sequence inside entries",
			input: map[string]any{
				"key": []any{map[string]any{"a": 1, "b": 2}, []any{1, 2}},
			},
			indent: codec.Indent(4),
			want: `key:
- a: 1
  b: 2
- - 1
  - 2
`,
		},
		{
			name: "a mapping nested under a key inside an entry",
			input: map[string]any{
				"key": []any{map[string]any{"a": 1, "b": map[string]any{"c": 2}}},
			},
			indent: codec.Indent(3),
			want: `key:
- a: 1
  b:
     c: 2
`,
		},
	})
}

// TestMarshalStringHoldingCarriageReturn checks that a string holding a
// carriage return survives Marshal and Unmarshal.
//
// YAML normalizes a stream's line breaks on read: "\r\n" and a lone "\r" both
// become "\n". So a carriage return has no plain, single-quoted or block
// spelling -- only a double-quoted scalar, where "\r" is an escape, carries
// one. Marshal wrote a literal block for these and dropped every CR.
func TestMarshalStringHoldingCarriageReturn(t *testing.T) {
	for test := range marshalStringHoldingCarriageReturnCases() {
		t.Run(test.name, func(t *testing.T) {
			out, err := codec.Marshal(test.in)
			require.NoError(t, err)
			require.Equal(t, test.want, string(out))

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, test.in, back)
		})
	}
}

// marshalStringHoldingCarriageReturnCase holds a string and the YAML it encodes to.
type marshalStringHoldingCarriageReturnCase struct {
	name string
	in   string
	want string
}

func marshalStringHoldingCarriageReturnCases() iter.Seq[marshalStringHoldingCarriageReturnCase] {
	return slices.Values([]marshalStringHoldingCarriageReturnCase{
		{name: "CRLF throughout", in: "a\r\nb\r\n", want: "\"a\\r\\nb\\r\\n\"\n"},
		{name: "a lone CR", in: "a\rb", want: "\"a\\rb\"\n"},
		{name: "one CRLF at the end", in: "x\r\n", want: "\"x\\r\\n\"\n"},
		{name: "a CR and nothing else", in: "\r", want: "\"\\r\"\n"},
		{name: "CR and LF mixed", in: "a\r\nb\nc", want: "\"a\\r\\nb\\nc\"\n"},

		// A value holding only line feeds keeps the block scalar it always had.
		{name: "LF throughout", in: "a\nb\n", want: "|\n  a\n  b\n"},
	})
}

// TestMarshalCarriageReturnUnderLiteralStyle checks the same for the option
// that asks for a block scalar by name. A value holding a carriage return has
// no block spelling, so the option cannot be given it.
func TestMarshalCarriageReturnUnderLiteralStyle(t *testing.T) {
	const in = "a\r\nb\r\n"

	var out []byte
	out, err := codec.Marshal(in, codec.UseLiteralStyleIfMultiline(true))
	require.NoError(t, err)

	var back string
	require.NoError(t, codec.Unmarshal(out, &back))
	require.Equal(t, in, back)
}

// TestMarshalCarriageReturnInAMapping checks that the value of a mapping entry
// is quoted the same way, since the entry renders through the block path.
func TestMarshalCarriageReturnInAMapping(t *testing.T) {
	in := map[string]string{"a": "p\r\nq\r\n"}

	out, err := codec.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, "a: \"p\\r\\nq\\r\\n\"\n", string(out))

	back := map[string]string{}
	require.NoError(t, codec.Unmarshal(out, &back))
	require.Equal(t, in, back)
}

// TestMarshalQuotesLeadingZeroDecimals checks that a string of decimal digits
// written with a leading zero comes out quoted.
//
// This library resolves integers as YAML 1.1 does, where "0" followed by digits
// is octal and "088253" is not octal at all, so it reads a string. A reader
// following YAML 1.2's core schema resolves [-+]?[0-9]+ and reads the integer
// 88253 from the same unquoted text. Quoting settles it for both.
func TestMarshalQuotesLeadingZeroDecimals(t *testing.T) {
	quoted := []string{"088253", "09", "0777", "00", "010", "-088", "+09"}
	for _, in := range quoted {
		t.Run(in, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.Equal(t, "\""+in+"\"\n", string(out))

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}

	// A leading zero followed by anything but digits resolves to a string
	// under every schema, so it stays plain.
	plain := []string{"08a", "0-8", "0 9"}
	for _, in := range plain {
		t.Run(in, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.Equal(t, in+"\n", string(out))
		})
	}
}

// TestMarshalStringHoldingAByteOrderMark checks that a string holding U+FEFF
// survives Marshal and Unmarshal.
//
// nb-char excludes the mark, so no plain or block scalar may hold one. A quoted
// scalar may -- nb-double-char and nb-single-char are built from nb-json
// instead -- so the value has to come out quoted, where "\ufeff" is an escape.
func TestMarshalStringHoldingAByteOrderMark(t *testing.T) {
	const mark = "\ufeff"

	for tc := range marshalStringHoldingAByteOrderMarkCases(mark) {
		in := tc.in
		t.Run(tc.name, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.NotContains(t, string(out[:len(out)-1]), mark, "the mark is written as an escape, not as itself")

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}
}

// marshalStringHoldingAByteOrderMarkCase holds a string with a byte order mark in it.
type marshalStringHoldingAByteOrderMarkCase struct {
	name string
	in   string
}

func marshalStringHoldingAByteOrderMarkCases(mark string) iter.Seq[marshalStringHoldingAByteOrderMarkCase] {
	return slices.Values([]marshalStringHoldingAByteOrderMarkCase{
		{name: "in the middle", in: "x" + mark + "y"},
		{name: "opening the value", in: mark + "x"},
		{name: "the whole value", in: mark},
		{name: "with a line feed", in: "x\n" + mark + "y\n"},
	})
}

// TestAStringSpellingAnInfinityIsQuoted holds a Go string that spells an
// infinity or a NaN to coming back as the same string.
//
// token.reservedEncKeywordTypes decides what the encoder quotes, and its own
// doc comment calls it a superset of reservedKeywordTypes -- the keywords the
// scanner resolves. It was not one: init filled it with the nulls and the
// booleans and left the infinities and the NaNs out, so the string ".inf" was
// written as "a: .inf" and read back as the float +Inf. A string turned into a
// number by being written down and read again.
//
// The float path is unaffected: IsNeedQuoted is asked about strings, so +Inf
// still encodes as ".inf" with no quotes and still reads back as +Inf.
func TestAStringSpellingAnInfinityIsQuoted(t *testing.T) {
	t.Run("a string survives the round trip", func(t *testing.T) {
		for _, spelling := range []string{
			".inf", ".Inf", ".INF",
			"-.inf", "-.Inf", "-.INF",
			".nan", ".NaN", ".NAN",
		} {
			out, err := codec.Marshal(map[string]string{"a": spelling})
			require.NoErrorf(t, err, "%q", spelling)

			var back map[string]any
			require.NoErrorf(t, codec.Unmarshal(out, &back), "%q wrote %q", spelling, out)
			assert.Equalf(t, spelling, back["a"], "%q wrote %q", spelling, out)
		}
	})

	t.Run("the float still writes the bare spelling", func(t *testing.T) {
		for tc := range aStringSpellingAnInfinityIsQuotedFloatCases() {
			out, err := codec.Marshal(map[string]float64{"a": tc.value})
			require.NoError(t, err)
			assert.Equal(t, tc.wants, string(out))

			var back map[string]any
			require.NoError(t, codec.Unmarshal(out, &back))
			assert.Equal(t, tc.value, back["a"], "%q", out)
		}

		out, err := codec.Marshal(map[string]float64{"a": math.NaN()})
		require.NoError(t, err)
		assert.Equal(t, "a: .nan\n", string(out))
	})
}

// aStringSpellingAnInfinityIsQuotedFloatCase holds a float and the YAML it encodes to.
type aStringSpellingAnInfinityIsQuotedFloatCase struct {
	value float64
	wants string
}

func aStringSpellingAnInfinityIsQuotedFloatCases() iter.Seq[aStringSpellingAnInfinityIsQuotedFloatCase] {
	return slices.Values([]aStringSpellingAnInfinityIsQuotedFloatCase{
		{math.Inf(1), "a: .inf\n"},
		{math.Inf(-1), "a: -.inf\n"},
	})
}

// TestAStringReadingAsADocumentMarkerRoundTrips checks that Marshal quotes a
// string opening with "---" or "..." followed by a blank or by nothing.
//
// A document's own scalar and a key of its top mapping start their line, where
// the same text written plain is a marker: "---" read back as null, "..." as an
// empty stream, "---\tx" as "x", and "... x" did not parse. Each is written
// alone, as a key and as a sequence entry, and read back as the value it was.
func TestAStringReadingAsADocumentMarkerRoundTrips(t *testing.T) {
	for _, s := range []string{
		"---", "...", "--- x", "... x", "---\tx", "...\tx",
		// Shapes outside the fault: a fourth character that is no blank.
		"----", "....", "...x", "---x",
	} {
		for tc := range aStringReadingAsADocumentMarkerRoundTripsCases(s) {
			name, v := tc.name, tc.value
			t.Run(s+" "+name, func(t *testing.T) {
				out, err := codec.Marshal(v)
				require.NoError(t, err)

				switch want := v.(type) {
				case string:
					var got string
					require.NoErrorf(t, codec.Unmarshal(out, &got), "%q wrote %q", s, out)
					assert.Equalf(t, want, got, "%q wrote %q", s, out)
				case map[string]int:
					var got map[string]int
					require.NoErrorf(t, codec.Unmarshal(out, &got), "%q wrote %q", s, out)
					assert.Equalf(t, want, got, "%q wrote %q", s, out)
				case []string:
					var got []string
					require.NoErrorf(t, codec.Unmarshal(out, &got), "%q wrote %q", s, out)
					assert.Equalf(t, want, got, "%q wrote %q", s, out)
				}
			})
		}
	}
}

// aStringReadingAsADocumentMarkerRoundTripsCase holds a value with the string in one place.
type aStringReadingAsADocumentMarkerRoundTripsCase struct {
	name  string
	value any
}

func aStringReadingAsADocumentMarkerRoundTripsCases(s string) iter.Seq[aStringReadingAsADocumentMarkerRoundTripsCase] {
	return slices.Values([]aStringReadingAsADocumentMarkerRoundTripsCase{
		{name: "alone", value: s},
		{name: "as a key", value: map[string]int{s: 1}},
		{name: "in a sequence", value: []string{s}},
	})
}

// TestAMultiLineStringReadsBackAsWritten encodes strings holding line breaks, at the root, in a mapping and in a
// sequence, with and without UseLiteralStyleIfMultiline, and requires the decode to read each one back.
//
// "\n" was written "|" over one empty line and read back as "", and "\n 0" was written "|-" over an empty line
// and "   0", and read back as "\n0". FromJSON writes through the same encoder, so a JSON document lost both.
func TestAMultiLineStringReadsBackAsWritten(t *testing.T) {
	values := []string{
		"\n", "\n\n", "\n\n\n", "\n 0", "\n\n  x\ny", " a\nb", "a\n b", "\n0", "x\n", "a\n\n", "\t\nx", "\n\tx", "\n \n",
	}
	for _, v := range values {
		for _, literal := range []bool{false, true} {
			opt := codec.UseLiteralStyleIfMultiline(literal)
			for name, doc := range map[string]any{"root": v, "mapping": map[string]string{"k": v}, "sequence": []string{v, "z"}} {
				out, err := codec.Marshal(doc, opt)
				require.NoErrorf(t, err, "%s %q", name, v)

				back := reflect.New(reflect.TypeOf(doc))
				require.NoErrorf(t, codec.Unmarshal(out, back.Interface()), "%s %q written as %q", name, v, out)
				assert.Equalf(t, doc, back.Elem().Interface(), "%s %q written as %q, literal=%v", name, v, out, literal)
			}
		}

		js, err := json.Marshal(map[string]string{"k": v})
		require.NoError(t, err)
		yml, err := codec.FromJSON(js)
		require.NoErrorf(t, err, "FromJSON %q", v)
		back, err := codec.ToJSON(yml)
		require.NoErrorf(t, err, "ToJSON of %q", yml)
		assert.JSONEqf(t, string(js), string(back), "FromJSON %q wrote %q", v, yml)
	}
}
