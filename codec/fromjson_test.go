// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// TestFromJSONWritesWhatTheJSONWrote converts JSON whose values Go types would change: numbers keep their text, null
// and empty collections stay as they are, member order is kept, and each string is written as Marshal writes it.
func TestFromJSONWritesWhatTheJSONWrote(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"numbers as written": {
			`{"exp": 1e3, "zero": -0, "upper": 1.0E+2, "big": 12345678901234567890123, "huge": 1e400, "frac": 0.10}`,
			"exp: 1e3\nzero: -0\nupper: 1.0E+2\nbig: 12345678901234567890123\nhuge: 1e400\nfrac: 0.10\n",
		},
		// "n" is a bool in YAML 1.1, so Marshal quotes it, and FromJSON with it.
		"null, booleans and empty collections": {
			`{"n": null, "t": true, "f": false, "a": [], "o": {}}`,
			"\"n\": null\nt: true\nf: false\na: []\no: {}\n",
		},
		"member order": {
			`{"b": 1, "a": 2, "c": 3}`,
			"b: 1\na: 2\nc: 3\n",
		},
		"strings plain where they read back": {
			`{"s": "two words", "url": "http://x/y"}`,
			"s: two words\nurl: http://x/y\n",
		},
		"strings quoted where plain text would read as something else": {
			`{"yes": "yes", "one": "1", "null": "null", "empty": "", "lead": " x"}`,
			"\"yes\": \"yes\"\none: \"1\"\n\"null\": \"null\"\nempty: \"\"\nlead: \" x\"\n",
		},
		"a string over several lines": {
			`{"w": "a\nb"}`,
			"w: |-\n  a\n  b\n",
		},
		"nested collections": {
			`{"a": [{"b": [1, [2, 3]]}, {}], "c": {"d": {"e": []}}}`,
			"a:\n- b:\n  - 1\n  - - 2\n    - 3\n- {}\nc:\n  d:\n    e: []\n",
		},
		"a scalar alone":  {`"x"`, "x\n"},
		"a number alone":  {`-1.5e-3`, "-1.5e-3\n"},
		"an array alone":  {`[1, "a"]`, "- 1\n- a\n"},
		"an empty object": {`{}`, "{}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := codec.FromJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

// TestFromJSONReadsBackTheSameJSON converts JSON to YAML and back with ToJSON, which writes numbers as written too.
func TestFromJSONReadsBackTheSameJSON(t *testing.T) {
	for _, src := range []string{
		`{"a":[1,2.5,-0,1e3,12345678901234567890123],"b":{"c":null,"d":true},"e":"x\ny","f":"\u0001\u007f\uffff"}`,
		`[[],{},[[]],{"":""}]`,
		`{"` + strings.Repeat("k", 1100) + `":{"nested":[1]}}`,
		`{"yes":"no","1":"0x1F","~":"- x","#":": y","---":"..."}`,
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
