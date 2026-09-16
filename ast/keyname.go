// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"time"

	"github.com/go-openapi/go-yaml/token"
)

// KeyName is the name a mapping key is identified by, and the type it resolved
// to.
//
// ⚠️ It answers "is this the same key?", not "how is this key written". Two keys
// with one name are one key, and 3.2.1.1 makes that an error. Nothing here
// renders a document: for the characters a key is written with, call the node's
// String, or build a [Renderer] with [NewRenderer].
//
// Three callers read it, and two of the three want a different form:
//
//   - the parser's duplicate check compares keys, so it reads [ComparedKeyName],
//     which folds this name through [CanonicalKeyName]. The two differ only for
//     a timestamp, which compares as its instant in UTC.
//   - codec names a Go map entry or a JSON member by this name and not the
//     compared one, so a timestamp key keeps the zone the document wrote it in.
//   - [KeyIdentity] names the keys this one leaves unnamed, which are the
//     collections.
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
// Those three named a key from a node separately before this, and only codec
// left out the tag and the alias, so "!!float 1.0: x" came back keyed "1" -- a
// float in the integers' namespace, where an entry keyed "1" then displaces it.
func KeyName(n Node) (string, token.KeyKind) {
	name, kind, named := ScalarKeyName(n, aliasTarget)
	if !named {
		return "", token.KeyOther
	}

	return name, kind
}

// aliasTarget names an alias as the node its anchor named (3.2.2.2), through
// [AliasNode.Target]. Reading Target is safe on a walk as well as on a tree
// since ast.Arena.Commit holds an anchored node's cells for the document;
// before that it read another part of the document.
//
// A scalar answers in constant time and a collection hands back nothing, so
// this cannot expand the way an identity does -- there is no structure to write
// out, only a name. The target is named with no alias rule of its own: an
// anchored node is never an alias.
func aliasTarget(a *AliasNode) (string, token.KeyKind, bool) {
	return scalarKeyNameAt(a.Target, 1, nil)
}

// KeyAlias names the key an alias stands for, and reports false where it cannot.
//
// Each reader of a key answers an alias its own way: [KeyName] through
// [AliasNode.Target], the parser through the identities its anchors recorded,
// which a walk still holds once the anchored node is gone.
type KeyAlias func(*AliasNode) (string, token.KeyKind, bool)

// ScalarKeyName names the scalar a mapping key stands for, and its type. It is
// the one walk from a key node to its name: [KeyName], [KeyIdentity] and the
// parser's duplicate check all read it, so a rule for one kind of key reaches
// all three.
//
// It names a key to tell it from another, not to write it out. See [KeyName].
//
// It looks through an explicit "?", an anchor and a tag that names no type. A
// tag that names one says what the key is, through [TaggedKeyName]. A quoted
// key is named by what its escapes resolved to, a block scalar by its string
// and not its "|-" header, and an alias by alias, which may be nil.
//
// It reports false for a collection, which [KeyIdentity] names from what it
// holds, and for an alias alias does not answer.
func ScalarKeyName(n Node, alias KeyAlias) (string, token.KeyKind, bool) {
	return scalarKeyNameAt(n, 0, alias)
}

// ComparedKeyName is [ScalarKeyName] in the form two keys are compared by,
// which [CanonicalKeyName] gives.
//
// Use it to compare two keys. [KeyName] gives the same string except for a
// timestamp, which this one writes as its instant in UTC.
func ComparedKeyName(n Node, alias KeyAlias) (string, token.KeyKind, bool) {
	name, kind, named := ScalarKeyName(n, alias)
	if !named {
		return "", token.KeyOther, false
	}

	return CanonicalKeyName(name, kind), kind, true
}

// scalarKeyNameAt is [ScalarKeyName] with the descent bounded: a tag stands
// over a node that may carry another.
func scalarKeyNameAt(n Node, depth int, alias KeyAlias) (string, token.KeyKind, bool) {
	if n == nil || depth > maxKeyNameDepth {
		return "", token.KeyOther, false
	}

	switch nn := n.(type) {
	case *MappingKeyNode:
		return scalarKeyNameAt(nn.Value, depth+1, alias)
	case *AnchorNode:
		return scalarKeyNameAt(nn.Value, depth+1, alias)
	case *TagNode:
		if name, kind, tagged := TaggedKeyName(nn); tagged {
			return name, kind, true
		}

		return scalarKeyNameAt(nn.Value, depth+1, alias)
	case *StringNode:
		// The node's own text and the string kind, not the token's: a
		// double-quoted key holds what the escapes resolved to, and a "<<" the
		// core schema leaves an ordinary key stands over a token still typed
		// MergeKey, which token.KeyName would file under KeyOther.
		return nn.Value, token.KeyString, true
	case *LiteralNode:
		if nn.Value == nil {
			return "", token.KeyString, true
		}

		return nn.Value.Value, token.KeyString, true
	case *AliasNode:
		if alias == nil {
			return "", token.KeyOther, false
		}

		return alias(nn)
	case *SequenceNode, *MappingNode, *MappingValueNode:
		return "", token.KeyOther, false
	}

	tk := n.GetToken()
	if tk == nil {
		return "", token.KeyOther, false
	}
	name, kind := token.KeyName(tk.Value, tk.Type)

	return name, kind, true
}

// TaggedKeyName names a key from the tag standing on it, for the tags that name
// one of the types a key is told apart by. It reports false where the tag names
// no such type, and the caller then names [TagNode.Value] instead: a tag the
// schema does not resolve, one naming a kind -- !!seq, !!map -- or one the
// application declared.
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
		name, kind := token.KeyName(res.Text, integerKeyType(res.Text, n.Schema))

		return name, kind, true
	case token.FloatTag:
		name, kind := token.KeyName(res.Text, token.FloatType)

		return name, kind, true
	case token.TimestampTag:
		// Named in RFC 3339 in the zone the document wrote, which is how
		// ToJSON writes a timestamp, so a string-keyed map and the JSON name
		// the entry alike. [CanonicalKeyName] compares it in UTC.
		stamp, ok := ParseTimestamp(res.Text)
		if !ok {
			return "", token.KeyOther, false
		}

		return stamp.Format(time.RFC3339Nano), token.KeyTimestamp, true
	case token.BinaryTag:
		// A type of its own, so "!!binary AA==" and the string "AA==" are two
		// keys. Read as its node, it was the string and the pair was refused
		// as a repeat, while the decoder held a Base64 and a string apart.
		return token.CanonicalBase64(res.Text), token.KeyBinary, true
	default:
		return "", token.KeyOther, false
	}
}

// CanonicalKeyName returns the name two keys are compared by, given the name
// and kind [KeyName] or [TaggedKeyName] returned.
//
// It differs from the name only for a timestamp, which it writes as its instant
// in UTC: yaml.org/type/timestamp.html gives that as the canonical form, and
// 3.2.1.3 compares scalars by their canonical forms. So "2001-12-14" and
// "2001-12-14 00:00:00" are one key, and so is one instant written at "-05:00"
// and in UTC.
//
// Compared by the text, two spellings passed the duplicate check and then met
// in the decoder as one time.Time, which kept the second value and dropped the
// first with nothing reported. The decoder's own == is no help: it compares a
// time.Time's *time.Location and not its offset, so "+00:00" and "Z" are two
// keys to a Go map.
func CanonicalKeyName(name string, kind token.KeyKind) string {
	if kind != token.KeyTimestamp {
		return name
	}
	stamp, err := time.Parse(time.RFC3339Nano, name)
	if err != nil {
		return name
	}

	return stamp.UTC().Format(time.RFC3339Nano)
}

// maxKeyNameDepth bounds the descent through the properties standing in front
// of a key.
const maxKeyNameDepth = 64

// integerKeyType is the type the digits under an "!!int" tag are read as,
// which settles what base they are written in.
//
// [token.IntegerBase] holds the rule and the reason. A key reaching here has
// already resolved, so a base is there to be had and the fallback never fires.
func integerKeyType(text string, schema token.Schema) token.Type {
	typ, _ := token.IntegerBase(text, schema)

	return typ
}
