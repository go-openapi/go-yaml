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
//
// A walk hands an entry's cells out again once the entry has gone over, so an
// anchored node read back without the pin holds whatever the parse built there
// next. "a: &x {k: 1, z: 9}" over "b: *x" reads back correctly; put one mapping
// entry between the two lines and the same alias reads "{b: 0, z: 9}".
func TestAnchoredTargetSurvivesTheRewind(t *testing.T) {
	t.Parallel()

	for anchored := range anchoredTestCases() {
		t.Run(anchored.name, func(t *testing.T) {
			// The distance is the whole test. A cell is handed out again only
			// once the walk has gone over the entry holding it, so the entries
			// below put another node on the anchor's cells. A fixture
			// with the anchor and the alias on consecutive lines reads back
			// correctly whatever the pin does.
			for _, entries := range []int{0, 1, 4, 64, 500} {
				var got ast.Node
				var read bool
				v := aliasTargetReader{onAlias: func(target ast.Node) {
					got, read = target, true
				}}

				p := parser.New(parser.WithOmitNodePaths())
				_, err := p.Walk(betweenAnchorAndAlias(anchored.text, entries), &v)
				require.NoError(t, err)

				require.Truef(t, read, "the walk reached no alias with %d entries between", entries)
				require.NotNilf(t, got, "Target is nil with %d entries between", entries)

				// The type, not only the text. A recycled cell holds a node of
				// the type the filler allocated, and betweenAnchorAndAlias
				// fills with mapping entries whose values are integers -- so a
				// sequence or a mapping read off a reused cell fails here even
				// where it renders as something plausible.
				assert.IsTypef(t, anchored.want, got,
					"Target is a %T with %d entries between", got, entries)
				assert.Equalf(t, anchored.text[len("&x "):], got.String(),
					"%d entries between the anchor and the alias", entries)

				// The token the node was built from is read by everything that
				// reports a position, and it is held in a second arena. A node
				// that renders correctly over a released token still breaks a
				// caller asking where it came from.
				assert.NotNilf(t, got.GetToken(), "Target has no token with %d entries between", entries)
			}
		})
	}
}

// anchoredTestCase is an anchored node, written as "&x" and its text, and the
// [ast.Node] the alias naming it must read back as.
type anchoredTestCase struct {
	name string
	text string
	want ast.Node
}

func anchoredTestCases() iter.Seq[anchoredTestCase] {
	return slices.Values([]anchoredTestCase{
		{"an integer", "&x 1", &ast.IntegerNode{}},
		{"a string", "&x hello", &ast.StringNode{}},
		{"a flow sequence", "&x [1, 2]", &ast.SequenceNode{}},
		{"a flow mapping", "&x {k: 1, z: 9}", &ast.MappingNode{}},
	})
}

// betweenAnchorAndAlias writes an anchor, then entries mapping entries whose
// keys are strings and whose values are integers, then an alias naming the
// anchor.
//
// The filler has to allocate the node types the anchor does. Cells are handed
// out per node type, so an integer written between the two lines can only land
// on an integer's cell, and the flow cases need the mapping and sequence cells
// the entries themselves build.
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
