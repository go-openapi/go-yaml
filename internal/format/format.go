// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package format

import (
	"github.com/go-openapi/go-yaml/ast"
)

// NodeWithResolvedAlias writes n as YAML, with every alias replaced by
// the node its anchor names. Pass the anchors collected so far as anchorNodeMap.
//
// The decoder calls it to build the bytes a custom UnmarshalYAML or
// UnmarshalText receives. Those bytes are parsed again by the unmarshaler, so
// what matters is that they read back as the node they were written from -- not
// that they match the source. They already could not: an alias is written out
// as the value it stands for, because an unmarshaler handed "*a1" has no anchor
// map to look it up in.
//
// So the layout is the renderer's rather than the document's. A block sequence
// under a key sits at the key's own indentation whatever the source did, and
// the text is indented from the node rather than from wherever the node stood.
// Reproducing the source byte for byte is a separate feature and would need the
// source kept alongside the tree.
func NodeWithResolvedAlias(n ast.Node, anchorNodeMap map[string]ast.Node) string {
	text := ast.NewRenderer(ast.WithAliasTargets(anchorNodeMap)).String(n)

	// The renderer leaves the break that ends a node to whatever follows it,
	// there being none to write while the node is nested in a document; a
	// caller standing the text on its own writes it, as ast.File does between
	// documents. Here nothing follows, so a block scalar loses its last line:
	// "|" then "  a" reads back as "a" where the node holds "a\n".
	//
	// Written whatever the text already ends on, because the count is what "|+"
	// keeps: "a\n\n\n" renders as "|+", "  a" and one blank line, and reads back
	// a break short unless this one is added too.
	//
	// Only a block scalar. Adding it to every node hands "9\n" to an unmarshaler
	// that calls strconv.Atoi, and "\"bar\"\n" to one comparing against the text
	// it expects.
	if _, block := n.(*ast.LiteralNode); block && text != "" {
		text += "\n"
	}

	return text
}
