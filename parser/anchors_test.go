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

// TestParseAnchorsOnEmptyScalars checks that an anchor with nothing after it names the empty node,
// in each position a value may be absent, and that the document renders back unchanged.
//
// "a: &x" is a valid document, and *x resolves to null.
func TestParseAnchorsOnEmptyScalars(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"as a mapping value at the end of the document": {
			source: "b: &b\n",
			want:   "b: &b\n",
		},
		"as a mapping value with more to follow": {
			source: "a: &x\nb: 1\n",
			want:   "a: &x\nb: 1\n",
		},
		"as an explicit key": {
			source: "? &d\n",
			want:   "? &d\n:\n",
		},
		"as an explicit key's value": {
			source: "? &e\n: &a\n",
			want:   "? &e\n: &a\n",
		},
		// The space before the ':' matters: ':' is a legal anchor character,
		// so "&a: a" anchors the name "a:" over the scalar a and is not a mapping.
		"as a mapping key": {
			source: "&a : a\n",
			want:   "&a : a\n",
		},
		"as a mapping key nested under another": {
			source: "x:\n  &a : a\n  b: 1\n",
			want:   "x:\n  &a : a\n  b: 1\n",
		},
		"as a mapping key in a sequence entry": {
			source: "-\n  &a : a\n  b: 1\n",
			want:   "- &a : a\n  b: 1\n",
		},
		"as a sequence entry": {
			source: "- &a\n- a\n",
			want:   "- &a\n- a\n",
		},
		// The next token stands left of the '-', so the entry ends and the anchor names the empty node.
		"as a nested sequence entry before a key further out": {
			source: "k:\n  - &a\nb: 1\n",
			want:   "k:\n- &a\nb: 1\n",
		},
		"as a nested sequence entry before a comment and a key further out": {
			source: "k:\n  - &a\n  # c\nb: 1\n",
			want:   "k:\n- &a\n# c\nb: 1\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			// The rendering reads back to the same text.
			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseAnchorsOnFlowCollectionKeys checks that a property written before a flow collection used as a mapping key
// belongs to the key: "&k [a, b]: v" anchors the sequence, not the mapping holding the entry.
func TestParseAnchorsOnFlowCollectionKeys(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"anchor on a flow sequence key": {
			source: "&key [a, b]: value\n",
			want:   "&key [a, b]: value\n",
		},
		"anchor on a flow mapping key": {
			source: "&key {a: 1}: value\n",
			want:   "&key {a: 1}: value\n",
		},
		"tag on a flow sequence key": {
			source: "!!seq [a]: value\n",
			want:   "!!seq [a]: value\n",
		},
		"beneath an anchor of the mapping's own": {
			source: "&mapping\n&key [ a ]: value\n",
			want:   "&mapping\n  &key [a]: value\n",
		},
		"inside a flow mapping": {
			source: "{ &a [a, b]: c }\n",
			want:   "{&a [a, b]: c}\n",
		},
		"with an anchored entry as well": {
			source: "&key [ &item a, b ]: value\n",
			want:   "&key [&item a, b]: value\n",
		},
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

// TestParseAnchorsStillNeedANameAndOneValue checks that an anchor without a name, two anchors in a row,
// and a block sequence after an anchor on the same line are rejected.
func TestParseAnchorsStillNeedANameAndOneValue(t *testing.T) {
	sources := map[string]string{
		"no name":               "a: &\n",
		"two anchors in a row":  "a: &x &y value\n",
		"sequence after anchor": "a: &x - b\n",
	}

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}
}

// TestParseAnchorNamesTakeEveryAnchorChar checks that an anchor or alias name may hold any ns-anchor-char,
// including the characters that open a token elsewhere.
//
// ns-anchor-char is ns-char less the flow indicators, so "@", "`", "#", a quote and a "%" are all name characters.
// Section 5.5 reserves "@" and "`" only at the start of a plain scalar.
func TestParseAnchorNamesTakeEveryAnchorChar(t *testing.T) {
	t.Run("as a name, and as the alias that reaches it", func(t *testing.T) {
		for _, name := range []string{
			"@", "#", `"`, "'", "`", "%", "@x", "x@", ":",
			// Each of these opens a tag, a block scalar, an anchor or an alias elsewhere.
			"!", "!x", "!!", "!<!foo>", "|", "|x", ">", ">x", "&", "&x", "*", "*x",
			// These open a sequence entry, an explicit key or a merge key elsewhere.
			"-", "-x", "?", "?x", "<<", "<<:",
		} {
			source := "a: &" + name + " 1\nb: *" + name + "\n"

			f, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoErrorf(t, err, "%q", source)
			assert.Equal(t, source, f.String(), "%q", source)

			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(source), &got), "%q", source)
			assert.Equal(t, map[string]any{"a": uint64(1), "b": uint64(1)}, got, "%q", source)
		}
	})

	t.Run("in a flow collection and in a block sequence", func(t *testing.T) {
		for source, want := range map[string]any{
			"[&- x, *-]\n":       []any{"x", "x"},
			"{&? k: v, j: *?}\n": map[string]any{"k": "v", "j": "k"},
			"- &- x\n- *-\n":     []any{"x", "x"},
			"- &? x\n- *?\n":     []any{"x", "x"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(source), &got), "%q", source)
			assert.Equal(t, want, got, "%q", source)
		}
	})

	t.Run("a flow indicator still ends the name", func(t *testing.T) {
		// ns-anchor-char excludes the flow indicators, so an anchor opening on one has no name.
		for _, source := range []string{"a: &[ 1\n", "a: &] 1\n", "a: &{ 1\n", "a: &} 1\n", "a: &, 1\n"} {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.Errorf(t, err, "%q", source)
			assert.Contains(t, err.Error(), "must be followed by a name", "%q", source)
		}
	})

	t.Run("and the same characters still open their own token elsewhere", func(t *testing.T) {
		// A "#" inside a plain scalar is ordinary text, because a comment needs a space before it.
		// The comment rule applies to a "#" pressed against a token already cut, as after a quoted scalar.
		for source, says := range map[string]string{
			"a: @x\n":      "'@' is a reserved character",
			"a: `x\n":      "'`' is a reserved character",
			"a: %x\n":      "a plain scalar cannot begin with '%'",
			"a: \"b\"#c\n": "a comment must be preceded by a space",
		} {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.Errorf(t, err, "%q", source)
			assert.Contains(t, err.Error(), says, "%q", source)
		}
	})
}

// TestParseRefusesAPropertyOnAnAlias covers an anchor or a tag written in front of an alias.
//
// c-ns-alias-node is the '*' and its name and nothing else, so an alias takes no property. parseScalarValue met the
// '*' in its catch-all arm and refused it as "unexpected scalar value type". yaml/yaml-reference-parser refuses each
// of these, and libfyaml with "unexpected alias".
func TestParseRefusesAPropertyOnAnAlias(t *testing.T) {
	for _, source := range []string{
		"&a *b\n", "!!bool *TRUE\n", "!x *b\n", "a: &x *y\n", "a: !t *b\n", "- &a *b\n", "[&a *b]\n",
	} {
		_, err := parser.ParseBytes([]byte(source))
		require.Errorf(t, err, "%q", source)
		assert.Containsf(t, err.Error(), "an alias cannot carry an anchor or a tag", "%q", source)
	}
}
