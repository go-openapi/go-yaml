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

// TestParseFlowKeyLineBreaks checks where a flow mapping's key may end and its ':' may begin.
//
// A flow mapping's key may span lines, and a line break before its ':' is ordinary separation.
// A pair written inside a flow sequence is an implicit key, and must fit on one line with its ':'.
func TestParseFlowKeyLineBreaks(t *testing.T) {
	valid := map[string]string{
		"colon on the line after the key": "{foo\n: bar}\n",
		"key and colon on their own lines": "k: {\n" +
			" k\n" +
			" :\n" +
			" v\n" +
			" }\n",
		"key spanning lines":        "- { multi\n  line: value}\n",
		"key spanning lines nested": "{ matches\n% : 20 }\n",

		// A mapping's key may take the next line for its ':', whatever the key is made of.
		"quoted key and colon on separate lines":     "{ \"key\"\n  : value }\n",
		"collection key and colon on separate lines": "{ {a: 1}\n  : value }\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}

	invalid := map[string]string{
		// A single-pair entry in a flow sequence is an implicit key, which must fit on one line with its ':'.
		// Quoting the key does not exempt it.
		"implicit key followed by a newline":          "[ key\n  : value ]\n",
		"quoted implicit key followed by a newline":   "[ \"key\"\n  : value ]\n",
		"quoted implicit key with an adjacent value":  "[ \"key\"\n  :value ]\n",
		"collection key followed by a newline":        "[ {a: 1}\n  : value ]\n",
		"single-quoted implicit key across two lines": "[ 'key'\n  : value ]\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}
}

// TestParseAdjacentValuesInFlow checks a ':' written with no space before its value.
//
// The spec allows it only after a JSON-like key (a quoted scalar or a flow collection),
// so the parser reads JSON's own {"a":1} as well as "{a: 1}".
// After a plain scalar the ':' belongs to the scalar, so [ a:b ] holds one entry and not a pair.
func TestParseAdjacentValuesInFlow(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"quoted key in a flow sequence":     {"[ \"JSON like\":adjacent ]\n", "[\"JSON like\": adjacent]\n"},
		"collection key in a flow sequence": {"[ {JSON: like}:adjacent ]\n", "[{JSON: like}: adjacent]\n"},
		"quoted key in a flow mapping":      {"{\"a\":1}\n", "{\"a\": 1}\n"},
		"several in one collection":         {"[\"a\":1, [b]:2]\n", "[\"a\": 1, [b]: 2]\n"},

		// After a plain scalar the ':' belongs to the scalar.
		// In a sequence that leaves one entry, and in a mapping one key with no value, rendered with a ':' after it.
		"plain key in a flow sequence": {"[ a:b ]\n", "[a:b]\n"},
		"plain key in a flow mapping":  {"{ a:b }\n", "{a:b:}\n"},

		// A ':' right before the end of the entry closes the key, since a plain scalar cannot hold it there.
		"absent value before a brace": {"{a:}\n", "{a:}\n"},
		"absent value before a comma": {"{a:, b: 1}\n", "{a:, b: 1}\n"},
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

// TestParseFlowCollectionsAsKeys checks a flow collection used as a mapping key,
// nested inside another one used the same way.
//
// Once "[b]: d" is grouped as an entry, the group reports the type of its first token, a '['.
// The search for the start of the enclosing key must not count that '[' as an open bracket with no ']' to match.
func TestParseFlowCollectionsAsKeys(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"one level":            {"[ [b]: d ]\n", "[[b]: d]\n"},
		"two levels":           {"[ [[b]: d]: 23 ]\n", "[[[b]: d]: 23]\n"},
		"beside other entries": {"[ [a, [ [[b,c]]: d, e]]: 23 ]\n", "[[a, [[[b, c]]: d, e]]: 23]\n"},
		"in a flow mapping":    {"{ [[b]: d]: 23 }\n", "{[[b]: d]: 23}\n"},
		"spanning lines":       {"[\n  [[b]: d]: 23\n]\n", "[[[b]: d]: 23]\n"},
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

// TestParseFlowComments checks comments written inside a flow collection.
//
// A comment runs to the end of its line, so a flow collection holding one renders over several lines.
// On the collection's single line, the comment would hide everything after it, including the closing bracket.
func TestParseFlowComments(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"before the comma": {
			source: "[ word1\n# comment\n, word2]\n",
			want:   "[\n  word1,\n  # comment\n  word2\n]\n",
		},
		"after the comma on its own line": {
			source: "[ a,\n# comment\n  b ]\n",
			want:   "[\n  a,\n  # comment\n  b\n]\n",
		},
		"in a flow mapping": {
			source: "{ a: 1,\n# comment\n  b: 2 }\n",
			want:   "{\n  a: 1,\n  # comment\n  b: 2\n}\n",
		},
		"before the closing bracket": {
			source: "[ a, b\n# comment\n]\n",
			want:   "[\n  a,\n  b\n  # comment\n]\n",
		},
		// A comment on the ',' line belongs to the entry before the ',', and stays with it.
		"on the comma's line in a sequence": {
			source: "[ a, # comment\n  b ]\n",
			want:   "[\n  a, # comment\n  b\n]\n",
		},
		"on the comma's line in a mapping": {
			source: "{ a: 1, # comment\n  b: 2 }\n",
			want:   "{\n  a: 1, # comment\n  b: 2\n}\n",
		},
		// Without a comment the collection stays on one line.
		"none at all": {
			source: "{a: 1, b: 2}\n",
			want:   "{a: 1, b: 2}\n",
		},
		"none at all in a sequence": {
			source: "[a, b]\n",
			want:   "[a, b]\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			// The rendered text must read back to the same text.
			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseEmptyNodeInAFlowCollection checks the flow collection entries that carry no scalar of their own.
//
// Two productions admit them: c-ns-flow-map-empty-key-entry, an entry whose key is e-node,
// and ns-flow-pair, which a flow sequence admits as an entry and whose value may be e-node as well.
//
// The recognizer compiled from yaml-spec-1.2.json accepts each rendered form below,
// and each reads back to the same text.
func TestParseEmptyNodeInAFlowCollection(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
		value  any
	}{
		"an anchor alone in a flow mapping": {
			source: "{&a}\n",
			want:   "{&a :}\n",
			value:  map[any]any{nil: nil},
		},
		"an anchor alone before another entry": {
			source: "{&a, b: 1}\n",
			want:   "{&a :, b: 1}\n",
			value:  map[any]any{nil: nil, "b": uint64(1)},
		},
		"an anchor alone after another entry": {
			source: "{b: 1, &a}\n",
			want:   "{b: 1, &a :}\n",
			value:  map[any]any{nil: nil, "b": uint64(1)},
		},
		"a pair with neither side": {
			source: "[:]\n",
			want:   "[:]\n",
			value:  []any{map[any]any{nil: nil}},
		},
		"a pair with neither side, before an entry": {
			source: "[:, a]\n",
			want:   "[:, a]\n",
			value:  []any{map[any]any{nil: nil}, "a"},
		},
		"a pair with neither side, after an entry": {
			source: "[a, :]\n",
			want:   "[a, :]\n",
			value:  []any{"a", map[any]any{nil: nil}},
		},
		"a pair with no value": {
			source: "[a:]\n",
			want:   "[a:]\n",
			value:  []any{map[string]any{"a": nil}},
		},
		"an explicit key with no value": {
			source: "[? a]\n",
			want:   "[? a :]\n",
			value:  []any{map[string]any{"a": nil}},
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

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &got))
			assert.Equal(t, test.value, got)
		})
	}
}

// TestAnUnclosedFlowNamesTheReasonItEnded checks that a flow collection the
// scanner refused is reported by the scanner's complaint, not by the missing
// closer the descent runs into afterwards.
//
// The key window hands a flow collection on as it reads one that cannot stand
// as a mapping key, so the descent now reaches the end of the tokens before the
// scanner has refused the badly indented line. Both complaints are true; the
// scanner's names the line at fault, which is where libfyaml points too, and
// the descent's names only the "[" it started from.
func TestAnUnclosedFlowNamesTheReasonItEnded(t *testing.T) {
	for name, test := range map[string]struct{ source, want string }{
		"sequence continued at the parent's indent": {
			source: "flow: [a,\nb,\nc]\n",
			want:   "a flow collection continues on a line that is not indented past the one it started on",
		},
		"mapping continued at the parent's indent": {
			source: "flow: {a: 1,\nb: 2}\n",
			want:   "a flow collection continues on a line that is not indented past the one it started on",
		},
		"nested, continued at the parent's indent": {
			source: "a:\n  flow: [x,\n  y]\n",
			want:   "a flow collection continues on a line that is not indented past the one it started on",
		},
		"sequence that simply ends": {
			source: "[a, b\n",
			want:   "sequence end token ']' not found",
		},
		"mapping that simply ends": {
			source: "{a: 1\n",
			want:   "could not find flow mapping end token '}'",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(test.source))
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}
