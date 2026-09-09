// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// KeyIdentity writes what a mapping key resolves to, so that two keys the
// document spelled differently come out the same where 3.2.1.1 makes them one
// key.
//
// The text a document wrote is not the identity. "[a]", "[ a ]", "[a,]" and
// '["a"]' are four spellings of one sequence holding one string, and naming a
// key by its own characters reads them as four keys. Naming it by its first
// token is worse: every sequence key becomes "[", every block scalar key "|-",
// and each collides with every other.
//
// A scalar's identity is its type and that type's canonical spelling, which is
// [token.KeyName]'s rule -- "7" and "007" are one integer and so one key, where
// "1" and "1.0" are an integer and a float and so two. A collection's is its
// kind and the identities of what it holds, in order.
//
// It reads the built node, so it can only be asked once the node is built: a
// key is a run of tokens more often than not, and the parser has none of its
// children while it is still cutting them.
//
// The empty string is no identity at all, which [Unnamed] reports. An alias
// whose anchor the parse could not fill is the case that reaches it.
func KeyIdentity(n Node) string {
	return KeyIdentityWithAnchors(n, nil)
}

// AnchorIdentity answers what the node an anchor names resolves to, by the
// anchor's name, and reports whether it knows.
//
// It is how an alias is named without reading [AliasNode.Target]. A caller that
// holds the anchors -- the parser does, and records each identity as the anchor
// closes -- answers in constant time from a string it already has.
type AnchorIdentity func(name string) (string, bool)

// KeyIdentityWithAnchors is [KeyIdentity] with an alias answered from anchors
// rather than left unnamed.
//
// ⚠️ It never reads [AliasNode.Target], and neither does KeyIdentity. Following
// it expands the alias graph into the identity string, and an alias graph is
// exponential in the document's length: nine anchors each naming the one before
// it nine times is 200 bytes of YAML and 9^9 expansions, which built a **3.7 GB
// string in 9.7 seconds** before this took the target out. maxIdentityDepth is
// no guard, since the depth there is 9 and the width does the damage.
//
// So an alias costs one map lookup whatever it names. anchors may be nil, and
// then an alias contributes nothing and the key goes unnamed, which a caller
// reads with [Unnamed].
func KeyIdentityWithAnchors(n Node, anchors AnchorIdentity) string {
	var b strings.Builder
	if !writeKeyIdentity(&b, n, 0, anchors) {
		return ""
	}

	return b.String()
}

// Unnamed reports whether KeyIdentity gave up on a node.
func Unnamed(identity string) bool { return identity == "" }

// writeKeyIdentity appends n's identity to b, and reports whether it had one.
//
// depth bounds the descent: an alias may name a collection that holds the
// alias, and "&a [ *a ]" is a document the parser reads.
func writeKeyIdentity(b *strings.Builder, n Node, depth int, anchors AnchorIdentity) bool {
	if n == nil || depth > maxIdentityDepth || b.Len() > maxIdentityBytes {
		return false
	}

	switch nn := n.(type) {
	case *MappingKeyNode:
		return writeKeyIdentity(b, nn.Value, depth, anchors)
	case *AnchorNode:
		// The anchor names the node; it does not change what the node is.
		return writeKeyIdentity(b, nn.Value, depth, anchors)
	case *AliasNode:
		// The anchor's own identity, taken when the anchor closed. Reading
		// Target instead walks the anchored subtree again, once per alias
		// naming it, which is the amplification KeyIdentityWithAnchors
		// describes.
		if anchors == nil {
			return false
		}
		identity, known := anchors(anchorName(nn))
		if !known {
			return false
		}
		b.WriteString(identity)

		return true
	case *TagNode:
		return writeTaggedIdentity(b, nn, depth, anchors)
	case *LiteralNode:
		// A literal or folded block scalar is a string whatever it spells, and
		// its own token is the header -- "|-" or ">-". Falling through to the
		// token named every block scalar key after its header, so two of them
		// collided however differently they read.
		if nn.Value == nil {
			writeScalarIdentity(b, "", token.KeyString)

			return true
		}
		writeScalarIdentity(b, nn.Value.Value, token.KeyString)

		return true
	case *SequenceNode:
		b.WriteString("seq(")
		for i, v := range nn.Values {
			if i > 0 {
				b.WriteByte(',')
			}
			if !writeKeyIdentity(b, v, depth+1, anchors) {
				return false
			}
		}
		b.WriteByte(')')

		return true
	case *MappingNode:
		return writeEntriesIdentity(b, nn.Values, depth, anchors)
	case *MappingValueNode:
		// A mapping written as one entry, which is how "{a: 1}" arrives where
		// the braces hold a single pair.
		return writeEntriesIdentity(b, []*MappingValueNode{nn}, depth, anchors)
	}

	tk := n.GetToken()
	if tk == nil {
		return false
	}
	name, kind := token.KeyName(tk.Value, tk.Type)
	writeScalarIdentity(b, name, kind)

	return true
}

// writeEntriesIdentity writes a mapping's identity from its entries.
func writeEntriesIdentity(b *strings.Builder, entries []*MappingValueNode, depth int, anchors AnchorIdentity) bool {
	b.WriteString("map(")
	for i, e := range entries {
		if i > 0 {
			b.WriteByte(',')
		}
		if e == nil || !writeKeyIdentity(b, e.Key, depth+1, anchors) {
			return false
		}
		b.WriteByte(':')
		if !writeKeyIdentity(b, e.Value, depth+1, anchors) {
			return false
		}
	}
	b.WriteByte(')')

	return true
}

// writeTaggedIdentity writes the identity of a node under a tag.
//
// A tag names the type, so it names the identity: "!!str 1" is the string and
// "1" is the integer, and the two are two keys. A tag the schema does not
// resolve leaves the node to speak for itself.
func writeTaggedIdentity(b *strings.Builder, n *TagNode, depth int, anchors AnchorIdentity) bool {
	name, kind, tagged := TaggedKeyName(n)
	if !tagged {
		// A tag the schema does not resolve, or one naming a kind -- !!seq,
		// !!map, !!binary, !!timestamp. What it tags is what it is.
		return writeKeyIdentity(b, n.Value, depth, anchors)
	}
	writeScalarIdentity(b, name, kind)

	return true
}

// writeScalarIdentity writes a scalar's kind and canonical spelling.
//
// The kind is written too, so that the string "1" and the integer 1 do not
// share an identity: 3.2.1.1 makes them two keys.
func writeScalarIdentity(b *strings.Builder, name string, kind token.KeyKind) {
	b.WriteString(kind.String())
	b.WriteByte('/')
	b.WriteString(name)
}

// maxIdentityDepth bounds the descent.
const maxIdentityDepth = 64

// maxIdentityBytes caps how long an identity may grow, and a key that reaches
// it goes unnamed.
//
// The depth bound alone does not hold: an alias graph is exponential in the
// document's *width*, not its depth. Nine anchors each naming the one before it
// nine times is 9^9 expansions from 200 bytes of YAML.
//
// A key too big to name is simply not compared, so a repeat of one goes
// unreported. That is the quiet direction and the safe one: the loud failure
// would be naming two different keys alike and refusing a valid document.
const maxIdentityBytes = 4096

// anchorName is the name an alias writes.
func anchorName(n *AliasNode) string {
	if n == nil || n.Value == nil {
		return ""
	}
	tk := n.Value.GetToken()
	if tk == nil {
		return ""
	}

	return tk.Value
}
