// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"io"
	"slices"
	"strings"
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
	require.Zerof(t, backwards, "%d documents go backwards", backwards)
}

// TestAnEntryWithNoColonIsReadInOrder walks entries written with no ':'. Their Start holds the key's first token,
// which the key hands over itself, and the descent handed it a second time after the key: "{&a1 ”}" read the "&"
// at offset 1 after the "”" ending at 7.
func TestAnEntryWithNoColonIsReadInOrder(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"{&a1 ''}\n", "{&a1 x, b: 1}\n", "{? ! 'a'}\n", "? []: x\n", "-\n  ? &d\n"} {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoErrorf(t, err, "%q", src)

		ends := ast.SourceTokenEnds(file.Docs[0])
		require.NotEmptyf(t, ends, "%q", src)
		for i := 1; i < len(ends); i++ {
			require.GreaterOrEqualf(t, ends[i], ends[i-1], "%q: token %d", src, i)
		}
	}
}

// TestVerbatimWritesANodeBack checks the per-node half: a node writes the
// stretch of source it covers, and nothing of its neighbors.
func TestVerbatimWritesANodeBack(t *testing.T) {
	t.Parallel()

	const src = "# lead\nname: &a Pet   # trail\nlist:\n  - 1\n  - !!str two\nlit: |\n  body\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	var checked int
	for _, doc := range file.Docs {
		walkEveryNode(doc, func(n ast.Node) {
			var out bytes.Buffer
			var carryingAToken int
			labeling := ast.NewRenderer(
				ast.WithSource([]byte(src)),
				ast.WithTransform(func(w io.Writer, s ast.Written) error {
					if s.Token != nil {
						carryingAToken++
					}
					_, err := w.Write(s.Text)

					return err
				}),
			)
			require.NoError(t, labeling.Verbatim(&out, n))
			if out.Len() == 0 {
				return
			}
			checked++
			require.Containsf(t, src, out.String(),
				"a %s wrote %q, which the document does not hold", n.Type(), out.String())

			// Verbatim ends on upTo(span.to), so a node writes its own stretch of
			// the document whether the descent ran or not. What the descent adds
			// is the naming, and that is what is checked.
			require.Positivef(t, carryingAToken,
				"a %s wrote %q and named none of it", n.Type(), out.String())
		})
	}
	require.Positive(t, checked)
}

// suiteDocumentsAccepted is how many Test Suite documents the parser accepts and the descent is measured over. It may
// not fall.
//
// Every token the tree holds reaches a transform as the token it is, in every suite document and every seed. It was
// 10 suite documents and 1,259 seeds until a ':' or a flow collection's closer stopped going out as filler in front of
// a comment written after it, "hr: # c", and a block scalar's blank content began to cover the lines it holds. A token
// of no width, the content of an empty block scalar, holds no byte to label and is not counted.
const suiteDocumentsAccepted = 308

// TestVerbatimRebuildsEveryDocument is the verbatim census: for every document
// the parser accepts, src == VerbatimFile(ParseBytes(src)).
//
// The equality is weak evidence on this path. VerbatimFile copies the source
// forward and ends on upTo(len(r.src)), so with Renderer.write deleted outright
// every document still comes back byte for byte. transform.Walk has no such
// tail -- it joins the document out of token tiles -- which is why
// TestIdentityRebuildsTheCorpus can test the same invariant by byte equality
// alone.
//
// So the census also checks that every token the tree holds reaches the
// transform labeled with the token it came from. walkSourceTokens finds them
// through a switch of its own, so the two traversals have to agree about what
// the document contains. Containment and not equality: the descent also labels
// comment tokens, which walkSourceTokens does not hand over.
//
// No document is exempt from the equality. Escaping, a leading byte order mark
// and surrogate pairs distort a document rebuilt from token values, since a
// token holds the unescaped text and records nothing about how it was spelled
// -- but this path copies the source between two offsets and never reads the
// value. Invalid UTF-8 does not reach the renderer: the parser refuses it.
func TestVerbatimRebuildsEveryDocument(t *testing.T) {
	t.Parallel()

	var accepted, rebuilt, suiteN, suiteUnlabeled, seedUnlabeled int
	var differing, unlabeled []string

	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}
		accepted++

		var joined bytes.Buffer
		var written []int32
		record := func(_ io.Writer, s ast.Written) error {
			joined.Write(s.Text)
			if s.Token != nil {
				written = append(written, s.Token.EndOffset())
			}

			return nil
		}

		renderer := ast.NewRenderer(ast.WithSource([]byte(src.text)), ast.WithTransform(record))
		require.NoErrorf(t, renderer.VerbatimFile(io.Discard, file), "%s: verbatim failed", src.name)

		if joined.String() == src.text {
			rebuilt++
		} else if len(differing) < 10 {
			differing = append(differing, src.name)
		}

		var held []int32
		for _, doc := range file.Docs {
			held = append(held, ast.SourceTextEnds(doc)...)
		}
		missing := slices.ContainsFunc(held, func(end int32) bool {
			return !slices.Contains(written, end)
		})

		switch {
		case !strings.HasPrefix(src.name, "suite/"):
			if missing {
				seedUnlabeled++
			}
		default:
			suiteN++
			if missing {
				suiteUnlabeled++
				if len(unlabeled) < 12 {
					unlabeled = append(unlabeled, src.name)
				}
			}
		}
	}

	require.Positive(t, accepted)
	t.Logf("verbatim: %d of %d accepted documents rebuilt byte for byte -- which a run with Renderer.write deleted also manages, so read the next line",
		rebuilt, accepted)
	t.Logf("descent: a token the tree holds arrives unlabeled in %d of %d suite documents and %d seeds",
		suiteUnlabeled, suiteN, seedUnlabeled)

	require.Equalf(t, accepted, rebuilt,
		"%d documents do not come back as they were written, starting with %v", accepted-rebuilt, differing)
	require.GreaterOrEqualf(t, suiteN, suiteDocumentsAccepted,
		"%d suite documents are measured where %d were: fewer are accepted than were", suiteN, suiteDocumentsAccepted)
	require.Zerof(t, suiteUnlabeled, "%d suite documents hold a token that arrives unlabeled: %v", suiteUnlabeled, unlabeled)
	require.Zerof(t, seedUnlabeled, "%d seeds hold a token that arrives unlabeled", seedUnlabeled)
}

// TestVerbatimKeepsWhatARebuildFromValuesWouldLose pins the four distortions
// that make "verbatim" a claim worth testing, and that a renderer working from
// token values cannot avoid.
//
// A token holds the unescaped text. Nothing on it records whether "é" was
// written as a literal, as "\u00e9" or as "\xC3\xA9", so a document rebuilt
// from values has to pick a spelling and will pick the wrong one. The verbatim
// path never reads the value: it copies the source between two offsets, so the
// spelling survives because it is never decoded.
func TestVerbatimKeepsWhatARebuildFromValuesWouldLose(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"byte order mark", "\uFEFFa: 1\n"},
		{"byte order mark before a document", "a: 1\n\uFEFF---\nb: 2\n"},
		{"escaped line break", "a: \"x\\ny\"\n"},
		{"escaped code point", "a: \"\\u00e9\"\n"},
		{"escaped byte", "a: \"\\x41\"\n"},
		{"surrogate pair", "a: \"\\uD83D\\uDE00\"\n"},
		{"the same character written out", "a: \"\U0001F600\"\n"},
		{"doubled quote in a single-quoted scalar", "a: 'it''s'\n"},
		{"carriage returns", "a: 1\r\nb: 2\r\n"},
		{"no trailing line break", "a: 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)

			requireWrittenThroughTheDescent(t, tc.src, file)
		})
	}
}

// TestInvalidUTF8NeverReachesTheRenderer is the fourth distortion, and the
// parser settles it before rendering is reached.
func TestInvalidUTF8NeverReachesTheRenderer(t *testing.T) {
	t.Parallel()

	_, err := parser.ParseBytes([]byte("a: \"\xff\xfe\"\n"), parser.WithComments())
	require.ErrorContains(t, err, "found a byte that is part of no character")
}

// requireWrittenThroughTheDescent renders file and requires two things of the
// result: it is the document byte for byte, and it was written by the descent.
//
// The second is not implied by the first, which is the trap this package keeps
// walking into. VerbatimFile ends on upTo(len(src)), so a run with
// Renderer.write deleted outright hands the whole document over as one stretch
// carrying no token, and every byte-identity check in the package passes. What
// the descent adds is that the stretches are named, so that is what is asked.
func requireWrittenThroughTheDescent(t *testing.T, src string, file *ast.File) {
	t.Helper()

	var out bytes.Buffer
	var carryingAToken int
	renderer := ast.NewRenderer(
		ast.WithSource([]byte(src)),
		ast.WithTransform(func(w io.Writer, s ast.Written) error {
			if s.Token != nil {
				carryingAToken++
			}
			_, err := w.Write(s.Text)

			return err
		}),
	)

	require.NoError(t, renderer.VerbatimFile(&out, file))
	require.Equal(t, src, out.String())
	require.Positive(t, carryingAToken,
		"the document came back whole and not one stretch of it carried a token")
}
