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

// cursorSpy reads every answer the cursor gives, at each Enter and each Leave.
type cursorSpy struct {
	// open holds the answers of the nodes entered and not yet left, innermost last.
	open []string
	// pairs holds "Enter answers | Leave answers" for each node that was left.
	pairs []string
	// roots counts the nodes the cursor called a root.
	roots int
	// disagreed holds every node whose IsRoot did not match Depth == 0 and In == KindNone.
	disagreed []string
}

func (s *cursorSpy) Enter(node ast.Node, at parser.Cursor) error {
	answer := s.read(node, at)
	if at.IsRoot() {
		s.roots++
	}
	if at.IsRoot() != (at.Depth() == 0 && at.In() == parser.KindNone) {
		s.disagreed = append(s.disagreed, answer)
	}
	s.open = append(s.open, answer)

	return nil
}

func (s *cursorSpy) Leave(node ast.Node, at parser.Closing) error {
	answer := s.read(node, at)
	entered := s.open[len(s.open)-1]
	s.open = s.open[:len(s.open)-1]
	s.pairs = append(s.pairs, entered+" | "+answer)

	return nil
}

func (s *cursorSpy) read(node ast.Node, at parser.Cursor) string {
	return fmt.Sprintf("%v depth=%d in=%v key=%v root=%v doc=%d",
		node.Type(), at.Depth(), at.In(), at.IsKey(), at.IsRoot(), at.Document())
}

// TestTheCursorAnswersTheSameAtLeave checks the contract a consumer folds a document on.
//
// A consumer builds what a collection holds between Enter and Leave and delivers it on the way out, so it
// needs the same answers at both. Step was a copy taken at each call and the two agreed by construction;
// the cursor reads the walk's own stacks, and leave pops them before it calls the visitor, which is what
// makes the answers match.
func TestTheCursorAnswersTheSameAtLeave(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"a: 1\n",
		"a:\n  b: [1, {c: 2}]\n  d: &x 3\ne: *x\n",
		"? [1, 2]\n: v\n",
		"!!str a: !!int 1\n",
		"%YAML 1.2\n---\na: 1\n---\nb: 2\n",
		"- - - deep\n",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			s := &cursorSpy{}
			_, err := parser.New(parser.WithComments()).Walk([]byte(src), s)
			require.NoErrorf(t, err, "%q", src)

			require.NotEmpty(t, s.pairs, "%q handed no node over, so nothing was compared", src)
			for _, pair := range s.pairs {
				before, after, _ := strings.Cut(pair, " | ")
				assert.Equalf(t, before, after, "%q: Leave answers differently from Enter", src)
			}
			assert.Emptyf(t, s.disagreed, "%q: IsRoot disagrees with Depth == 0 and In == KindNone", src)
			assert.Positivef(t, s.roots, "%q: no node was called a root", src)
		})
	}
}

// keySpy records the type and token of each node the cursor marks a key.
type keySpy struct{ keys []string }

func (s *keySpy) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsKey() {
		s.keys = append(s.keys, fmt.Sprintf("%T %s", node, node.GetToken().Value))
	}

	return nil
}

func (s *keySpy) Leave(ast.Node, parser.Closing) error { return nil }

// TestAnExplicitMergeKeyIsHandedOverAsItsQuestionMark walks every spelling of an explicit "?" merge key, and asks
// for the key the tree holds: the "?" node, with the merge key inside it.
//
// "? !!merge <<" and "? &k !!merge <<" handed the tag over as the key and nothing for the "?", while "? <<" and
// "{? <<: *a}" handed the "?". A reader comparing keys by their token, as a path ".name" step does, found a
// different first key on the walk and on the tree.
func TestAnExplicitMergeKeyIsHandedOverAsItsQuestionMark(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"? <<", "? &k <<", "? !!merge <<", "? &k !!merge <<", "? !<tag:yaml.org,2002:merge> <<"} {
		src := "%YAML 1.1\n---\na: &a {x: 1}\nm:\n  " + key + "\n  : *a\n  w: 2\n"

		spy := &keySpy{}
		_, err := parser.New().Walk([]byte(src), spy)
		require.NoErrorf(t, err, "%q", src)

		file, err := parser.ParseBytes([]byte(src))
		require.NoErrorf(t, err, "%q", src)
		mapping, ok := file.Docs[len(file.Docs)-1].Body.(*ast.MappingNode)
		require.Truef(t, ok, "%q", src)
		inner, ok := mapping.Values[1].Value.(*ast.MappingNode)
		require.Truef(t, ok, "%q", src)
		require.IsTypef(t, &ast.MappingKeyNode{}, inner.Values[0].Key, "%q", src)

		assert.Equalf(t, []string{"*ast.StringNode a", "*ast.StringNode x", "*ast.StringNode m", "*ast.MappingKeyNode ?", "*ast.StringNode w"},
			spy.keys, "%q", src)
	}
}
