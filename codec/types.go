// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strconv"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// Base64 holds the base64 text of a "!!binary" scalar, as an any destination decodes it. [Base64.Bytes] returns the
// bytes.
//
// It holds the text, not the bytes, for three reasons:
//
//   - The text is comparable, so a "!!binary" scalar can key a mapping. A []byte cannot, because Go hashes no slice.
//   - It survives a round trip: a Base64 tells the encoder to write "!!binary", where a string or a []byte does not.
//   - JSON writes binary as base64 text, so [ToJSON] and a decode followed by a JSON encode write the same thing.
//
// The underlying type is string, so you can compare two values, use one as a map key, and print one without decoding
// anything. RFC 2045 lets the encoded text carry line breaks. A Base64 keeps them as the document wrote them, and
// [Base64.Canonical] removes them.
type Base64 string

// Bytes returns the bytes the text stands for.
//
// The bytes are decoded here, not when the document is read, so a caller that only passes the value on never pays for
// the decode. Bytes returns an error when the text is not RFC 2045 base64. A decoded "!!binary" is always valid, so
// the error can only come from a Base64 the caller built.
func (b Base64) Bytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(b.Canonical())
}

// String returns the base64 text.
//
// It returns the encoded form, not the bytes, because fmt calls String wherever the value is logged, and the raw
// payload may not be printable. Use [Base64.Bytes] for the payload.
func (b Base64) String() string { return string(b) }

// Canonical returns the text without the line breaks and spaces that RFC 2045 allows inside the encoded text.
// It applies [token.CanonicalBase64], which the parser also uses to name a "!!binary" key.
func (b Base64) Canonical() string { return token.CanonicalBase64(string(b)) }

// MarshalJSON writes the base64 text as a JSON string, in canonical form.
//
// JSON has no binary type and no tags, so a Base64 is written as its text, as [ToJSON] writes the "!!binary" it came
// from and as encoding/json writes a []byte. The line breaks are removed, so the value comes back on one line.
func (b Base64) MarshalJSON() ([]byte, error) {
	return appendJSONString(nil, b.Canonical()), nil
}

// UnmarshalJSON reads a JSON string as base64 text.
//
// UnmarshalJSON checks the text but does not decode it, because a Base64 holds the encoded form and [Base64.Bytes]
// decodes it. A string that is not RFC 2045 base64 returns an error here, so the first call to Bytes cannot fail.
func (b *Base64) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if _, err := base64.StdEncoding.DecodeString(Base64(text).Canonical()); err != nil {
		return fmt.Errorf("cannot read %q as base64: %w", text, err)
	}
	*b = Base64(text)

	return nil
}

// RawMessage holds an encoded YAML value as it stands, as [encoding/json.RawMessage] does for JSON.
//
// Use it to delay the decoding of part of a document, or to write YAML text that is already encoded. It implements
// [Marshaler] and [Unmarshaler], and also [encoding/json.Marshaler] and [encoding/json.Unmarshaler].
type RawMessage []byte

// MarshalYAML returns the encoded value, or "null" for a nil RawMessage.
func (m RawMessage) MarshalYAML() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalYAML stores a copy of the encoded value.
func (m *RawMessage) UnmarshalYAML(dt []byte) error {
	if m == nil {
		return errors.New("yaml.RawMessage: UnmarshalYAML on nil pointer")
	}
	*m = append((*m)[0:0], dt...)
	return nil
}

// UnmarshalJSON stores a copy of a JSON value, which is also YAML.
func (m *RawMessage) UnmarshalJSON(b []byte) error {
	return m.UnmarshalYAML(b)
}

// MarshalJSON converts the encoded YAML value to JSON.
func (m RawMessage) MarshalJSON() ([]byte, error) {
	return ToJSON(m)
}

// MapItem is one entry of a [MapSlice] or a [MapSliceSeq].
//
// A decode fills Key with the value the key resolves to, as it fills a map[any]any key: "1:" gives uint64(1), "1.0:"
// gives float64(1), "null:" gives nil and "true:" gives true. So the string "1.0" and the float 1.0 are two entries,
// as YAML 1.2.2 section 3.2.1.1 requires. Use [UseStringKeys] to read every key as text instead.
//
// A key must be comparable. Go hashes no slice, map or function, so a decode returns an error for a collection
// written as a mapping key, as it does for every other destination.
type MapItem struct {
	Key, Value any
}

// MapSlice decodes and encodes as a YAML mapping, and keeps the order of its entries and keys that a Go map cannot
// tell apart.
//
// It holds both entries of "1: a" and "\"1\": b". A map[string]any rejects the pair as a duplicate, and a map[any]any
// keeps both but loses the order.
//
// Every key is unique and comparable. [MapSlice.Set] overwrites the entry that already holds a key, and returns an
// error for a key Go cannot hash. The zero value is an empty map ready for Set. Use [NewMapSlice] to build one from
// entries you already have.
//
// Use [UseOrderedMap] to read every mapping of a document into one.
type MapSlice struct {
	items []MapItem
}

// NewMapSlice returns a MapSlice holding items, in the order given.
//
// It returns an error for a key Go cannot hash, and for a key given twice: two items with one key are a mistake here,
// while [MapSlice.Set] replaces the value on purpose.
func NewMapSlice(items ...MapItem) (MapSlice, error) {
	var m MapSlice
	if len(items) == 0 {
		// The zero value, so an empty MapSlice built here compares equal to one
		// a decode builds for an empty mapping.
		return m, nil
	}
	m.items = make([]MapItem, 0, len(items))
	for _, item := range items {
		if !codecutils.HashableKey(item.Key) {
			return MapSlice{}, codecutils.UnusableKey(item.Key)
		}
		if i := m.index(item.Key); i >= 0 {
			return MapSlice{}, fmt.Errorf("key %v is given twice: %w", item.Key, yamlerrors.ErrDuplicateKey)
		}
		m.items = append(m.items, item)
	}

	return m, nil
}

// Len returns the number of entries.
func (s MapSlice) Len() int { return len(s.items) }

// At returns the entry standing at position i, counting from 0.
//
// It panics where a slice index panics. Range over [MapSlice.All] unless you need the position.
func (s MapSlice) At(i int) MapItem { return s.items[i] }

// Get returns the value stored under key, and whether the map holds one.
//
// A key Go cannot hash addresses no entry: Get reports it absent, and does not panic.
func (s MapSlice) Get(key any) (any, bool) {
	i := s.index(key)
	if i < 0 {
		return nil, false
	}

	return s.items[i].Value, true
}

// Set stores value under key.
//
// Where the map already holds the key, the entry keeps its position and takes the new value. Otherwise the entry goes
// at the end. Set returns an error for a key Go cannot hash.
func (s *MapSlice) Set(key, value any) error {
	if !codecutils.HashableKey(key) {
		return codecutils.UnusableKey(key)
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

// Values iterates the values in entry order.
func (s MapSlice) Values() iter.Seq[any] {
	return func(yield func(any) bool) {
		for _, item := range s.items {
			if !yield(item.Value) {
				return
			}
		}
	}
}

// All iterates the keys and values in entry order.
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

// MarshalJSON writes the entries as a JSON object, in entry order.
//
// JSON gives an object no order, so a reader that loads the object into a map loses it. The bytes keep it, for a
// reader that reads them in order or compares them. A [MapSliceSeq] writes the same object, because JSON cannot write
// the "!!omap" tag.
//
// The encoder names the keys as [Marshal] with [JSON] does, so a MapSlice writes the same JSON as [ToJSON] writes for
// the document it was read from.
func (s MapSlice) MarshalJSON() ([]byte, error) {
	out, err := Marshal(s, JSON())
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
// encoding/json reads an object into a map and loses the order, so UnmarshalJSON reads the token stream instead.
// A nested object becomes a MapSlice too, so the order is kept at every depth, and an array becomes a []any.
//
// A number resolves as the YAML decoder resolves it, to a uint64, an int64 or a float64, where encoding/json gives a
// float64 for every number. A value read here and written back as YAML keeps the type the YAML decoder gives it.
func (s *MapSlice) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	v, err := jsonOrdered(dec)
	if err != nil {
		return err
	}
	m, isMap := v.(MapSlice)
	if !isMap {
		return fmt.Errorf("cannot read %s into a MapSlice: %w", jsonKindOf(v), yamlerrors.ErrNotAJSONObject)
	}
	*s = m

	return nil
}

// index is where key stands, or -1.
//
// A scan: a MapSlice carries no lookup table, so a mapping of n entries costs
// n²/2 comparisons to build. Documents hold small mappings, and the table would
// have to be kept through every Set and Delete.
func (s MapSlice) index(key any) int {
	for i := range s.items {
		if codecutils.SameMapKey(s.items[i].Key, key) {
			return i
		}
	}

	return -1
}

// MapSliceSeq is a [MapSlice] that reads and writes YAML's "!!omap", a sequence of one-entry mappings:
// "!!omap [{x: 1}, {y: 2}]".
//
// It holds the same entries under the same rules. The encoder writes a MapSlice as a plain mapping, and a MapSliceSeq
// as the tagged sequence, with or without [UseOrderedMap]. A decode into an any builds a MapSliceSeq for a node tagged
// "!!omap", and a decode into a named destination builds the type it names.
//
// The two convert into each other: MapSlice(seq) and MapSliceSeq(m).
type MapSliceSeq MapSlice

// NewMapSliceSeq returns a [MapSliceSeq] holding items, with the checks of [NewMapSlice].
func NewMapSliceSeq(items ...MapItem) (MapSliceSeq, error) {
	m, err := NewMapSlice(items...)

	return MapSliceSeq(m), err
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

// Values iterates the values in entry order.
func (s MapSliceSeq) Values() iter.Seq[any] { return MapSlice(s).Values() }

// All iterates the entries in order.
func (s MapSliceSeq) All() iter.Seq2[any, any] { return MapSlice(s).All() }

// ToMap returns the entries as a Go map, dropping the order.
func (s MapSliceSeq) ToMap() map[any]any { return MapSlice(s).ToMap() }

// MarshalJSON writes the entries as a JSON object, as [MapSlice.MarshalJSON] does, because JSON cannot write the
// "!!omap" tag that tells the two types apart.
func (s MapSliceSeq) MarshalJSON() ([]byte, error) { return MapSlice(s).MarshalJSON() }

// UnmarshalJSON reads a JSON object, as [MapSlice.UnmarshalJSON] does.
func (s *MapSliceSeq) UnmarshalJSON(b []byte) error { return (*MapSlice)(s).UnmarshalJSON(b) }

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
				return nil, fmt.Errorf("a JSON member name is a string: %w", yamlerrors.ErrNotAJSONObject)
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
		return nil, fmt.Errorf("unexpected %q: %w", delim, yamlerrors.ErrNotAJSONObject)
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
