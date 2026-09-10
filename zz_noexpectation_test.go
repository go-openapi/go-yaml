// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package yaml_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// noExpectationPins says what each fixture decodeLedger files under
// reasonNoExpectation means.
//
// Those fixtures carry in.yaml alone. No in.json can exist for them: every one
// holds a key JSON has no spelling for -- an empty key, or a whole collection
// standing as one -- except syntax-character-edge-cases/02, whose document is
// the non-specific tag on the empty node. TestYAMLTestSuite counts them and
// scores none of them, so nothing said what they compose to until these pins.
//
// Each pin decodes the vendored in.yaml into codec.MapSlice, which carries a
// null or collection key where map[string]any cannot: Unmarshal into any gives
// the keys as "null" and "map[earth:blue]", the fmt rendering of a value the
// map's key type will not hold.
//
// Three are specification examples, and the composition the specification
// publishes for each is quoted in its pin. The other six state what the parser
// and the decoder agree the document holds.
//
// zero-indented-sequences-in-explicit-mapping-keys was a valid document the
// parser lost outright until 8.2.2's seq-space was admitted as an explicit
// key's body, and no figure moved when it was: this is where that would have
// shown. TestAZeroIndentedSequenceIsAnExplicitKeysBody pins the parser half.
var noExpectationPins = map[string]func(*testing.T, []byte){ //nolint:gochecknoglobals // a test table
	// ": a\n: b\n". One mapping, and the same empty key twice, which 3.2.1.1
	// forbids.
	"block-mapping-with-missing-keys": func(t *testing.T, src []byte) {
		var ms codec.MapSlice
		err := codec.NewDecoder(bytes.NewReader(src)).Decode(&ms)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `mapping key "null" already defined at [1:1]`)
	},

	// Four documents writing the same two mappings in block style and in flow
	// style. Both spellings compose to the same thing, which is what the
	// fixture is for.
	"empty-keys-in-block-and-flow-mapping": func(t *testing.T, src []byte) {
		assert.Equal(t, []codec.MapSlice{
			mapSliceOf(item("key", "value"), item(nil, "empty key")),
			mapSliceOf(item("key", "value"), item(nil, "empty key")),
			mapSliceOf(item(nil, nil)),
			mapSliceOf(item(nil, nil)),
		}, mapSlicesOf(t, src))
	},

	// ":\n\n\n". The blank lines end the document and add nothing to it.
	"empty-lines-at-end-of-document": func(t *testing.T, src []byte) {
		assert.Equal(t, []codec.MapSlice{mapSliceOf(item(nil, nil))}, mapSlicesOf(t, src))
	},

	// Example 7.3, Completely Empty Flow Nodes. The specification composes it
	// to { "foo": null, null: "bar" }.
	"spec-example-7-3-completely-empty-flow-nodes": func(t *testing.T, src []byte) {
		assert.Equal(t, []codec.MapSlice{
			mapSliceOf(item("foo", nil), item(nil, "bar")),
		}, mapSlicesOf(t, src))
	},

	// Example 8.18, Implicit Block Mapping Entries. The specification composes
	// it to { "plain key": "in-line value", null: null, "quoted key": ["entry"] }.
	"spec-example-8-18-implicit-block-mapping-entries": func(t *testing.T, src []byte) {
		assert.Equal(t, []codec.MapSlice{mapSliceOf(
			item("plain key", "in-line value"),
			item(nil, nil),
			item("quoted key", []any{"entry"}),
		)}, mapSlicesOf(t, src))
	},

	// Example 8.19, Compact Block Mappings. The specification composes it to
	// [ { "sun": "yellow" }, { { "earth": "blue" }: { "moon": "white" } } ],
	// so the second entry is a mapping keyed on a mapping.
	"spec-example-8-19-compact-block-mappings": func(t *testing.T, src []byte) {
		// The second document keys a mapping on a mapping, which no Go
		// destination holds: a MapSlice took one until its keys had to be
		// comparable, and it was the last that did.
		var got []codec.MapSlice
		err := codec.NewDecoder(bytes.NewReader(src)).Decode(&got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a mapping cannot be a key in a Go map")
	},

	// "- :\n". One sequence entry holding a mapping, its key and its value both
	// empty.
	"syntax-character-edge-cases/00": func(t *testing.T, src []byte) {
		var got []codec.MapSlice
		require.NoError(t, codec.NewDecoder(bytes.NewReader(src)).Decode(&got))
		assert.Equal(t, []codec.MapSlice{mapSliceOf(item(nil, nil))}, got)
	},

	// "!\n", the non-specific tag on the empty node. It denotes null, and the
	// stream holds one document, not none: the decoder read it as no document
	// at all until it stopped folding every document into a value to decide
	// whether it held one. go.yaml.in/yaml/v3 hands back one document holding
	// nil as well.
	"syntax-character-edge-cases/02": func(t *testing.T, src []byte) {
		dec := codec.NewDecoder(bytes.NewReader(src))
		var got any
		require.NoError(t, dec.Decode(&got))
		assert.Nil(t, got)
		assert.ErrorIs(t, dec.Decode(&got), io.EOF, "the stream holds one document")
	},

	// "---\n?\n- a\n- b\n:\n- c\n- d\n". One entry, keyed on the sequence
	// [a, b], valued [c, d].
	"zero-indented-sequences-in-explicit-mapping-keys": func(t *testing.T, src []byte) {
		// Keyed on the sequence [a, b], which a Go map cannot hold and a
		// MapSlice no longer holds either.
		var got codec.MapSlice
		err := codec.NewDecoder(bytes.NewReader(src)).Decode(&got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a sequence cannot be a key in a Go map")
	},
}

// TestFixturesThatStateNoExpectation runs the pins against the vendored
// fixtures, so a pin fails when the fixture text changes under it as well as
// when the decoder does.
func TestFixturesThatStateNoExpectation(t *testing.T) {
	for name, pin := range noExpectationPins {
		t.Run(name, func(t *testing.T) {
			pin(t, fixture(t, name).InYAML)
		})
	}
}

// TestEveryFixtureStatingNoExpectationHasAPin holds noExpectationPins and
// decodeLedger to the same set of names, in both directions.
//
// A fixture filed under reasonNoExpectation and left unpinned is one nothing
// scores and nothing states, which is how a valid document can be lost without
// a figure moving. A pin naming a fixture that is no longer filed there has
// been overtaken: TestYAMLTestSuite scores that one now.
func TestEveryFixtureStatingNoExpectationHasAPin(t *testing.T) {
	for name, reason := range decodeLedger {
		if reason != reasonNoExpectation {
			continue
		}
		assert.Containsf(t, noExpectationPins, name,
			"%s states no expectation and nothing pins what it means", name)
	}

	for name := range noExpectationPins {
		assert.Equalf(t, reasonNoExpectation, decodeLedger[name],
			"%s is no longer filed under reasonNoExpectation, so this pin duplicates what TestYAMLTestSuite scores", name)
	}
}

// mapSlicesOf decodes every document of src into a codec.MapSlice.
func mapSlicesOf(t *testing.T, src []byte) []codec.MapSlice {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewReader(src))
	var documents []codec.MapSlice
	for {
		var ms codec.MapSlice
		err := dec.Decode(&ms)
		if errors.Is(err, io.EOF) {
			return documents
		}
		require.NoError(t, err)
		documents = append(documents, ms)
	}
}

// fixture returns the vendored YAML Test Suite case of that name.
func fixture(t *testing.T, name string) *yamltestsuite.TestSuite {
	t.Helper()

	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, test := range tests {
		if test.Name == name {
			return test
		}
	}
	t.Fatalf("%s: no such fixture", name)

	return nil
}
