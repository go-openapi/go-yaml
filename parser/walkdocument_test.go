// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"errors"
	"iter"
	"slices"
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestStepDocumentNumbersTheStream checks the document number a walk gives each node.
//
// A walk hands the nodes of every document over in one run, and a stream's documents are independent:
// an anchor and a "%YAML" directive each apply to one. Step.Document tells them apart.
//
// An empty document hands over no node, so the count moves when the document is read, not at its first node:
// "---" over "---" over "b: 2" hands over one mapping, and it belongs to document 1.
// Counting the bodies handed over would put that mapping in document 0.
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

		// Step.Document indexes File.Docs, so a caller holding both can line them up.
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
			// The count indexes the File's Docs, and a directive line is one of them,
			// so "a: 1" is document 1, not document 0.
			// To find the first document holding a value, a caller steps past the directives, as codec.ToJSON does.
			name: "a directive is a document of its own",
			src:  "%YAML 1.2\n---\na: 1\n---\nb: 2\n",
			want: []int{0, 1, 2},
		},
		{
			// Each directive line is a document of its own, so two of them put the mapping at 2.
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

func (d *documentSpy) Enter(_ ast.Node, at parser.Cursor) error {
	if at.Depth() == 0 && at.In() == parser.KindNone {
		d.bodies = append(d.bodies, at.Document())
	}

	return nil
}

func (d *documentSpy) Leave(ast.Node, parser.Closing) error { return nil }

// everyStep records the document of every node handed over, at any depth.
type everyStep struct {
	seen []int
}

func (e *everyStep) Enter(_ ast.Node, at parser.Cursor) error {
	e.seen = append(e.seen, at.Document())

	return nil
}

func (e *everyStep) Leave(ast.Node, parser.Closing) error { return nil }

// TestLeaveDocumentEndsEachDocument checks that a DocumentVisitor hears of every document of the stream once it is
// read, in order, between the last Leave of one document and the first Enter of the next.
//
// An empty document hands no node over, so without LeaveDocument a visitor cannot see it at all, and sees the end
// of any other document only once the next one hands its root over.
func TestLeaveDocumentEndsEachDocument(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{
			name: "two documents",
			src:  "a: 1\n---\nb: 2\n",
			want: []string{"enter 0", "leave 0", "document 0", "enter 1", "leave 1", "document 1"},
		},
		{
			name: "an empty document between two",
			src:  "a: 1\n---\n---\nb: 2\n",
			want: []string{"enter 0", "leave 0", "document 0", "document 1", "enter 2", "leave 2", "document 2"},
		},
		{
			name: "a directive line is a document of its own",
			src:  "%YAML 1.2\n---\na: 1\n",
			want: []string{"enter 0", "leave 0", "document 0", "enter 1", "leave 1", "document 1"},
		},
		{
			name: "an empty source holds one document",
			src:  "",
			want: []string{"document 0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := &documentEnds{}
			f, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte(tc.src), v)
			require.NoError(t, err)

			assert.Equal(t, tc.want, v.events)
			assert.Len(t, v.docs, len(f.Docs), "one call for each document the walk returns")
			for i, doc := range v.docs {
				assert.Same(t, f.Docs[i], doc, "document %d", i)
			}
		})
	}
}

// TestLeaveDocumentStopsTheWalk checks the two ways LeaveDocument ends a walk: ErrStopWalk after the document,
// with no error, and any other error, which Walk returns.
func TestLeaveDocumentStopsTheWalk(t *testing.T) {
	t.Parallel()

	// The second document would be refused, so a walk that read it would fail.
	const src = "a: 1\n---\nb: [\n"

	t.Run("ErrStopWalk ends the walk after the document", func(t *testing.T) {
		t.Parallel()

		v := &documentEnds{stopAt: 0, stop: parser.ErrStopWalk}
		f, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte(src), v)
		require.NoError(t, err, "the parse never reads the broken document")

		require.NotNil(t, f)
		assert.Len(t, f.Docs, 1, "the file ends with the document the walk stopped after")
		assert.Equal(t, []string{"enter 0", "leave 0", "document 0"}, v.events)
	})

	t.Run("an error stops the walk and Walk returns it", func(t *testing.T) {
		t.Parallel()

		v := &documentEnds{stopAt: 0, stop: errLeftTheDocument}
		_, err := parser.New(parser.WithOmitNodePaths()).Walk([]byte("a: 1\n---\nb: 2\n"), v)
		require.ErrorIs(t, err, errLeftTheDocument)

		assert.Equal(t, []string{"enter 0", "leave 0", "document 0"}, v.events,
			"nothing more is handed over, and LeaveDocument is not called again")
	})
}

var errLeftTheDocument = errors.New("left the document")

// documentEnds records the roots it is handed and the documents it is told have ended, in order. It returns stop
// from the LeaveDocument of document stopAt, when stop is set.
type documentEnds struct {
	events []string
	docs   []*ast.DocumentNode
	stopAt int
	stop   error
}

func (d *documentEnds) Enter(_ ast.Node, at parser.Cursor) error {
	if at.IsRoot() {
		d.events = append(d.events, "enter "+strconv.Itoa(at.Document()))
	}

	return nil
}

func (d *documentEnds) Leave(_ ast.Node, at parser.Closing) error {
	if at.IsRoot() {
		d.events = append(d.events, "leave "+strconv.Itoa(at.Document()))
	}

	return nil
}

func (d *documentEnds) LeaveDocument(doc *ast.DocumentNode, n int) error {
	d.events = append(d.events, "document "+strconv.Itoa(n))
	d.docs = append(d.docs, doc)
	if d.stop != nil && n == d.stopAt {
		return d.stop
	}

	return nil
}
