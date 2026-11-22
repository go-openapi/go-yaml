// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// aliasBomb writes levels of aliases, each naming the one below it width times,
// so the document names width^levels values in a few hundred bytes.
func aliasBomb(levels, width int) string {
	var b strings.Builder
	b.WriteString("l0: &l0 [" + strings.Repeat(`"x",`, width-1) + `"x"` + "]\n")
	for i := 1; i < levels; i++ {
		fmt.Fprintf(&b, "l%d: &l%d [", i, i)
		for j := range width {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "*l%d", i-1)
		}
		b.WriteString("]\n")
	}
	fmt.Fprintf(&b, "top: *l%d\n", levels-1)

	return b.String()
}

type bombDestination struct {
	Top [][][][][]string `yaml:"top"`
}

// TestAliasAmplificationIsRefused reads a document whose aliases name far more
// than it holds. Each level multiplies, so the source grows by 40 bytes where
// the value grows by a factor of the width, and maxDecodeDepth never sees it:
// the depth is five.
func TestAliasAmplificationIsRefused(t *testing.T) {
	for _, width := range []int{6, 8, 10, 12} {
		src := aliasBomb(5, width)

		var dst bombDestination
		err := Unmarshal([]byte(src), &dst)
		if !assert.Errorf(t, err, "width %d: %d bytes naming %d values was accepted", width, len(src), pow(width, 5)) {
			continue
		}
		assert.ErrorIsf(t, err, yamlerrors.ErrExcessiveAliasing, "width %d", width)
	}
}

func pow(n, e int) int {
	out := 1
	for range e {
		out *= n
	}

	return out
}

// TestOrdinaryAliasingIsNotRefused holds the other side of the budget: an
// anchor written once and used many times is what a generated configuration
// looks like, and must read.
func TestOrdinaryAliasingIsNotRefused(t *testing.T) {
	t.Run("a small anchor used five times", func(t *testing.T) {
		const src = "base: &b\n  a: 1\n  b: 2\nuse1: *b\nuse2: *b\nuse3: *b\nuse4: *b\nuse5: *b\n"

		var dst map[string]map[string]int
		require.NoError(t, Unmarshal([]byte(src), &dst))
		assert.Equal(t, 1, dst["use5"]["a"])
	})

	t.Run("a hundred keys used fifty times", func(t *testing.T) {
		var src strings.Builder
		src.WriteString("base: &b\n")
		for i := range 100 {
			fmt.Fprintf(&src, "  k%d: %d\n", i, i)
		}
		for i := range 50 {
			fmt.Fprintf(&src, "use%d: *b\n", i)
		}

		var dst map[string]map[string]int
		require.NoError(t, Unmarshal([]byte(src.String()), &dst))
		assert.Len(t, dst, 51)
		assert.Equal(t, 99, dst["use49"]["k99"])
	})
}

// TestBudgetLeavesRealDocumentsAlone checks the headroom on a document with no
// alias at all, which must never come near the budget.
func TestBudgetLeavesRealDocumentsAlone(t *testing.T) {
	var src strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&src, "k%d:\n  a: %d\n  b: [1, 2, 3]\n", i, i)
	}

	type entry struct {
		A int   `yaml:"a"`
		B []int `yaml:"b"`
	}
	var dst map[string]entry
	require.NoError(t, Unmarshal([]byte(src.String()), &dst))
	assert.Len(t, dst, 2000)
}

// TestARefusedExpansionNamesAnAlias holds every decode path to naming an alias when the budget runs out.
//
// The walk names the alias it was copying. The tree named the node it had reached, which sat inside an
// anchored tree, or in another document's source for an anchor passed with parser.WithAnchors.
func TestARefusedExpansionNamesAnAlias(t *testing.T) {
	refusedAt := func(t *testing.T, err error) *token.Token {
		t.Helper()
		var refusal *yamlerrors.Error
		require.ErrorAs(t, err, &refusal)
		require.ErrorIs(t, err, yamlerrors.ErrExcessiveAliasing)
		tk := refusal.GetToken()
		require.NotNil(t, tk, "the refusal names no token")
		require.Equal(t, token.AliasType, tk.Type, "the refusal names %v, want an alias", tk)

		return tk
	}

	t.Run("an alias bomb", func(t *testing.T) {
		src := []byte(aliasBomb(5, 8))
		for _, dst := range []any{new(any), new(map[string]any), new(bombDestination)} {
			t.Run(fmt.Sprintf("%T", dst), func(t *testing.T) {
				refusedAt(t, Unmarshal(src, dst))
			})
		}
	})

	t.Run("an anchor passed with WithAnchors", func(t *testing.T) {
		declared, err := parser.ParseBytes([]byte("base: &x [" + strings.Repeat("1, ", 2000) + "1]\n"))
		require.NoError(t, err)
		opt := WithParserOptions(parser.WithAnchors(declared.Docs[0].Anchors))
		for _, dst := range []any{new(any), new(map[string]any)} {
			t.Run(fmt.Sprintf("%T", dst), func(t *testing.T) {
				tk := refusedAt(t, Unmarshal([]byte("a: *x\n"), dst, opt))
				assert.Equal(t, int32(1), tk.Position.Line, "line of the alias")
				assert.Equal(t, int32(4), tk.Position.Column, "column of the alias")
			})
		}
	})
}

type aliasPair struct {
	N int `yaml:"n"`
}

const aliasSrc = "base: &b\n  n: 1\nfirst: *b\nsecond: *b\n"

// TestAliasesAreIndependent covers every destination that used to hand two
// aliases one value. Whether it did turned on whether the destination declared
// a field for the anchor itself: with one, the decoded value was recorded and
// every alias got it; without one, each alias read the node afresh.
func TestAliasesAreIndependent(t *testing.T) {
	t.Run("into an any, walking", func(t *testing.T) {
		var v any
		require.NoError(t, Unmarshal([]byte(aliasSrc), &v))
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		assert.NotEqual(t, 999, m["second"].(map[string]any)["n"], "writing through first wrote through second")
		assert.NotEqual(t, 999, m["base"].(map[string]any)["n"], "writing through first wrote through the anchor")
	})

	t.Run("into an any, tree", func(t *testing.T) {
		var v any
		require.NoError(t, treeDecoder(aliasSrc).Decode(&v))
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		assert.NotEqual(t, 999, m["second"].(map[string]any)["n"], "writing through first wrote through second")
	})

	t.Run("into pointers, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		require.NoError(t, Unmarshal([]byte(aliasSrc), &dst))
		require.NotSame(t, dst.Second, dst.First, "two aliases stand at one address")
		require.NotSame(t, dst.Base, dst.First, "an alias and its anchor stand at one address")
		dst.First.N = 999
		assert.NotEqual(t, 999, dst.Second.N, "writing through first wrote through second")
		assert.NotEqual(t, 999, dst.Base.N, "writing through first wrote through the anchor")
	})

	t.Run("into maps, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   map[string]int `yaml:"base"`
			First  map[string]int `yaml:"first"`
			Second map[string]int `yaml:"second"`
		}
		require.NoError(t, Unmarshal([]byte(aliasSrc), &dst))
		dst.First["n"] = 999
		assert.NotEqual(t, 999, dst.Second["n"], "writing through first wrote through second")
		assert.NotEqual(t, 999, dst.Base["n"], "writing through first wrote through the anchor")
	})
}

// TestShareAliasesGivesOneValue is the other half: the option puts the two
// names back at one address, which is what MarshalAnchor and WithSmartAnchor
// read to write an alias back out.
func TestShareAliasesGivesOneValue(t *testing.T) {
	t.Run("into an any, walking", func(t *testing.T) {
		var v any
		require.NoError(t, Unmarshal([]byte(aliasSrc), &v, ShareAliases()))
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		assert.Equal(t, 999, m["second"].(map[string]any)["n"], "second reads the value first was given")
	})

	t.Run("into pointers", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		dec := NewDecoder(bytes.NewReader([]byte(aliasSrc)), ShareAliases())
		require.NoError(t, dec.Decode(&dst))
		assert.Same(t, dst.First, dst.Second, "first and second stand at one address")
		assert.Same(t, dst.Base, dst.First, "the anchor and first stand at one address")
	})
}

// TestSharingSurvivesTheBudget checks that the option also puts back the
// immunity: an alias that builds nothing extra cannot run past the budget, so a
// document naming 10^9 values reads in no time.
func TestSharingSurvivesTheBudget(t *testing.T) {
	src := aliasBomb(9, 9)

	var v any
	require.NoErrorf(t, Unmarshal([]byte(src), &v, ShareAliases()), "%d bytes", len(src))
}

// underEleven is src read under YAML 1.1, where "<<" is the merge key.
//
// The merge key is tag:yaml.org,2002:merge, a 1.1 type, so the version decides
// whether a bare "<<" merges at all. These tests are about what a merge means,
// so they ask 1.1; what the same documents mean as written is asserted in
// codec_test's TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema.
//
// The package has its own copy because codec_test is a different package.
func underEleven(src string) string { return "%YAML 1.1\n---\n" + src }

type mergePair struct {
	A int `yaml:"a"`
	B int `yaml:"b"`
	C int `yaml:"c"`
}

// TestMergedKeyLosesToTheMappingsOwn covers the point of "<<": the mapping
// writes what the merged one already writes, and its own value stands.
//
// Both were validated against one another until now, so overriding a merged key
// was refused as a duplicate -- with the position of the anchor's key when the
// "<<" came last. The document writes each key once; the two stand in different
// mappings.
func TestMergedKeyLosesToTheMappingsOwn(t *testing.T) {
	for name, src := range map[string]string{
		"merge first": underEleven("base: &b\n  a: 1\n  b: 2\nuse:\n  <<: *b\n  b: 3\n"),
		"merge last":  underEleven("base: &b\n  a: 1\n  b: 2\nuse:\n  b: 3\n  <<: *b\n"),
	} {
		t.Run(name, func(t *testing.T) {
			var dst struct {
				Base mergePair `yaml:"base"`
				Use  mergePair `yaml:"use"`
			}
			require.NoError(t, Unmarshal([]byte(src), &dst))
			assert.Equal(t, 1, dst.Use.A, "a from the merge")
			assert.Equal(t, 3, dst.Use.B, "b from the mapping's own")

			// The two paths read one document, so they answer the same.
			var walked any
			require.NoError(t, Unmarshal([]byte(src), &walked))
			use, _ := walked.(map[string]any)["use"].(map[string]any)
			assert.Contains(t, []any{uint64(1), int64(1), 1}, use["a"], "walk read a as %T", use["a"])
			assert.Contains(t, []any{uint64(3), int64(3), 3}, use["b"], "walk read b as %T", use["b"])
		})
	}
}

// TestEarlierMergeWinsOverLater covers a mapping merging two others that write
// the same key: the first named stands, as YAML 1.1's merge says.
func TestEarlierMergeWinsOverLater(t *testing.T) {
	src := underEleven("one: &one\n  a: 1\n  b: 1\ntwo: &two\n  b: 2\n  c: 2\nuse:\n  <<: [*one, *two]\n")

	var dst struct {
		Use mergePair `yaml:"use"`
	}
	require.NoError(t, Unmarshal([]byte(src), &dst))
	assert.Equal(t, 1, dst.Use.A, "a from the first merge")
	assert.Equal(t, 2, dst.Use.C, "c from the second merge")
	assert.Equal(t, 1, dst.Use.B, "b from the mapping named first")
}

// TestDuplicateKeyInOneMappingIsStillRefused holds the line the fix moves: two
// keys of one mapping still collide.
func TestDuplicateKeyInOneMappingIsStillRefused(t *testing.T) {
	var dst mergePair
	assert.Error(t, Unmarshal([]byte("a: 1\na: 2\n"), &dst), "the repeated key was accepted")
}

// TestNodeToValueBudgetsTheNodeItIsGiven holds the alias budget of a decode from a node. Such a decode has no source,
// and its budget fell to aliasBudgetFloor, so any document past a few hundred nodes was refused as excessive aliasing.
// The budget now comes from the nodes the tree holds written out, an alias counting once.
func TestNodeToValueBudgetsTheNodeItIsGiven(t *testing.T) {
	flat := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "k%d: v\n", i)
		}

		return b.String()
	}

	t.Run("a document with no alias reads", func(t *testing.T) {
		for _, n := range []int{300, 5000} {
			file, err := parser.ParseBytes([]byte(flat(n)))
			require.NoError(t, err)
			var dst map[string]string
			require.NoErrorf(t, NodeToValue(file.Docs[0].Body, &dst), "%d entries", n)
			require.Lenf(t, dst, n, "%d entries", n)
		}
	})

	t.Run("one decoder reads one node after another", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte(flat(100)))
		require.NoError(t, err)
		dec := NewDecoder(&bytes.Buffer{})
		for i := range 50 {
			var dst map[string]string
			require.NoErrorf(t, dec.DecodeFromNode(file.Docs[0].Body, &dst), "decode %d", i)
		}
	})

	t.Run("an alias bomb is still refused", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte(aliasBomb(5, 12)))
		require.NoError(t, err)
		var dst bombDestination
		require.ErrorIs(t, NodeToValue(file.Docs[0].Body, &dst), yamlerrors.ErrExcessiveAliasing)
	})
}
