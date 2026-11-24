// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFromJSONWritesWhatTheJSONWrote converts JSON whose values Go types would change: numbers keep their text, null
// and empty collections stay as they are, member order is kept, and each string is written as Marshal writes it.
func TestFromJSONWritesWhatTheJSONWrote(t *testing.T) {
	for tc := range fromJSONWritesWhatTheJSONWroteCases() {
		t.Run(tc.name, func(t *testing.T) {
			out, err := codec.FromJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

// fromJSONWritesWhatTheJSONWroteCase holds a JSON document and the YAML FromJSON writes for it.
type fromJSONWritesWhatTheJSONWroteCase struct {
	name string
	src  string
	want string
}

func fromJSONWritesWhatTheJSONWroteCases() iter.Seq[fromJSONWritesWhatTheJSONWroteCase] {
	return slices.Values([]fromJSONWritesWhatTheJSONWroteCase{
		{
			name: "numbers as written",
			src:  `{"exp": 1e3, "zero": -0, "upper": 1.0E+2, "big": 12345678901234567890123, "huge": 1e400, "frac": 0.10}`,
			want: "exp: 1e3\nzero: -0\nupper: 1.0E+2\nbig: 12345678901234567890123\nhuge: 1e400\nfrac: 0.10\n",
		},
		// "n" is a bool in YAML 1.1, so Marshal quotes it, and FromJSON with it.
		{
			name: "null, booleans and empty collections",
			src:  `{"n": null, "t": true, "f": false, "a": [], "o": {}}`,
			want: "\"n\": null\nt: true\nf: false\na: []\no: {}\n",
		},
		{
			name: "member order",
			src:  `{"b": 1, "a": 2, "c": 3}`,
			want: "b: 1\na: 2\nc: 3\n",
		},
		{
			name: "strings plain where they read back",
			src:  `{"s": "two words", "url": "http://x/y"}`,
			want: "s: two words\nurl: http://x/y\n",
		},
		{
			name: "strings quoted where plain text would read as something else",
			src:  `{"yes": "yes", "one": "1", "null": "null", "empty": "", "lead": " x"}`,
			want: "\"yes\": \"yes\"\none: \"1\"\n\"null\": \"null\"\nempty: \"\"\nlead: \" x\"\n",
		},
		{
			name: "a string over several lines",
			src:  `{"w": "a\nb"}`,
			want: "w: |-\n  a\n  b\n",
		},
		{
			name: "nested collections",
			src:  `{"a": [{"b": [1, [2, 3]]}, {}], "c": {"d": {"e": []}}}`,
			want: "a:\n- b:\n  - 1\n  - - 2\n    - 3\n- {}\nc:\n  d:\n    e: []\n",
		},
		{name: "a scalar alone", src: `"x"`, want: "x\n"},
		{name: "a number alone", src: `-1.5e-3`, want: "-1.5e-3\n"},
		{name: "an array alone", src: `[1, "a"]`, want: "- 1\n- a\n"},
		{name: "an empty object", src: `{}`, want: "{}\n"},
	})
}

// TestFromJSONReadsBackTheSameJSON converts JSON to YAML and back with ToJSON, which writes numbers as written too.
func TestFromJSONReadsBackTheSameJSON(t *testing.T) {
	for _, src := range []string{
		`{"a":[1,2.5,-0,1e3,12345678901234567890123],"b":{"c":null,"d":true},"e":"x\ny","f":"\u0001\u007f\uffff"}`,
		`[[],{},[[]],{"":""}]`,
		`{"` + strings.Repeat("k", 1100) + `":{"nested":[1]}}`,
		`{"yes":"no","1":"0x1F","~":"- x","#":": y","---":"..."}`,
		// A key opening with "-" or "?" before a tab reads as a sequence or an explicit key written plain.
		`{"-\tZA":null,"x":[{"-\ta\"":[1],"?\tq":"a:\tb"}]}`,
	} {
		yml, err := codec.FromJSON([]byte(src))
		require.NoError(t, err)

		back, err := codec.ToJSON(yml)
		require.NoError(t, err, "%.80q", yml)
		assert.JSONEq(t, src, string(back), "via\n%s", yml)
	}
}

// TestFromJSONRejectsWhatIsNotJSON passes YAML and broken JSON.
func TestFromJSONRejectsWhatIsNotJSON(t *testing.T) {
	for _, src := range []string{"a: b", `{"a": 1,}`, `['a']`, `{"a": 0x1F}`, "", `{"a": 1} # c`} {
		_, err := codec.FromJSON([]byte(src))
		require.ErrorIs(t, err, yamlerrors.ErrNotJSONSyntax, "%q", src)
	}

	_, err := codec.FromJSON([]byte(`{"a": 1, "a": 2}`))
	require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
}

// TestFromJSONLaysTheYAMLOutWithRenderOptions passes the indent options.
func TestFromJSONLaysTheYAMLOutWithRenderOptions(t *testing.T) {
	src := []byte(`{"a": {"b": [1, {"c": 2, "d": 3}]}}`)

	out, err := codec.FromJSON(src, ast.WithIndent(4))
	require.NoError(t, err)
	assert.Equal(t, "a:\n    b:\n    - 1\n    - c: 2\n      d: 3\n", string(out))

	out, err = codec.FromJSON(src, ast.WithIndentSequence(true))
	require.NoError(t, err)
	assert.Equal(t, "a:\n  b:\n    - 1\n    - c: 2\n      d: 3\n", string(out))
}

// TestFromJSONTokensWritesWhatFromJSONWrites feeds FromJSONTokens the tokens ToJSONTokens reads from JSON text, and
// compares the output with FromJSON's for the same text, on the JSON of every YAML Test Suite document that has one.
func TestFromJSONTokensWritesWhatFromJSONWrites(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)

	var compared int
	for _, s := range suites {
		src, err := codec.ToJSON(s.InYAML)
		if err != nil {
			continue
		}
		want, err := codec.FromJSON(src)
		require.NoErrorf(t, err, "%s", src)

		tokens := codec.ToJSONTokens(src, parser.WithJSONSyntax())
		got, err := codec.FromJSONTokens(tokens.Tokens())
		require.NoError(t, tokens.Err())
		require.NoErrorf(t, err, "%s", src)
		assert.Equalf(t, string(want), string(got), "%s", src)
		compared++
	}
	assert.Greater(t, compared, 200)
}

// TestFromJSONTokensRejectsASourceThatStopsEarly hands FromJSONTokens an object that never closes, as a reader stopped
// by an error leaves it.
func TestFromJSONTokensRejectsASourceThatStopsEarly(t *testing.T) {
	t.Parallel()

	stopped := slices.Values([]codec.JSONToken{
		{Kind: codec.JSONObjectStart},
		{Kind: codec.JSONKey, Value: "a"},
		{Kind: codec.JSONNumber, Value: "1"},
	})
	_, err := codec.FromJSONTokens(stopped)
	require.ErrorIs(t, err, ast.ErrStreamOrder)
}
