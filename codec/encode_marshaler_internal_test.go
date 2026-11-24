// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding"
	"encoding/json"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

type selfMarshaled struct{}

func (selfMarshaled) MarshalYAML() ([]byte, error) { return []byte("x"), nil }

type jsonOnly struct{}

func (jsonOnly) MarshalJSON() ([]byte, error) { return []byte(`"x"`), nil }

// boxedMarshalerKind is the type switch canEncodeByMarshaler ran on a boxed value before it decided from the type.
func boxedMarshalerKind(iface any) marshalerKind {
	switch iface.(type) {
	case ast.Node:
		return encodedByTheEncoder
	case MapSlice, MapSliceSeq, MapItem, Base64:
		return encodedByTheEncoder
	case Marshaler, GoYAMLMarshaler, time.Time, *time.Time, time.Duration, encoding.TextMarshaler:
		return marshalsItself
	case jsonMarshaler:
		return marshalsAsJSON
	}
	return encodedByTheEncoder
}

// TestMarshalerKindOfMatchesTheTypeSwitch checks marshalerKindOf against the type switch it replaces, on a value of
// each case, a value that falls through, and the same values held in an interface.
func TestMarshalerKindOfMatchesTheTypeSwitch(t *testing.T) {
	values := []any{
		"s", 1, 1.5, true,
		[]int{1},
		map[string]int{},
		ast.String(token.New("a", "a", token.Position{})),
		MapSlice{},
		MapSliceSeq{},
		MapItem{},
		Base64("YQ=="),
		selfMarshaled{},
		&selfMarshaled{},
		time.Time{},
		&time.Time{}, time.Second,
		net.IPv4(127, 0, 0, 1),
		jsonOnly{},
		json.RawMessage(`1`),
	}
	for _, value := range values {
		v := reflect.ValueOf(value)
		want := boxedMarshalerKind(value)

		assert.Equalf(t, want, marshalerKindOf(v.Type()), "%T", value)

		held := reflect.ValueOf(&value).Elem()
		for _, useJSON := range []bool{false, true} {
			e := &Encoder{useJSONMarshaler: useJSON}
			assert.Equalf(t, want == marshalsItself || (useJSON && want == marshalsAsJSON), e.canEncodeByMarshaler(held),
				"%T in an interface, UseJSONMarshaler %v", value, useJSON)
		}
	}

	var empty any
	assert.False(t, (&Encoder{}).canEncodeByMarshaler(reflect.ValueOf(&empty).Elem()), "a nil interface")
}

// TestCanEncodeByMarshalerDoesNotBoxAString covers the allocation canEncodeByMarshaler made on every value it was
// asked about: v.Interface() boxes a string into 16 bytes.
func TestCanEncodeByMarshalerDoesNotBoxAString(t *testing.T) {
	e := &Encoder{}
	v := reflect.ValueOf([]string{"key"}).Index(0)
	_ = e.canEncodeByMarshaler(v)

	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = e.canEncodeByMarshaler(v) }))
}
