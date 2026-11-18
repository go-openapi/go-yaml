// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// rendererCeiling is how many suite documents may fail to survive a round trip
// through ast.Renderer.
//
// A ceiling on failures rather than a floor on successes: the two differ
// whenever acceptance changes, and tightening the parser legitimately shrinks
// the set of documents this measures at all. Counting what is broken keeps the
// number about rendering.
//
// A count rather than a ledger: the per-case ledger lives in roundtrip_test.go,
// which measures the same thing through String. This one measures the renderer
// directly, so that a caller driving it themselves -- with their own indent, or
// with comments off -- is covered by more than the default path. It is at zero,
// and is not allowed to rise.
const rendererCeiling = 0

// TestRendererRoundTrip measures ast.Renderer the way roundtrip_test.go
// measures rendering through String, so the two numbers mean the same thing.
func TestRendererRoundTrip(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)

	renderer := ast.NewRenderer()

	var accepted, stable, unreadable, drifting int
	for _, test := range tests {
		file, err := parser.ParseBytes(test.InYAML, parser.WithComments())
		if err != nil {
			continue
		}
		accepted++

		rendered := renderer.File(file)

		reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
		switch {
		case err != nil:
			unreadable++
		case renderer.File(reread) != rendered:
			drifting++
		default:
			stable++
		}
	}

	t.Logf("ast.Renderer: %d accepted, %d survive (%.1f%%), %d do not parse when read back, %d drift",
		accepted, stable, 100*float64(stable)/float64(accepted), unreadable, drifting)

	assert.LessOrEqualf(t, unreadable+drifting, rendererCeiling,
		"the renderer lost ground: %d documents do not survive, ceiling is %d",
		unreadable+drifting, rendererCeiling)
}

// TestRendererReachesAFixedPoint pins the property the redesign exists for.
//
// Rendering is idempotent by construction: laying a tree out by its depth
// cannot depend on where the text ended up last time, so a second render has
// nothing left to change. These are the shapes that grew without bound before.
func TestRendererReachesAFixedPoint(t *testing.T) {
	sources := map[string]string{
		"tagged mapping in a sequence":    "- !!map\n  a: 1\n",
		"tagged mapping nested deeper":    "a:\n  - !!map\n    b: 1\n",
		"flow mapping in a flow sequence": "\"k\" : [\n \"j\" : v,\n ]\n",
		"tagged collections side by side": "foo: !!seq\n  - !!str a\n  - !!map\n    key: !!str value\n",
		"plain nesting":                   "a:\n  b:\n    c: 1\n",
		"sequence of mappings":            "- a: 1\n  b: 2\n- c: 3\n",
	}

	renderer := ast.NewRenderer()

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)

			first := renderer.File(file)

			reread, err := parser.ParseBytes([]byte(first))
			require.NoError(t, err)

			assert.Equal(t, first, renderer.File(reread), "a second render changed the document")
		})
	}
}

// TestRendererIndentWidth covers the option that falls out of laying documents
// out by depth, which upstream #614 asks for.
func TestRendererIndentWidth(t *testing.T) {
	file, err := parser.ParseBytes([]byte("a:\n  b:\n    c: 1\n"))
	require.NoError(t, err)

	assert.Equal(t, "a:\n  b:\n    c: 1\n", ast.NewRenderer().File(file))
	assert.Equal(t, "a:\n    b:\n        c: 1\n", ast.NewRenderer(ast.WithIndent(4)).File(file))
	assert.Equal(t, "a:\n b:\n  c: 1\n", ast.NewRenderer(ast.WithIndent(1)).File(file))
}

// TestRendererSequenceIndentation pins the layout choice a block sequence under
// a mapping key gets. Both are legal YAML; not indenting is what this library
// has always emitted, and yaml.IndentSequence is the option that asks for the
// other one.
func TestRendererSequenceIndentation(t *testing.T) {
	file, err := parser.ParseBytes([]byte("tags:\n- a\n- b\n"))
	require.NoError(t, err)

	assert.Equal(t, "tags:\n- a\n- b\n", ast.NewRenderer().File(file))
	assert.Equal(t, "tags:\n  - a\n  - b\n", ast.NewRenderer(ast.WithIndentSequence(true)).File(file))
}

// TestRendererBlockScalarIndentation covers the one scalar that spans lines.
//
// Its content is indented from the start of the line its header shares, not
// from where the header sits, so nesting it deeper does not compound: the
// second case is the first one level in, and its content moves by exactly that
// one level.
func TestRendererBlockScalarIndentation(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"under a key":          {"a: |\n  x\n  y\n", "a: |\n  x\n  y\n"},
		"one level deeper":     {"a:\n  b: |\n    x\n    y\n", "a:\n  b: |\n    x\n    y\n"},
		"as a sequence entry":  {"- |\n  x\n  y\n", "- |\n  x\n  y\n"},
		"beside a sibling key": {"- a: |\n    x\n  b: 1\n", "- a: |\n    x\n  b: 1\n"},
		"with a stated width":  {"a: |2\n   x\n", "a: |2\n   x\n"},
		"with a chomping mark": {"a: |-\n  x\n", "a: |-\n  x\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source))
			require.NoError(t, err)

			assert.Equal(t, test.want, ast.NewRenderer().File(file))
		})
	}
}

// TestRendererKeepsAuthoredBlankLines pins what rendering carries over from the
// source besides the values themselves.
//
// A blank line between entries is the author's grouping, and unlike a column it
// does not compound: it says "there was a gap here", which stays true however
// many times the document is read and written.
func TestRendererKeepsAuthoredBlankLines(t *testing.T) {
	tests := map[string]string{
		"between mapping entries":  "a: 1\n\nb: 2\n",
		"between nested entries":   "a:\n  b: 1\n\n  c: 2\n",
		"above a head comment":     "a:\n- b: 1\n\n# c\n- c: 2\n",
		"between mapping items":    "- a: 1\n\n- b: 2\n",
		"between scalar items":     "- a\n\n- b\n",
		"between flow items":       "- [a]\n\n- [b]\n",
		"between multi-line items": "- a: 1\n  b: 2\n\n- c: 3\n",
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoError(t, err)

			assert.Equal(t, source, ast.NewRenderer().File(file))
		})
	}

	// And only where the author left one. A scalar written across two lines
	// occupies them; the second is not a gap, and reading it as one put a stray
	// blank line after every such entry.
	t.Run("not for a value spanning lines", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte("a: 'x\n  y'\nb: 2\n"), parser.WithComments())
		require.NoError(t, err)

		assert.NotContains(t, ast.NewRenderer().File(file), "\n\n")
	})

	// An entry whose '-' sits on a line of its own occupies both lines. The
	// second is the entry's layout, not a gap above the entry after it.
	t.Run("not for an entry written under its dash", func(t *testing.T) {
		for _, source := range []string{"-\n  a\n-\n  b\n", "-\n  a: 1\n-\n  ? b\n"} {
			file, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoError(t, err)

			assert.NotContainsf(t, ast.NewRenderer().File(file), "\n\n",
				"invented a blank line rendering %q", source)
		}
	})
}

// TestRendererFlowCollectionsWithComments covers the one thing that stops a
// flow collection being written on a single line.
//
// A comment cannot go on that line: everything after it is commented out,
// including the bracket that closes the collection, which is how "{a: 1, b: 2}"
// with a note on it used to come back as text that no longer parses. Several
// lines is the only layout that holds both, and it is still a flow collection.
func TestRendererFlowCollectionsWithComments(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"comment in a flow sequence": {
			source: "[a,\n# note\nb]\n",
			want:   "[\n  a,\n  # note\n  b\n]\n",
		},
		"comment in a flow mapping": {
			source: "{a: 1,\n# note\nb: 2}\n",
			want:   "{\n  a: 1,\n  # note\n  b: 2\n}\n",
		},
		"comment before the closing bracket": {
			source: "[a, b\n# note\n]\n",
			want:   "[\n  a,\n  b\n  # note\n]\n",
		},
		// Under a key the whole collection moves below it. Left on the key's
		// line, the lines under it carry no indentation of their own and the
		// closing bracket lands in column one, outside the mapping it belongs
		// to -- a document that no longer reads back.
		"comment in a flow mapping under a key": {
			source: "foo: {a: 1,\n  # note\n  b: 2}\n",
			want:   "foo:\n  {\n    a: 1,\n    # note\n    b: 2\n  }\n",
		},
		"comment in a flow sequence in an entry": {
			source: "- {a: 1,\n  # note\n  b: 2}\n",
			want:   "- {\n    a: 1,\n    # note\n    b: 2\n  }\n",
		},

		// Without a comment there is nothing to make room for, and the
		// collection stays where it was written.
		"no comment": {
			source: "[a, b]\n",
			want:   "[a, b]\n",
		},
		"no comment under a key": {
			source: "foo: {a: 1, b: 2}\n",
			want:   "foo: {a: 1, b: 2}\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)

			assert.Equal(t, test.want, ast.NewRenderer().File(file))

			// With comments off there is nothing to make room for either.
			assert.NotContains(t, ast.NewRenderer(ast.WithComments(false)).File(file), "\n  ")
		})
	}
}

// TestRendererPlacesKeyComments covers where a comment written on a key's line
// ends up. Before the ':' it would be read back as part of the key, so it goes
// after it -- and a collection that would have shared the line moves down to
// make room.
func TestRendererPlacesKeyComments(t *testing.T) {
	tests := map[string]string{
		"on a key with a block value":  "a: # c\n  b: 1\n",
		"on a key with a flow value":   "a: # c\n  {b: 1}\n",
		"on a key with a scalar value": "a: 1 # c\n",
		"on a key with a sequence":     "a: # c\n- b\n",
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoError(t, err)

			assert.Equal(t, source, ast.NewRenderer().File(file))
		})
	}
}
