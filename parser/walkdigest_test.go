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

// TestWalkDigest logs one digest over everything a walk hands to a visitor, for every document of the corpus.
//
// Use it to check the arena rewind: run it with and without -tags yamlprobe and compare the two digests.
// The tag makes a rewind clear the cells it reuses,
// so a walk that reads a node it has already handed over reads an empty one and the digest moves.
// The same digest both ways means no read went past its own handover.
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

// TestTheWalkHandsOverTheSameTree pins the digest of a walk over the YAML Test Suite and the synthetic corpus.
//
// digestVisitor writes each node's type, the step's depth, index and key,
// the line and column, the byte span and the value,
// so the digest moves on any change to the shape of the tree or to where its tokens stand.
// The other checks score what a document means, renders as or is rejected for,
// and a change that moves every token's column by one, or renames a node type, passes all of them.
//
// The digest catches a change that alters every tree the same way,
// and misses one that alters a shape no source document holds.
// A pin holds such a shape once it is found: see TestEveryLedgerEntryNamesItsPin in internal/testintegration.
//
// The fuzz seeds are left out, because they come from the generated corpus artifact,
// and regenerating it moves the digest for a reason unrelated to the parser.
// The YAML Test Suite and the generators in internal/corpus are fixed, so the digest moves only when the parser does.
//
// A failure means the tree changed, not that it is wrong.
// Run TestWalkDigest with and without -tags yamlprobe to see where,
// then re-baseline fixedWalkDigest in the commit that moved it, as parserComplaints is re-baselined in yamlcorpus.
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

// fixedWalkDigest is the expected digest of a walk over the YAML Test Suite and the synthetic corpus.
//
// Re-baselined on 2026-09-11 when a tab began to move the column: the positions changed in 925 of the 19,838
// sources, 9 of them Test Suite documents, and every one of the 925 holds a tab. No source is accepted or refused
// differently.
//
// Re-baselined again the same day when a double-quoted scalar's end began to take the blanks after a dropped tab.
// One source moved, suite/trailing-tabs-in-double-quoted/05, whose token now ends on its closing quote.
//
// And once more when a plain scalar continued by a "- " line began to start where its text does. One source moved,
// suite/sequence-entry-that-looks-like-two-with-wrong-indentation, whose token now stands on line 1.
//
// Re-baselined on 2026-09-15 when the key window began releasing a flow collection that cannot stand as a key. One
// source moved, suite/wrong-indented-flow-sequence, and only in what the walk sees before the document is refused:
// the grouping used to hold the whole sequence, so the visitor received a null for "flow:" and then the error, and
// it now receives the sequence and the "a" the parse did read. The complaint is unchanged -- "[2:1] a flow
// collection continues on a line that is not indented past the one it started on" -- and so is every other source.
const fixedWalkDigest = "d269cc4da95606027d0ac1050dca62463bec3f16f5d6f76057f40cbb8ebad237"

// digestVisitor writes each node it is handed, so a node the walk reads from a reused cell changes the digest.
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

// TestACommentDoesNotHandANodeOverTwice compares a walk of a commented document with a walk of the bare one.
//
// parseComment runs inside parseToken, which hands over the node it returns.
// The test catches parseComment calling parseToken again for the node after the comment,
// which hands that node over twice.
// A collection hands itself over as it opens and closes,
// so the cases that show the fault have a scalar or a directive as the document body.
//
// It counts by node type and not by pointer: a walk reuses its cells, so two nodes of one document can share a pointer.
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

// walkTypeCounts returns how many nodes of each type one walk of src hands over.
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
