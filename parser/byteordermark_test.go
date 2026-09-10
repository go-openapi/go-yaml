// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// byteOrderMark is U+FEFF, written as an escape because Go refuses one in the
// source text of a file.
const byteOrderMark = "\ufeff"

// TestAByteOrderMarkOpensAPrefixOrNothing holds where 5.2 admits the mark.
//
// l-document-prefix is `c-byte-order-mark? l-comment*` and l-yaml-stream takes
// those prefixes one after another, so a run of marks opens a stream and
// anything else in front of one puts it inside a line, where nb-char excludes
// it.
//
// checkByteOrderMark asked Scanner.column, and the column does not answer this:
// a space advances it through progressColumn and a tab does not, since the tab
// branch of the scan loop calls progress, which moves the cursor and leaves the
// column alone. So " \ufeff" was refused and "\t\ufeff" was read -- and read as
// nothing, with no error and no content, so a caller could not tell that
// document from an empty one. Of the three implementations ours was the only
// one that lost the byte: go.yaml.in/yaml/v3 v3.0.5 refuses it and libfyaml
// 1.0.0b1 reads the mark as the document's content.
//
// Context.opensADocumentPrefix reads the source instead, which is the question
// the production asks.
func TestAByteOrderMarkOpensAPrefixOrNothing(t *testing.T) {
	t.Run("anything in front of it on the line refuses it", func(t *testing.T) {
		for _, src := range []string{
			"\t" + byteOrderMark + "\n",
			"\t" + byteOrderMark + "a: 1\n",
			" " + byteOrderMark + "\n",
			" " + byteOrderMark + "a: 1\n",
			"a: " + byteOrderMark + "b\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(),
				"found a byte order mark inside a line, where a node may not hold one", "%q", src)
		}
	})

	t.Run("a mark opening the stream reads, and so does a run of them", func(t *testing.T) {
		// grammar.NewRecognizer accepts each of these and so does
		// go.yaml.in/yaml/v3. A run is a run of prefixes, which is why
		// documentOpensAtMark steps over one.
		for _, src := range []string{
			byteOrderMark + "\n",
			byteOrderMark + "a: 1\n",
			byteOrderMark + "---\na: 1\n",
			byteOrderMark + byteOrderMark + "a: 1\n",
			byteOrderMark + byteOrderMark + "\n",
			"a: 1\n...\n" + byteOrderMark + "---\nb: 2\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("and one where no document begins is still refused", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("a: 1\n" + byteOrderMark + "b: 2\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "found a byte order mark where no document begins")
	})

	t.Run("a mark inside a quoted scalar is content", func(t *testing.T) {
		// nb-json takes it like any other character, so the quoted readers keep
		// it and never reach checkByteOrderMark.
		_, err := parser.ParseBytes([]byte("a: \"x" + byteOrderMark + "y\"\n"))
		assert.NoError(t, err)
	})
}
