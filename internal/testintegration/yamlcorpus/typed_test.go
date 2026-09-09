// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"math/big"
	"reflect"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// TestTheEnumeratedShapesReadIntoAGoType runs every enumerated document through
// the reflection path as well as through the `any` path.
//
// These are the documents the generator cannot write, and they are the ones the
// reflection path most needs. Two of the three defects found on it on
// 2026-09-06 were merge keys: `use: {<<: *base, b: 3}` refused its own key as a
// duplicate of the one it overrides, and `<<: [*one, *two]` was refused
// outright. Both were correct into an `any` and correct in
// go.yaml.in/yaml/v3, and wrong only into a struct -- so no corpus of verdicts
// and no comparison against another library would ever have found them.
//
// The `any` read is the yardstick: a document it refuses says nothing here, and
// yamlcorpus.GoYAML is what holds that half. The destination is built from what
// it read, so the type fits by construction and a disagreement is the
// destination's.
func TestTheEnumeratedShapesReadIntoAGoType(t *testing.T) {
	var structs, compared int

	for _, group := range [][]stance.Shape{
		yamlcorpus.KeyShapes(), yamlcorpus.TagShapes(), yamlcorpus.SchemaShapes(),
		yamlcorpus.MergeShapes(), yamlcorpus.DirectiveShapes(),
	} {
		for _, s := range group {
			var loose any
			if yaml.Unmarshal(s.Src, &loose) != nil {
				continue
			}

			// Each shape is a destination a caller writes, and the same
			// document read into all three has to give one answer. A struct
			// and a map[any]any are different code in the decoder, and the
			// pointer shape is the only one that makes it allocate before it
			// fills.
			for _, shape := range []yamlgen.TargetShape{
				yamlgen.ShapePlain, yamlgen.ShapePointers, yamlgen.ShapeAnyKeyedMap,
			} {
				target := yamlgen.TargetForDecodedAs(loose, shape)
				if !target.Reached() {
					continue
				}
				structs++

				into := reflect.New(target.Type)
				err := yaml.Unmarshal(s.Src, into.Interface())

				want := yamlgen.Normalize(reflect.ValueOf(loose))
				got := yamlgen.Normalize(into.Elem())
				failed := err != nil || !sameNumerically(want, got)

				if yard, unusable := yardstickDefects[s.Name]; unusable && yard.failsFor(shape) {
					t.Logf("yardstick unusable -- %s into %s: %s", s.Name, shape, yard.why)

					continue
				}

				defect, known := typedPathDefects[s.Name]
				known = known && defect.failsFor(shape)

				switch {
				case failed && known:
					t.Logf("still fails -- %s into %s: %s", s.Name, shape, defect.why)
				case failed:
					t.Errorf("%s: %q reads into an `any` and differently into %v (%s)\n"+
						"  as an any: %#v\n  as a type: %#v\n  error:     %v",
						s.Name, s.Src, target.Type, shape, want, got, err)
				case known:
					t.Errorf("no longer fails into %s, so its entry is stale: %s -- %s", shape, s.Name, defect.why)
				default:
					compared++
				}
			}
		}
	}

	t.Logf("%d enumerated reads reached the reflection path, %d of them read the same both ways", structs, compared)

	// A floor rather than a count, since a shape added to any family may or may
	// not be a mapping. It is here so that a change which stops the enumerated
	// documents reaching the reflection path at all shows up as a failure
	// rather than as a quieter run.
	require.GreaterOrEqual(t, structs, 20,
		"the enumerated shapes have stopped reaching the reflection path")
}

// The enumerated documents the reflection path gets wrong, and why.
//
// Every one is a defect already recorded elsewhere; this test found nothing the
// registers did not hold, which is the answer it was built to give. An entry
// leaves by being fixed -- the test says so rather than passing quietly.
//
// Eleven left that way on 2026-09-07, when the decoder branch landed. Eight
// were the struct zeroing, closed in two steps: 7dc4075 inverted decodeStruct,
// so a key no field can be named after is a key no field claims rather than one
// that abandons the whole mapping, and entryName then named a key by the
// type's own canonical spelling, so "true: x" reaches a field tagged "true" and
// "1.0: x" one tagged "1.0" -- which is how the same document reads into a
// map[string]any. Three were the merge path, closed by 6c10f40: a mapping's own
// key was refused as a duplicate of the one it overrides, and a merge given a
// sequence was refused with "sequence was used where mapping is expected".
//
// This test also found one the registers did not hold, which is what it was
// built for: the walking decoder read a merge written in place -- "<<: {a: 1}"
// rather than "<<: *b" -- as a key no field claims and dropped it. A merge
// written as an alias gave up on the alias and fell back to the tree; one
// written in place had nothing else to give up on. Fixed in the same branch.
// typedDefect is one enumerated shape whose typed read is wrong, and which
// destinations it is wrong for.
type typedDefect struct {
	why string
	// fails names the destinations. Empty means all three, which is the usual
	// case: a defect in the reflection path rarely cares which type it is
	// filling.
	fails []yamlgen.TargetShape
}

func (d typedDefect) failsFor(shape yamlgen.TargetShape) bool {
	if len(d.fails) == 0 {
		return true
	}

	return slices.Contains(d.fails, shape)
}

var typedPathDefects = map[string]typedDefect{}

// yardstickDefects names the enumerated shapes where the two reads differ and
// the *typed* one is right, so this test cannot use the `any` read as its
// yardstick.
//
// The inverse of typedPathDefects and worth keeping apart from it: an entry
// here is not a reason to look at the reflection path.
var yardstickDefects = map[string]typedDefect{
	// A collection key has no Go map key to be. `map[any]any` and
	// `map[string]any` both refuse the document with `cannot use
	// map[string]interface {} as a map key: Go cannot hash it`, and codec.ToJSON
	// refuses it with `a mapping cannot be a JSON key`. Those are the right
	// answers: Go cannot hash a map and JSON has no mapping key.
	//
	// The `any` read names the key by stringifying it -- "map[:0]" -- which
	// KeyText's own comment records as a divergence rather than a meaning. So
	// the two reads differ, the typed one is right, and there is nothing here to
	// fix in the reflection path.
	"two collection keys in one mapping": {why: "a collection key is not a Go map key"},
	// A timestamp and a byte string have no canonical YAML spelling of their
	// own, so ast.KeyName names such a key by the text the document wrote --
	// "2001-12-14" and "aGVsbG8=". A map[any]any does not name a key at all: it
	// keeps the time.Time, which is right, and has nowhere to put the []byte,
	// which it says so. The two reads differ because one names and the other
	// keeps the type, and neither is the reflection path's fault.
	//
	// They used to agree by accident: the `any` read named a timestamp key
	// "2001-12-14 00:00:00 +0000 UTC", which is Go's printing of the very
	// time.Time the typed read holds, so the two rendered alike. Naming by the
	// document's own text ended the coincidence.
	"a key tagged !!timestamp": {
		why:   "a map[any]any keeps the time.Time rather than naming it",
		fails: []yamlgen.TargetShape{yamlgen.ShapeAnyKeyedMap},
	},
	// A map[any]any is keyed on the []byte itself, which Go cannot use as a
	// map key. A string-keyed destination is named rather than keyed and now
	// agrees with the `any` read: both give "aGVsbG8=".
	"a key tagged !!binary": {
		why:   "a []byte cannot key a map[any]any",
		fails: []yamlgen.TargetShape{yamlgen.ShapeAnyKeyedMap},
	},
	// A duplicate that only collides once an alias is resolved. "k: &a n" over
	// "*a : 1" over "n: 2" reads into an `any` as {"k": "n", "n": 2}, one
	// entry short and nothing reported, and every typed map refuses it with
	// `duplicate key "n"`.
	//
	// The check exists and one path skips it, which is what makes this
	// actionable: map[string]any and map[any]any both refuse the document, and
	// so do the two spellings of the same collision without an alias --
	// "1: x" over "\"1\": y". The `any` path catches a duplicate written the
	// same way ("a: 1" over "a: 2") and misses one that only collides after
	// resolution. See yamlcorpus.Departures, "two keys alike in text and
	// different once resolved", which records the `any` half.
	"a key colliding with one an alias resolves to": {why: "the `any` read loses an entry the typed read refuses"},
	// The same fault without an alias, and the clearest evidence for it. "1: x"
	// over "\"1\": y" reads into a map[any]any as both keys -- uint64(1) => "x"
	// and "1" => "y", which is what 3.2.1.1 asks for, since an integer and a
	// string are two nodes. The `any` path names both "1" and keeps the last,
	// so it holds one entry and has lost "x".
	//
	// So the library preserves both wherever the destination can hold them, and
	// the merge is the naming rather than the read. yamlgen.Normalize flattens
	// a map[any]any by KeyText to compare it, which collapses the pair again
	// and makes the comparison order-dependent -- another reason this document
	// cannot be scored against the `any` read.
	"two keys alike in text and different once resolved": {why: "the `any` read merges two keys the typed read keeps apart"},
	// The merge key escaping the duplicate check on one path.
	// "{<<: {x: 1}, <<}" reads into an `any` as {"<<": null}, with the first
	// entry's mapping gone and nothing reported, and every typed map refuses it
	// with `duplicate key "<<"`. The typed read is right: under the core schema
	// the two entries are one key spelled "<<" twice.
	//
	// The same fault as the alias collision above, reached by a flow entry
	// written as a key alone rather than by resolution. `{a: 1, a}` is refused
	// on both paths, so it is the merge key that escapes and not the spelling.
	// Departures records both readings.
	"two merge keys, the second written as a key alone": {why: "the `any` read loses an entry the typed read refuses"},
}

// sameNumerically compares two decodes of one document, with numbers compared
// across the Go types the destination chooses: an int64 field holds 1 where an
// `any` holds uint64(1).
func sameNumerically(want, got any) bool {
	if w, isNumber := asFloat(want); isNumber {
		g, ok := asFloat(got)

		return ok && (w == g || (w != w && g != g))
	}

	switch w := want.(type) {
	case *big.Int:
		g, ok := got.(*big.Int)

		return ok && w.Cmp(g) == 0
	case *big.Float:
		g, ok := got.(*big.Float)

		return ok && w.Cmp(g) == 0
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for k, v := range w {
			other, found := g[k]
			if !found || !sameNumerically(v, other) {
				return false
			}
		}

		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for i := range w {
			if !sameNumerically(w[i], g[i]) {
				return false
			}
		}

		return true
	default:
		return assert.ObjectsAreEqual(want, got)
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
