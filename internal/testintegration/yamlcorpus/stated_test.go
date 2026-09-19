// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// statedShapes are the enumerated families whose shapes may state a meaning.
func statedShapes() map[string][]stance.Shape {
	return map[string][]stance.Shape{
		"key":        yamlcorpus.KeyShapes(),
		"merge":      yamlcorpus.MergeShapes(),
		"tag":        yamlcorpus.TagShapes(),
		"directive":  yamlcorpus.DirectiveShapes(),
		"separation": yamlcorpus.SeparationShapes(),
		"reach":      yamlcorpus.ReachShapes(),
	}
}

// TestEveryStatedMeaningIsWhatTheLibraryReads holds the library to what the
// corpus says these documents denote.
//
// [stance.Shape.Means] is the specification's answer, so this is not a
// round-trip: it is the corpus telling the library what a document means and
// the library agreeing or not. A shape naming a [stance.Shape.Pin] is one we do
// not agree with yet, and the pin says what we do instead.
//
// # Which way each direction fails
//
// A shape with no pin must agree, and a disagreement fails: the corpus states
// what the specification settles, so the library is what moved.
//
// A shape with a pin must disagree, and agreement is *reported* rather than
// failed -- the same shape [yamlgen.Lax] uses. A defect being fixed should not
// turn a tree red; it should say "drop the pin" and let somebody drop it.
func TestEveryStatedMeaningIsWhatTheLibraryReads(t *testing.T) {
	var stated, pinned int

	for family, shapes := range statedShapes() {
		for _, s := range shapes {
			if s.Means == nil {
				continue
			}

			t.Run(family+"/"+s.Name, func(t *testing.T) {
				want, err := json.Marshal(s.Means)
				require.NoError(t, err, "the shape's own Means does not marshal")

				var got any
				readErr := codec.Unmarshal(s.Src, &got)

				if s.Pin != "" {
					pinned++

					if readErr != nil {
						t.Logf("still refused -- %q -- %s", s.Src, firstLine(readErr.Error()))

						return
					}

					encoded, merr := json.Marshal(yamlgen.NamedKeys(got))
					if merr != nil || string(encoded) != string(want) {
						t.Logf("still differs -- %q reads %s where the corpus says %s",
							s.Src, encoded, want)

						return
					}

					t.Logf("NOW AGREES -- %q -- drop Pin %q from the shape", s.Src, s.Pin)

					return
				}

				stated++

				require.NoErrorf(t, readErr, "the corpus states a meaning for %q and the library refuses it", s.Src)

				encoded, merr := json.Marshal(yamlgen.NamedKeys(got))
				require.NoError(t, merr)
				assert.Equalf(t, string(want), string(encoded),
					"%q: the corpus states what the specification settles, so the library is what moved", s.Src)
			})
		}
	}

	t.Logf("%d shapes state a meaning the library produces, %d state one it does not yet", stated, pinned)
}

// TestEveryShapePinNamesATest keeps a pin from outliving the test it names.
//
// The pins live in yamlgen's tests rather than here, because that is where the
// document was reduced and the oracles were asked. A pin naming nothing would
// leave a shape claiming a defect nobody can reproduce.
func TestEveryShapePinNamesATest(t *testing.T) {
	tests := testNamesIn(t, filepath.Join("..", "yamlgen"))

	for family, shapes := range statedShapes() {
		for _, s := range shapes {
			if s.Pin == "" {
				continue
			}

			assert.Containsf(t, tests, s.Pin,
				"%s/%s names pin %q and no test in yamlgen has that name", family, s.Name, s.Pin)
		}
	}
}

// testNamesIn returns every Test function declared in a directory's _test.go
// files.
func testNamesIn(t *testing.T, dir string) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	out := map[string]bool{}
	fset := token.NewFileSet()

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
			}
		}
	}

	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")

	return line
}
