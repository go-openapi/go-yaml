// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// keyOf is the first key of the document's root mapping.
func keyOf(t *testing.T, src string) ast.Node {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithAllowDuplicateMapKey())
	require.NoErrorf(t, err, "%q", src)
	require.NotEmptyf(t, f.Docs, "%q", src)

	body, ok := f.Docs[len(f.Docs)-1].Body.(*ast.MappingNode)
	if !ok {
		entry, single := f.Docs[len(f.Docs)-1].Body.(*ast.MappingValueNode)
		require.Truef(t, single, "%q holds no mapping", src)

		return entry.Key
	}
	require.NotEmptyf(t, body.Values, "%q", src)

	return body.Values[0].Key
}

// TestKeyIdentityReadsWhatAKeyResolvesTo holds spellings of one key to one
// answer, and keeps different keys apart.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node. The text a
// document wrote is not that: "[a]", "[ a ]", "[a,]" and '["a"]' are four
// spellings of one sequence holding one string. Naming a key by its first token
// is worse -- every sequence key becomes "[", every block scalar key "|-".
func TestKeyIdentityReadsWhatAKeyResolvesTo(t *testing.T) {
	t.Run("spellings of one key agree", func(t *testing.T) {
		for _, group := range [][]string{
			{"[a]: 1\n", "[ a ]: 2\n", "[a,]: 3\n", "[\"a\"]: 4\n", "['a']: 5\n", "[ a , ]: 6\n"},
			{"{a: 1}: x\n", "{ a : 1 }: y\n", "{\"a\": 1}: z\n"},
			{"? - a\n: 1\n", "? [a]\n: 2\n", "?\n  - a\n: 3\n"},
			{"a: 1\n", "\"a\": 2\n", "'a': 3\n"},
			{"1: x\n", "01: y\n", "0x1: z\n"},
			{"? |-\n  a\n: 1\n", "? >-\n  a\n: 2\n", "a: 3\n"},
		} {
			want := ast.KeyIdentity(keyOf(t, group[0]))
			require.NotEmptyf(t, want, "%q has no identity", group[0])
			for _, src := range group[1:] {
				assert.Equalf(t, want, ast.KeyIdentity(keyOf(t, src)),
					"%q and %q spell one key", group[0], src)
			}
		}
	})

	t.Run("different keys stay apart", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"[a]: 1\n", "[b]: 2\n"},
			{"[a]: 1\n", "[a, b]: 2\n"},
			{"[a]: 1\n", "{a: 1}: 2\n"},
			{"[[a]]: 1\n", "[a]: 2\n"},
			{"{a: 1}: x\n", "{a: 2}: y\n"},
			{"{a: 1}: x\n", "{b: 1}: y\n"},
			{"1: x\n", "\"1\": y\n"},
			{"1: x\n", "1.0: y\n"},
			// A tag names the type, so it changes the identity only where the
			// plain spelling resolves to another type: "a" is a string either
			// way, and "1" is an integer plain and a string tagged.
			{"1: x\n", "!!str 1: y\n"},
			// Chomping is part of the string a block scalar holds.
			{"? |-\n  a\n: 1\n", "? |\n  a\n: 2\n"},
			{"? - a\n: 1\n", "? - b\n: 2\n"},
		} {
			left, right := ast.KeyIdentity(keyOf(t, pair[0])), ast.KeyIdentity(keyOf(t, pair[1]))
			require.NotEmptyf(t, left, "%q", pair[0])
			assert.NotEqualf(t, left, right, "%q and %q are two keys", pair[0], pair[1])
		}
	})

	t.Run("an alias is answered from the anchors, not from its target", func(t *testing.T) {
		// KeyIdentity never reads AliasNode.Target: following it expands the
		// alias graph into the string, which is exponential in the document's
		// width. An alias goes unnamed unless the caller hands over what its
		// anchors resolved to, and then it costs one lookup.
		f, err := parser.ParseBytes([]byte("x: &a [1]\n? *a\n: 1\n"))
		require.NoError(t, err)
		body, ok := f.Docs[0].Body.(*ast.MappingNode)
		require.True(t, ok)
		require.Len(t, body.Values, 2)

		alias := body.Values[1].Key
		sequence := ast.KeyIdentity(keyOf(t, "[1]: 1\n"))

		assert.True(t, ast.Unnamed(ast.KeyIdentity(alias)),
			"on its own, an alias has nothing to say")

		anchors := func(name string) (string, bool) {
			if name == "a" {
				return sequence, true
			}

			return "", false
		}
		assert.Equal(t, sequence, ast.KeyIdentityWithAnchors(alias, anchors),
			"handed the anchors, an alias names the sequence its anchor named")

		assert.True(t, ast.Unnamed(ast.KeyIdentityWithAnchors(alias, func(string) (string, bool) {
			return "", false
		})), "an anchor the caller does not know leaves the key unnamed")
	})

	t.Run("an alias graph does not expand", func(t *testing.T) {
		// Nine anchors each naming the one before it nine times: 9^9
		// expansions, which built a 3.7 GB string before KeyIdentity stopped
		// following Target. Parsing it is what calls KeyIdentity, through
		// Parser.keepAnchorIdentity on every anchored node.
		var b strings.Builder
		b.WriteString("? &a0 x\n: v\n")
		for d := 1; d <= 9; d++ {
			fmt.Fprintf(&b, "? &a%d [", d)
			for w := range 9 {
				if w > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "*a%d", d-1)
			}
			fmt.Fprintf(&b, "]\n: v%d\n", d)
		}

		start := time.Now()
		_, err := parser.ParseBytes([]byte(b.String()))
		require.NoError(t, err)
		assert.Lessf(t, time.Since(start), time.Second,
			"%d bytes of alias graph parses in linear time", b.Len())
	})

	t.Run("a node with nothing to say has no identity", func(t *testing.T) {
		assert.True(t, ast.Unnamed(ast.KeyIdentity(nil)))
		assert.True(t, ast.Unnamed(ast.KeyIdentity(&ast.AliasNode{})))
	})
}
