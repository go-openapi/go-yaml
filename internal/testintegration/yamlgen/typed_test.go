// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	yaml "github.com/go-openapi/go-yaml"
)

// TestDecodingIntoAGoTypeGivesTheSameValue reads each document twice, once into
// an `any` and once into a Go type built from what that read gave.
//
// The destination is the only variable. Reading into an `any` builds the value
// with one builder; reading into a Go type fills fields by reflection with
// another, or through a tree where the walk cannot serve, and the two can
// disagree with nothing to say so. Every other property here decodes into an `any`, and so does every
// corpus-driven decode in yamlcorpus and in internal/conformance/ -- three defects were
// found living on the reflection path in one afternoon of reading it, none of
// them by any harness.
//
// The comparison is against the `any` answer rather than against
// Value.Decoded, on purpose: a defect that hits both paths cancels out and does
// not have to be excused twice. What is left is the destination.
func TestDecodingIntoAGoTypeGivesTheSameValue(t *testing.T) {
	tally := newTally()

	var structs, reached int

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		shape := yamlgen.TargetShape(rapid.IntRange(0, 2).Draw(rt, "shape"))
		src := yamlgen.Emit(value, style)

		// A collection key is not a Go map key: Go cannot hash a map or a
		// slice, so `map[any]any` refuses the document with `Go cannot hash
		// it` and every generated struct is named after keys the `any` path
		// invented by stringifying. There is no typed destination that could
		// hold it, so the two reads have nothing to say about the reflection
		// path. yamlcorpus's yardstickDefects records the same thing for the
		// enumerated shape.
		if holdsACollectionKey(value) {
			return
		}

		// The `any` path is the yardstick, so a document it will not read has
		// nothing to say here. TestPresentationInvariance holds that half.
		var loose any
		if yaml.Unmarshal([]byte(src), &loose) != nil {
			return
		}

		target := yamlgen.TargetForDecodedAs(loose, shape)
		if !target.Reached() {
			// No mapping in the document reached decodeStruct or decodeMap, so
			// the run says nothing about the reflection path.
			return
		}
		structs++

		into := reflect.New(target.Type)
		err := yaml.Unmarshal([]byte(src), into.Interface())

		want := yamlgen.Normalize(reflect.ValueOf(loose))
		diverged := err != nil || !sameThroughAType(want, yamlgen.Normalize(into.Elem()))

		if known := yamlgen.Known(ledger, yamlgen.DecodeTyped, value, style); known != nil {
			tally.record(known.Name, diverged)

			return
		}

		reached++

		if err != nil {
			rt.Fatalf("%s into a %s destination read into an `any` and not into %v:\n%s\n---\nerror: %v",
				style, shape, target.Type, src, err)
		}
		if diverged {
			rt.Fatalf("%s into a %s destination read differently into %v:\n%s\n---\nas an any: %#v\nas a type: %#v",
				style, shape, target.Type, src, want, yamlgen.Normalize(into.Elem()))
		}
	})

	t.Logf("%d documents reached the reflection path, %d of them compared", structs, reached)
	tally.report(t, yamlgen.DecodeTyped)
}

// sameThroughAType is [sameValue] with numbers compared across the Go types a
// destination chooses.
//
// An int64 field holds 1 where an `any` holds uint64(1), and a float64 field
// holds 1 where an `any` holds the same uint64. That is the destination doing
// its job rather than the two paths disagreeing, so the comparison is by value.
// Everything else -- a NaN, a big.Float, a string -- goes to sameValue.
func sameThroughAType(want, got any) bool {
	if w, isNumber := asFloat(want); isNumber {
		g, ok := asFloat(got)

		return ok && (w == g || (w != w && g != g))
	}

	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for k, v := range w {
			other, found := g[k]
			if !found || !sameThroughAType(v, other) {
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
			if !sameThroughAType(w[i], g[i]) {
				return false
			}
		}

		return true
	default:
		return sameValue(want, got)
	}
}

// asFloat reports the Go number kinds a destination can turn an integer into.
//
// The big types are left out: they are not a destination this package builds,
// and comparing one against a float64 would lose the precision that makes them
// worth having.
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
