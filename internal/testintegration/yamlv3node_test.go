// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"fmt"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/codec"
)

// V3Version reads "1.2.3" through go.yaml.in/yaml/v3's UnmarshalYAML(*yaml.Node), the shape v3 code writes.
type V3Version struct {
	Major, Minor, Patch int
}

func (v *V3Version) UnmarshalYAML(node *v3.Node) error {
	if node.Kind != v3.ScalarNode {
		return fmt.Errorf("a version is a scalar, not kind %d", node.Kind)
	}
	_, err := fmt.Sscanf(node.Value, "%d.%d.%d", &v.Major, &v.Minor, &v.Patch)

	return err
}

// V3Upper decodes its node with yaml.Node.Decode, which runs v3's own decoder, and upper-cases every string.
type V3Upper map[string]string

func (u *V3Upper) UnmarshalYAML(node *v3.Node) error {
	var raw map[string]string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*u = V3Upper{}
	for k, v := range raw {
		(*u)[k] = strings.ToUpper(v)
	}

	return nil
}

type V3Release struct {
	Name    string     `yaml:"name"`
	Version V3Version  `yaml:"version"`
	Prior   *V3Version `yaml:"prior"`
	Labels  V3Upper    `yaml:"labels"`
	History []V3Version
}

// TestAV3NodeHookDecodesAsInV3 decodes documents into types whose UnmarshalYAML takes a *yaml.Node, and checks the
// value against the one go.yaml.in/yaml/v3 decodes from the same document.
func TestAV3NodeHookDecodesAsInV3(t *testing.T) {
	for tc := range v3NodeHookCases() {
		t.Run(tc.name, func(t *testing.T) {
			var ours, theirs V3Release
			require.NoError(t, v3.Unmarshal([]byte(tc.doc), &theirs))
			require.NoError(t, codec.Unmarshal([]byte(tc.doc), &ours))
			assert.Equal(t, theirs, ours)
		})
	}
}

// TestAV3NodeHookAtTheRoot checks a hook type passed to Unmarshal directly.
func TestAV3NodeHookAtTheRoot(t *testing.T) {
	var ours, theirs V3Version
	require.NoError(t, v3.Unmarshal([]byte("4.5.6\n"), &theirs))
	require.NoError(t, codec.Unmarshal([]byte("4.5.6\n"), &ours))
	assert.Equal(t, theirs, ours)
}

// TestAV3NodeHookErrorIsReturned checks the decoder returns the error the hook returns.
func TestAV3NodeHookErrorIsReturned(t *testing.T) {
	var ours V3Release
	err := codec.Unmarshal([]byte("version: [1, 2]\n"), &ours)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a version is a scalar")
}

// TestAYAMLNodeIsBuiltAsInV3 decodes documents into a yaml.Node, at the root and as a field, and compares the tree
// with the one v3 builds: kind, style, tag, value, anchor, alias, line and column, node by node.
func TestAYAMLNodeIsBuiltAsInV3(t *testing.T) {
	for tc := range v3NodeTreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var ours, theirs v3.Node
			require.NoError(t, v3.Unmarshal([]byte(tc.doc), &theirs))
			require.NoError(t, codec.Unmarshal([]byte(tc.doc), &ours))
			assert.Equal(t, describeV3Node(&theirs), describeV3Node(&ours))
		})
		if strings.HasPrefix(tc.doc, "---") {
			// A document marker cannot stand under a key.
			continue
		}
		t.Run(tc.name+", as a field", func(t *testing.T) {
			type holder struct {
				N v3.Node `yaml:"n"`
			}
			doc := "n: " + indentUnder(tc.doc)
			var ours, theirs holder
			require.NoError(t, v3.Unmarshal([]byte(doc), &theirs))
			require.NoError(t, codec.Unmarshal([]byte(doc), &ours))
			assert.Equal(t, describeV3Node(&theirs.N), describeV3Node(&ours.N))
		})
		t.Run(tc.name+", behind a pointer", func(t *testing.T) {
			// v3 compares a destination with its Node type before it dereferences a pointer, so a *yaml.Node field
			// is decoded as a struct: it stays empty for a mapping, and a scalar is an error. This library gives
			// the pointer the node a yaml.Node field gets.
			type direct struct {
				N v3.Node `yaml:"n"`
			}
			type holder struct {
				N *v3.Node `yaml:"n"`
			}
			doc := "n: " + indentUnder(tc.doc)
			var theirs direct
			var ours holder
			require.NoError(t, v3.Unmarshal([]byte(doc), &theirs))
			require.NoError(t, codec.Unmarshal([]byte(doc), &ours))
			require.NotNil(t, ours.N)
			assert.Equal(t, describeV3Node(&theirs.N), describeV3Node(ours.N))
		})
	}
}

// TestAYAMLNodeTakesThisLibrarysResolution records the tags where this library and v3 read a plain scalar
// differently. The yaml.Node carries this library's reading: YAML 1.2, unless the document declares 1.1.
func TestAYAMLNodeTakesThisLibrarysResolution(t *testing.T) {
	for _, tc := range []struct{ doc, ours, theirs string }{
		{"1_000", "!!str", "!!int"},
		{"2001-12-14", "!!str", "!!timestamp"},
		{"<<", "!!str", "!!merge"},
	} {
		var ours, theirs v3.Node
		require.NoError(t, v3.Unmarshal([]byte(tc.doc), &theirs))
		require.NoError(t, codec.Unmarshal([]byte(tc.doc), &ours))
		require.Len(t, ours.Content, 1)
		assert.Equalf(t, tc.theirs, theirs.Content[0].Tag, "%q under v3", tc.doc)
		assert.Equalf(t, tc.ours, ours.Content[0].Tag, "%q here", tc.doc)
	}
}

// describeV3Node returns one line per node of the tree under n, so a comparison names the node that differs.
// An alias is described by its target's anchor and position, since the target is a node of the same tree.
func describeV3Node(n *v3.Node) []string {
	var lines []string
	var walk func(n *v3.Node, depth int)
	walk = func(n *v3.Node, depth int) {
		line := fmt.Sprintf("%s kind=%d style=%d tag=%q value=%q anchor=%q at %d:%d",
			strings.Repeat("  ", depth), n.Kind, n.Style, n.Tag, n.Value, n.Anchor, n.Line, n.Column)
		if n.Alias != nil {
			line += fmt.Sprintf(" alias of &%s at %d:%d", n.Alias.Anchor, n.Alias.Line, n.Alias.Column)
		}
		lines = append(lines, line)
		for _, child := range n.Content {
			walk(child, depth+1)
		}
	}
	walk(n, 0)

	return lines
}

// indentUnder returns doc indented to stand as the value of a key, on the lines below it.
func indentUnder(doc string) string {
	var b strings.Builder
	b.WriteString("\n")
	for line := range strings.Lines(doc) {
		b.WriteString("  ")
		b.WriteString(line)
	}
	if !strings.HasSuffix(doc, "\n") {
		b.WriteString("\n")
	}

	return b.String()
}

// ***************** v3 node test cases **************************

// v3NodeCase holds a case name and a document.
type v3NodeCase struct {
	name string
	doc  string
}

func v3NodeHookCases() iter.Seq[v3NodeCase] {
	return slices.Values([]v3NodeCase{
		{"a scalar hook in a field", "name: x\nversion: 1.2.3\n"},
		{"a hook behind a pointer", "prior: 0.9.1\n"},
		{"a null leaves a pointer nil", "prior: ~\n"},
		{"a null leaves a value as it was", "version: ~\n"},
		{"a hook that calls Node.Decode", "labels: {a: x, b: y}\n"},
		{"hooks in a sequence", "History: [1.0.0, 1.1.0]\n"},
		{"a hook reading an alias", "name: &v 2.0.0\nversion: *v\n"},
	})
}

func v3NodeTreeCases() iter.Seq[v3NodeCase] {
	return slices.Values([]v3NodeCase{
		{"a plain string", "hello\n"},
		{"scalars of each type", "s: x\ni: 12\nf: 1.5\nb: true\nn: null\ne:\n"},
		{"quoted strings", "d: \"x\"\ns: 'y'\n"},
		{"a literal and a folded block", "l: |\n  a\n  b\nf: >\n  c\n  d\n"},
		{"a block sequence", "- a\n- b\n"},
		{"a flow mapping and sequence", "{a: [1, 2], b: {c: d}}\n"},
		{"nested block collections", "a:\n  b:\n    - c\n    - d: e\n"},
		{"an anchor and its alias", "a: &x 1\nb: *x\n"},
		{"an anchored mapping", "a: &m {k: v}\nb: *m\n"},
		{"explicit tags", "a: !!str 12\nb: !local x\nc: !!int \"7\"\n"},
		{"a document start", "---\na: 1\n"},
		{"a bare dash", "- \n- a\n"},
		{"a tag and an anchor on one node", "a: !!str &x 1\nb: &y !!int 2\nc: [*x, *y]\n"},
		{"a plain scalar over two lines", "a: one\n  two\n"},
		{"a mapping holding an alias to itself", "&m {a: *m}\n"},
		{"a document with comments", "# head\na: 1 # line\n# foot\n"},
		{"a key with properties", "&k key: v\n!!str other: w\n"},
	})
}
