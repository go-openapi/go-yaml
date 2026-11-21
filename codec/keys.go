// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"math/big"
	"reflect"
	"strconv"
	"time"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/token"
)

// canonicalKey is a decoded key named by its canonical spelling, whatever it was written as: a Base64 is its
// canonical base64 text, so the same bytes written across lines are one key.
type canonicalKey interface {
	Canonical() string
}

// sameMapKey reports whether two decoded keys are one key.
//
// It compares the resolved values, so the string "1.0" and the float 1.0 are
// two keys and a merge cannot override one with the other. Comparing the text
// made them one, which is the half of defect 69 a MapSlice can represent; a
// map[string]any has lost one of them before this runs.
//
// Only a key mapKeyNodeToValue judged hashable reaches this as a value, so ==
// on the interfaces cannot panic -- the guard is there for a MapSlice a caller
// built by hand.
//
// Two Go values that == tells apart can still be one YAML key, and keyIDOf
// says which: YAML has one integer type, one float type and one timestamp
// type, so int(1) and uint64(1) are one key, float32(0.5) and 0.5 are one,
// and so are one instant in two zones. The parser refuses a mapping that
// writes one such key twice, and a merge or MapSlice.Set has to agree with it.
func sameMapKey(a, b any) bool {
	if !codecutils.HashableKey(a) || !codecutils.HashableKey(b) {
		return reflect.DeepEqual(a, b)
	}
	if a == b {
		return true
	}
	aKind, aName, aNamed := keyIDOf(a)
	if !aNamed {
		return false
	}
	bKind, bName, bNamed := keyIDOf(b)

	return bNamed && aKind == bKind && aName == bName
}

// keyIDOf returns the kind and the name a Go key is compared by in YAML, and
// whether it is a scalar YAML has a kind for.
//
// It is the value side of the identity [ast.ComparedKeyName] gives a key node,
// and gives the same answer for the value a key node decodes to:
// TestAKeyNodeAndItsValueHaveOneIdentity holds the two together over the
// corpus. A string is its text, a key with a canonical spelling -- a Base64 --
// that spelling, a time.Time its instant in UTC, and a number
// [codecutils.NumberKey]'s name.
func keyIDOf(v any) (token.KeyKind, string, bool) {
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

	return codecutils.NumberKey(v)
}

// comparesByValue reports whether a decoded key can be one YAML key with a key
// a Go map lookup does not find it under: a time.Time in another zone, a
// *big.Int or *big.Float, which == compares by address, or a key with a
// canonical spelling, such as a Base64 written across lines.
func comparesByValue(key any) bool {
	switch key.(type) {
	case time.Time, *big.Int, *big.Float, canonicalKey:
		return true
	default:
		return false
	}
}

// sameKeyIn returns the key of m that is one YAML key with key where a Go map
// lookup cannot find it, and whether m holds one.
//
// A scan, and only on a merge: the decoder builds uint64, int64 and float64
// keys that == compares by value, and scans only for the keys comparesByValue
// names.
func sameKeyIn(m map[any]any, key any) (any, bool) {
	if !comparesByValue(key) {
		return nil, false
	}
	for held := range m {
		if sameMapKey(held, key) {
			return held, true
		}
	}

	return nil, false
}
