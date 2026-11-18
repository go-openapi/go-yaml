// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestTagResolvesToItsURI records that a tag is read by the URI it names and
// not by the shorthand it was written with.
//
// "!!int" is the secondary handle plus the suffix "int", and the secondary
// handle stands for tag:yaml.org,2002: unless a "%TAG" line says otherwise
// (§6.8.2.2). So "!!int", "!<tag:yaml.org,2002:int>" and "!e!int" under
// "%TAG !e! tag:yaml.org,2002:" are three spellings of one tag.
func TestTagResolvesToItsURI(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shorthand", `a: !!int "12"` + "\n", `{"a":12}`},
		{"verbatim", "a: !<tag:yaml.org,2002:int> \"12\"\n", `{"a":12}`},
		{"named handle", "%TAG !e! tag:yaml.org,2002:\n---\na: !e!int \"12\"\n", `{"a":12}`},
		{"primary handle repointed", "%TAG ! tag:yaml.org,2002:\n---\na: !int \"12\"\n", `{"a":12}`},
		{"secondary handle restated", "%TAG !! tag:yaml.org,2002:\n---\na: !!int \"12\"\n", `{"a":12}`},

		// ⚠️ And the other way: a "%TAG !!" line takes the secondary handle out
		// of YAML's namespace, so "!!int" names the document's own tag. Nothing
		// resolves it, and the scalar keeps the text it was written with.
		{"secondary handle repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", `{"a":"12"}`},
		{"repointed on a collection", "%TAG !! tag:example.com,2020:\n---\na: !!seq [1,2]\n", `{"a":[1,2]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))

			var decoded any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			asJSON, err := json.Marshal(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(asJSON), "the decoder reads the same tag")
		})
	}
}

// TestTagNodeCarriesTheURI records that the expansion is on the node, so a
// reader asks what a tag means rather than what it says.
func TestTagNodeCarriesTheURI(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shorthand", "a: !!int 12\n", "tag:yaml.org,2002:int"},
		{"verbatim", "a: !<tag:yaml.org,2002:int> 12\n", "tag:yaml.org,2002:int"},
		{"local", "a: !thing 12\n", "!thing"},
		{"named handle", "%TAG !e! tag:example.com,2020:\n---\na: !e!thing 12\n", "tag:example.com,2020:thing"},
		{"secondary repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", "tag:example.com,2020:int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			var found []string
			for _, doc := range file.Docs {
				collectTagURIs(doc, &found)
			}
			assert.Equal(t, []string{tc.want}, found)
		})
	}
}

// TestUndeclaredTagHandleIsRefused records that "!name!" needs a "%TAG" line.
// The primary and secondary handles never do.
func TestUndeclaredTagHandleIsRefused(t *testing.T) {
	_, err := parser.ParseBytes([]byte("a: !x!thing 12\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag handle !x! is not defined by a TAG directive")
}

// collectTagURIs appends the URI of every ast.TagNode under n, in the order
// they are reached.
func collectTagURIs(n ast.Node, out *[]string) {
	ast.Walk(uriCollector{out: out}, n)
}

type uriCollector struct{ out *[]string }

func (c uriCollector) Visit(n ast.Node) ast.Visitor {
	if tag, ok := n.(*ast.TagNode); ok {
		*c.out = append(*c.out, tag.URI)
	}

	return c
}
