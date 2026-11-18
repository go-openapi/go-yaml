// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

var updateGolden = flag.Bool("update", false, "rewrite ast/testdata/render_golden.tsv from what the renderer does now")

const goldenPath = "testdata/render_golden.tsv"

// renderConfigs are the option sets the golden covers, so that a caller driving
// the renderer themselves is held as well as the default path.
func renderConfigs() []struct {
	name string
	opts []ast.RenderOption
} {
	return []struct {
		name string
		opts []ast.RenderOption
	}{
		{"default", nil},
		{"nocomments", []ast.RenderOption{ast.WithComments(false)}},
		{"indent4", []ast.RenderOption{ast.WithIndent(4)}},
		{"indentseq", []ast.RenderOption{ast.WithIndentSequence(true)}},
	}
}

// TestTheRendererWritesWhatItWroteBefore pins ast.Renderer's exact output.
//
// Every document of the YAML Test Suite gets a line of its own, so a change
// names the document it broke. The fuzz seeds are 12,000-odd and would drown
// that, so they are folded into one hash per configuration -- over the sorted
// per-document hashes, which makes it independent of the order the seeds are
// generated in, though not of which documents they are.
//
// It is a golden and not a judgement: nothing here says the output is right, and
// 131 documents are known to render to a different document than the one that
// went in. Regenerate deliberately with -update when a rendering change is
// intended, and read the diff.
func TestTheRendererWritesWhatItWroteBefore(t *testing.T) {
	t.Parallel()

	got := renderAll(t)
	if *updateGolden {
		require.NoError(t, os.WriteFile(goldenPath, []byte(strings.Join(got, "\n")+"\n"), 0o600))
		t.Logf("wrote %s with %d lines", goldenPath, len(got))

		return
	}

	raw, err := os.ReadFile(goldenPath)
	require.NoErrorf(t, err, "run: go test ./ast/ -run TestTheRendererWritesWhatItWroteBefore -update")
	want := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")

	require.Equalf(t, len(want), len(got), "the golden holds %d lines and this run produced %d", len(want), len(got))

	var differed int
	for i := range want {
		if want[i] != got[i] {
			differed++
			if differed <= 10 {
				t.Errorf("golden line %d\n  was %s\n  now %s", i+1, want[i], got[i])
			}
		}
	}
	if differed > 10 {
		t.Errorf("and %d more lines differ", differed-10)
	}
}

// renderAll renders every document under every configuration and returns the
// golden's lines.
func renderAll(t *testing.T) []string {
	t.Helper()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	lines := make([]string, 0, len(suites)*len(renderConfigs())+len(renderConfigs()))
	for _, cfg := range renderConfigs() {
		r := ast.NewRenderer(cfg.opts...)

		for _, s := range suites {
			lines = append(lines, fmt.Sprintf("%s\tsuite/%s\t%s", cfg.name, s.Name, digestOf(r, string(s.InYAML))))
		}

		seedDigests := make([]string, 0, len(seeds))
		for _, src := range seeds {
			seedDigests = append(seedDigests, digestOf(r, src))
		}
		sort.Strings(seedDigests)
		sum := sha256.Sum256([]byte(strings.Join(seedDigests, "\n")))
		lines = append(lines, fmt.Sprintf("%s\tseeds-aggregate\t%s", cfg.name, hex.EncodeToString(sum[:])))
	}

	return lines
}

// digestOf renders src and returns a short hash of the result, or a word saying
// why there was none.
func digestOf(r *ast.Renderer, src string) string {
	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	if err != nil {
		return "refused"
	}
	sum := sha256.Sum256([]byte(r.File(file)))

	return hex.EncodeToString(sum[:8])
}
