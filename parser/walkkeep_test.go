// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/testcorpus"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// keepSpy keeps every node at one depth, logs every handover, and clones what it keeps.
type keepSpy struct {
	depth int
	// atLeave sends KeepNode from Leave instead of Enter.
	atLeave bool

	log  []string
	kept []keptNode
	// cursors holds, for each kept node, what the cursor answered at Enter and then at Leave.
	cursors [][2]string
}

type keptNode struct {
	offset int
	kind   ast.NodeType
	text   string
}

func (s *keepSpy) Enter(node ast.Node, at parser.Cursor) error {
	s.log = append(s.log, fmt.Sprintf("enter %s %d", node.Type(), at.Depth()))
	if at.Depth() != s.depth || s.atLeave {
		return nil
	}
	s.cursors = append(s.cursors, [2]string{cursorOf(at)})

	return parser.KeepNode
}

func (s *keepSpy) Leave(node ast.Node, at parser.Closing) error {
	s.log = append(s.log, fmt.Sprintf("leave %s %d", node.Type(), at.Depth()))
	if at.Depth() != s.depth {
		return nil
	}
	if s.atLeave {
		return parser.KeepNode
	}
	s.cursors[len(s.cursors)-1][1] = cursorOf(at)

	offset := -1
	if tk := node.GetToken(); tk != nil {
		offset = int(tk.Position.Offset())
	}
	s.kept = append(s.kept, keptNode{offset: offset, kind: node.Type(), text: ast.Clone(node).String()})

	return nil
}

func cursorOf(at parser.Cursor) string {
	return fmt.Sprintf("depth=%d in=%v key=%v root=%v doc=%d", at.Depth(), at.In(), at.IsKey(), at.IsRoot(), at.Document())
}

// TestKeepNodeHandsTheNodeOverWhole checks what a visitor answering KeepNode receives.
//
// A walk keeps none of a collection's entries, so a mapping reached Leave empty and a visitor had no way to
// hold a matched subtree as an AST.
func TestKeepNodeHandsTheNodeOverWhole(t *testing.T) {
	t.Parallel()

	const src = "match:\n  k0: {x: 0, y: [a, b]}\n  k1:\n    - p\n    - q: r\nafter: 1\n"

	t.Run("a kept mapping arrives whole, and nothing inside it is handed over", func(t *testing.T) {
		t.Parallel()

		s := &keepSpy{depth: 1}
		_, err := parser.New().Walk([]byte(src), s)
		require.NoError(t, err)

		assert.Equal(t, []string{
			"enter Mapping 0",
			"enter String 1", "leave String 1",
			"enter Mapping 1", "leave Mapping 1",
			"enter String 1", "leave String 1",
			"enter Integer 1", "leave Integer 1",
			"leave Mapping 0",
		}, s.log)

		require.Len(t, s.kept, 4)
		assert.Equal(t, "k0: {x: 0, y: [a, b]}\nk1:\n- p\n- q: r", s.kept[1].text)
		assert.Equal(t, "1", s.kept[3].text, "a scalar reads KeepNode as nil and still gets its Leave")
	})

	t.Run("the cursor answers at Leave as it did at Enter", func(t *testing.T) {
		t.Parallel()

		s := &keepSpy{depth: 1}
		_, err := parser.New().Walk([]byte(src), s)
		require.NoError(t, err)

		for _, pair := range s.cursors {
			assert.Equal(t, pair[0], pair[1])
		}
	})

	t.Run("a kept sequence, and a kept root", func(t *testing.T) {
		t.Parallel()

		seq := &keepSpy{depth: 0}
		_, err := parser.New().Walk([]byte("- a\n- {b: [c, d]}\n- - e\n"), seq)
		require.NoError(t, err)
		require.Len(t, seq.kept, 1)
		assert.Equal(t, "- a\n- {b: [c, d]}\n- - e", seq.kept[0].text)
		assert.Equal(t, []string{"enter Sequence 0", "leave Sequence 0"}, seq.log)
	})

	t.Run("the clone outlives the walk", func(t *testing.T) {
		t.Parallel()

		// The kept mapping comes first and ten thousand keys follow, so its cells and tokens are handed out
		// again long before the walk ends.
		var b strings.Builder
		b.WriteString("first:\n  a: [1, 2]\n  b: {c: d}\n")
		b.WriteString(corpus.FlatMap(10000))

		var clone ast.Node
		v := &keepFirst{into: &clone}
		_, err := parser.New(parser.WithChunkSize(tokenarena.MinChunk)).Walk([]byte(b.String()), v)
		require.NoError(t, err)
		require.NotNil(t, clone)
		assert.Equal(t, "a: [1, 2]\nb: {c: d}", clone.String())
	})

	t.Run("KeepNode from Leave stops the walk", func(t *testing.T) {
		t.Parallel()

		s := &keepSpy{depth: 1, atLeave: true}
		_, err := parser.New().Walk([]byte(src), s)
		require.ErrorIs(t, err, parser.KeepNode)
	})
}

// keepFirst keeps the value of the document's first entry and skips everything else.
// It clones the kept node into into, unless into is nil, and keeps nothing when skipOnly is set.
type keepFirst struct {
	into     *ast.Node
	skipOnly bool
	seen     int
}

func (v *keepFirst) Enter(_ ast.Node, at parser.Cursor) error {
	if at.Depth() != 1 {
		return nil
	}
	v.seen++
	if v.seen == 2 && !v.skipOnly {
		return parser.KeepNode
	}

	return parser.SkipNode
}

func (v *keepFirst) Leave(node ast.Node, at parser.Closing) error {
	if at.Depth() == 1 && v.seen == 2 && v.into != nil && *v.into == nil {
		*v.into = ast.Clone(node)
	}

	return nil
}

// TestAKeptNodeMatchesTheParsedTree holds every kept node to the node a parse builds from the same token.
//
// A walk reuses node cells and tokens as it hands entries over. Keeping a node turns that off below it, and
// every gate of the descent reads the same switch. A gate left behind shows here as a kept node that renders
// differently from the tree's, and the smallest chunk size makes the tape reuse its tokens as soon as it may.
func TestAKeptNodeMatchesTheParsedTree(t *testing.T) {
	t.Parallel()

	sources := keepSources(t)

	var checked, unmatched int
	for _, src := range sources {
		file, treeErr := parser.ParseBytes(src)
		tree := renderedByToken(file)

		for depth := range 4 {
			s := &keepSpy{depth: depth}
			_, walkErr := parser.New(parser.WithChunkSize(tokenarena.MinChunk)).Walk(src, s)
			require.Equalf(t, treeErr == nil, walkErr == nil,
				"%q: the parse returned %v and the walk %v", src, treeErr, walkErr)
			if treeErr != nil {
				continue
			}

			for _, kept := range s.kept {
				want, found := tree[keptKey(kept.offset, kept.kind)]
				if !found {
					// The walk hands over a few nodes the tree does not hold as such, such as the second root
					// "&!" gives. Counted, so a change in how many is visible.
					unmatched++

					continue
				}
				checked++
				assert.Containsf(t, want, kept.text, "%q at depth %d: kept %s at offset %d", src, depth, kept.kind, kept.offset)
			}
		}
	}

	t.Logf("%d sources, %d kept nodes checked, %d with no counterpart in the tree", len(sources), checked, unmatched)
	assert.Greater(t, checked, 10*unmatched, "most kept nodes have a counterpart, or the check checks little")
}

func keptKey(offset int, kind ast.NodeType) string { return fmt.Sprintf("%d/%s", offset, kind) }

// renderedByToken renders every node of a parsed file, keyed by its token's offset and its type.
func renderedByToken(file *ast.File) map[string][]string {
	out := map[string][]string{}
	if file == nil {
		return out
	}
	for _, doc := range file.Docs {
		ast.Walk(visitFunc(func(n ast.Node) {
			if n == nil || reflect.ValueOf(n).IsNil() {
				// A tree holds typed nil nodes, such as the value of a key written alone.
				return
			}
			if tk := n.GetToken(); tk != nil {
				key := keptKey(int(tk.Position.Offset()), n.Type())
				out[key] = append(out[key], ast.Clone(n).String())
			}
		}), doc)
	}

	return out
}

type visitFunc func(ast.Node)

func (f visitFunc) Visit(n ast.Node) ast.Visitor {
	f(n)

	return f
}

func keepSources(t *testing.T) [][]byte {
	t.Helper()

	var sources [][]byte
	for _, dir := range []string{testcorpus.Dir(), filepath.Join(testcorpus.Dir(), "stress")} {
		for _, doc := range testcorpus.Docs(t, dir) {
			sources = append(sources, doc.Data)
		}
	}
	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, s := range suites {
		sources = append(sources, s.InYAML)
	}
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	for _, seed := range seeds {
		sources = append(sources, []byte(seed))
	}

	return sources
}

// TestKeepingASubtreeCostsTheSubtree holds a walk that keeps one small subtree to the memory of one that
// skips it.
//
// The tape is pinned while the kept node is read and let go once its Leave returns. Left pinned, the walk
// would hold every token after the kept node. Measured, keeping the subtree costs five allocations more
// than skipping it, on 5,000 keys as on 20,000.
func TestKeepingASubtreeCostsTheSubtree(t *testing.T) {
	src := []byte("first:\n  a: [1, 2]\n  b: {c: d}\n" + corpus.FlatMap(5000))

	allocs := func(skipOnly bool) float64 {
		return testing.AllocsPerRun(3, func() {
			_, err := parser.New(parser.WithOmitNodePaths()).Walk(src, &keepFirst{skipOnly: skipOnly})
			require.NoError(t, err)
		})
	}

	assert.LessOrEqual(t, allocs(false), allocs(true)+16)
}
