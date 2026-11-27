// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// AnchorNode is an anchor, "&name", on the node it names.
type AnchorNode struct {
	BaseNode

	Start *token.Token
	Name  Node
	Value Node
}

func (n *AnchorNode) SetName(name string) error {
	if n.Name == nil {
		return ErrInvalidAnchorName
	}
	s, ok := n.Name.(*StringNode)
	if !ok {
		return ErrInvalidAnchorName
	}
	s.Value = name
	return nil
}

// Type returns [AnchorType].
func (n *AnchorNode) Type() NodeType { return AnchorType }

// GetToken returns n.Start, the "&name".
func (n *AnchorNode) GetToken() *token.Token {
	return n.Start
}

func (n *AnchorNode) GetValue() any {
	return n.Value.GetToken().Value
}

// String renders "&name" and the anchored node.
func (n *AnchorNode) String() string {
	return defaultRenderer.String(n)
}

// IsMergeKey returns whether the anchored node is a merge key.
func (n *AnchorNode) IsMergeKey() bool {
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

func (n *AnchorNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// AliasNode is an alias, "*name", to an anchored node.
type AliasNode struct {
	BaseNode

	Start *token.Token
	Value Node
	// Target is the node the alias names, which the parser fills as it reads
	// the alias. Read it to expand an alias without collecting anchors of your
	// own; the parser resolves the name where the alias stands, so two aliases
	// of one name written either side of a redefined anchor point at the two
	// nodes and not both at the last.
	//
	// It is nil for an alias built by hand and for one the encoder writes,
	// neither of which went through a parse.
	//
	// ⚠️ Target may reach back to a node the alias stands inside: "&x [ *x ]"
	// is a document, and YAML's representation is a graph. Expand it with a
	// guard, as [github.com/go-openapi/go-yaml.Decoder] does -- it rejects a
	// cycle, because a Go value built by walking has nowhere to put one.
	//
	// Nothing follows Target on its own: [Walk] does not, and neither does the
	// renderer, which writes an alias as the "*name" it was written as.
	Target Node
}

func (n *AliasNode) SetName(name string) error {
	if n.Value == nil {
		return ErrInvalidAliasName
	}
	s, ok := n.Value.(*StringNode)
	if !ok {
		return ErrInvalidAliasName
	}
	s.Value = name
	return nil
}

// Type returns [AliasType].
func (n *AliasNode) Type() NodeType { return AliasType }

// GetToken returns n.Start, the "*name".
func (n *AliasNode) GetToken() *token.Token {
	return n.Start
}

func (n *AliasNode) GetValue() any {
	return n.Value.GetToken().Value
}

// String returns "*name".
func (n *AliasNode) String() string {
	return "*" + n.Value.String()
}

// IsMergeKey returns false.
func (n *AliasNode) IsMergeKey() bool {
	return false
}

func (n *AliasNode) stringWithoutComment() string {
	// An alias carries no comment of its own, and it is the '*' that makes it
	// one: dropping it here would write the name as a plain scalar.
	return n.String()
}

// TagNode is a tag, such as "!!str", on the node it types.
type TagNode struct {
	BaseNode

	Start *token.Token
	Value Node
	// URI is the tag this node carries, expanded from the shorthand the
	// document wrote: "!!int" and "!<tag:yaml.org,2002:int>" both give
	// tag:yaml.org,2002:int, and "!thing" gives "!thing", since the primary
	// handle expands to "!". A "%TAG" directive changes what a handle expands
	// to, so read this instead of Start to find which tag the node carries.
	URI string
	// Schema is the scalar schema the document is read under, which sets the
	// spellings the tag's type accepts: "017" is octal under %YAML 1.1 and
	// decimal without it, and "0b101" is an integer under the first and no
	// number at all under the second.
	//
	// The scanner types a plain scalar under this already, so for one the
	// token already holds the type. A quoted or block scalar it never sniffs --
	// the quotes make it a string when nothing else types it -- and a tag does
	// type it, so its text is read under this instead. Both spellings of the
	// same content then resolve alike, as §3.3.2 requires: the
	// "!" non-specific tag is given only to a node *lacking* an explicit tag,
	// so `!!int "017"` and `!!int 017` are one node and cannot differ.
	//
	// It is stamped on the node for the same reason LaxTags is: a subtree
	// handed to a consumer carries how to read it.
	Schema token.Schema
	// LaxTags records that the document was read with
	// [github.com/go-openapi/go-yaml/parser.WithLaxTags], so a consumer that
	// can fall back to the text should, where it would otherwise reject a tag
	// naming a type its scalar is not. [TagNode.Resolve] reports it as
	// [Resolution.Lax].
	//
	// It is stamped on the node instead of kept beside the document, so that a
	// node handed over on its own -- as DecodeFromNode and
	// expressions.Path.Read receive one -- carries the policy it was read under.
	LaxTags bool
	// Implicit records that the parse built this tag and the document did not write it.
	//
	// YAML 1.1 resolves types 1.2's core schema leaves out, and a timestamp is
	// one: under a "%YAML 1.1" directive, or
	// [github.com/go-openapi/go-yaml/parser.WithYAMLVersion] at 1.1,
	// "a: 2001-12-14" resolves to tag:yaml.org,2002:timestamp where under 1.2
	// it is the string. The tag records which type the scalar resolved to,
	// where the document wrote nothing.
	//
	// A renderer writing the document back leaves an implicit tag off, since
	// the source holds none; one reformatting to explicit tags writes it. A
	// consumer reading types -- the decoder, ToJSON -- reads URI and does not
	// care which way it arrived.
	Implicit bool
}

func (n *TagNode) GetValue() any {
	scalar, ok := n.Value.(ScalarNode)
	if !ok {
		return nil
	}
	return scalar.GetValue()
}

// Type returns [TagType].
func (n *TagNode) Type() NodeType { return TagType }

// GetToken returns n.Start, the tag as written.
func (n *TagNode) GetToken() *token.Token {
	return n.Start
}

// String renders the tag and the tagged node.
func (n *TagNode) String() string {
	return defaultRenderer.String(n)
}

// IsMergeKey returns true for "!!merge", and otherwise whether the tagged node is a merge key.
func (n *TagNode) IsMergeKey() bool {
	if name, known := token.ReservedTagOf(n.URI); known && name == token.MergeTag {
		// The tag names the type, and a written tag is not tied to a spec
		// version: "!!merge << : *a" folds under the core schema, where a bare
		// "<<" is an ordinary key. Reading through to the value would ask the
		// wrong question there, since the parser builds a string for the "<<"
		// it did not resolve.
		return true
	}
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

func (n *TagNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}
