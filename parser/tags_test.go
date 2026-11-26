// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseTagsOnEmptyScalars covers a scalar tag with nothing after it.
//
// Such a tag marks the empty node, as an anchor with nothing after it names it.
// What follows the tag is the next entry of the enclosing collection, not the tag's value.
func TestParseTagsOnEmptyScalars(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"as a mapping value at the end of the document": {
			source: "a: !!str\n",
			want:   "a: !!str\n",
		},
		"as a mapping value with a sibling below": {
			source: "b: !!str\nc: 1\n",
			want:   "b: !!str\nc: 1\n",
		},
		"as a sequence entry at the end of the document": {
			source: "- !!str\n",
			want:   "- !!str\n",
		},
		"as a sequence entry with a sibling below": {
			source: "- !!str\n- a\n",
			want:   "- !!str\n- a\n",
		},
		"as an explicit key": {
			source: "? !!str\n",
			want:   "? !!str\n:\n",
		},
		"as a value in a flow mapping": {
			source: "{a: !!str, b: 1}\n",
			want:   "{a: !!str, b: 1}\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			// It settles: the rendered text parses and renders to the same text.
			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseEmptyKeysCarryingProperties covers a key that is nothing but an anchor, an alias or a tag,
// in the positions such a key may appear in.
//
// The key is cut into tokens before the ':' is reached.
// The test catches a scanner left with no column to measure the following lines against,
// which reads the line below the entry as a continuation of its value instead of the next entry.
func TestParseEmptyKeysCarryingProperties(t *testing.T) {
	sources := map[string]string{
		"anchored, at the top level":       "&a : a\n",
		"tagged, at the top level":         "!!null : a\n",
		"local tag, at the top level":      "!t : a\nb: 1\n",
		"non-specific tag, at the top":     "! : a\nb: 1\n",
		"local tag, in a flow mapping":     "{!t : 1, b: 2}\n",
		"non-specific tag, in a flow map":  "{! : 1, b: 2}\n",
		"non-specific tag, in a flow seq":  "[! : 1, b]\n",
		"tagged key and tagged value":      "!!str : !!null\n",
		"anchored, nested under a key":     "x:\n  &a : a\n  b: 1\n",
		"tagged, nested under a key":       "x:\n  !!null : a\n  b: 1\n",
		"anchored, in a sequence entry":    "-\n  &a : a\n  b: 1\n",
		"tagged, in a sequence entry":      "-\n  !!null : a\n  b: 1\n",
		"quoted, nested under a key":       "x:\n  \"q\" : a\n  b: 1\n",
		"aliased, nested under a key":      "k: &r v\nx:\n  *r : a\n  b: 1\n",
		"anchored, in a flow mapping":      "{&a : 1, b: 2}\n",
		"anchored, as an explicit key":     "x:\n  ? &a\n  : a\n  b: 1\n",
		"anchored, with a sibling further": "x:\n  &a : a\n  b: 1\n  c: 2\n",
	}

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoErrorf(t, err, "rejected %q", source)

			// The entry below the key is a sibling of it, not part of its value.
			assert.NotContainsf(t, file.String(), "a b",
				"%q read the next line as a continuation", source)
		})
	}
}

// TestATagLedKeyBelowAnEmptyNestedValueOpensAnOuterEntry holds a key led by a tag, written further out than the key
// above it, where that key has no value: the key opens the next entry of the enclosing mapping.
//
// The value check read the key group by its first token, the tag, and refused it as a tag standing below a key with
// "tag is not indented past its key". An untagged key and an anchor-led one read fine in the same place.
// yaml/yaml-reference-parser, libfyaml and go.yaml.in/yaml/v3 read each document below.
func TestATagLedKeyBelowAnEmptyNestedValueOpensAnOuterEntry(t *testing.T) {
	for source, want := range map[string]any{
		"a:\n b:\n!!null : 1\n":       map[any]any{"a": map[string]any{"b": nil}, nil: uint64(1)},
		"a:\n b:\n!!null &x : 1\n":    map[any]any{"a": map[string]any{"b": nil}, nil: uint64(1)},
		"a:\n b:\n!!str : 1\n":        map[string]any{"a": map[string]any{"b": nil}, "": uint64(1)},
		"a:\n  b:\n!t : 1\n":          map[string]any{"a": map[string]any{"b": nil}, "": uint64(1)},
		"a:\n b:\n  c:\n!t : 1\n":     map[string]any{"a": map[string]any{"b": map[string]any{"c": nil}}, "": uint64(1)},
		"- a:\n   b:\n  !t : 1\n":     []any{map[string]any{"a": map[string]any{"b": nil}, "": uint64(1)}},
		"a:\n b:\n!!null : 1\nc: 2\n": map[any]any{"a": map[string]any{"b": nil}, nil: uint64(1), "c": uint64(2)},
	} {
		t.Run(source, func(t *testing.T) {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(source), &got))
			assert.Equal(t, want, got)
		})
	}

	t.Run("a tag alone below the key is still refused", func(t *testing.T) {
		for _, source := range []string{"a:\n b:\n!t\n", "a:\n!!str x\n"} {
			_, err := parser.ParseBytes([]byte(source))
			require.Errorf(t, err, "%q", source)
			assert.Contains(t, err.Error(), "tag is not indented past its key")
		}
	})
}

// TestRenderPropertyKeysKeepTheirSeparator covers the space between such a key and its ':'.
//
// ':' is a legal character in an anchor name, an alias name and a tag,
// so a key that ends on one absorbs a ':' written straight after it.
// "&a: v" anchors the name "a:" over the scalar v, where "&a : v" anchors the empty key of a mapping.
// Dropping the space changes the document's meaning.
func TestRenderPropertyKeysKeepTheirSeparator(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"anchor on the empty key":           {source: "&a : v\n", want: "&a : v\n"},
		"tag on the empty key":              {source: "!!str : v\n", want: "!!str : v\n"},
		"local tag on the empty key":        {source: "!t : v\n", want: "!t : v\n"},
		"non-specific tag on the empty key": {source: "! : v\n", want: "! : v\n"},
		"in a flow mapping":                 {source: "{! : v, !t : w}\n", want: "{! : v, !t : w}\n"},
		"alias as the key":                  {source: "k: &r v\n*r : a\n", want: "k: &r v\n*r : a\n"},
		"anchor and empty value":            {source: "-\n  &c : &a\n", want: "- &c : &a\n"},

		// A scalar after the property ends the key, and then the ':' is its own.
		"anchor on a named key": {source: "&a k : v\n", want: "&a k: v\n"},
		"tag on a named key":    {source: "!!str k : v\n", want: "!!str k: v\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseTagOnTheEmptyNodeInAFlowCollection covers a tag standing against the punctuation of its collection.
//
// ns-flow-node admits c-ns-properties followed by e-scalar,
// so "!" with a ']' after it is the non-specific tag on the empty node.
// The test catches a tag grouped with the ']' after it, which runs the sequence to the end of the stream,
// and a ']' or a '}' read into the tag's name.
//
// The null a tag stands on when nothing follows it is implicit,
// so the renderer writes nothing for it and the document comes back as it went in.
// Written out as "null", it would read back as the string "null",
// because a tag that resolves to nothing leaves its scalar as text.
func TestParseTagOnTheEmptyNodeInAFlowCollection(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"the non-specific tag alone in a sequence": {source: "[!]\n", want: "[!]\n"},
		"before an entry":                          {source: "[!, a]\n", want: "[!, a]\n"},
		"after an entry":                           {source: "[a, !]\n", want: "[a, !]\n"},
		"a local tag":                              {source: "[!str]\n", want: "[!str]\n"},
		"a resolved tag takes its own default":     {source: "[!!str]\n", want: "[!!str]\n"},
		"as the value of a flow mapping entry":     {source: "{a: !}\n", want: "{a: !}\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestDecodeTagOnTheEmptyNodeInAFlowCollection holds the decoded values of an empty node under a tag.
//
// An empty node is a plain scalar with no characters (7.2), and only the core schema resolves it to null, for a node
// with no tag. "!" makes a scalar a string (6.9.1, 10.1.2), a local tag leaves its scalar the text it holds, and
// "!!str" names the string: all three give the empty string. "!!int" gives its own default.
func TestDecodeTagOnTheEmptyNodeInAFlowCollection(t *testing.T) {
	tests := map[string]struct {
		source string
		want   any
	}{
		"the null tag":         {source: "[!!null]\n", want: []any{nil}},
		"the non-specific tag": {source: "[!]\n", want: []any{""}},
		"a local tag":          {source: "[!str]\n", want: []any{""}},
		"a verbatim tag":       {source: "[!<tag:example.com,2000:x>]\n", want: []any{""}},
		"the string tag":       {source: "[!!str]\n", want: []any{""}},
		"the integer tag":      {source: "[!!int]\n", want: []any{uint64(0)}},
		"mixed with entries":   {source: "[a, !, b]\n", want: []any{"a", "", "b"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &got))
			assert.Equal(t, test.want, got)
		})
	}
}

// TestParseRefusesATagTheGrammarRefuses covers the tag shapes YAML 1.2 has no production for.
//
// The test catches a scanner that reads a tag through to whatever ends it without checking its characters,
// so that "!<> x" decodes to "x" with no error. The productions are:
//
//	c-verbatim-tag  ::= "!" "<" ns-uri-char+ ">"
//	ns-uri-char     ::= "%" ns-hex-digit ns-hex-digit | ns-word-char | ...
//	ns-tag-char     ::= ns-uri-char - "!" - c-flow-indicator
//
// A verbatim tag takes at least one URI character, a "%" stands only in front of two hexadecimal digits,
// and neither "<" nor ">" is a character any tag may hold.
func TestParseRefusesATagTheGrammarRefuses(t *testing.T) {
	for source, want := range map[string]string{
		"k: !<> x\n":    "must name a URI",
		"k: !<%> x\n":   "two hexadecimal digits",
		"k: !<%4> x\n":  "two hexadecimal digits",
		"k: !<%zz> x\n": "two hexadecimal digits",
		"k: !<a x\n":    "must end with '>'",
		"k: !!<x> y\n":  "a tag cannot hold",
		"k: !!\n":       "must name a suffix",
		"k: !a!\n":      "must name a suffix",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			require.Error(t, err)
			assert.Contains(t, err.Error(), want)
		})
	}
}

// TestParseKeepsEveryTagTheGrammarAdmits holds the other side: every tag shape the productions admit parses.
//
// A local tag matters most: CloudFormation writes "!Ref" with one, and GitLab CI writes "!reference".
func TestParseKeepsEveryTagTheGrammarAdmits(t *testing.T) {
	for _, source := range []string{
		"k: ! x\n",                               // the non-specific tag
		"k: !\n",                                 // and on the empty node
		"k: !!str x\n",                           // the secondary handle
		"k: !fred x\n",                           // a local tag
		"k: !Ref x\n",                            // as CloudFormation writes one
		"k: !foo/bar x\n",                        // "/" is a URI character
		"k: !x'y z\n",                            // so is "'"
		"k: !<%41> x\n",                          // a percent escape that is one
		"k: !<tag:a,2000:b> x\n",                 // "," and ":" inside a verbatim tag
		"k: !<urn:a:b> x\n",                      // and a URN
		"k: !<!foo> x\n",                         // "!" is a URI character, though not a tag character
		"[!!str a, !b c]\n",                      // a tag ended by a flow indicator
		"%TAG !e! tag:a,2000:\n---\nk: !e!x y\n", // a named handle a directive defined
	} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			assert.NoError(t, err)
		})
	}
}

// TestParseKeepsATagThatEndsWhereTheDocumentDoes covers a tag with no break after it.
//
// The scanner emits the tag token on the character that ends the tag: a space, a break or a flow indicator.
// A tag running to the end of the source ends on none of them.
// The test catches the token going missing, so that "k: !!str" with no closing break parses to "k:" with no error.
func TestParseKeepsATagThatEndsWhereTheDocumentDoes(t *testing.T) {
	for source, want := range map[string]string{
		"k: !!str":           "k: !!str\n",
		"k: !fred":           "k: !fred\n",
		"k: !<tag:a,2000:b>": "k: !<tag:a,2000:b>\n",
		"k: !":               "k: !\n",
		"!!str":              "!!str\n",
		"- !!int":            "- !!int\n",
	} {
		t.Run(source, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)
			assert.Equal(t, want, f.String())
		})
	}
}

// TestParseReadsATagNamingAnotherKind covers "!!seq 5" and its relatives.
//
// The grammar puts no constraint on which tag stands on which node, so these documents are YAML 1.2.
// The parse builds the node the document wrote and keeps the tag on it, so the document renders as written.
//
// [github.com/go-openapi/go-yaml/ast.TagNode.Resolve] reports the mismatch, and the load rejects the document.
// TestLaxTagsDoesNotExcuseAKindMismatch holds that the load rejects it under any tag policy.
func TestParseReadsATagNamingAnotherKind(t *testing.T) {
	for _, source := range []string{
		"k: !!seq 5\n",
		"k: !!map 5\n",
		"k: !!set 5\n",
		"k: !!omap 5\n",
		"k: !!str [1, 2]\n",
		"k: !!int [1, 2]\n",
		"k: !!str {a: 1}\n",
		"k: !!bool [1]\n",
	} {
		t.Run(source, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)
			assert.Equal(t, source, f.String(), "the document does not render as it was written")
		})
	}
}

// TestParseReadsACollectionTagBeforeAnAnchor covers a collection tag and an anchor written in either order.
//
// A tag and an anchor stand in either order with the same meaning.
// With the tag first, the token after the tag is the anchor and not the "[" the tag types,
// and the test catches a parse that rejects "a: !!seq &a1 [1]" for it.
func TestParseReadsACollectionTagBeforeAnAnchor(t *testing.T) {
	for _, source := range []string{
		"a: !!seq &a1 [1]\n",
		"a: !!map &a1 {b: 1}\n",
		"a: &a1 !!seq [1]\n",
		"a: &a1 !!map {b: 1}\n",
	} {
		t.Run(source, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)
			assert.Equal(t, source, f.String())
		})
	}
}

// TestParseRefusesASecondTagOrAnchorOnANode covers two tags, or two anchors, written on one node.
//
// c-ns-properties gives a node at most one tag and one anchor, in either order.
// A line break may separate a node's properties, so "!a" over "!b x" puts two tags on the scalar "x".
// The test catches a parse that reads the second tag as the value of the first, and the document as "x".
// grammar.NewRecognizer, libfyaml and go.yaml.in/yaml/v3 all refuse every document here.
func TestParseRefusesASecondTagOrAnchorOnANode(t *testing.T) {
	for source, want := range map[string]string{
		"!a\n!b x\n":        "a node takes at most one tag",
		"!a !b x\n":         "a node takes at most one tag",
		"---\n!\n!int 23\n": "a node takes at most one tag",
		"!!str\n!!int 2\n":  "a node takes at most one tag",
		"!a\n!b\nk: v\n":    "a node takes at most one tag",
		"!a\n!b [1]\n":      "a node takes at most one tag",
		"k: !a\n  !b x\n":   "a node takes at most one tag",
		"- !a\n  !b x\n":    "a node takes at most one tag",
		"? !a\n  !b x\n":    "a node takes at most one tag",
		"[!a\n !b x]\n":     "a node takes at most one tag",

		// A second tag over a collection, behind a tag the core schema resolves to a scalar kind.
		"!!int !a\n? x\n": "a node takes at most one tag",
		"!!str !a [b]\n":  "a node takes at most one tag",

		// An anchor between the two tags does not part them.
		"!a &x\n!b y\n": "a node takes at most one tag",
		"!a &x !b y\n":  "a node takes at most one tag",

		// Nor does a tag between two anchors.
		"&x\n&y y\n":    "a node takes at most one anchor",
		"&x !a\n&y y\n": "a node takes at most one anchor",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			require.Error(t, err)
			assert.Contains(t, err.Error(), want)
		})
	}
}

// TestParseKeepsOneTagAndOneAnchorOnANode holds the accepting side of TestParseRefusesASecondTagOrAnchorOnANode.
//
// A tag and an anchor may stand on separate lines.
// A block mapping and its first key may each carry a tag:
// "!a" over "!b k: v" types the mapping with "!a" and the key with "!b", so the second tag stands inside the mapping.
func TestParseKeepsOneTagAndOneAnchorOnANode(t *testing.T) {
	for _, source := range []string{
		"!a\n&x y\n",
		"&x\n!a y\n",
		"!a &x\nk: v\n",
		"!a\n&x\nk: v\n",
		"!a\n!b k: v\n",
		"&x\n&y k: v\n",
		"!a &x\n!b &y k: v\n",
		"- !a\n  !b k: v\n",
		"k: !a\n  !b k2: v\n",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			assert.NoError(t, err)
		})
	}

	t.Run("and the second tag types the first key", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("!a\n!b k: v\n"))
		require.NoError(t, err)
		require.Len(t, f.Docs, 1)

		outer, ok := f.Docs[0].Body.(*ast.TagNode)
		require.True(t, ok, "the document body is %T", f.Docs[0].Body)
		assert.Equal(t, "!a", outer.Start.Value)

		mapping, ok := outer.Value.(*ast.MappingNode)
		require.True(t, ok, "the tag stands on %T", outer.Value)
		require.Len(t, mapping.Values, 1)

		key, ok := mapping.Values[0].Key.(*ast.TagNode)
		require.True(t, ok, "the first key is %T", mapping.Values[0].Key)
		assert.Equal(t, "!b", key.Start.Value)
	})
}

// TestParseKeepsTheTagOnAnEmptyCollection holds the line between a tag standing on nothing
// and one standing on a node of another kind.
//
// "k: !!seq" leaves the value out, which gives the tag's own default and is not a mismatch.
func TestParseKeepsTheTagOnAnEmptyCollection(t *testing.T) {
	for _, source := range []string{"k: !!seq\n", "k: !!map\n", "k: !!seq\nj: 1\n", "[!!seq]\n"} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			assert.NoError(t, err)
		})
	}
}

// TestParseReadsAScalarUnderATagThatResolvesToNothing covers a tag naming no type this library reads.
//
// The scalar under such a tag keeps the text it was written with, so "!thing 1" holds the string "1".
// The test catches parseTagValue leaving the cursor on the scalar it built,
// which makes the parse reject the document with "a document holds one root node, and this stands past it".
//
// A "%TAG !!" line repoints the secondary handle, so "!!seq" names !local-seq,
// which resolves to nothing while the grouping still joins it to its scalar.
func TestParseReadsAScalarUnderATagThatResolvesToNothing(t *testing.T) {
	for _, source := range []string{
		"v: !!foo 1\n",
		"v: !!foo 1\nw: 2\n",
		"- !!foo 1\n",
		"[!!foo 1]\n",
		"!!foo 1\n",
		"%TAG !! !local-\n---\nv: !!seq 1\n",
		"%TAG !! !local-\n---\nv: !!map 1\n",
		"%TAG !e! tag:example.com,2026:\n---\nv: !e!thing 1\n",
	} {
		t.Run(source, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)
			assert.Equal(t, source, f.String())

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(source), &got))
		})
	}
}

// TestParseReadsAnAnchorNamingNothingInAFlowCollection covers an anchor with nothing after it,
// under a tag naming a scalar type, inside a flow collection.
//
// The anchor names the empty node, which takes the tag's own default, and the flow collection carries on.
// The test catches a parse that reads the "}" or the "," behind the anchor as part of its value,
// and then rejects "{a: !!str &x}" for a missing '}'.
//
// anchorNamesNothing applies endsValue to the token after the anchor's name, the same test the token after a tag gets.
func TestParseReadsAnAnchorNamingNothingInAFlowCollection(t *testing.T) {
	for _, source := range []string{
		"{a: !!str &x}\n",
		"{a: !!str &x, b: 1}\n",
		"{a: !!int &x}\n",
		"{a: !!null &x}\n",
		"{a: !!bool &x}\n",
		"[!!str &x]\n",
		"[!!str &x, 1]\n",
		// Neighboring spellings: the other property order, an unresolved tag, a collection tag, block context.
		// They are held so that a fix cannot trade one for another.
		"{a: &x !!str}\n",
		"{a: !foo &x}\n",
		"{a: !!seq &x}\n",
		"{a: !!str &x b}\n",
		"a: !!str &x\nb: 1\n",
		"- !!str &x\n- 1\n",
	} {
		t.Run(source, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, source, f.String())

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(source), &got))
		})
	}

	t.Run("the anchor names the tag's own default, and an alias reads it", func(t *testing.T) {
		for source, want := range map[string]any{
			"{a: !!str &x, b: *x}\n":  map[string]any{"a": "", "b": ""},
			"{a: !!int &x, b: *x}\n":  map[string]any{"a": uint64(0), "b": uint64(0)},
			"{a: !!bool &x, b: *x}\n": map[string]any{"a": false, "b": false},
			"[!!str &x, *x]\n":        []any{"", ""},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(source), &got), "%q", source)
			assert.Equal(t, want, got, "%q", source)
		}
	})
}

// TestParseReadsATaggedMappingEntryOnTheTagsOwnLine covers a mapping entry that begins on its tag's line,
// with the tag standing over that entry.
//
// The grouping hands "&a [1]" and its ":" over as a map key group, and only a tag written first meets it.
// The test catches parseTagValue reading that group as the next entry of the enclosing collection,
// which leaves the tag on the empty node and the mapping unparsed.
// tagStandsOver is opensNextEntry with an exception for a "-".
//
// The document is still a kind mismatch: "!!str" names a scalar and the node is a mapping.
// The parser checks syntax and the decoder checks resolution,
// so the parser accepts the document and the decoder reports the mismatch at the tag, as for "!!str [1]: v".
func TestParseReadsATaggedMappingEntryOnTheTagsOwnLine(t *testing.T) {
	t.Run("the parser reads them", func(t *testing.T) {
		for _, source := range []string{
			"!!str &a [1]: v\n",
			"!!seq &a [1]: v\n",
			"!!str &a {a: 1}: v\n",
			"!!map &a [1]: v\n",
			// The same entry with the anchor written first.
			"&a !!str [1]: v\n",
			// A tag on its own line over a block collection.
			"!!str\nb: 1\n",
			"!!str\n- 1\n",
			"a: !!str\n- 1\n",
			"- !!str\n  - 1\n",
			// Neighboring shapes, held so that a fix cannot trade one for another.
			"!foo &a [1]: v\n",
			"!!str [1]: v\n",
			"&a [1]: v\n",
			"!!str &a [1]\n",
			"!!seq\n- 1\n",
			"a: !!seq\n- 1\n",
		} {
			f, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoErrorf(t, err, "%q", source)
			require.NotNil(t, f)
		}
	})

	t.Run("a tag naming a kind its node is not is reported at the tag", func(t *testing.T) {
		// "!!str &a [1]: v" is left out, and its spelling without the anchor is here.
		// The decoder rejects both, but the anchored one for a collection in key position:
		// decoding into an any strips a key's properties before naming the key, and the naming fails first.
		for _, source := range []string{
			"!!str [1]: v\n",
			"!!str\nb: 1\n", "!!str\n- 1\n", "a: !!str\n- 1\n",
		} {
			var got any
			err := yaml.Unmarshal([]byte(source), &got)
			require.Errorf(t, err, "%q", source)
			assert.Contains(t, err.Error(), "!!str does not support this kind of node", "%q", source)
		}
	})

	t.Run("a tag naming the kind it stands on reads", func(t *testing.T) {
		// These documents are keyed on a collection, which has no text to name an entry by.
		// They parse but do not decode, so this loop checks the parse alone.
		for _, source := range []string{
			"!!seq &a [1]: v\n",
			"!!map &a [1]: v\n",
			"!foo &a [1]: v\n",
			"&a [1]: v\n",
			"!!str &a [1]: v\n",
			"!!str &a {a: 1}: v\n",
		} {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "%q parses", source)
		}

		for source, want := range map[string]any{
			"!!seq\n- 1\n":       []any{uint64(1)},
			"a: !!seq\n- 1\n":    map[string]any{"a": []any{uint64(1)}},
			"a: !!str\nb: 1\n":   map[string]any{"a": "", "b": uint64(1)},
			"- !!str\n- 1\n":     []any{"", uint64(1)},
			"{a: !!str, b: 1}\n": map[string]any{"a": "", "b": uint64(1)},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(source), &got), "%q", source)
			assert.Equal(t, want, got, "%q", source)
		}
	})

	t.Run("a block sequence still may not open on the tag's own line", func(t *testing.T) {
		// Section 8.2.1 keeps a "-" off the line a node's properties are written on.
		for _, source := range []string{"!!int - 8\n", "---\n!!int - 23\n"} {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.Errorf(t, err, "%q", source)
		}
	})
}

// TestATagBeforeAnImplicitKeyTagsTheKey parses a tag standing before an implicit key on its line, and finds it on the
// key, as yaml.v3 and YAML 1.2 read it.
//
// A collection tag or a "!!" tag no schema defines stood on its own, and the parser put it on the mapping the key
// opens: "&a1 !!se q: 1" anchored and tagged the mapping. A tag on a line of its own still tags the mapping.
//
// So did such a tag written ahead of an anchor, "!!se &a1 q: 1": the anchor group was read as no inline scalar.
func TestATagBeforeAnImplicitKeyTagsTheKey(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"!!se q: 1\n", "&a1 !!se q: 1\n", "!!map q: 1\n", "!!omap 'q': 1\n", "a:\n  !!se q: 1\n",
		"!!se &a1 q: 1\n", "!!seq &a1 q: 1\n", "!!se &a1 'q': 1\n", "a:\n  !!se &a1 q: 1\n",
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoErrorf(t, err, "%q", src)

		body := file.Docs[0].Body
		if outer, ok := body.(*ast.MappingNode); ok && len(outer.Values) == 1 && outer.Values[0].Key.GetToken().Value == "a" {
			body = outer.Values[0].Value
		}
		mapping, ok := body.(*ast.MappingNode)
		require.Truef(t, ok, "%q: the tag went to the mapping, %T", src, body)

		key := ast.Unwrap(mapping.Values[0].Key)
		assert.Equalf(t, "q", key.GetToken().Value, "%q", src)
		_, tagged := mapping.Values[0].Key.(*ast.TagNode)
		_, anchored := mapping.Values[0].Key.(*ast.AnchorNode)
		assert.Truef(t, tagged || anchored, "%q: the key holds no property", src)
	}

	t.Run("a tag on its own line tags the mapping", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte("!!map\nq: 1\n"))
		require.NoError(t, err)
		_, tagged := file.Docs[0].Body.(*ast.TagNode)
		assert.True(t, tagged)
	})

	t.Run("a block scalar after the anchor stays outside the tag", func(t *testing.T) {
		// The parser reads what the tag stands on, as before the anchor was looked through.
		_, err := parser.ParseBytes([]byte("!!seq &a |\n x\n"))
		require.NoError(t, err)
	})
}
