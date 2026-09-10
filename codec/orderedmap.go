// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strconv"
)

// MapItem is one entry of a [MapSlice] or a [MapSliceSeq].
//
// A decode fills Key with the value the key resolves to, as it fills a
// map[any]any key: "1:" gives uint64(1), "1.0:" float64(1), "null:" nil,
// "true:" true. So the string "1.0" and the float 1.0 are two entries, which
// 3.2.1.1 makes them. Use [UseStringKeys] to read every key as text instead.
//
// A key must be comparable. Go hashes no slice, map or function, so a
// collection written as a mapping key is refused on the way in, as it is for
// every other destination.
type MapItem struct {
	Key, Value any
}

// MapSlice decodes and encodes as a YAML mapping, keeping the order the
// document wrote and the keys a Go map cannot tell apart.
//
// It holds both entries of "1: a" over "\"1\": b", where a map[string]any
// refuses the pair as a duplicate and a map[any]any keeps both and loses the
// order.
//
// Every key is unique and comparable. [MapSlice.Set] overwrites the entry
// already holding a key rather than appending a second, and refuses a key Go
// cannot hash. The zero value is an empty map ready to Set; [NewMapSlice]
// builds one from entries you already have.
//
// Use [UseOrderedMap] to read every mapping of a document into one.
type MapSlice struct {
	items []MapItem
}

// MapSliceSeq is a [MapSlice] that reads and writes YAML's "!!omap", a sequence
// of one-entry mappings: "!!omap [{x: 1}, {y: 2}]".
//
// It holds the same entries under the same rules and differs in what the
// encoder writes: a MapSlice writes a plain mapping, a MapSliceSeq writes the
// tagged sequence, with or without [UseOrderedMap]. A decode builds one for a
// node tagged "!!omap", and builds whichever type the destination names where
// the destination names one.
//
// The two convert into each other: MapSlice(seq) and MapSliceSeq(m).
type MapSliceSeq MapSlice

// NewMapSlice returns a MapSlice holding items, in the order given.
//
// It returns an error for a key Go cannot hash, and for a key given twice --
// listing one key twice is the caller's mistake, where [MapSlice.Set] is the
// caller asking for the value to be replaced.
func NewMapSlice(items ...MapItem) (MapSlice, error) {
	var m MapSlice
	if len(items) == 0 {
		// The zero value, so an empty MapSlice built here compares equal to one
		// a decode builds for an empty mapping.
		return m, nil
	}
	m.items = make([]MapItem, 0, len(items))
	for _, item := range items {
		if !hashableKey(item.Key) {
			return MapSlice{}, unusableKey(item.Key)
		}
		if i := m.index(item.Key); i >= 0 {
			return MapSlice{}, fmt.Errorf("key %v is given twice: %w", item.Key, ErrDuplicateKey)
		}
		m.items = append(m.items, item)
	}

	return m, nil
}

// NewMapSliceSeq returns a [MapSliceSeq] holding items, reading them as
// [NewMapSlice] does.
func NewMapSliceSeq(items ...MapItem) (MapSliceSeq, error) {
	m, err := NewMapSlice(items...)

	return MapSliceSeq(m), err
}

// unusableKey reports a key that cannot address an entry.
func unusableKey(key any) error {
	return fmt.Errorf("cannot use %T as a key: %w", key, ErrKeyNotComparable)
}

// Len returns the number of entries.
func (s MapSlice) Len() int { return len(s.items) }

// At returns the entry standing at position i, counting from 0.
//
// It panics where a slice index panics, so range over [MapSlice.All] unless the
// position is what you want.
func (s MapSlice) At(i int) MapItem { return s.items[i] }

// Get returns the value stored under key, and whether the map holds one.
//
// A key Go cannot hash addresses no entry and reads as absent rather than
// panicking.
func (s MapSlice) Get(key any) (any, bool) {
	i := s.index(key)
	if i < 0 {
		return nil, false
	}

	return s.items[i].Value, true
}

// Set stores value under key.
//
// Where the key already stands in the map, the entry keeps its position and
// takes the new value; otherwise the entry goes on the end. It returns an error
// for a key Go cannot hash, which no map may hold.
func (s *MapSlice) Set(key, value any) error {
	if !hashableKey(key) {
		return unusableKey(key)
	}
	if i := s.index(key); i >= 0 {
		s.items[i].Value = value

		return nil
	}
	s.items = append(s.items, MapItem{Key: key, Value: value})

	return nil
}

// Delete removes the entry stored under key and reports whether it held one.
// The entries after it move up.
func (s *MapSlice) Delete(key any) bool {
	i := s.index(key)
	if i < 0 {
		return false
	}
	s.items = append(s.items[:i], s.items[i+1:]...)

	return true
}

// Keys iterates the keys in order.
func (s MapSlice) Keys() iter.Seq[any] {
	return func(yield func(any) bool) {
		for _, item := range s.items {
			if !yield(item.Key) {
				return
			}
		}
	}
}

// Values iterates the values in key order.
func (s MapSlice) Values() iter.Seq[any] {
	return func(yield func(any) bool) {
		for _, item := range s.items {
			if !yield(item.Value) {
				return
			}
		}
	}
}

// All iterates the entries in order, as ranging over a map would without one.
func (s MapSlice) All() iter.Seq2[any, any] {
	return func(yield func(any, any) bool) {
		for _, item := range s.items {
			if !yield(item.Key, item.Value) {
				return
			}
		}
	}
}

// ToMap returns the entries as a Go map, dropping the order.
//
// Every key is comparable, so the map holds as many entries as the MapSlice.
func (s MapSlice) ToMap() map[any]any {
	v := make(map[any]any, len(s.items))
	for _, item := range s.items {
		v[item.Key] = item.Value
	}

	return v
}

// index is where key stands, or -1.
//
// A scan: a MapSlice carries no lookup table, so a mapping of n entries costs
// n²/2 comparisons to build. Documents hold small mappings, and the table would
// have to be kept through every Set and Delete.
func (s MapSlice) index(key any) int {
	for i := range s.items {
		if sameMapKey(s.items[i].Key, key) {
			return i
		}
	}

	return -1
}

// Len returns the number of entries.
func (s MapSliceSeq) Len() int { return MapSlice(s).Len() }

// At returns the entry standing at position i, counting from 0.
func (s MapSliceSeq) At(i int) MapItem { return MapSlice(s).At(i) }

// Get returns the value stored under key, and whether the map holds one.
func (s MapSliceSeq) Get(key any) (any, bool) { return MapSlice(s).Get(key) }

// Set stores value under key, as [MapSlice.Set] does.
func (s *MapSliceSeq) Set(key, value any) error { return (*MapSlice)(s).Set(key, value) }

// Delete removes the entry stored under key and reports whether it held one.
func (s *MapSliceSeq) Delete(key any) bool { return (*MapSlice)(s).Delete(key) }

// Keys iterates the keys in order.
func (s MapSliceSeq) Keys() iter.Seq[any] { return MapSlice(s).Keys() }

// Values iterates the values in key order.
func (s MapSliceSeq) Values() iter.Seq[any] { return MapSlice(s).Values() }

// All iterates the entries in order.
func (s MapSliceSeq) All() iter.Seq2[any, any] { return MapSlice(s).All() }

// ToMap returns the entries as a Go map, dropping the order.
func (s MapSliceSeq) ToMap() map[any]any { return MapSlice(s).ToMap() }

// MarshalJSON writes the entries as a JSON object, in the order they stand.
//
// JSON specifies no ordering, so nothing carries the order to a reader that
// takes the object apart into a map -- but the bytes hold it, which is what a
// caller reading them in order, or diffing them, has. A MapSliceSeq writes the
// same object: "!!omap" is a YAML tag and JSON has no spelling for it.
//
// The keys are named by the encoder in its JSON style, which is the same path
// [MarshalWithOptions] with [JSON] takes, so a MapSlice and the document it was
// read from write the same JSON. TestToJSONMatchesTheValueConverter holds that
// path against [ToJSON] over the whole corpus.
func (s MapSlice) MarshalJSON() ([]byte, error) {
	out, err := MarshalWithOptions(s, JSON())
	if err != nil {
		return nil, err
	}

	// The encoder's JSON style writes a space after every ':' and ',', which
	// encoding/json does not. Compacted here so a MapSlice inside a struct
	// writes like everything around it.
	var compact bytes.Buffer
	if err := json.Compact(&compact, out); err != nil {
		return nil, err
	}

	return compact.Bytes(), nil
}

// UnmarshalJSON reads a JSON object, keeping the order its members were written
// in.
//
// encoding/json reads an object into a map and loses that order; this reads the
// token stream instead. A nested object becomes a MapSlice too, so the order is
// kept all the way down, and an array becomes a []any.
//
// Numbers resolve the way the YAML decoder resolves them -- uint64, int64 or
// float64 -- and not encoding/json's float64 for everything. JSON has one
// number type and YAML has integers, so a value read here and written back as
// YAML keeps the type the YAML side gives it.
func (s *MapSlice) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	v, err := jsonOrdered(dec)
	if err != nil {
		return err
	}
	m, isMap := v.(MapSlice)
	if !isMap {
		return fmt.Errorf("cannot read %s into a MapSlice: %w", jsonKindOf(v), ErrNotAJSONObject)
	}
	*s = m

	return nil
}

// MarshalJSON writes the entries as a JSON object, as [MapSlice.MarshalJSON]
// does: JSON has no spelling for the "!!omap" that tells the two types apart.
func (s MapSliceSeq) MarshalJSON() ([]byte, error) { return MapSlice(s).MarshalJSON() }

// UnmarshalJSON reads a JSON object, as [MapSlice.UnmarshalJSON] does.
func (s *MapSliceSeq) UnmarshalJSON(b []byte) error { return (*MapSlice)(s).UnmarshalJSON(b) }

// ErrNotAJSONObject reports JSON that is not an object handed to a type that
// holds one.
var ErrNotAJSONObject = errors.New("not a JSON object")

// jsonOrdered reads one JSON value, building a MapSlice for every object.
func jsonOrdered(dec *json.Decoder) (any, error) {
	tk, err := dec.Token()
	if err != nil {
		return nil, err
	}

	delim, isDelim := tk.(json.Delim)
	if !isDelim {
		return jsonScalar(tk), nil
	}

	switch delim {
	case '{':
		var m MapSlice
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			value, err := jsonOrdered(dec)
			if err != nil {
				return nil, err
			}
			name, isString := key.(string)
			if !isString {
				return nil, fmt.Errorf("a JSON member name is a string: %w", ErrNotAJSONObject)
			}
			// A member written twice keeps the last value, where the first
			// stands, as Set does everywhere else.
			if err := m.Set(name, value); err != nil {
				return nil, err
			}
		}
		_, err := dec.Token() // the closing brace

		return m, err
	case '[':
		list := []any{}
		for dec.More() {
			value, err := jsonOrdered(dec)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		_, err := dec.Token() // the closing bracket

		return list, err
	default:
		return nil, fmt.Errorf("unexpected %q: %w", delim, ErrNotAJSONObject)
	}
}

// jsonScalar is what a JSON scalar token is worth, reading a number as the YAML
// decoder reads one.
func jsonScalar(tk json.Token) any {
	number, isNumber := tk.(json.Number)
	if !isNumber {
		return tk
	}
	if u, err := strconv.ParseUint(number.String(), 10, 64); err == nil {
		return u
	}
	if i, err := strconv.ParseInt(number.String(), 10, 64); err == nil {
		return i
	}
	f, _ := strconv.ParseFloat(number.String(), 64)

	return f
}

// jsonKindOf names what a JSON document holds, for the error that refuses it.
func jsonKindOf(v any) string {
	switch v.(type) {
	case []any:
		return "an array"
	case string:
		return "a string"
	case nil:
		return "null"
	default:
		return "a scalar"
	}
}
