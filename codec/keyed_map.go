// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import "github.com/go-openapi/go-yaml/codec/internal/codecutils"

// keyedMap is a mapping being built for an `any` destination.
//
// It holds map[string]any while every key is a string, which is nearly every
// mapping, and widens to map[any]any on the first key that resolves to anything
// else -- an integer, a float, a null, a Base64, a time.Time. So a key keeps
// what it resolves to rather than the text that spells it, and "1: a" beside
// "\"1\": b" stays two entries where naming both "1" left one.
//
// valueBuilder.buildFrame does the same for the walk, and the two have to
// agree: a document read through either comes back the same way.
type keyedMap struct {
	byName map[string]any
	byKey  map[any]any
}

func newKeyedMap(size int) *keyedMap {
	return &keyedMap{byName: make(map[string]any, size)}
}

// holds reports whether the mapping already writes key.
//
// A mapping's own entries are written before the ones it merges, so this is
// where an own key beats a merged one. A timestamp key stands for its instant
// in any zone, and a wide number for its value, neither of which a Go map
// lookup finds.
func (k *keyedMap) holds(key any) bool {
	if k.byKey != nil {
		if _, stands := k.byKey[key]; stands {
			return true
		}
		_, stands := codecutils.SameKeyIn(k.byKey, key)

		return stands
	}
	name, isString := key.(string)
	if !isString {
		return false
	}
	_, stands := k.byName[name]

	return stands
}

// put writes value under key, widening the mapping where the key is not a
// string.
func (k *keyedMap) put(key any, value any) {
	if name, isString := key.(string); isString && k.byKey == nil {
		k.byName[name] = value

		return
	}
	if k.byKey == nil {
		k.widen()
	}
	k.byKey[key] = value
}

// widen moves the mapping to map[any]any. The keys already written are strings
// and go over as they stand.
func (k *keyedMap) widen() {
	k.byKey = make(map[any]any, len(k.byName)+1)
	for name, value := range k.byName {
		k.byKey[name] = value
	}
	k.byName = nil
}

// value is the mapping, under whichever key type it settled on.
func (k *keyedMap) value() any {
	if k.byKey != nil {
		return k.byKey
	}

	return k.byName
}
