// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec/internal/structfields"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
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
	// Aliased takes the anchor a "<<" names into its `,inline,alias` field.
	Aliased aliasedLeaf `yaml:"aliased"`
}

type aliasedLeaf struct {
	*WalkLeaf `yaml:",inline,alias"`

	Extra string `yaml:"extra"`
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
		if walkableType(typ, structfields.YAMLTags) {
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
		if !walkableType(typ, structfields.YAMLTags) {
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
		"an anchored null struct":     "leaf: &l\nptr: &p\n",
		"an anchored null pointer":    "ptr: &p ~\nleaf: &l ~\n",
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

// TestAnAnchorTheWalkCannotReadGoesToTheTree pins the anchors the walk still hands
// to the tree, so each one that starts to walk is noticed and moved above.
func TestAnAnchorTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an alias as a key":       "name: &n k\nplain: {*n : v}\n",
		"an alias inside an any":  "name: &n top\nany: {k: *n}\n",
		"an anchored key":         "&k name: top\n",
		"an anchor inside an any": "any: {k: &a v}\n",
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

// TestADirectiveKeepsTheWalk reads documents opened by a "%YAML" or a "%TAG"
// line, and requires the value the tree gives without the decode building one.
func TestADirectiveKeepsTheWalk(t *testing.T) {
	for name, src := range map[string]string{
		"a version":               "%YAML 1.1\n---\nname: x\n",
		"a tag handle":            "%TAG !e! tag:example.com,2000:\n---\nleaf: {s: x}\n",
		"a directive and no body": "%YAML 1.2\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			walked := walkShape{Name: "default"}
			dec := NewDecoder(bytes.NewReader([]byte(src)))
			walkErr := dec.Decode(&walked)

			built := walkShape{Name: "default"}
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

// TestAnAliasKeepsTheWalk reads documents whose aliases name an anchor written
// before them, with and without ShareAliases, and requires the value the tree
// gives without the decode building one.
//
// Without the option an alias decodes its anchor's node again, so two aliases
// give two values. With it an alias takes the anchor's own value wherever
// codecutils.CastToAssignableValue accepts it, as the tree does, and decodes the node again
// where it does not: an anchor written into a map[string]string has nothing to
// hand to a map[string]any.
func TestAnAliasKeepsTheWalk(t *testing.T) {
	docs := map[string]string{
		"a scalar":                   "name: &n top\nleaf:\n  s: *n\n",
		"a struct into a pointer":    "leaf: &l {s: x, i: 1}\nptr: *l\n",
		"a pointer into a struct":    "ptr: &p {s: x}\nleaf: *p\n",
		"twice into a slice":         "leaf: &l {s: x}\nlist: [*l, *l]\n",
		"into map values":            "leaf: &l {s: x}\nbyName: {a: *l, b: *l}\n",
		"a struct holding a slice":   "leaf: &l {xs: [1, 2]}\nptr: *l\n",
		"into another map type":      "plain: &p {k: v}\nanys: *p\n",
		"into an any":                "leaf: &l {s: x}\nany: *l\n",
		"into a slice of any":        "leaf: &l {s: x}\nlist2: [*l]\n",
		"a scalar deeper down":       "name: &n top\nlist: [{s: *n}]\n",
		"an anchor inside an anchor": "leaf: &l {s: &s x}\nname: *s\n",
		"an anchor no field claims":  "nope: &n {k: v}\nplain: *n\n",
		"an alias to an alias":       "name: &a x\nleaf: &l {s: *a}\nptr: *l\n",
		"a sequence":                 "counts: {a: &c [1, 2]}\nleaf: {xs: *c}\n",
		"a block anchor":             "leaf: &l\n  s: x\n  i: 2\nptr: *l\n",
	}
	for _, share := range []bool{false, true} {
		for name, src := range docs {
			t.Run(fmt.Sprintf("share=%v/%s", share, name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}

				var walked walkShape
				dec := NewDecoder(bytes.NewReader([]byte(src)), opts...)
				walkErr := dec.Decode(&walked)

				var built walkShape
				treeErr := NewDecoder(
					bytes.NewReader([]byte(src)),
					append(opts, CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))...,
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
}

// TestASharedAliasPointsWhereTheTreeDoes holds ShareAliases to the tree on
// identity, which DeepEqual does not see: a pointer alias either points at the
// anchor's own value or holds a copy, and both paths must make the same choice.
func TestASharedAliasPointsWhereTheTreeDoes(t *testing.T) {
	type shared struct {
		A  WalkLeaf            `yaml:"a"`
		P  *WalkLeaf           `yaml:"p"`
		Q  *WalkLeaf           `yaml:"q"`
		L  []WalkLeaf          `yaml:"l"`
		LP *WalkLeaf           `yaml:"lp"`
		M  map[string]WalkLeaf `yaml:"m"`
		MP *WalkLeaf           `yaml:"mp"`
	}
	const src = "a: &x {s: one}\np: *x\nq: *x\nl: [&y {s: two}]\nlp: *y\nm: {k: &z {s: three}}\nmp: *z\n"

	read := func(opts ...DecodeOption) shared {
		t.Helper()
		var v shared
		if err := NewDecoder(bytes.NewReader([]byte(src)), append(opts, ShareAliases())...).Decode(&v); err != nil {
			t.Fatal(err)
		}

		return v
	}
	walked := read()
	built := read(CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))

	for _, c := range []struct {
		name       string
		walk, tree bool
	}{
		{"p points at a", walked.P == &walked.A, built.P == &built.A},
		{"p and q are one value", walked.P == walked.Q, built.P == built.Q},
		{"lp points at the slice element", len(walked.L) > 0 && walked.LP == &walked.L[0], len(built.L) > 0 && built.LP == &built.L[0]},
	} {
		if c.walk != c.tree {
			t.Errorf("%s: walk %v, tree %v", c.name, c.walk, c.tree)
		}
	}
	if !reflect.DeepEqual(walked, built) {
		t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", walked, built)
	}
}

// TestABrokenAliasIsRefusedAsByTheTree holds the walk to the tree's error on an
// alias that names nothing it can read. The parse refuses an alias to an anchor
// written after it; one inside its own anchor goes to the tree, which refuses
// the cycle.
func TestABrokenAliasIsRefusedAsByTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an alias to a later anchor": "leaf:\n  s: *n\nname: &n top\n",
		"an alias inside its anchor": "leaf: &l {s: x, xs: *l}\n",
	} {
		t.Run(name, func(t *testing.T) {
			var walked, built walkShape
			walkErr := NewDecoder(bytes.NewReader([]byte(src))).Decode(&walked)
			treeErr := NewDecoder(
				bytes.NewReader([]byte(src)),
				CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }),
			).Decode(&built)

			if walkErr == nil || treeErr == nil {
				t.Fatalf("walk err=%v, tree err=%v", walkErr, treeErr)
			}
			if walkErr.Error() != treeErr.Error() {
				t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", walkErr, treeErr)
			}
		})
	}
}

// bothWays holds one document read through the walk and through the tree. dec
// is the walk's decoder, to ask whether it built a tree.
type bothWays struct {
	walked, built    walkShape
	walkErr, treeErr error
	dec              *Decoder
}

// decodeBothWays reads src into a walkShape through the walk and through the
// tree, under YAML 1.1 where "<<" merges.
func decodeBothWays(t *testing.T, src string, opts ...DecodeOption) bothWays {
	t.Helper()
	var r bothWays
	opts = append(opts, WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
	r.dec = NewDecoder(bytes.NewReader([]byte(src)), opts...)
	r.walkErr = r.dec.Decode(&r.walked)
	r.treeErr = NewDecoder(
		bytes.NewReader([]byte(src)),
		append(opts, CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))...,
	).Decode(&r.built)

	return r
}

// TestAMergeKeepsTheWalk reads documents whose "<<" merges into a struct or a
// typed map, and requires the value the tree gives without the decode building
// one. A mapping's own entries win on either side of the "<<", and the earlier
// mapping of a merge sequence wins over a later one.
func TestAMergeKeepsTheWalk(t *testing.T) {
	docs := map[string]string{
		"an alias before the own keys":   "ptr: &b {s: base, i: 1}\nleaf: {<<: *b, s: own}\n",
		"an alias after the own keys":    "ptr: &b {s: base, i: 1}\nleaf: {s: own, <<: *b}\n",
		"a mapping in place":             "leaf: {<<: {s: base, i: 1}, s: own}\n",
		"a sequence of aliases":          "ptr: &a {s: a}\nlist: [&b {s: b, i: 2}]\nleaf: {<<: [*a, *b]}\n",
		"a sequence in place":            "ptr: &a {s: a}\nleaf: {<<: [{i: 3, s: first}, *a]}\n",
		"into a map":                     "plain: {<<: {a: x, b: w}, a: own}\n",
		"into a map of structs":          "ptr: &b {s: base}\nbyName: {<<: {k: *b, j: {s: no}}, j: {s: j}}\n",
		"an own struct replaces a merge": "<<: {leaf: {s: base, i: 1}}\nleaf: {s: own}\n",
		"a merge inside a merged anchor": "ptr: &a {i: 7}\nlist: [&b {<<: *a, s: b}]\nleaf: {<<: *b}\n",
		"an aliased mapping that merges": "ptr: &a {i: 7}\nleaf: &l {<<: *a, s: l}\nlist: [*l]\n",
		"a merge no field claims":        "nope: {<<: {s: x}}\nname: m\n",
		"a merge into an any":            "any: {<<: {a: 1}, b: 2}\n",
		"a block mapping":                "ptr: &b\n  s: base\n  i: 1\nleaf:\n  <<: *b\n  s: own\n",
		"a sequence naming a merger":     "ptr: &a {<<: {s: zero}, s: own}\nleaf: {<<: [*a]}\n",
		"a merged slice":                 "ptr: &b {xs: [1, 2]}\nleaf: {<<: *b}\nlist: [{<<: *b, xs: [3]}]\n",
	}
	for _, share := range []bool{false, true} {
		for name, src := range docs {
			t.Run(fmt.Sprintf("share=%v/%s", share, name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, src, opts...)
				if r.walkErr != nil || r.treeErr != nil {
					t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
				}
				if !reflect.DeepEqual(r.walked, r.built) {
					t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
				}
				if r.dec.parsedFile != nil {
					t.Error("the decode built a tree")
				}
			})
		}
	}
}

// TestAMergeTheWalkCannotReadGoesToTheTree pins the merges the walk hands to
// the tree, and requires the tree's value or the tree's error from both.
func TestAMergeTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an anchor named again before the close": "name: &x one\nleaf: {<<: {s: *x}, p: &x two}\n",
		"an anchored merge":                      "leaf: {<<: &m {s: x}}\nptr: *m\n",
		"a merge of a scalar":                    "name: &n top\nleaf: {<<: *n}\n",
		"a null merge":                           "leaf: {<<: }\n",
		"a sequence holding a scalar":            "leaf: {<<: [{s: x}, 1]}\n",
		"a sequence holding a sequence":          "leaf: {<<: [[{s: x}]]}\n",
		"a struct with an alias field":           "ptr: &b {s: base}\naliased: {<<: *b, extra: e}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if (r.walkErr == nil) != (r.treeErr == nil) {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr != nil {
				if r.walkErr.Error() != r.treeErr.Error() {
					t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
				}

				return
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile == nil {
				t.Error("the walk read it: move the case to TestAMergeKeepsTheWalk")
			}
		})
	}
}

// TestAMergeKeepsTheDefaults holds the walk to the tree on what a merge does to
// the fields of a struct and the entries of a map the caller set: a merged
// entry is written over the default, as an own entry is, and one neither
// writes keeps it.
func TestAMergeKeepsTheDefaults(t *testing.T) {
	const src = "ptr: &b {s: base}\nleaf: {<<: *b, i: 2}\nplain: {<<: {a: x, b: y}, b: own}\n"
	read := func(opts ...DecodeOption) walkShape {
		t.Helper()
		v := walkShape{
			Leaf:  WalkLeaf{S: "default", U: 9, F: 1.5},
			Plain: map[string]string{"a": "default", "b": "default", "z": "kept"},
		}
		opts = append(opts, WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
		if err := NewDecoder(bytes.NewReader([]byte(src)), opts...).Decode(&v); err != nil {
			t.Fatal(err)
		}

		return v
	}
	walked := read()
	built := read(CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))
	if !reflect.DeepEqual(walked, built) {
		t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", walked, built)
	}
}

// declaredAnchors parses each source into the tree parser.WithAnchors takes.
func declaredAnchors(t *testing.T, srcs map[string]string) map[string]ast.Node {
	t.Helper()
	anchors := make(map[string]ast.Node, len(srcs))
	for name, src := range srcs {
		f, err := parser.ParseBytes([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		anchors[name] = f.Docs[0].Body
	}

	return anchors
}

// TestADeclaredAnchorKeepsTheWalk reads documents whose aliases name an anchor
// parser.WithAnchors declared, with and without ShareAliases, and requires the
// value the tree gives without the decode building one. The tree reads a
// declared node into each destination again whatever the option says, so no
// two aliases share a value; TestADeclaredAnchorIsNeverShared holds that.
func TestADeclaredAnchorKeepsTheWalk(t *testing.T) {
	anchors := declaredAnchors(t, map[string]string{
		"t": "!!str 1\n",
		"l": "{s: declared, i: 1, xs: [1, 2]}\n",
		"n": "declared\n",
		"c": "[3, 4]\n",
	})
	docs := map[string]string{
		"a struct":                      "leaf: *l\n",
		"a pointer":                     "ptr: *l\n",
		"twice":                         "leaf: *l\nptr: *l\n",
		"a scalar":                      "name: *n\nleaf: {s: *n}\n",
		"a slice element":               "list: [*l, {s: own}]\n",
		"a map value":                   "byName: {a: *l}\n",
		"a sequence":                    "counts: {a: *c}\n",
		"a merge":                       "leaf: {<<: *l, s: own}\n",
		"a merge sequence":              "leaf: {<<: [{s: first}, *l]}\n",
		"the document's anchor wins":    "name: &n mine\nleaf: {s: *n}\n",
		"an anchor named after a merge": "leaf: {<<: *l}\nname: &l mine\nplain: {k: *l}\n",
		"a tag in the declared tree":    "name: *t\n",
	}
	for _, share := range []bool{false, true} {
		for name, src := range docs {
			t.Run(fmt.Sprintf("share=%v/%s", share, name), func(t *testing.T) {
				opts := []DecodeOption{WithParserOptions(parser.WithAnchors(anchors))}
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, src, opts...)
				if r.walkErr != nil || r.treeErr != nil {
					t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
				}
				if !reflect.DeepEqual(r.walked, r.built) {
					t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
				}
				if r.dec.parsedFile != nil {
					t.Error("the decode built a tree")
				}
			})
		}
	}
}

// TestADeclaredAnchorIsNeverShared holds the walk to the tree on identity:
// under ShareAliases two aliases of a declared anchor still give two values,
// and writing through one leaves the declared tree and the other alone.
func TestADeclaredAnchorIsNeverShared(t *testing.T) {
	anchors := declaredAnchors(t, map[string]string{"l": "{s: declared, xs: [1, 2]}\n"})
	type shared struct {
		P *WalkLeaf `yaml:"p"`
		Q *WalkLeaf `yaml:"q"`
	}
	const src = "p: *l\nq: *l\n"
	for name, extra := range map[string][]DecodeOption{
		"walk": nil,
		"tree": {CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })},
	} {
		t.Run(name, func(t *testing.T) {
			var v shared
			opts := append([]DecodeOption{WithParserOptions(parser.WithAnchors(anchors)), ShareAliases()}, extra...)
			if err := NewDecoder(bytes.NewReader([]byte(src)), opts...).Decode(&v); err != nil {
				t.Fatal(err)
			}
			if v.P == nil || v.Q == nil {
				t.Fatalf("an alias decoded to nil: %+v", v)
			}
			if v.P == v.Q {
				t.Error("two aliases of a declared anchor share a value")
			}
			v.P.Xs[0] = 9
			if v.Q.Xs[0] != 1 {
				t.Error("writing through one alias reached the other")
			}
		})
	}
}

// TestADeclaredAnchorTheWalkCannotReadGoesToTheTree pins the declared anchors
// the walk hands to the tree, and requires the tree's value or error from both.
func TestADeclaredAnchorTheWalkCannotReadGoesToTheTree(t *testing.T) {
	anchors := declaredAnchors(t, map[string]string{"l": "{s: declared}\n"})
	anchors["nothing"] = nil
	for name, src := range map[string]string{
		"a declaration with no node":                "name: *nothing\n",
		"an alias inside an any":                    "any: {k: *l}\n",
		"an anchor named again while a merge waits": "leaf: {<<: {s: *l}, p: &l mine}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src, WithParserOptions(parser.WithAnchors(anchors)))
			if (r.walkErr == nil) != (r.treeErr == nil) {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr != nil {
				if r.walkErr.Error() != r.treeErr.Error() {
					t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
				}

				return
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile == nil {
				t.Error("the walk read it: move the case to TestADeclaredAnchorKeepsTheWalk")
			}
		})
	}
}

// TestUseStringKeysReachesAnAnyFieldOfAStruct holds the walk to the tree on an
// "any" field under UseStringKeys: its mapping keys are the names of the keys,
// so "1: x" is "1" and two keys of one name are refused. valueBuilder keys a
// mapping by value, so the walk hands the decode to the tree.
func TestUseStringKeysReachesAnAnyFieldOfAStruct(t *testing.T) {
	type doc struct {
		K any `yaml:"k"`
	}
	tree := CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })
	for _, src := range []string{"k: {1: x, true: y}\n", "k: {1: x, \"1\": y}\n"} {
		t.Run(src, func(t *testing.T) {
			var walked, built doc
			walkErr := NewDecoder(bytes.NewReader([]byte(src)), UseStringKeys()).Decode(&walked)
			treeErr := NewDecoder(bytes.NewReader([]byte(src)), UseStringKeys(), tree).Decode(&built)
			if (walkErr == nil) != (treeErr == nil) {
				t.Fatalf("walk err=%v, tree err=%v", walkErr, treeErr)
			}
			if !reflect.DeepEqual(walked, built) {
				t.Errorf("the two paths disagree:\n  walk %#v\n  tree %#v", walked, built)
			}
		})
	}
	var v doc
	if err := NewDecoder(bytes.NewReader([]byte("k: {1: x}\n")), UseStringKeys()).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"1": "x"}; !reflect.DeepEqual(v.K, want) {
		t.Errorf("got %#v, want %#v", v.K, want)
	}
}

// TestAConvertedScalarKeepsTheWalk reads scalars of another kind than their
// field, and requires the value the tree gives without the decode building a
// tree. The walk asks the tree decoder for the one scalar, so the two convert
// by the same rules.
func TestAConvertedScalarKeepsTheWalk(t *testing.T) {
	for name, src := range map[string]string{
		"a number into a string":        "name: 1\n",
		"a float into a string":         "name: 1.5\n",
		"a bool into a string":          "name: true\n",
		"an infinity into a string":     "name: .inf\n",
		"a wide number into a string":   "name: 12345678901234567890123\n",
		"a float into an int":           "leaf: {i: 1.5}\n",
		"a quoted number into an int":   "leaf: {i: \"5\"}\n",
		"a float into a uint":           "leaf: {u: 2.0}\n",
		"a number into a pointer":       "leaf: {p: 1}\n",
		"a number into a map value":     "plain: {k: 1, j: true}\n",
		"a slice element":               "leaf: {xs: [1.0, \"2\", 3]}\n",
		"in a sequence of structs":      "list: [{s: 1}, {s: two}]\n",
		"under an anchor and its alias": "name: &n 1\nplain: {k: *n}\n",
		"the fields after it":           "leaf: {s: 1, i: 2, b: true}\nname: n\n",
		"a date into a string":          "name: 2001-12-14\n",
		"a tagged date into a string":   "name: !!timestamp 2001-12-14\n",
		"a tagged quoted date":          "name: !!timestamp \"2001-12-14\"\n",
		"a time in its own zone":        "name: 2001-12-14t21:59:43.10-05:00\n",
		"a date into a pointer":         "leaf: {p: 2001-12-14}\n",
		"a date into a map value":       "plain: {k: 2001-12-14, j: !!timestamp 2002-01-01}\n",
		"a date into an any":            "any: 2001-12-14\nanys: {a: !!timestamp 2002-01-01}\n",
		"a date under an alias":         "name: &d 2001-12-14\nplain: {k: *d}\nany: *d\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if r.walkErr != nil || r.treeErr != nil {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile != nil {
				t.Error("the decode built a tree")
			}
		})
	}
}

// TestAValueThatDoesNotFitKeepsTheWalk reads values the tree decoder refuses
// for their field, and requires the tree's error and the value it leaves,
// without the decode building a tree. Both keep the first error, write what
// decoded in place and decode the fields after it.
func TestAValueThatDoesNotFitKeepsTheWalk(t *testing.T) {
	for name, src := range map[string]string{
		"text into an int":            "leaf: {i: x}\n",
		"a negative into a uint":      "leaf: {u: -1}\n",
		"a number into a bool":        "leaf: {b: 1}\n",
		"an int that overflows":       "leaf: {i: 9999999999999999999999}\n",
		"later fields decode":         "leaf: {i: x, s: after}\nname: n\n",
		"the first error is kept":     "leaf: {i: x}\nname: [1]\n",
		"a mapping into a string":     "name: {a: 1}\nleaf: {s: after}\n",
		"a sequence into a struct":    "leaf: [1]\nname: n\n",
		"a date into an int":          "leaf: {i: 2001-12-14, s: after}\n",
		"a date into a slice element": "leaf: {xs: [1, 2001-12-14, 3]}\n",
		"a tag that is no timestamp":  "name: !!timestamp nope\nleaf: {s: after}\n",
		"a scalar into a struct":      "leaf: 1\nname: n\n",
		"a sequence into a map value": "plain: {k: [1], j: v}\n",
		"in a slice element":          "list: [{s: a}, {i: x}, {s: c}]\n",
		"in a map of structs":         "byName: {a: {s: new, i: x}, b: {s: b}}\n",
		"in a slice in a map":         "counts: {a: [1, x, 3]}\n",
		"in a pointer":                "ptr: {s: new, i: x}\n",
		"under an alias":              "ptr: &p {i: x}\nleaf: *p\n",
		"in a merge":                  "leaf: {<<: {i: x}, s: own}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if r.walkErr == nil || r.treeErr == nil {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr.Error() != r.treeErr.Error() {
				t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths fill the value differently:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile != nil {
				t.Error("the decode built a tree")
			}
		})
	}
}
