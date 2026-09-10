// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestCloneLetsANodeBePutSomewhereElse is what Clone exists for.
//
// A verbatim rendering copies the document forward once, so a node is written
// once and where the document wrote it. Reaching the same node from a second
// place, or moving it, leaves the second place with no source ahead of the copy.
// A clone claims no source and is laid out where it was put.
func TestCloneLetsANodeBePutSomewhereElse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		src   string
		place func(*ast.File)
		want  string
	}{
		{
			"a value used again", "a: 1\nb: 2\n",
			func(f *ast.File) {
				m := f.Docs[0].Body.(*ast.MappingNode)
				m.Values[1].Value = ast.Clone(m.Values[0].Value)
			},
			"a: 1\nb: 1\n",
		},
		{
			"an entry copied beside itself", "a: 1\n",
			func(f *ast.File) {
				m := f.Docs[0].Body.(*ast.MappingNode)
				m.Values = append(m.Values, m.Values[0].Clone())
			},
			"a: 1\na: 1\n",
		},
		{
			"an entry taken from another document", "a: 1\n---\nb: 2\n",
			func(f *ast.File) {
				lifted := f.Docs[1].Body.(*ast.MappingNode).Values[0]
				first := f.Docs[0].Body.(*ast.MappingNode)
				first.Values = append(first.Values, lifted.Clone())
			},
			"a: 1\nb: 2\n---\nb: 2\n",
		},
		{
			"a mapping written under a key", "base:\n  k: v\nuse: x\n",
			func(f *ast.File) {
				m := f.Docs[0].Body.(*ast.MappingNode)
				m.Values[1].Value = ast.Clone(m.Values[0].Value)
			},
			"base:\n  k: v\nuse:\n  k: v\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			tc.place(file)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())
		})
	}
}

// TestTheSameNodeInTwoPlacesIsWrittenOnce records the constraint Clone exists to
// work around, so that it fails here if it ever changes.
//
// Putting the node itself rather than a copy of it leaves the document as it
// was, and says so with a nil error. There is nothing to write: the bytes the
// node names stand where the document put them, and the copy has already gone
// past.
func TestTheSameNodeInTwoPlacesIsWrittenOnce(t *testing.T) {
	t.Parallel()

	const src = "a: 1\nb: 2\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	mapping := file.Docs[0].Body.(*ast.MappingNode)
	mapping.Values[1].Value = mapping.Values[0].Value

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, src, out.String(), "the second use of a node is written as nothing")
}

// TestACloneSharesNothingWithWhatItCopied checks the copy is deep: editing one
// leaves the other as it was, tokens and comments included.
func TestACloneSharesNothingWithWhatItCopied(t *testing.T) {
	t.Parallel()

	const src = "# head\na: 1  # beside\nlist:\n  - one\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	original := file.Docs[0].Body.(*ast.MappingNode)
	cloned := original.Clone()

	cloned.Values[0].Key.GetToken().Value = "changed"
	cloned.Values[0].Value.GetToken().Value = "99"
	// The head comment hangs on the entry and the one beside it on the value.
	cloned.Values[0].GetComment().Comments[0].Token.Value = " rewritten"
	cloned.Values[0].Value.GetComment().Comments[0].Token.Value = " also rewritten"
	cloned.Values[1].Value.(*ast.SequenceNode).Values[0].GetToken().Value = "two"

	require.Equal(t, "a", original.Values[0].Key.GetToken().Value)
	require.Equal(t, "1", original.Values[0].Value.GetToken().Value)
	require.Equal(t, " head", original.Values[0].GetComment().Comments[0].Token.Value)
	require.Equal(t, " beside", original.Values[0].Value.GetComment().Comments[0].Token.Value)
	require.Equal(t, "one", original.Values[1].Value.(*ast.SequenceNode).Values[0].GetToken().Value)

	// And the original still renders as the document wrote it.
	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, src, out.String())
}

// TestCloningTheCorpusChangesNothing asks two things of every node the corpus
// holds: the clone writes what the original writes, and it claims no source.
//
// The second is what makes the first useful. A clone that kept its tokens marked
// would render the same here and be written as nothing the moment it was put
// anywhere.
func TestCloningTheCorpusChangesNothing(t *testing.T) {
	t.Parallel()

	renderer := ast.NewRenderer()

	var nodes, differ, claiming int
	var wrong []string
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}
		for _, doc := range file.Docs {
			walkEveryNode(doc, func(n ast.Node) {
				nodes++
				cloned := ast.Clone(n)

				if renderer.String(cloned) != renderer.String(n) {
					differ++
					if len(wrong) < 5 {
						wrong = append(wrong, src.name+": "+renderer.String(n))
					}

					return
				}
				if claimsSource(cloned) {
					claiming++
				}
			})
		}
	}

	require.Positive(t, nodes)
	t.Logf("cloned %d nodes: %d render differently, %d still claim the source", nodes, differ, claiming)
	require.Zerof(t, differ, "%d clones do not write what they copied, starting with %v", differ, wrong)
	require.Zerof(t, claiming, "%d clones still hold a token marked as read from the document", claiming)
}

// claimsSource reports whether any token under n is still marked as read from a
// document.
func claimsSource(n ast.Node) bool {
	var found bool
	walkEveryNode(n, func(n ast.Node) {
		if tk := n.GetToken(); tk != nil && tk.FromSource() {
			found = true
		}
	})

	return found
}

// TestCloneCopiesEveryFieldThatPointsSomewhere walks a clone against what it
// copied with reflection and fails on any field the two still share.
//
// Clone is written out by hand, one method per node type, so a type that grows a
// field loses it silently: the struct copy carries the new pointer across and
// the clone shares it. That happened the day after Clone landed --
// BaseNode.HeadComment was added and cloneBase went on copying Comment alone,
// and 17 of the corpus's nodes rendered differently from what they copied. The
// render census found it; this finds it at the field, which is where the fix is.
//
// Three fields are shared on purpose, being references to nodes a clone does not
// own. They are named here so that adding a fourth is a decision and not an
// omission.
func TestCloneCopiesEveryFieldThatPointsSomewhere(t *testing.T) {
	t.Parallel()

	shared := map[string]string{
		"AliasNode":    "Target",
		"DocumentNode": "Anchors",
	}

	var checked int
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}
		for _, doc := range file.Docs {
			walkEveryNode(doc, func(n ast.Node) {
				cloned := ast.Clone(n)
				require.NotNil(t, cloned)
				checked++

				original := reflect.ValueOf(n).Elem()
				copied := reflect.ValueOf(cloned).Elem()
				require.Equal(t, original.Type(), copied.Type())

				name := original.Type().Name()
				for i := range original.NumField() {
					field := original.Type().Field(i)
					if !field.IsExported() || shared[name] == field.Name {
						continue
					}
					if field.Name == "BaseNode" {
						assertUnshared(t, src.name, name, original.Field(i), copied.Field(i), shared)

						continue
					}
					assertFieldUnshared(t, src.name, name, field.Name, original.Field(i), copied.Field(i))
				}
			})
		}
	}
	require.Positive(t, checked)
	t.Logf("compared %d clones field by field", checked)
}

// assertUnshared walks an embedded struct's own fields.
func assertUnshared(t *testing.T, doc, owner string, original, copied reflect.Value, shared map[string]string) {
	t.Helper()

	for i := range original.NumField() {
		field := original.Type().Field(i)
		if !field.IsExported() || shared[original.Type().Name()] == field.Name {
			continue
		}
		assertFieldUnshared(t, doc, owner, field.Name, original.Field(i), copied.Field(i))
	}
}

// assertFieldUnshared fails when a field of the clone points where the original
// points. Anything that is not a reference is carried by the struct copy and is
// nothing to check.
func assertFieldUnshared(t *testing.T, doc, owner, field string, original, copied reflect.Value) {
	t.Helper()

	switch original.Kind() {
	case reflect.Pointer:
		if original.IsNil() {
			return
		}
	case reflect.Map, reflect.Slice:
		if original.IsNil() || original.Len() == 0 {
			return
		}
	default:
		// Anything else is carried by the struct copy and shares nothing.
		return
	}

	require.NotEqualf(t, original.Pointer(), copied.Pointer(),
		"%s: %s.%s is shared with what it was cloned from -- add it to Clone, or name it in the shared list with a reason",
		doc, owner, field)
}
