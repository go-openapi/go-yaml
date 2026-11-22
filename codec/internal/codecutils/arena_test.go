// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"runtime"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestArenaBoxHoldsWhatItWasGiven checks the hand-built interface behaves like
// one the compiler made: it asserts, compares and keys a map.
func TestArenaBoxHoldsWhatItWasGiven(t *testing.T) {
	var a Arena

	want := make([]string, 0, 3*arenaSlab)
	got := make([]any, 0, 3*arenaSlab)
	for i := range 3 * arenaSlab {
		s := string(rune('a'+i%26)) + string(rune('0'+i%10)) + strings.Repeat("x", i%64)
		want = append(want, s)
		got = append(got, a.Value(s))
	}

	// The interface points into a slab the arena no longer holds, so the
	// collector has to keep that slab alive on the strength of the interface
	// alone. Dropping the arena and collecting twice checks that it does.
	a = Arena{}
	runtime.GC()
	runtime.GC()

	m := map[any]int{}
	for i, v := range got {
		s, ok := v.(string)
		require.Truef(t, ok, "%d: holds %T, not a string", i, v)
		require.Equalf(t, want[i], s, "%d", i)
		assert.Truef(t, v == any(want[i]), "%d: does not compare equal to the same string", i)
		m[v] = i
	}
	assert.Len(t, m, len(got), "boxed strings key a map, one entry each")
}
