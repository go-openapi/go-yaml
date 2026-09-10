// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAnchoredTargetSurvivesTheRewind reads [ast.AliasNode.Target] on a walk,
// with the anchor and the alias moved further and further apart.

// TODO: replace this wall of comment by more precisely target comments inside the test and just an example.
//
// ⚠️ The distance is the whole test. A walk hands an entry's cells out again
// once the entry has gone over, so an anchored node read back without the pin
// holds whatever the parse built there next -- and the cells are per node type,
// so the filler has to allocate the same type the anchor does. Written with the
// anchor and the alias adjacent, every case passes and nothing is checked:
// "a: &x {k: 1, z: 9}" over "b: *x" reads correctly, and one line of filler
// between them reads "{b: 0, z: 9}".
func TestAnchoredTargetSurvivesTheRewind(t *testing.T) {
	t.Parallel()

	for anchored := range anchoredTestCases() {
		t.Run(anchored.name, func(t *testing.T) {
			for _, entries := range []int{0, 1, 4, 64, 500} {
				var got string
				v := aliasTargetReader{onAlias: func(target ast.Node) {
					if target == nil {
						got = "<nil>"

						return
					}
					got = target.String()
				}}

				p := parser.New(parser.WithOmitNodePaths())
				_, err := p.Walk(betweenAnchorAndAlias(anchored.text, entries), &v)
				require.NoError(t, err)
				assert.Equalf(t, anchored.want, got, "%d entries between the anchor and the alias", entries)
				// TODO: assert the node
			}
		})
	}
}

type anchoredTestCase struct{ name, text, want string }

func anchoredTestCases() iter.Seq[anchoredTestCase] {
	return slices.Values([]anchoredTestCase{
		{"an integer", "&x 1", "1"},
		{"a string", "&x hello", "hello"},
		{"a flow sequence", "&x [1, 2]", "[1, 2]"},
		{"a flow mapping", "&x {k: 1, z: 9}", "{k: 1, z: 9}"},
	})
}

// betweenAnchorAndAlias writes an anchor, then entries mapping entries whose
// keys are strings and whose values are integers, then an alias naming the
// anchor.
func betweenAnchorAndAlias(anchor string, entries int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "a: %s\n", anchor)
	for i := range entries {
		fmt.Fprintf(&b, "f%d: %d\n", i, i)
	}
	b.WriteString("b: *x\n")

	return []byte(b.String())
}

// aliasTargetReader hands every alias it is given to onAlias, as a consumer expanding one would.
type aliasTargetReader struct {
	onAlias func(target ast.Node)
}

func (r *aliasTargetReader) Enter(node ast.Node, _ parser.Step) bool {
	if alias, ok := node.(*ast.AliasNode); ok {
		r.onAlias(alias.Target)
	}

	return true
}

func (r *aliasTargetReader) Leave(ast.Node, parser.Step) {}
