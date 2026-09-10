// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/token"
)

// recordMapKey records that the mapping starting at base uses text as a key,
// written at pos.
//
// It returns where text was first written, and whether the mapping had already
// used it.
func (p *Parser) recordMapKey(base int, text string, kind token.KeyKind, pos token.Position) (token.Position, bool, bool) {
	if probe.Enabled {
		p.checkKeyStackTail(base)
	}

	return p.keys.record(base, text, kind, pos)
}

// checkKeyStackTail records whether the keys above base all belong to the
// mapping recording now.
//
// Mappings nest, so a mapping records keys only while it is the innermost one
// open: an outer mapping's next key waits for the inner one to close. If that
// holds, the keys above base are exactly one mapping's, and a duplicate can be
// found by scanning that tail instead of hashing base into an index shared by
// every open mapping.
//
// probeBases shadows the stack with the base each key was recorded under, which
// the entries themselves stopped carrying once the scan made it redundant --
// which is the very thing under test, so the probe keeps its own copy rather
// than reading the answer off the state it is checking.
//
// Nothing may raise this: a disagreement means an outer mapping recorded a key
// over an inner one's, and closeMapping would then drop a key the outer still
// owns. keySet.record reads the tail on that promise.
func (p *Parser) checkKeyStackTail(base int) {
	// A repeated key records no entry, so the shadow can stand one ahead of the
	// stack it shadows. Trim it back before reading either.
	p.probeBases = p.probeBases[:min(len(p.probeBases), len(p.keys.entries))]

	held := true
	for i := base; i < len(p.probeBases); i++ {
		if int(p.probeBases[i]) == base {
			continue
		}
		at, was, text := i, p.probeBases[i], p.keys.entries[i].text
		probe.Check("mapkey.stackTailIsOneMapping", false, func() string {
			return fmt.Sprintf("key %q at %d was recorded under mapping %d, recording now for %d",
				text, at, was, base)
		})
		held = false

		break
	}
	if held {
		probe.Check("mapkey.stackTailIsOneMapping", true, nil)
	}

	p.probeBases = append(p.probeBases, int32(base))
}

// closeMapping drops the keys of the mapping that started at base.
func (p *Parser) closeMapping(base int) {
	if probe.Enabled {
		p.probeBases = p.probeBases[:min(base, len(p.probeBases))]
	}
	p.keys.close(base)
}

// recordKeyOnce records tk among the keys of the mapping being parsed, and
// notes a repeat on the mapping rather than refusing the document.
//
// The parse reads a document that repeats a key and says where: 3.2.1.1 makes
// the repeat an error, but which error and whether to stop is the caller's, and
// a document that cannot be parsed cannot be linted, rendered or colorized
// either. codec refuses it at the load.
//
// Every entry's key passes through here, including a flow entry written as a
// key with no value: "{a, a: 1}" repeats a key as much as "{a: 1, a: 2}" does,
// and was read without complaint while the check only saw keys that came with
// a ':'.
//
// Told to allow duplicates, nothing is recorded at all: the mapping carries no
// Duplicates, the load has nothing to refuse, and the last entry written wins
// because that is what filling a map does.
func (p *Parser) recordKeyOnce(ctx context, tk *token.Token, name string, kind token.KeyKind) {
	if p.allowDuplicateMapKey {
		return
	}

	if unnamedKey(name, kind) {
		// [Parser.mapKeyIdentity] had nothing to say about this key: an alias,
		// whose target the load resolves, or a collection, whose identity is a
		// comparison of trees. Recording it would make every such key the same
		// key, so "{[a]: 1, [b]: 2}" was refused as a repeat of "" -- and,
		// before that, as a repeat of "[".
		//
		// A key written empty is not this: "" from a quoted key comes back as
		// [token.KeyString] and the empty node as "null", so both are recorded
		// and both still catch a genuine repeat.
		return
	}

	pos, jsonOnly, defined := p.recordMapKey(ctx.keyBase, name, kind, tk.Position)
	if !defined {
		return
	}

	if n := len(p.openMaps); n > 0 {
		open := p.openMaps[n-1]
		open.Duplicates = append(open.Duplicates,
			ast.DuplicateKey{Name: name, At: tk.Position, FirstAt: pos, JSONNameOnly: jsonOnly})
	}
}

// unnamedKey reports whether mapKeyIdentity gave up on a key, which it says by
// handing back no name under [token.KeyOther].
func unnamedKey(name string, kind token.KeyKind) bool {
	return name == "" && kind == token.KeyOther
}

// openMapping records the mapping being read, and returns what takes it off.
func (p *Parser) openMapping(node *ast.MappingNode) func() {
	p.openMaps = append(p.openMaps, node)
	p.builtKeys = append(p.builtKeys, nil)

	return func() {
		p.openMaps = p.openMaps[:len(p.openMaps)-1]
		p.builtKeys = p.builtKeys[:len(p.builtKeys)-1]
	}
}

// recordBuiltKeyOnce records an entry whose key a single token cannot name, and
// notes it where the key repeats one an earlier entry of this mapping wrote.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node, so "[a]" and
// "[ a ]" are one key and the second entry repeats the first. A scalar key is
// recorded as it is read, where its one token names it; these are what is left,
// and [ast.KeyIdentity] names them from the built node.
//
// It runs from [Parser.mappingValue], where the entry is complete and its key
// with it. Every earlier attempt named the key while the parser was still
// cutting it, which is why a collection had no children to be named by and a
// block scalar offered its header.
//
// The refusal names the repeat's own token and the token of the entry that
// first wrote the key, which is what a reader gets for a scalar key.
func (p *Parser) recordBuiltKeyOnce(key ast.MapKeyNode) {
	if p.allowDuplicateMapKey || len(p.builtKeys) == 0 {
		return
	}

	if key == nil {
		return
	}
	if name, kind := p.mapKeyIdentity(key); !unnamedKey(name, kind) {
		// Recorded as it was read, by the one token that names it.
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

	top := len(p.builtKeys) - 1
	if first, repeated := p.builtKeys[top][identity]; repeated {
		if n := len(p.openMaps); n > 0 {
			open := p.openMaps[n-1]
			open.Duplicates = append(open.Duplicates,
				ast.DuplicateKey{Name: keyDisplayName(key), At: tk.Position, FirstAt: first})
		}

		return
	}
	if p.builtKeys[top] == nil {
		p.builtKeys[top] = make(map[string]token.Position, 4)
	}
	p.builtKeys[top][identity] = tk.Position
}

// keepsNothing reports whether a walk may hand the cells of what it has just
// read out again.
//
// It holds inside a key and inside an anchor, and for the same reason: the node
// is read a second time, so it has to still be there. A key is named by what it
// holds, and an alias names the anchored node, which [ast.KeyIdentity] reads
// through [ast.AliasNode.Target].
//
// The anchor costs nothing new. closeAnchor already saves the tokens an anchor
// covers until the document ends, so a walk of "a: &x" over ten thousand block
// entries holds 80 tape chunks where the same document without the anchor holds
// 2 -- 1.46 MiB against 55 KiB. The nodes stand on content the tape is keeping
// either way.
func (p *Parser) keepsNothing() bool {
	return p.readingKey == 0 && len(p.openAnchors) == 0
}

// builtKeyIdentity names a key that a single token could not.
//
// An alias is read from the identities the anchors recorded rather than through
// [ast.AliasNode.Target]: 3.2.1.1 makes "&a [1]" and a later "*a" one key,
// because an alias node is the anchored node rather than a copy of it, and on a
// walk the anchored node is scrubbed by the time the alias is read. Everything
// else is named from the node in hand.
func (p *Parser) builtKeyIdentity(key ast.MapKeyNode) string {
	n := ast.Node(key)
	if explicit, isExplicit := n.(*ast.MappingKeyNode); isExplicit {
		n = explicit.Value
	}
	if alias, isAlias := n.(*ast.AliasNode); isAlias {
		return p.anchorIdentities[anchorNameOf(alias.Value)].identity
	}

	return ast.KeyIdentityWithAnchors(key, p.anchorIdentityOf)
}

// anchorIdentityOf is what the node an anchor names resolves to, for
// [ast.KeyIdentityWithAnchors] to answer an alias with.
//
// Taken when the anchor closed, so it costs a lookup rather than a walk of the
// anchored subtree -- and an anchor still being read is not in the table, which
// is what stops "&x [ *x ]" naming itself.
func (p *Parser) anchorIdentityOf(name string) (string, bool) {
	at, known := p.anchorIdentities[name]
	if !known || at.identity == "" {
		return "", false
	}

	return at.identity, true
}

// keyDisplayName is what a refusal calls a key a single token cannot name.
//
// The rendered node rather than the identity: "[a]" reads back to a user where
// "seq(string/a)" does not. The explicit "?" comes off and the lines are joined
// with a space, so a key written over three lines still names itself on the one
// line an error message has.
func keyDisplayName(n ast.Node) string {
	if key, explicit := n.(*ast.MappingKeyNode); explicit {
		n = key.Value
	}
	if n == nil {
		return ""
	}

	return strings.Join(strings.Fields(n.String()), " ")
}

// isScalarKeyToken reports whether tk is a scalar written where a key goes,
// quoted or not.
// isScalarKeyToken reports whether a key's token sits where the entry begins,
// so that the column of what follows can be measured against it.
//
// A plain or quoted key does. So does the implicit null standing for a key that
// was never written: implicitNullKeyToken copies the ':' position, and the ':'
// is where the entry begins. Without it ":\n1\n" read as {null: 1}, where the
// same document with the key written out, "k:\n1\n", is refused.
func isScalarKeyToken(tk *token.Token) bool {
	switch tk.Type {
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType, token.ImplicitNullType:
		return true
	default:
		return false
	}
}

// mapKeyText is the key as the document wrote it, which is what a path
// addresses the entry by: "$.0x10" reaches the entry written "0x10:" whatever
// the number resolves to. Telling one key from another is a different question
// and mapKeyIdentity's.
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

func (p *Parser) mapKeyIdentity(n ast.Node) (string, token.KeyKind) {
	if n == nil {
		return "", token.KeyOther
	}

	switch nn := n.(type) {
	case *ast.MappingKeyNode:
		return p.mapKeyIdentity(nn.Value)
	case *ast.AnchorNode:
		return p.mapKeyIdentity(nn.Value)
	case *ast.TagNode:
		// A tag names the type, so it names the key's identity: "!!str 1" is
		// the string "1" and not the integer, and the two are two keys.
		// Unwrapping to the node under it read the tag off and made them one.
		if name, kind, tagged := ast.TaggedKeyName(nn); tagged {
			return name, kind
		}

		return p.mapKeyIdentity(nn.Value)
	case *ast.AliasNode:
		// An alias node is the node its anchor named (3.2.2.2), so it is that
		// node's key. keepAnchorIdentity read it where the node was whole; a
		// walk has scrubbed it by now, and AliasNode.Target points at a cell
		// holding whatever was built there next.
		//
		// A scalar anchor answers here and is recorded among the scalar keys,
		// so "{&a x: 1, *a : 2}" is one key written twice. A collection anchor
		// hands back nothing and is checked when its entry is built, by
		// builtKeyIdentity.
		at := p.anchorIdentities[anchorNameOf(nn.Value)]

		return at.text, at.kind
	case *ast.StringNode:
		// The node's own text and the string kind, not the token's, which is
		// what [ast.KeyName] reads for the same reason.
		//
		// A "<<" that the core schema leaves an ordinary key reaches here as a
		// StringNode over a token still typed MergeKey, and token.KeyName has
		// no case for that type: it fell through to KeyOther, so the entry was
		// recorded as "other/<<" where a bare "{<<}" is recorded as
		// "string/<<". A key's identity is its kind and its name, so the two
		// never met and "{<<: {x: 1}, <<}" read as {"<<": nil} with the first
		// entry's mapping gone and nothing reported. Under 1.1 the merge key
		// is a merge key and reaches here as another node, so it keeps its own
		// identity and still collides only with another merge key.
		return nn.Value, token.KeyString
	case *ast.LiteralNode:
		// A literal or folded block scalar is a string whatever it spells, and
		// its own token is the header, "|-" or ">-". Falling through to the
		// token named every block scalar key after its header, so two of them
		// collided however differently they read. Named by the string instead,
		// "? |-" over "  a" is the same key as a plain "a", which 3.2.1.1 makes
		// it -- and it is recorded beside the plain keys, so the two meet.
		if nn.Value == nil {
			return "", token.KeyString
		}

		return nn.Value.Value, token.KeyString
	case *ast.SequenceNode, *ast.MappingNode, *ast.MappingValueNode:
		// A key a single token cannot name. It is checked when the mapping
		// closes instead, by [ast.KeyIdentity] over the built node: a
		// collection is named by what it holds, and it holds nothing while the
		// entry is being read.
		//
		// Naming one here has been tried three ways and all three were wrong.
		// The node's first token names every sequence key "[" and every block
		// mapping key ":". Nothing at all stops the check, and the walk then
		// folds a repeat's two entries into one and drops the first value. The
		// document's own source text reads "[a]" and "[ a ]" as two keys, which
		// the walk then folds anyway -- the same dropped value, reached by a
		// longer route.
		return "", token.KeyOther
	}

	tk := n.GetToken()
	if tk == nil {
		return "", token.KeyOther
	}

	return token.KeyName(tk.Value, tk.Type)
}
