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
