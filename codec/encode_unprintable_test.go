// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestMarshalEscapesWhatAStreamCannotHoldRaw marshals strings holding each kind of character c-printable leaves out,
// and the two a quoted scalar alone can carry, as a value and as a key, with and without UseSingleQuote. Each one is
// double-quoted with an escape, and reads back as it was. Written raw, or single-quoted where there are no escapes,
// it did not.
func TestMarshalEscapesWhatAStreamCannotHoldRaw(t *testing.T) {
	for name, s := range map[string]string{
		"a C0 control":         "a" + string(rune(0x01)) + "b",
		"an escape":            "a" + string(rune(0x1b)) + "b",
		"DEL":                  "a" + string(rune(0x7f)) + "b",
		"a C1 control":         "a" + string(rune(0x9b)) + "b",
		"U+FFFE":               "a" + string(rune(0xfffe)) + "b",
		"U+FFFF":               "a" + string(rune(0xffff)) + "b",
		"a carriage return":    "a\rb",
		"a byte order mark":    "a" + string(rune(0xfeff)) + "b",
		"over several lines":   "a\n" + string(rune(0x01)) + "\nb",
		"NEL, which may stand": "a" + string(rune(0x85)) + "b",
		"a tab, which may too": "a\tb",
	} {
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
