// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"reflect"
	"sync"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
)

var (
	customMarshalerMx    sync.RWMutex
	customUnmarshalerMx  sync.RWMutex
	customMarshalerMap   = map[reflect.Type]func(any) ([]byte, error){}
	customUnmarshalerMap = map[reflect.Type]func(any, []byte) error{}
)

// RegisterCustomMarshaler makes every encoder encode a T with marshaler, which returns the YAML text to write, in place
// of any other encoding of T. Use [CustomMarshaler] to set it for one encoder only.
//
// Where T implements MarshalYAML on a pointer receiver, give *T as the type parameter. A [CustomMarshaler] option for
// the same type takes precedence.
func RegisterCustomMarshaler[T any](marshaler func(T) ([]byte, error)) {
	customMarshalerMx.Lock()
	defer customMarshalerMx.Unlock()

	var typ T
	customMarshalerMap[reflect.TypeOf(typ)] = func(v any) ([]byte, error) {
		return marshaler(codecutils.CustomArg[T](v))
	}
}

// RegisterCustomUnmarshaler makes every decoder decode a T with unmarshaler, which receives the YAML text of the node,
// in place of any other decoding of T. Use [CustomUnmarshaler] to set it for one decoder only.
//
// A [CustomUnmarshaler] option for the same type takes precedence.
func RegisterCustomUnmarshaler[T any](unmarshaler func(*T, []byte) error) {
	customUnmarshalerMx.Lock()
	defer customUnmarshalerMx.Unlock()

	var typ *T
	customUnmarshalerMap[reflect.TypeOf(typ)] = func(v any, b []byte) error {
		return unmarshaler(codecutils.CustomArg[*T](v), b)
	}
	// A type read by walkableType before the registration may hold T.
	for mode := range walkableTypes {
		walkableTypes[mode].Clear()
	}
}

// registeredUnmarshaler reports whether RegisterCustomUnmarshaler holds a
// function for the type ptr, a pointer to the type it was registered for.
func registeredUnmarshaler(ptr reflect.Type) bool {
	customUnmarshalerMx.RLock()
	_, exists := customUnmarshalerMap[ptr]
	customUnmarshalerMx.RUnlock()

	return exists
}

// hasCustomMarshalers reports whether RegisterCustomMarshaler has registered any marshaler.
func hasCustomMarshalers() bool {
	customMarshalerMx.RLock()
	defer customMarshalerMx.RUnlock()

	return len(customMarshalerMap) > 0
}
