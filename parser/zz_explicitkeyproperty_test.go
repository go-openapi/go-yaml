// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAPropertyKeepsAnExplicitKeyOffItsLine covers a node property written before an explicit key's "?" on the same
// line.
//
// The properties of a block mapping stand on a line of their own, as those of a block sequence do: "&m" over "? a"
// anchors the mapping, and "&m ? a" is no document. The grouping refused a "-" after a property and let a "?" through,
// so "!!null ?" was read as a tagged mapping and "k: &m ? a" as {k: {a: null}}. Inside a flow collection the "?"
// after a property is refused the same way.
//
// Every verdict here is yaml/yaml-reference-parser's and libfyaml's.
func TestAPropertyKeepsAnExplicitKeyOffItsLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"tag-then-an-empty-key", "!!null ?\n", "[1:8] explicit keys are not allowed after a tag on the same line"},
		{"anchor-then-an-empty-key", "&m ?\n", "[1:4] explicit keys are not allowed after anchor on the same line"},
		{"anchor-under-a-key", "k: &m ? a\n", "[1:7] explicit keys are not allowed after anchor on the same line"},
		{"tag-under-a-key", "k: !!map ? a\n", "[1:10] explicit keys are not allowed after a tag on the same line"},
		{"anchor-in-a-sequence-entry", "- &m ? a\n", "[1:6] explicit keys are not allowed after anchor on the same line"},
		{"anchor-on-the-line-below-its-key", "k:\n  &m ? a\n", "[2:6] explicit keys are not allowed after anchor on the same line"},
		{"anchor-then-a-tag", "k: &m !!map ? a\n", "[1:13] explicit keys are not allowed after a tag on the same line"},
		{"tag-then-an-anchor", "k: !!map &m ? a\n", "[1:13] explicit keys are not allowed after anchor on the same line"},
		{"anchor-in-a-flow-sequence", "[&m ? a]", "[1:5] explicit keys are not allowed after anchor on the same line"},
		{"anchor-in-a-flow-mapping", "{&m ? a : b}", "[1:5] explicit keys are not allowed after anchor on the same line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"property-on-a-line-of-its-own", "&m\n? a\n: b\n"},
		{"property-on-its-own-line-under-a-key", "k:\n  &m\n  ? a\n"},
		{"tag-on-a-line-of-its-own", "!!map\n? a\n"},
		{"plain-scalar-starting-with-a-question-mark", "k: &m ?x\n"},
		{"property-on-the-key-itself", "? &m a\n: b\n"},
		{"property-on-the-value", "&m\n? a\n: &n b\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
		})
	}
}
