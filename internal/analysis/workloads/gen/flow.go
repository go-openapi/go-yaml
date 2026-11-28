// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v3 "go.yaml.in/yaml/v3"
)

// flowFrom is the workload the flow-style one is written over, and flowName is what the result is stored as.
const (
	flowFrom = "citm_catalog"
	flowName = "citm_catalog_flow"
)

// flowWidth is the column a flow collection may reach on one line. A wider collection writes one entry a line.
const flowWidth = 80

// flowIndent is the number of spaces each open collection adds to the lines of its entries.
const flowIndent = 2

// rewriteFlow writes one stored workload in flow style and saves the result beside it.
func rewriteFlow(dir string) error {
	src, err := readGzip(filepath.Join(dir, flowFrom+".yaml.gz"))
	if err != nil {
		return err
	}

	out, err := toFlow(src)
	if err != nil {
		return err
	}
	if err := sameValue(src, out, "the flow layout"); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%-18s %8d bytes -> %8d in flow style (%d lines)\n",
		flowFrom, len(src), len(out), lineCount(out))

	return writeGzip(filepath.Join(dir, flowName+".yaml.gz"), out)
}

// toFlow returns src with every collection written in flow style, the top-level mapping included.
//
// A collection that fits on its line is written there whole, "{id: 1, name: x}", and a longer one opens a line for
// each entry, with a ',' after every entry but the last.
//
// The top-level mapping is a flow mapping too. Under a block mapping, a flow value's closing bracket at column 0 is
// not valid YAML: every line of a flow collection in a block value must be indented past the block. yaml.v3 and
// goccy/go-yaml accept it anyway, and this library and the YAML reference parser reject it.
func toFlow(src []byte) ([]byte, error) {
	var doc v3.Node
	if err := v3.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("reading the source: %w", err)
	}
	if doc.Kind != v3.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != v3.MappingNode {
		return nil, errors.New("the source is not one document holding a mapping")
	}

	var b strings.Builder
	if err := writeFlow(&b, doc.Content[0], 0, 0); err != nil {
		return nil, err
	}
	b.WriteByte('\n')

	return []byte(b.String()), nil
}

// writeFlow writes n at the column after a prefix of width used, on a line indented by indent.
func writeFlow(b *strings.Builder, n *v3.Node, indent, used int) error {
	if n.Kind == v3.ScalarNode {
		text, err := scalarText(n)
		if err != nil {
			return err
		}
		b.WriteString(text)

		return nil
	}

	inline, err := inlineFlow(n)
	if err != nil {
		return err
	}
	if indent+used+len(inline) <= flowWidth {
		b.WriteString(inline)

		return nil
	}

	open, closing, step := "[", "]", 1
	if n.Kind == v3.MappingNode {
		open, closing, step = "{", "}", 2
	}
	inner := indent + flowIndent
	pad := strings.Repeat(" ", inner)

	b.WriteString(open)
	for i := 0; i < len(n.Content); i += step {
		b.WriteByte('\n')
		b.WriteString(pad)
		used := 0
		if n.Kind == v3.MappingNode {
			key, err := scalarText(n.Content[i])
			if err != nil {
				return err
			}
			b.WriteString(key)
			b.WriteString(": ")
			used = len(key) + len(": ")
		}
		if err := writeFlow(b, n.Content[i+step-1], inner, used); err != nil {
			return err
		}
		if i+step < len(n.Content) {
			b.WriteByte(',')
		}
	}
	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", indent))
	b.WriteString(closing)

	return nil
}

// inlineFlow returns n written whole on one line.
func inlineFlow(n *v3.Node) (string, error) {
	if n.Kind == v3.ScalarNode {
		return scalarText(n)
	}

	open, closing, step := "[", "]", 1
	if n.Kind == v3.MappingNode {
		open, closing, step = "{", "}", 2
	}

	parts := make([]string, 0, len(n.Content)/step)
	for i := 0; i < len(n.Content); i += step {
		value, err := inlineFlow(n.Content[i+step-1])
		if err != nil {
			return "", err
		}
		if n.Kind == v3.MappingNode {
			key, err := scalarText(n.Content[i])
			if err != nil {
				return "", err
			}
			value = key + ": " + value
		}
		parts = append(parts, value)
	}

	return open + strings.Join(parts, ", ") + closing, nil
}

// scalarText returns a scalar as go.yaml.in/yaml/v3 writes it inside a flow sequence, so it is quoted wherever a
// plain scalar would end at a flow indicator or read as another type.
func scalarText(n *v3.Node) (string, error) {
	if n.Kind != v3.ScalarNode {
		return "", fmt.Errorf("a key of kind %v, not a scalar", n.Kind)
	}

	holder := &v3.Node{Kind: v3.SequenceNode, Style: v3.FlowStyle, Content: []*v3.Node{n}}
	out, err := v3.Marshal(holder)
	if err != nil {
		return "", err
	}

	text := strings.TrimSuffix(string(out), "\n")
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") || strings.Contains(text, "\n") {
		return "", fmt.Errorf("scalar %q does not fit on one flow line: %q", n.Value, text)
	}

	return text[1 : len(text)-1], nil
}
