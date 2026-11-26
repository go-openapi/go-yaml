// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yamlparser "github.com/go-openapi/go-yaml/parser"
)

// messagePin holds a document that provokes one parser message, and the options it is parsed with.
type messagePin struct {
	src  string
	opts []yamlparser.Option
	says string
}

// messagePins provokes the parser messages that neither the corpus nor yamlbuild.Refusals reaches.
//
// yamlbuild.Refusals lives in conformance-suites, so a message pinned for go-yaml's own sake is pinned here.
// TestTheParserVocabularyGapIsMeasured counts these with the corpus, and TestEveryMessagePinSaysWhy holds each
// document to its message.
var messagePins = []messagePin{ //nolint:gochecknoglobals // a fixed table shared by two tests
	// A '\uD800' high surrogate needs a '\u' low surrogate after it, and there is room for one.
	{src: "\"\\uD800abcdefgh\"\n", says: "found unexpected character after high surrogate for UTF-16 surrogate pair"},
	// A line of a quoted scalar holding only a tab, in block context: the entry's content needs a space of
	// indentation before any tab. yaml/yaml-reference-parser and libfyaml refuse both.
	{src: "- \"a\n\t\n\"\n", says: "tab character cannot be used for indentation in double-quoted text"},
	{src: "- 'a\n\t\n'\n", says: "tab character cannot be used for indentation in single-quoted text"},
	// Content after "..." on its line: the marker ends the document, and nothing may follow it there.
	{src: "a\n... x\n", says: "unexpected end content"},
	// WithJSONSyntax holds the source to RFC 8259, which admits no raw control character.
	{
		src: "{\"a\": \"\x01\"}", opts: []yamlparser.Option{yamlparser.WithJSONSyntax()},
		says: "found character '\\x01' that JSON text may not hold raw",
	},
}

func TestEveryMessagePinSaysWhy(t *testing.T) {
	for _, pin := range messagePins {
		_, err := yamlparser.ParseBytes([]byte(pin.src), pin.opts...)
		require.Errorf(t, err, "%q", pin.src)
		assert.Containsf(t, err.Error(), pin.says, "%q", pin.src)
	}
}
