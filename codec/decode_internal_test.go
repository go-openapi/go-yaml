// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unsafe"

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
		if err := Unmarshal([]byte(aliasSrc), &v); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through second")
		}
		if got := m["base"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through the anchor")
		}
	})

	t.Run("into an any, tree", func(t *testing.T) {
		var v any
		if err := treeDecoder(aliasSrc).Decode(&v); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through second")
		}
	})

	t.Run("into pointers, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		if err := Unmarshal([]byte(aliasSrc), &dst); err != nil {
			t.Fatal(err)
		}
		if dst.First == dst.Second || dst.First == dst.Base {
			t.Fatal("two aliases stand at one address")
		}
		dst.First.N = 999
		if dst.Second.N == 999 || dst.Base.N == 999 {
			t.Error("writing through first wrote through the others")
		}
	})

	t.Run("into maps, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   map[string]int `yaml:"base"`
			First  map[string]int `yaml:"first"`
			Second map[string]int `yaml:"second"`
		}
		if err := Unmarshal([]byte(aliasSrc), &dst); err != nil {
			t.Fatal(err)
		}
		dst.First["n"] = 999
		if dst.Second["n"] == 999 || dst.Base["n"] == 999 {
			t.Error("writing through first wrote through the others")
		}
	})
}

// TestShareAliasesGivesOneValue is the other half: the option puts the two
// names back at one address, which is what MarshalAnchor and WithSmartAnchor
// read to write an alias back out.
func TestShareAliasesGivesOneValue(t *testing.T) {
	t.Run("into an any, walking", func(t *testing.T) {
		var v any
		if err := UnmarshalWithOptions([]byte(aliasSrc), &v, ShareAliases()); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got != 999 {
			t.Errorf("second reads %v, want the value first was given", got)
		}
	})

	t.Run("into pointers", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		dec := NewDecoder(bytes.NewReader([]byte(aliasSrc)), ShareAliases())
		if err := dec.Decode(&dst); err != nil {
			t.Fatal(err)
		}
		if dst.First != dst.Second || dst.First != dst.Base {
			t.Error("the three names do not stand at one address")
		}
	})
}

// TestSharingSurvivesTheBudget checks that the option also puts back the
// immunity: an alias that builds nothing extra cannot run past the budget, so a
// document naming 10^9 values reads in no time.
func TestSharingSurvivesTheBudget(t *testing.T) {
	src := aliasBomb(9, 9)

	var v any
	if err := UnmarshalWithOptions([]byte(src), &v, ShareAliases()); err != nil {
		t.Fatalf("%d bytes: %v", len(src), err)
	}
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
			if err := Unmarshal([]byte(src), &dst); err != nil {
				t.Fatal(err)
			}
			if dst.Use.A != 1 {
				t.Errorf("a: got %d, want 1 from the merge", dst.Use.A)
			}
			if dst.Use.B != 3 {
				t.Errorf("b: got %d, want 3, the mapping's own", dst.Use.B)
			}

			// The two paths read one document, so they answer the same.
			var walked any
			if err := Unmarshal([]byte(src), &walked); err != nil {
				t.Fatal(err)
			}
			use, _ := walked.(map[string]any)["use"].(map[string]any)
			if use["a"] != uint64(1) && use["a"] != int64(1) && use["a"] != 1 {
				t.Errorf("walk read a as %v (%T)", use["a"], use["a"])
			}
			if use["b"] != uint64(3) && use["b"] != int64(3) && use["b"] != 3 {
				t.Errorf("walk read b as %v (%T)", use["b"], use["b"])
			}
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
	if err := Unmarshal([]byte(src), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Use.A != 1 || dst.Use.C != 2 {
		t.Errorf("the merge did not take both: %+v", dst.Use)
	}
	if dst.Use.B != 1 {
		t.Errorf("b: got %d, want 1 from the mapping named first", dst.Use.B)
	}
}

// TestDuplicateKeyInOneMappingIsStillRefused holds the line the fix moves: two
// keys of one mapping still collide.
func TestDuplicateKeyInOneMappingIsStillRefused(t *testing.T) {
	var dst mergePair
	if err := Unmarshal([]byte("a: 1\na: 2\n"), &dst); err == nil {
		t.Error("the repeated key was accepted")
	}
}

// TestReadAllDoesNotCopyWhatItIsGiven checks the decoder reads the caller's
// bytes rather than a copy of them.
//
// Unmarshal wraps the caller's slice in a bytes.Buffer, and the parser keeps
// windows into what it parses, so a copy here would be one the whole tree then
// holds. This fails the moment readAll starts allocating again.
func TestReadAllDoesNotCopyWhatItIsGiven(t *testing.T) {
	data := []byte("a: 1\nb: two\n")

	got, err := readAll(bytes.NewBuffer(data))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("readAll changed the document: %q", got)
	}
	if &got[0] != &data[0] {
		t.Fatal("readAll copied a bytes.Buffer's contents instead of reading them where they were")
	}
}

// TestReadAllReadsWhatIsNotASlice checks the fallback still reads a plain
// reader through, since only a byte-backed one can be read in place.
func TestReadAllReadsWhatIsNotASlice(t *testing.T) {
	const doc = "a: 1\nb: two\n"

	got, err := readAll(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != doc {
		t.Fatalf("readAll changed the document: %q", got)
	}
}

// TestReadAllReadsAByteReader covers *bytes.Reader, which holds the bytes but
// will not hand them over.
func TestReadAllReadsAByteReader(t *testing.T) {
	data := []byte("a: 1\n")

	got, err := readAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("readAll changed the document: %q", got)
	}
	if len(got) > 0 && uintptr(unsafe.Pointer(&got[0])) == uintptr(unsafe.Pointer(&data[0])) {
		t.Fatal("bytes.Reader does not expose its slice; this now aliases it, which is worth knowing")
	}
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
			if err != nil {
				t.Fatal(err)
			}
			var dst map[string]string
			if err := NodeToValue(file.Docs[0].Body, &dst); err != nil {
				t.Fatalf("%d entries: %v", n, err)
			}
			if len(dst) != n {
				t.Fatalf("%d entries: read %d", n, len(dst))
			}
		}
	})

	t.Run("one decoder reads one node after another", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte(flat(100)))
		if err != nil {
			t.Fatal(err)
		}
		dec := NewDecoder(&bytes.Buffer{})
		for i := range 50 {
			var dst map[string]string
			if err := dec.DecodeFromNode(file.Docs[0].Body, &dst); err != nil {
				t.Fatalf("decode %d: %v", i, err)
			}
		}
	})

	t.Run("an alias bomb is still refused", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte(aliasBomb(5, 12)))
		if err != nil {
			t.Fatal(err)
		}
		var dst bombDestination
		err = NodeToValue(file.Docs[0].Body, &dst)
		if !errors.Is(err, yamlerrors.ErrExcessiveAliasing) {
			t.Fatalf("refused as %v, want ErrExcessiveAliasing", err)
		}
	})
}
