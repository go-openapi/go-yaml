// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import (
	"bytes"
	"encoding/json"
	"iter"
	"reflect"
	"strings"
)

// Base64 is the expected value of a "!!binary" scalar: the base64 text of its bytes.
//
// It marshals to JSON as that text, which is how JSON writes binary data.
type Base64 string

// Canonical returns the text without the line breaks and spaces RFC 2045 allows inside base64.
//
// Two Base64 values are one key when their canonical texts are equal.
func (b Base64) Canonical() string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		default:
			return r
		}
	}, string(b))
}

// OrderedEntry is one entry of an [OrderedMap].
type OrderedEntry struct {
	Key, Value any
}

// OrderedMap is the expected value of a sequence tagged "!!omap": its entries, in document order.
//
// Every key is unique under [sameDecodedKey]. Use [OrderedMap.All] to range over the entries.
type OrderedMap struct {
	entries []OrderedEntry
}

// newOrderedMap returns the ordered map holding entries, and false when two entries hold one key.
//
// A sequence tagged "!!omap" with a repeated key is not the shape the tag names,
// so the caller reads it as the plain sequence it is written as.
func newOrderedMap(entries []OrderedEntry) (OrderedMap, bool) {
	for i := range entries {
		for j := range i {
			if sameDecodedKey(entries[i].Key, entries[j].Key) {
				return OrderedMap{}, false
			}
		}
	}

	if len(entries) == 0 {
		return OrderedMap{}, true
	}

	return OrderedMap{entries: entries}, true
}

// Len returns the number of entries.
func (m OrderedMap) Len() int { return len(m.entries) }

// All returns the keys and values in entry order.
func (m OrderedMap) All() iter.Seq2[any, any] {
	return func(yield func(any, any) bool) {
		for _, entry := range m.entries {
			if !yield(entry.Key, entry.Value) {
				return
			}
		}
	}
}

// MarshalJSON writes the entries as a JSON object, in entry order.
//
// Each key is named as [Normalize] names a mapping key, and each value is written as [MeaningJSON] writes one.
func (m OrderedMap) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer

	out.WriteByte('{')

	for i, entry := range m.entries {
		if i > 0 {
			out.WriteByte(',')
		}

		key, err := json.Marshal(keyString(reflect.ValueOf(&entry.Key).Elem()))
		if err != nil {
			return nil, err
		}

		value, err := json.Marshal(jsonNumbers(NamedKeys(entry.Value)))
		if err != nil {
			return nil, err
		}

		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}

	out.WriteByte('}')

	return out.Bytes(), nil
}

// sameDecodedKey reports whether a and b are one mapping key, once decoded.
//
// Keys compare with ==, except for the kinds [sameKeyValue] compares by value:
// a time.Time at the same instant, a *big.Int or *big.Float of the same value,
// and a [Base64] of the same canonical text.
// A key Go cannot compare, which no decoded document holds, compares with reflect.DeepEqual.
func sameDecodedKey(a, b any) bool {
	if !comparableKey(a) || !comparableKey(b) {
		return reflect.DeepEqual(a, b)
	}

	return a == b || sameKeyValue(a, b)
}

// comparableKey reports whether == can compare key without a panic.
func comparableKey(key any) bool {
	return key == nil || reflect.TypeOf(key).Comparable()
}

// keyClass is the YAML type of a mapping key, which tells apart two keys with one name.
type keyClass uint8

const (
	classOther keyClass = iota
	classString
	classNull
	classBool
	classInt
	classFloat
	classTimestamp
	classBinary
)
