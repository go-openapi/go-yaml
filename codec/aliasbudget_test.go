// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
		if err == nil {
			t.Errorf("width %d: %d bytes naming %d values was accepted", width, len(src), pow(width, 5))

			continue
		}
		if !errors.Is(err, yamlerrors.ErrExcessiveAliasing) {
			t.Errorf("width %d: refused as %v, want ErrExcessiveAliasing", width, err)
		}
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
		if err := Unmarshal([]byte(src), &dst); err != nil {
			t.Fatal(err)
		}
		if dst["use5"]["a"] != 1 {
			t.Errorf("read %v", dst["use5"])
		}
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
		if err := Unmarshal([]byte(src.String()), &dst); err != nil {
			t.Fatal(err)
		}
		if len(dst) != 51 || dst["use49"]["k99"] != 99 {
			t.Errorf("read %d entries", len(dst))
		}
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
	if err := Unmarshal([]byte(src.String()), &dst); err != nil {
		t.Fatal(err)
	}
	if len(dst) != 2000 {
		t.Errorf("read %d entries, want 2000", len(dst))
	}
}

// TestARefusedExpansionNamesAnAlias holds every decode path to naming an alias when the budget runs out.
//
// The walk names the alias it was copying. The tree named the node it had reached, which sat inside an
// anchored tree, or in another document's source for an anchor passed with parser.WithAnchors.
func TestARefusedExpansionNamesAnAlias(t *testing.T) {
	refusedAt := func(t *testing.T, err error) *token.Token {
		t.Helper()
		var refusal *yamlerrors.Error
		if !errors.As(err, &refusal) || !errors.Is(err, yamlerrors.ErrExcessiveAliasing) {
			t.Fatalf("refused as %v, want ErrExcessiveAliasing", err)
		}
		tk := refusal.GetToken()
		if tk == nil || tk.Type != token.AliasType {
			t.Fatalf("the refusal names %v, want an alias", tk)
		}

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
		if err != nil {
			t.Fatal(err)
		}
		opt := WithParserOptions(parser.WithAnchors(declared.Docs[0].Anchors))
		for _, dst := range []any{new(any), new(map[string]any)} {
			t.Run(fmt.Sprintf("%T", dst), func(t *testing.T) {
				tk := refusedAt(t, UnmarshalWithOptions([]byte("a: *x\n"), dst, opt))
				if tk.Position.Line != 1 || tk.Position.Column != 4 {
					t.Errorf("the refusal names %d:%d, want the alias at 1:4", tk.Position.Line, tk.Position.Column)
				}
			})
		}
	})
}
