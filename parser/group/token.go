// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"slices"

	"github.com/go-openapi/go-yaml/token"
)

// keyEndLine reports the line on which a key token ends.
//
// A plain scalar may span several lines and its position records where it
// starts, so the line that matters for the implicit-key rule has to be
// computed. Whitespace on either side of the origin belongs to the neighboring
// tokens rather than to this one -- a trailing newline in particular would
// otherwise push the end line one past where the token really finishes.
func keyEndLine(tk *TapeToken) int {
	raw := tk.RawToken()
	if raw == nil {
		return tk.Line()
	}

	return int(raw.EndLine())
}

// closesFlowCollection reports whether tk ends a flow collection.
func closesFlowCollection(tk *TapeToken) bool {
	return tk.Type() == token.MappingEndType || tk.Type() == token.SequenceEndType
}

// flowCollectionStart finds where the flow collection Ending at the last token
// of ret begins, so the whole of it can be taken as a mapping key. It reports
// -1 when the brackets do not balance.
func flowCollectionStart(ret []*TapeToken) int {
	var depth int
	for i, r := range slices.Backward(ret) {
		if r.GroupType() != TokenGroupNone {
			// A group is balanced within itself, and reports the type of the
			// token it opens with. A key already grouped as "[b]: d" would
			// otherwise read as one more '[' with no ']' to match it.
			continue
		}
		switch r.Type() {
		case token.MappingEndType, token.SequenceEndType:
			depth++
		case token.MappingStartType, token.SequenceStartType:
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// withKeyProperties widens a key that starts at start to take in the anchors
// and tags written before it on the same line.
//
// They are the key's, not the mapping's: the anchor of "&k [a, b]: v" names the
// sequence used as the key. Left outside, it was read as naming the mapping the
// entry belongs to -- silently, and as a second anchor on that mapping when the
// mapping already had one of its own.
func withKeyProperties(ret []*TapeToken, start int) int {
	for start > 0 && ret[start-1].Line() == ret[start].Line() {
		prev := ret[start-1]
		if prev.GroupType() != TokenGroupAnchorName && prev.Type() != token.TagType {
			break
		}
		start--
	}

	return start
}

// precedesAbsentKey reports whether tk is punctuation that cannot itself be a
// key and does not close a collection, so that a ':' right after it has no key
// at all.
//
// The exclusion of '}' and ']' is the point of this being separate from
// isNotMapKeyType: a ':' after them has a key -- the collection that just
// closed -- which is a different feature, and still unsupported. Treating those
// as absent keys would turn a clear "found an invalid key for this map" into a
// null key and a misleading error further on.
func precedesAbsentKey(tk *TapeToken) bool {
	switch tk.Type() {
	case token.CollectEntryType,
		token.MappingStartType,
		token.SequenceStartType,
		token.SequenceEntryType,
		token.MappingValueType,
		token.DirectiveType,
		token.DocumentHeaderType,
		token.DocumentEndType:
		return true
	default:
		return false
	}
}

// implicitNullKeyToken builds the null node standing in for an absent mapping
// key, positioned where the key would have been -- immediately before its ':'.
func (g *Grouper) implicitNullKeyToken(colon *TapeToken) *TapeToken {
	pos := colon.RawToken().Position
	tk := token.New("null", "null", pos)
	tk.Type = token.ImplicitNullType

	wrapped := g.token(colon.Seq())
	wrapped.raw = *tk

	return wrapped
}

func isNotMapKeyType(tk *TapeToken) bool {
	typ := tk.Type()
	return typ == token.DirectiveType ||
		typ == token.DocumentHeaderType ||
		typ == token.DocumentEndType ||
		typ == token.CollectEntryType ||
		typ == token.MappingStartType ||
		typ == token.MappingValueType ||
		typ == token.MappingEndType ||
		typ == token.SequenceStartType ||
		typ == token.SequenceEntryType ||
		typ == token.SequenceEndType
}

// isFlowType reports whether a token is punctuation a tag cannot be grouped
// with, because the token belongs to the collection around the tag rather than
// naming what the tag is on.
//
// The two closers and the ',' are here for the same reason as the openers: "[!]"
// is the non-specific tag on the empty node followed by the closer, and grouping
// the two swallowed the ']' -- the sequence then ran to the end of the stream
// looking for it.
func isFlowType(tk *TapeToken) bool {
	typ := tk.Type()
	return typ == token.MappingStartType ||
		typ == token.MappingEndType ||
		typ == token.SequenceStartType ||
		typ == token.SequenceEndType ||
		typ == token.SequenceEntryType ||
		typ == token.CollectEntryType
}
