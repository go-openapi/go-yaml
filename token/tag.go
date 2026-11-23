// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "strings"

// YAMLTagPrefix is the prefix the secondary tag handle "!!" expands to unless
// a "%TAG" directive gives it another, so "!!int" and
// "!<tag:yaml.org,2002:int>" name one tag and not two.
const YAMLTagPrefix = "tag:yaml.org,2002:"

// ReservedTagOf returns the reserved tag a URI names, or false for every other URI: a tag the document defines,
// or a tag of another namespace.
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
	if _, reserved := reservedTags[keyword]; !reserved {
		return "", false
	}

	return keyword, true
}

// ReservedTagKeyword is a tag of the YAML type repository, in its "!!" shorthand, such as "!!int".
type ReservedTagKeyword string

const (
	// IntegerTag is "!!int".
	IntegerTag ReservedTagKeyword = "!!int"
	// FloatTag is "!!float".
	FloatTag ReservedTagKeyword = "!!float"
	// NullTag is "!!null".
	NullTag ReservedTagKeyword = "!!null"
	// SequenceTag is "!!seq".
	SequenceTag ReservedTagKeyword = "!!seq"
	// MappingTag is "!!map".
	MappingTag ReservedTagKeyword = "!!map"
	// StringTag is "!!str".
	StringTag ReservedTagKeyword = "!!str"
	// BinaryTag is "!!binary".
	BinaryTag ReservedTagKeyword = "!!binary"
	// OrderedMapTag is "!!omap".
	OrderedMapTag ReservedTagKeyword = "!!omap"
	// SetTag is "!!set".
	SetTag ReservedTagKeyword = "!!set"
	// TimestampTag is "!!timestamp".
	TimestampTag ReservedTagKeyword = "!!timestamp"
	// BooleanTag is "!!bool".
	BooleanTag ReservedTagKeyword = "!!bool"
	// MergeTag is "!!merge".
	MergeTag ReservedTagKeyword = "!!merge"
)

// reservedTags holds the tags YAML 1.2 reserves. A tag outside it belongs to the document, and a scalar carrying
// one keeps the type of its text.
var reservedTags = map[ReservedTagKeyword]struct{}{
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
