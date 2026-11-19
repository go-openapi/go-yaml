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

// TestDuplicateMapKeyIsReportedPerMapping checks that the parse records a key written twice in one mapping,
// and records nothing for two mappings holding the same key.
//
// A mapping's keys are compared with each other and with no others.
// The cases cover each shape a mapping comes in: block and flow, one nested in the other,
// repeated down a sequence, and written with '?'.
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
			// The parse records a repeat and returns no error:
			// section 3.2.1.1 leaves the repeat to the loader, and codec rejects it there.
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

// yamlRepeatsOf returns the keys of f that repeat as 3.2.1.1 says, leaving out two keys only a name makes one.
func yamlRepeatsOf(f *ast.File) []ast.DuplicateKey {
	var found []ast.DuplicateKey
	for _, d := range duplicatesOf(f) {
		if d.Repeats(false) {
			found = append(found, d)
		}
	}

	return found
}

// TestARepeatByNameIsRecordedBesideTheYAMLRepeat checks the record of two keys that YAML tells apart and a name
// does not, below the spill and past it: the parse records them as NameOnly, and a key repeated after them is still
// found as the repeat it is. "{1: a, \"1\": b, \"1\": c}" repeats the string "1", which the scan used to compare
// against the integer and stop at.
func TestARepeatByNameIsRecordedBesideTheYAMLRepeat(t *testing.T) {
	wide := func(body string) string {
		var b strings.Builder
		for i := range 80 {
			fmt.Fprintf(&b, "k%d: %d\n", i, i)
		}
		b.WriteString(body)

		return b.String()
	}
	type record struct {
		name     string
		nameOnly bool
	}
	for name, src := range map[string]string{
		"in the scan":    "1: a\n\"1\": b\n\"1\": c\n",
		"past the spill": wide("1: a\n\"1\": b\n\"1\": c\n"),
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)

			var got []record
			for _, d := range duplicatesOf(f) {
				got = append(got, record{d.Name, d.NameOnly})
				assert.False(t, d.JSONNameOnly, "only WithJSONCompatible marks a repeat for JSON")
			}
			assert.Equal(t, []record{{"1", true}, {"1", false}}, got)
		})
	}

	t.Run("under WithJSONCompatible the name-only repeat is marked for JSON", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("1: a\n\"1\": b\n"), parser.WithJSONCompatible())
		require.NoError(t, err)
		found := duplicatesOf(f)
		require.Len(t, found, 1)
		assert.True(t, found[0].NameOnly)
		assert.True(t, found[0].JSONNameOnly)
	})
}

type duplicateWalker struct{ out *[]ast.DuplicateKey }

func (w duplicateWalker) Visit(n ast.Node) ast.Visitor {
	if m, ok := n.(*ast.MappingNode); ok {
		*w.out = append(*w.out, m.Duplicates...)
	}

	return w
}

// TestDuplicateMapKeyIsFoundPastTheScanLimit checks the index a large mapping switches to.
//
// A mapping of a few keys compares them in a slice.
// Here the first writing of the repeated key falls on either side of the point where key.Set spills into its index,
// and the repeat past it.
//
// The recorded lines are checked too: the index keeps the position a key was written at, not its node.
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

// TestDuplicateMapKeyAllowed checks that the parse records a repeat under [parser.WithAllowDuplicateMapKey]
// all the same, marked [ast.DuplicateKey.Allowed], so every load can tell which entry repeats and keep one.
func TestDuplicateMapKeyAllowed(t *testing.T) {
	const src = "foo: 1\nfoo: 2\n"

	f, err := parser.ParseBytes([]byte(src), parser.WithAllowDuplicateMapKey())
	require.NoError(t, err)
	found := duplicatesOf(f)
	require.Len(t, found, 1)
	assert.True(t, found[0].Allowed, "a repeat the parse was told to allow is marked so")
	assert.Equal(t, 2, int(found[0].At.Line))

	f, err = parser.ParseBytes([]byte(src))
	require.NoError(t, err)
	found = duplicatesOf(f)
	require.Len(t, found, 1)
	assert.False(t, found[0].Allowed, "without the option a repeat is not allowed")
}

// TestOrderedMapRepeatIsRecordedOnTheSequence checks where the parse records a key two entries of an "!!omap" write.
//
// Each entry is a mapping of its own, so the repeat is recorded on the sequence the tag stands on,
// with the index of the entry that repeats the key, and on no mapping.
// An entry written as an alias opens no mapping, and is named from the mapping its anchor named.
func TestOrderedMapRepeatIsRecordedOnTheSequence(t *testing.T) {
	type repeat struct {
		name  string
		index int
	}

	for src, want := range map[string][]repeat{
		"!!omap [{a: 1}, {b: 2}, {a: 3}]\n":         {{"a", 2}},
		"k: !!omap\n- a: 1\n- b: 2\n- a: 3\n":       {{"a", 2}},
		"k: !!omap [{1: a}, {0x1: b}, {1: c}]\n":    {{"1", 1}, {"1", 2}},
		"m: &m {x: 2}\nk: !!omap [{x: 1}, *m]\n":    {{"x", 1}},
		"m: &m {x: 2}\nk: !!omap [*m, {x: 1}]\n":    {{"x", 1}},
		"k: !!omap &o [{a: 1}, {a: 2}]\n":           {{"a", 1}},
		"k: &o !!omap [{a: 1}, {a: 2}]\n":           {{"a", 1}},
		"k: !!omap [{a: !!omap [{a: 1}]}, {a: 2}]":  {{"a", 1}},
		"k: !!omap [{a: [{a: 1}]}, {b: 2}]\n":       nil,
		"k: !!omap {a: [{a: 1}, {a: 2}]}\n":         nil,
		"k: !!omap [[{a: 1}, {a: 2}]]\n":            nil,
		"k: [{a: 1}, {a: 2}]\n":                     nil,
		"k: !!omap [{a: 1}]\nl: [{a: 1}, {a: 1}]\n": nil,
		"k: !!omap\nl: [{a: 1}, {a: 1}]\n":          nil,
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)
			assert.Empty(t, duplicatesOf(f), "a repeat across entries is recorded on no mapping")

			var got []repeat
			for _, d := range sequenceDuplicatesOf(f) {
				got = append(got, repeat{d.Name, d.Index})
			}
			assert.Equal(t, want, got)
		})
	}

	t.Run("under WithAllowDuplicateMapKey the repeat is marked allowed", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("k: !!omap [{a: 1}, {a: 2}]\n"), parser.WithAllowDuplicateMapKey())
		require.NoError(t, err)
		found := sequenceDuplicatesOf(f)
		require.Len(t, found, 1)
		assert.True(t, found[0].Allowed)
	})
}

// TestAMergeInAnOrderedMapEntryRecordsWhatItBringsIn checks the key the parse records for an "!!omap" entry holding a
// "<<" under YAML 1.1: the keys the merge brings in, through a mapping, an alias or a sequence of them, and not the
// "<<". A key one entry writes twice -- its own and a merged one, or two mappings of the merge -- is no repeat.
func TestAMergeInAnOrderedMapEntryRecordsWhatItBringsIn(t *testing.T) {
	type repeat struct {
		name  string
		index int
	}

	for src, want := range map[string][]repeat{
		"k: !!omap [{<<: {a: 1}}, {<<: {b: 2}}]\n":                     nil,
		"k: !!omap\n- <<: {a: 1}\n- <<: {b: 2}\n":                      nil,
		"k: !!omap [{<<: {a: 1}, a: 2}]\n":                             nil,
		"k: !!omap [{<<: [{a: 1}, {a: 2}]}]\n":                         nil,
		"k: !!omap [{a: 1}, {<<: {a: 2}}]\n":                           {{"a", 1}},
		"k: !!omap [{<<: {a: 1}}, {a: 2}]\n":                           {{"a", 1}},
		"k: !!omap\n- a: 1\n- <<: {a: 2}\n":                            {{"a", 1}},
		"m: &m {a: 2}\nk: !!omap [{a: 1}, {<<: *m}]\n":                 {{"a", 1}},
		"m: &m {a: 2}\nk: !!omap [{a: 1}, {<<: [{a: 3}, *m]}]\n":       {{"a", 1}},
		"m: &m {a: 2}\nk: !!omap\n- a: 1\n- <<:\n  - {b: 3}\n  - *m\n": {{"a", 1}},
		"m: &m {<<: {a: 2}}\nk: !!omap [{b: 1}, *m]\n":                 nil,
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src), parser.WithYAMLVersion(parser.YAML11))
			require.NoError(t, err)

			var got []repeat
			for _, d := range sequenceDuplicatesOf(f) {
				got = append(got, repeat{d.Name, d.Index})
			}
			assert.Equal(t, want, got)
		})
	}
}

// sequenceDuplicatesOf returns every key the parse recorded as repeated across the entries of an "!!omap" in f.
func sequenceDuplicatesOf(f *ast.File) []ast.DuplicateKey {
	var found []ast.DuplicateKey
	for _, doc := range f.Docs {
		ast.Walk(sequenceDuplicateWalker{&found}, doc)
	}

	return found
}

type sequenceDuplicateWalker struct{ out *[]ast.DuplicateKey }

func (w sequenceDuplicateWalker) Visit(n ast.Node) ast.Visitor {
	if s, ok := n.(*ast.SequenceNode); ok {
		*w.out = append(*w.out, s.Duplicates...)
	}

	return w
}

// TestDuplicateMapKeyIsPerType checks that two keys repeat when they resolve to the same node,
// as section 3.2.1.1 defines.
//
// A key's identity is its type and its canonical text,
// so "7" and "007" are one integer written twice, and "1" and "\"1\"" are a number and a string.
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
				assert.NotEmpty(t, yamlRepeatsOf(f))

				return
			}
			assert.Empty(t, yamlRepeatsOf(f))
		})
	}
}

// TestDuplicateMapKeyUnderATagFollowsTheResolvedName checks that a key under a tagged mapping
// is compared by what it resolves to, both below and past the point where key.Set spills into its index.
//
// The scanner types a scalar by the tag's URI, so "!!map", "!<tag:yaml.org,2002:map>" and a local "!foo"
// all leave the keys under them to resolve.
// key.Set scans a mapping's keys up to its spill threshold of 64, and builds a hash index past it.
// Two keys are equal when they resolve to the same node, so "False" and "false" are one key and "1" and "\"1\"" two.
//
// The index reads each key's kind from token.KeyName, as the scan does.
// An index that compared the written characters would pass every other test in this file,
// since none of them reaches the spill with a key whose name differs from its text.
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

		// Past the spill, the index finds the repeat instead of the scan.
		"a repeat past the spill":          {src: wide("k0: first\n", "k0: again\n", 80), duplicate: true},
		"no repeat past the spill":         {src: wide("", "", 80)},
		"a resolved repeat past the spill": {src: wide("False: 1\n", "false: 2\n", 80), duplicate: true},
		"two nodes alike past the spill":   {src: wide("1: a\n", "\"1\": b\n", 80)},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			if test.duplicate {
				assert.NotEmpty(t, yamlRepeatsOf(f))

				return
			}
			assert.Empty(t, yamlRepeatsOf(f))
		})
	}
}

// TestWideMappingsAreToldApart checks that two wide mappings each hold their own keys in key.Set's index.
//
// In TestDuplicateMapKeyIsFoundPastTheScanLimit every mapping starts its keys at the bottom of the key stack.
// Here a nested mapping starts partway up, and two siblings start at the same place one after the other,
// so a key the first left behind would read as a repeat in the second.
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
