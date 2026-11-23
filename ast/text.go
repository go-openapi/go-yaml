// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// Text and Bytes hand a scalar over as the document wrote it, leaving what it
// means to the caller.
//
// A number's text is the source's own bytes -- the scanner cuts it as a window
// into the document rather than copying it -- so a caller that validates
// numbers rather than converting them reads the document itself and allocates
// nothing. A quoted or block scalar had to be rewritten while it was scanned,
// and its text is that rewrite.
//
// GetValue converts, and converts when it is asked to: parsing a document whose
// numbers nothing reads converts none of them.

// Text returns the null as it was written: "null", "~", or nothing at all
// where the document left the value out.
//
// The scanner gives the token of a value left out the text "null", which Text does not return.
func (n *NullNode) Text() string {
	if n.Token.Type == token.ImplicitNullType {
		return ""
	}

	return n.Token.Value
}

// Bytes returns [NullNode.Text] as bytes, without copying it: nil where the document left the value out.
func (n *NullNode) Bytes() []byte {
	if n.Token.Type == token.ImplicitNullType {
		return nil
	}

	return n.Token.Bytes()
}

// Text returns the integer's digits as the document wrote them, with the base
// prefix and any '_' separators still in place.
func (n *IntegerNode) Text() string { return n.Token.Value }

// Bytes returns [IntegerNode.Text] as bytes, without copying it.
func (n *IntegerNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the float as the document wrote it.
func (n *FloatNode) Text() string { return n.Token.Value }

// Bytes returns [FloatNode.Text] as bytes, without copying it.
func (n *FloatNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the string, with its quotes and escapes resolved.
func (n *StringNode) Text() string { return n.Value }

// Bytes returns [StringNode.Text] as bytes, without copying it.
func (n *StringNode) Bytes() []byte { return token.TextBytes(n.Value) }

// Text returns the block scalar's content, folded and chomped, or nothing
// where it has none.
func (n *LiteralNode) Text() string {
	if n.Value == nil {
		return ""
	}

	return n.Value.Text()
}

// Bytes returns [LiteralNode.Text] as bytes, without copying it.
func (n *LiteralNode) Bytes() []byte {
	if n.Value == nil {
		return nil
	}

	return n.Value.Bytes()
}

// Text returns "<<".
func (n *MergeKeyNode) Text() string { return n.Token.Value }

// Bytes returns [MergeKeyNode.Text] as bytes, without copying it.
func (n *MergeKeyNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the bool as the document wrote it, in the case it wrote it in.
func (n *BoolNode) Text() string { return n.Token.Value }

// Bytes returns [BoolNode.Text] as bytes, without copying it.
func (n *BoolNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the infinity as the document wrote it: ".inf" or "-.inf", in
// the case it wrote it in.
func (n *InfinityNode) Text() string { return n.Token.Value }

// Bytes returns [InfinityNode.Text] as bytes, without copying it.
func (n *InfinityNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the not-a-number as the document wrote it, in the case it wrote
// it in.
func (n *NanNode) Text() string { return n.Token.Value }

// Bytes returns [NanNode.Text] as bytes, without copying it.
func (n *NanNode) Bytes() []byte { return n.Token.Bytes() }

// Text returns the text of the node the anchor names, as [AnchorNode.GetValue]
// does.
func (n *AnchorNode) Text() string { return n.Value.GetToken().Value }

// Bytes returns [AnchorNode.Text] as bytes, without copying it.
func (n *AnchorNode) Bytes() []byte { return n.Value.GetToken().Bytes() }

// Text returns the text of the node the alias names, as [AliasNode.GetValue]
// does.
func (n *AliasNode) Text() string { return n.Value.GetToken().Value }

// Bytes returns [AliasNode.Text] as bytes, without copying it.
func (n *AliasNode) Bytes() []byte { return n.Value.GetToken().Bytes() }

// Text returns the text of the tagged scalar, or nothing where what is tagged
// is a collection.
func (n *TagNode) Text() string {
	scalar, ok := n.Value.(ScalarNode)
	if !ok {
		return ""
	}

	return scalar.Text()
}

// Bytes returns [TagNode.Text] as bytes, without copying it.
func (n *TagNode) Bytes() []byte {
	scalar, ok := n.Value.(ScalarNode)
	if !ok {
		return nil
	}

	return scalar.Bytes()
}
