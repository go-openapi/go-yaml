// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestTheVerbatimDescentFollowsTheDocument checks that the descent reaches the
// source tokens in the order the document wrote them.
//
// ⚠️ The output cannot show this. Renderer.VerbatimFile copies the source
// forward to each token's end and finishes at the end of the source, so it
// writes the document back whatever order the tokens arrive in -- and with the
// descent removed altogether. Only the sequence of ends says whether the descent
// followed the document, and it is the sequence that matters: the cursor stops
// covering for it as soon as a node the source does not reach interrupts the
// copy.
func TestTheVerbatimDescentFollowsTheDocument(t *testing.T) {
	t.Parallel()

	var docs, walked, backwards int
	shown := 0
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}
		docs++

		out := true
		for _, doc := range file.Docs {
			ends := ast.SourceTokenEnds(doc)
			for i := 1; i < len(ends); i++ {
				if ends[i] < ends[i-1] {
					out = false
					if shown < 5 {
						t.Logf("%s: the descent went from %d back to %d in %q",
							src.name, ends[i-1], ends[i], src.text)
						shown++
					}
				}
			}
			walked += len(ends)
		}
		if !out {
			backwards++
		}
	}

	t.Logf("walked %d source tokens over %d documents, %d of which went backwards", walked, docs, backwards)
	require.Positive(t, walked)
	require.LessOrEqualf(t, backwards, descentCeiling,
		"the descent lost ground: %d documents go backwards, ceiling is %d", backwards, descentCeiling)
}

// descentCeiling is how many documents the verbatim descent may read out of
// order.
//
// A count rather than a ledger, and it is not allowed to rise. Two shapes make
// up the 23, and both are the tree reporting a token that is not the node's:
//
//   - "? []: x" gives a MappingValueNode whose Start is a SequenceStart "[" at
//     offset 2, where the field holds the ":" that closes a key. Handed over
//     after the key's own subtree, which reaches offset 6, it reads backwards.
//     That document also decodes to map[string]any{"map[[]:x]": nil}, so the
//     shape is odd well before a renderer sees it.
//   - "anchors-on-empty-scalars" and its kind, where an anchor stands on
//     nothing and the node built for the empty value carries a position from
//     the property that opened it.
//
// Neither loses text today: Renderer.VerbatimFile only ever copies forward, so a
// token read out of order was already written with an earlier one. They matter
// when a node the source does not reach interrupts the copy, which is what the
// insertion case does.
const descentCeiling = 23

// TestVerbatimWritesTheDocumentBack renders// TestVerbatimWritesANodeBack checks the per-node half: a node writes the
// stretch of source it covers, and nothing of its neighbors.
func TestVerbatimWritesANodeBack(t *testing.T) {
	t.Parallel()

	const src = "# lead\nname: &a Pet   # trail\nlist:\n  - 1\n  - !!str two\nlit: |\n  body\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	r := ast.NewRenderer(ast.WithSource([]byte(src)))

	var checked int
	for _, doc := range file.Docs {
		walkEveryNode(doc, func(n ast.Node) {
			var out bytes.Buffer
			require.NoError(t, r.Verbatim(&out, n))
			if out.Len() == 0 {
				return
			}
			checked++
			require.Containsf(t, src, out.String(),
				"a %s wrote %q, which the document does not hold", n.Type(), out.String())
		})
	}
	require.Positive(t, checked)
}
