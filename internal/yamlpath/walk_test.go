// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/testcorpus"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// readByTree is how ReadNode read a path before it walked: parse the stream, then filter the file.
func (p *Path) readByTree(src []byte) (ast.Node, error) {
	f, err := parser.ParseBytes(src)
	if err != nil {
		return nil, err
	}

	return p.FilterFile(f)
}

// TestReadingAPathByWalkingAnswersAsTheTree holds readByWalking to FilterFile on the parsed stream.
//
// The paths are drawn from each document's own tree, so most of them address something: every key and index
// down to a few levels, "[*]" under each sequence, "..name" for each key, and a few that match nothing or
// ask a mapping for an index. Both readers must refuse alike, in the same words, and otherwise return nodes
// that render alike and carry the same path.
func TestReadingAPathByWalkingAnswersAsTheTree(t *testing.T) {
	t.Parallel()

	var checked, answered int
	for _, src := range pathSources(t) {
		file, err := parser.ParseBytes(src)
		if err != nil {
			continue
		}

		exprs := pathsOf(file)
		if len(src) > largeSource && len(exprs) > largePaths {
			// Each path reads the whole stream twice, and the corpus holds documents of several megabytes.
			exprs = exprs[:largePaths]
		}
		for _, expr := range exprs {
			path, err := PathString(expr)
			if err != nil {
				continue
			}
			checked++

			want, wantErr := path.readByTree(src)
			got, gotErr := path.readByWalking(src)
			if wantErr != nil || gotErr != nil {
				if !assert.Equalf(t, errText(wantErr), errText(gotErr), "%s on %q", expr, src) {
					return
				}

				continue
			}
			answered++
			if want.String() != got.String() {
				t.Errorf("%s on %q:\nwant %q\ngot  %q", expr, src, want.String(), got.String())

				return
			}
			if !assert.Equalf(t, want.GetPath(), got.GetPath(), "%s on %q", expr, src) {
				return
			}
		}
	}

	t.Logf("%d paths read, %d answered with a node", checked, answered)
	require.Greater(t, answered, checked/4, "most paths are drawn to address something")
}

// largeSource and largePaths cap the paths drawn from a large document.
const (
	largeSource = 64 << 10
	largePaths  = 12
)

// TestReadingAPathStopsAtTheAnswer checks that ReadNode reads no further than the node it returns.
//
// A document after the answer is not read, so an invalid one is not refused, where the tree reader parses
// the whole stream first and refuses it.
func TestReadingAPathStopsAtTheAnswer(t *testing.T) {
	t.Parallel()

	const src = "a: 1\n---\n- b\nc: 2\n"
	path, err := PathString("$.a")
	require.NoError(t, err)

	got, err := path.ReadNode(strings.NewReader(src))
	require.NoError(t, err)
	assert.Equal(t, "1", got.String())

	_, err = path.readByTree([]byte(src))
	require.Error(t, err, "the second document is invalid")
}

func errText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

// pathsOf draws paths from the first document of file that has a body.
func pathsOf(file *ast.File) []string {
	paths := []string{"$", "$.zzz_absent", "$[999]", "$[*]", "$..zzz_absent"}
	for _, doc := range file.Docs {
		if doc.Body == nil || doc.Body.Type() == ast.DirectiveType {
			continue
		}
		collectPaths(doc.Body, "$", 0, &paths)

		break
	}
	if len(paths) > 60 {
		paths = paths[:60]
	}

	return paths
}

func collectPaths(node ast.Node, prefix string, depth int, paths *[]string) {
	if depth > 3 || len(*paths) > 60 {
		return
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		*paths = append(*paths, prefix+"[0]")
		for i, entry := range n.Values {
			if i >= 4 {
				break
			}
			key := entry.Key.GetToken().Value
			if key == "" || strings.ContainsAny(key, ".*'[]$\\\" \t\n") {
				continue
			}
			*paths = append(*paths, prefix+"."+key, prefix+".."+key)
			collectPaths(entry.Value, prefix+"."+key, depth+1, paths)
		}
	case *ast.SequenceNode:
		*paths = append(*paths, prefix+"[*]", fmt.Sprintf("%s[%d]", prefix, len(n.Values)), prefix+".zzz")
		for i, value := range n.Values {
			if i >= 3 {
				break
			}
			index := fmt.Sprintf("%s[%d]", prefix, i)
			*paths = append(*paths, index)
			collectPaths(value, index, depth+1, paths)
			if i == 0 {
				// "[*]" with a step after it, read through the first entry's shape.
				var below []string
				collectPaths(value, prefix+"[*]", depth+1, &below)
				*paths = append(*paths, below...)
			}
		}
	}
}

func pathSources(t *testing.T) [][]byte {
	t.Helper()

	var sources [][]byte
	for _, dir := range []string{testcorpus.Dir(), filepath.Join(testcorpus.Dir(), "stress")} {
		for _, doc := range testcorpus.Docs(t, dir) {
			sources = append(sources, doc.Data)
		}
	}
	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, s := range suites {
		sources = append(sources, s.InYAML)
	}
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	for _, seed := range seeds {
		sources = append(sources, []byte(seed))
	}

	return sources
}

// BenchmarkReadNode reads one path from a workload by parsing a tree and by walking.
//
// The paths reach a scalar at the front of a document, a scalar behind the whole of it, and a subtree that is
// most of it: the walk holds what it returns, and still reads the stream to its end.
func BenchmarkReadNode(b *testing.B) {
	for _, c := range []struct{ doc, path string }{
		{"canada_geometry", "$.type"},
		{"golang_source", "$.username"},
		{"golang_source", "$.tree"},
		{"twitter_status", "$.search_metadata"},
		{"twitter_status", "$.statuses[3].user"},
		{"citm_catalog", "$..seatCategoryId"},
	} {
		src := testcorpus.ReadGzipped(b, filepath.Join(testcorpus.Dir(), c.doc+".yaml.gz"))
		path, err := PathString(c.path)
		require.NoError(b, err)

		for _, reader := range []struct {
			name string
			read func([]byte) (ast.Node, error)
		}{
			{"tree", path.readByTree},
			{"walk", path.readByWalking},
		} {
			b.Run(c.doc+"/"+c.path+"/"+reader.name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(src)))
				for b.Loop() {
					if _, err := reader.read(src); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
