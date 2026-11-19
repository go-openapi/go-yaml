// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

type WalkLeaf struct {
	S  string  `yaml:"s"`
	I  int64   `yaml:"i"`
	U  uint32  `yaml:"u"`
	F  float64 `yaml:"f"`
	B  bool    `yaml:"b"`
	P  *string `yaml:"p"`
	Xs []int64 `yaml:"xs"`
}

type embedded struct {
	WalkLeaf `yaml:",inline"`

	Extra string `yaml:"extra"`
}

// selfEmbedding embeds itself through a pointer, which flatten must not follow
// for ever.
type selfEmbedding struct {
	*selfEmbedding `yaml:",inline"` //nolint:unused // the embedding is the point: flatten has to stop on it

	Name string `yaml:"name"`
}

type walkShape struct {
	Any    any                 `yaml:"any"`
	Anys   map[string]any      `yaml:"anys"`
	List2  []any               `yaml:"list2"`
	Name   string              `yaml:"name"`
	Leaf   WalkLeaf            `yaml:"leaf"`
	Ptr    *WalkLeaf           `yaml:"ptr"`
	List   []WalkLeaf          `yaml:"list"`
	ByName map[string]WalkLeaf `yaml:"byName"`
	Counts map[string][]int64  `yaml:"counts"`
	Plain  map[string]string   `yaml:"plain"`
}

// TestTypedWalkMatchesTheTree reads each document both ways and requires the
// same Go value. The walk fills the destination as the parse hands the document
// over; the tree decoder builds the document first and reads it after.
func TestEmbeddedWalkMatchesTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"outer and embedded": "s: outer\ni: 3\nxs: [1, 2]\nextra: e\n",
		"embedded only":      "s: inner\n",
		"outer only":         "extra: e\n",
		"neither":            "nope: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			var walked embedded
			walkErr := Unmarshal([]byte(src), &walked)

			var built embedded
			treeErr := NewDecoder(
				bytes.NewReader([]byte(src)),
				CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }),
			).Decode(&built)

			if (walkErr == nil) != (treeErr == nil) {
				t.Fatalf("walk err=%v, tree err=%v", walkErr, treeErr)
			}
			if !reflect.DeepEqual(walked, built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", walked, built)
			}
		})
	}
}

func TestTypedWalkMatchesTheTree(t *testing.T) {
	docs := map[string]string{
		"scalars":          "name: top\nleaf:\n  s: text\n  i: -12\n  u: 7\n  f: 1.5\n  b: true\n  xs: [1, 2, 3]\n",
		"empty sequence":   "leaf:\n  xs: []\n",
		"missing fields":   "name: only\n",
		"null value":       "name: ~\nleaf: null\n",
		"pointer set":      "ptr:\n  s: inner\n  p: here\n",
		"pointer null":     "ptr: ~\n",
		"list":             "list:\n  - s: one\n  - s: two\n    i: 2\n",
		"map of structs":   "byName:\n  a:\n    s: aa\n  b:\n    i: 3\n",
		"map of slices":    "counts:\n  a: [1, 2]\n  b: []\n",
		"plain map":        "plain:\n  k: v\n  k2: v2\n",
		"empty mapping":    "leaf: {}\n",
		"flow":             "leaf: {s: flow, i: 1, xs: [9]}\n",
		"unknown key":      "name: x\nnope: 1\nleaf:\n  s: y\n",
		"non-string key":   "name: x\n1: skipped\nleaf:\n  s: y\n",
		"quoted number":    "leaf:\n  s: \"12\"\n",
		"negative":         "leaf:\n  i: -9223372036854775808\n",
		"exponent":         "leaf:\n  f: 1e3\n",
		"any scalar":       "any: hello\n",
		"any null":         "any: ~\n",
		"any mapping":      "any:\n  a: 1\n  b: [x, y]\n",
		"any sequence":     "any: [1, two, {k: v}]\n",
		"map of any":       "anys:\n  a: 1\n  b:\n    c: [1, 2]\n  d: text\n",
		"list of any":      "list2:\n  - 1\n  - k: v\n  - [a, b]\n",
		"any beside typed": "name: x\nany:\n  deep:\n    deeper: [1, 2]\nleaf:\n  s: y\n",
		"any empty map":    "any: {}\n",
		"any empty list":   "any: []\n",
	}

	for name, src := range docs {
		t.Run(name, func(t *testing.T) {
			var walked walkShape
			walkErr := Unmarshal([]byte(src), &walked)

			var built walkShape
			treeErr := NewDecoder(
				bytes.NewReader([]byte(src)),
				CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }),
			).Decode(&built)

			switch {
			case walkErr != nil && treeErr != nil:
			case walkErr != nil || treeErr != nil:
				t.Fatalf("walk err=%v, tree err=%v", walkErr, treeErr)
			}
			if !reflect.DeepEqual(walked, built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", walked, built)
			}
		})
	}
}

// TestWalkableTypeRefusesWhatTheTreeReads holds the gate to the destinations it
// must keep off the walk. Deciding on the type rather than on the document is
// what keeps a refusal from costing a parse: a walk that gives up halfway has
// read the document for nothing.
func TestWalkableTypeRefusesWhatTheTreeReads(t *testing.T) {
	refused := map[string]reflect.Type{
		"a time.Time":           reflect.TypeFor[struct{ A time.Time }](),
		"a time.Duration":       reflect.TypeFor[struct{ A time.Duration }](),
		"a MapSlice":            reflect.TypeFor[struct{ A MapSlice }](),
		"a RawMessage":          reflect.TypeFor[struct{ A RawMessage }](),
		"a map keyed by an int": reflect.TypeFor[struct{ A map[int]string }](),
		"an array":              reflect.TypeFor[struct{ A [3]int }](),
		"a complex number":      reflect.TypeFor[struct{ A complex128 }](),
	}
	for name, typ := range refused {
		if walkableType(typ, yamlTags) {
			t.Errorf("%s: the gate let %s through", name, typ)
		}
	}

	accepted := map[string]reflect.Type{
		"the shape above":    reflect.TypeFor[walkShape](),
		"plain scalars":      reflect.TypeFor[WalkLeaf](),
		"nested maps":        reflect.TypeFor[struct{ A map[string][]map[string]int }](),
		"an any field":       reflect.TypeFor[struct{ A any }](),
		"a map of any":       reflect.TypeFor[struct{ A map[string]any }](),
		"a nested any":       reflect.TypeFor[struct{ A []map[string]any }](),
		"an interface slice": reflect.TypeFor[struct{ A []any }](),
		"an embedded struct": reflect.TypeFor[embedded](),
		"a self-embedding":   reflect.TypeFor[selfEmbedding](),
	}
	for name, typ := range accepted {
		if !walkableType(typ, yamlTags) {
			t.Errorf("%s: the gate refused %s", name, typ)
		}
	}
}

// TestWalkPreservesDefaults checks a null leaving the destination as the caller
// set it, which is what the tree decoder does by writing the default over a
// fresh zero.
func TestWalkPreservesDefaults(t *testing.T) {
	dst := walkShape{Name: "default", Leaf: WalkLeaf{S: "kept"}}
	if err := Unmarshal([]byte("leaf:\n"), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Name != "default" || dst.Leaf.S != "kept" {
		t.Errorf("got %+v, want the defaults kept", dst)
	}
}

// TestAnAnchorKeepsTheWalk reads documents whose values carry an anchor, and
// requires the value the tree gives without the decode building one.
//
// The walk handed every anchor to the tree, which parses the whole source again:
// citm_catalog into its Go types peaked at 0.69 MiB, and at 19.5 MiB with one
// anchor written anywhere in it.
func TestAnAnchorKeepsTheWalk(t *testing.T) {
	for name, src := range map[string]string{
		"a scalar":                    "name: &n top\n",
		"a block mapping":             "leaf: &l\n  s: text\n  i: 3\n",
		"a flow mapping":              "leaf: &l {s: text, i: 3}\n",
		"a sequence":                  "leaf:\n  xs: &x [1, 2]\n",
		"a sequence entry":            "list:\n  - &e {s: one}\n  - s: two\n",
		"a scalar in a sequence":      "leaf:\n  xs: [1, &x 2, 3]\n",
		"a map value":                 "byName:\n  a: &a {s: aa}\n  b: {i: 3}\n",
		"a plain map value":           "plain:\n  k: &k v\n",
		"a pointer":                   "ptr: &p {s: inner}\n",
		"an any":                      "any: &a {k: [1, 2]}\n",
		"the root":                    "&r\nname: top\n",
		"a field no field claims":     "nope: &n {k: v}\nname: x\n",
		"two anchors":                 "name: &a x\nleaf: &b {s: y}\n",
		"an anchor inside an anchor":  "leaf: &l {s: &s text}\n",
		"a sequence of anchored maps": "list: [&a {s: one}, &b {s: two}]\n",
		"an anchor on nothing":        "name: &n\nlist: &l\n",
	} {
		t.Run(name, func(t *testing.T) {
			var walked walkShape
			dec := NewDecoder(bytes.NewReader([]byte(src)))
			walkErr := dec.Decode(&walked)

			var built walkShape
			treeErr := NewDecoder(
				bytes.NewReader([]byte(src)),
				CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }),
			).Decode(&built)

			if walkErr != nil || treeErr != nil {
				t.Fatalf("walk err=%v, tree err=%v", walkErr, treeErr)
			}
			if !reflect.DeepEqual(walked, built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", walked, built)
			}
			if dec.parsedFile != nil {
				t.Error("the decode built a tree")
			}
		})
	}
}

// TestAnAnchoredNullFillsAStructWithItsZero checks the walk alone on an anchor
// standing on nothing where the destination is a struct or a pointer to one.
//
// The walk reads it as the empty node, as it reads "leaf:" without the anchor,
// and so does go.yaml.in/yaml/v3. The tree refuses it with "null was used where
// mapping is expected", which is defect 148 and not the walk's.
func TestAnAnchoredNullFillsAStructWithItsZero(t *testing.T) {
	for _, src := range []string{"leaf: &l\n", "leaf: &l ~\n", "ptr: &p\n"} {
		t.Run(src, func(t *testing.T) {
			walked := walkShape{Name: "preset"}
			dec := NewDecoder(bytes.NewReader([]byte(src)))
			if err := dec.Decode(&walked); err != nil {
				t.Fatal(err)
			}
			if want := (walkShape{Name: "preset"}); !reflect.DeepEqual(walked, want) {
				t.Errorf("got %+v, want %+v", walked, want)
			}
			if dec.parsedFile != nil {
				t.Error("the decode built a tree")
			}
		})
	}
}

// TestAnAnchorTheWalkCannotReadGoesToTheTree pins the anchors the walk still hands
// to the tree, so each one that starts to walk is noticed and moved above.
func TestAnAnchorTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an alias":                     "name: &n top\nleaf:\n  s: *n\n",
		"an anchored key":              "&k name: top\n",
		"an anchor on a tagged scalar": "name: &n !!str top\n",
		"an anchor inside an any":      "any: {k: &a v}\n",
	} {
		t.Run(name, func(t *testing.T) {
			var walked walkShape
			dec := NewDecoder(bytes.NewReader([]byte(src)))
			if err := dec.Decode(&walked); err != nil {
				t.Fatal(err)
			}
			if dec.parsedFile == nil {
				t.Error("the walk read it: move the case to TestAnAnchorKeepsTheWalk")
			}
		})
	}
}
