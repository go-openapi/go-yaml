// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestALoneQuestionMarkInAFlowCollectionIsRefused covers a '?' that a flow indicator follows.
//
// ns-plain-first [126] opens a plain scalar with a '?' only before an ns-plain-safe character, which leaves out the
// flow indicators, and an explicit key's '?' needs a separation after it ([142], [150]). The scanner read "[?]" as
// ["?"] and "{?}" as {"?": null}. yaml/yaml-reference-parser refuses each document below; libfyaml reads them as the
// scanner did.
func TestALoneQuestionMarkInAFlowCollectionIsRefused(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		for _, src := range []string{"[?]\n", "{?}\n", "[?, a]\n", "{a: ?}\n", "{?, a}\n", "[?]: b\n", "[?[a]]\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "'?' is not a scalar, and an explicit key needs a space after it", "%q", src)
		}
	})

	t.Run("read as before", func(t *testing.T) {
		for src, want := range map[string]any{
			"[? ]\n":       []any{map[any]any{nil: nil}},
			"[?\n]\n":      []any{map[any]any{nil: nil}},
			"{? : a}\n":    map[any]any{nil: "a"},
			"[?x]\n":       []any{"?x"},
			"{a: ?x}\n":    map[string]any{"a": "?x"},
			"[&? x, *?]\n": []any{"x", "x"},
			// Outside a flow collection a ']' or a '}' is plain-safe, and the '?' opens a plain scalar.
			"k: ?]\n": map[string]any{"k": "?]"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, want, got, "%q", src)
		}
	})
}
