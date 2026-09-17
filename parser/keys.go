// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// recordKeyOnce records tk among the keys of the mapping being parsed, and notes a repeat on the mapping.
//
// A repeated key does not stop the parse. Section 3.2.1.1 makes the repeat an error,
// but the caller chooses which error to return and whether to stop,
// and a document the parser rejects cannot be linted, rendered or colorized either.
// codec rejects the repeat when it loads the tree.
//
// Every entry's key passes through here, including a flow entry written as a key with no value:
// "{a, a: 1}" repeats a key as much as "{a: 1, a: 2}" does.
//
// Under [WithAllowDuplicateMapKey] the repeat is recorded all the same, marked [ast.DuplicateKey.Allowed],
// so a load can tell which entry repeats and keep one of them.
func (p *Parser) recordKeyOnce(ctx context, tk *token.Token, name string, kind token.KeyKind) {
	if unnamedKey(name, kind) {
		// mapKeyIdentity returned no name: the key is an alias whose target the load resolves,
		// or a collection, whose identity is a comparison of trees.
		// Recording such keys would make every one of them the same key.
		// A quoted "" comes back as token.KeyString and the empty node as "null", so both are still recorded.
		return
	}

	p.keys.RecordOnce(ctx.keyBase, name, kind, tk.Position)
}

// unnamedKey reports whether mapKeyIdentity returned no name for a key: "" under [token.KeyOther].
func unnamedKey(name string, kind token.KeyKind) bool {
	return name == "" && kind == token.KeyOther
}

// recordBuiltKeyOnce records an entry whose key no single token names, and notes a repeat on the mapping.
//
// Section 3.2.1.1 makes two keys equal when they resolve to the same node, so "[a]" and "[ a ]" are one key.
// recordKeyOnce records a scalar key as it is read, from its one token.
// The keys left over reach here, and [ast.KeyIdentity] names them from the built node.
//
// It runs from [Parser.mappingValue], once the entry and its key are complete.
// Before that a collection key has no children to be named by, and a block scalar key has only its header.
//
// The repeat is recorded with its own position and the position of the entry that first wrote the key,
// as for a scalar key.
func (p *Parser) recordBuiltKeyOnce(key ast.MapKeyNode) {
	if !p.keys.InMapping() || key == nil {
		return
	}
	if name, kind := p.mapKeyIdentity(key); !unnamedKey(name, kind) {
		// recordKeyOnce recorded this key as it was read.
		return
	}

	identity := p.builtKeyIdentity(key)
	if ast.Unnamed(identity) {
		// Nothing to compare.
		return
	}
	tk := key.GetToken()
	if tk == nil {
		return
	}

	p.keys.RecordBuilt(identity, keyDisplayName(key), tk.Position)
}

// keepsNothing reports whether a walk may reuse the cells of the nodes it has just read.
//
// It returns false inside a key and inside an anchor, because there the node is read a second time.
// A key is named by its content, and an alias names the anchored node,
// which [ast.KeyIdentity] reads through [ast.AliasNode.Target].
// It also returns false inside a node a visitor answered with [KeepNode], which it receives whole.
//
// Keeping an anchor's nodes holds no extra tape: closeAnchor already saves the tokens an anchor covers
// until the document ends.
func (p *Parser) keepsNothing() bool {
	return !p.descent.readingAKey() && !p.anchors.reading() && (p.walk == nil || !p.walk.keeping)
}

// builtKeyIdentity names a key that no single token names.
//
// An alias is named from the identities the anchors recorded, not through [ast.AliasNode.Target].
// Section 3.2.1.1 makes "&a [1]" and a later "*a" one key, because an alias node is the anchored node and not a copy,
// and on a walk the anchored node is scrubbed by the time the alias is read.
// Every other key is named from the node in hand.
func (p *Parser) builtKeyIdentity(key ast.MapKeyNode) string {
	n := ast.Node(key)
	if explicit, isExplicit := n.(*ast.MappingKeyNode); isExplicit {
		n = explicit.Value
	}
	if alias, isAlias := n.(*ast.AliasNode); isAlias {
		return p.anchors.identity(anchorNameOf(alias.Value)).identity
	}

	return ast.KeyIdentityWithAnchors(key, p.anchors.identityOf)
}

// keyDisplayName returns the text an error shows for a key that no single token names.
//
// It renders the node, because "[a]" reads well in an error where the identity "seq(string/a)" does not.
// It drops an explicit "?" and joins the lines with a space, so a key written over three lines fits on one line.
func keyDisplayName(n ast.Node) string {
	if key, explicit := n.(*ast.MappingKeyNode); explicit {
		n = key.Value
	}
	if n == nil {
		return ""
	}

	return strings.Join(strings.Fields(n.String()), " ")
}

// isScalarKeyToken reports whether tk is a key token that stands at the start of its entry,
// so the column of what follows can be measured against it.
//
// A plain or quoted key does. So does the implicit null standing for a key that was never written:
// implicitNullKeyToken copies the position of the ':', and the entry begins at the ':'.
// Without it ":\n1\n" would read as {null: 1}, although "k:\n1\n", the same document with the key written out,
// is rejected.
func isScalarKeyToken(tk *token.Token) bool {
	switch tk.Type {
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType, token.ImplicitNullType:
		return true
	default:
		return false
	}
}

// mapKeyText returns the key as the document wrote it, and a path addresses the entry by that text:
// "$.0x10" reaches the entry written "0x10:" whatever the number resolves to.
// mapKeyIdentity tells one key from another.
func (p *Parser) mapKeyText(n ast.Node) string {
	if n == nil {
		return ""
	}

	switch nn := n.(type) {
	case *ast.MappingKeyNode:
		return p.mapKeyText(nn.Value)
	case *ast.TagNode:
		return p.mapKeyText(nn.Value)
	case *ast.AnchorNode:
		return p.mapKeyText(nn.Value)
	case *ast.AliasNode:
		return ""
	}

	return n.GetToken().Value
}

// mapKeyIdentity returns the name and the kind the duplicate check compares a key by.
//
// The name comes from [ast.ComparedKeyName], the walk every reader of a key shares,
// so the duplicate check compares a key by the rule [ast.KeyName] names it by.
// aliasKeyIdentity answers an alias.
//
// A collection key gets no name here: its first token would name every sequence key "[" and every block mapping key ":".
// recordBuiltKeyOnce checks it with ast.KeyIdentity once its entry is complete,
// since a collection holds nothing while its entry is being read.
func (p *Parser) mapKeyIdentity(n ast.Node) (string, token.KeyKind) {
	name, kind, named := ast.ComparedKeyName(n, p.aliasKeyIdentity)
	if !named {
		return "", token.KeyOther
	}

	return name, kind
}

// aliasKeyIdentity names an alias key as the node its anchor named (section 3.2.2.2).
//
// keepAnchorIdentity recorded that node while it was whole; on a walk AliasNode.Target now points at a reused cell.
// A scalar anchor is named here, so "{&a x: 1, *a : 2}" is one key written twice.
// A collection anchor returns no name, and builtKeyIdentity checks it once its entry is built.
func (p *Parser) aliasKeyIdentity(a *ast.AliasNode) (string, token.KeyKind, bool) {
	at := p.anchors.identity(anchorNameOf(a.Value))

	return at.text, at.kind, true
}

// oneEntryKeyIdentity names the key of a mapping holding exactly one entry, as mapKeyIdentity names a scalar key,
// looking through the tags and anchors on the mapping. It returns no name for any other node.
func (p *Parser) oneEntryKeyIdentity(n ast.Node) (string, token.KeyKind) {
	for {
		switch nn := n.(type) {
		case *ast.TagNode:
			n = nn.Value
		case *ast.AnchorNode:
			n = nn.Value
		case *ast.MappingNode:
			if len(nn.Values) != 1 {
				return "", token.KeyOther
			}

			return p.mapKeyIdentity(nn.Values[0].Key)
		case *ast.MappingValueNode:
			return p.mapKeyIdentity(nn.Key)
		default:
			return "", token.KeyOther
		}
	}
}

// recordAliasEntry records the key of an "!!omap" entry written as an alias, from the one-entry mapping its anchor named.
//
// The entry opens no mapping, so no key is recorded for it as it is read:
// under "m: &m {x: 2}", the entries of "!!omap [{x: 1}, *m]" write x twice.
// keepAnchorIdentity named the key while the anchored mapping was whole; on a walk AliasNode.Target is a reused cell.
func (p *Parser) recordAliasEntry(value ast.Node) {
	alias, isAlias := value.(*ast.AliasNode)
	if !isAlias {
		return
	}
	at := p.anchors.identity(anchorNameOf(alias.Value))
	if unnamedKey(at.entryText, at.entryKind) {
		return
	}
	p.keys.RecordEntry(at.entryText, at.entryKind, alias.GetToken().Position)
}
