// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/parser"
)

// TestJSONTokensAllocateNothingPerValue holds the token walk to a fixed number
// of allocations on top of its parse, whatever the document's length.
//
// A fresh buffer for every number and a keys slice grown from nothing for every
// mapping made the token walk allocate 166,790 times on golang_source, where
// ToJSON allocates 207. ToJSON is to run on this walk, so an allocation per
// value here is one in every conversion. The parse allocates in proportion to
// the document by itself, so a walk handing its nodes to a visitor that does
// nothing is measured on the same source and taken away.
func TestJSONTokensAllocateNothingPerValue(t *testing.T) {
	overWalk := func(src []byte) float64 {
		tokens := testing.AllocsPerRun(5, func() {
			s := ToJSONTokens(src)
			for range s.Tokens() {
			}
		})
		walk := testing.AllocsPerRun(5, func() {
			_, _ = parser.New(parser.WithOmitNodePaths(), parser.WithJSONCompatible()).Walk(src, discardVisitor{})
		})

		return tokens - walk
	}

	for _, gen := range []struct {
		name string
		doc  func(int) string
	}{
		{"flat mapping", corpus.FlatMap},
		{"flat sequence", corpus.FlatSequence},
		{"nested mappings", corpus.NestedDoc},
		{"numbers, bools and nulls", scalarSequence},
	} {
		t.Run(gen.name, func(t *testing.T) {
			small := overWalk([]byte(gen.doc(10)))
			large := overWalk([]byte(gen.doc(2000)))
			t.Logf("allocations over the walk: %.0f at 10, %.0f at 2000", small, large)

			// A few frames and a buffer grow once as the document widens; one per
			// value would add thousands.
			assert.LessOrEqualf(t, large-small, 16.0,
				"the token walk allocates %.0f more at 2000 than at 10", large-small)
		})
	}
}

// scalarSequence is a block sequence of n entries cycling through an integer, a
// float, a bool and a null, each spelled as JSON spells it.
//
// The corpus generators write strings, which the token walk takes from the node
// and never writes out, so they leave the scalar path unmeasured. A number JSON
// spells differently -- "0x1F" is 31 -- costs its token a string of its own.
func scalarSequence(n int) string {
	spellings := [...]string{"- 12\n", "- 1.5\n", "- true\n", "- null\n"}

	var b strings.Builder
	for i := range n {
		b.WriteString(spellings[i%len(spellings)])
	}

	return b.String()
}

// discardVisitor is handed a walk's nodes and does nothing with them.
type discardVisitor struct{}

func (discardVisitor) Enter(ast.Node, parser.Cursor) error { return nil }

func (discardVisitor) Leave(ast.Node, parser.Cursor) error { return nil }
