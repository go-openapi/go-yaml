// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAKeyIsNamedByItsType covers the text a non-string mapping key addresses
// its entry by.
//
// The name is the type's own canonical spelling and not the document's. An
// integer is written in decimal whatever base it was read from, so "0x10" and
// "007" name 16 and 7 -- one key each, however many ways YAML spells it. A
// float always carries a '.' or an exponent, which keeps the floats out of the
// integers' namespace: "1" and "1.0" are an integer and a float, two keys, and
// two names.
//
// keyText wrote fmt.Sprint of the resolved value before this, which spells
// float64(1) and int(1) alike, so "1: a" beside "1.0: b" came out of ToJSON as
// {"1":"a","1":"b"} -- one name twice, which reads back with one of them gone.
func TestAKeyIsNamedByItsType(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want map[string]any
	}{
		// The two keys that started this.
		{src: "1: a\n1.0: b\n", want: map[string]any{"1": "a", "1.0": "b"}},

		// An integer in decimal, whatever base the document wrote.
		{src: "0x10: a\n", want: map[string]any{"16": "a"}},
		{src: "007: a\n", want: map[string]any{"7": "a"}},
		// ⚠️ "-0x1" is a string and not an integer: the core schema writes a
		// hexadecimal as "0x" and digits with no sign in front of the prefix,
		// so it resolves as text and is named by it. That is a separate open
		// question about the resolver, not about naming.
		{src: "-0x1: a\n", want: map[string]any{"-0x1": "a"}},

		// A float in the shortest text that reads back as the same float64,
		// with ".0" where that text has neither a point nor an exponent.
		{src: "1e3: a\n", want: map[string]any{"1000.0": "a"}},
		{src: "1.5e3: a\n", want: map[string]any{"1500.0": "a"}},
		{src: "1.00: a\n", want: map[string]any{"1.0": "a"}},
		{src: "0.1: a\n", want: map[string]any{"0.1": "a"}},

		// And the exponent stays where the shortest text keeps one, so a wide
		// float is not expanded to precision it never had.
		{src: "1e30: a\n", want: map[string]any{"1e+30": "a"}},
		{src: "1e23: a\n", want: map[string]any{"1e+23": "a"}},
		{src: "1e300: a\n", want: map[string]any{"1e+300": "a"}},
		{src: "1e-6: a\n", want: map[string]any{"1e-06": "a"}},

		// Every spelling of a null and of a boolean names one entry.
		{src: "~: a\n", want: map[string]any{"null": "a"}},
		{src: "NULL: a\n", want: map[string]any{"null": "a"}},
		{src: "True: a\n", want: map[string]any{"true": "a"}},
		{src: "FALSE: a\n", want: map[string]any{"false": "a"}},

		// A key written empty is a null, and a quoted empty key is not.
		{src: "\"\": a\nnull: b\n", want: map[string]any{"": "a", "null": "b"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			var decoded map[string]any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			assert.Equal(t, tc.want, decoded, "the decoder")

			var stringKeyed map[string]any
			require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &stringKeyed, codec.UseStringKeys()))
			assert.Equal(t, tc.want, stringKeyed, "UseStringKeys")

			converted, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)

			var fromJSON map[string]any
			require.NoError(t, json.Unmarshal(converted, &fromJSON))
			assert.Equal(t, tc.want, fromJSON, "ToJSON wrote %s", converted)

			// What it wrote holds as many names as the document wrote keys.
			// JSON with one name twice reads back with one of them gone.
			assert.Len(t, fromJSON, len(tc.want), "ToJSON wrote %s", converted)
		})
	}
}

// TestAnInfinityKeyTakesYAMLsSpelling covers the floats that are not finite.
//
// §10.2.1.4 gives ".inf" and ".nan" as the canonical forms, and they read back
// into YAML as the same value. Go's fmt gives "+Inf" and libfyaml gives
// "Infinity", which is JavaScript's spelling and follows no standard.
//
// codec.ToJSON never reaches them: parser.WithJSONCompatible refuses a document
// holding one, since JSON has no number for it either.
func TestAnInfinityKeyTakesYAMLsSpelling(t *testing.T) {
	for src, want := range map[string]string{
		".inf: a\n":  ".inf",
		"-.inf: a\n": "-.inf",
		".nan: a\n":  ".nan",
		".Inf: a\n":  ".inf",
		".NAN: a\n":  ".nan",
	} {
		t.Run(src, func(t *testing.T) {
			var decoded map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &decoded))
			assert.Equal(t, map[string]any{want: "a"}, decoded)

			_, err := codec.ToJSON([]byte(src))
			assert.Error(t, err, "JSON has no number for it")
		})
	}
}
