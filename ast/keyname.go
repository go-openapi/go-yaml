// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"github.com/go-openapi/go-yaml/token"
)

// KeyName is the text a mapping key addresses an entry by, and the type it
// resolved to.
//
// [token.KeyName] holds the rule for a scalar's own spelling -- "7" and "007"
// are one integer written two ways, "1" and "1.0" are an integer and a float --
// and says why it sits in token rather than in any one caller. This is the
// other half: the walk from the node a document wrote down to the scalar that
// rule applies to.
//
// It looks through everything that stands in front of a key without being the
// key. An explicit "?" and an anchor name the node and say nothing about its
// type. A tag says what the type is, so it is resolved rather than stripped:
// "!!float 1" is the float 1.0 and is named "1.0", where stripping the tag
// would read the token "1" and name it after an integer. A block scalar is a
// string whatever it spells, and its own token is the "|-" header.
//
// The empty string under [token.KeyOther] means no name, which a caller has to
// answer for itself:
//
//   - a sequence or a mapping, which has no scalar spelling at all. Compare
//     those with [KeyIdentity].
//
// An alias is named as the node its anchor named, through [AliasNode.Target].
//
// Three packages named a key from a node before this: ast for [KeyIdentity],
// the parser for its duplicate check, and codec for the string a Go map or a
// JSON member is keyed by. Only the last left out the tag and the alias, so
// "!!float 1.0: x" came back keyed "1" -- a float in the integers' namespace,
// where an entry keyed "1" then displaces it.
func KeyName(n Node) (string, token.KeyKind) {
	return keyNameAt(n, 0)
}

// keyNameAt is [KeyName] with the descent bounded: a tag stands over a node
// that may carry another.
func keyNameAt(n Node, depth int) (string, token.KeyKind) {
	if n == nil || depth > maxKeyNameDepth {
		return "", token.KeyOther
	}

	switch nn := n.(type) {
	case *MappingKeyNode:
		return keyNameAt(nn.Value, depth+1)
	case *AnchorNode:
		return keyNameAt(nn.Value, depth+1)
	case *TagNode:
		if name, kind, tagged := TaggedKeyName(nn); tagged {
			return name, kind
		}

		return keyNameAt(nn.Value, depth+1)
	case *StringNode:
		// The node's own text, not the token's: a double-quoted key holds what
		// the escapes resolved to.
		return nn.Value, token.KeyString
	case *LiteralNode:
		if nn.Value == nil {
			return "", token.KeyString
		}

		return nn.Value.Value, token.KeyString
	case *AliasNode:
		// An alias node is the node its anchor named (3.2.2.2), so it is named
		// as that node. Reading Target is safe on a walk as well as on a tree
		// since ast.Arena.Commit holds an anchored node's cells for the
		// document; before that it read another part of the document, and this
		// arm handed back nothing rather than a wrong answer.
		//
		// A scalar answers in constant time and a collection hands back
		// nothing, so this cannot expand the way an identity does -- there is
		// no structure to write out, only a name.
		return keyNameAt(nn.Target, depth+1)
	case *SequenceNode, *MappingNode, *MappingValueNode:
		return "", token.KeyOther
	}

	tk := n.GetToken()
	if tk == nil {
		return "", token.KeyOther
	}

	return token.KeyName(tk.Value, tk.Type)
}

// TaggedKeyName names a key from the tag standing on it, for the tags that name
// one of the types a key is told apart by. It reports false where the tag names
// no such type, and the caller then names [TagNode.Value] instead: a tag the
// schema does not resolve, one naming a kind -- !!seq, !!map, !!binary -- or one
// the application declared.
//
// Three walks reach a tagged key and all three must agree, or one names a key
// that another does not: [KeyName], [KeyIdentity], and the parser's duplicate
// check, which keeps a walk of its own because it resolves an alias through the
// anchors it has recorded and not through [AliasNode.Target]. They read this
// rather than each carrying the switch. Three copies of it named "!!int 0x10"
// the string "0x10" where "0x10" alone named the integer 16, so the two
// spellings passed the duplicate check and then collided in the decoder, which
// dropped the first value.
func TaggedKeyName(n *TagNode) (string, token.KeyKind, bool) {
	res := n.Resolve()
	if res.Verdict != TagResolved {
		return "", token.KeyOther, false
	}

	switch res.Tag {
	case token.StringTag:
		return res.Text, token.KeyString, true
	case token.NullTag:
		return "null", token.KeyNull, true
	case token.BooleanTag:
		name, kind := token.KeyName(res.Text, token.BoolType)

		return name, kind, true
	case token.IntegerTag:
		name, kind := token.KeyName(res.Text, integerKeyType(n.Value, res.Text))

		return name, kind, true
	case token.FloatTag:
		name, kind := token.KeyName(res.Text, token.FloatType)

		return name, kind, true
	default:
		return "", token.KeyOther, false
	}
}

// maxKeyNameDepth bounds the descent through the properties standing in front
// of a key.
const maxKeyNameDepth = 64

// integerKeyType is the type the digits under an "!!int" tag are read as,
// which settles what base they are written in.
//
// The scanner records the base on the token: "0x10" is HexIntegerType in a 1.2
// document and "017" is OctetIntegerType in a 1.1 one, so the scalar's own
// token answers first. A scalar the tag alone turned into an integer carries
// no base -- the quoted key of `!!int "0x10"` is a DoubleQuoteType -- and its
// text is read again under the 1.2 core schema, which is what
// codec.castToInteger does with the same characters.
//
// Handing token.IntegerType over whatever the scalar was cost a based key its
// resolution: `0x10: v` named the key "16" and `!!int 0x10: v` named it
// "0x10", so the tag put the key in the strings' namespace.
func integerKeyType(n Node, text string) token.Type {
	typ, _ := integerBase(taggedScalarType(n), text)

	return typ
}
