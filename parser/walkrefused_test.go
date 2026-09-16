// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// balanceSpy records the nodes a walk opened and never closed.
//
// It names each node by its type and the depth it opened at, which is what the walk reports on Leave too.
// A node pointer would not do: the parse hands its cells out again behind the descent,
// so two nodes of one document are often one pointer.
type balanceSpy struct {
	open  []string
	extra []string
}

func (s *balanceSpy) Enter(node ast.Node, at parser.Cursor) error {
	s.open = append(s.open, fmt.Sprintf("%s@%d", node.Type(), at.Depth()))

	return nil
}

func (s *balanceSpy) Leave(node ast.Node, at parser.Cursor) error {
	want := fmt.Sprintf("%s@%d", node.Type(), at.Depth())
	if len(s.open) == 0 {
		s.extra = append(s.extra, want)

		return nil
	}
	s.open = s.open[:len(s.open)-1]

	return nil
}

// TestARefusedDocumentLeavesEveryNodeItEntered checks that a walk closes what it opened
// when the parse gives up partway through the document.
//
// A consumer writes what a node holds between Enter and Leave, so a node entered and never left is a
// collection it never closes: codec.ToJSONTokens handed over "{" for a mapping and no "}", and a reader
// of the token stream saw an unbalanced run with the reason only in Err.
//
// The descent unwinds an error through its defers, so every p.leave registered next to its p.enter runs.
// parser/mapping.go registered it 61 lines later, past four returns, and a mapping that failed in its
// entries was entered and never left.
//
// ⚠️ This is the accepting side of the walk's contract and nothing else asserts it: TestTheWalkHandsOverTheSameTree
// digests what a refused document hands over, so it pins whatever the walk does rather than saying what it should.
func TestARefusedDocumentLeavesEveryNodeItEntered(t *testing.T) {
	t.Parallel()

	t.Run("over the corpus", func(t *testing.T) {
		t.Parallel()

		var refused int
		for _, src := range walkSources(t) {
			s := &balanceSpy{}
			if _, err := parser.New(parser.WithComments()).Walk([]byte(src.text), s); err == nil {
				continue
			}
			refused++
			assert.Emptyf(t, s.open, "%s: the walk entered these and never left them", src.name)
			assert.Emptyf(t, s.extra, "%s: the walk left these without entering them", src.name)
		}

		// The census says what the run covered. A change that stopped the parser refusing anything
		// would leave every assertion above unreached and the test green.
		t.Logf("%d refused documents walked", refused)
		require.Positive(t, refused, "no document was refused, so nothing above was checked")
	})

	for _, tc := range []struct {
		name string
		src  string
		// depth is how deep the refusal stands, so the case fails for its own reason
		// and not because the parser stopped refusing the document.
		depth int
	}{
		{name: "a flow sequence in a nested mapping never closes", src: "a:\n  b: 1\n  c: [1, 2\n", depth: 2},
		{name: "an alias in a nested mapping names nothing", src: "a:\n  b: 1\n  c: *nope\n", depth: 2},
		{name: "a sequence entry follows a mapping entry", src: "a: 1\n- b\n", depth: 1},
		{name: "a flow mapping never closes", src: "a: {b: 1\n", depth: 1},
		{name: "the first entry of a mapping is refused", src: "a: [1\n", depth: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &balanceSpy{}
			_, err := parser.New().Walk([]byte(tc.src), s)
			require.Errorf(t, err, "%q parses, so this case checks nothing", tc.src)

			assert.Emptyf(t, s.open, "%q: entered and never left\n%s", tc.src, strings.Join(s.open, "\n"))
			assert.Empty(t, s.extra, "left without entering")
		})
	}
}
