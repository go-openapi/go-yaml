// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAPieceKnowsWhatItWrites holds the facts a rendered piece carries against
// the text it flattens to, node by node, over the test suite and the fuzz seeds.
//
// The layout decisions used to ask the finished string whether a value spans
// lines and whether it opens on a break. A piece answers both before it is
// written, which is what lets a parent place a child without building it -- and
// a piece whose answers drift from its own text puts a value on the wrong line
// rather than merely costing time. This is the guard on that.
func TestAPieceKnowsWhatItWrites(t *testing.T) {
	t.Parallel()

	for _, cfg := range []struct {
		name string
		opts []ast.RenderOption
	}{
		{"default", nil},
		{"nocomments", []ast.RenderOption{ast.WithComments(false)}},
		{"indent4", []ast.RenderOption{ast.WithIndent(4)}},
		{"indentseq", []ast.RenderOption{ast.WithIndentSequence(true)}},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()

			r := ast.NewRenderer(cfg.opts...)
			var checked int
			wrong := map[string]int{}
			examples := map[string]string{}

			for _, src := range renderSources(t) {
				file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
				if err != nil {
					continue
				}
				for _, doc := range file.Docs {
					walkEveryNode(doc, func(n ast.Node) {
						checked++
						text, spans, leads, empty := r.RenderedFacts(n)
						for what, ok := range map[string]bool{
							"spans": spans == strings.Contains(text, "\n"),
							"leads": leads == strings.HasPrefix(text, "\n"),
							"empty": empty == (text == ""),
						} {
							if ok {
								continue
							}
							key := fmt.Sprintf("%s on %s", what, n.Type())
							wrong[key]++
							if _, seen := examples[key]; !seen || len(src.text) < len(examples[key]) {
								examples[key] = src.text
							}
						}
					})
				}
			}

			t.Logf("checked %d nodes", checked)
			require.Positive(t, checked)

			keys := make([]string, 0, len(wrong))
			for k := range wrong {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return wrong[keys[i]] > wrong[keys[j]] })
			for _, k := range keys {
				t.Errorf("%6d  %s\n        smallest: %q", wrong[k], k, examples[k])
			}
		})
	}
}

// walkEveryNode calls fn for every node of a tree, including the ones ast.Walk
// does not reach on their own.
func walkEveryNode(n ast.Node, fn func(ast.Node)) {
	if n == nil {
		return
	}
	fn(n)

	switch v := n.(type) {
	case *ast.DocumentNode:
		walkEveryNode(v.Body, fn)
	case *ast.MappingNode:
		for _, e := range v.Values {
			walkEveryNode(e, fn)
		}
	case *ast.MappingValueNode:
		walkEveryNode(v.Key, fn)
		walkEveryNode(v.Value, fn)
	case *ast.MappingKeyNode:
		walkEveryNode(v.Value, fn)
	case *ast.SequenceNode:
		for _, e := range v.Values {
			walkEveryNode(e, fn)
		}
	case *ast.AnchorNode:
		walkEveryNode(v.Name, fn)
		walkEveryNode(v.Value, fn)
	case *ast.AliasNode:
		walkEveryNode(v.Value, fn)
	case *ast.TagNode:
		walkEveryNode(v.Value, fn)
	case *ast.LiteralNode:
		walkEveryNode(v.Value, fn)
	case *ast.DirectiveNode:
		walkEveryNode(v.Name, fn)
		for _, e := range v.Values {
			walkEveryNode(e, fn)
		}
	}
}

type renderSource struct {
	name string
	text string
}

func renderSources(t *testing.T) []renderSource {
	t.Helper()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	srcs := make([]renderSource, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, renderSource{"suite/" + s.Name, string(s.InYAML)})
	}
	for i, s := range seeds {
		srcs = append(srcs, renderSource{fmt.Sprintf("seed/%04d", i), s})
	}

	return srcs
}
