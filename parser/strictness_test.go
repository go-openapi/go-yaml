// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestRejectsMalformedDocuments checks that each construct YAML forbids is rejected.
//
// Each case pairs the invalid document with the nearest well-formed one, which must parse,
// so a fix that rejects too much fails as well.
func TestRejectsMalformedDocuments(t *testing.T) {
	tests := map[string]struct {
		invalid string
		valid   string
	}{
		// A comment starts a line or follows white space.
		"comment after a quoted scalar": {
			invalid: "key: \"value\"# comment\n",
			valid:   "key: \"value\" # comment\n",
		},
		"comment after a comma": {
			invalid: "[a, b,#comment\n]\n",
			valid:   "[a, b, #comment\n]\n",
		},
		"comment after a closing bracket": {
			invalid: "[a, b]#comment\n",
			valid:   "[a, b] #comment\n",
		},
		// '#' cannot begin a scalar, though it is ordinary inside one.
		"scalar beginning with a hash": {
			invalid: "key: [#a]\n",
			valid:   "key: [a#b]\n",
		},

		// A flow collection holds no block sequence entries, and '-' alone is not a scalar there.
		"dash as a flow sequence entry": {
			invalid: "[-]\n",
			valid:   "[-a]\n",
		},
		"dashes as flow sequence entries": {
			invalid: "- [-, -]\n",
			valid:   "- [-a, -b]\n",
		},
		// Nor before a blank, where it read as a block sequence entry: "[- a]" as [[a]], "{a: - b}" as {a: [b]}.
		"dash before a space in a flow sequence": {
			invalid: "[- a]\n",
			valid:   "[a - b]\n",
		},
		"dash before a space in a flow mapping": {
			invalid: "{a: - b}\n",
			valid:   "{-: b}\n",
		},
		"dash before a line break in a flow mapping": {
			invalid: "{a: -\n}\n",
			valid:   "{a: -x\n}\n",
		},
		"dash before a tab in a flow sequence": {
			invalid: "[-\ta]\n",
			valid:   "[a -]\n",
		},

		// A tag shorthand cannot hold a ',', which must be percent-encoded.
		// A verbatim tag holds a URI as written.
		"comma in a tag shorthand": {
			invalid: "- !!str, xxx\n",
			valid:   "- !<tag:yaml.org,2002:str> xxx\n",
		},

		// A TAG directive defines a handle for the next document only.
		"tag handle used past its document": {
			invalid: "%TAG !p! tag:example.com,2011:\n--- !p!A\na: b\n--- !p!B\nc: d\n",
			valid:   "%TAG !p! tag:example.com,2011:\n--- !p!A\na: b\n",
		},

		// A construct continued on the next line must be indented past what introduced it.
		"flow collection not indented past its key": {
			invalid: "flow: [a,\nb]\n",
			valid:   "flow: [a,\n b]\n",
		},
		"flow collection continued after a tab": {
			invalid: "- [\n\tfoo,\n foo\n ]\n",
			valid:   "- [\n foo,\n foo\n ]\n",
		},
		"quoted scalar not indented past its key": {
			invalid: "quoted: \"a\nb\nc\"\n",
			valid:   "quoted: \"a\n b\n c\"\n",
		},
		// The same rule with the key left out.
		// The entry begins at the ':', so a token level with it opens the next entry, and "1" opens nothing.
		"value level with an empty key": {
			invalid: ":\n1\n",
			valid:   ":\n  1\n",
		},

		// ns-anchor-name holds one character or more and ends at a flow indicator.
		// A collection opening straight after it has no separation in front of it.
		"anchor with no name": {
			invalid: "& e\n",
			valid:   "&a e\n",
		},
		"flow sequence touching the anchor in front of it": {
			invalid: "&a[]\n",
			valid:   "&a []\n",
		},
		"flow mapping touching the anchor in front of it": {
			invalid: "[&a{1: 2}]\n",
			valid:   "[&a {1: 2}]\n",
		},

		// A '%' opens a directive only at the start of a line.
		// Anywhere else it opens no directive, and it cannot open a plain scalar either.
		"percent sign as a value": {
			invalid: " k: %\n",
			valid:   " k: a%b\n",
		},
		"percent sign as a sequence entry": {
			invalid: " - %\n",
			valid:   " - 100%\n",
		},
		"percent sign opening a block value": {
			invalid: "k:\n  %\n",
			valid:   "%YAML 1.2\n---\nk: 1\n",
		},

		// A plain scalar cannot open on an indicator, though indicators are ordinary characters inside one.
		// Inside a flow collection the indicator ends the entry before a scalar can begin,
		// so an anchor on the empty node parses there.
		"closing brace as a whole document": {
			invalid: "}\n",
			valid:   "{}\n",
		},
		"comma as a whole document": {
			invalid: ",\n",
			valid:   "a,b\n",
		},
		"comma after an anchor outside a flow collection": {
			invalid: "&a,\n",
			valid:   "[&a, b]\n",
		},
		"closing brace after an anchor outside a flow mapping": {
			invalid: "&a}\n",
			valid:   "{&a: b}\n",
		},
		"closing bracket after an anchor outside a flow sequence": {
			invalid: "&a]\n",
			valid:   "[&a]\n",
		},

		// A block scalar header takes one indentation indicator and one chomping indicator,
		// in either order, and either may be left out. Two of either is not a header.
		"two chomping indicators": {
			invalid: "|--\n",
			valid:   "|-\n",
		},
		"two chomping indicators, folded": {
			invalid: ">++\n",
			valid:   ">+\n",
		},
		"two indentation indicators": {
			invalid: "|12\n  a\n",
			valid:   "|1\n  a\n",
		},
		// A comment after a header needs white space before it, like every other comment.
		"comment touching a block scalar header": {
			invalid: "|-#\n",
			valid:   "|- #\n",
		},

		// A numeric escape takes hexadecimal digits, and the escape fixes how many.
		// An escape holding any other character is rejected, not decoded to another character.
		"escaped 8-bit character with no hex digits": {
			invalid: `"\xZZ"` + "\n",
			valid:   `"\x41"` + "\n",
		},
		"escaped UTF-16 character shifted by a letter": {
			invalid: `"\uu0BA"` + "\n",
			valid:   `"º"` + "\n",
		},
		"escaped UTF-32 character with no hex digits": {
			invalid: `"\U0000004G"` + "\n",
			valid:   `"\U00000041"` + "\n",
		},
		"low surrogate with no hex digits": {
			invalid: `"\uD83D\uDEZZ"` + "\n",
			valid:   `"😀"` + "\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(test.invalid), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", test.invalid)

			_, err = parser.ParseBytes([]byte(test.valid), parser.WithComments())
			require.NoErrorf(t, err, "rejected %q", test.valid)
		})
	}
}

// TestAcceptsDocumentsAtTheRoot checks that a construct at the document's root may continue at any indentation.
//
// Nothing encloses it, so its further lines have nothing to be indented past.
func TestAcceptsDocumentsAtTheRoot(t *testing.T) {
	sources := map[string]string{
		"flow mapping at the root":   "{\n? explicit: entry,\nimplicit: entry,\n}\n",
		"flow sequence at the root":  "[\na,\nb,\n]\n",
		"nested flow at the root":    "{ key: [[[\n  value\n ]]]\n}\n",
		"quoted scalar at the root":  "\"a\nb\nc\"\n",
		"blank line inside a scalar": "key: \"a\n\n b\"\n",
	}

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}
}

// TestParseRefusesWhatIsNotAStream checks that the parser rejects a source that is not a YAML character stream.
//
// c-printable excludes the control characters below x20 other than tab, line feed and carriage return.
// A stream is Unicode too, so a byte that belongs to no character is rejected, not replaced with U+FFFD.
//
// Escapes are unaffected: they write these characters into a value, not into the source.
func TestParseRefusesWhatIsNotAStream(t *testing.T) {
	invalid := map[string]string{
		"a NUL as the whole document":  "\x00\n",
		"a NUL in a value":             "a: \x00\n",
		"a control character in a key": "a\x01b: 1\n",
		"a byte that is not text":      "\xbf\n",
		"a truncated character":        "a: \xe2\x82\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}

	valid := map[string]string{
		"a NUL written as an escape":  "k: \"\\x00\"\n",
		"text outside ASCII":          "k: héllo\n",
		"a character outside the BMP": "k: 😀\n",
		"a replacement character":     "k: \uFFFD\n",
		"a next-line character":       "k: \u0085\n",
		"a non-breaking space":        "k: \u00a0\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}
}

// TestParseDropsAByteOrderMarkOpeningTheStream checks that a byte order mark at the head of the stream is dropped.
//
// nb-char is c-printable less b-char and c-byte-order-mark, so no node may hold a mark:
// it marks an l-document-prefix and nothing else.
// Read as an ordinary character, it would join the next token: the first key, or the '%' of a directive.
func TestParseDropsAByteOrderMarkOpeningTheStream(t *testing.T) {
	const mark = "\ufeff"

	tests := map[string]struct {
		source string
		want   string
	}{
		"before a mapping key":     {mark + "a: 1\n", "a: 1\n"},
		"before a document marker": {mark + "---\na: 1\n", "---\na: 1\n"},
		"before a comment":         {mark + "# c\na: 1\n", "# c\na: 1\n"},
		"before a directive":       {mark + "%YAML 1.2\n---\na: 1\n", "%YAML 1.2\n---\na: 1\n"},

		// l-yaml-stream opens on any number of l-document-prefix, so two marks in a row are both dropped.
		"twice over": {mark + mark + "a: 1\n", "a: 1\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoErrorf(t, err, "rejected %q", test.source)
			assert.Equal(t, test.want, file.String())
		})
	}
}

// TestParseTabWhereIndentationBelongs checks a tab opening a line at the root.
//
// s-indent(n) is n spaces, so only spaces introduce block structure, and "\tfoo: 1" is invalid.
// A tab is separation, though, and a flow node or a scalar at the root follows s-separate, so "\t{}" is valid.
//
// The quoted-key cases matter because a quoted scalar resets the origin buffer that the check reads.
func TestParseTabWhereIndentationBelongs(t *testing.T) {
	invalid := map[string]string{
		"a tab before a quoted key at the root": "\t\"\": a\n",
		"a tab before a single-quoted key":      "\t'k': v\n",
		"a tab before a plain key at the root":  "\tfoo: a\n",
		"a tab before a sequence at the root":   "\t- a\n",
		"a tab before a key, further entries":   "\ta: 1\nb: 2\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}

	// The same tab in front of something reached through s-separate.
	valid := map[string]string{
		"a tab before a flow mapping":     "\t{}\n",
		"a tab before a flow sequence":    "\t[\n\t]\n",
		"a tab before a flow pair":        "\t{a: 1}\n",
		"a tab before a plain scalar":     "\tfoo\n",
		"a tab before a quoted scalar":    "\t\"foo\"\n",
		"a tab before a comment":          "\t#c\n",
		"a tab after a space of indent":   "foo:\n \tbar\n",
		"a tab inside a folded plain one": "x:\n - x\n  \tx\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}
}

// TestParseWhitespaceOnlyLines checks that a line holding only white space is a blank line, whatever it holds.
//
// A tab cannot be indentation, but a line holding only a tab has no indentation to reject:
// it is a gap between the entries around it.
func TestParseWhitespaceOnlyLines(t *testing.T) {
	valid := map[string]string{
		"a line holding one tab":            "foo: 1\n\t\nbar: 2\n",
		"a line mixing tabs and spaces":     "foo: 1\n\t \t\nbar: 2\n",
		"a tab line inside a nested map":    "foo:\n  a: 1\n\t\n  b: 2\n",
		"a tab line between sequence items": "- a\n\t\n- b\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}

	// A tab standing in for indentation is still rejected.
	invalid := map[string]string{
		"a tab before a mapping entry":  "foo: 1\n\tbar: 2\n",
		"a tab before a sequence entry": "foo:\n\t- a\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}
}

// TestParseValueMustBeIndentedPastItsKey checks that a value written level with its key is rejected.
//
// A value goes further in than its key. Level with the key, a token can only open the next entry:
// another key, or the '-' of a block sequence, which by convention sits at its key's column.
// Any other token there belongs nowhere.
func TestParseValueMustBeIndentedPastItsKey(t *testing.T) {
	invalid := map[string]string{
		"a plain scalar level with the key": "a:\nb\n",
		"a quoted key, same":                "\"a\":\nb\n",
		"a flow collection level with it":   "a:\n[1, 2]\n",
		"an alias level with it":            "k: &x 1\na:\n*x\n",
		"nested one level in":               "top:\n  a:\n  b\n",

		// A property on the key's line names a block node, which must be indented past the key like any other value.
		"an anchor on the key's line":    "k: &a\n1\n",
		"a tag on the key's line":        "k: !!str\n1\n",
		"an anchor under an empty key":   ": &a\n1\n",
		"an anchor, nested one level in": "top:\n  k: &a\n  1\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}

	valid := map[string]string{
		"the value indented past the key":      "a:\n  b\n",
		"a block sequence at the key's column": "a:\n- x\n",
		"the next entry":                       "a:\nb: 1\n",
		"an empty value at the end":            "a:\n",
		"a comment between them":               "a:\n# c\nb: 1\n",
		"a new document":                       "a:\n---\nb: 1\n",

		// The same property with its node where a node belongs.
		"an anchor naming an indented value":     "k: &a\n  1\n",
		"an anchor naming a sequence":            "k: &a\n- 1\n",
		"an anchor under an empty key, indented": ": &a\n  1\n",
		"an anchor naming nothing at all":        ": &a\n",
		"an anchor before the next entry":        "k: &a\nnext: 1\n",
		"an anchor before a new document":        "k: &a\n---\nb: 1\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}
}
