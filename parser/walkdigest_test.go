// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// TestWalkDigest prints one digest over everything a walk hands to a visitor,
// for every document of the corpus.
//
// It is the A/B for the arena rewind: run it with and without -tags yamlprobe
// and compare the two digests. The tag makes a rewind clear the cells it hands
// back, so a walk that reads a node it has already handed over reads an empty
// one and the digest moves. Same digest both ways means nothing read past its
// own handover.
//
//	go test -run TestWalkDigest -v ./parser/ | grep digest
//	go test -tags yamlprobe -run TestWalkDigest -v ./parser/ | grep digest
func TestWalkDigest(t *testing.T) {
	sum := sha256.New()

	var documents, refused int
	for _, src := range walkSources(t) {
		d := &digestVisitor{out: sum}
		fmt.Fprintf(sum, "\n=== %s\n", src.name)
		if _, err := parser.New(parser.WithComments()).Walk([]byte(src.text), d); err != nil {
			fmt.Fprintf(sum, "refused: %v\n", err)
			refused++

			continue
		}
		documents++
	}

	t.Logf("digest %s over %d documents walked and %d refused",
		hex.EncodeToString(sum.Sum(nil)), documents, refused)
}

// TestTheWalkHandsOverTheSameTree pins the digest over the sources that do not
// move, and is what replaces TestLabParserMatchesProduction.
//
// That gate compared every tree the parser built against one built by a frozen
// copy of itself, node type by node type and position by position, over 18,554
// documents. internal/refparser is deleted, so nothing compares two trees any
// more -- and the live checks all score an *answer*: what a document means,
// what it renders as, which complaints the parser can make. A change that moved
// every token's column by one, or renamed a node type, would pass all of them.
//
// This catches that class for 64 hex characters instead of a stored tree dump.
// Over 417 documents, not the gate's 18,554 -- the difference is the generated
// corpus, which cannot be in here for the reason below.
//
// # What it does not catch, measured rather than reasoned
//
// Two reverts of real fixes, run against this digest:
//
//   - 28f926d, which decides the node a ':' keys on: the digest MOVES. That is
//     the uniform class, and it works.
//   - a6cc538, which measures an entry with no key from its own colon: the
//     digest is IDENTICAL. No document among the 417 holds an entry with no key
//     carrying a block scalar, so there is nothing for the change to move.
//
// So this catches a change that alters every tree the same way and misses one
// that alters a shape nobody wrote down. Adding documents does not fix that:
// none of the 91 hand-written shapes in yamlcorpus holds that shape either.
// Finding shapes nobody wrote down is what the generated corpus does, and
// holding one once found is what a pin does -- see TestEveryLedgerEntryNamesItsPin
// in internal/testintegration. This is the third thing, and only the third: it
// says the tree is the same tree.
// digestVisitor writes the node type, the walk step's depth, index and key, the
// line and column, the byte span and the value, so the digest moves on any
// change to the shape of the tree or to where its tokens stand.
//
// # Why not every source
//
// TestWalkDigest above runs the fuzz seeds too, which come out of the generated
// corpus artifact: regenerating it reshuffles all 14,000 and the digest moves
// for a reason that has nothing to do with the parser. This one takes the YAML
// Test Suite and the synthetic generators in internal/corpus, both of which are
// fixed, so the digest moves only when the parser does.
//
// # When it fails
//
// Read it as "the tree changed", not as "the tree is wrong" -- exactly as the
// gate's failures were read. Run TestWalkDigest with and without -tags
// yamlprobe to see where, then re-baseline here on the same commit as the change
// that moved it, the way parserComplaints is re-baselined in yamlcorpus.
func TestTheWalkHandsOverTheSameTree(t *testing.T) {
	sum := sha256.New()

	var documents, refused int
	for _, src := range walkSources(t) {
		if strings.HasPrefix(src.name, "fuzzseed/") {
			continue
		}

		d := &digestVisitor{out: sum}
		fmt.Fprintf(sum, "\n=== %s\n", src.name)
		if _, err := parser.New(parser.WithComments()).Walk([]byte(src.text), d); err != nil {
			fmt.Fprintf(sum, "refused: %v\n", err)
			refused++

			continue
		}
		documents++
	}

	t.Logf("digest over %d documents walked and %d refused", documents, refused)
	assert.Equal(t, fixedWalkDigest, hex.EncodeToString(sum.Sum(nil)),
		"the walk hands over a different tree than it did; see the doc comment before re-baselining")
}

// fixedWalkDigest is what the walk hands over today, over the 417 documents of
// the YAML Test Suite and the synthetic corpus -- 323 walked and 94 refused.
//
// Re-baselined 2026-09-09 with the commit that kept the blanks a line ends with
// out of a plain scalar's position and back in its extent. The counts held at
// 323 and 94, and no position moved: all 18 entries that shifted are span ends
// growing by the length of the blank run, which cursor.originTrimmed counts
// back into the token after removeRightSpaceFromBuf cuts it.
//
// Re-baselined 2026-09-09 before that, with the commit that admitted a zero-indented block
// sequence as an explicit key's body. The counts held at 323 and 94, so no
// document moved between walked and refused; what moved is the tree for a "?"
// whose content is such a sequence -- it used to end at the first "-", so the
// key was the empty node and the entries below it became a value.
//
// Re-baselined 2026-09-08 before that, with the commit that counted a tag's "!" in the
// column as well as in the offset. Every token standing after a tag on its line
// moved one column right, which is where it always addressed: digestVisitor
// writes the column, so any suite document holding a tag shifts the digest. The
// counts held at 323 and 94.
//
// Re-baselined 2026-09-08 before that, with the commit that stopped parseComment
// handing a node over twice. The counts did not move -- 323 and 94 before and after, so
// no document changed between walked and refused -- and every document that
// shifted the digest carries a comment, since parseComment runs only under
// WithComments and only on a comment token. What moved is a node behind a
// comment going over once instead of twice.
//
// Re-baselined 2026-09-08 before that, with the commit that made the merge key
// a YAML 1.1 type. The counts did not move -- 323 and 94 before and after -- so no
// document changed between walked and refused; what moved is the node a bare
// "<<" builds. Under the core schema the parser builds a String where it built
// a MergeKey, and digestVisitor writes the node's type, so every suite document
// holding a "<<" shifts the digest without changing what it reads as.
//
// Re-baselined 2026-09-07 before that, with the commit that joined the
// scanner's two tab checks: the counts held there too, and what moved was the
// message on a document refused either way, since digestVisitor writes
// "refused: %v" and one of the two messages was retired.
const fixedWalkDigest = "cbc3e1fec5d9852e7a5b1520f80e08e5357fc2ab052b40a69c5d4ac42011eb23"

// digestVisitor writes what it is handed, so that anything the walk reads out
// of a reclaimed cell shows up as a different document.
type digestVisitor struct {
	out interface{ Write([]byte) (int, error) }
}

func (d *digestVisitor) Enter(node ast.Node, at parser.Step) bool {
	d.write("enter", node, at)

	return true
}

func (d *digestVisitor) Leave(node ast.Node, at parser.Step) { d.write("leave", node, at) }

func (d *digestVisitor) write(what string, node ast.Node, at parser.Step) {
	tk := node.GetToken()
	value := ""
	var offset, end int32
	if tk != nil {
		value, offset, end = tk.Value, tk.Position.Offset(), tk.EndOffset()
	}
	fmt.Fprintf(d.out, "%s %v in=%v depth=%d idx=%d key=%v at=%d:%d span=%d:%d value=%q\n",
		what, node.Type(), at.In, at.Depth, at.Index, at.Key,
		at.At.Line, at.At.Column, offset, end, value)
}

type walkSource struct{ name, text string }

func walkSources(t *testing.T) []walkSource {
	t.Helper()

	var srcs []walkSource

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, s := range suites {
		srcs = append(srcs, walkSource{name: "suite/" + s.Name, text: string(s.InYAML)})
	}

	for _, c := range []struct {
		name string
		gen  func(int) string
	}{
		{"flat-map", corpus.FlatMap},
		{"flat-sequence", corpus.FlatSequence},
		{"nested-doc", corpus.NestedDoc},
		{"anchored", corpus.Anchored},
		{"block-scalars", corpus.BlockScalars},
	} {
		for _, n := range []int{1, 10, 200} {
			srcs = append(srcs, walkSource{name: fmt.Sprintf("corpus/%s-%d", c.name, n), text: c.gen(n)})
		}
	}

	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	for i, seed := range seeds {
		srcs = append(srcs, walkSource{name: fmt.Sprintf("fuzzseed/%04d", i), text: seed})
	}

	return srcs
}

// TestACommentDoesNotHandANodeOverTwice: a walk of a commented document hands
// the same nodes over as a walk of the same document without the comment.
//
// parseComment runs inside parseToken, which reports the node it returns, and
// it called parseToken again for the node after the comment -- so that node was
// reported twice. "# c" over "%YAML 1.2" gave Enter and Leave on one
// DirectiveNode twice in a row; "# c" over "foo" did it to the string. Found on
// the transform work, where a stream of nodes is joined to a stream of tokens
// and a repeat has nowhere to go.
//
// A collection hid it, because parseToken leaves those to hand themselves over
// as they open and close -- so the shapes that show it are the ones whose
// document body is a scalar or a directive.
//
// Counted by node type rather than by pointer: a walk hands its cells out again
// behind the descent, so two nodes of one document can be the same pointer and
// pointer identity says nothing.
func TestACommentDoesNotHandANodeOverTwice(t *testing.T) {
	for _, tc := range []struct{ commented, bare string }{
		{"# c\n%YAML 1.2\n---\na: 1\n", "%YAML 1.2\n---\na: 1\n"},
		{"# c\nfoo\n", "foo\n"},
		{"# c\n# d\nfoo\n", "foo\n"},
		{"# c\n- 1\n", "- 1\n"},
		{"# c\na: 1\n", "a: 1\n"},
		{"a: 1\n# c\nb: 2\n", "a: 1\nb: 2\n"},
	} {
		assert.Equalf(t, walkTypeCounts(t, tc.bare), walkTypeCounts(t, tc.commented),
			"the comment changed which nodes went over: %q", tc.commented)
	}
}

// walkTypeCounts is how many nodes of each type one walk hands over.
func walkTypeCounts(t *testing.T, src string) map[string]int {
	t.Helper()

	counts := &countingVisitor{counts: map[string]int{}}
	_, err := parser.New(parser.WithComments()).Walk([]byte(src), counts)
	require.NoErrorf(t, err, "%q", src)

	return counts.counts
}

type countingVisitor struct{ counts map[string]int }

func (c *countingVisitor) Enter(n ast.Node, _ parser.Step) bool {
	c.counts[fmt.Sprintf("%T", n)]++

	return true
}

func (c *countingVisitor) Leave(ast.Node, parser.Step) {}
