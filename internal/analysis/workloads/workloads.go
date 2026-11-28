// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package workloads holds the YAML documents the parser is measured on.
//
// They are the JSON benchmark corpus every JSON parser is compared on --
// canada_geometry, citm_catalog, golang_source, twitter_status -- plus one
// Azure OpenAPI specification, rewritten once as block-style YAML. See
// SOURCE.md for where each came from and how to rewrite them again.
//
// Rewritten rather than fed in as JSON on purpose. JSON is YAML, so the files
// would parse as they stand, and they would exercise flow collections and
// double-quoted scalars and nothing else. The documents here are laid out the
// way a person writes YAML: block mappings and sequences, plain scalars, and
// quotes only where the scalar needs them.
//
// None of those five holds a comment, because JSON has none to carry over.
// commented_swagger is azure_swagger with comments written over it, and it is
// the only workload that reaches the comment code at all: the scanner drops a
// comment where the mode does not ask for one, the grouping lifts a line
// comment out of the token stream, and the parse attaches it to a node
// afterwards. Measure ParseComments on that one; the others report what
// the parser does when there is nothing to attach.
//
// citm_catalog_flow is citm_catalog written in flow style, from its top-level
// mapping down: 10,937 flow mappings and 10,451 flow sequences, one entry a
// line where a collection does not fit in 80 columns. It is the only workload
// that reaches the flow collection code with entries in it. The others hold no
// flow collection but "[]" and "{}", or one of a single element.
package workloads

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

//go:embed testdata/*.yaml.gz
var corpus embed.FS

// stressCorpus holds the documents built to strain a parser rather than to
// resemble one anybody wrote. They are kept out of [All] on purpose: a geomean
// over the corpus is meant to say what an ordinary document costs, and a
// fifty-thousand-key mapping in that average would stop it saying so.
//
//go:embed testdata/stress/*.yaml.gz
var stressCorpus embed.FS

// Workload is one document, named by the file it came from.
type Workload struct {
	Name string
	Data []byte
}

// All returns every workload, decompressed, ordered by name.
func All() ([]Workload, error) {
	return readDir(corpus, "testdata")
}

// readDir reads every gzipped document standing directly under dir.
func readDir(fsys embed.FS, dir string) ([]Workload, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	out := make([]Workload, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml.gz") {
			continue
		}

		data, err := readGzipped(fsys, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("loading workload %s: %w", name, err)
		}

		out = append(out, Workload{
			Name: strings.TrimSuffix(name, ".yaml.gz"),
			Data: data,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// Stress returns every stress document, decompressed, ordered by name.
//
// Each strains one thing a token store that reclaims behind the parse finds
// hard: a flow collection spanning many blocks, an anchor referred to a
// document away, anchors nested inside anchors, a level wider than any a person
// writes, and runs of comments long enough that reading past them is itself the
// cost. See SOURCE.md for what each one holds.
func Stress() ([]Workload, error) {
	return readDir(stressCorpus, "testdata/stress")
}

// ByName returns one workload.
func ByName(name string) (Workload, error) {
	data, err := read(path.Join("testdata", name+".yaml.gz"))
	if err != nil {
		return Workload{}, fmt.Errorf("loading workload %s: %w", name, err)
	}

	return Workload{Name: name, Data: data}, nil
}

func read(name string) ([]byte, error) {
	return readGzipped(corpus, name)
}

func readGzipped(fsys embed.FS, name string) ([]byte, error) {
	raw, err := fsys.ReadFile(name)
	if err != nil {
		return nil, err
	}

	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()

	return io.ReadAll(gz)
}
