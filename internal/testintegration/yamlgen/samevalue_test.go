// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// sameValue is ObjectsAreEqual with NaN equal to itself.
//
// # Why the properties need this
//
// A NaN is not equal to a NaN, by IEEE 754 and therefore by reflect.DeepEqual.
// Every property here compares what the generator meant against what the
// library read, so a document holding a NaN would fail all of them however
// perfectly it round-tripped -- the two sides would be the same bits and still
// compare unequal.
//
// The generator excluded the specials for years, partly for this reason. They
// are worth having: ".inf" and ".nan" are floats the library resolves, JSON has
// no spelling for either, and the encoder round-trips them. So the comparison
// moves rather than the value model.
//
// It is a *test* helper and not a method on Value, because "did this document
// survive" and "are these two values equal" are different questions and only
// the first wants NaN to match itself.
func sameValue(want, got any) bool {
	if w, ok := want.(float64); ok && math.IsNaN(w) {
		g, isFloat := got.(float64)

		return isFloat && math.IsNaN(g)
	}

	switch w := want.(type) {
	case *big.Float:
		// reflect's equality compares a big.Float's Accuracy, which records how
		// the last rounding went rather than what the number is -- and this
		// library parses the magnitude and negates it, so -1e+330 comes back
		// carrying Exact where the same text parsed whole carries Above. An
		// alias adds a second way to reach the same split: the walking reader
		// copies the anchored value with big.Float.Set, which always reports
		// Exact, so "&a1 1e+330" carries Below and "*a1" carries Exact. Same
		// number, same precision, different bookkeeping.
		//
		// The precision is still compared, because a change there would be a
		// real change in what the library built.
		g, ok := got.(*big.Float)

		return ok && w.Prec() == g.Prec() && w.Cmp(g) == 0
	case big.Float:
		// Normalize follows a pointer to the value it names, so a comparison
		// made after it never sees the *big.Float above and would fall through
		// to reflect's equality, Accuracy and all.
		g, ok := got.(big.Float)

		return ok && w.Prec() == g.Prec() && w.Cmp(&g) == 0
	case *big.Int:
		g, ok := got.(*big.Int)

		return ok && w.Cmp(g) == 0
	case big.Int:
		g, ok := got.(big.Int)

		return ok && w.Cmp(&g) == 0
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for k, v := range w {
			other, found := g[k]
			if !found || !sameValue(v, other) {
				return false
			}
		}

		return true
	case map[any]any:
		// A mapping widened by a key that is not a string. Its keys are paired
		// by sameValue and not looked up: a NaN key never finds itself in a Go
		// map, and a *big.Int key is a pointer the other side holds its own
		// copy of.
		g, ok := got.(map[any]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for wk, wv := range w {
			found := false

			for gk, gv := range g {
				if sameValue(wk, gk) && sameValue(wv, gv) {
					found = true

					break
				}
			}

			if !found {
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
			if !sameValue(w[i], g[i]) {
				return false
			}
		}

		return true
	case codec.MapSliceSeq:
		// An "!!omap" holds its values out of reach of the cases above, so
		// without this a *big.Float inside one was compared Accuracy and all.
		g, ok := got.(codec.MapSliceSeq)

		return ok && sameItems(codec.MapSlice(w), codec.MapSlice(g))
	case codec.MapSlice:
		g, ok := got.(codec.MapSlice)

		return ok && sameItems(w, g)
	default:
		return assert.ObjectsAreEqual(want, got)
	}
}

// sameItems compares two ordered mappings entry by entry, in order.
func sameItems(want, got codec.MapSlice) bool {
	if want.Len() != got.Len() {
		return false
	}

	for i := range want.Len() {
		w, g := want.At(i), got.At(i)
		if !sameValue(w.Key, g.Key) || !sameValue(w.Value, g.Value) {
			return false
		}
	}

	return true
}

// TestSameValueLooksInsideAnOrderedMap holds sameValue to its big.Float rule inside an "!!omap".
func TestSameValueLooksInsideAnOrderedMap(t *testing.T) {
	rounded, _, err := big.ParseFloat("0.1", 10, 64, big.ToNearestEven)
	require.NoError(t, err)
	copied := new(big.Float).Set(rounded)
	require.NotEqual(t, rounded.Acc(), copied.Acc(), "the fixture needs two Accuracy values for one number")

	omap := func(v any) codec.MapSliceSeq {
		seq, err := codec.NewMapSliceSeq(codec.MapItem{Key: "k", Value: v})
		require.NoError(t, err)

		return seq
	}

	assert.True(t, sameValue(omap(rounded), omap(copied)), "one number, two Accuracy values")
	assert.False(t, sameValue(omap(rounded), omap(big.NewFloat(0.2))), "two numbers")
	assert.False(t, sameValue(omap(rounded), omap("0.1")), "a float and a string")
}
