// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
)

// TestPropertiesComposeIntoOneNode parses one document for each state the
// property machine passes through, and holds the tree each one builds.
//
// group.propState has eight states, and stageProperties joins an anchor, an
// alias and a tag standing before a node into the one node they belong to. Each
// state records how much of that has been read: "a: &x !!str 1" builds an anchor
// over a tag over a scalar, and "a: !!str &x 1" builds a tag over an anchor over
// the same scalar, because the tag tags what the anchor names.
//
// Each case names the node type the value composes to and the text the whole
// document renders back to. Seven of the eight round-trip byte for byte; a tag
// alone on its line is written back with the mapping indented under it.
func TestPropertiesComposeIntoOneNode(t *testing.T) {
	t.Parallel()

	for c := range propertyTestCases() {
		t.Run(c.state, func(t *testing.T) {
			t.Parallel()

			f, err := New().Parse([]byte(c.src))
			require.NoError(t, err)
			require.Len(t, f.Docs, 1)

			body := f.Docs[0].Body
			require.NotNil(t, body)
			assert.IsTypef(t, c.body, body, "the document body is a %T", body)
			assert.Equal(t, c.want, f.String())

			values := entryValues(body)
			require.Lenf(t, values, len(c.values), "the mapping holds %d entries", len(values))
			for i, want := range c.values {
				assert.IsTypef(t, want, values[i], "entry %d holds a %T", i, values[i])
			}
		})
	}
}

// propertyCase is a document that drives the property machine into one state,
// and what the parse must build from it.
type propertyCase struct {
	// state names the group.propState the document reaches. That package's
	// states are unexported, so the name is written out here.
	state  string
	src    string
	want   string
	body   ast.Node
	values []ast.Node
}

func propertyTestCases() iter.Seq[propertyCase] {
	return slices.Values([]propertyCase{
		{
			state:  "propNone",
			src:    "a: 1\n",
			want:   "a: 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.IntegerNode{}},
		},
		{
			state:  "propSawAnchor, propHaveAnchor",
			src:    "a: &x 1\n",
			want:   "a: &x 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.AnchorNode{}},
		},
		{
			state:  "propSawAlias",
			src:    "a: &x 1\nb: *x\n",
			want:   "a: &x 1\nb: *x\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.AnchorNode{}, &ast.AliasNode{}},
		},
		{
			state:  "propSawTag",
			src:    "a: !!str 1\n",
			want:   "a: !!str 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.TagNode{}},
		},
		{
			// The anchor is read first and holds the tag, so it is outermost.
			state:  "propAnchorAndTag",
			src:    "a: &x !!str 1\n",
			want:   "a: &x !!str 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.AnchorNode{}},
		},
		{
			// The tag tags what the anchor names, so the tag is outermost here
			// and the two documents build different trees from the same three
			// tokens.
			state:  "propTagSawAnchor, propTagHaveAnchor",
			src:    "a: !!str &x 1\n",
			want:   "a: !!str &x 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.TagNode{}},
		},
		{
			// An anchor with nothing after it names the empty node, and the
			// entry below is a second entry and not what the anchor names.
			state:  "propHaveAnchor over the empty node",
			src:    "a: &x\nb: 1\n",
			want:   "a: &x\nb: 1\n",
			body:   &ast.MappingNode{},
			values: []ast.Node{&ast.AnchorNode{}, &ast.IntegerNode{}},
		},
		{
			// A tag on its own line tags the whole mapping under it, so the
			// body is the tag and the mapping hangs off it.
			state: "propSawTag over a collection",
			src:   "!!map\na: 1\n",
			want:  "!!map\n  a: 1\n",
			body:  &ast.TagNode{},
		},
	})
}

// entryValues returns the value of each entry of a mapping body, in the order
// the document wrote them. A body that is not a mapping has none.
func entryValues(body ast.Node) []ast.Node {
	m, isMapping := body.(*ast.MappingNode)
	if !isMapping {
		return nil
	}

	values := make([]ast.Node, 0, len(m.Values))
	for _, v := range m.Values {
		values = append(values, v.Value)
	}

	return values
}
