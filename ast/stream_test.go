// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// errNotStreamable marks a document the test cannot hand to a Stream: a collection key, or a node a Stream takes no
// event for.
var errNotStreamable = errors.New("not streamable")

// streamNode hands n to s as events. It also sets every collection of n to block style, so that the tree renders as
// the stream does.
func streamNode(s *ast.Stream, n ast.Node) error {
	switch node := n.(type) {
	case *ast.MappingNode:
		node.SetIsFlowStyle(false)
		if err := s.BeginMapping(); err != nil {
			return err
		}
		for _, mv := range node.Values {
			if err := streamEntry(s, mv); err != nil {
				return err
			}
		}

		return s.End()
	case *ast.SequenceNode:
		node.SetIsFlowStyle(false)
		if err := s.BeginSequence(); err != nil {
			return err
		}
		for _, v := range node.Values {
			if err := streamNode(s, v); err != nil {
				return err
			}
		}

		return s.End()
	case *ast.AnchorNode, *ast.AliasNode, *ast.TagNode, *ast.MergeKeyNode, *ast.MappingKeyNode,
		*ast.MappingValueNode, *ast.CommentGroupNode, *ast.DocumentNode:
		return errNotStreamable
	default:
		return s.Value(n)
	}
}

// noBlankLines clears the blank line recorded above every token of a tree, the "- " tokens included. A Stream writes
// none, and the Renderer would keep them.
type noBlankLines struct{}

func (noBlankLines) Visit(n ast.Node) ast.Visitor {
	if tk := n.GetToken(); tk != nil {
		tk.SetBlankLineAbove(false)
	}
	if seq, ok := n.(*ast.SequenceNode); ok {
		for _, e := range seq.Entries {
			if e != nil && e.Start != nil {
				e.Start.SetBlankLineAbove(false)
			}
		}
	}

	return noBlankLines{}
}

func streamEntry(s *ast.Stream, mv *ast.MappingValueNode) error {
	// An explicit "? " key stays explicit in the tree, whatever its length, and a Stream writes "? " only for a key
	// past 1,024 characters. The two would differ by construction.
	var key ast.Node = mv.Key
	switch key.(type) {
	case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.AliasNode, *ast.TagNode, *ast.MergeKeyNode,
		*ast.MappingKeyNode, nil:
		return errNotStreamable
	default:
	}
	if err := s.Key(key); err != nil {
		return err
	}

	return streamNode(s, mv.Value)
}

// TestAStreamWritesWhatTheRendererWrites hands every YAML Test Suite document and every fuzz seed to a Stream, a node
// at a time, and compares the output with what Renderer.Render writes for the same tree in block style, under four
// settings. A document holding a collection key, an explicit "? " key, an anchor, a tag, an alias or a merge key is
// left out: a Stream takes none of them.
func TestAStreamWritesWhatTheRendererWrites(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	srcs := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, string(s.InYAML))
	}
	srcs = append(srcs, seeds...)

	for _, cfg := range []struct {
		name string
		opts []ast.RenderOption
	}{
		{"default", nil},
		{"indent4", []ast.RenderOption{ast.WithIndent(4)}},
		{"indentseq", []ast.RenderOption{ast.WithIndentSequence(true)}},
		{"indent3seq", []ast.RenderOption{ast.WithIndent(3), ast.WithIndentSequence(true)}},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			r := ast.NewRenderer(append([]ast.RenderOption{ast.WithComments(false)}, cfg.opts...)...)

			var compared, differed int
			for _, src := range srcs {
				file, err := parser.ParseBytes([]byte(src))
				if err != nil {
					continue
				}
				for _, doc := range file.Docs {
					streamed, want, ok := streamAndRender(t, r, doc.Body)
					if !ok {
						continue
					}
					compared++
					if streamed != want {
						differed++
						if differed <= 3 {
							t.Errorf("the stream wrote\n%s\nthe renderer wrote\n%s\nfor\n%s", streamed, want, src)
						}
					}
				}
			}
			if differed > 3 {
				t.Errorf("%d of %d documents differ", differed, compared)
			}
			t.Logf("%d documents compared", compared)
			assert.Greater(t, compared, 5000)
		})
	}
}

// streamAndRender writes body through a Stream and through Renderer.Render, and returns both. It returns false for a
// body a Stream takes no events for.
func streamAndRender(t *testing.T, r *ast.Renderer, body ast.Node) (string, string, bool) {
	t.Helper()

	if body == nil {
		return "", "", false
	}
	ast.Walk(noBlankLines{}, body)

	var streamed strings.Builder
	s := r.Stream(&streamed)
	if err := streamNode(s, body); errors.Is(err, errNotStreamable) {
		return "", "", false
	} else if err != nil {
		t.Fatalf("stream: %v", err)
	}
	require.NoError(t, s.Close())

	var tree strings.Builder
	require.NoError(t, r.Render(&tree, body))

	return streamed.String(), tree.String() + "\n", true
}

// TestAStreamRejectsCallsOutOfOrder makes each call a Stream cannot place.
func TestAStreamRejectsCallsOutOfOrder(t *testing.T) {
	file, err := parser.ParseBytes([]byte("a: [b, c]\n"))
	require.NoError(t, err)
	mapping, ok := file.Docs[0].Body.(*ast.MappingNode)
	require.True(t, ok)
	key := mapping.Values[0].Key
	scalar := mapping.Values[0].Value.(*ast.SequenceNode).Values[0]

	for name, calls := range map[string]func(s *ast.Stream) error{
		"a value with no key in a mapping": func(s *ast.Stream) error {
			_ = s.BeginMapping()

			return s.Value(scalar)
		},
		"a key in a sequence": func(s *ast.Stream) error {
			_ = s.BeginSequence()

			return s.Key(key)
		},
		"two keys": func(s *ast.Stream) error {
			_ = s.BeginMapping()
			_ = s.Key(key)

			return s.Key(key)
		},
		"a collection key": func(s *ast.Stream) error {
			_ = s.BeginMapping()

			return s.Key(mapping)
		},
		"an end with nothing open": func(s *ast.Stream) error {
			return s.End()
		},
		"an end after a key": func(s *ast.Stream) error {
			_ = s.BeginMapping()
			_ = s.Key(key)

			return s.End()
		},
		"a second node": func(s *ast.Stream) error {
			_ = s.Value(scalar)

			return s.Value(scalar)
		},
		"a close with a collection open": func(s *ast.Stream) error {
			_ = s.BeginSequence()

			return s.Close()
		},
		"a close with nothing written": func(s *ast.Stream) error {
			return s.Close()
		},
	} {
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			require.ErrorIs(t, calls(ast.NewRenderer().Stream(&b)), ast.ErrStreamOrder)
		})
	}
}

// TestAStreamWritesTheShapesTheCorpusRarelyHolds streams documents built for the cases the suite holds few of: a long
// key over a collection, empty collections in every position, and sequences nested in sequences. Each comes out as
// the Renderer writes it, and reads back.
func TestAStreamWritesTheShapesTheCorpusRarelyHolds(t *testing.T) {
	long := strings.Repeat("k", 1100)

	for name, src := range map[string]string{
		"a long key over a mapping":        long + ":\n  a: 1\n",
		"a long key over a sequence":       long + ":\n- 1\n",
		"a long key over an empty mapping": long + ": {}\n",
		"a long key over a scalar":         long + ": 1\n",
		"a long key in a nested mapping":   "a:\n  " + long + ":\n    b: 1\n",
		"an empty mapping alone":           "{}\n",
		"an empty sequence alone":          "[]\n",
		"empty collections as values":      "a: {}\nb: []\nc:\n  d: []\n",
		"empty collections as entries":     "- {}\n- []\n- - []\n",
		"sequences in sequences":           "- - - 1\n    - 2\n  - 3\n- 4\n",
		"a mapping in a sequence in a key": "a:\n- b: 1\n  c:\n  - d: 2\n",
		"a scalar alone":                   "x\n",
		"a block scalar alone":             "|-\n  a\n  b\n",
		"a block scalar in an entry":       "- |-\n  a\n  b\n- c: |\n    d\n",
		"a key reading --- at the margin":  "'---': 1\n",
	} {
		for _, opts := range [][]ast.RenderOption{nil, {ast.WithIndent(4)}, {ast.WithIndentSequence(true)}} {
			t.Run(name, func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(src))
				require.NoError(t, err)
				body := file.Docs[0].Body
				r := ast.NewRenderer(opts...)

				var streamed strings.Builder
				s := r.Stream(&streamed)
				require.NoError(t, streamNode(s, body))
				require.NoError(t, s.Close())

				var tree strings.Builder
				require.NoError(t, r.Render(&tree, body))
				assert.Equal(t, tree.String()+"\n", streamed.String())

				again, err := parser.ParseBytes([]byte(streamed.String()))
				require.NoError(t, err, "%.80q", streamed.String())
				assert.Equal(t, file.String(), again.String())
			})
		}
	}
}

// handOver hands n to s, streaming each collection above depth cut and handing the rest to [ast.Stream.Value] whole:
// a collection at depth cut or deeper, and an anchor, an alias or a tag at any depth. A negative cut streams every
// collection. It sets the streamed collections to block style, and leaves a handed-over subtree as it is.
func handOver(s *ast.Stream, n ast.Node, depth, cut int, handed *int) error {
	streamed := cut < 0 || depth < cut
	switch node := n.(type) {
	case *ast.MappingNode:
		if !streamed {
			return handWhole(s, n, depth, handed)
		}
		node.SetIsFlowStyle(false)
		if err := s.BeginMapping(); err != nil {
			return err
		}
		for _, mv := range node.Values {
			switch mv.Key.(type) {
			case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.AliasNode, *ast.TagNode, *ast.MergeKeyNode,
				*ast.MappingKeyNode, nil:
				// A Stream takes a scalar key only, and an explicit "? " key stays explicit in the tree.
				return errNotStreamable
			}
			if err := s.Key(mv.Key); err != nil {
				return err
			}
			if err := handOver(s, mv.Value, depth+1, cut, handed); err != nil {
				return err
			}
		}

		return s.End()
	case *ast.SequenceNode:
		if !streamed {
			return handWhole(s, n, depth, handed)
		}
		node.SetIsFlowStyle(false)
		if err := s.BeginSequence(); err != nil {
			return err
		}
		for _, v := range node.Values {
			if err := handOver(s, v, depth+1, cut, handed); err != nil {
				return err
			}
		}

		return s.End()
	case *ast.MappingKeyNode, *ast.MappingValueNode, *ast.CommentGroupNode, *ast.DocumentNode, *ast.MergeKeyNode:
		return errNotStreamable
	case *ast.AnchorNode, *ast.AliasNode, *ast.TagNode:
		return handWhole(s, n, depth, handed)
	default:
		return s.Value(n)
	}
}

// handWhole hands n to [ast.Stream.Value], and counts it when it stands inside a collection the stream opened.
func handWhole(s *ast.Stream, n ast.Node, depth int, handed *int) error {
	if depth > 0 {
		*handed++
	}

	return s.Value(n)
}

// TestAStreamWritesAHandedOverSubtreeAsTheRendererDoes hands whole subtrees to [ast.Stream.Value]: every anchor, alias
// and tag, and every collection from a cut depth down, in the author's style. An encoder that streams what it can and
// builds a tree for the rest relies on this. The corpus and the settings are those of
// TestAStreamWritesWhatTheRendererWrites, and the output must match Renderer.Render byte for byte.
func TestAStreamWritesAHandedOverSubtreeAsTheRendererDoes(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	srcs := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, string(s.InYAML))
	}
	srcs = append(srcs, seeds...)

	for _, cfg := range []struct {
		name string
		opts []ast.RenderOption
	}{
		{"default", nil},
		{"indent4", []ast.RenderOption{ast.WithIndent(4)}},
		{"indentseq", []ast.RenderOption{ast.WithIndentSequence(true)}},
		{"indent3seq", []ast.RenderOption{ast.WithIndent(3), ast.WithIndentSequence(true)}},
	} {
		for _, cut := range []int{-1, 0, 1, 2, 3} {
			t.Run(fmt.Sprintf("%s/cut%d", cfg.name, cut), func(t *testing.T) {
				r := ast.NewRenderer(append([]ast.RenderOption{ast.WithComments(false)}, cfg.opts...)...)
				compareHandOver(t, r, srcs, cut)
			})
		}
	}
}

// compareHandOver streams every source with [handOver] at one cut and compares it with Renderer.Render.
func compareHandOver(t *testing.T, r *ast.Renderer, srcs []string, cut int) {
	t.Helper()

	var compared, differed, withHandover int
	for _, src := range srcs {
		file, err := parser.ParseBytes([]byte(src))
		if err != nil {
			continue
		}
		for _, doc := range file.Docs {
			streamed, want, handed, ok := handOverAndRender(t, r, doc.Body, cut)
			if !ok {
				continue
			}
			compared++
			if handed > 0 {
				withHandover++
			}
			if streamed != want {
				differed++
				if differed <= 3 {
					t.Errorf("the stream wrote\n%s\nthe renderer wrote\n%s\nfor\n%s", streamed, want, src)
				}
			}
		}
	}
	if differed > 3 {
		t.Errorf("%d of %d documents differ", differed, compared)
	}
	t.Logf("%d documents compared, %d with a subtree handed over inside a streamed collection", compared, withHandover)
	assert.Greater(t, compared, 5000)
	if cut != 0 {
		// At cut 0 the whole document goes to Value, and the stream opens no collection.
		assert.Greater(t, withHandover, 1000)
	}
}

// handOverAndRender writes body through a Stream with [handOver] and through Renderer.Render, and returns both and
// the number of subtrees handed over inside a streamed collection. It returns false for a body a Stream cannot take.
func handOverAndRender(t *testing.T, r *ast.Renderer, body ast.Node, cut int) (string, string, int, bool) {
	t.Helper()

	if body == nil {
		return "", "", 0, false
	}
	ast.Walk(noBlankLines{}, body)

	var streamed strings.Builder
	var handed int
	s := r.Stream(&streamed)
	if err := handOver(s, body, 0, cut, &handed); errors.Is(err, errNotStreamable) {
		return "", "", 0, false
	} else if err != nil {
		t.Fatalf("stream: %v", err)
	}
	require.NoError(t, s.Close())

	var tree strings.Builder
	require.NoError(t, r.Render(&tree, body))

	return streamed.String(), tree.String() + "\n", handed, true
}
