// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// RefuseDuplicateKeys returns an error for the first key the parse recorded as repeated on the mapping n.
//
// The parser records repeated keys and rejects none of them,
// so a document that repeats a key can still be read, rendered and linted.
// The loader decides what to do with a repeat.
// YAML 1.2 §3.2.1.1 makes it an error, and RefuseDuplicateKeys returns one.
//
// parser.WithAllowDuplicateMapKey marks a repeat [ast.DuplicateKey.Allowed].
// RefuseDuplicateKeys returns no error for it, and the loader keeps one of the entries.
//
// A parse under parser.WithJSONCompatible records one more kind of pair:
// two keys that YAML tells apart and JSON does not, such as "1" and "\"1\"".
// The error for such a pair is ErrNotJSON, because the two keys are not a repeat.
func RefuseDuplicateKeys(n ast.Node) error { return RefuseRepeats(n, false) }

// RefuseRepeats returns an error for the first key the parse recorded as repeated on the mapping n,
// for a reader that keys its entries by name (byName) or by value.
//
// A reader keyed by name is a struct, a map[string]T, an inline map, or an any under UseStringKeys.
// It holds one entry for "1" and "\"1\"", although YAML tells the two keys apart.
// RefuseRepeats therefore treats the pair as a repeat: an error by default,
// and under AllowDuplicateMapKey the last of the two is kept.
// A reader keyed by value holds both keys. See [ast.DuplicateKey.Repeats].
func RefuseRepeats(n ast.Node, byName bool) error {
	m, ok := n.(*ast.MappingNode)
	if !ok {
		return nil
	}
	d, refused := ast.FirstRefused(m.Duplicates, byName)
	if !refused {
		return nil
	}

	return yamlerrors.NewDuplicateKeyOf(d, keyTokenAt(m, d))
}

// NewAllowedRepeat reports whether node recorded a repeat since its reader last looked, and the parser allowed it.
//
// seen holds the count of repeats the reader last saw. NewAllowedRepeat updates it to node's current count.
//
// A converter writing JSON calls it when an entry's value begins.
// The parser records a key before it hands over the key's value, so a repeat is already recorded at that point.
// This holds even for a key under an anchor, a tag or a "?", which the parser hands over before naming the key.
//
// A collection key is the exception: the parser names it only once the entry is built, after the value has gone.
// Its repeat would show at the next entry, which would be dropped in its place.
// Neither converter meets one: both parse under parser.WithJSONCompatible,
// which rejects a sequence or a mapping as a key.
func NewAllowedRepeat(node *ast.MappingNode, seen *int) bool {
	if node == nil {
		return false
	}
	n := len(node.Duplicates)
	if n == *seen {
		return false
	}
	*seen = n

	return node.Duplicates[n-1].Allowed
}

// RefuseOrderedMapDuplicates returns an error for the first key the parse recorded as repeated across the entries
// of an "!!omap", as [RefuseDuplicateKeys] does for a mapping. seq is the sequence the tag stands on, and may be nil.
//
// Each entry is a mapping of its own, so the parse records a repeat across
// entries on the sequence, and every load reads it there.
func RefuseOrderedMapDuplicates(seq *ast.SequenceNode) error {
	if seq == nil {
		return nil
	}
	d, refused := ast.FirstRefused(seq.Duplicates, false)
	if !refused {
		return nil
	}

	// A walk hands the sequence over without its entries, so the key's token
	// is not to hand. See keyTokenAt.
	return yamlerrors.NewDuplicateKeyOf(d, token.New(d.Name, d.Name, d.At))
}

// AllowedRepeatEntries returns the index of each "!!omap" entry that repeats the key of an earlier entry,
// where the parser allowed the repeat. A converter writing
// JSON drops these entries and keeps the first. It is nil for nearly every
// ordered map.
func AllowedRepeatEntries(seq *ast.SequenceNode) []int {
	if seq == nil {
		return nil
	}

	var drop []int
	for _, d := range seq.Duplicates {
		if d.Allowed {
			drop = append(drop, d.Index)
		}
	}

	return drop
}

// keyTokenAt returns the key the mapping wrote at pos for the complaint to
// point at, and the mapping's own token where the entry is not to hand.
//
// The parse keeps the position and not the token: a token held against a
// mapping outlives the entry that carried it, and a mapping of 5,000 keys would
// hold 5,000 tokens spread over the whole document.
func keyTokenAt(m *ast.MappingNode, d ast.DuplicateKey) *token.Token {
	pos := d.At

	for _, v := range m.Values {
		if v == nil || v.Key == nil {
			continue
		}
		if tk := v.Key.GetToken(); tk != nil && tk.Position.Line == pos.Line && tk.Position.Column == pos.Column {
			return tk
		}
	}

	// A walk hands the mapping over without gathering its entries, so the token
	// is not to hand. The position was recorded when the repeat was read, and
	// token.New fills in the rest.
	// A token built as a struct literal carries no spans, so its EndLine reads 0.
	// excerpt.Source then draws a window that closes before it opens, and shows nothing.
	return token.New(d.Name, d.Name, pos)
}
