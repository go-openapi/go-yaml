// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAQuotedScalarMayHoldAnyNonC0Character checks the characters nb-json admits and c-printable does not: DEL, the C1
// controls but NEL, U+FFFE and U+FFFF.
//
// YAML 1.2.2 5.1 has processors "allow all non-C0 characters inside quoted scalars", so that a JSON string holding one
// raw reads as YAML. Everywhere else the stream is c-printable, and the character is refused.
func TestAQuotedScalarMayHoldAnyNonC0Character(t *testing.T) {
	t.Run("read inside a quoted scalar", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{src: "k: \"a\x7fb\"\n", want: "a\x7fb"},
			{src: "k: 'a\x7fb'\n", want: "a\x7fb"},
			{src: "{\"k\": \"a\u0080b\"}\n", want: "a\u0080b"},
			{src: "k: \"a\u009fb\"\n", want: "a\u009fb"},
			{src: "k: \"a\ufffeb\"\n", want: "a\ufffeb"},
			{src: "k: 'a\uffffb'\n", want: "a\uffffb"},
			{src: "k: \"a\n  b\x7fc\"\n", want: "a b\x7fc"},
			{src: "k: \"\x7f\\t\x7f\"\n", want: "\x7f\t\x7f"},
		} {
			var got map[string]string
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got["k"], "%q", tc.src)

			// The AST writes a double-quoted scalar's character as an escape, and leaves a single-quoted one raw, since
			// a single-quoted scalar has no escapes. Either way the text decodes to the same value.
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			var back map[string]string
			require.NoErrorf(t, yaml.Unmarshal([]byte(file.String()), &back), "%q", file.String())
			assert.Equalf(t, tc.want, back["k"], "%q renders as %q", tc.src, file.String())
		}
	})

	t.Run("parsed under a tag the decoder then refuses", func(t *testing.T) {
		// The case the consensus runner found, which libfyaml and the Perl reference parser read. The parse reads it,
		// and decoding it fails later, at the timestamp.
		_, err := parser.ParseBytes([]byte("{k: !!timestamp \"1906-02-07T00:00:\x7f0Z\"} # c1\n"))
		require.NoError(t, err)
	})

	t.Run("refused outside one", func(t *testing.T) {
		for _, src := range []string{
			"k: a\x7fb\n",
			"a\x7f: \"x\"\n",
			"# a\x7fb\nk: v\n",
			"k: v # c\x7f\n",
			"k: |\n  a\x7fb\n",
			"\x7f",
			"k: \"a\"\x7f\n",
			"- \"\x7f\"\n- \x7f\n",
			"k: \"x\x7f\" # c\x7f\n",
			"--- a\n--- b\u0080\n",
			"%TAG !e\x7f! tag:e,2000:\n--- !e\x7f!x \"v\"\n",
			// A root block scalar holding one, ended by a less indented line: the dropped content left the header as the
			// last token, and the block was refused as having no content.
			">\n \x7f\n0\n",
			"|\n a\x7fb\n0\n",
			// C0 stays refused, quoted or not.
			"k: \"a\x01b\"\n",
		} {
			var got any
			assert.ErrorContainsf(t, yaml.Unmarshal([]byte(src), &got), "that a YAML stream may not hold", "%q", src)
		}
	})
}
