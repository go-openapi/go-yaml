// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestInlineOnlyGoesOnAStructOrAMap covers what `,inline` may be written on.
//
// go.yaml.in/yaml/v3 takes a struct, or a chain of pointers ending in one, and
// promotes its fields; or a map with string keys, which takes the entries no
// field claims. It refuses everything else, refuses a map whose keys are not
// exactly string, and refuses a second map in the same struct, since only one
// of them could take the leftovers.
//
// All three passed silently here. `,inline` on an int marked the field inline
// and then matched no entry, so the field stayed zero and Unmarshal reported
// success; two inline maps left both empty.
func TestInlineOnlyGoesOnAStructOrAMap(t *testing.T) {
	refused := func(t *testing.T, v any, want string) {
		t.Helper()

		err := codec.Unmarshal([]byte("b: B\nx: X\n"), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), want)
	}

	const notInlinable = "option ,inline may only be used on a struct or map field"

	t.Run("an int", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a slice", func(t *testing.T) {
		var v struct {
			S []string `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("an interface", func(t *testing.T) {
		var v struct {
			I any `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a pointer to a map, which v3 does not follow", func(t *testing.T) {
		var v struct {
			M *map[string]any `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a map keyed by anything but string", func(t *testing.T) {
		var v struct {
			M map[int]any `yaml:",inline"`
		}
		refused(t, &v, "option ,inline needs a map with string keys in struct ")
	})

	t.Run("a map keyed by a named string type", func(t *testing.T) {
		type key string

		var v struct {
			M map[key]any `yaml:",inline"`
		}
		refused(t, &v, "option ,inline needs a map with string keys in struct ")
	})

	t.Run("two maps", func(t *testing.T) {
		var v struct {
			A map[string]any `yaml:",inline"`
			B map[string]any `yaml:",inline"`
		}
		refused(t, &v, "multiple ,inline maps in struct ")
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		_, err := codec.Marshal(v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), notInlinable)
	})

	t.Run("the message names the field and its type", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		err := codec.Unmarshal([]byte("b: B\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".N is int")
	})
}

// TestTheInlineTargetsV3AcceptsStillRead holds the other side of the rule.
func TestTheInlineTargetsV3AcceptsStillRead(t *testing.T) {
	type Payload struct {
		B string `yaml:"b"`
	}

	t.Run("a struct", func(t *testing.T) {
		var v struct {
			Payload `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\n"), &v))
		assert.Equal(t, "B", v.B)
	})

	t.Run("a chain of pointers to a struct", func(t *testing.T) {
		var v struct {
			P **Payload `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\n"), &v))
	})

	t.Run("a named map type with string keys", func(t *testing.T) {
		type rest map[string]any

		var v struct {
			Known string `yaml:"known"`
			Rest  rest   `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("known: K\nx: X\n"), &v))
		assert.Equal(t, rest{"x": "X"}, v.Rest)
	})

	t.Run("one map beside an inlined struct", func(t *testing.T) {
		var v struct {
			Payload `yaml:",inline"`

			Rest map[string]any `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\nx: X\n"), &v))
		assert.Equal(t, "B", v.B)
		assert.Equal(t, map[string]any{"x": "X"}, v.Rest)
	})
}
