// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec/internal/structfields"
	yamlerrors "github.com/go-openapi/go-yaml/errors"

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

// TestEmbeddedWalkMatchesTheTree reads each document into embedded, whose WalkLeaf fields are inlined, on the walk
// and on the tree, and requires the same Go value.
func TestEmbeddedWalkMatchesTheTree(t *testing.T) {
	for tc := range embeddedWalkMatchesTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var walked embedded
			walkErr := Unmarshal([]byte(tc.src), &walked)

			var built embedded
			treeErr := NewDecoder(
				bytes.NewReader([]byte(tc.src)),
				treeOnly(),
			).Decode(&built)

			require.Equal(t, treeErr == nil, walkErr == nil, "walk err=%v, tree err=%v", walkErr, treeErr)
			assert.Equal(t, built, walked, "the two paths disagree")
		})
	}
}

// embeddedWalkMatchesTheTreeCase holds a case name and a document that writes the outer or the embedded fields.
type embeddedWalkMatchesTheTreeCase struct {
	name string
	src  string
}

func embeddedWalkMatchesTheTreeCases() iter.Seq[embeddedWalkMatchesTheTreeCase] {
	return slices.Values([]embeddedWalkMatchesTheTreeCase{
		{name: "embedded only", src: "s: inner\n"},
		{name: "neither", src: "nope: 1\n"},
		{name: "outer and embedded", src: "s: outer\ni: 3\nxs: [1, 2]\nextra: e\n"},
		{name: "outer only", src: "extra: e\n"},
	})
}

// TestTypedWalkMatchesTheTree reads each document both ways and requires the
// same Go value. The walk fills the destination as the parse hands the document
// over; the tree decoder builds the document first and reads it after.
func TestTypedWalkMatchesTheTree(t *testing.T) {
	for tc := range typedWalkMatchesTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var walked walkShape
			walkErr := Unmarshal([]byte(tc.src), &walked)

			var built walkShape
			treeErr := NewDecoder(
				bytes.NewReader([]byte(tc.src)),
				treeOnly(),
			).Decode(&built)

			require.Equal(t, treeErr == nil, walkErr == nil, "walk err=%v, tree err=%v", walkErr, treeErr)
			assert.Equal(t, built, walked, "the two paths disagree")
		})
	}
}

// typedWalkMatchesTheTreeCase holds a case name and a document the walk and the tree both read.
type typedWalkMatchesTheTreeCase struct {
	name string
	src  string
}

//nolint:dupl // aTagKeepsTheWalkCases has rows of the same shape and length; no two rows are the same
func typedWalkMatchesTheTreeCases() iter.Seq[typedWalkMatchesTheTreeCase] {
	return slices.Values([]typedWalkMatchesTheTreeCase{
		{name: "any beside typed", src: "name: x\nany:\n  deep:\n    deeper: [1, 2]\nleaf:\n  s: y\n"},
		{name: "any empty list", src: "any: []\n"},
		{name: "any empty map", src: "any: {}\n"},
		{name: "any mapping", src: "any:\n  a: 1\n  b: [x, y]\n"},
		{name: "any null", src: "any: ~\n"},
		{name: "any scalar", src: "any: hello\n"},
		{name: "any sequence", src: "any: [1, two, {k: v}]\n"},
		{name: "empty mapping", src: "leaf: {}\n"},
		{name: "empty sequence", src: "leaf:\n  xs: []\n"},
		{name: "exponent", src: "leaf:\n  f: 1e3\n"},
		{name: "flow", src: "leaf: {s: flow, i: 1, xs: [9]}\n"},
		{name: "list", src: "list:\n  - s: one\n  - s: two\n    i: 2\n"},
		{name: "list of any", src: "list2:\n  - 1\n  - k: v\n  - [a, b]\n"},
		{name: "map of any", src: "anys:\n  a: 1\n  b:\n    c: [1, 2]\n  d: text\n"},
		{name: "map of slices", src: "counts:\n  a: [1, 2]\n  b: []\n"},
		{name: "map of structs", src: "byName:\n  a:\n    s: aa\n  b:\n    i: 3\n"},
		{name: "missing fields", src: "name: only\n"},
		{name: "negative", src: "leaf:\n  i: -9223372036854775808\n"},
		{name: "non-string key", src: "name: x\n1: skipped\nleaf:\n  s: y\n"},
		{name: "null value", src: "name: ~\nleaf: null\n"},
		{name: "plain map", src: "plain:\n  k: v\n  k2: v2\n"},
		{name: "pointer null", src: "ptr: ~\n"},
		{name: "pointer set", src: "ptr:\n  s: inner\n  p: here\n"},
		{name: "quoted number", src: "leaf:\n  s: \"12\"\n"},
		{name: "scalars", src: "name: top\nleaf:\n  s: text\n  i: -12\n  u: 7\n  f: 1.5\n  b: true\n  xs: [1, 2, 3]\n"},
		{name: "unknown key", src: "name: x\nnope: 1\nleaf:\n  s: y\n"},
	})
}

// TestWalkableTypeRefusesWhatTheTreeReads holds the gate to the destinations it
// must keep off the walk. Deciding on the type rather than on the document is
// what keeps a refusal from costing a parse: a walk that gives up halfway has
// read the document for nothing.
func TestWalkableTypeRefusesWhatTheTreeReads(t *testing.T) {
	for tc := range walkableTypeRefusedCases() {
		assert.Falsef(t, walkableType(tc.typ, structfields.YAMLTags), "%s: the gate let %s through", tc.name, tc.typ)
	}

	for tc := range walkableTypeAcceptedCases() {
		assert.Truef(t, walkableType(tc.typ, structfields.YAMLTags), "%s: the gate refused %s", tc.name, tc.typ)
	}
}

// walkableTypeRefusesWhatTheTreeReadsCase holds a case name and a destination type for the gate.
type walkableTypeRefusesWhatTheTreeReadsCase struct {
	name string
	typ  reflect.Type
}

func walkableTypeRefusedCases() iter.Seq[walkableTypeRefusesWhatTheTreeReadsCase] {
	return slices.Values([]walkableTypeRefusesWhatTheTreeReadsCase{
		{name: "a RawMessage", typ: reflect.TypeFor[struct{ A RawMessage }]()},
		{name: "a channel", typ: reflect.TypeFor[struct{ A chan int }]()},
		{name: "a NodeUnmarshaler in an array", typ: reflect.TypeFor[struct{ A [2]walkNodeHook }]()},
		// A NodeUnmarshaler and an ast.Node field may keep the node they receive, which the walk recycles.
		{name: "a NodeUnmarshaler", typ: reflect.TypeFor[struct{ A walkNodeHook }]()},
		{name: "an ast.Node field", typ: reflect.TypeFor[struct{ A ast.Node }]()},
		// A self-decoding inline field reads the whole mapping, which the walk passes entry by entry.
		{name: "an inline field that decodes itself", typ: reflect.TypeFor[struct {
			A walkBytesHook `yaml:",inline"`
		}]()},
	})
}

// The hooks the gate sorts: each walkXHook implements one interface of the decoder's.
type (
	walkBytesHook struct{ text string }
	walkFuncHook  struct{ v any }
	walkTextHook  struct{ text string }
	walkNodeHook  struct{ node ast.Node }
)

func (h *walkBytesHook) UnmarshalYAML(b []byte) error { h.text = string(b); return nil }

func (h *walkFuncHook) UnmarshalYAML(unmarshal func(any) error) error { return unmarshal(&h.v) }

func (h *walkTextHook) UnmarshalText(b []byte) error { h.text = string(b); return nil }

func (h *walkNodeHook) UnmarshalYAML(n ast.Node) error { h.node = n; return nil }

func walkableTypeAcceptedCases() iter.Seq[walkableTypeRefusesWhatTheTreeReadsCase] {
	return slices.Values([]walkableTypeRefusesWhatTheTreeReadsCase{
		{name: "a map of any", typ: reflect.TypeFor[struct{ A map[string]any }]()},
		{name: "a nested any", typ: reflect.TypeFor[struct{ A []map[string]any }]()},
		{name: "a self-embedding", typ: reflect.TypeFor[selfEmbedding]()},
		{name: "an any field", typ: reflect.TypeFor[struct{ A any }]()},
		{name: "an embedded struct", typ: reflect.TypeFor[embedded]()},
		{name: "an interface slice", typ: reflect.TypeFor[struct{ A []any }]()},
		{name: "nested maps", typ: reflect.TypeFor[struct{ A map[string][]map[string]int }]()},
		{name: "plain scalars", typ: reflect.TypeFor[WalkLeaf]()},
		{name: "the shape above", typ: reflect.TypeFor[walkShape]()},
		// The walk keeps the node of each of these, and the tree decoder reads that one node.
		{name: "a MapSlice", typ: reflect.TypeFor[struct{ A MapSlice }]()},
		{name: "a time.Duration", typ: reflect.TypeFor[struct{ A time.Duration }]()},
		{name: "a time.Time", typ: reflect.TypeFor[struct{ A time.Time }]()},
		{name: "a bytes Unmarshaler", typ: reflect.TypeFor[struct{ A walkBytesHook }]()},
		{name: "a GoYAMLUnmarshaler", typ: reflect.TypeFor[struct{ A walkFuncHook }]()},
		{name: "a TextUnmarshaler", typ: reflect.TypeFor[struct{ A *walkTextHook }]()},
		{name: "a time.Time at the root", typ: reflect.TypeFor[time.Time]()},
		// The walk keeps the node of each of these too.
		{name: "a complex number", typ: reflect.TypeFor[struct{ A complex128 }]()},
		{name: "a map keyed by an int", typ: reflect.TypeFor[struct{ A map[int]string }]()},
		{name: "an array", typ: reflect.TypeFor[struct{ A [3]int }]()},
		{name: "a narrower interface", typ: reflect.TypeFor[struct{ A fmt.Stringer }]()},
	})
}

// TestWalkPreservesDefaults checks a null leaving the destination as the caller
// set it, which is what the tree decoder does by writing the default over a
// fresh zero.
func TestWalkPreservesDefaults(t *testing.T) {
	dst := walkShape{Name: "default", Leaf: WalkLeaf{S: "kept"}}
	require.NoError(t, Unmarshal([]byte("leaf:\n"), &dst))
	assert.Equal(t, "default", dst.Name)
	assert.Equal(t, "kept", dst.Leaf.S)
}

// TestAnAnchorKeepsTheWalk reads documents whose values carry an anchor, and
// requires the value the tree gives without the decode building one.
//
// The walk handed every anchor to the tree, which parses the whole source again:
// citm_catalog into its Go types peaked at 0.69 MiB, and at 19.5 MiB with one
// anchor written anywhere in it.
func TestAnAnchorKeepsTheWalk(t *testing.T) {
	for tc := range anAnchorKeepsTheWalkCases() {
		t.Run(tc.name, func(t *testing.T) {
			var walked walkShape
			dec := NewDecoder(bytes.NewReader([]byte(tc.src)))
			walkErr := dec.Decode(&walked)

			var built walkShape
			treeErr := NewDecoder(
				bytes.NewReader([]byte(tc.src)),
				treeOnly(),
			).Decode(&built)

			require.NoError(t, walkErr, "walk")
			require.NoError(t, treeErr, "tree")
			assert.Equal(t, built, walked, "the two paths disagree")
			assert.Nil(t, dec.parsedFile, "the decode built a tree")
		})
	}
}

// anAnchorKeepsTheWalkCase holds a case name and a document whose values carry an anchor.
type anAnchorKeepsTheWalkCase struct {
	name string
	src  string
}

func anAnchorKeepsTheWalkCases() iter.Seq[anAnchorKeepsTheWalkCase] {
	return slices.Values([]anAnchorKeepsTheWalkCase{
		{name: "a block mapping", src: "leaf: &l\n  s: text\n  i: 3\n"},
		{name: "a field no field claims", src: "nope: &n {k: v}\nname: x\n"},
		{name: "a flow mapping", src: "leaf: &l {s: text, i: 3}\n"},
		{name: "a map value", src: "byName:\n  a: &a {s: aa}\n  b: {i: 3}\n"},
		{name: "a plain map value", src: "plain:\n  k: &k v\n"},
		{name: "a pointer", src: "ptr: &p {s: inner}\n"},
		{name: "a scalar", src: "name: &n top\n"},
		{name: "a scalar in a sequence", src: "leaf:\n  xs: [1, &x 2, 3]\n"},
		{name: "a sequence", src: "leaf:\n  xs: &x [1, 2]\n"},
		{name: "a sequence entry", src: "list:\n  - &e {s: one}\n  - s: two\n"},
		{name: "a sequence of anchored maps", src: "list: [&a {s: one}, &b {s: two}]\n"},
		{name: "an anchor inside an anchor", src: "leaf: &l {s: &s text}\n"},
		{name: "an anchor on nothing", src: "name: &n\nlist: &l\n"},
		{name: "an anchored null pointer", src: "ptr: &p ~\nleaf: &l ~\n"},
		{name: "an anchored null struct", src: "leaf: &l\nptr: &p\n"},
		{name: "an any", src: "any: &a {k: [1, 2]}\n"},
		{name: "the root", src: "&r\nname: top\n"},
		{name: "two anchors", src: "name: &a x\nleaf: &b {s: y}\n"},
	})
}

// TestAnAnchorTheWalkCannotReadGoesToTheTree pins the anchors the walk still hands
// to the tree, so each one that starts to walk is noticed and moved above.
func TestAnAnchorTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for tc := range anAnchorTheWalkCannotReadGoesToTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var walked walkShape
			dec := NewDecoder(bytes.NewReader([]byte(tc.src)))
			require.NoError(t, dec.Decode(&walked))
			assert.NotNil(t, dec.parsedFile, "the walk read it: move the case to TestAnAnchorKeepsTheWalk")
		})
	}
}

// anAnchorTheWalkCannotReadGoesToTheTreeCase holds a case name and a document with an anchor the walk hands to the tree.
type anAnchorTheWalkCannotReadGoesToTheTreeCase struct {
	name string
	src  string
}

func anAnchorTheWalkCannotReadGoesToTheTreeCases() iter.Seq[anAnchorTheWalkCannotReadGoesToTheTreeCase] {
	return slices.Values([]anAnchorTheWalkCannotReadGoesToTheTreeCase{
		{name: "an alias as a key", src: "name: &n k\nplain: {*n : v}\n"},
		{name: "an alias inside an any", src: "name: &n top\nany: {k: *n}\n"},
		{name: "an anchor inside an any", src: "any: {k: &a v}\n"},
		{name: "an anchored key", src: "&k name: top\n"},
	})
}

// TestADirectiveKeepsTheWalk reads documents opened by a "%YAML" or a "%TAG"
// line, and requires the value the tree gives without the decode building one.
func TestADirectiveKeepsTheWalk(t *testing.T) {
	for tc := range aDirectiveKeepsTheWalkCases() {
		t.Run(tc.name, func(t *testing.T) {
			walked := walkShape{Name: "default"}
			dec := NewDecoder(bytes.NewReader([]byte(tc.src)))
			walkErr := dec.Decode(&walked)

			built := walkShape{Name: "default"}
			treeErr := NewDecoder(
				bytes.NewReader([]byte(tc.src)),
				treeOnly(),
			).Decode(&built)

			require.NoError(t, walkErr, "walk")
			require.NoError(t, treeErr, "tree")
			assert.Equal(t, built, walked, "the two paths disagree")
			assert.Nil(t, dec.parsedFile, "the decode built a tree")
		})
	}
}

// aDirectiveKeepsTheWalkCase holds a case name and a document opened by a directive.
type aDirectiveKeepsTheWalkCase struct {
	name string
	src  string
}

func aDirectiveKeepsTheWalkCases() iter.Seq[aDirectiveKeepsTheWalkCase] {
	return slices.Values([]aDirectiveKeepsTheWalkCase{
		{name: "a directive and no body", src: "%YAML 1.2\n---\n"},
		{name: "a tag handle", src: "%TAG !e! tag:example.com,2000:\n---\nleaf: {s: x}\n"},
		{name: "a version", src: "%YAML 1.1\n---\nname: x\n"},
	})
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
	for _, share := range []bool{false, true} {
		for tc := range anAliasKeepsTheWalkCases() {
			t.Run(fmt.Sprintf("share=%v/%s", share, tc.name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}

				var walked walkShape
				dec := NewDecoder(bytes.NewReader([]byte(tc.src)), opts...)
				walkErr := dec.Decode(&walked)

				var built walkShape
				treeErr := NewDecoder(
					bytes.NewReader([]byte(tc.src)),
					append(opts, treeOnly())...,
				).Decode(&built)

				require.NoError(t, walkErr, "walk")
				require.NoError(t, treeErr, "tree")
				assert.Equal(t, built, walked, "the two paths disagree")
				assert.Nil(t, dec.parsedFile, "the decode built a tree")
			})
		}
	}
}

// anAliasKeepsTheWalkCase holds a case name and a document whose aliases name an earlier anchor.
type anAliasKeepsTheWalkCase struct {
	name string
	src  string
}

func anAliasKeepsTheWalkCases() iter.Seq[anAliasKeepsTheWalkCase] {
	return slices.Values([]anAliasKeepsTheWalkCase{
		{name: "a block anchor", src: "leaf: &l\n  s: x\n  i: 2\nptr: *l\n"},
		{name: "a pointer into a struct", src: "ptr: &p {s: x}\nleaf: *p\n"},
		{name: "a scalar", src: "name: &n top\nleaf:\n  s: *n\n"},
		{name: "a scalar deeper down", src: "name: &n top\nlist: [{s: *n}]\n"},
		{name: "a sequence", src: "counts: {a: &c [1, 2]}\nleaf: {xs: *c}\n"},
		{name: "a struct holding a slice", src: "leaf: &l {xs: [1, 2]}\nptr: *l\n"},
		{name: "a struct into a pointer", src: "leaf: &l {s: x, i: 1}\nptr: *l\n"},
		{name: "an alias to an alias", src: "name: &a x\nleaf: &l {s: *a}\nptr: *l\n"},
		{name: "an anchor inside an anchor", src: "leaf: &l {s: &s x}\nname: *s\n"},
		{name: "an anchor no field claims", src: "nope: &n {k: v}\nplain: *n\n"},
		{name: "into a slice of any", src: "leaf: &l {s: x}\nlist2: [*l]\n"},
		{name: "into an any", src: "leaf: &l {s: x}\nany: *l\n"},
		{name: "into another map type", src: "plain: &p {k: v}\nanys: *p\n"},
		{name: "into map values", src: "leaf: &l {s: x}\nbyName: {a: *l, b: *l}\n"},
		{name: "twice into a slice", src: "leaf: &l {s: x}\nlist: [*l, *l]\n"},
	})
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
		require.NoError(t, NewDecoder(bytes.NewReader([]byte(src)), append(opts, ShareAliases())...).Decode(&v))

		return v
	}
	walked := read()
	built := read(treeOnly())

	for _, c := range []struct {
		name       string
		walk, tree bool
	}{
		{"p points at a", walked.P == &walked.A, built.P == &built.A},
		{"p and q are one value", walked.P == walked.Q, built.P == built.Q},
		{"lp points at the slice element", len(walked.L) > 0 && walked.LP == &walked.L[0], len(built.L) > 0 && built.LP == &built.L[0]},
	} {
		assert.Equal(t, c.tree, c.walk, c.name)
	}
	assert.Equal(t, built, walked, "the two paths disagree")
}

// TestABrokenAliasIsRefusedAsByTheTree holds the walk to the tree's error on an
// alias that names nothing it can read. The parse refuses an alias to an anchor
// written after it; one inside its own anchor goes to the tree, which refuses
// the cycle.
func TestABrokenAliasIsRefusedAsByTheTree(t *testing.T) {
	for tc := range aBrokenAliasIsRefusedAsByTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var walked, built walkShape
			walkErr := NewDecoder(bytes.NewReader([]byte(tc.src))).Decode(&walked)
			treeErr := NewDecoder(
				bytes.NewReader([]byte(tc.src)),
				treeOnly(),
			).Decode(&built)

			require.Error(t, walkErr, "walk")
			require.Error(t, treeErr, "tree")
			assert.EqualError(t, walkErr, treeErr.Error(), "the two paths refuse differently")
		})
	}
}

// aBrokenAliasIsRefusedAsByTheTreeCase holds a case name and a document with an alias that names nothing it can read.
type aBrokenAliasIsRefusedAsByTheTreeCase struct {
	name string
	src  string
}

func aBrokenAliasIsRefusedAsByTheTreeCases() iter.Seq[aBrokenAliasIsRefusedAsByTheTreeCase] {
	return slices.Values([]aBrokenAliasIsRefusedAsByTheTreeCase{
		{name: "an alias inside its anchor", src: "leaf: &l {s: x, xs: *l}\n"},
		{name: "an alias to a later anchor", src: "leaf:\n  s: *n\nname: &n top\n"},
	})
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
		append(opts, treeOnly())...,
	).Decode(&r.built)

	return r
}

// TestAMergeKeepsTheWalk reads documents whose "<<" merges into a struct or a
// typed map, and requires the value the tree gives without the decode building
// one. A mapping's own entries win on either side of the "<<", and the earlier
// mapping of a merge sequence wins over a later one.
func TestAMergeKeepsTheWalk(t *testing.T) {
	for _, share := range []bool{false, true} {
		for tc := range aMergeKeepsTheWalkCases() {
			t.Run(fmt.Sprintf("share=%v/%s", share, tc.name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, tc.src, opts...)
				require.NoError(t, r.walkErr, "walk")
				require.NoError(t, r.treeErr, "tree")
				assert.Equal(t, r.built, r.walked, "the two paths disagree")
				assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
			})
		}
	}
}

// aMergeKeepsTheWalkCase holds a case name and a document whose "<<" the walk reads.
type aMergeKeepsTheWalkCase struct {
	name string
	src  string
}

func aMergeKeepsTheWalkCases() iter.Seq[aMergeKeepsTheWalkCase] {
	return slices.Values([]aMergeKeepsTheWalkCase{
		{name: "a block mapping", src: "ptr: &b\n  s: base\n  i: 1\nleaf:\n  <<: *b\n  s: own\n"},
		{name: "a mapping in place", src: "leaf: {<<: {s: base, i: 1}, s: own}\n"},
		{name: "a merge inside a merged anchor", src: "ptr: &a {i: 7}\nlist: [&b {<<: *a, s: b}]\nleaf: {<<: *b}\n"},
		{name: "a merge into an any", src: "any: {<<: {a: 1}, b: 2}\n"},
		{name: "a merge no field claims", src: "nope: {<<: {s: x}}\nname: m\n"},
		{name: "a merged slice", src: "ptr: &b {xs: [1, 2]}\nleaf: {<<: *b}\nlist: [{<<: *b, xs: [3]}]\n"},
		{name: "a sequence in place", src: "ptr: &a {s: a}\nleaf: {<<: [{i: 3, s: first}, *a]}\n"},
		{name: "a sequence naming a merger", src: "ptr: &a {<<: {s: zero}, s: own}\nleaf: {<<: [*a]}\n"},
		{name: "a sequence of aliases", src: "ptr: &a {s: a}\nlist: [&b {s: b, i: 2}]\nleaf: {<<: [*a, *b]}\n"},
		{name: "an alias after the own keys", src: "ptr: &b {s: base, i: 1}\nleaf: {s: own, <<: *b}\n"},
		{name: "an alias before the own keys", src: "ptr: &b {s: base, i: 1}\nleaf: {<<: *b, s: own}\n"},
		{name: "an aliased mapping that merges", src: "ptr: &a {i: 7}\nleaf: &l {<<: *a, s: l}\nlist: [*l]\n"},
		{name: "an own struct replaces a merge", src: "<<: {leaf: {s: base, i: 1}}\nleaf: {s: own}\n"},
		{name: "into a map", src: "plain: {<<: {a: x, b: w}, a: own}\n"},
		{name: "into a map of structs", src: "ptr: &b {s: base}\nbyName: {<<: {k: *b, j: {s: no}}, j: {s: j}}\n"},
	})
}

// TestAMergeTheWalkCannotReadGoesToTheTree pins the merges the walk hands to
// the tree, and requires the tree's value or the tree's error from both.
func TestAMergeTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for tc := range aMergeTheWalkCannotReadGoesToTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.Equal(t, r.treeErr == nil, r.walkErr == nil, "walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			if r.walkErr != nil {
				assert.EqualError(t, r.walkErr, r.treeErr.Error(), "the two paths refuse differently")

				return
			}
			assert.Equal(t, r.built, r.walked, "the two paths disagree")
			assert.NotNil(t, r.dec.parsedFile, "the walk read it: move the case to TestAMergeKeepsTheWalk")
		})
	}
}

// aMergeTheWalkCannotReadGoesToTheTreeCase holds a case name and a document whose "<<" the walk hands to the tree.
type aMergeTheWalkCannotReadGoesToTheTreeCase struct {
	name string
	src  string
}

func aMergeTheWalkCannotReadGoesToTheTreeCases() iter.Seq[aMergeTheWalkCannotReadGoesToTheTreeCase] {
	return slices.Values([]aMergeTheWalkCannotReadGoesToTheTreeCase{
		{name: "a merge of a scalar", src: "name: &n top\nleaf: {<<: *n}\n"},
		{name: "a null merge", src: "leaf: {<<: }\n"},
		{name: "a sequence holding a scalar", src: "leaf: {<<: [{s: x}, 1]}\n"},
		{name: "a sequence holding a sequence", src: "leaf: {<<: [[{s: x}]]}\n"},
		{name: "a struct with an alias field", src: "ptr: &b {s: base}\naliased: {<<: *b, extra: e}\n"},
		{name: "an anchor named again before the close", src: "name: &x one\nleaf: {<<: {s: *x}, p: &x two}\n"},
		{name: "an anchored merge", src: "leaf: {<<: &m {s: x}}\nptr: *m\n"},
	})
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
		require.NoError(t, NewDecoder(bytes.NewReader([]byte(src)), opts...).Decode(&v))

		return v
	}
	walked := read()
	built := read(treeOnly())
	assert.Equal(t, built, walked, "the two paths disagree")
}

// declaredAnchors parses each source into the tree parser.WithAnchors takes.
func declaredAnchors(t *testing.T, srcs map[string]string) map[string]ast.Node {
	t.Helper()
	anchors := make(map[string]ast.Node, len(srcs))
	for name, src := range srcs {
		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
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
	for _, share := range []bool{false, true} {
		for tc := range aDeclaredAnchorKeepsTheWalkCases() {
			t.Run(fmt.Sprintf("share=%v/%s", share, tc.name), func(t *testing.T) {
				opts := []DecodeOption{WithParserOptions(parser.WithAnchors(anchors))}
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, tc.src, opts...)
				require.NoError(t, r.walkErr, "walk")
				require.NoError(t, r.treeErr, "tree")
				assert.Equal(t, r.built, r.walked, "the two paths disagree")
				assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
			})
		}
	}
}

// aDeclaredAnchorKeepsTheWalkCase holds a case name and a document whose aliases name a declared anchor.
type aDeclaredAnchorKeepsTheWalkCase struct {
	name string
	src  string
}

func aDeclaredAnchorKeepsTheWalkCases() iter.Seq[aDeclaredAnchorKeepsTheWalkCase] {
	return slices.Values([]aDeclaredAnchorKeepsTheWalkCase{
		{name: "a map value", src: "byName: {a: *l}\n"},
		{name: "a merge", src: "leaf: {<<: *l, s: own}\n"},
		{name: "a merge sequence", src: "leaf: {<<: [{s: first}, *l]}\n"},
		{name: "a pointer", src: "ptr: *l\n"},
		{name: "a scalar", src: "name: *n\nleaf: {s: *n}\n"},
		{name: "a sequence", src: "counts: {a: *c}\n"},
		{name: "a slice element", src: "list: [*l, {s: own}]\n"},
		{name: "a struct", src: "leaf: *l\n"},
		{name: "a tag in the declared tree", src: "name: *t\n"},
		{name: "an anchor named after a merge", src: "leaf: {<<: *l}\nname: &l mine\nplain: {k: *l}\n"},
		{name: "the document's anchor wins", src: "name: &n mine\nleaf: {s: *n}\n"},
		{name: "twice", src: "leaf: *l\nptr: *l\n"},
	})
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
	for tc := range aDeclaredAnchorIsNeverSharedCases() {
		t.Run(tc.name, func(t *testing.T) {
			var v shared
			opts := append([]DecodeOption{WithParserOptions(parser.WithAnchors(anchors)), ShareAliases()}, tc.extra...)
			require.NoError(t, NewDecoder(bytes.NewReader([]byte(src)), opts...).Decode(&v))
			require.NotNil(t, v.P, "p decoded to nil")
			require.NotNil(t, v.Q, "q decoded to nil")
			assert.NotSame(t, v.P, v.Q, "two aliases of a declared anchor share a value")
			v.P.Xs[0] = 9
			assert.Equal(t, int64(1), v.Q.Xs[0], "writing through one alias reached the other")
		})
	}
}

// aDeclaredAnchorIsNeverSharedCase holds a decode path and the options that select it.
type aDeclaredAnchorIsNeverSharedCase struct {
	name  string
	extra []DecodeOption
}

func aDeclaredAnchorIsNeverSharedCases() iter.Seq[aDeclaredAnchorIsNeverSharedCase] {
	return slices.Values([]aDeclaredAnchorIsNeverSharedCase{
		{name: "walk"},
		{name: "tree", extra: []DecodeOption{treeOnly()}},
	})
}

// TestADeclaredAnchorTheWalkCannotReadGoesToTheTree pins the declared anchors
// the walk hands to the tree, and requires the tree's value or error from both.
func TestADeclaredAnchorTheWalkCannotReadGoesToTheTree(t *testing.T) {
	anchors := declaredAnchors(t, map[string]string{"l": "{s: declared}\n"})
	anchors["nothing"] = nil
	for tc := range aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src, WithParserOptions(parser.WithAnchors(anchors)))
			require.Equal(t, r.treeErr == nil, r.walkErr == nil, "walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			if r.walkErr != nil {
				assert.EqualError(t, r.walkErr, r.treeErr.Error(), "the two paths refuse differently")

				return
			}
			assert.Equal(t, r.built, r.walked, "the two paths disagree")
			assert.NotNil(t, r.dec.parsedFile, "the walk read it: move the case to TestADeclaredAnchorKeepsTheWalk")
		})
	}
}

// aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCase holds a case name and a document with a declared anchor the walk hands to the tree.
type aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCase struct {
	name string
	src  string
}

func aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCases() iter.Seq[aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCase] {
	return slices.Values([]aDeclaredAnchorTheWalkCannotReadGoesToTheTreeCase{
		{name: "a declaration with no node", src: "name: *nothing\n"},
		{name: "an alias inside an any", src: "any: {k: *l}\n"},
		{name: "an anchor named again while a merge waits", src: "leaf: {<<: {s: *l}, p: &l mine}\n"},
	})
}

// TestUseStringKeysWalksAndAgreesWithTheTree decodes under UseStringKeys into a bare "any", a struct holding an "any",
// and maps keyed by any type, on the walk and on the tree. Both paths name each key of an "any" mapping by its text,
// so "1: x" gives the key "1", and both reject "1" and "\"1\"" in one mapping as a repeat.
func TestUseStringKeysWalksAndAgreesWithTheTree(t *testing.T) {
	type doc struct {
		K   any            `yaml:"k"`
		Any map[any]string `yaml:"any"`
		Str map[string]any `yaml:"str"`
	}
	for tc := range useStringKeysCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, into := range []func() any{func() any { return new(any) }, func() any { return new(doc) }} {
				walked, built := into(), into()
				dec := NewDecoder(bytes.NewReader([]byte(tc.src)), UseStringKeys())
				walkErr := dec.Decode(walked)
				treeErr := NewDecoder(bytes.NewReader([]byte(tc.src)), UseStringKeys(), treeOnly()).Decode(built)
				require.Equalf(t, treeErr == nil, walkErr == nil, "%T: walk err=%v, tree err=%v", walked, walkErr, treeErr)
				assert.Equalf(t, built, walked, "%T: the two paths disagree", walked)
				if _, isStruct := walked.(*doc); !isStruct || !tc.anchored {
					// An anchor inside an "any" field sends the decode to the tree. See errNeedsTheTree.
					assert.NotNilf(t, dec.stream, "%T was not walked", walked)
				}
			}
		})
	}

	var v doc
	require.NoError(t, NewDecoder(bytes.NewReader([]byte("k: {1: x, 1.50: y, true: z}\n")), UseStringKeys()).Decode(&v))
	assert.Equal(t, map[string]any{"1": "x", "1.5": "y", "true": "z"}, v.K)
}

type useStringKeysCase struct {
	name     string
	src      string
	anchored bool
}

func useStringKeysCases() iter.Seq[useStringKeysCase] {
	return slices.Values([]useStringKeysCase{
		{name: "keys of every type", src: "k: {1: x, true: y, null: z, 0x10: w}\nany: {2: a}\nstr: {3: {4: b}}\n"},
		{name: "a repeat by name", src: "k: {1: x, \"1\": y}\n"},
		{name: "a repeat by name at the root", src: "1: x\n\"1\": y\n"},
		{name: "nested", src: "k: [{1: [{2: x}]}]\n"},
		{name: "a merge", src: "k: {base: &b {1: x}, m: {<<: *b, 2: y}}\n", anchored: true},
		{name: "an alias as a key", src: "k: {a: &n 7, m: {*n : x}}\n", anchored: true},
		{name: "a collection as a key", src: "k: {[1, 2]: x}\n"},
		{name: "a timestamp key", src: "k: {2001-12-14: x}\n"},
	})
}

// TestUseOrderedMapWalksAndAgreesWithTheTree decodes under UseOrderedMap into a bare "any" and into a struct holding
// one, on the walk and on the tree, alone and with the options that change how a mapping is keyed or shared. Each
// mapping becomes a MapSlice: its own entries in document order, then the ones a "<<" brings in.
func TestUseOrderedMapWalksAndAgreesWithTheTree(t *testing.T) {
	type doc struct {
		K   any            `yaml:"k"`
		Str map[string]any `yaml:"str"`
	}
	// "<<" merges under YAML 1.1 only, and is a plain key under 1.2.
	yaml11 := WithParserOptions(parser.WithYAMLVersion(parser.YAML11))
	optionSets := map[string][]DecodeOption{
		"alone":                {UseOrderedMap()},
		"1.1":                  {UseOrderedMap(), yaml11},
		"1.1, UseStringKeys":   {UseOrderedMap(), yaml11, UseStringKeys()},
		"1.1, ShareAliases":    {UseOrderedMap(), yaml11, ShareAliases()},
		"AllowDuplicateMapKey": {UseOrderedMap(), yaml11, AllowDuplicateMapKey()},
	}
	for tc := range useOrderedMapCases() {
		t.Run(tc.name, func(t *testing.T) {
			for name, opts := range optionSets {
				for _, into := range []func() any{func() any { return new(any) }, func() any { return new(doc) }} {
					walked, built := into(), into()
					dec := NewDecoder(bytes.NewReader([]byte(tc.src)), opts...)
					walkErr := dec.Decode(walked)
					treeOpts := append(slices.Clone(opts), treeOnly())
					treeErr := NewDecoder(bytes.NewReader([]byte(tc.src)), treeOpts...).Decode(built)
					require.Equalf(t, treeErr == nil, walkErr == nil, "%s, %T: walk err=%v, tree err=%v", name, walked, walkErr, treeErr)
					assert.Equalf(t, built, walked, "%s, %T: the two paths disagree", name, walked)
					if _, isStruct := walked.(*doc); !isStruct || !tc.anchored {
						// An anchor inside an "any" field sends the decode to the tree. See errNeedsTheTree.
						assert.NotNilf(t, dec.stream, "%s, %T was not walked", name, walked)
					}
				}
			}
		})
	}

	var v any
	require.NoError(t, NewDecoder(bytes.NewReader([]byte("k: {z: 1, <<: {a: 2, z: 3}, m: 4}\n")), UseOrderedMap(), yaml11).Decode(&v))
	inner, err := NewMapSlice(MapItem{Key: "z", Value: uint64(1)}, MapItem{Key: "m", Value: uint64(4)}, MapItem{Key: "a", Value: uint64(2)})
	require.NoError(t, err)
	outer, err := NewMapSlice(MapItem{Key: "k", Value: inner})
	require.NoError(t, err)
	assert.Equal(t, outer, v)
}

type useOrderedMapCase struct {
	name     string
	src      string
	anchored bool
}

func useOrderedMapCases() iter.Seq[useOrderedMapCase] {
	return slices.Values([]useOrderedMapCase{
		{name: "order kept", src: "k: {z: 1, a: 2, m: 3}\nstr: {b: {y: 1, x: 2}}\n"},
		{name: "empty", src: "k: {}\nstr: {}\n"},
		{name: "keys of every type", src: "k: {1: x, true: y, null: z, 2001-12-14: t, 1.5: f}\n"},
		{name: "a repeat", src: "k: {a: 1, b: 2, a: 3}\n"},
		{name: "a repeat by name", src: "k: {1: x, \"1\": y}\n"},
		{name: "a merge written first", src: "k: {<<: {a: 1, b: 2}, b: 3, c: 4}\n"},
		{name: "a merge written last", src: "k: {b: 3, c: 4, <<: {a: 1, b: 2}}\n"},
		{name: "a merge alone", src: "k: {<<: {a: 1}}\n"},
		{name: "a merge of nothing", src: "k: {<<: {}}\n"},
		{name: "a merge of a sequence", src: "k: {<<: [{a: 1, b: 2}, {b: 3, c: 4}], d: 5}\n"},
		{name: "a merge of a null", src: "k: {<<: null, a: 1}\n"},
		{name: "a merge of a scalar", src: "k: {<<: 1, a: 1}\n"},
		{name: "a nested merge", src: "k: {<<: {<<: {x: 1, y: 1}, y: 2}, z: 3}\n"},
		{name: "an aliased merge", src: "k: {base: &b {a: 1, b: 2}, m: {b: 3, <<: *b}}\n", anchored: true},
		{name: "an aliased merge of a sequence", src: "k: {p: &p {a: 1}, q: &q {a: 2, b: 2}, m: {<<: [*p, *q]}}\n", anchored: true},
		{name: "an alias copied", src: "k: {a: &a {x: [1, {y: 2}]}, b: *a, c: *a}\n", anchored: true},
		{name: "a merge of a timestamp key", src: "k: {<<: {2001-12-14: a}, 2001-12-14T00:00:00Z: b}\n"},
		{name: "a tagged map", src: "k: !!map {b: 1, a: 2}\n"},
		{name: "a set", src: "k: !!set {b, a}\n"},
		{name: "an omap", src: "k: !!omap [{b: 1}, {a: {y: 1, x: 2}}]\n"},
		{name: "an omap of a non-entry", src: "k: !!omap [{b: 1, a: 2}]\n"},
		{name: "a mapping as a key", src: "k: {{a: 1}: x}\n"},
		{name: "a sequence of mappings", src: "k: [{b: 1, a: 2}, {}]\n"},
		{name: "a root mapping", src: "z: 1\na: 2\n"},
	})
}

// TestValidatorWalksAndAgreesWithTheTree decodes structs a validator rejects, on the walk and on the tree, and
// requires the same values and the same error, position and excerpt included. The walk records the tokens the
// tree reads from the mapping again: a field's value, the key of a mapping or a sequence, the key of the entry
// that writes a struct, and the struct's first key.
func TestValidatorWalksAndAgreesWithTheTree(t *testing.T) {
	yaml11 := WithParserOptions(parser.WithYAMLVersion(parser.YAML11))
	optionSets := map[string][]DecodeOption{
		"alone":                {Validator(validateItem)},
		"1.1":                  {Validator(validateItem), yaml11},
		"1.1, ShareAliases":    {Validator(validateItem), yaml11, ShareAliases()},
		"AllowDuplicateMapKey": {Validator(validateItem), AllowDuplicateMapKey()},
	}
	for tc := range validatorCases() {
		t.Run(tc.name, func(t *testing.T) {
			for name, opts := range optionSets {
				var walked, built validatedDoc
				dec := NewDecoder(bytes.NewReader([]byte(tc.src)), opts...)
				walkErr := dec.Decode(&walked)
				treeOpts := append(slices.Clone(opts), treeOnly())
				treeErr := NewDecoder(bytes.NewReader([]byte(tc.src)), treeOpts...).Decode(&built)
				require.Equalf(t, fmt.Sprint(treeErr), fmt.Sprint(walkErr), "%s: the two paths return different errors", name)
				if !tc.repeats || name == "AllowDuplicateMapKey" {
					// A repeated key stops the document, and the walk has written the fields before it.
					// A Decode that fails may leave its destination partly written.
					assert.Equalf(t, built, walked, "%s: the two paths disagree", name)
				}
				if !tc.anchored {
					assert.NotNilf(t, dec.stream, "%s: the document was not walked", name)
				}
				if tc.fails && (!tc.repeats || name == "AllowDuplicateMapKey") {
					require.ErrorIsf(t, walkErr, yamlerrors.ErrValidation, "%s", name)
				}
			}
		})
	}
}

type validatedSub struct {
	N int `yaml:"n"`
}

type validatedEmbedded struct {
	Inner string `yaml:"inner"`
}

type validatedItem struct {
	validatedEmbedded `yaml:",inline"`

	Name string                    `yaml:"name"`
	Size int                       `yaml:"size"`
	Sub  *validatedSub             `yaml:"sub"`
	Tags []string                  `yaml:"tags"`
	Meta map[string]int            `yaml:"meta"`
	When time.Time                 `yaml:"when"`
	Any  any                       `yaml:"any"`
	Kids []validatedItem           `yaml:"kids"`
	ByID map[string]*validatedItem `yaml:"byid"`
}

type validatedDoc struct {
	validatedItem `yaml:",inline"`

	Item  validatedItem  `yaml:"item"`
	After string         `yaml:"after"`
	Base  *validatedItem `yaml:"base"`
	Other *validatedItem `yaml:"other"`
}

// validatedFieldError names a struct field, as an element of go-playground/validator's error does.
type validatedFieldError struct{ field string }

func (e validatedFieldError) StructField() string { return e.field }

type validatedFieldErrors []validatedFieldError

func (e validatedFieldErrors) Error() string { return "invalid " + e[0].field }

// validateItem rejects a validatedItem by its name: "bad-<Field>" names that field, "plain" returns an error that
// names none, and "bad-Nothing" a field the struct does not have.
func validateItem(v any) error {
	var it validatedItem
	switch t := v.(type) {
	case validatedItem:
		it = t
	case validatedDoc:
		// The root inlines an item, so the validator reads it through the root.
		it = t.validatedItem
	default:
		return nil
	}
	switch {
	case it.Name == "plain":
		return errors.New("plain failure")
	case strings.HasPrefix(it.Name, "bad-"):
		return validatedFieldErrors{{"Missing"}, {strings.TrimPrefix(it.Name, "bad-")}}
	default:
		return nil
	}
}

type validatorCase struct {
	name     string
	src      string
	anchored bool
	fails    bool
	repeats  bool
}

func validatorCases() iter.Seq[validatorCase] {
	return slices.Values([]validatorCase{
		{name: "valid", src: "item: {name: ok, size: 1}\nafter: x\n"},
		{name: "a scalar field", src: "item:\n  name: bad-Size\n  size: 3\nafter: x\n", fails: true},
		{name: "a mapping field", src: "item:\n  name: bad-Sub\n  sub:\n    n: 1\n", fails: true},
		{name: "a sequence field", src: "item:\n  name: bad-Tags\n  tags: [a]\n", fails: true},
		{name: "a map field", src: "item: {name: bad-Meta, meta: {a: 1}}\n", fails: true},
		{name: "a kept field", src: "item:\n  name: bad-When\n  when: 2001-12-14\n", fails: true},
		{name: "an any field", src: "item:\n  name: bad-Any\n  any: {x: [1]}\n", fails: true},
		{name: "an embedded field", src: "item:\n  name: bad-Inner\n  inner: i\n", fails: true},
		{name: "a missing field", src: "item:\n  name: bad-Size\nafter: x\n", fails: true},
		{name: "a missing field at the root", src: "name: bad-Size\nafter: x\n", fails: true},
		{name: "a missing field in a sequence", src: "item:\n  kids:\n    - name: bad-Size\n", fails: true},
		{name: "a missing field in a map", src: "item:\n  byid:\n    k1:\n      name: bad-Size\n", fails: true},
		{name: "a missing field in a flow mapping", src: "item: {kids: [{name: bad-Size}]}\n", fails: true},
		{name: "no field named", src: "item: {name: plain, size: 1}\n", fails: true},
		{name: "an unknown field named", src: "item: {name: bad-Nothing}\n", fails: true},
		{name: "a field written twice", src: "item:\n  name: bad-Size\n  size: 1\n  size: 2\n", fails: true, repeats: true},
		{name: "a null field", src: "item:\n  name: bad-Sub\n  sub:\n", fails: true},
		{name: "an explicit key", src: "item:\n  ? name\n  : bad-Size\n  ? size\n  : 1\n", fails: true},
		{name: "a tagged field", src: "item:\n  name: bad-Size\n  size: !!int 3\n", fails: true},
		{name: "a tagged struct", src: "item: !!map\n  name: bad-Size\n", fails: true},
		{name: "a field error first", src: "item:\n  name: bad-Size\n  size: x\n", fails: false},
		{name: "a nested failure", src: "item:\n  name: bad-Size\n  kids:\n    - name: bad-Tags\n      tags: [t]\n", fails: true},
		{name: "an anchored field", src: "item:\n  name: bad-Sub\n  sub: &s\n    n: 1\n", fails: true, anchored: true},
		{name: "an alias", src: "base: &b {name: bad-Size, size: 2}\nother: *b\n", fails: true},
		{name: "an aliased field", src: "base: &b {name: x, sub: &s {n: 1}}\nitem:\n  name: bad-Sub\n  sub: *s\n", fails: true},
		{name: "a merged field", src: "base: &b {size: 4, tags: [m]}\nitem:\n  name: bad-Size\n  <<: *b\n", fails: true},
		{name: "a merged collection", src: "base: &b {size: 4, tags: [m]}\nitem:\n  <<: *b\n  name: bad-Tags\n", fails: true},
		{name: "an own key over a merge", src: "base: &b {size: 4}\nitem:\n  <<: *b\n  name: bad-Size\n  size: 5\n", fails: true},
		{name: "a merge in place", src: "item:\n  <<: {size: 4}\n  name: bad-Size\n", fails: true},
	})
}

// TestAConvertedScalarKeepsTheWalk reads scalars of another kind than their
// field, and requires the value the tree gives without the decode building a
// tree. The walk asks the tree decoder for the one scalar, so the two convert
// by the same rules.
func TestAConvertedScalarKeepsTheWalk(t *testing.T) {
	for tc := range aConvertedScalarKeepsTheWalkCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.NoError(t, r.walkErr, "walk")
			require.NoError(t, r.treeErr, "tree")
			assert.Equal(t, r.built, r.walked, "the two paths disagree")
			assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
		})
	}
}

// aConvertedScalarKeepsTheWalkCase holds a case name and a document with a scalar of another kind than its field.
type aConvertedScalarKeepsTheWalkCase struct {
	name string
	src  string
}

func aConvertedScalarKeepsTheWalkCases() iter.Seq[aConvertedScalarKeepsTheWalkCase] {
	return slices.Values([]aConvertedScalarKeepsTheWalkCase{
		{name: "a bool into a string", src: "name: true\n"},
		{name: "a date into a map value", src: "plain: {k: 2001-12-14, j: !!timestamp 2002-01-01}\n"},
		{name: "a date into a pointer", src: "leaf: {p: 2001-12-14}\n"},
		{name: "a date into a string", src: "name: 2001-12-14\n"},
		{name: "a date into an any", src: "any: 2001-12-14\nanys: {a: !!timestamp 2002-01-01}\n"},
		{name: "a date under an alias", src: "name: &d 2001-12-14\nplain: {k: *d}\nany: *d\n"},
		{name: "a float into a string", src: "name: 1.5\n"},
		{name: "a float into a uint", src: "leaf: {u: 2.0}\n"},
		{name: "a float into an int", src: "leaf: {i: 2.0}\n"},
		{name: "a number into a map value", src: "plain: {k: 1, j: true}\n"},
		{name: "a number into a pointer", src: "leaf: {p: 1}\n"},
		{name: "a number into a string", src: "name: 1\n"},
		{name: "a quoted number into an int", src: "leaf: {i: \"5\"}\n"},
		{name: "a slice element", src: "leaf: {xs: [1.0, \"2\", 3]}\n"},
		{name: "a tagged date into a string", src: "name: !!timestamp 2001-12-14\n"},
		{name: "a tagged quoted date", src: "name: !!timestamp \"2001-12-14\"\n"},
		{name: "a time in its own zone", src: "name: 2001-12-14t21:59:43.10-05:00\n"},
		{name: "a wide number into a string", src: "name: 12345678901234567890123\n"},
		{name: "an infinity into a string", src: "name: .inf\n"},
		{name: "in a sequence of structs", src: "list: [{s: 1}, {s: two}]\n"},
		{name: "the fields after it", src: "leaf: {s: 1, i: 2, b: true}\nname: n\n"},
		{name: "under an anchor and its alias", src: "name: &n 1\nplain: {k: *n}\n"},
	})
}

// TestAValueThatDoesNotFitKeepsTheWalk reads values the tree decoder refuses
// for their field, and requires the tree's error and the value it leaves,
// without the decode building a tree. Both keep the first error, write what
// decoded in place and decode the fields after it.
func TestAValueThatDoesNotFitKeepsTheWalk(t *testing.T) {
	for tc := range aValueThatDoesNotFitKeepsTheWalkCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.Error(t, r.walkErr, "walk")
			require.Error(t, r.treeErr, "tree")
			assert.EqualError(t, r.walkErr, r.treeErr.Error(), "the two paths refuse differently")
			assert.Equal(t, r.built, r.walked, "the two paths fill the value differently")
			assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
		})
	}
}

// aValueThatDoesNotFitKeepsTheWalkCase holds a case name and a document with a value the tree decoder refuses for its field.
type aValueThatDoesNotFitKeepsTheWalkCase struct {
	name string
	src  string
}

func aValueThatDoesNotFitKeepsTheWalkCases() iter.Seq[aValueThatDoesNotFitKeepsTheWalkCase] {
	return slices.Values([]aValueThatDoesNotFitKeepsTheWalkCase{
		{name: "a date into a slice element", src: "leaf: {xs: [1, 2001-12-14, 3]}\n"},
		{name: "a date into an int", src: "leaf: {i: 2001-12-14, s: after}\n"},
		{name: "a mapping into a string", src: "name: {a: 1}\nleaf: {s: after}\n"},
		{name: "a negative into a uint", src: "leaf: {u: -1}\n"},
		{name: "a number into a bool", src: "leaf: {b: 1}\n"},
		{name: "a scalar into a struct", src: "leaf: 1\nname: n\n"},
		{name: "a sequence into a map value", src: "plain: {k: [1], j: v}\n"},
		{name: "a sequence into a struct", src: "leaf: [1]\nname: n\n"},
		{name: "a tag that is no timestamp", src: "name: !!timestamp nope\nleaf: {s: after}\n"},
		{name: "an int that overflows", src: "leaf: {i: 9999999999999999999999}\n"},
		{name: "in a map of structs", src: "byName: {a: {s: new, i: x}, b: {s: b}}\n"},
		{name: "in a merge", src: "leaf: {<<: {i: x}, s: own}\n"},
		{name: "in a pointer", src: "ptr: {s: new, i: x}\n"},
		{name: "in a slice element", src: "list: [{s: a}, {i: x}, {s: c}]\n"},
		{name: "in a slice in a map", src: "counts: {a: [1, x, 3]}\n"},
		{name: "later fields decode", src: "leaf: {i: x, s: after}\nname: n\n"},
		{name: "text into an int", src: "leaf: {i: x}\n"},
		{name: "the first error is kept", src: "leaf: {i: x}\nname: [1]\n"},
		{name: "under an alias", src: "ptr: &p {i: x}\nleaf: *p\n"},
	})
}

// TestAKeptKeyKeepsTheWalk reads documents whose keys are written with an
// explicit "?" or a tag, with and without ShareAliases, and requires the value
// the tree gives without the decode building a tree. The walk holds such a key
// whole and names the entry by it, as the tree does.
func TestAKeptKeyKeepsTheWalk(t *testing.T) {
	for _, share := range []bool{false, true} {
		for tc := range aKeptKeyKeepsTheWalkCases() {
			t.Run(fmt.Sprintf("share=%v/%s", share, tc.name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, tc.src, opts...)
				require.NoError(t, r.walkErr, "walk")
				require.NoError(t, r.treeErr, "tree")
				assert.Equal(t, r.built, r.walked, "walk against tree")
				assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
			})
		}
	}
}

// aKeptKeyKeepsTheWalkCase holds a case name and a document with a key written with "?" or a tag.
type aKeptKeyKeepsTheWalkCase struct {
	name string
	src  string
}

func aKeptKeyKeepsTheWalkCases() iter.Seq[aKeptKeyKeepsTheWalkCase] {
	return slices.Values([]aKeptKeyKeepsTheWalkCase{
		{name: "a block key", src: "? name\n: top\n"},
		{name: "a block key in a field", src: "leaf:\n  ? s\n  : x\n  i: 2\n"},
		{name: "a collection key skipped", src: "? [a, b]\n: x\nname: m\n"},
		{name: "a flow key", src: "leaf: {? s : x, i: 2}\n"},
		{name: "a key no field claims", src: "? nope\n: x\nname: m\n"},
		{name: "a key with no value", src: "leaf: {? s, i: 2}\n"},
		{name: "a local tag on a key", src: "leaf: {!k s: x}\n"},
		{name: "a merge key", src: "leaf: {? << : {s: a}, i: 1}\n"},
		{name: "a number key into a map", src: "plain: {? 1 : v}\n"},
		{name: "a tag inside the key", src: "plain: {? !!str 1 : v}\n"},
		{name: "a tagged key in a sequence", src: "list: [{!!str s: one}, {s: two}]\n"},
		{name: "a tagged key into a field", src: "leaf: {!!str s: x}\n"},
		{name: "a tagged key into a map", src: "plain: {!!str 1: v, !!int 2: w}\n"},
		{name: "in a flow sequence", src: "list: [{? s : one}, {s: two}]\n"},
		{name: "in a sequence entry", src: "list:\n- ? s\n  : one\n- s: two\n"},
		{name: "inside an any", src: "any: {? k : v, !!str 1: w}\n"},
		{name: "into a map", src: "plain: {? k : v, j: w}\n"},
		{name: "replayed for an alias", src: "ptr: &p {? s : x}\nleaf: *p\n"},
	})
}

// TestAKeptKeyTheWalkCannotReadGoesToTheTree pins the keys the walk still hands
// to the tree, so each one that starts to walk is noticed and moved above. It
// requires the tree's value or its error from both.
func TestAKeptKeyTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for tc := range aKeptKeyTheWalkCannotReadGoesToTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.Equalf(t, r.treeErr == nil, r.walkErr == nil,
				"walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			if r.walkErr != nil {
				assert.EqualError(t, r.walkErr, r.treeErr.Error(), "walk against tree")

				return
			}
			assert.Equal(t, r.built, r.walked, "walk against tree")
			assert.NotNil(t, r.dec.parsedFile, "the walk read it: move the case to TestAKeptKeyKeepsTheWalk")
		})
	}
}

// aKeptKeyTheWalkCannotReadGoesToTheTreeCase holds a case name and a document with a key the walk hands to the tree.
type aKeptKeyTheWalkCannotReadGoesToTheTreeCase struct {
	name string
	src  string
}

func aKeptKeyTheWalkCannotReadGoesToTheTreeCases() iter.Seq[aKeptKeyTheWalkCannotReadGoesToTheTreeCase] {
	return slices.Values([]aKeptKeyTheWalkCannotReadGoesToTheTreeCase{
		{name: "a collection key into a map", src: "plain: {? [a] : v}\n"},
		{name: "an alias inside a key", src: "name: &n k\nplain: {? *n : v}\n"},
		{name: "an anchor inside a key", src: "? &a name\n: top\nplain: {k: *a}\n"},
	})
}

// TestAKeptKeyRefusesARepeatAsTheTreeDoes requires the tree's error, word for
// word, from a key written with "?" or a tag that repeats another: a repeat the
// parse records, and two keys that one name makes one, which a reader keyed by
// name refuses. Where the refusal comes from is left out: a repeat written
// with plain keys reaches the tree too.
func TestAKeptKeyRefusesARepeatAsTheTreeDoes(t *testing.T) {
	for tc := range aKeptKeyRefusesARepeatAsTheTreeDoesCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.Error(t, r.walkErr, "walk")
			require.Error(t, r.treeErr, "tree")
			assert.EqualError(t, r.walkErr, r.treeErr.Error(), "walk against tree")
		})
	}
}

// aKeptKeyRefusesARepeatAsTheTreeDoesCase holds a case name and a document whose kept key repeats another.
type aKeptKeyRefusesARepeatAsTheTreeDoesCase struct {
	name string
	src  string
}

func aKeptKeyRefusesARepeatAsTheTreeDoesCases() iter.Seq[aKeptKeyRefusesARepeatAsTheTreeDoesCase] {
	return slices.Values([]aKeptKeyRefusesARepeatAsTheTreeDoesCase{
		{name: "a repeat in a field", src: "leaf: {? s : x, s: y}\n"},
		{name: "a tagged key of one name", src: "plain: {!!str 1: x, 1: y}\n"},
		{name: "one name into a map", src: "plain: {? 1 : x, \"1\": y}\n"},
	})
}

// TestATagKeepsTheWalk reads documents holding a tagged node, with and without
// ShareAliases, and requires the value the tree gives without the decode
// building a tree for the document. The walk hands each tagged node to the
// tree decoder alone.
func TestATagKeepsTheWalk(t *testing.T) {
	for _, share := range []bool{false, true} {
		for tc := range aTagKeepsTheWalkCases() {
			t.Run(fmt.Sprintf("share=%v/%s", share, tc.name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, tc.src, opts...)
				require.NoError(t, r.walkErr, "walk")
				require.NoError(t, r.treeErr, "tree")
				assert.Equal(t, r.built, r.walked, "the two paths disagree")
				assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
			})
		}
	}
}

// aTagKeepsTheWalkCase holds a case name and a document holding a tagged node.
type aTagKeepsTheWalkCase struct {
	name string
	src  string
}

//nolint:dupl // typedWalkMatchesTheTreeCases has rows of the same shape and length; no two rows are the same
func aTagKeepsTheWalkCases() iter.Seq[aTagKeepsTheWalkCase] {
	return slices.Values([]aTagKeepsTheWalkCase{
		{name: "a block root", src: "!!map\nname: top\n"},
		{name: "a bool", src: "leaf: {b: !!bool true}\n"},
		{name: "a default kept", src: "leaf: !!map {i: 4}\n"},
		{name: "a field no field claims", src: "nope: !!str x\nname: m\n"},
		{name: "a float on an int", src: "leaf: {f: !!float 1}\n"},
		{name: "a flow root", src: "--- !!map {name: top}\n"},
		{name: "a local tag on a mapping", src: "leaf: !thing {s: x, i: 2}\n"},
		{name: "a local tag on a scalar", src: "name: !Ref x\n"},
		{name: "a map tag", src: "leaf: !!map {s: x}\n"},
		{name: "a map value", src: "plain: {k: !!str 7}\n"},
		{name: "a merge inside", src: "leaf: !!map {<<: {s: a}, i: 1}\n"},
		{name: "a null on a pointer", src: "ptr: !!null ~\n"},
		{name: "a pointer", src: "ptr: !!map {s: p}\n"},
		{name: "a seq tag", src: "leaf: {xs: !!seq [1, 2]}\n"},
		{name: "a slice element", src: "list: [!!map {s: one}, {s: two}]\n"},
		{name: "a string on a number", src: "name: !!str 1\n"},
		{name: "a string tag into an int", src: "leaf: {i: !!str 5}\n"},
		{name: "a struct map value", src: "byName: {a: !!map {s: aa}}\n"},
		{name: "an alias to a tagged any", src: "any: &m !!omap [{x: 1}]\nlist2: [*m, *m]\n"},
		{name: "an alias to a tagged scalar", src: "anys: {a: &s !!str 1, b: *s}\n"},
		{name: "an anchor on a tagged mapping", src: "leaf: &l !!map {s: x}\nptr: *l\nlist: [*l]\n"},
		{name: "an anchor on a tagged scalar", src: "name: &n !!str 1\nplain: {k: *n}\n"},
		{name: "an any", src: "any: !!str 3\n"},
		{name: "an empty string", src: "name: !!str\n"},
		{name: "an int on a string", src: "leaf: {i: !!int \"5\"}\n"},
		{name: "inside an any", src: "any: {k: !!str 3}\n"},
	})
}

// TestATagTheWalkRefusesAsTheTreeDoes requires the tree's error, word for
// word, and the value it leaves, from a tagged node the tree decoder refuses,
// without the decode building a tree.
func TestATagTheWalkRefusesAsTheTreeDoes(t *testing.T) {
	for tc := range aTagTheWalkRefusesAsTheTreeDoesCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.Error(t, r.walkErr, "walk")
			require.Error(t, r.treeErr, "tree")
			assert.EqualError(t, r.walkErr, r.treeErr.Error(), "the two paths refuse differently")
			assert.Equal(t, r.built, r.walked, "the two paths fill the value differently")
			assert.Nil(t, r.dec.parsedFile, "the decode built a tree")
		})
	}
}

// aTagTheWalkRefusesAsTheTreeDoesCase holds a case name and a document with a tagged node the tree decoder refuses.
type aTagTheWalkRefusesAsTheTreeDoesCase struct {
	name string
	src  string
}

func aTagTheWalkRefusesAsTheTreeDoesCases() iter.Seq[aTagTheWalkRefusesAsTheTreeDoesCase] {
	return slices.Values([]aTagTheWalkRefusesAsTheTreeDoesCase{
		{name: "a map tag on a sequence", src: "leaf: {xs: !!map {a: 1}}\n"},
		{name: "a sequence into a field", src: "name: !!seq [1]\nleaf: {s: after}\n"},
		{name: "a tagged root", src: "!!seq [1]\n"},
		{name: "an int tag on text", src: "leaf: {i: !!int x}\n"},
	})
}

// TestATagTheWalkCannotReadGoesToTheTree pins the tagged nodes the walk still
// hands to the tree, so each one that starts to walk is noticed and moved
// above.
func TestATagTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for tc := range aTagTheWalkCannotReadGoesToTheTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeBothWays(t, tc.src)
			require.NoError(t, r.walkErr, "walk")
			require.NoError(t, r.treeErr, "tree")
			assert.Equal(t, r.built, r.walked, "the two paths disagree")
			assert.NotNil(t, r.dec.parsedFile, "the walk read it: move the case to TestATagKeepsTheWalk")
		})
	}
}

// aTagTheWalkCannotReadGoesToTheTreeCase holds a case name and a document with a tagged node the walk hands to the tree.
type aTagTheWalkCannotReadGoesToTheTreeCase struct {
	name string
	src  string
}

func aTagTheWalkCannotReadGoesToTheTreeCases() iter.Seq[aTagTheWalkCannotReadGoesToTheTreeCase] {
	return slices.Values([]aTagTheWalkCannotReadGoesToTheTreeCase{
		{name: "a tag on an anchor", src: "name: !!str &n 1\nplain: {k: *n}\n"},
		{name: "an alias inside a tagged node", src: "name: &n x\nleaf: !!map {s: *n}\n"},
		{name: "an anchor inside a tagged node", src: "leaf: !!map {s: &s x}\nname: *s\n"},
	})
}

// TestAStrictDecodeWalks checks that DisallowUnknownField no longer sends a struct decode to the tree.
func TestAStrictDecodeWalks(t *testing.T) {
	type strict struct {
		A int `yaml:"a"`
	}
	dec := NewDecoder(bytes.NewReader([]byte("a: 1\n")), DisallowUnknownField())
	var v strict
	require.NoError(t, dec.Decode(&v))
	assert.Equal(t, 1, v.A)
	require.NotNil(t, dec.stream, "the struct was not walked")
}

// TestADuplicateKeyDecodeWalks checks that AllowDuplicateMapKey no longer sends a struct decode to the tree.
func TestADuplicateKeyDecodeWalks(t *testing.T) {
	type repeated struct {
		A int `yaml:"a"`
	}
	dec := NewDecoder(bytes.NewReader([]byte("a: 1\na: 2\n")), AllowDuplicateMapKey())
	var v repeated
	require.NoError(t, dec.Decode(&v))
	assert.Equal(t, 2, v.A)
	require.NotNil(t, dec.stream, "the struct was not walked")
}

// TestARootOtherThanAStructWalks checks that a slice, a string map, a scalar and a pointer at the root take the walk.
// TestAStructHoldingAFieldTheTreeReadsWalks checks a struct holding a time.Time, a MapSlice or a hook type walks:
// the walk keeps that field's node, and the tree decoder reads it alone.
func TestAStructHoldingAFieldTheTreeReadsWalks(t *testing.T) {
	type holder struct {
		When  time.Time     `yaml:"when"`
		Order MapSlice      `yaml:"order"`
		Raw   walkBytesHook `yaml:"raw"`
		Text  walkTextHook  `yaml:"text"`
	}
	dec := NewDecoder(bytes.NewReader([]byte("when: 2001-12-14\norder: {a: 1}\nraw: [1]\ntext: x\n")))
	var v holder
	require.NoError(t, dec.Decode(&v))
	assert.NotNil(t, dec.stream, "the struct was not walked")
	assert.Equal(t, 2001, v.When.Year())
	assert.Equal(t, "x", v.Text.text)
}

func TestARootOtherThanAStructWalks(t *testing.T) {
	var p *int
	for _, dst := range []any{new([]int), new(map[string]int), new(int), &p} {
		dec := NewDecoder(bytes.NewReader([]byte("1\n")))
		_ = dec.Decode(dst)
		assert.NotNilf(t, dec.stream, "%T was not walked", dst)
	}
}
