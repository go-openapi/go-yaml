// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/ast/render_golden.tsv from what the renderer does now")

const goldenPath = "../testdata/ast/render_golden.tsv"

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
// The aggregates hold only for the seeds they were taken over, and the first line
// of the golden names that set by a hash of its sources. Run over another
// corpus -- the extended one a release ships beside the compact one -- the
// aggregates are left out and the Test Suite lines are still compared, so the
// test pins the renderer there too instead of failing on the corpus.
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

	if want[0] != got[0] {
		t.Logf("the golden's seed aggregates were taken over another seed set (%s), and this run loads %s: "+
			"they are left out, and the Test Suite lines are compared", want[0], got[0])
		want, got = withoutSeedAggregates(want), withoutSeedAggregates(got)
	}

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

// withoutSeedAggregates returns the golden's lines past the seed set's, with no
// seed aggregate among them.
func withoutSeedAggregates(lines []string) []string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines[1:] {
		if !strings.Contains(line, "\tseeds-aggregate\t") {
			kept = append(kept, line)
		}
	}

	return kept
}

// renderAll renders every document under every configuration and returns the
// golden's lines. The first names the seed set, by its size and a hash over its
// sorted sources.
func renderAll(t *testing.T) []string {
	t.Helper()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	lines := make([]string, 0, 1+len(suites)*len(renderConfigs())+len(renderConfigs()))
	lines = append(lines, seedSetLine(seeds))
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

// seedSetLine names the seeds the aggregates are taken over, independently of the order they are generated in.
func seedSetLine(seeds []string) string {
	sorted := slices.Clone(seeds)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\x00")))

	return fmt.Sprintf("seeds\t%d\t%s", len(seeds), hex.EncodeToString(sum[:]))
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
