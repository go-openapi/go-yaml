// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// refuseDuplicateKeys reports the first key the parse recorded as repeated on
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
func refuseDuplicateKeys(n ast.Node) error {
	m, ok := n.(*ast.MappingNode)
	if !ok {
		return nil
	}
	d, refused := firstRefusedDuplicate(m.Duplicates)
	if !refused {
		return nil
	}

	return duplicateKeyError(d, keyTokenAt(m, d))
}

// newAllowedRepeat reports whether node has recorded a repeat since seen, the
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
func newAllowedRepeat(node *ast.MappingNode, seen *int) bool {
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

// refuseOrderedMapDuplicates reports the first key the parse recorded as
// repeated across the entries of an "!!omap", as refuseDuplicateKeys does for a
// mapping. seq is the sequence the tag stands on, and may be nil.
//
// Each entry is a mapping of its own, so the parse records a repeat across
// entries on the sequence, and every load reads it there.
func refuseOrderedMapDuplicates(seq *ast.SequenceNode) error {
	if seq == nil {
		return nil
	}
	d, refused := firstRefusedDuplicate(seq.Duplicates)
	if !refused {
		return nil
	}

	// A walk hands the sequence over without its entries, so the key's token
	// is not to hand. See keyTokenAt.
	return duplicateKeyError(d, token.New(d.Name, d.Name, d.At))
}

// allowedRepeatEntries returns the index of each "!!omap" entry whose key an
// earlier entry wrote, where the parse allowed the repeat. A converter writing
// JSON drops these entries and keeps the first. It is nil for nearly every
// ordered map.
func allowedRepeatEntries(seq *ast.SequenceNode) []int {
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

// distinctKeys returns how many keys m holds, a repeat the parse recorded
// counting once: "{a: 1, a: 2}" holds one key written twice, which is a repeat
// and not a second entry.
func distinctKeys(m *ast.MappingNode) int {
	return len(m.Values) - len(m.Duplicates)
}

// orderedMapSequence returns the sequence an "!!omap" tag stands on, looking
// through an anchor, and nil where the tag stands on something else.
func orderedMapSequence(n ast.Node) *ast.SequenceNode {
	for {
		switch nn := n.(type) {
		case *ast.AnchorNode:
			n = nn.Value
		case *ast.SequenceNode:
			return nn
		default:
			return nil
		}
	}
}

// firstRefusedDuplicate returns the first repeat of dups that the parse did not
// allow, and whether there is one.
func firstRefusedDuplicate(dups []ast.DuplicateKey) (ast.DuplicateKey, bool) {
	for _, d := range dups {
		if !d.Allowed {
			return d, true
		}
	}

	return ast.DuplicateKey{}, false
}

// duplicateKeyError is the error a load returns for the repeat d, pointing at
// tk.
func duplicateKeyError(d ast.DuplicateKey, tk *token.Token) error {
	if d.JSONNameOnly {
		return yamlerrors.NewNotJSON(
			fmt.Sprintf("two keys write the JSON member %q, first defined at [%d:%d]",
				d.Name, d.FirstAt.Line, d.FirstAt.Column),
			tk,
		)
	}

	return yamlerrors.NewDuplicateKey(
		fmt.Sprintf("mapping key %q already defined at [%d:%d]", d.Name, d.FirstAt.Line, d.FirstAt.Column),
		tk,
	)
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
	// spans, its EndLine reads 0, and printer.PrintErrorSource then draws a
	// window that closes before it opens and shows nothing.
	return token.New(d.Name, d.Name, pos)
}
