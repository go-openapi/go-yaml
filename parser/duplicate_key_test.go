// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestDuplicateMapKeyIsReportedPerMapping checks that a key written twice in
// one mapping is refused, and that two mappings holding the same key are not.
//
// The keys of a mapping are compared against each other and against no others,
// so every shape a mapping comes in has to hold its own set: block and flow,
// nested one in the other, repeated down a sequence, and written with '?'.
func TestDuplicateMapKeyIsReportedPerMapping(t *testing.T) {
	tests := map[string]struct {
		src       string
		duplicate bool
	}{
		"block mapping repeats a key": {
			src:       "foo: 1\nfoo: 2\n",
			duplicate: true,
		},
		"block mapping repeats a key with entries between": {
			src:       "foo: 1\nbar: 2\nbaz: 3\nfoo: 4\n",
			duplicate: true,
		},
		"sibling mappings share a key": {
			src: "a:\n  foo: 1\nb:\n  foo: 2\n",
		},
		"nested mapping repeats its parent's key": {
			src: "foo:\n  foo: 1\n",
		},
		"nested mapping repeats its own key": {
			src:       "foo:\n  bar: 1\n  bar: 2\n",
			duplicate: true,
		},
		"entries of a sequence share a key": {
			src: "- foo: 1\n- foo: 2\n",
		},
		"one entry of a sequence repeats a key": {
			src:       "- foo: 1\n- foo: 2\n  foo: 3\n",
			duplicate: true,
		},
		"flow mapping repeats a key": {
			src:       "{foo: 1, foo: 2}\n",
			duplicate: true,
		},
		"flow mappings side by side share a key": {
			src: "[{foo: 1}, {foo: 2}]\n",
		},
		"flow mapping nested in a block mapping repeats a key": {
			src:       "a: {foo: 1, foo: 2}\n",
			duplicate: true,
		},
		"flow mapping repeats the key it hangs under": {
			src: "foo: {foo: 1}\n",
		},
		"explicit key repeats a plain one": {
			src:       "foo: 1\n? foo\n: 2\n",
			duplicate: true,
		},
		"explicit keys repeat each other": {
			src:       "? foo\n: 1\n? foo\n: 2\n",
			duplicate: true,
		},
		"quoted key repeats the plain spelling": {
			src:       "foo: 1\n\"foo\": 2\n",
			duplicate: true,
		},
		// A key holding a path character is quoted when its path is built. Two
		// such keys are still two keys.
		"keys holding path characters differ": {
			src: "a.b: 1\na[0]: 2\n$: 3\n",
		},
		"key holding a path character repeats": {
			src:       "a.b: 1\na.b: 2\n",
			duplicate: true,
		},
		"separate documents share a key": {
			src: "foo: 1\n---\nfoo: 2\n",
		},
		"merge keys repeat": {
			src:       "a: &a {x: 1}\nb:\n  <<: *a\n  <<: *a\n",
			duplicate: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// The parse reads the document and records the repeat rather than
			// refusing: a document that cannot be parsed cannot be linted or
			// rendered either, and 3.2.1.1 leaves what to do about a repeat to
			// whoever loads it. codec refuses it there.
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			found := duplicatesOf(f)
			if !test.duplicate {
				assert.Empty(t, found, "the parse recorded a repeat where the mapping holds none")

				return
			}
			assert.NotEmpty(t, found, "the parse recorded no repeat")
		})
	}
}

// duplicatesOf returns every repeated key the parse recorded in f.
func duplicatesOf(f *ast.File) []ast.DuplicateKey {
	var found []ast.DuplicateKey
	for _, doc := range f.Docs {
		ast.Walk(duplicateWalker{&found}, doc)
	}

	return found
}

type duplicateWalker struct{ out *[]ast.DuplicateKey }

func (w duplicateWalker) Visit(n ast.Node) ast.Visitor {
	if m, ok := n.(*ast.MappingNode); ok {
		*w.out = append(*w.out, m.Duplicates...)
	}

	return w
}

// TestDuplicateMapKeyIsFoundPastTheScanLimit checks the index a large mapping
// switches to. A mapping of a handful of keys compares them in a slice; the
// repeated key here sits beyond that point, and beyond it in both directions.
//
// The position the error reports is checked with it. The index keeps where a
// key was written rather than the node it was written on, so the line and
// column are the only thing left to get wrong.
func TestDuplicateMapKeyIsFoundPastTheScanLimit(t *testing.T) {
	const keys = 200

	build := func(repeat int) string {
		var b strings.Builder
		for i := range keys {
			fmt.Fprintf(&b, "key%02d: %d\n", i, i)
		}
		if repeat >= 0 {
			fmt.Fprintf(&b, "key%02d: again\n", repeat)
		}

		return b.String()
	}

	f, err := parser.ParseBytes([]byte(build(-1)))
	require.NoError(t, err)
	assert.Empty(t, duplicatesOf(f))

	for _, repeat := range []int{0, 3, 17, 100, keys - 1} {
		t.Run(fmt.Sprintf("repeats key%02d", repeat), func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(build(repeat)))
			require.NoError(t, err)

			found := duplicatesOf(f)
			require.Len(t, found, 1)
			assert.Equal(t, fmt.Sprintf("key%02d", repeat), found[0].Name)
			assert.Equal(t, repeat+1, int(found[0].FirstAt.Line), "where it was first written")
			assert.Equal(t, keys+1, int(found[0].At.Line), "where the repeat stands")
		})
	}
}

// TestDuplicateMapKeyAllowed checks that the option records nothing at all, so
// that tolerating a repeat costs no memory and the load has nothing to refuse.
func TestDuplicateMapKeyAllowed(t *testing.T) {
	f, err := parser.ParseBytes([]byte("foo: 1\nfoo: 2\n"), parser.WithAllowDuplicateMapKey())
	require.NoError(t, err)
	assert.Empty(t, duplicatesOf(f))
}

// TestDuplicateMapKeyIsPerType covers the half of 3.2.1.1 that says which keys
// are the same key.
//
// Two keys are equal when they resolve to the same node, so the type is half a
// key's identity and its canonical text the other half. Comparing the
// characters alone read "7" and "007" as two keys where they are one integer
// written twice, and "1" and "\"1\"" as one where they are a number and a
// string.
func TestDuplicateMapKeyIsPerType(t *testing.T) {
	for name, test := range map[string]struct {
		src       string
		duplicate bool
	}{
		// One node written two ways is one key.
		"an integer in two bases":     {src: "0x10: a\n16: b\n", duplicate: true},
		"an integer with a leading 0": {src: "7: a\n007: b\n", duplicate: true},
		"a null in two spellings":     {src: "~: a\nnull: b\n", duplicate: true},
		"a null written empty":        {src: ": a\nnull: b\n", duplicate: true},
		"a boolean in two cases":      {src: "true: a\nTrue: b\n", duplicate: true},
		"a float in two spellings":    {src: "1e3: a\n1000.0: b\n", duplicate: true},

		// Two nodes are two keys, however alike they read.
		"an integer and a float":       {src: "1: a\n1.0: b\n"},
		"a number and a string":        {src: "1: a\n\"1\": b\n"},
		"a number and a tagged string": {src: "1: a\n!!str 1: b\n"},
		"a boolean and a string":       {src: "true: a\n\"true\": b\n"},
		"a null and an empty string":   {src: "null: a\n\"\": b\n"},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			if test.duplicate {
				assert.NotEmpty(t, duplicatesOf(f))

				return
			}
			assert.Empty(t, duplicatesOf(f))
		})
	}
}

// TestDuplicateMapKeyUnderATagFollowsTheResolvedName checks that a key under a
// tagged mapping is compared by what it resolves to, in both of the ways a
// mapping records its keys.
//
// Three pieces of work meet here and none of them has a test for the
// combination. The scanner types a scalar by the tag's URI rather than by the
// characters the tag was written with, so "!!map", "!<tag:yaml.org,2002:map>"
// and a local "!foo" all leave the keys under them to resolve. key.Set scans a
// mapping's keys up to its spill threshold of 64 and builds a hash index past
// it. And the key
// rules say two keys are equal when they resolve to the same node, so "False"
// and "false" are one key and "1" and "\"1\"" are two.
//
// The index inherits the resolution for free because the set's key filter
// reads the kind
// from token.KeyName, the way the map it replaced did -- the optimization made
// the lookup cheaper without hardcoding any typing. That is worth a test rather
// than a comment: a later index that compared the written characters would pass
// every other test in this file, since none of them reaches the spill with a key
// whose name is not its text.
func TestDuplicateMapKeyUnderATagFollowsTheResolvedName(t *testing.T) {
	// wide writes a tagged mapping of n ordinary keys, with first written above
	// them and last below, so the pair straddles the spill into the index.
	wide := func(first, last string, n int) string {
		var b strings.Builder

		b.WriteString("!foo\n" + first)
		for i := range n {
			fmt.Fprintf(&b, "k%d: %d\n", i, i)
		}
		b.WriteString(last)

		return b.String()
	}

	for name, test := range map[string]struct {
		src       string
		duplicate bool
	}{
		// The tag does not stop the keys under it resolving, in any spelling.
		"a local tag":    {src: "!foo\nFalse: 1\nfalse: 2\n", duplicate: true},
		"a shorthand":    {src: "!!map\nFalse: 1\nfalse: 2\n", duplicate: true},
		"a verbatim tag": {src: "!<tag:yaml.org,2002:map>\nFalse: 1\nfalse: 2\n", duplicate: true},
		"two bases":      {src: "!foo\n0x10: a\n16: b\n", duplicate: true},

		// And it does not merge keys that are two nodes.
		"two booleans":         {src: "!foo\nFalse: 1\nTrue: 2\n"},
		"two integers":         {src: "!foo\n1: a\n2: b\n"},
		"a number and a quote": {src: "!foo\n1: a\n\"1\": b\n"},

		// Past the spill, where the index answers instead of the scan.
		"a repeat past the spill":          {src: wide("k0: first\n", "k0: again\n", 80), duplicate: true},
		"no repeat past the spill":         {src: wide("", "", 80)},
		"a resolved repeat past the spill": {src: wide("False: 1\n", "false: 2\n", 80), duplicate: true},
		"two nodes alike past the spill":   {src: wide("1: a\n", "\"1\": b\n", 80)},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			if test.duplicate {
				assert.NotEmpty(t, duplicatesOf(f))

				return
			}
			assert.Empty(t, duplicatesOf(f))
		})
	}
}

// TestWideMappingsAreToldApart checks that two wide mappings hold their own
// keys, which is what the index a wide mapping spills into has to get right.
//
// TestDuplicateMapKeyIsFoundPastTheScanLimit covers a wide mapping on its own,
// and every mapping in it starts its keys at the bottom of the key stack. These
// do not: a nested one starts partway up, and two siblings start at the same
// place one after the other, so a key the first left behind would be read as a
// repeat in the second.
func TestWideMappingsAreToldApart(t *testing.T) {
	const wide = 200

	entries := func(indent string) string {
		var b strings.Builder
		for i := range wide {
			fmt.Fprintf(&b, "%skey%03d: %d\n", indent, i, i)
		}

		return b.String()
	}

	tests := map[string]struct {
		src       string
		duplicate bool
	}{
		"a wide mapping nested under a key": {
			src: "outer:\n" + entries("  "),
		},
		"a wide mapping nested under a key repeats one of its own": {
			src:       "outer:\n" + entries("  ") + "  key003: again\n",
			duplicate: true,
		},
		"sibling wide mappings hold the same keys": {
			src: "a:\n" + entries("  ") + "b:\n" + entries("  "),
		},
		"wide mappings down a sequence hold the same keys": {
			src: "- " + strings.TrimPrefix(entries("  "), "  ") + "- " + strings.TrimPrefix(entries("  "), "  "),
		},
		"a wide mapping repeats the key it hangs under": {
			src: "key003:\n" + entries("  "),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			found := duplicatesOf(f)
			if !test.duplicate {
				assert.Empty(t, found, "the parse recorded a repeat where the mappings hold none")

				return
			}
			assert.NotEmpty(t, found, "the parse missed a repeat")
		})
	}
}
