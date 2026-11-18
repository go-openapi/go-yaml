// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// TestNormalizeKeepsTheValueUnderANaNKey covers a map keyed on a NaN, which ".nan: x" decodes to under an `any`.
//
// normalizeMap read each value back with MapIndex, and a NaN key never finds itself, so the value read as nil and
// TestDecodingIntoAGoTypeGivesTheSameValue reported the `any` path as losing it. The decoder had kept it.
func TestNormalizeKeepsTheValueUnderANaNKey(t *testing.T) {
	got := yamlgen.Normalize(reflect.ValueOf(map[any]any{math.NaN(): "x", "a": uint64(1)}))

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("Normalize returned %T", got)
	}
	if m[".nan"] != "x" {
		t.Errorf(`the NaN key's value reads %#v, want "x"; the map is %#v`, m[".nan"], m)
	}
	if m["a"] != uint64(1) {
		t.Errorf("the other key's value reads %#v, want 1", m["a"])
	}
}
