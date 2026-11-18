// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// rebuildJSON writes the JSON a run of tokens stands for, putting back the
// separators the token stream leaves out.
func rebuildJSON(toks []JSONToken) []byte {
	var (
		out   []byte
		comma bool
	)
	for _, tk := range toks {
		switch tk.Kind {
		case JSONObjectStart, JSONArrayStart:
			if comma {
				out = append(out, ',')
			}
			if tk.Kind == JSONObjectStart {
				out = append(out, '{')
			} else {
				out = append(out, '[')
			}
			comma = false
		case JSONObjectEnd, JSONArrayEnd:
			if tk.Kind == JSONObjectEnd {
				out = append(out, '}')
			} else {
				out = append(out, ']')
			}
			comma = true
		case JSONKey:
			if comma {
				out = append(out, ',')
			}
			out = appendJSONString(out, tk.Value)
			out = append(out, ':')
			comma = false
		default:
			if comma {
				out = append(out, ',')
			}
			switch tk.Kind {
			case JSONString:
				out = appendJSONString(out, tk.Value)
			case JSONNumber:
				out = append(out, tk.Value...)
			case JSONBool:
				out = strconv.AppendBool(out, tk.Bool)
			default:
				out = append(out, "null"...)
			}
			comma = true
		}
	}

	return out
}

func collectJSONTokens(src []byte, opts ...parser.Option) ([]JSONToken, error) {
	s := ToJSONTokens(src, opts...)
	var toks []JSONToken
	for tk := range s.Tokens() {
		toks = append(toks, tk)
	}

	return toks, s.Err()
}

// TestJSONTokensRebuildWhatToJSONWrites holds ToJSON's text against a rebuild
// of the tokens it is written from, and against JSON itself, over every
// document of the corpus.
//
// ⚠️ It compared two readings of a document until ToJSON became one of them.
// ToJSON walked the document, recorded the text each anchor wrote and answered
// a merge by reading its own output back, where the token converter follows
// ast.AliasNode.Target and asks ast.MergeOf; that walk is gone. What is left to
// compare is the separators -- the commas and colons no token carries --
// written twice, in rebuildJSON here and in appendJSONToken.
//
// json.Valid is the assertion that outlived the two readings. ToJSON used to
// write text that is no JSON document at all for two shapes, and both were
// skipped here rather than asserted: an explicit "? <<" merge key, which wrote
// a key with no value, and an anchor the parse read as a node of its own, which
// wrote two root values. The emitter refuses the second and names the first
// "<<", so every document ToJSON accepts is a JSON document and the skip is
// gone.
func TestJSONTokensRebuildWhatToJSONWrites(t *testing.T) {
	var compared, refused int

	for _, src := range corpusSources() {
		want, wantErr := ToJSON([]byte(src.text))
		toks, gotErr := collectJSONTokens([]byte(src.text))

		if wantErr != nil {
			if assert.Errorf(t, gotErr, "%s: ToJSON refused %q and the tokens did not", src.name, src.text) {
				// One emitter refuses once, so the two carry one message. They
				// refused 97 corpus documents with different messages while
				// each walked the document itself -- "a mapping entry holds one
				// value" against the parse error that followed it.
				assert.Equalf(t, wantErr.Error(), gotErr.Error(), "%s: %q", src.name, src.text)
			}
			refused++

			continue
		}
		if !assert.NoErrorf(t, gotErr, "%s: ToJSON converted %q and the tokens did not", src.name, src.text) {
			continue
		}
		assert.Truef(t, json.Valid(want),
			"%s: ToJSON wrote %q for %q, which is not a JSON document", src.name, want, src.text)

		assert.Equalf(t, string(want), string(rebuildJSON(toks)), "%s: %q", src.name, src.text)
		compared++
	}

	t.Logf("%d documents converted alike, %d refused alike", compared, refused)
}

// TestATagDecidesWhetherAnInfinityConverts holds both converters to one answer
// for an infinity or a NaN under a tag.
//
// JSON has no number for either, so an untagged ".inf" and a "!!float .inf" are
// refused. A tag that makes the scalar a string leaves JSON a spelling for it:
// "k: !!str .inf" is the string ".inf", exactly as "k: \".inf\"" is. A "!!int"
// cannot read ".inf" and is refused as the decoder refuses it, unless
// parser.WithLaxTags keeps the text.
func TestATagDecidesWhetherAnInfinityConverts(t *testing.T) {
	convert := func(t *testing.T, src string, opts ...parser.Option) (string, string) {
		t.Helper()

		out, err := ToJSON([]byte(src), opts...)
		toks, tokErr := collectJSONTokens([]byte(src), opts...)
		if err != nil {
			require.Errorf(t, tokErr, "ToJSON refused %q and the tokens did not", src)

			return err.Error(), tokErr.Error()
		}
		require.NoErrorf(t, tokErr, "ToJSON converted %q and the tokens did not", src)

		return string(out), string(rebuildJSON(toks))
	}

	t.Run("a string tag converts", func(t *testing.T) {
		for src, want := range map[string]string{
			"k: !!str .inf\n":           `{"k":".inf"}`,
			"k: !!str -.inf\n":          `{"k":"-.inf"}`,
			"k: !!str .nan\n":           `{"k":".nan"}`,
			"k: !!str &x .inf\n":        `{"k":".inf"}`,
			"k: &x !!str .inf\n":        `{"k":".inf"}`,
			"!!str .inf: v\n":           `{".inf":"v"}`,
			"{!!str .inf: v}\n":         `{".inf":"v"}`,
			"[!!str .nan]\n":            `[".nan"]`,
			"k: !a .inf\n":              `{"k":".inf"}`,
			"k: \".inf\"\n":             `{"k":".inf"}`,
			"k: !!str &x .inf\nj: *x\n": `{"k":".inf","j":".inf"}`,
		} {
			t.Run(src, func(t *testing.T) {
				fromBytes, fromTokens := convert(t, src)
				assert.Equal(t, want, fromBytes)
				assert.Equal(t, want, fromTokens)
			})
		}
	})

	t.Run("a number is refused", func(t *testing.T) {
		for src, want := range map[string]string{
			"k: .inf\n":            "JSON has no number for .inf",
			"k: !!float .inf\n":    "JSON has no number for .inf",
			"k: !!float -.inf\n":   "JSON has no number for -.inf",
			"k: !!float &x .nan\n": "JSON has no number for .nan",
			"k: &x !!float .nan\n": "JSON has no number for .nan",
			"!!float .inf: v\n":    "JSON has no number for .inf",
			"k: !!int .inf\n":      `cannot read ".inf" as !!int`,
		} {
			t.Run(src, func(t *testing.T) {
				fromBytes, fromTokens := convert(t, src)
				assert.Contains(t, fromBytes, want)
				assert.Contains(t, fromTokens, want)
			})
		}
	})

	t.Run("and WithLaxTags keeps the text", func(t *testing.T) {
		fromBytes, fromTokens := convert(t, "k: !!int .inf\n", parser.WithLaxTags())
		assert.Equal(t, `{"k":".inf"}`, fromBytes)
		assert.Equal(t, `{"k":".inf"}`, fromTokens)
	})
}
