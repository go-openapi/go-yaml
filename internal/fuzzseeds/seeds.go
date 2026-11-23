// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package fuzzseeds supplies the shared seed corpus for the fuzz targets.
//
// It sits under internal/ so that nothing outside this module can import it: it
// is test support, and it reaches into the vendored YAML Test Suite and into
// the generated corpus the conformance module holds.
//
// Seeds matter more than they look. A fuzzer starting from an empty corpus
// spends its budget rediscovering that YAML has anchors; starting from the test
// suite and the corpus, it spends it on the combinations nobody thought to
// write down.
package fuzzseeds

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// All returns every seed document, deduplicated and in a fixed order: the whole
// YAML Test Suite, valid and invalid alike, then the inputs reduced from
// reports we track, then the generated YAML corpus where it is present.
func All() ([]string, error) {
	tests, err := yamltestsuite.TestSuites()
	if err != nil {
		return nil, fmt.Errorf("loading the YAML Test Suite: %w", err)
	}
	if len(tests) == 0 {
		return nil, errors.New("the vendored YAML Test Suite is empty")
	}

	corpus, err := corpusSeeds()
	if err != nil {
		return nil, err
	}

	seeds := make([]string, 0, len(tests)+len(reducedReports)+len(corpus))
	for _, test := range tests {
		seeds = append(seeds, string(test.InYAML))
	}
	seeds = append(seeds, reducedReports...)
	seeds = append(seeds, corpus...)

	// The corpus draws its own documents and repeats the shapes the suite
	// already carries, so the three sources overlap. Seeding the same document
	// twice costs a parse on every go test run and buys nothing.
	return unique(seeds), nil
}

// unique keeps the first occurrence of each seed, so the order stays fixed.
func unique(seeds []string) []string {
	seen := make(map[string]struct{}, len(seeds))
	kept := seeds[:0]

	for _, seed := range seeds {
		if _, dup := seen[seed]; dup {
			continue
		}
		seen[seed] = struct{}{}
		kept = append(kept, seed)
	}

	return kept
}

// reducedReports collects inputs reduced from issue reports and from suite cases
// we diverge on, so that a fuzzing run starts from the shapes already known to
// be awkward instead of rediscovering them.
//
// Add to this list whenever a report is reduced to a minimal input, whether or
// not the underlying defect is fixed: a fixed defect is exactly what a seed
// should keep guarding.
var reducedReports = []string{
	"key:\n}",                           // goccy/go-yaml#890, reported as a SIGSEGV
	"foo: {a: 1,\n  # comment\n  b: 2}", // goccy/go-yaml#903, comments in flow maps
	"foo: 1\n\t\nbar: 2",                // yaml-test-suite DK95/4, a tab-only line
	"{foo\n: bar}",                      // yaml-test-suite 4MUZ/2, a flow key on its own line
	"k: {\n k\n :\n v\n }\n",            // yaml-test-suite VJP3/1, the general form of the same
	"---\n[ a, b, c, ]#invalid\n",       // yaml-test-suite 9JBA, a comment with no leading space
	"- !!str, xxx\n",                    // yaml-test-suite U99R, a comma inside a tag
	"[-]\n",                             // yaml-test-suite YJV2, a plain dash in flow context

	// Shapes that reach the error paths of the scanner, which are the least
	// exercised part of it.
	"a: 'b",     // unterminated single quote
	`a: "b`,     // unterminated double quote
	`a: "\q"`,   // unrecognized escape
	`"\uZZZZ"`,  // malformed unicode escape
	"a: |z\n b", // invalid block scalar indicator
	"a: !!<>",   // malformed tag
	"a: @b",     // reserved indicator
	"a: `b",     // reserved indicator

	// Degenerate inputs.
	"\t",
	"\xef\xbb\xbf{}", // a leading UTF-8 BOM, which is not stripped today
	"---\n",
	"...\n",
	"",
}

// corpusPath is a copy of the generated YAML corpus, which
// github.com/go-openapi/conformance-suites/yaml/corpus embeds.
//
// Read by path rather than imported: importing that module would add a
// requirement for every user of this one. internal/testintegration requires it,
// and its TestTheCorpusCopyIsTheEmbeddedOne keeps this copy equal to it.
// A nested go.mod also keeps that directory out of a published copy of this
// module, so the file is often absent -- All then returns the suite seeds
// alone rather than failing.
const corpusPath = "../testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz"

// corpusCase is the one field of a suite.Case a seed needs.
//
// Declared here rather than imported for the same reason as corpusPath. The
// artifact's first line is a header object with no "src", which decodes to the
// zero value and is skipped.
type corpusCase struct {
	Src []byte `json:"src"`
}

// corpusSeeds returns the documents of the stored YAML corpus, or nothing when
// the artifact is not there.
//
// 10,413 documents over 605 of the grammar's 605 reachable buckets, where the
// YAML Test Suite reaches 561. Two thirds are well-formed and the rest were
// broken on purpose, which is the mix a fuzzer wants: it starts from the shapes
// a parser has to get right and the shapes it has to refuse, instead of
// rediscovering both.
func corpusSeeds() ([]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("cannot locate the fuzzseeds package")
	}

	f, err := os.Open(filepath.Join(filepath.Dir(file), corpusPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening the YAML corpus: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("reading the YAML corpus: %w", err)
	}
	defer func() { _ = gz.Close() }()

	var seeds []string
	lines := bufio.NewScanner(gz)
	const initialLineBuffer = 64 * 1024
	lines.Buffer(make([]byte, 0, initialLineBuffer), maxCorpusLine)

	for lines.Scan() {
		var c corpusCase
		if err := json.Unmarshal(lines.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("decoding a case of the YAML corpus: %w", err)
		}
		if len(c.Src) > 0 {
			seeds = append(seeds, string(c.Src))
		}
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("reading the YAML corpus: %w", err)
	}

	return seeds, nil
}

// maxCorpusLine bounds one line of the artifact. A case is a small document in
// base64, and the longest in the stored corpus is well under this.
const maxCorpusLine = 1 << 20
