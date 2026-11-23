// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
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

	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
)

// statedShapes are the enumerated families whose shapes may state a meaning.
func statedShapes() map[string][]stance.Shape {
	return map[string][]stance.Shape{
		"key":        yamlbuild.KeyShapes(),
		"merge":      yamlbuild.MergeShapes(),
		"tag":        yamlbuild.TagShapes(),
		"directive":  yamlbuild.DirectiveShapes(),
		"separation": yamlbuild.SeparationShapes(),
		"reach":      yamlbuild.ReachShapes(),
	}
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
