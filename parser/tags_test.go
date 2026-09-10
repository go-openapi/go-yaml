// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseTagsOnEmptyScalars covers a scalar tag with nothing after it.
//
// Such a tag marks the empty node, exactly as an anchor with nothing after it
// names it. What follows the tag is the next entry of the collection around it,
// not the tag's value, and reading it as one is what used to fail.
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

			// And it settles: what was written reads back to the same text.
			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseEmptyKeysCarryingProperties covers a key that is nothing but an
// anchor, an alias or a tag, in the positions such a key may appear in.
//
// Nested in a block collection these used to be refused. The key is cut into
// tokens before the ':' is reached, and the scanner then had no column to
// measure the following lines against, so the line below the entry was read as
// a continuation of its value rather than as the next entry.
func TestParseEmptyKeysCarryingProperties(t *testing.T) {
	sources := map[string]string{
		"anchored, at the top level":       "&a : a\n",
		"tagged, at the top level":         "!!null : a\n",
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

// TestRenderPropertyKeysKeepTheirSeparator covers the space between such a key
// and its ':'.
//
// ':' is a legal character in an anchor name, an alias name and a tag, so a key
// that ends on one absorbs a ':' written straight after it: "&a: v" anchors the
// name "a:" over the scalar v, where "&a : v" anchors the empty key of a
// mapping. Dropping the space changes what the document means.
func TestRenderPropertyKeysKeepTheirSeparator(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"anchor on the empty key": {source: "&a : v\n", want: "&a : v\n"},
		"tag on the empty key":    {source: "!!str : v\n", want: "!!str : v\n"},
		"alias as the key":        {source: "k: &r v\n*r : a\n", want: "k: &r v\n*r : a\n"},
		"anchor and empty value":  {source: "-\n  &c : &a\n", want: "- &c : &a\n"},

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

// TestParseTagOnTheEmptyNodeInAFlowCollection covers a tag standing against the
// punctuation of the collection it is written in.
//
// ns-flow-node admits c-ns-properties followed by e-scalar, so "!" with a ']'
// after it is the non-specific tag on the empty node. Two things stopped that
// from being read. isFlowType, which keeps a tag from being grouped with the
// token after it, listed the openers and '}' but not ']' or ',': "[!]" grouped
// the tag with the ']', and the sequence then ran to the end of the stream
// looking for a closer it had already passed. And scanTag treated '}' as a
// character no tag may hold instead of ending the tag there, and swallowed ']'
// into the tag's name.
//
// The null a tag stands on when nothing follows it is implicit, so the renderer
// writes nothing for it and the document comes back as it went in. Written out
// as "null" it read back as the *string* "null", since a tag that resolves to
// nothing leaves its scalar as text.
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

// TestDecodeTagOnTheEmptyNodeInAFlowCollection pins the values, which is where
// the resolved and unresolved tags part company: "!" and a local tag leave the
// empty node unresolved, which is null, and "!!str" makes it the empty string.
func TestDecodeTagOnTheEmptyNodeInAFlowCollection(t *testing.T) {
	tests := map[string]struct {
		source string
		want   any
	}{
		"the non-specific tag": {source: "[!]\n", want: []any{nil}},
		"a local tag":          {source: "[!str]\n", want: []any{nil}},
		"the string tag":       {source: "[!!str]\n", want: []any{""}},
		"the integer tag":      {source: "[!!int]\n", want: []any{0}},
		"mixed with entries":   {source: "[a, !, b]\n", want: []any{"a", nil, "b"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &got))
			assert.Equal(t, test.want, got)
		})
	}
}

// TestParseRefusesATagTheGrammarRefuses covers the tag shapes YAML 1.2 has no
// production for.
//
// The scanner read a tag through to whatever ended it and never looked at the
// characters. Three shapes came back as ordinary tags and resolved to !!str,
// so "!<> x" decoded to "x" and nothing said the tag was not one:
//
//	c-verbatim-tag  ::= "!" "<" ns-uri-char+ ">"
//	ns-uri-char     ::= "%" ns-hex-digit ns-hex-digit | ns-word-char | ...
//	ns-tag-char     ::= ns-uri-char - "!" - c-flow-indicator
//
// So a verbatim tag takes at least one URI character, a "%" stands only in
// front of two hexadecimal digits, and neither "<" nor ">" is a character any
// tag may hold. Checked against grammar.NewRecognizer, which reads the
// specification's own grammar and refuses all of these.
func TestParseRefusesATagTheGrammarRefuses(t *testing.T) {
	for source, want := range map[string]string{
		"k: !<> x\n":    "must name a URI",
		"k: !<%> x\n":   "two hexadecimal digits",
		"k: !<%4> x\n":  "two hexadecimal digits",
		"k: !<%zz> x\n": "two hexadecimal digits",
		"k: !<a x\n":    "must end with '>'",
		"k: !!<x> y\n":  "invalid tag character",
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

// TestParseKeepsEveryTagTheGrammarAdmits is the other half, and the reason the
// check above is written from the productions rather than from the three
// documents that started it.
//
// A local tag especially: sixteen YAML Test Suite cases carry one, and it is
// how CloudFormation writes "!Ref" and GitLab CI writes "!reference".
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

// TestParseKeepsATagThatEndsWhereTheDocumentDoes covers a tag with no break
// after it.
//
// The scanner emits the tag token on whatever character ends the tag -- a
// space, a break, a flow indicator. A tag running to the end of the source ends
// on none of them, so the loop fell out and the token was never made: "k: !!str"
// with no closing break parsed to "k:" and the tag was gone, with nothing
// reported. The grammar reads all of these.
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
// The grammar puts no constraint on which tag stands on which node, so these
// are YAML 1.2 and grammar.NewRecognizer reads them. The parse used to refuse
// them with "could not find map", "value is not allowed in this context" or
// "unexpected scalar value type" -- none of which names the tag, and each of
// which put the document out of reach of a consumer that only wanted to read or
// reformat it.
//
// The parse builds the node the document wrote and the tag stays on it, so the
// document renders as it was written. What the tag made of the node is
// [github.com/go-openapi/go-yaml/ast.TagNode.Resolve]'s to report, and the load
// refuses it -- see TestLaxTagsDoesNotExcuseAKindMismatch, which holds that the
// refusal stands whatever the tag policy.
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

// TestParseReadsACollectionTagBeforeAnAnchor covers the shape that made the
// refusal above worth removing rather than rewording.
//
// A tag and an anchor stand in either order and mean the same. "a: !!seq &a1
// [1]" was refused outright, because the token after the tag was the anchor and
// not the "[" the tag insisted on.
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

// TestParseKeepsTheTagOnAnEmptyCollection holds the line between a tag standing
// on nothing and one standing on a node of another kind.
//
// "k: !!seq" left the value out, which is the tag's own default and not a
// mismatch, so it must not be caught by the change above.
func TestParseKeepsTheTagOnAnEmptyCollection(t *testing.T) {
	for _, source := range []string{"k: !!seq\n", "k: !!map\n", "k: !!seq\nj: 1\n", "[!!seq]\n"} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source))
			assert.NoError(t, err)
		})
	}
}

// TestParseReadsAScalarUnderATagThatResolvesToNothing: a document whose tag
// names no type this library reads is a document, not an error.
//
// The scalar under such a tag keeps the text it was written with, so "!thing 1"
// holds the string "1". parseTagValue builds that string and used to leave the
// cursor on the scalar it had just read, so the next thing to look at the
// stream found a token where the entry had already ended and refused the
// document with "value is not allowed in this context".
//
// Only a "%TAG !!" line reached it before 2026-09-11: repointing the secondary
// handle takes "!!seq" out of tag:yaml.org,2002: and leaves it naming
// !local-seq, which resolves to nothing while still being spelled like a tag
// the grouping joins to its scalar.
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

// TestParseReadsAnAnchorNamingNothingInAFlowCollection: an anchor with nothing
// after it, under a tag naming a scalar type, inside a flow collection.
//
// "{a: !!str &x}" was refused with `could not find flow mapping end token '}'`
// and "{a: !!str &x, b: 1}" at the comma with `',' or '}' must be specified`,
// so whatever read the anchor read the entry separator behind it. The anchor
// names the empty node, which takes the tag's own default, and the flow
// collection carries on.
//
// The order of the two properties decided it -- "{a: &x !!str}" read -- and so
// did the tag: "{a: !foo &x}" read, because a tag the schema does not resolve
// leaves parseTagValue by another branch, and "{a: !!seq &x}" read as a
// collection tag. In block context "a: !!str &x" over "b: 1" always read.
//
// anchorNamesNothing asks endsValue of the token after the anchor's name, which
// is the test the tag's own next token already got.
func TestParseReadsAnAnchorNamingNothingInAFlowCollection(t *testing.T) {
	for _, source := range []string{
		"{a: !!str &x}\n",
		"{a: !!str &x, b: 1}\n",
		"{a: !!int &x}\n",
		"{a: !!null &x}\n",
		"{a: !!bool &x}\n",
		"[!!str &x]\n",
		"[!!str &x, 1]\n",
		// The spellings that read before, held here so a fix cannot trade one
		// for another.
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

// TestParseReadsATaggedMappingEntryOnTheTagsOwnLine: a mapping entry may begin
// on the line its tag is written on, and the tag stands over that entry.
//
// "!!str &a [1]: v" was refused with `value is not allowed in this context`,
// the caret on the anchor. The grouping hands "&a [1]" and its ":" over as a
// map key, and parseTagValue read any map key as the next entry of the
// collection around the tag -- so the tag took the empty node and the whole
// mapping was left unparsed. It asks tagStandsOver now, which is
// opensNextEntry with a carve-out for a "-".
//
// The order of the two properties decided it, and the parse was where that
// showed: "&a !!str [1]: v" parsed on master and "!!str &a [1]: v" did not.
// The key position is the cause and the order was the symptom -- only a tag
// written first meets the map key group -- so the fix collapses the two into
// one answer rather than settling which order is right.
//
// The document is a kind mismatch and the decoder still reports one, at the
// tag: "!!str" names a scalar and the node is a mapping, which is the same
// answer "!!str [1]: v" has always given. That is resolution and not syntax,
// and the two are settled at their own layers.
func TestParseReadsATaggedMappingEntryOnTheTagsOwnLine(t *testing.T) {
	t.Run("the parser reads them", func(t *testing.T) {
		for _, source := range []string{
			"!!str &a [1]: v\n",
			"!!seq &a [1]: v\n",
			"!!str &a {a: 1}: v\n",
			"!!map &a [1]: v\n",
			// Read on master too, where the same document with the tag
			// written first did not.
			"&a !!str [1]: v\n",
			// The same shapes with a tag on its own line over a block
			// collection, which reported the complaint one line down.
			"!!str\nb: 1\n",
			"!!str\n- 1\n",
			"a: !!str\n- 1\n",
			"- !!str\n  - 1\n",
			// These always read, and still do.
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
		// ⚠️ "!!str &a [1]: v" is not here, and its spelling without the anchor
		// is. Both are refused; the anchored one is refused for being a
		// collection in key position rather than for the tag, because the walk
		// that reads an any strips a key's properties before naming it and the
		// naming speaks first. The two spellings of one document give two
		// messages, which is worth having written down.
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
		// The four keyed on a sequence parse and do not decode -- a collection
		// has no text to name an entry by -- so they are asserted below, where
		// the parse is the question this test asks.
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
		// 8.2.1 keeps a "-" off the line a node's properties are written on.
		// grammar.NewRecognizer refuses these, and so do libfyaml 1.0.0b1, the
		// reference parser and go.yaml.in/yaml/v3.
		for _, source := range []string{"!!int - 8\n", "---\n!!int - 23\n"} {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.Errorf(t, err, "%q", source)
		}
	})
}
