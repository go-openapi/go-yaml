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

// RegisterCustomMarshaler overrides any encoding process for the type specified in generics.
// If you want to switch the behavior for each encoder, use `CustomMarshaler` defined as EncodeOption.
//
// NOTE: If type T implements MarshalYAML for pointer receiver, the type specified in RegisterCustomMarshaler must be *T.
// If RegisterCustomMarshaler and CustomMarshaler of EncodeOption are specified for the same type,
// the CustomMarshaler specified in EncodeOption takes precedence.
func RegisterCustomMarshaler[T any](marshaler func(T) ([]byte, error)) {
	customMarshalerMx.Lock()
	defer customMarshalerMx.Unlock()

	var typ T
	customMarshalerMap[reflect.TypeOf(typ)] = func(v any) ([]byte, error) {
		return marshaler(codecutils.CustomArg[T](v))
	}
}

// RegisterCustomUnmarshaler overrides any decoding process for the type specified in generics.
// If you want to switch the behavior for each decoder, use `CustomUnmarshaler` defined as DecodeOption.
//
// NOTE: If RegisterCustomUnmarshaler and CustomUnmarshaler of DecodeOption are specified for the same type,
// the CustomUnmarshaler specified in DecodeOption takes precedence.
func RegisterCustomUnmarshaler[T any](unmarshaler func(*T, []byte) error) {
	customUnmarshalerMx.Lock()
	defer customUnmarshalerMx.Unlock()

	var typ *T
	customUnmarshalerMap[reflect.TypeOf(typ)] = func(v any, b []byte) error {
		return unmarshaler(codecutils.CustomArg[*T](v), b)
	}
	// A type walkableType read before the registration may hold T.
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
