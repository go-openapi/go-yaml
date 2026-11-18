// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestTimestampFormats records which of the shapes yaml.org/type/timestamp.html
// allows decode into a time.Time, and that the rest are refused.
//
// Refused is the point. A text no layout read came back as the zero time with
// no error, so "2001-12-14 21:59:43.10 -5" -- which that expression allows --
// decoded to 0001-01-01 and nothing said so.
//
// Nothing here is reached by resolution: YAML 1.2's core schema resolves null,
// bool, int, float and str and no timestamp, so a document gets a time.Time by
// being decoded into one.
func TestTimestampFormats(t *testing.T) {
	for _, tc := range []struct {
		text string
		want string
	}{
		{"2001-12-14t21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14T21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10 -05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-15T02:59:43.1Z", "2001-12-15T02:59:43.1Z"},
		{"2001-12-14 21:59:43.10", "2001-12-14T21:59:43.1Z"},
		{"2002-12-14", "2002-12-14T00:00:00Z"},

		// ⚠️ Allowed by the expression and refused here: it writes the zone as
		// "[-+][0-9][0-9]?(:[0-9][0-9])?", and Go's reference layouts spell
		// neither "-5" nor "-05". Refused rather than read as something else.
		{"2001-12-14 21:59:43.10 -5", ""},
		{"2001-12-14 21:59:43.10 -05", ""},

		{"not a date", ""},
		{"", "0001-01-01T00:00:00Z"},
	} {
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
	const stamp = "2001-12-14"
	instant := time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		src  string
		want any
	}{
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
	} {
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
	for src, want := range map[string]string{
		"a: !!timestamp not-a-date\n":   `cannot read "not-a-date" as !!timestamp`,
		"a: !!binary \"not base64!\"\n": `cannot read "not base64!" as !!binary`,
	} {
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
