// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestCommentDensity reports how much of a workload is comment.
//
// Six of the seven hold none: they were rewritten from JSON, which has no
// comment to carry over. Every other measurement in this package therefore
// reports a parse that dropped comments at the door, and commented_swagger is
// the one workload that does not. Run with -v for the table.
func TestCommentDensity(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	t.Logf("%-19s %8s %9s %8s %6s %11s", "workload", "tokens", "comments", "share", "line", "standalone")

	for _, w := range all {
		tokens := tokenize(t, string(w.Data))

		var comments, line int
		for i, tk := range tokens {
			if tk.Type != token.CommentType {
				continue
			}
			comments++
			if i > 0 && tokens[i-1].Position.Line == tk.Position.Line {
				line++
			}
		}

		t.Logf("%-19s %8d %9d %7.2f%% %6d %11d",
			w.Name, len(tokens), comments,
			100*float64(comments)/float64(len(tokens)), line, comments-line)
	}
}

// TestCommentCost measures what ParseComments retains.
//
// A comment costs more than the token it arrives as. The token stays in
// rawTokens, the grouping records a line comment in a map keyed by the token it
// closes -- one that nothing ever deletes from -- and the parse builds a
// CommentGroupNode and a CommentNode for each, neither of them from the arena.
//
// The three rows separate the source from the mode: the same document read both
// ways shows what the comments cost, and azure_swagger shows that a document
// with none is unaffected either way.
func TestCommentCost(t *testing.T) {
	plain, err := workloads.ByName("azure_swagger")
	require.NoError(t, err)

	annotated, err := workloads.ByName("commented_swagger")
	require.NoError(t, err)

	// The annotated document read both ways comes first: the second row is what
	// the comments cost. azure_swagger follows to show a document holding none
	// is unaffected by the mode.
	rows := []struct {
		label   string
		data    []byte
		opts    []parser.Option
		against bool
	}{
		{"commented, dropped", annotated.Data, nil, false},
		{"commented, parsed", annotated.Data, []parser.Option{parser.WithComments()}, true},
		{"azure_swagger", plain.Data, []parser.Option{parser.WithComments()}, false},
	}

	var base uint64
	for _, row := range rows {
		data, opts := row.data, row.opts

		retained := retainedBytes(t, func() any {
			file, err := parser.ParseBytes(data, opts...)
			require.NoError(t, err)

			return file
		})
		if base == 0 {
			base = retained
		}

		var delta string
		if row.against {
			delta = fmt.Sprintf("%+d B for %d comments", int64(retained)-int64(base), commentCount(t, annotated.Data))
		}

		t.Logf("%-20s %6d bytes of source  %9d retained  %s", row.label, len(data), retained, delta)
	}
}

// commentCount is how many comment tokens a document holds.
func commentCount(t *testing.T, data []byte) int {
	t.Helper()

	var n int
	for _, tk := range tokenize(t, string(data)) {
		if tk.Type == token.CommentType {
			n++
		}
	}

	return n
}

// TestCommentShapes counts the comments the parse attaches, by what holds them.
//
// Two of the three outlive the node they read as belonging to, which is what
// makes them worth counting separately:
//
// A foot comment is read after the block it belongs to has closed, and written
// into an entry the parser had already finished. A caller consuming entries as
// they complete cannot be handed the last entry of a block until the token
// after it settles whether a foot comment attaches.
//
// A head comment on a sequence entry is filed in ValueHeadComments, a slice on
// the sequence indexed alongside Values, and again on the SequenceEntryNode.
// The sequence holds it either way until the sequence itself is done.
func TestCommentShapes(t *testing.T) {
	w, err := workloads.ByName("commented_swagger")
	require.NoError(t, err)

	file, err := parser.ParseBytes(w.Data, parser.WithComments())
	require.NoError(t, err)

	var c commentCensus
	for _, doc := range file.Docs {
		ast.Walk(&c, doc)
	}

	require.NotZero(t, c.foot, "the workload no longer holds a foot comment")
	require.NotZero(t, c.attached, "the workload no longer holds an attached comment")
	require.NotZero(t, c.valueHead, "the workload no longer holds a sequence entry head comment")

	t.Logf("%d foot comments, %d in ValueHeadComments, %d nodes carrying a comment",
		c.foot, c.valueHead, c.attached)
}

// commentCensus counts comments by where the parse put them.
type commentCensus struct {
	foot      int
	valueHead int
	attached  int
}

func (c *commentCensus) Visit(n ast.Node) ast.Visitor {
	switch t := n.(type) {
	case *ast.MappingValueNode:
		if t.FootComment != nil {
			c.foot++
		}
	case *ast.MappingNode:
		if t.FootComment != nil {
			c.foot++
		}
	case *ast.SequenceNode:
		if t.FootComment != nil {
			c.foot++
		}
		for _, comment := range t.ValueHeadComments {
			if comment != nil {
				c.valueHead++
			}
		}
	}

	if n.GetComment() != nil {
		c.attached++
	}

	return c
}
