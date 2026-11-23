// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "strings"

// YAMLTagPrefix is the prefix the secondary tag handle "!!" expands to unless
// a "%TAG" directive gives it another, so "!!int" and
// "!<tag:yaml.org,2002:int>" name one tag and not two.
const YAMLTagPrefix = "tag:yaml.org,2002:"

// ReservedTagOf returns the reserved tag a URI names, and reports false for
// every other URI -- a tag the document defines, or one of another namespace.
//
// A document usually writes the shorthand "!!int" and means
// tag:yaml.org,2002:int. Expanding the shorthand before matching here resolves
// both spellings to one tag, and lets a "%TAG !!" line that repoints the
// handle take "!!int" out of this namespace.
func ReservedTagOf(uri string) (ReservedTagKeyword, bool) {
	name, ok := strings.CutPrefix(uri, YAMLTagPrefix)
	if !ok {
		return "", false
	}

	keyword := ReservedTagKeyword("!!" + name)
	if _, reserved := ReservedTagKeywordMap[keyword]; !reserved {
		return "", false
	}

	return keyword, true
}

// ReservedTagKeyword type of reserved tag keyword.
type ReservedTagKeyword string

const (
	// IntegerTag `!!int` tag.
	IntegerTag ReservedTagKeyword = "!!int"
	// FloatTag `!!float` tag.
	FloatTag ReservedTagKeyword = "!!float"
	// NullTag `!!null` tag.
	NullTag ReservedTagKeyword = "!!null"
	// SequenceTag `!!seq` tag.
	SequenceTag ReservedTagKeyword = "!!seq"
	// MappingTag `!!map` tag.
	MappingTag ReservedTagKeyword = "!!map"
	// StringTag `!!str` tag.
	StringTag ReservedTagKeyword = "!!str"
	// BinaryTag `!!binary` tag.
	BinaryTag ReservedTagKeyword = "!!binary"
	// OrderedMapTag `!!omap` tag.
	OrderedMapTag ReservedTagKeyword = "!!omap"
	// SetTag `!!set` tag.
	SetTag ReservedTagKeyword = "!!set"
	// TimestampTag `!!timestamp` tag.
	TimestampTag ReservedTagKeyword = "!!timestamp"
	// BooleanTag `!!bool` tag.
	BooleanTag ReservedTagKeyword = "!!bool"
	// MergeTag `!!merge` tag.
	MergeTag ReservedTagKeyword = "!!merge"
)

// ReservedTagKeywordMap holds the tags YAML 1.2 reserves. A tag outside it is
// the document's own, and a scalar carrying one keeps the type its text says
// rather than the one the tag would resolve to.
var ReservedTagKeywordMap = map[ReservedTagKeyword]struct{}{
	IntegerTag:    {},
	FloatTag:      {},
	NullTag:       {},
	SequenceTag:   {},
	MappingTag:    {},
	StringTag:     {},
	BinaryTag:     {},
	OrderedMapTag: {},
	SetTag:        {},
	TimestampTag:  {},
	BooleanTag:    {},
	MergeTag:      {},
}
