// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Command gen rewrites a JSON benchmark document as block-style YAML and
// stores it beside the others.
//
// It is how ../testdata was produced. See ../SOURCE.md for where the JSON came
// from and what the two adjustments below change:
//
//	go run ./gen -json ../../../../../core/json/testdata/workloads -out ../testdata
//
// Leave -json out to rebuild only commented_swagger, which is written over the
// stored azure_swagger and needs no checkout of go-openapi/core:
//
//	go run ./gen -out testdata
//
// The output is byte-deterministic: gzip level 9, no modification time, and the
// operating system byte set to 255, so rerunning this on another machine
// produces the same file.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

// sources are the JSON documents to rewrite, by the subdirectory each sits in.
var sources = map[string]string{
	"canada_geometry": "standard",
	"citm_catalog":    "standard",
	"golang_source":   "standard",
	"twitter_status":  "standard",
	"azure_swagger":   "additional",
}

func main() {
	jsonDir := flag.String("json", "", "the workloads directory of a go-openapi/core checkout; leave it out to rebuild only "+commentedName)
	outDir := flag.String("out", "testdata", "where to write the .yaml.gz files")
	flag.Parse()

	if *jsonDir != "" {
		for name, sub := range sources {
			if err := rewrite(filepath.Join(*jsonDir, sub, name+".json.gz"), filepath.Join(*outDir, name+".yaml.gz")); err != nil {
				fmt.Fprintf(os.Stderr, "gen: %s: %v\n", name, err)
				os.Exit(1)
			}
		}
	}

	// The commented workload is written over a stored one, so it is rebuilt
	// whether or not the JSON corpus was rewritten just now.
	if err := annotate(*outDir); err != nil {
		fmt.Fprintf(os.Stderr, "gen: %s: %v\n", commentedName, err)
		os.Exit(1)
	}

	if err := stress(filepath.Join(*outDir, "stress")); err != nil {
		fmt.Fprintf(os.Stderr, "gen: %v\n", err)
		os.Exit(1)
	}
}

// rewrite reads one gzipped JSON document and writes the YAML beside it.
func rewrite(from, to string) error {
	raw, err := readGzip(from)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	value, err := ordered(dec)
	if err != nil {
		return fmt.Errorf("decoding: %w", err)
	}

	out, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}

	fmt.Printf("%-18s %8d bytes of JSON -> %8d of YAML\n", filepath.Base(from), len(raw), len(out))

	return writeGzip(to, out)
}

// ordered decodes a JSON document into codec.MapSlice, so a mapping keeps the
// order its object had. yaml.Marshal of a map would sort the keys and the
// documents would stop resembling what they were.
func ordered(dec *json.Decoder) (any, error) {
	tk, err := dec.Token()
	if err != nil {
		return nil, err
	}

	switch t := tk.(type) {
	case json.Delim:
		switch t {
		case '{':
			var items codec.MapSlice
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				value, err := ordered(dec)
				if err != nil {
					return nil, err
				}
				if err := items.Set(key.(string), value); err != nil {
					return nil, err
				}
			}
			_, err := dec.Token() // the closing brace

			return items, err
		case '[':
			list := []any{}
			for dec.More() {
				value, err := ordered(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, value)
			}
			_, err := dec.Token() // the closing bracket

			return list, err
		}

		return nil, fmt.Errorf("unexpected %v", t)
	case json.Number:
		return rawNumber(t), nil
	case string:
		// YAML normalizes a line break inside a scalar, so a string holding
		// CRLF cannot survive being written and read back. Normalize here and
		// keep the corpus a document that reads back as itself.
		return yamlString(strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\r", "\n")), nil
	default:
		return t, nil
	}
}

// rawNumber writes a JSON number back as the plain scalar it was written as, so
// the corpus keeps the digits the source had rather than a float64's idea of
// them.
type rawNumber string

func (n rawNumber) MarshalYAML() ([]byte, error) { return []byte(n), nil }

// yamlString returns s, or a forced double-quoted spelling of it when the
// emitter would write a plain scalar that reads back as something else.
//
// "088253" is the case here: written plain it resolves to the integer 88253.
func yamlString(s string) any {
	out, err := yaml.Marshal(s)
	if err != nil {
		return quoted(s)
	}

	// Read back with go.yaml.in/yaml/v3 rather than with this library: the
	// question is whether the scalar is a string to any reader, and our own
	// resolver is one of the two that disagree about "088253".
	var back any
	if err := v3.Unmarshal(out, &back); err != nil {
		return quoted(s)
	}
	if got, ok := back.(string); ok && got == s {
		return s
	}

	return quoted(s)
}

// quoted is a YAML double-quoted scalar holding s.
type quoted string

func (q quoted) MarshalYAML() ([]byte, error) {
	var b strings.Builder

	b.WriteByte('"')
	for _, r := range string(q) {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')

	return []byte(b.String()), nil
}

func readGzip(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()

	return io.ReadAll(gz)
}

// writeGzip stores data with no modification time and no operating system
// byte, so the file does not record the machine that produced it.
func writeGzip(name string, data []byte) error {
	var buf bytes.Buffer

	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.ModTime = time.Time{}
	gz.OS = 255

	if _, err := gz.Write(data); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	return os.WriteFile(name, buf.Bytes(), 0o600)
}
