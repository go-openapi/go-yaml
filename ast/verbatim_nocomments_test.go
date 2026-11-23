// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// withoutCommentsCensus records the corpus rebuilt through VerbatimFile from a
// parse without parser.WithComments: how many documents the parse accepts, and
// how many of those do not come back byte for byte.
//
// TestVerbatimRebuildsEveryDocument parses with comments, and a tree parsed
// without them differs in ways that census cannot see: it holds no comment and
// no SequenceNode.Entries. Leaving out a removed node took a "-" no entry hands
// over, a "..." no document holds and every comment for a removed node's text,
// and 27 documents no caller had touched came back wrong or were refused before
// this census was kept.
//
// The 30 recorded on 2026-09-11 were there before any of that, and all 30 held
// one shape: a block sequence whose values carry no token of their own, written
// as layout where it should have been copied. A lone "-" came back "- " and
// "---\n-\n" came back "---- \n".
// TestVerbatimRebuildsABlockSequenceOfImplicitNulls holds those shapes now, and
// the count has stood at 0 since 2026-09-13.
//
// 12708 -> 12680 on 2026-11-18, when a directive with no name after its '%' and a "%TAG" prefix holding a character
// outside ns-uri-char began to be refused. The 28 documents that left all hold one, and yaml/yaml-reference-parser
// refuses each of them.
//
// 12680 -> 12679 the same day, when a comment holding a byte order mark began to be refused: seed/7167 opens on
// "#\ufeff%YAML 1.1", and c-nb-comment-text takes nb-char, which leaves the mark out.
//
// 12679 -> 12668 the same day, when a "%YAML" with no version began to be refused: eleven seeds open on a bare
// "%YAML", and ns-yaml-directive requires the version.
//
// 12668 -> 12662 on 2026-11-23, when a '}' or a ']' closing no collection inside a flow collection began to be
// refused: six seeds hold one, "[... 1}]" among them, and go.yaml.in/yaml/v3 refuses each of them.
var withoutCommentsCensus = struct{ tested, differing int }{tested: 12662, differing: 0}

func TestVerbatimRebuildsTheCorpusParsedWithoutComments(t *testing.T) {
	t.Parallel()

	var tested, differing int
	var first []string
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text))
		if err != nil {
			continue
		}
		tested++

		var out bytes.Buffer
		err = ast.NewRenderer(ast.WithSource([]byte(src.text))).VerbatimFile(&out, file)
		if err != nil || out.String() != src.text {
			differing++
			if len(first) < 10 {
				first = append(first, src.name)
			}
		}
	}

	t.Logf("without comments: %d of %d accepted documents do not come back byte for byte (recorded %d of %d), starting with %v",
		differing, tested, withoutCommentsCensus.differing, withoutCommentsCensus.tested, first)

	require.GreaterOrEqualf(t, tested, withoutCommentsCensus.tested,
		"%d documents are measured where %d were: fewer are accepted than were, so re-measure before reading the count below",
		tested, withoutCommentsCensus.tested)
	mustHold(t, "the count of documents that do not come back byte for byte",
		differing, withoutCommentsCensus.differing, tested, withoutCommentsCensus.tested)
}

// TestVerbatimRebuildsABlockSequenceOfImplicitNulls holds the sequence whose
// values all carry no token of their own.
//
// walkSourceTokens reaches a block sequence's "-" through entryFor, and a parse
// without parser.WithComments records no SequenceNode.Entries. Where the values
// carry no token either, the walk handed over nothing and sourceExtent found no
// extent, so writeInPlaceOf took the sequence for a node a caller had put in: it
// wrote the layout render "- " and set dropping, which took the document's own
// "-", the break in front of it and the comment on its line. "---\n-\n" came
// back "---- \n", one plain scalar where the document wrote a marker and a
// sequence.
//
// A sibling entry that holds text hides it, and so does a property on the entry:
// either one gives the sequence an extent.
func TestVerbatimRebuildsABlockSequenceOfImplicitNulls(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"-",
		"-\n",
		"-\n-\n",
		"---\n-\n",
		"---\n- \n",
		"%YAML 1.1\n---\n-\n",
		"a:\n  -\n",
		"- # c\n",
		"-\n# c\n",
		"---\n# c\n-\n",
		"- a\n-\n",
		"-\n- b\n",
		"- &x\n",
		"- !!null\n",
		"- {}\n-\n",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
			assert.Equal(t, src, out.String())
		})
	}
}
