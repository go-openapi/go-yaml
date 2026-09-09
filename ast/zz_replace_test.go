// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestVerbatimWritesAReplacedNode covers a caller assigning over a node the
// document holds: MappingValueNode.Key, MappingValueNode.Value and
// DocumentNode.Body.
//
// The node that knew where the old text was is gone, so the copy cannot be given
// a bound and is told to drop what it meets before the next token instead. Every
// case here is one where that drop has to stop somewhere exact: at the break
// before the next entry, at the spaces in front of a comment, after the last
// line of a block.
func TestVerbatimWritesAReplacedNode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		src     string
		replace func(*testing.T, *ast.File)
		want    string
	}{
		{
			"a value", "a: 1\nb: 2\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, "replaced") },
			"a: replaced\nb: 2\n",
		},
		{
			"the last value", "a: 1\nb: 2\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 1, "replaced") },
			"a: 1\nb: replaced\n",
		},
		{
			"a key", "a: 1\nb: 2\n",
			func(t *testing.T, f *ast.File) { setKey(t, f, 0, "k2") },
			"k2: 1\nb: 2\n",
		},
		{
			"a key and its value", "a: 1\nb: 2\n",
			func(t *testing.T, f *ast.File) { setKey(t, f, 0, "k2"); setValue(t, f, 0, 7) },
			"k2: 7\nb: 2\n",
		},
		{
			"a value written in quotes", "a: \"one\"\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, "two") },
			"a: two\n",
		},
		{
			"a value written as a block scalar", "a: |\n  body\nb: 2\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, "short") },
			"a: short\nb: 2\n",
		},
		{
			"a value written below its key", "a:\n  \"over\n   two lines\"\nb: 2\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, "short") },
			"a: short\nb: 2\n",
		},
		{
			"a value in a flow mapping", "{a: 1, b: 2}\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, "replaced") },
			"{a: replaced, b: 2}\n",
		},
		{
			"a document with no trailing break", "a: 1",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, 2) },
			"a: 2",
		},
		{
			"the whole body", "a: 1\nb: 2\n",
			func(t *testing.T, f *ast.File) {
				built, err := codec.ValueToNode(map[string]any{"z": 1})
				require.NoError(t, err)
				f.Docs[0].Body = built
			},
			"z: 1\n",
		},
		{
			"a value that becomes a mapping", "a: 1\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, map[string]any{"p": 1, "q": 2}) },
			"a:\n  p: 1\n  q: 2\n",
		},
		{
			"a value that becomes a sequence", "a: 1\n",
			func(t *testing.T, f *ast.File) { setValue(t, f, 0, []any{1, 2}) },
			"a:\n- 1\n- 2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			tc.replace(t, file)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())
		})
	}
}

// TestVerbatimIndentsAReplacedBlockFromTheDocument checks the one thing the
// layout renderer cannot do: a collection put in place of a scalar is indented
// from the line it lands on, not from its depth in the tree.
func TestVerbatimIndentsAReplacedBlockFromTheDocument(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src, want string }{
		{"two spaces", "root:\n  a: 1\n", "root:\n  a:\n    p: 1\n"},
		{"four spaces", "root:\n    a: 1\n", "root:\n    a:\n      p: 1\n"},
		{"one space", "root:\n a: 1\n", "root:\n a:\n   p: 1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			built, err := codec.ValueToNode(map[string]any{"p": 1})
			require.NoError(t, err)

			outer := file.Docs[0].Body.(*ast.MappingNode)
			outer.Values[0].Value.(*ast.MappingNode).Values[0].Value = built

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())
		})
	}
}

// TestReplacingAValueDropsTheCommentOnIt records a consequence of where the
// parser puts a comment rather than a decision the renderer makes.
//
// "a: 1  # note" attaches the note to the value node and not to the entry, since
// an entry written the short way has nowhere else to put it. Assigning over the
// value therefore replaces the node the note belonged to, and the note goes with
// it. A caller who wants it kept sets it on the node they put in.
func TestReplacingAValueDropsTheCommentOnIt(t *testing.T) {
	t.Parallel()

	const src = "# top\na: 1  # note\nb: 2\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	require.NotNil(t, file.Docs[0].Body.(*ast.MappingNode).Values[0].Value.GetComment(),
		"the note is the value's, which is what this test is about")

	setValue(t, file, 0, "replaced")

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, "# top\na: replaced\nb: 2\n", out.String())
}

// TestAssigningOverACollectionEntryInsertsInstead records the limit of the
// design, so that it fails here rather than surprising a caller.
//
// Assigning to MappingNode.Values[i] or SequenceNode.Values[i] leaves a tree
// that cannot be told from one where an entry was inserted at i: both hold a
// node the document does not, at the same index, with the same siblings. The
// renderer reads it as an insertion and the entry that was there stays. Replace
// the entry's Value or Key instead, which name one slot each.
func TestAssigningOverACollectionEntryInsertsInstead(t *testing.T) {
	t.Parallel()

	const src = "- 1\n- 2\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	built, err := codec.ValueToNode(9)
	require.NoError(t, err)
	file.Docs[0].Body.(*ast.SequenceNode).Values[0] = built

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, "- 1\n- 9\n- 2\n", out.String())
}

func setValue(t *testing.T, f *ast.File, i int, v any) {
	t.Helper()

	built, err := codec.ValueToNode(v)
	require.NoError(t, err)
	f.Docs[0].Body.(*ast.MappingNode).Values[i].Value = built
}

func setKey(t *testing.T, f *ast.File, i int, v any) {
	t.Helper()

	built, err := codec.ValueToNode(v)
	require.NoError(t, err)
	f.Docs[0].Body.(*ast.MappingNode).Values[i].Key = built.(ast.MapKeyNode)
}

// TestReplacingAValueAcrossTheCorpus replaces one scalar value in every document
// that holds a plain one, and asks three things of the result: it parses, it
// holds what was put in, and it differs from the document in one run and nothing
// else.
//
// Documents naming anchors are left out: removing any node breaks the aliases
// pointing at it, which is the caller's doing and says nothing about placement.
// So are values carrying a tag or an anchor, since replacing one takes the
// property with it.
//
// replacedDocuments is how many that leaves, and it is checked with the three.
// It follows the corpus and the acceptance line; the three follow the renderer.
// Zero failures over a set that quietly shrank is not the same result, and only
// the denominator says which happened.
const replacedDocuments = 1005

func TestReplacingAValueAcrossTheCorpus(t *testing.T) {
	t.Parallel()

	const marker = "x-marked-9"

	var tried, unreadable, lost, disturbed int
	var wrong []string
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil || len(file.Docs) == 0 || strings.ContainsAny(src.text, "&*") {
			continue
		}
		entry := firstPlainScalarEntry(file.Docs[0].Body)
		if entry == nil {
			continue
		}
		built, err := codec.ValueToNode(marker)
		require.NoError(t, err)
		entry.Value = built
		tried++

		var out bytes.Buffer
		require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src.text))).VerbatimFile(&out, file))

		if _, err := parser.ParseBytes(out.Bytes(), parser.WithComments()); err != nil {
			unreadable++

			continue
		}
		if !strings.Contains(out.String(), marker) {
			lost++

			continue
		}

		// The run can come out shorter than the marker where the two share a
		// character with the text they replaced, which the common prefix and
		// suffix absorb.
		run := strings.TrimSpace(changedRun(src.text, out.String()))
		if run != marker && !strings.Contains(marker, run) {
			disturbed++
			if len(wrong) < 8 {
				wrong = append(wrong, src.name+": "+run)
			}
		}
	}

	require.Positive(t, tried)
	t.Logf("replaced one value in %d documents (recorded %d): %d no longer parse, %d lost it, %d changed something else",
		tried, replacedDocuments, unreadable, lost, disturbed)

	require.GreaterOrEqualf(t, tried, replacedDocuments,
		"%d documents were measured where %d were: fewer reach the replacement than did, so zero failures below says less than it did",
		tried, replacedDocuments)
	require.Zerof(t, unreadable, "%d documents no longer parse", unreadable)
	require.Zerof(t, lost, "%d documents lost the value that was put in", lost)
	require.Zerof(t, disturbed, "%d documents changed somewhere else as well: %v", disturbed, wrong)
}

// firstPlainScalarEntry finds an entry whose value the document wrote as a
// scalar carrying no property of its own.
func firstPlainScalarEntry(n ast.Node) *ast.MappingValueNode {
	var found *ast.MappingValueNode
	walkEveryNode(n, func(n ast.Node) {
		if found != nil {
			return
		}
		entry, isEntry := n.(*ast.MappingValueNode)
		if !isEntry || entry.Key.GetToken() == nil {
			return
		}
		switch entry.Value.(type) {
		case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.TagNode, *ast.AliasNode, nil:
			return
		}
		if tk := entry.Value.GetToken(); tk != nil && tk.FromSource() {
			found = entry
		}
	})

	return found
}

// changedRun is what after holds between its common prefix with before and its
// common suffix with it.
func changedRun(before, after string) string {
	head := 0
	for head < len(before) && head < len(after) && before[head] == after[head] {
		head++
	}
	tail := 0
	for tail < len(before)-head && tail < len(after)-head &&
		before[len(before)-1-tail] == after[len(after)-1-tail] {
		tail++
	}

	return after[head : len(after)-tail]
}
