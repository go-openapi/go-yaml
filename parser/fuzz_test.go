// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/parser"
)

// parseModes holds the two comment settings a caller can choose between.
// WithComments changes which tokens reach the parser, so each setting runs its own code path.
var parseModes = map[string][]parser.Option{
	"default":  nil,
	"comments": {parser.WithComments()},
}

func FuzzParserParseBytes(f *testing.F) {
	addSuiteSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		for name, opts := range parseModes {
			file, err := parser.ParseBytes([]byte(src), opts...)
			if err != nil {
				// A rejected stream comes back with the documents before the one that failed, each complete:
				// rendering them must not panic, and a second parse must return the same ones and the same error.
				require.NotNilf(t, file, "%s: no file with the error for %q", name, src)
				rendered := file.String()

				again, againErr := parser.ParseBytes([]byte(src), opts...)
				require.Errorf(t, againErr, "%s: parse is not stable for %q", name, src)
				assert.Equalf(t, err.Error(), againErr.Error(), "%s: the error is not stable for %q", name, src)
				assert.Equalf(t, rendered, again.String(), "%s: the documents before the error are not stable for %q", name, src)

				continue
			}

			require.NotNilf(t, file, "%s: neither a file nor an error for %q", name, src)

			// Rendering an accepted document must not panic.
			rendered := file.String()

			// Parsing is a pure function of its input: the same source and mode must give the same answer every time.
			again, err := parser.ParseBytes([]byte(src), opts...)
			require.NoErrorf(t, err, "%s: parse is not stable for %q", name, src)
			assert.Equalf(t, rendered, again.String(), "%s: rendering is not stable for %q", name, src)
		}
	})
}

// FuzzParserWalk exercises the AST, by visiting every node of every document that parses and rendering each on its own.
//
// Tooling that walks the tree for positions or values calls String on nodes the library never renders in isolation,
// and this target covers that use.
func FuzzParserWalk(f *testing.F) {
	addSuiteSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		if err != nil {
			return
		}

		for _, doc := range file.Docs {
			if doc == nil {
				continue
			}

			ast.Walk(visitorFunc(func(node ast.Node) ast.Visitor {
				require.NotNil(t, node)
				_ = node.String()
				_ = node.Type()
				_ = node.GetToken()

				return nil
			}), doc)
		}
	})
}

// visitorFunc adapts a function to ast.Visitor.
type visitorFunc func(ast.Node) ast.Visitor

func (fn visitorFunc) Visit(node ast.Node) ast.Visitor { return fn(node) }

// addSuiteSeeds seeds a fuzz target with fuzzseeds.All: every document of the YAML Test Suite,
// valid and invalid alike, and the reduced inputs of bug reports.
func addSuiteSeeds(f *testing.F) {
	f.Helper()

	seeds, err := fuzzseeds.All()
	require.NoError(f, err)

	for _, src := range seeds {
		f.Add(src)
	}
}
