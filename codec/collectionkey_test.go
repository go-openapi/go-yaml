// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestTwoCollectionKeysAreTwoKeys is the key-identity rule where the key is not
// a scalar.
//
// Parser.mapKeyIdentity names a key by its text and its type, and unwraps the
// wrappers it knows -- an explicit key, an anchor, a tag. An alias hands back
// nothing on purpose, since the load resolves what it names. A sequence or a
// mapping fell past all of that to the node's own first token, so every
// sequence key was named "[" and every mapping key "{": "{[a]: 1, [b]: 2}" was
// refused as `mapping key "[" already defined`, two keys sharing not one
// character between them.
//
// The block spelling was always read, so the two disagreed as well.
//
// A collection has no name to be had -- two of them repeat a key when their
// contents match, which is a comparison of trees and not of text -- so it hands
// back nothing, as the alias does, and recordKeyOnce records neither.
//
// grammar.NewRecognizer reads every document here. go.yaml.in/yaml/v3 v3.0.5
// and libfyaml 1.0.0b1 both parse them and then refuse to hold a collection as
// a map key, which is a value model declining rather than a syntax verdict.
func TestTwoCollectionKeysAreTwoKeys(t *testing.T) {
	t.Run("two collection keys parse as two, and neither decodes", func(t *testing.T) {
		// The parse is where this rule lives: two collections that differ are
		// two keys, and the check that says so is ast.KeyIdentity's. Reading
		// them into Go is a separate question and the answer is no -- a
		// collection has no text to name an entry by -- so the decode is where
		// they stop, not the parse.
		for _, src := range []string{
			"{[a]: 1, [b]: 2}\n",
			"{{a: 1}: x, {b: 2}: y}\n",
			"? [a]\n: 1\n? [b]\n: 2\n",
			"{[a]: 1, a: 2}\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.NoErrorf(t, err, "%q parses", src)

			var got any
			assert.Errorf(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()),
				"%q read %v", src, got)
		}
	})

	t.Run("one collection key parses and does not decode", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("{[a]: 1}\n"))
		require.NoError(t, err)

		var got any
		assert.Error(t, codec.UnmarshalWithOptions([]byte("{[a]: 1}\n"), &got, codec.UseOrderedMap()))
	})

	t.Run("a collection key repeated is refused, in every spelling", func(t *testing.T) {
		// 913fb19 stopped naming a collection key and so stopped checking it,
		// and the walking reader then folded two entries into one and dropped
		// the first value without a word. The check that commit ran to prove it
		// had not over-reached -- "<<: {a: 1, a: 2}" still refused -- was a
		// repeat INSIDE the key rather than a repeat OF the key, which is why
		// it passed while this went out.
		for _, src := range []string{
			"{{a: 0}: 1, {a: 0}: 2}\n",
			"{[a]: 1, [a]: 2}\n",
			"{[\"\"]: 1, [\"\"]: 2}\n",
			"{[a, b]: 1, [a, b]: 2}\n",
			"? [a]\n: 1\n? [a]\n: 2\n",
			"? {a: 0}\n: 1\n? {a: 0}\n: 2\n",
			"[a]: 1\n[a]: 2\n",
			"? [a]\n: 1\n[a]: 2\n",
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", src, got)
			assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})

	t.Run("and two collections that differ are still two keys", func(t *testing.T) {
		// The name is the document's own spelling, taken from the source rather
		// than from the node: mapKeyIdentity runs before a collection's
		// children are hung on it, so String() renders "[]" and "{}" and every
		// sequence key collided with every other. The suite caught that --
		// spec-example-2-11-mapping-between-sequences has two sequence keys.
		for _, src := range []string{
			"{{a: 0}: 1, {a: 1}: 2}\n",
			"{[a]: 1, [b]: 2}\n",
			"{[a]: 1, [a, b]: 2}\n",
			"{{\"\": 0}: a, {\"\": 1}: b}\n",
			"? [a]\n: 1\n? [b]\n: 2\n",
			"? - Detroit Tigers\n  - Chicago cubs\n: 1\n? [ New York Yankees ]\n: 2\n",
		} {
			// Two keys and not one, which the parse says by accepting: a repeat
			// is refused above and these are not repeats.
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("and a scalar key repeated is still refused", func(t *testing.T) {
		// The skip is for keys mapKeyIdentity gave up on. Everything it can
		// name is recorded, and a key written empty is one of those: a quoted
		// "" is the string, the empty node is "null", and neither is the
		// nothing a collection hands back.
		for _, src := range []string{
			"{a: 1, a: 2}\n",
			"a: 1\na: 2\n",
			"{\"\": 1, \"\": 2}\n",
			"? a\n: 1\n? a\n: 2\n",
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", src, got)
			assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})

	t.Run("and the refusal names both positions", func(t *testing.T) {
		// The guard for a fix that is coming rather than a rule of its own.
		//
		// 65 and 66 want the duplicate check moved to the mapping's close,
		// where every key is built and can be compared by what it resolves to.
		// A check that runs there has the MAPPING in hand and will point at it
		// unless it carries each key's own position along -- and the position
		// is what a user reads. Nothing asserted it: yamlgen's
		// TestFixedARepeatedCollectionKeyIsRefused matches "already defined"
		// and no more, so it passes either way.
		//
		// Both positions are measured, not chosen: the repeat's own token and
		// the token of the entry that first wrote the key.
		for _, tc := range []struct{ src, says string }{
			{"{{a: 0}: 1, {a: 0}: 2}\n", `[1:13] mapping key "{a: 0}" already defined at [1:2]`},
			{"{[a]: 1, [a]: 2}\n", `[1:10] mapping key "[a]" already defined at [1:2]`},
			{"{[\"\"]: 1, [\"\"]: 2}\n", `[1:11] mapping key "[\"\"]" already defined at [1:2]`},
			{"{[a, b]: 1, [a, b]: 2}\n", `[1:13] mapping key "[a, b]" already defined at [1:2]`},
			{"? [a]\n: 1\n? [a]\n: 2\n", `[3:1] mapping key "[a]" already defined at [1:1]`},
			{"? {a: 0}\n: 1\n? {a: 0}\n: 2\n", `[3:1] mapping key "{a: 0}" already defined at [1:1]`},
			{"[a]: 1\n[a]: 2\n", `[2:1] mapping key "[a]" already defined at [1:1]`},
			{"? [a]\n: 1\n[a]: 2\n", `[3:1] mapping key "[a]" already defined at [1:1]`},

			// The scalar spellings, so a change that moves a collection key's
			// position cannot quietly move theirs too.
			{"a: 1\na: 2\n", `[2:1] mapping key "a" already defined at [1:1]`},
			{"{a: 1, a: 2}\n", `[1:8] mapping key "a" already defined at [1:2]`},
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(tc.src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", tc.src, got)
			assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
		}
	})

	t.Run("an empty key of either spelling still reads", func(t *testing.T) {
		for _, src := range []string{"{\"\": 1}\n", ": 1\n"} {
			var got codec.MapSlice
			require.NoErrorf(t,
				codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()), "%q", src)
			assert.Equalf(t, 1, got.Len(), "%q", src)
		}
	})
}
