// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

// KeyedMap builds a mapping for an `any` destination.
//
// It holds a map[string]any while every key is a string, as in nearly every mapping.
// The first key that resolves to anything else widens it to a map[any]any:
// an integer, a float, a null, a Base64, a time.Time.
// Each key keeps the value it resolves to, not its text, so "1: a" and "\"1\": b" stay two entries.
// Naming both keys "1" kept only one of them.
//
// The codec's valueBuilder.buildFrame does the same on the walk.
// The two must agree, so that a document decodes to the same value on either path.
type KeyedMap struct {
	byName map[string]any
	byKey  map[any]any
}

// NewKeyedMap returns an empty KeyedMap with room for size string keys.
func NewKeyedMap(size int) *KeyedMap {
	return &KeyedMap{byName: make(map[string]any, size)}
}

// Holds reports whether the mapping already writes key.
//
// A mapping's own entries are written before the merged ones, so Holds gives an own key precedence over a merged one.
// A timestamp key matches its instant in any zone, and a wide number matches its value.
// A Go map lookup finds neither, so Holds also asks SameKeyIn.
func (k *KeyedMap) Holds(key any) bool {
	if k.byKey != nil {
		if _, stands := k.byKey[key]; stands {
			return true
		}
		_, stands := SameKeyIn(k.byKey, key)

		return stands
	}
	name, isString := key.(string)
	if !isString {
		return false
	}
	_, stands := k.byName[name]

	return stands
}

// Put writes value under key, widening the mapping where the key is not a
// string.
func (k *KeyedMap) Put(key any, value any) {
	if name, isString := key.(string); isString && k.byKey == nil {
		k.byName[name] = value

		return
	}
	if k.byKey == nil {
		k.widen()
	}
	k.byKey[key] = value
}

// Value returns the mapping: a map[string]any, or a map[any]any once a key that is not a string widened it.
func (k *KeyedMap) Value() any {
	if k.byKey != nil {
		return k.byKey
	}

	return k.byName
}

// widen moves the mapping to map[any]any. The keys already written are strings
// and go over as they stand.
func (k *KeyedMap) widen() {
	k.byKey = make(map[any]any, len(k.byName)+1)
	for name, value := range k.byName {
		k.byKey[name] = value
	}
	k.byName = nil
}
