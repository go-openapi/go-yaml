// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// The interfaces a type implements to encode itself.
//
// [Marshaler] writes the YAML text of the value, as [encoding/json.Marshaler]
// writes its JSON. [GoYAMLMarshaler] hands back another Go value to encode in
// its place, which is what github.com/go-yaml/yaml asks for and is kept so a
// type written for that library encodes here unchanged.
//
// A type satisfying more than one is written by the first the encoder looks
// for, so implement the one that says what the type means.
//
// An error returned by MarshalYAML stops the encoding and reaches the caller.

// Marshaler returns the YAML text to write for the value.
type Marshaler interface {
	MarshalYAML() ([]byte, error)
}

// GoYAMLMarshaler returns another Go value to encode in place of this one.
//
// This is github.com/go-yaml/yaml's shape. Prefer [Marshaler], which writes the
// text and needs no second pass over what it returns.
type GoYAMLMarshaler interface {
	MarshalYAML() (any, error)
}

// The interfaces a type implements to decode itself.
//
// [Unmarshaler] is handed the YAML text of the value, as
// [encoding/json.Unmarshaler] is handed its JSON. [GoYAMLUnmarshaler] is handed
// a function that decodes into whatever it is given, which is what
// github.com/go-yaml/yaml asks for. [NodeUnmarshaler] is handed the
// [ast.Node], which carries what the text does not -- the comments, and where
// each token stood.

// Unmarshaler is handed the YAML text of the value.
type Unmarshaler interface {
	UnmarshalYAML(data []byte) error
}

// GoYAMLUnmarshaler is handed a function that decodes the value into whatever
// it is given.
//
// This is github.com/go-yaml/yaml's shape. Prefer [Unmarshaler], which is handed
// the text directly.
type GoYAMLUnmarshaler interface {
	UnmarshalYAML(unmarshal func(any) error) error
}

// NodeUnmarshaler is handed the node the value was read from.
type NodeUnmarshaler interface {
	UnmarshalYAML(node ast.Node) error
}

// Base64 is the value a "!!binary" scalar decodes to: the base64 text the
// document carries, with the bytes it stands for a method call away.
//
// The text and not the bytes, for three reasons. It is comparable, so a
// "!!binary" scalar can key a mapping where a []byte cannot -- Go hashes no
// slice. It survives a round trip, since a Base64 tells an encoder it is
// binary where a plain string or a []byte does not. And it is what JSON means
// by binary, so [ToJSON] and a decode followed by a JSON encode write the same
// thing.
//
// The underlying type is string, so a caller may compare two, use one as a map
// key, and print one without decoding anything. RFC 2045 lets the encoded
// stream carry line breaks; a Base64 holds them as the document wrote them, and
// [Base64.Canonical] takes them out.
type Base64 string

// Bytes returns the bytes the text stands for.
//
// The decode happens here and not when the document was read, so a caller who
// only moves the value about never pays for it. An error says the text is not
// RFC 2045 base64, which a "!!binary" that resolved cannot be -- reach for it
// on a Base64 a caller built themselves.
func (b Base64) Bytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(b.Canonical())
}

// String returns the base64 text, which is what a Base64 holds.
//
// It is the encoded form and not the bytes: printing binary would write
// whatever the payload happens to be, and a Stringer is read by fmt wherever a
// value is logged. Use [Base64.Bytes] for the payload.
func (b Base64) String() string { return string(b) }

// Canonical returns the text with the line breaks and spacing RFC 2045 permits
// inside an encoded stream taken out, which is base64's canonical spelling.
// [token.CanonicalBase64] holds the rule, which the parser also names a
// "!!binary" key by.
func (b Base64) Canonical() string { return token.CanonicalBase64(string(b)) }

// MarshalJSON writes the base64 text as a JSON string, in canonical form.
//
// JSON has no binary type and no tags, so a Base64 is its text -- which is what
// [ToJSON] writes for the "!!binary" it came from, and what encoding/json
// writes for a []byte. The line breaks RFC 2045 permits are taken out, so a
// value carried through Go comes back on one line.
func (b Base64) MarshalJSON() ([]byte, error) {
	return appendJSONString(nil, b.Canonical()), nil
}

// UnmarshalJSON reads a JSON string as base64 text.
//
// The text is validated and not decoded: a Base64 holds the encoded form, and
// [Base64.Bytes] is where the payload is read. A string that is not RFC 2045
// base64 is refused here rather than at the first call to Bytes.
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

var (
	globalCustomMarshalerMu    sync.Mutex
	globalCustomUnmarshalerMu  sync.Mutex
	globalCustomMarshalerMap   = map[reflect.Type]func(any) ([]byte, error){}
	globalCustomUnmarshalerMap = map[reflect.Type]func(any, []byte) error{}
)

// customArg returns v as the T a custom (un)marshaler was registered for.
//
// The custom (un)marshaler maps are keyed by the reflect.Type of T and looked
// up by the type of the value at hand, so v is always a T.
func customArg[T any](v any) T {
	return v.(T) //nolint:forcetypeassert // the maps are keyed by T's type and looked up by v's
}

// RegisterCustomMarshaler overrides any encoding process for the type specified in generics.
// If you want to switch the behavior for each encoder, use `CustomMarshaler` defined as EncodeOption.
//
// NOTE: If type T implements MarshalYAML for pointer receiver, the type specified in RegisterCustomMarshaler must be *T.
// If RegisterCustomMarshaler and CustomMarshaler of EncodeOption are specified for the same type,
// the CustomMarshaler specified in EncodeOption takes precedence.
func RegisterCustomMarshaler[T any](marshaler func(T) ([]byte, error)) {
	globalCustomMarshalerMu.Lock()
	defer globalCustomMarshalerMu.Unlock()

	var typ T
	globalCustomMarshalerMap[reflect.TypeOf(typ)] = func(v any) ([]byte, error) {
		return marshaler(customArg[T](v))
	}
}

// RegisterCustomUnmarshaler overrides any decoding process for the type specified in generics.
// If you want to switch the behavior for each decoder, use `CustomUnmarshaler` defined as DecodeOption.
//
// NOTE: If RegisterCustomUnmarshaler and CustomUnmarshaler of DecodeOption are specified for the same type,
// the CustomUnmarshaler specified in DecodeOption takes precedence.
func RegisterCustomUnmarshaler[T any](unmarshaler func(*T, []byte) error) {
	globalCustomUnmarshalerMu.Lock()
	defer globalCustomUnmarshalerMu.Unlock()

	var typ *T
	globalCustomUnmarshalerMap[reflect.TypeOf(typ)] = func(v any, b []byte) error {
		return unmarshaler(customArg[*T](v), b)
	}
}
