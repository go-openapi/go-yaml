// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"math/big"
	"reflect"
	"strconv"
	"time"

	"github.com/go-openapi/go-yaml/token"
)

// canonicalKey matches a decoded key named by its canonical spelling, however the document wrote it.
// A Base64 is named by its canonical base64 text, so the same bytes written across lines make one key.
type canonicalKey interface {
	Canonical() string
}

// SameMapKey reports whether two decoded keys are one key.
//
// It compares the resolved values, so the string "1.0" and the float 1.0 are two keys,
// and a merge cannot override one with the other. Comparing the text made them one key (defect 69).
// Only a MapSlice holds both keys: a map[string]any has already lost one of them before SameMapKey runs.
//
// The codec's mapKeyNodeToValue checks that a key is hashable before SameMapKey sees it,
// so == on the interfaces cannot panic. The guard covers a MapSlice that a caller built by hand.
//
// Two Go values that == tells apart can still be one YAML key, and KeyIDOf
// detects such a pair: YAML has one integer type, one float type and one timestamp
// type, so int(1) and uint64(1) are one key, float32(0.5) and 0.5 are one,
// and so are one instant in two zones. The parser refuses a mapping that
// writes one such key twice, and a merge or MapSlice.Set has to agree with it.
func SameMapKey(a, b any) bool {
	if !HashableKey(a) || !HashableKey(b) {
		return reflect.DeepEqual(a, b)
	}
	if a == b {
		return true
	}
	aKind, aName, aNamed := KeyIDOf(a)
	if !aNamed {
		return false
	}
	bKind, bName, bNamed := KeyIDOf(b)

	return bNamed && aKind == bKind && aName == bName
}

// KeyIDOf returns the kind and the name a Go key is compared by in YAML, and
// whether it is a scalar YAML has a kind for.
//
// [ast.ComparedKeyName] gives a key node its identity, and KeyIDOf gives the same identity to the value the node
// decodes to. TestAKeyNodeAndItsValueHaveOneIdentity checks that the two agree over the corpus.
//
// The name of a key is:
//
//   - for a string, its text;
//   - for a key with a Canonical method, such as a Base64, what that method returns;
//   - for a time.Time, its instant in UTC;
//   - for a number, the name [NumberKey] returns.
func KeyIDOf(v any) (token.KeyKind, string, bool) {
	switch k := v.(type) {
	case nil:
		return token.KeyNull, "null", true
	case string:
		return token.KeyString, k, true
	case bool:
		return token.KeyBool, strconv.FormatBool(k), true
	case canonicalKey:
		return token.KeyBinary, k.Canonical(), true
	case time.Time:
		return token.KeyTimestamp, k.UTC().Format(time.RFC3339Nano), true
	}

	return NumberKey(v)
}

// ComparesByValue reports whether a decoded key can equal, as a YAML key, a key that a Go map lookup misses.
//
// Three kinds of key do:
//
//   - a time.Time, which can name the same instant in another zone;
//   - a *big.Int or a *big.Float, which == compares by address;
//   - a key with a Canonical method, such as a Base64 written across lines.
func ComparesByValue(key any) bool {
	switch key.(type) {
	case time.Time, *big.Int, *big.Float, canonicalKey:
		return true
	default:
		return false
	}
}

// SameKeyIn returns the key of m that equals key as a YAML key where a Go map lookup misses it,
// and whether m holds one.
//
// SameKeyIn scans m, and the decoder calls it only on a merge.
// The decoder builds uint64, int64 and float64 keys, which == compares by value,
// so it scans only for the keys that ComparesByValue accepts.
func SameKeyIn(m map[any]any, key any) (any, bool) {
	if !ComparesByValue(key) {
		return nil, false
	}
	for held := range m {
		if SameMapKey(held, key) {
			return held, true
		}
	}

	return nil, false
}
