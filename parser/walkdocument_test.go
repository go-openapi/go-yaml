// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestStepDocumentNumbersTheStream checks which document of a stream a walk
// says each node belongs to.
//
// A walk hands the nodes of every document over in one run, and a stream's
// documents are independent: an anchor and a "%YAML" directive are both scoped
// to one. Step.Document is what tells them apart.
//
// An empty document hands over no node at all, so the count has to move where
// the document is read rather than where its first node is. That is the whole
// point of the field: "---" over "---" over "b: 2" hands over one mapping, and
// it belongs to document 1. codec.ToJSON counted the bodies it saw instead and
// converted that mapping as though it were the first document.
func TestStepDocumentNumbersTheStream(t *testing.T) {
	t.Parallel()

	for tc := range walkDocumentTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := &documentSpy{}
			_, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte(tc.src), v)
			require.NoErrorf(t, err, "%q", tc.src)

			assert.Equal(t, tc.want, v.bodies, "%q: the document each body belongs to", tc.src)
		})
	}

	t.Run("the count indexes the documents the walk returns", func(t *testing.T) {
		t.Parallel()

		// Step.Document and File.Docs have to agree, or a caller holding both
		// cannot line them up.
		const src = "%YAML 1.2\n---\na: 1\n---\n---\nc: 3\n"

		v := &documentSpy{}
		f, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte(src), v)
		require.NoError(t, err)

		assert.Len(t, f.Docs, 4, "a directive, its document, an empty one, and c")
		assert.Equal(t, []int{0, 1, 3}, v.bodies, "the empty document hands over nothing")
	})

	t.Run("everything a document holds carries the document's own number", func(t *testing.T) {
		t.Parallel()

		v := &everyStep{}
		_, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte("---\n---\nb: [1, 2]\n"), v)
		require.NoError(t, err)

		require.NotEmpty(t, v.seen, "the mapping and what it holds went over")
		for _, got := range v.seen {
			assert.Equal(t, 1, got, "a node nested in document 1 belongs to document 1")
		}
	})
}

type walkDocumentTestCase struct {
	name, src string
	want      []int
}

func walkDocumentTestCases() iter.Seq[walkDocumentTestCase] {
	return slices.Values([]walkDocumentTestCase{
		{
			name: "an empty first document still counts",
			src:  "---\n---\nb: 2\n",
			want: []int{1},
		},
		{
			name: "an explicit null is a node of document 0",
			src:  "---\nnull\n---\nb: 2\n",
			want: []int{0, 1},
		},
		{
			name: "two documents with bodies",
			src:  "a: 1\n---\nb: 2\n",
			want: []int{0, 1},
		},
		{
			name: "an empty document between two others",
			src:  "a: 1\n---\n---\nc: 3\n",
			want: []int{0, 2},
		},
		{
			// The count indexes the File's Docs, and a directive line is an
			// entry of those, so "a: 1" is document 1 rather than document 0.
			// A caller that means "the first document holding a value" has to
			// step past the directives itself, which is what codec.ToJSON does.
			name: "a directive is a document of its own",
			src:  "%YAML 1.2\n---\na: 1\n---\nb: 2\n",
			want: []int{0, 1, 2},
		},
		{
			// Each directive line is a document of its own, so two of them put
			// the mapping at 2. codec.ToJSON steps past them one at a time.
			name: "two directive lines are two documents",
			src:  "%YAML 1.2\n%TAG !e! tag:yaml.org,2002:\n---\na: 1\n",
			want: []int{0, 1, 2},
		},
		{
			name: "one document",
			src:  "a: 1\n",
			want: []int{0},
		},
	})
}

// documentSpy records the document each top-level body belongs to.
type documentSpy struct {
	bodies []int
}

func (d *documentSpy) Enter(_ ast.Node, at parser.Step) bool {
	if at.Depth == 0 && at.In == parser.KindNone {
		d.bodies = append(d.bodies, at.Document)
	}

	return true
}

func (d *documentSpy) Leave(ast.Node, parser.Step) {}

// everyStep records the document of every node handed over, at any depth.
type everyStep struct {
	seen []int
}

func (e *everyStep) Enter(_ ast.Node, at parser.Step) bool {
	e.seen = append(e.seen, at.Document)

	return true
}

func (e *everyStep) Leave(ast.Node, parser.Step) {}
