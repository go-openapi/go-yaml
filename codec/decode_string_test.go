// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

// decodeSingleQuotedCases covers single-quoted scalars: the doubled quote, backslashes kept as written, and line folding.
func decodeSingleQuotedCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "single/plain",
			source: `'1': '2'`,
			value:  map[any]any{"1": `2`},
		},
		{
			name:   "single/double-quotes-inside",
			source: `'1': '"2"'`,
			value:  map[any]any{"1": `"2"`},
		},
		{
			name:   "single/escaped-quote-alone",
			source: `'1': ''''`,
			value:  map[any]any{"1": `'`},
		},
		{
			name:   "single/escaped-quotes-around",
			source: `'1': '''2'''`,
			value:  map[any]any{"1": `'2'`},
		},
		{
			name:   "single/escaped-quote-inside",
			source: `'1': 'B''z'`,
			value:  map[any]any{"1": `B'z`},
		},
		{
			name:   "single/backslash",
			source: `'1': '\'`,
			value:  map[any]any{"1": `\`},
		},
		{
			name:   "single/two-backslashes",
			source: `'1': '\\'`,
			value:  map[any]any{"1": `\\`},
		},
		{
			name:   "single/backslashed-double-quotes",
			source: `'1': '\"2\"'`,
			value:  map[any]any{"1": `\"2\"`},
		},
		{
			name:   "single/double-backslashed-double-quotes",
			source: `'1': '\\"2\\"'`,
			value:  map[any]any{"1": `\\"2\\"`},
		},
		{
			name:   "single/folded-lines",
			source: "'1': '   1\n    2\n    3'",
			value:  map[any]any{"1": "   1 2 3"},
		},
		{
			name:   "single/folded-leading-break",
			source: "'1': '\n    2\n    3'",
			value:  map[any]any{"1": " 2 3"},
		},
		{
			name:   "single/dash",
			source: "a: '-'\n",
			value:  map[string]string{"a": "-"},
		},
		{
			name:   "single/colon-space-inside",
			source: "a: 'b: c'\n",
			value:  map[string]string{"a": "b: c"},
		},
		{
			name:   "single/hash-inside",
			source: "a: 'Hello #comment'\n",
			value:  map[string]string{"a": "Hello #comment"},
		},
	})
}

// decodeDoubleQuotedCases covers double-quoted scalars: escapes, surrogate pairs, escaped line breaks and line folding.
func decodeDoubleQuotedCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "double/plain",
			source: `"1": "2"`,
			value:  map[any]any{"1": `2`},
		},
		{
			name:   "double/escaped-quotes-around",
			source: `"1": "\"2\""`,
			value:  map[any]any{"1": `"2"`},
		},
		{
			name:   "double/escaped-quote-alone",
			source: `"1": "\""`,
			value:  map[any]any{"1": `"`},
		},
		{
			name:   "double/escaped-quote-inside",
			source: `"1": "X\"z"`,
			value:  map[any]any{"1": `X"z`},
		},
		{
			name:   "double/backslash",
			source: `"1": "\\"`,
			value:  map[any]any{"1": `\`},
		},
		{
			name:   "double/two-backslashes",
			source: `"1": "\\\\"`,
			value:  map[any]any{"1": `\\`},
		},
		{
			name:   "double/escaped-backslash-and-quotes",
			source: `"1": "\\\"2\\\""`,
			value:  map[any]any{"1": `\"2\"`},
		},
		{
			name:   "double/folded-lines",
			source: "'1': \"   1\n    2\n    3\"",
			value:  map[any]any{"1": "   1 2 3"},
		},
		{
			name:   "double/folded-leading-break",
			source: "'1': \"\n    2\n    3\"",
			value:  map[any]any{"1": " 2 3"},
		},
		{
			name:   "double/hex-escape",
			source: `"1": "a\x2Fb"`,
			value:  map[any]any{"1": `a/b`},
		},
		{
			name:   "double/unicode-escape",
			source: `"1": "a\u002Fb"`,
			value:  map[any]any{"1": `a/b`},
		},
		{
			name:   "double/all-escape-widths",
			source: `"1": "a\x2Fb\u002Fc\U0000002Fd"`,
			value:  map[any]any{"1": `a/b/c/d`},
		},
		{
			name:   "double/newline-escape",
			source: "'1': \"2\\n3\"",
			value:  map[any]any{"1": "2\n3"},
		},
		{
			name:   "double/crlf-escape",
			source: "'1': \"2\\r\\n3\"",
			value:  map[any]any{"1": "2\r\n3"},
		},
		{
			name:   "double/escaped-line-break",
			source: "'1': \"a\\\nb\\\nc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/escaped-crlf-break",
			source: "'1': \"a\\\r\nb\\\r\nc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/escaped-cr-break",
			source: "'1': \"a\\\rb\\\rc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/empty",
			source: "v: \"\"\n",
			value:  map[string]string{"v": ""},
		},
		{
			name:   "double/colon-inside",
			source: "a: \"1:1\"\n",
			value:  map[string]string{"a": "1:1"},
		},
		{
			name:   "double/null-escape",
			source: "a: \"\\0\"\n",
			value:  map[string]string{"a": "\x00"},
		},
		{
			name: "double/spec-escapes",
			source: `
- "Fun with \\"
- "\" \a \b \e \f"
- "\n \r \t \v \0"
- "\  \_ \N \L \P \
  \x41 \u0041 \U00000041"
`,
			value: []string{"Fun with \\", "\" \u0007 \b \u001b \f", "\n \r \t \u000b \u0000", "\u0020 \u00a0 \u0085 \u2028 \u2029 A A A"},
		},
		{
			name:   "double/surrogate-pair",
			source: `"\ud83e\udd23"`,
			value:  "🤣",
		},
		{
			name:   "double/two-surrogate-pairs",
			source: `"\uD83D\uDE00\uD83D\uDE01"`,
			value:  "😀😁",
		},
		{
			name:   "double/surrogate-pairs-around-char",
			source: `"\uD83D\uDE00a\uD83D\uDE01"`,
			value:  "😀a😁",
		},
	})
}

// decodeKeyAndPlainCases covers mapping keys of non-string types, and plain scalars that stay strings.
func decodeKeyAndPlainCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "key/int",
			source: "1: v\n",
			value:  map[int]string{1: "v"},
		},
		{
			name:   "key/float",
			source: "1.1: v\n",
			value:  map[float64]string{1.1: "v"},
		},
		{
			name:   "key/bool",
			source: "true: v\n",
			value:  map[bool]string{true: "v"},
		},
		{
			name:   "key/timestamp",
			source: "2015-01-01T00:00:00Z: v\n",
			value:  map[time.Time]string{time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC): "v"},
		},
		{
			name:   "key/int-into-string-key",
			source: "42: 100",
			value:  map[string]any{"42": 100},
		},
		{
			name:   "key/int-into-int-key",
			source: "42: 100",
			value:  map[int]any{42: 100},
		},
		{
			name:   "plain/leading-dash",
			source: "a: -b_c",
			value:  map[string]any{"a": "-b_c"},
		},
		{
			name:   "plain/leading-plus",
			source: "a: +b_c",
			value:  map[string]any{"a": "+b_c"},
		},
		{
			name:   "plain/leading-digits",
			source: "a: 50cent_of_dollar",
			value:  map[string]any{"a": "50cent_of_dollar"},
		},
		{
			name:   "plain/timestamps-into-strings",
			source: "t2: 2018-01-09T10:40:47Z\nt4: 2098-01-09T10:40:47Z\n",
			value: map[string]string{
				"t2": "2018-01-09T10:40:47Z",
				"t4": "2098-01-09T10:40:47Z",
			},
		},
		{
			name:   "plain/duration-into-string",
			source: "a: 3s\n",
			value: map[string]string{
				"a": "3s",
			},
		},
		{
			name:   "plain/angle-brackets",
			source: "a: <foo>\n",
			value:  map[string]string{"a": "<foo>"},
		},
		{
			name:   "plain/dotted-quad",
			source: "a: 1.2.3.4\n",
			value:  map[string]string{"a": "1.2.3.4"},
		},
		{
			name:   "plain/dots-and-slashes",
			source: "v: go test ./...",
			value:  map[string]string{"v": "go test ./..."},
		},
		{
			name:   "plain/dashes-inside",
			source: "v: echo ---",
			value:  map[string]string{"v": "echo ---"},
		},
		{
			name:   "plain/path-with-braces",
			source: "v: /a/{b}",
			value:  map[string]string{"v": "/a/{b}"},
		},
		{
			name:   "plain/indicators-after-digit",
			source: "v: 1[]{},!%?&*",
			value:  map[string]string{"v": "1[]{},!%?&*"},
		},
		{
			name:   "plain/apostrophe",
			source: "v: user's item",
			value:  map[string]string{"v": "user's item"},
		},
		{
			name:   "plain/multibyte",
			source: "v: あいうえお\nv2: かきくけこ",
			value:  map[string]string{"v": "あいうえお", "v2": "かきくけこ"},
		},
	})
}

// decodeBlockScalarCases covers literal and folded block scalars under each line break, and !!binary.
func decodeBlockScalarCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "block/literal-with-dots-line",
			source: "v: |\n  hello\n  ...\n  world\n",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-with-dots-line-crlf",
			source: "v: |\r\n  hello\r\n  ...\r\n  world\r\n",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-with-dots-line-cr",
			source: "v: |\r  hello\r  ...\r  world\r",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-strip-in-sequence",
			source: "v:\n- A\n- |-\n  B\n  C\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-in-sequence-crlf",
			source: "v:\r\n- A\r\n- |-\r\n  B\r\n  C\r\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-in-sequence-cr",
			source: "v:\r- A\r- |-\r  B\r  C\r",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-trailing-blank-lines",
			source: "v:\n- A\n- |-\n  B\n  C\n\n\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence",
			source: "v:\n- A\n- >-\n  B\n  C\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence-crlf",
			source: "v:\r\n- A\r\n- >-\r\n  B\r\n  C\r\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence-cr",
			source: "v:\r- A\r- >-\r  B\r  C\r",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-trailing-blank-lines",
			source: "v:\n- A\n- >-\n  B\n  C\n\n\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name: "binary/plain",
			// A "!!binary" reads as codec.Base64, the encoded text, so a string
			// destination takes that text. Ask for a []byte to get the payload.
			source: "a: !!binary gIGC\n",
			value:  map[string]string{"a": "gIGC"},
		},
		{
			name: "binary/literal",
			// The line breaks RFC 2045 allows are the document's; the value
			// keeps them, and codec.Base64.Canonical takes them out.
			source: "a: !!binary |\n  " + strings.Repeat("kJCQ", 17) + "kJ\n  CQ\n",
			value:  map[string]string{"a": strings.Repeat("kJCQ", 17) + "kJ\nCQ\n"},
		},
	})
}

func TestDecoder_LiteralWithNewLine(t *testing.T) {
	type A struct {
		Node     string `yaml:"b"`
		LastNode string `yaml:"last"`
	}
	tests := []A{
		{
			Node: "hello\nworld",
		},
		{
			Node: "hello\nworld\n",
		},
		{
			LastNode: "hello\nworld",
		},
		{
			LastNode: "hello\nworld\n",
		},
	}
	// struct(want) -> Marshal -> Unmarchal -> struct(got)
	for _, want := range tests {
		bytes, _ := codec.Marshal(want)
		got := A{}
		require.NoError(t, codec.Unmarshal(bytes, &got))
		require.Equal(t, want.Node, got.Node)
		require.Equal(t, want.LastNode, got.LastNode)
	}
}

func TestDecoder_TabCharacterAtRight(t *testing.T) {
	yml := `
- a: [2 , 2] 			
  b: [2 , 2] 			
  c: [2 , 2]`
	var v []map[string][]int
	require.NoError(t, codec.Unmarshal([]byte(yml), &v))
	require.Len(t, v, 1)
	require.Len(t, v[0], 3)
}
