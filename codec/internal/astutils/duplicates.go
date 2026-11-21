// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// RefuseDuplicateKeys reports the first key the parse recorded as repeated on
// the mapping.
//
// The parse records repeats and refuses nothing, so a document that repeats a
// key can still be read, rendered and linted; what to do about one is the
// load's. §3.2.1.1 makes it an error, so the load refuses it.
//
// A repeat marked [ast.DuplicateKey.Allowed], which parser.WithAllowDuplicateMapKey
// asks for, is not refused: the load keeps one of the entries.
//
// A parse under parser.WithJSONCompatible records one more pair: two keys that
// YAML tells apart and JSON does not, such as "1" and "\"1\"". Those are two
// keys rather than a repeat, so the complaint is ErrNotJSON.
func RefuseDuplicateKeys(n ast.Node) error { return RefuseRepeats(n, false) }

// RefuseRepeats reports the first key the parse recorded as repeated on the
// mapping for a reader that keys its entries by name, byName, or by value.
//
// A reader keyed by name -- a struct, a map[string]T, an inline map, an any
// under UseStringKeys -- holds one entry for "1" and "\"1\"", two keys YAML
// tells apart, and refuses the pair as it refuses a repeat: by default, and not
// under AllowDuplicateMapKey, where the last of them stands. A reader keyed by
// value holds both. See [ast.DuplicateKey.Repeats].
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

// NewAllowedRepeat reports whether node has recorded a repeat since seen, the
// count its reader last saw, and whether that repeat is one the parse allowed.
// It moves seen on to what node holds now.
//
// A converter writing JSON asks it as an entry's value begins. The parse
// records a key before the value under it is handed over, so the record of a
// repeat is there by then -- even for a key under an anchor, a tag or a "?",
// which goes over before the parse has named it.
//
// A key the parse can name only once the entry is built records its repeat
// after the value has gone over, and the entry after it would take the drop.
// Those are the collection keys, and neither converter reads one: both parse
// under parser.WithJSONCompatible, which refuses a sequence or a mapping
// standing as a key.
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

// RefuseOrderedMapDuplicates reports the first key the parse recorded as
// repeated across the entries of an "!!omap", as RefuseDuplicateKeys does for a
// mapping. seq is the sequence the tag stands on, and may be nil.
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

// AllowedRepeatEntries returns the index of each "!!omap" entry whose key an
// earlier entry wrote, where the parse allowed the repeat. A converter writing
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
	// token.New fills in the rest: a token built as a struct literal carries no
	// spans, its EndLine reads 0, and excerpt.Source then draws a
	// window that closes before it opens and shows nothing.
	return token.New(d.Name, d.Name, pos)
}
