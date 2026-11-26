// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAFlowCollectionStandsAsAFlowMappingKey holds two documents yamlgen.Strict recorded as refused, which the parser
// reads.
//
// 7.4.2 lets a flow mapping entry be a key with no value, and its key be any flow node: "{{\"\": 0}}" is one entry
// keyed on the mapping {"": 0}. A flow mapping's key is under neither implicit-key restriction either, so it may span
// lines: "{[a" over "b]: 1}" is one entry keyed on [a, b]. Only a flow sequence entry has to keep its key on one line.
//
// The second is a ruling. grammar.NewRecognizer and yaml/yaml-reference-parser, both generated from the
// specification's grammar, accept it; libfyaml 1.0.0rc2 refuses it with "missing comma in flow mapping" and
// go.yaml.in/yaml/v3 v3.0.6 with "did not find expected ',' or '}'", both in their parsers and not in building a
// value. This library reads it as the grammar does. The long form, "{? [a" over "b]" over ": 1}", every one of the
// four reads -- libfyaml's "unhashable type" comes after the parse, from its binding -- and this library refused it
// with "a flow sequence needs ',' or ']' after an entry" until a flow scalar could run onto a line left of the last ':'.
//
// Neither decodes into an any: Go cannot key a map on a collection, which is the decoder's mapping model and not the
// syntax.
func TestAFlowCollectionStandsAsAFlowMappingKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"mapping-as-a-key-with-no-value", "{{\"\": 0}}\n", "{{\"\": 0}:}\n"},
		{"sequence-key-spanning-two-lines", "{[a\nb]: 1}\n", "{[a b]: 1}\n"},
		{"explicit-sequence-key-spanning-two-lines", "{? [a\nb]\n: 1}\n", "{? [a b] : 1}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, file.String())
		})
	}
}

// TestAFlowCollectionEndingOnAnAliasTakesAnAdjacentColon holds a flow collection key whose last item is an alias, with
// the ':' written straight after the closing bracket.
//
// c-ns-flow-map-adjacent-value lets a ':' follow a JSON-like key with no space, and a flow collection is one. The scan
// took that ':' for a character of the alias's name, because the name ended on the ']' and the alias was still marked
// as being read: "{[*m]:, c: 1}" was refused with "a flow mapping needs ',' or '}' after an entry", and "{[*m] :, c: 1}" read. A ':'
// right after a name stays part of it, so "{&a: b}" still anchors a node named "a:". yaml/yaml-reference-parser reads
// each document below, and go.yaml.in/yaml/v3 parses each one and refuses only the sequence as a Go map key.
func TestAFlowCollectionEndingOnAnAliasTakesAnAdjacentColon(t *testing.T) {
	for _, source := range []string{
		"a: &m x\nb: {[*m]:, c: 1}\n",
		"a: &m x\nb: {[*m]: y}\n",
		"a: &m x\nb: {[*m]:}\n",
		"a: &m x\nb: {[a, *m]: y}\n",
		"a: &m x\nb: {{k: *m}: y}\n",
		"a: &m x\nb: [[*m]: y]\n",
	} {
		t.Run(source, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source))
			require.NoError(t, err)

			reread, err := parser.ParseBytes([]byte(file.String()))
			require.NoErrorf(t, err, "cannot read back %q", file.String())
			assert.Equal(t, file.String(), reread.String())
		})
	}

	t.Run("a colon after a name is part of it", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte("{&a: b}\n"))
		require.NoError(t, err)
		assert.Contains(t, file.String(), "&a:")
	})
}
