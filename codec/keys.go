// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"time"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

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
func refuseDuplicateKeys(n ast.Node) error { return refuseRepeats(n, false) }

// refuseRepeats reports the first key the parse recorded as repeated on the
// mapping for a reader that keys its entries by name, byName, or by value.
//
// A reader keyed by name -- a struct, a map[string]T, an inline map, an any
// under UseStringKeys -- holds one entry for "1" and "\"1\"", two keys YAML
// tells apart, and refuses the pair as it refuses a repeat: by default, and not
// under AllowDuplicateMapKey, where the last of them stands. A reader keyed by
// value holds both. See [ast.DuplicateKey.Repeats].
func refuseRepeats(n ast.Node, byName bool) error {
	m, ok := n.(*ast.MappingNode)
	if !ok {
		return nil
	}
	d, refused := ast.FirstRefused(m.Duplicates, byName)
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
	d, refused := ast.FirstRefused(seq.Duplicates, false)
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

// orderedMapSequence returns the sequence an "!!omap" tag stands on, looking
// through an anchor and an alias, and nil where the tag stands on something else.
// "!!omap *x" reads the sequence x names, as the walk and ToJSON read it.
func orderedMapSequence(n ast.Node) *ast.SequenceNode {
	seq, _ := ast.Unwrap(n).(*ast.SequenceNode)

	return seq
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

	if d.NameOnly {
		return yamlerrors.NewDuplicateKey(
			fmt.Sprintf("two keys name the entry %q, first defined at [%d:%d]", d.Name, d.FirstAt.Line, d.FirstAt.Column),
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
	// spans, its EndLine reads 0, and excerpt.Source then draws a
	// window that closes before it opens and shows nothing.
	return token.New(d.Name, d.Name, pos)
}

// sameMapKey reports whether two decoded keys are one key.
//
// It compares the resolved values, so the string "1.0" and the float 1.0 are
// two keys and a merge cannot override one with the other. Comparing the text
// made them one, which is the half of defect 69 a MapSlice can represent; a
// map[string]any has lost one of them before this runs.
//
// Only a key mapKeyNodeToValue judged hashable reaches this as a value, so ==
// on the interfaces cannot panic -- the guard is there for a MapSlice a caller
// built by hand.
//
// Two Go values that == tells apart can still be one YAML key, and keyIDOf
// says which: YAML has one integer type, one float type and one timestamp
// type, so int(1) and uint64(1) are one key, float32(0.5) and 0.5 are one,
// and so are one instant in two zones. The parser refuses a mapping that
// writes one such key twice, and a merge or MapSlice.Set has to agree with it.
func sameMapKey(a, b any) bool {
	if !codecutils.HashableKey(a) || !codecutils.HashableKey(b) {
		return reflect.DeepEqual(a, b)
	}
	if a == b {
		return true
	}
	aKind, aName, aNamed := keyIDOf(a)
	if !aNamed {
		return false
	}
	bKind, bName, bNamed := keyIDOf(b)

	return bNamed && aKind == bKind && aName == bName
}

// keyIDOf returns the kind and the name a Go key is compared by in YAML, and
// whether it is a scalar YAML has a kind for.
//
// It is the value side of the identity [ast.ComparedKeyName] gives a key node,
// and gives the same answer for the value a key node decodes to:
// TestAKeyNodeAndItsValueHaveOneIdentity holds the two together over the
// corpus. A string is its text, a Base64 its canonical base64 text, a
// time.Time its instant in UTC, and a number [codecutils.NumberKey]'s name.
func keyIDOf(v any) (token.KeyKind, string, bool) {
	switch k := v.(type) {
	case nil:
		return token.KeyNull, "null", true
	case string:
		return token.KeyString, k, true
	case bool:
		return token.KeyBool, strconv.FormatBool(k), true
	case Base64:
		return token.KeyBinary, k.Canonical(), true
	case time.Time:
		return token.KeyTimestamp, k.UTC().Format(time.RFC3339Nano), true
	}

	return codecutils.NumberKey(v)
}

// comparesByValue reports whether a decoded key can be one YAML key with a key
// a Go map lookup does not find it under: a time.Time in another zone, a
// *big.Int or *big.Float, which == compares by address, or a Base64 written
// across lines.
func comparesByValue(key any) bool {
	switch key.(type) {
	case time.Time, *big.Int, *big.Float, Base64:
		return true
	default:
		return false
	}
}

// sameKeyIn returns the key of m that is one YAML key with key where a Go map
// lookup cannot find it, and whether m holds one.
//
// A scan, and only on a merge: the decoder builds uint64, int64 and float64
// keys that == compares by value, and scans only for the keys comparesByValue
// names.
func sameKeyIn(m map[any]any, key any) (any, bool) {
	if !comparesByValue(key) {
		return nil, false
	}
	for held := range m {
		if sameMapKey(held, key) {
			return held, true
		}
	}

	return nil, false
}
