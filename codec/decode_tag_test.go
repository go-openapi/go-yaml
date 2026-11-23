// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestIssue617(t *testing.T) {
	data := `
a: !Not [!Equals [!Ref foo, 'bar']]
`
	var v map[string][]any
	require.NoError(t, codec.Unmarshal([]byte(data), &v))
	require.Equal(t, map[string][]any{
		"a": {[]any{"foo", "bar"}},
	}, v)
}

func TestIssue735(t *testing.T) {
	type Slice struct {
		Items []string `yaml:"items"`
	}
	type Map struct {
		Items map[string]string `yaml:"items"`
	}

	t.Run("tag on scalar decoded as slice errors", func(t *testing.T) {
		// A YAML tag applied to a non-sequence value on a slice field must
		// return an error, not panic.
		var v Slice
		require.Error(t, codec.Unmarshal([]byte("items:\n  ! scalar"), &v))
	})

	t.Run("tag on scalar decoded as map errors", func(t *testing.T) {
		// A YAML tag applied to a non-map value on a map field must
		// return an error, not panic.
		var v Map
		require.Error(t, codec.Unmarshal([]byte("items: !!str scalar"), &v))
	})

	t.Run("!!seq tag on sequence decoded as slice succeeds", func(t *testing.T) {
		var v Slice
		require.NoError(t, codec.Unmarshal([]byte("items: !!seq\n  - a\n  - b"), &v))
		require.Equal(t, []string{"a", "b"}, v.Items)
	})

	t.Run("!!map tag on mapping decoded as map succeeds", func(t *testing.T) {
		var v Map
		require.NoError(t, codec.Unmarshal([]byte("items: !!map\n  a: b"), &v))
		require.Equal(t, "b", v.Items["a"])
	})

	t.Run("!!null tag on slice field yields nil slice", func(t *testing.T) {
		// A tagged null value on a slice field should decode to nil, not error.
		// Exercises the recursive dispatch through TagNode -> NullNode.
		var v Slice
		require.NoError(t, codec.Unmarshal([]byte("items: !!null null"), &v))
		require.Nil(t, v.Items)
	})

	t.Run("anchored tagged scalar aliased to slice field errors", func(t *testing.T) {
		// An anchor whose value is a tagged scalar, aliased into a slice field,
		// must return an error rather than panic via the nil ArrayRange iterator.
		var v Slice
		src := "anchor: &anchor !!str scalar\nitems: *anchor"
		require.Error(t, codec.Unmarshal([]byte(src), &v))
	})

	t.Run("anchored tagged sequence aliased to slice field succeeds", func(t *testing.T) {
		var v Slice
		src := "anchor: &anchor !!seq\n  - a\n  - b\nitems: *anchor"
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		require.Equal(t, []string{"a", "b"}, v.Items)
	})
}

// Data is a named []byte, to hold the conversion rather than only the plain
// type.
type Data []byte

// TestBinaryTagReadsIntoAByteSlice: "!!binary" reads into the one Go type it
// names.
//
// The tag names a sequence of bytes and the decoder builds a []byte for an
// `any`, but decodeSlice read the node as a YAML sequence and reported `string
// was used where sequence is expected` -- while the same document into a
// *string* field gave "hello", the decoded bytes. So the conversion was there
// and the destination the tag is for was the one it could not reach.
func TestBinaryTagReadsIntoAByteSlice(t *testing.T) {
	type box struct {
		B []byte `yaml:"b"`
		D Data   `yaml:"d"`
		S string `yaml:"s"`
		A any    `yaml:"a"`
	}

	t.Run("into a field, a named type, a slice and a map", func(t *testing.T) {
		var got box
		require.NoError(t, codec.Unmarshal([]byte("b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"), &got))
		assert.Equal(t, box{B: []byte("hello"), D: Data("world")}, got)

		var raw []byte
		require.NoError(t, codec.Unmarshal([]byte("!!binary aGVsbG8=\n"), &raw))
		assert.Equal(t, []byte("hello"), raw)

		var byName map[string][]byte
		require.NoError(t, codec.Unmarshal([]byte("b: !!binary aGVsbG8=\n"), &byName))
		assert.Equal(t, map[string][]byte{"b": []byte("hello")}, byName)

		var items [][]byte
		require.NoError(t, codec.Unmarshal([]byte("- !!binary aGVsbG8=\n"), &items))
		assert.Equal(t, [][]byte{[]byte("hello")}, items)
	})

	t.Run("through an anchor and through an alias", func(t *testing.T) {
		var got box
		require.NoError(t, codec.Unmarshal([]byte("a: &x !!binary aGVsbG8=\nb: *x\n"), &got))
		assert.Equal(t, []byte("hello"), got.B)
	})

	t.Run("the tag's own default is an empty slice", func(t *testing.T) {
		var got box
		require.NoError(t, codec.Unmarshal([]byte("b: !!binary\n"), &got))
		assert.Equal(t, []byte{}, got.B)
	})

	t.Run("a string field and an any hold the encoded text", func(t *testing.T) {
		// An any holds a codec.Base64, the base64 the document wrote: it is
		// comparable, so a "!!binary" can key a mapping, and it tells the
		// encoder the value is binary so a round trip keeps the tag. Bytes
		// gives the payload. A string field takes the text for the same reason
		// -- the caller asked for text, and the text is base64.
		var got box
		require.NoError(t, codec.Unmarshal([]byte("s: !!binary aGVsbG8=\na: !!binary aGVsbG8=\n"), &got))
		assert.Equal(t, "aGVsbG8=", got.S)
		assert.Equal(t, codec.Base64("aGVsbG8="), got.A)

		raw, err := got.A.(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("and a sequence of numbers still reads into a byte slice", func(t *testing.T) {
		var got box
		require.NoError(t, codec.Unmarshal([]byte("b: [104, 101]\n"), &got))
		assert.Equal(t, []byte{104, 101}, got.B)
	})

	t.Run("a text that is not base64 is still refused", func(t *testing.T) {
		var got box
		err := codec.Unmarshal([]byte("b: !!binary not base64!\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "!!binary")
	})

	t.Run("the walking reader and the tree agree", func(t *testing.T) {
		const src = "b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"

		var walked, treed box
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		require.NoError(t, codec.Unmarshal([]byte(src), &treed, codec.UseOrderedMap(), treeOnly()))
		assert.Equal(t, treed, walked)
	})
}

// TestBinaryTagReadsIntoAByteArray: "!!binary" fills a byte array whose length matches the decoded bytes, on both
// decode paths, and a length the array does not have is an error. A sequence of numbers still fills an array.
func TestBinaryTagReadsIntoAByteArray(t *testing.T) {
	type box struct {
		B [5]byte `yaml:"b"`
	}

	for path := range decodePaths() {
		t.Run(path.name, func(t *testing.T) {
			t.Run("into a field", func(t *testing.T) {
				var got box
				require.NoError(t, codec.Unmarshal([]byte("b: !!binary aGVsbG8=\n"), &got, path.opts...))
				assert.Equal(t, [5]byte{'h', 'e', 'l', 'l', 'o'}, got.B)
			})

			t.Run("into a bare array", func(t *testing.T) {
				var got [5]byte
				require.NoError(t, codec.Unmarshal([]byte("!!binary aGVsbG8=\n"), &got, path.opts...))
				assert.Equal(t, [5]byte{'h', 'e', 'l', 'l', 'o'}, got)
			})

			t.Run("into the arrays of a sequence", func(t *testing.T) {
				var got [][5]byte
				require.NoError(t, codec.Unmarshal([]byte("- !!binary aGVsbG8=\n- !!binary d29ybGQ=\n"), &got, path.opts...))
				assert.Equal(t, [][5]byte{{'h', 'e', 'l', 'l', 'o'}, {'w', 'o', 'r', 'l', 'd'}}, got)
			})

			t.Run("a sequence of numbers still fills an array", func(t *testing.T) {
				var got [2]byte
				require.NoError(t, codec.Unmarshal([]byte("[104, 101]\n"), &got, path.opts...))
				assert.Equal(t, [2]byte{104, 101}, got)
			})

			t.Run("a length the array does not have is an error", func(t *testing.T) {
				var got box
				err := codec.Unmarshal([]byte("b: !!binary aGk=\n"), &got, path.opts...)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "a 5 byte array cannot hold the 2 bytes")
				assert.Equal(t, [5]byte{}, got.B)
			})
		})
	}
}

// decodePath names a decode path and the options that select it.
type decodePath struct {
	name string
	opts []codec.DecodeOption
}

// decodePaths returns the walking reader and the tree decoder, which treeOnly selects. The tree path also sets
// UseOrderedMap, so an untyped mapping decodes into a MapSlice there.
func decodePaths() iter.Seq[decodePath] {
	return slices.Values([]decodePath{
		{name: "walk"},
		{name: "tree", opts: []codec.DecodeOption{codec.UseOrderedMap(), treeOnly()}},
	})
}

// TestAnIntTagReadsIntoEveryIntegerField decodes a number tagged "!!int" into signed and unsigned integer fields on
// both decode paths, as the same number without a tag reads.
func TestAnIntTagReadsIntoEveryIntegerField(t *testing.T) {
	type ints struct {
		I   int    `yaml:"i"`
		I8  int8   `yaml:"i8"`
		I64 int64  `yaml:"i64"`
		U   uint   `yaml:"u"`
		U8  uint8  `yaml:"u8"`
		U64 uint64 `yaml:"u64"`
	}
	const src = "i: !!int -5\ni8: !!int -5\ni64: !!int -5\nu: !!int 5\nu8: !!int 5\nu64: !!int 5\n"

	for path := range decodePaths() {
		t.Run(path.name, func(t *testing.T) {
			var got ints
			require.NoError(t, codec.Unmarshal([]byte(src), &got, path.opts...))
			assert.Equal(t, ints{I: -5, I8: -5, I64: -5, U: 5, U8: 5, U64: 5}, got)
		})
	}
}

// TestABinaryTagReadsAsTheTextItCarries: "!!binary" decodes to [codec.Base64],
// the base64 the document wrote, with the bytes a method call away.
//
// It resolved to []byte before. A []byte cannot key a mapping -- Go hashes no
// slice -- so "? !!binary" was refused; it carries no sign of being binary, so
// an encode wrote a plain sequence of numbers and the tag was gone; and ToJSON
// wrote the bytes as a JSON array, which the decoder then disagreed with. A
// string fixes the first, and only a named type fixes the other two.
func TestABinaryTagReadsAsTheTextItCarries(t *testing.T) {
	const src = "a: !!binary aGVsbG8=\n"

	t.Run("an any holds the text, and Bytes gives the payload", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": codec.Base64("aGVsbG8=")}, got)

		raw, err := got.(map[string]any)["a"].(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("it keys a mapping, where a []byte could not", func(t *testing.T) {
		var byKey map[any]any
		require.NoError(t, codec.Unmarshal([]byte("? !!binary aGVsbG8=\n: x\n"), &byKey))
		assert.Equal(t, map[any]any{codec.Base64("aGVsbG8="): "x"}, byKey)
	})

	t.Run("a round trip keeps the tag", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, src, string(out))
	})

	t.Run("JSON gets the base64 string, whichever path wrote it", func(t *testing.T) {
		folded, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		through, err := codec.Marshal(got, codec.JSON())
		require.NoError(t, err)
		assert.JSONEq(t, string(folded), string(through))

		// And the same thing encoding/json writes for a []byte, which is what
		// a JSON reader means by binary.
		std, err := json.Marshal(map[string][]byte{"a": []byte("hello")})
		require.NoError(t, err)
		assert.JSONEq(t, string(std), string(folded))
	})

	t.Run("a []byte destination still gets the payload", func(t *testing.T) {
		var into struct {
			A []byte `yaml:"a"`
		}
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, []byte("hello"), into.A)
	})

	t.Run("the line breaks RFC 2045 allows are kept, and Canonical takes them out", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("a: !!binary |\n  aGVs\n  bG8=\n"), &got))

		b, ok := got.(map[string]any)["a"].(codec.Base64)
		require.True(t, ok)
		assert.Equal(t, codec.Base64("aGVs\nbG8=\n"), b, "the document's own spelling")
		assert.Equal(t, "aGVsbG8=", b.Canonical())

		raw, err := b.Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)

		// JSON and a re-encode both write the canonical form.
		folded, err := codec.ToJSON([]byte("a: !!binary |\n  aGVs\n  bG8=\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, "a: !!binary aGVsbG8=\n", string(out))
	})

	t.Run("String is the text and not the payload", func(t *testing.T) {
		// A Stringer is what fmt reaches for, and printing the payload would
		// write whatever bytes it happens to hold.
		assert.Equal(t, "aGVsbG8=", codec.Base64("aGVsbG8=").String())
	})

	t.Run("Bytes reports a text that is not base64", func(t *testing.T) {
		_, err := codec.Base64("not base64!").Bytes()
		assert.Error(t, err)
	})
}

// TestTimestampFormats records which texts decode into a time.Time, and that the rest return an error.
//
// The texts accepted are the ones the expression at yaml.org/type/timestamp.html accepts.
// An error is the point of the refused cases: a text no format read used to come back as the zero time, with no error.
//
// Nothing here is reached by resolution: YAML 1.2's core schema resolves null,
// bool, int, float and str and no timestamp, so a document gets a time.Time by
// being decoded into one.
func TestTimestampFormats(t *testing.T) {
	for tc := range timestampFormatsCases() {
		t.Run(tc.text, func(t *testing.T) {
			var into struct {
				A time.Time `yaml:"a"`
			}
			err := codec.Unmarshal([]byte("a: "+tc.text+"\n"), &into)

			if tc.want == "" {
				assert.Errorf(t, err, "read as %v", into.A)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, into.A.Format(time.RFC3339Nano))
		})
	}
}

// timestampFormatsCase holds a timestamp text and the instant it decodes to, or "" when it is refused.
type timestampFormatsCase struct {
	text string
	want string
}

func timestampFormatsCases() iter.Seq[timestampFormatsCase] {
	return slices.Values([]timestampFormatsCase{
		{"2001-12-14t21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14T21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10 -05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-15T02:59:43.1Z", "2001-12-15T02:59:43.1Z"},
		{"2001-12-14 21:59:43.10", "2001-12-14T21:59:43.1Z"},
		{"2002-12-14", "2002-12-14T00:00:00Z"},
		{"2001-01-02", "2001-01-02T00:00:00Z"},
		{"2001-12-14 21:59:43.10 -5", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10 -05", "2001-12-14T21:59:43.1-05:00"},
		{"2015-02-03T03:04:05Z", "2015-02-03T03:04:05Z"},
		{"2015-2-3T3:04:05Z", "2015-02-03T03:04:05Z"},
		{"2001-12-14T21:59:43.Z", "2001-12-14T21:59:43Z"},

		// go.yaml.in/yaml/v3 reads these two, and the expression does not. A date alone takes two-digit month and day
		// fields, and a minute or a second takes two digits. v3's layouts use Go's one-or-two-digit fields "1", "2",
		// "4" and "5", so they accept more than the expression its comment cites.
		{"2001-1-2", ""},
		{"2015-2-3T3:4:5Z", ""},

		{"not a date", ""},
		{"", "0001-01-01T00:00:00Z"},
	})
}

// TestATimestampResolvesUnderTheVersionTheDocumentDeclares records which
// version reads a plain timestamp as one.
//
// tag:yaml.org,2002:timestamp is a YAML 1.1 type. 1.2's core schema resolves
// null, bool, int, float and str and no timestamp, so under 1.2 a plain
// "2002-12-14" is the string it looks like and only a Go field of type
// time.Time asks for the conversion. Under a "%YAML 1.1" directive, or
// parser.WithYAMLVersion at 1.1, it resolves -- the same rule the merge key
// follows, and Fred's ruling of 2026-09-07: resolution follows the version the
// document declares.
//
// The parse marks the tag implicit, so the document renders back as it was
// written and every consumer of types reads one answer: an `any` holds a
// time.Time and ToJSON writes the instant, exactly as they do for a written
// "!!timestamp".
func TestATimestampResolvesUnderTheVersionTheDocumentDeclares(t *testing.T) {
	t.Run("1.2 leaves it a string", func(t *testing.T) {
		const src = "a: 2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])

		got, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"2002-12-14"}`, string(got))
	})

	t.Run("1.1 resolves it", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: 2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, time.Time{}, v.(map[string]any)["a"])

		got, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"2002-12-14T00:00:00Z"}`, string(got))
	})

	t.Run("and the document renders back as it was written", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: 2002-12-14\n"

		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, src, f.String(), "the implicit tag is not a tag the document holds")
	})

	// A block scalar is tag:yaml.org,2002:str whatever it spells (10.2.1.2),
	// and the scanner cuts its content as a plain String token like any other,
	// so this is the shape that has to be told apart from a plain scalar. The
	// generated suite found it: parsing refused the document outright, since
	// parseLiteral requires a StringNode for its content.
	t.Run("a block scalar stays a string", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: >-\n  2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])
	})

	t.Run("a quoted scalar stays a string", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: \"2002-12-14\"\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])
	})
}

// TestAWrittenTagKeepsATimestampAString holds that a tag written on a plain scalar stops it resolving as a timestamp.
//
// Resolution types an untagged scalar. A written tag types the scalar itself,
// and a tag that resolves to nothing, the local "!a" or the non-specific "!", leaves it the text it was written with.
// So under %YAML 1.1 "!a 2001-12-14" is the string "2001-12-14", as libfyaml and go.yaml.in/yaml/v3 read it,
// and an anchor on either side of the tag changes nothing.
//
// A tag on a collection types the collection and not its members, so a plain timestamp inside one still resolves.
func TestAWrittenTagKeepsATimestampAString(t *testing.T) {
	for tc := range aWrittenTagKeepsATimestampAStringCases() {
		t.Run(tc.src, func(t *testing.T) {
			src := "%YAML 1.1\n---\n" + tc.src

			var v any
			require.NoError(t, codec.Unmarshal([]byte(src), &v))
			assert.Equal(t, tc.want, v)
		})
	}

	t.Run("and ToJSON writes the text", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("%YAML 1.1\n---\nk: !a 2001-12-14\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"k":"2001-12-14"}`, string(got))
	})

	t.Run("and the document renders as it was written", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n!a &x 2001-12-14\n"

		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, src, f.String())
	})
}

// aWrittenTagKeepsATimestampAStringCase holds a YAML 1.1 document body and the value it decodes to.
type aWrittenTagKeepsATimestampAStringCase struct {
	src  string
	want any
}

func aWrittenTagKeepsATimestampAStringCases() iter.Seq[aWrittenTagKeepsATimestampAStringCase] {
	const stamp = "2001-12-14"
	instant := time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC)

	return slices.Values([]aWrittenTagKeepsATimestampAStringCase{
		{"!a 2001-12-14\n", stamp},
		{"! 2001-12-14\n", stamp},
		{"!a\n2001-12-14\n", stamp},
		{"!a &x 2001-12-14\n", stamp},
		{"&x !a 2001-12-14\n", stamp},
		{"!!str &x 2001-12-14\n", stamp},
		{"k: !a 2001-12-14\n", map[string]any{"k": stamp}},
		{"k: !a &x 2001-12-14\nj: *x\n", map[string]any{"k": stamp, "j": stamp}},
		{"{k: !a 2001-12-14}\n", map[string]any{"k": stamp}},
		{"!a 2001-12-14: v\n", map[string]any{stamp: "v"}},
		{"? !a 2001-12-14\n: v\n", map[string]any{stamp: "v"}},
		{"{!a 2001-12-14: v}\n", map[string]any{stamp: "v"}},

		// The untagged entry beside it resolves.
		{"- !a 2001-12-14\n- 2001-12-14\n", []any{stamp, instant}},
		{"[!a 2001-12-14, 2001-12-14]\n", []any{stamp, instant}},

		// A tag on the collection leaves its members to resolution.
		{"!a\n2001-12-14: v\n", map[any]any{instant: "v"}},
		{"!!seq\n- 2001-12-14\n", []any{instant}},

		// A written "!!timestamp" is a timestamp.
		{"&x !!timestamp 2001-12-14\n", instant},
	})
}

// TestTimestampTagDoesNotFollowTheVersion records that an explicit
// "!!timestamp" resolves under every YAML version.
//
// WithYAMLVersion selects the schema an untagged plain scalar resolves by, and
// the versions disagree about several of those. A tag is not resolution: it
// names tag:yaml.org,2002:timestamp, which the 2005 type repository defines and
// which means the same thing whichever version the document declares. See
// TestATimestampResolvesUnderTheVersionTheDocumentDeclares for the untagged
// half of the rule.
func TestTimestampTagDoesNotFollowTheVersion(t *testing.T) {
	for _, src := range []string{
		"a: !!timestamp 2002-12-14\n",
		"%YAML 1.1\n---\na: !!timestamp 2002-12-14\n",
		"%YAML 1.2\n---\na: !!timestamp 2002-12-14\n",
		"a: !<tag:yaml.org,2002:timestamp> 2002-12-14\n",
	} {
		var v map[string]any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &v), "%q", src)
		assert.IsTypef(t, time.Time{}, v["a"], "%q", src)
	}
}

// TestATimestampIntoAStringIsWrittenInRFC3339 holds that a timestamp decoded
// into a string kind is the RFC 3339 text of its instant, in its own zone.
//
// A timestamp key into a map keyed by string has that name, and ToJSON writes
// that text. A string field used to refuse the time.Time, so "name: 2001-12-14"
// failed under YAML 1.1 and "name: !!timestamp 2001-12-14" under every version.
// An encoding.TextUnmarshaler is handed the same text as a string field: it
// used to read the scalar again as a YAML 1.2 document of its own, so it got
// the source text under 1.1 and a "!!timestamp" it could not read came back as
// a message about GoYAMLUnmarshaler.
func TestATimestampIntoAStringIsWrittenInRFC3339(t *testing.T) {
	type day string
	type fields struct {
		S  string            `yaml:"s"`
		D  day               `yaml:"d"`
		P  *string           `yaml:"p"`
		M  map[string]string `yaml:"m"`
		L  []string          `yaml:"l"`
		TU textOnly          `yaml:"tu"`
	}
	read := func(t *testing.T, version parser.YAMLVersion, value string) fields {
		t.Helper()
		src := "s: " + value + "\nd: " + value + "\np: " + value + "\nm: {k: " + value +
			"}\nl: [" + value + "]\ntu: " + value + "\n"
		var v fields
		require.NoError(t, codec.Unmarshal([]byte(src), &v,
			codec.WithParserOptions(parser.WithYAMLVersion(version))))

		return v
	}
	written := func(text string) fields {
		return fields{
			S: text, D: day(text), P: &text, M: map[string]string{"k": text}, L: []string{text},
			TU: textOnly{text: text},
		}
	}

	for tc := range aTimestampIntoAStringIsWrittenInRFC3339Cases() {
		t.Run(string(tc.version)+"/"+tc.value, func(t *testing.T) {
			assert.Equal(t, written(tc.want), read(t, tc.version, tc.value))
		})
	}

	t.Run("a tag that is no timestamp is refused", func(t *testing.T) {
		for tc := range aTimestampIntoAStringRefusedFieldCases() {
			field, at := tc.field, tc.at
			var v fields
			err := codec.Unmarshal([]byte(field+": !!timestamp nope\n"), &v)
			require.Error(t, err, field)
			assert.Contains(t, err.Error(), at+` cannot read "nope" as !!timestamp`, field)
		}
	})
}

// aTimestampIntoAStringIsWrittenInRFC3339Case holds a YAML version, a scalar and the text a string field reads from it.
type aTimestampIntoAStringIsWrittenInRFC3339Case struct {
	version parser.YAMLVersion
	value   string
	want    string
}

func aTimestampIntoAStringIsWrittenInRFC3339Cases() iter.Seq[aTimestampIntoAStringIsWrittenInRFC3339Case] {
	const (
		midnight = "2001-12-14T00:00:00Z"
		zoned    = "2001-12-14T21:59:43.1-05:00"
	)

	return slices.Values([]aTimestampIntoAStringIsWrittenInRFC3339Case{
		{parser.YAML11, "2001-12-14", midnight},
		{parser.YAML11, "2001-12-14t21:59:43.10-05:00", zoned},
		{parser.YAML11, "!!timestamp 2001-12-14", midnight},
		{parser.YAML11, `!!timestamp "2001-12-14"`, midnight},
		{parser.YAML12, "!!timestamp 2001-12-14", midnight},
		{parser.YAML12, "!!timestamp 2001-12-14t21:59:43.10-05:00", zoned},
		// YAML 1.2 resolves no timestamp, so the text stands as written.
		{parser.YAML12, "2001-12-14", "2001-12-14"},
		{parser.YAML12, "2001-12-14t21:59:43.10-05:00", "2001-12-14t21:59:43.10-05:00"},
		// A quoted scalar is a string under every version.
		{parser.YAML11, `"2001-12-14"`, "2001-12-14"},
	})
}

// aTimestampIntoAStringRefusedFieldCase holds a field and the position its error names.
type aTimestampIntoAStringRefusedFieldCase struct {
	field string
	at    string
}

func aTimestampIntoAStringRefusedFieldCases() iter.Seq[aTimestampIntoAStringRefusedFieldCase] {
	return slices.Values([]aTimestampIntoAStringRefusedFieldCase{
		{field: "p", at: "[1:16]"},
		{field: "s", at: "[1:16]"},
		{field: "tu", at: "[1:17]"},
	})
}

// textOnly reads itself through encoding.TextUnmarshaler and nothing else.
type textOnly struct{ text string }

func (x *textOnly) UnmarshalText(b []byte) error {
	x.text = string(b)

	return nil
}

// TestATagThatCannotConvertIsRefused records that a scalar no format reads is
// an error rather than a zero value.
//
// Both conversions used to discard it -- "!!timestamp not-a-date" came back as
// 0001-01-01 and "!!binary" on a text base64 cannot read came back as an empty
// []byte, each with a nil error. go.yaml.in/yaml/v3 and gopkg.in/yaml.v2 both
// refuse them.
//
// The complaint now names the tag rather than the type it stands for, because
// ast.TagNode.Resolve makes it for every tag alike and codec.ToJSON reports the
// same one: the converter used to write "not-a-date" through as a string while
// the decoder refused it.
func TestATagThatCannotConvertIsRefused(t *testing.T) {
	for tc := range aTagThatCannotConvertIsRefusedCases() {
		src, want := tc.src, tc.want
		var v any
		err := codec.Unmarshal([]byte(src), &v)
		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), want, "%q", src)
	}

	t.Run("and the tag on nothing takes its own default", func(t *testing.T) {
		for _, src := range []string{"a: !!timestamp\nb: 1\n", "a: !!timestamp null\n"} {
			var v map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &v), "%q", src)
			assert.Equalf(t, time.Time{}, v["a"], "%q", src)
		}
	})
}

// aTagThatCannotConvertIsRefusedCase holds a document and the text its error contains.
type aTagThatCannotConvertIsRefusedCase struct {
	src  string
	want string
}

func aTagThatCannotConvertIsRefusedCases() iter.Seq[aTagThatCannotConvertIsRefusedCase] {
	return slices.Values([]aTagThatCannotConvertIsRefusedCase{
		{src: "a: !!binary \"not base64!\"\n", want: `cannot read "not base64!" as !!binary`},
		{src: "a: !!timestamp not-a-date\n", want: `cannot read "not-a-date" as !!timestamp`},
	})
}

// laxDecode reads src with parser.WithLaxTags and decodes the node, which is
// the route a caller takes to hand the policy to the decoder.
func laxDecode(t *testing.T, src string) (any, error) {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)

	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)

	return v, err
}

// TestLaxTagsReadsTheTextTheScalarWasWrittenWith covers the fallback the option
// turns on.
//
// A tag is an assertion about the node under it, and by default one that does
// not hold is reported. Under the option the characters stand in for the value
// the tag could not make of them, which is what the library used to do for
// "!!int" and "!!float" -- except that it answered 0, a value a caller can tell
// neither from a written zero nor from the text that produced it.
func TestLaxTagsReadsTheTextTheScalarWasWrittenWith(t *testing.T) {
	for tc := range laxTagsReadsTheTextTheScalarWasWrittenWithCases() {
		src, want := tc.src, tc.want
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)

			// And the same document is refused without the option.
			var strict any
			assert.Error(t, codec.Unmarshal([]byte(src), &strict), "strict reads it")
		})
	}
}

// laxTagsReadsTheTextTheScalarWasWrittenWithCase holds a document whose tag fails and the value its key reads.
type laxTagsReadsTheTextTheScalarWasWrittenWithCase struct {
	src  string
	want any
}

func laxTagsReadsTheTextTheScalarWasWrittenWithCases() iter.Seq[laxTagsReadsTheTextTheScalarWasWrittenWithCase] {
	return slices.Values([]laxTagsReadsTheTextTheScalarWasWrittenWithCase{
		{src: "k: !!binary not base64!\n", want: "not base64!"},
		{src: "k: !!bool 7\n", want: "7"},
		{src: "k: !!float xyz\n", want: "xyz"},
		{src: "k: !!int abc\n", want: "abc"},
		{src: "k: !!null 5\n", want: "5"},
		{src: "k: !!timestamp not-a-date\n", want: "not-a-date"},
	})
}

// TestLaxTagsLeavesEveryOtherAnswerAlone is what says the option relaxes one
// thing and not the reading of tags in general.
func TestLaxTagsLeavesEveryOtherAnswerAlone(t *testing.T) {
	for tc := range laxTagsLeavesEveryOtherAnswerAloneCases() {
		src, want := tc.src, tc.want
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)
		})
	}
}

// laxTagsLeavesEveryOtherAnswerAloneCase holds a document and the value its key reads with the option.
type laxTagsLeavesEveryOtherAnswerAloneCase struct {
	src  string
	want any
}

func laxTagsLeavesEveryOtherAnswerAloneCases() iter.Seq[laxTagsLeavesEveryOtherAnswerAloneCase] {
	return slices.Values([]laxTagsLeavesEveryOtherAnswerAloneCase{
		// A tag naming what the scalar is.
		{src: "k: !!int 5\n", want: uint64(5)},
		{src: "k: !!bool true\n", want: true},
		{src: "k: !!str 0x10\n", want: "0x10"},
		// A tag this library has no rule for is read by its kind, option or no.
		{src: "k: !fred x\n", want: "x"},
		{src: "k: !Ref x\n", want: "x"},
		// A tag standing on no value takes the tag's own default.
		{src: "k: !!bool\n", want: false},
	})
}

// TestLaxTagsKeepsTheTagOnTheNode covers the round trip.
//
// The fallback replaces the value the tag could not make, not the tag: a
// document read laxly renders as it was written, so nothing about reading it
// this way is lost on the way out again.
func TestLaxTagsKeepsTheTagOnTheNode(t *testing.T) {
	for _, src := range []string{
		"k: !!int abc\n",
		"k: !!timestamp not-a-date\n",
		"k: !!bool 7\n",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
			require.NoError(t, err)
			assert.Equal(t, src, f.String())
		})
	}
}

// TestLaxTagsReachesTheJSONConverterToo is the point of hanging the policy on
// the node: two consumers of one document give one answer.
func TestLaxTagsReachesTheJSONConverterToo(t *testing.T) {
	const src = "k: !!int abc\n"

	_, err := codec.ToJSON([]byte(src))
	require.Error(t, err, "strict converts it")

	got, err := codec.ToJSON([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)
	assert.Equal(t, `{"k":"abc"}`, string(got))

	// The decoder reading the same document laxly agrees.
	decoded, err := laxDecode(t, src)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"k": "abc"}, decoded)
}

// TestLaxTagsDoesNotExcuseAKindMismatch holds the one refusal the option leaves
// standing.
//
// A tag naming a kind its node is not is a claim about shape, and no text
// stands in for a sequence. Built by hand because the parser still refuses
// "k: !!seq 5" before a loader sees it.
func TestLaxTagsDoesNotExcuseAKindMismatch(t *testing.T) {
	f, err := parser.ParseBytes([]byte("k: [1, 2]\n"), parser.WithLaxTags())
	require.NoError(t, err)

	entry := f.Docs[0].Body.(*ast.MappingNode).Values[0]
	tag := &ast.TagNode{URI: "tag:yaml.org,2002:str", Value: entry.Value, LaxTags: true}

	res := tag.Resolve()
	require.Equal(t, ast.TagKindMismatch, res.Verdict)
	assert.True(t, res.Lax, "the node carries the policy")

	entry.Value = tag
	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support this kind of node")
}

// TestStrTagKeepsTheText records that "!!str" hands over the text the scalar
// was written with, through both readers.
//
// The scanner types a scalar by the core schema before any tag is seen, so
// "!!str 0x10" arrives as an ast.IntegerNode holding 16 and "!!str False" as
// an ast.BoolNode holding false. Reading the value off those nodes wrote "16"
// and "false"; the tag says the scalar is a string, so its own text is the
// value.
func TestStrTagKeepsTheText(t *testing.T) {
	for tc := range strTagKeepsTheTextCases() {
		t.Run(tc.name, func(t *testing.T) {
			var decoded string
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			assert.Equal(t, tc.want, decoded)

			converted, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.JSONEq(t, `"`+tc.want+`"`, string(converted))
		})
	}
}

// strTagKeepsTheTextCase holds a "!!str" document and the text it decodes to.
type strTagKeepsTheTextCase struct {
	name, src, want string
}

func strTagKeepsTheTextCases() iter.Seq[strTagKeepsTheTextCase] {
	return slices.Values([]strTagKeepsTheTextCase{
		{"hexadecimal", "!!str 0x10", "0x10"},
		{"trailing zero", "!!str 1.50", "1.50"},
		{"capitalized bool", "!!str False", "False"},
		{"null spelled out", "!!str null", "null"},
		{"under an anchor", "!!str &x False", "False"},
	})
}

// TestStrTagOnNothingIsEmpty records that a "!!str" with no scalar after it
// tags the empty string.
//
// The parser puts an implicit null token there, and that token reads "null" --
// which is what the node resolves to, not what the document wrote. Only the
// spelled-out "null" of TestStrTagKeepsTheText is the word.
func TestStrTagOnNothingIsEmpty(t *testing.T) {
	var decoded map[string]any
	require.NoError(t, codec.Unmarshal([]byte("a: !!str\nb: 1\n"), &decoded))
	assert.Equal(t, "", decoded["a"])

	converted, err := codec.ToJSON([]byte("a: !!str\nb: 1\n"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"","b":1}`, string(converted))
}

// TestStrTagAliasesTheString records that an alias naming an anchor under a
// "!!str" gets the string, not what the scalar resolved to on its own.
func TestStrTagAliasesTheString(t *testing.T) {
	var decoded []string
	require.NoError(t, codec.Unmarshal([]byte("- !!str &x 1.50\n- *x\n"), &decoded))
	assert.Equal(t, []string{"1.50", "1.50"}, decoded)
}

// TestTagResolvesToItsURI records that a tag is read by the URI it names and
// not by the shorthand it was written with.
//
// "!!int" is the secondary handle plus the suffix "int", and the secondary
// handle stands for tag:yaml.org,2002: unless a "%TAG" line says otherwise
// (§6.8.2.2). So "!!int", "!<tag:yaml.org,2002:int>" and "!e!int" under
// "%TAG !e! tag:yaml.org,2002:" are three spellings of one tag.
func TestTagResolvesToItsURI(t *testing.T) {
	for tc := range tagResolvesToItsURICases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))

			var decoded any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			asJSON, err := json.Marshal(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(asJSON), "the decoder reads the same tag")
		})
	}
}

// tagResolvesToItsURICase holds a document whose tag is spelled one way and the JSON it reads as.
type tagResolvesToItsURICase struct {
	name, src, want string
}

func tagResolvesToItsURICases() iter.Seq[tagResolvesToItsURICase] {
	return slices.Values([]tagResolvesToItsURICase{
		{"shorthand", `a: !!int "12"` + "\n", `{"a":12}`},
		{"verbatim", "a: !<tag:yaml.org,2002:int> \"12\"\n", `{"a":12}`},
		{"named handle", "%TAG !e! tag:yaml.org,2002:\n---\na: !e!int \"12\"\n", `{"a":12}`},
		{"primary handle repointed", "%TAG ! tag:yaml.org,2002:\n---\na: !int \"12\"\n", `{"a":12}`},
		{"secondary handle restated", "%TAG !! tag:yaml.org,2002:\n---\na: !!int \"12\"\n", `{"a":12}`},

		// ⚠️ And the other way: a "%TAG !!" line takes the secondary handle out
		// of YAML's namespace, so "!!int" names the document's own tag. Nothing
		// resolves it, and the scalar keeps the text it was written with.
		{"secondary handle repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", `{"a":"12"}`},
		{"repointed on a collection", "%TAG !! tag:example.com,2020:\n---\na: !!seq [1,2]\n", `{"a":[1,2]}`},
	})
}

// TestTagNodeCarriesTheURI records that the expansion is on the node, so a
// reader asks what a tag means rather than what it says.
func TestTagNodeCarriesTheURI(t *testing.T) {
	for tc := range tagNodeCarriesTheURICases() {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			var found []string
			for _, doc := range file.Docs {
				collectTagURIs(doc, &found)
			}
			assert.Equal(t, []string{tc.want}, found)
		})
	}
}

// tagNodeCarriesTheURICase holds a document and the URI of the one tag it holds.
type tagNodeCarriesTheURICase struct {
	name, src, want string
}

func tagNodeCarriesTheURICases() iter.Seq[tagNodeCarriesTheURICase] {
	return slices.Values([]tagNodeCarriesTheURICase{
		{"shorthand", "a: !!int 12\n", "tag:yaml.org,2002:int"},
		{"verbatim", "a: !<tag:yaml.org,2002:int> 12\n", "tag:yaml.org,2002:int"},
		{"local", "a: !thing 12\n", "!thing"},
		{"named handle", "%TAG !e! tag:example.com,2020:\n---\na: !e!thing 12\n", "tag:example.com,2020:thing"},
		{"secondary repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", "tag:example.com,2020:int"},
	})
}

// TestUndeclaredTagHandleIsRefused records that "!name!" needs a "%TAG" line.
// The primary and secondary handles never do.
func TestUndeclaredTagHandleIsRefused(t *testing.T) {
	_, err := parser.ParseBytes([]byte("a: !x!thing 12\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag handle !x! is not defined by a TAG directive")
}

// collectTagURIs appends the URI of every ast.TagNode under n, in the order
// they are reached.
func collectTagURIs(n ast.Node, out *[]string) {
	ast.Walk(uriCollector{out: out}, n)
}

type uriCollector struct{ out *[]string }

func (c uriCollector) Visit(n ast.Node) ast.Visitor {
	if tag, ok := n.(*ast.TagNode); ok {
		*c.out = append(*c.out, tag.URI)
	}

	return c
}

// decodeTagCases covers explicit tags, and merge keys read under YAML 1.1, where "<<" is the merge key.
func decodeTagCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "tag/float-on-quoted",
			source: "v: !!float '1.1'",
			value:  map[string]any{"v": 1.1},
		},
		{
			name:   "tag/float-on-zero",
			source: "v: !!float 0",
			value:  map[string]any{"v": float64(0)},
		},
		{
			name:   "tag/float-on-negative-int",
			source: "v: !!float -1",
			value:  map[string]any{"v": float64(-1)},
		},
		{
			name:   "tag/null-on-empty-quoted",
			source: "v: !!null ''",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "tag/timestamp-on-quoted",
			source: "v: !!timestamp \"2015-01-01\"",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:   "tag/timestamp-on-plain",
			source: "v: !!timestamp 2015-01-01",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:   "tag/bool-yes",
			source: "v: !!bool yes",
			value:  map[string]bool{"v": true},
		},
		{
			name:   "tag/bool-capitalized-false",
			source: "v: !!bool False",
			value:  map[string]bool{"v": false},
		},
		{
			name: "tag/merge-on-key",
			source: `
!!merge <<: { a: 1, b: 2 }
c: 3
`,
			value: map[string]any{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "merge/aliases-into-maps",
			source: `%YAML 1.1
---
a: &a
 foo: 1
b: &b
 bar: 2
merge:
 <<: [*a, *b]
`,
			value: map[string]map[string]any{
				"a":     {"foo": 1},
				"b":     {"bar": 2},
				"merge": {"foo": 1, "bar": 2},
			},
		},
		{
			name: "merge/aliases-into-map-slices",
			source: `%YAML 1.1
---
a: &a
 foo: 1
b: &b
 bar: 2
merge:
 <<: [*a, *b]
`,
			value: map[string]codec.MapSlice{
				"a":     mapSliceOf(item("foo", 1)),
				"b":     mapSliceOf(item("bar", 2)),
				"merge": mapSliceOf(item("foo", 1), item("bar", 2)),
			},
		},
	})
}
