// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package key finds a mapping key that a document has already used.
//
// [Set] holds the keys of every mapping open at one point in a parse, and finds
// a repeat by scanning the innermost mapping's own keys. [Ledger] wraps it with
// the mapping nodes being built, so a repeat is recorded on the mapping that
// holds it as an [ast.DuplicateKey].
//
// Neither names a key. The caller hands in a name and a [token.KeyKind], and
// 3.2.1.1 makes two keys equal when they resolve to the same node -- so the
// kind carries half the identity and the caller settles it.
package key

import (
	"fmt"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/token"
)

// Ledger finds a key a mapping has already used, and notes the repeat on the
// mapping that holds it.
//
// It is reused for the whole parse: a mapping pushes its keys on the way in and
// drops them on the way out, so it grows once to the deepest, widest point of
// the document and allocates nothing after that.
//
// The parser names the keys; the ledger does not. Its mapKeyIdentity and
// builtKeyIdentity read the anchor table, and an alias key is named from what
// its anchor resolved to. The ledger is handed a name and answers whether this
// mapping has used it.
//
// Edit the two halves together. A fix to the naming that never reaches the
// ledger drops a value silently, and the package boundary between them hides
// nothing that used to be visible: the ledger never read parser state. The
// naming half is parser/keys.go.
type Ledger struct {
	// keys records where a key was first written and not the node it came
	// from: a node holds the token it was built from, and a token kept here
	// outlives the entry that carried it, so every key of every open mapping
	// would stay reachable until that mapping closed. A mapping of 5,000 keys
	// held 5,000 tokens spread over the whole document; it now holds 5,000
	// positions of 16 bytes and no token at all.
	keys Set

	// probeBases shadows keys.entries with the base each key was recorded
	// under, for the probe that holds mapKeyRef.base redundant. It is appended
	// to only where probe.Enabled, which is a constant false in a normal build.
	probeBases []int32

	// openMaps holds the mapping node at each level of the descent, innermost
	// last, so a repeated key is recorded on the mapping that holds it as it is
	// read. One entry per mapping open at once, which is the document's nesting
	// and not its width.
	openMaps []openMap
	// builtKeys holds, for each mapping being read, the identity of every key a
	// single token could not name, under the position it was first written at.
	// It stands beside openMaps and is pushed and popped with it.
	builtKeys []map[string]token.Position

	// allowRepeats marks each repeat [ast.DuplicateKey.Allowed]. The parser
	// sets it from its WithAllowDuplicateMapKey option.
	allowRepeats bool

	// orderedMaps holds one key set for each "!!omap" being read, innermost
	// last, with the sequence the tag stands on. An ordered map's keys are
	// spread over one entry each, so every entry's key is recorded there as
	// well, and a repeat across entries is recorded on the sequence.
	orderedMaps []orderedMapScope
	// pending is the "!!omap" whose entry is about to be read, as its index plus
	// one, and 0 for none; pendingIndex is that entry's place in the sequence.
	// The next mapping to open takes both.
	pending      int
	pendingIndex int
	// expectOrderedMap marks the next sequence to open as the one an "!!omap"
	// tag stands on. A mapping opening first clears it, so "!!omap {a: [1]}"
	// makes no ordered map of the "[1]".
	expectOrderedMap bool
}

// ExpectOrderedMap marks the next sequence to open as the one an "!!omap" tag
// stands on. An anchor may stand between the two, as in "!!omap &a [{x: 1}]".
func (l *Ledger) ExpectOrderedMap() { l.expectOrderedMap = true }

// TakeOrderedMap reports whether the sequence opening now is the one an
// "!!omap" tag stands on, and clears the mark.
func (l *Ledger) TakeOrderedMap() bool {
	expected := l.expectOrderedMap
	l.expectOrderedMap = false

	return expected
}

// openMap is one mapping being read: its node, and where it stands in an
// "!!omap" -- the index in orderedMaps of the one it is a direct entry of, or
// -1, and its place in that sequence. Pushed and popped together, so they share
// one slice and one growth.
type openMap struct {
	node       *ast.MappingNode
	entryOf    int
	entryIndex int
}

// orderedMapScope is one "!!omap" being read: the keys its entries have written
// so far, and the sequence a repeat is recorded on.
type orderedMapScope struct {
	keys *Set
	seq  *ast.SequenceNode
}

// OpenOrderedMap starts the key set of the "!!omap" whose sequence seq is, as
// its entries are about to be read, and returns what ends it.
//
// Section 3.2.1.1 holds an ordered map's keys to being unique as it holds a
// mapping's. The keys stand in separate entries, so a repeat across them is
// recorded on the sequence, in [ast.SequenceNode.Duplicates], with the index of
// the entry that repeats the key. Every load reads that one record.
func (l *Ledger) OpenOrderedMap(seq *ast.SequenceNode) func() {
	keys := &Set{}
	keys.UseJSONNames(l.keys.jsonNames)
	l.orderedMaps = append(l.orderedMaps, orderedMapScope{keys: keys, seq: seq})

	return func() {
		l.orderedMaps = l.orderedMaps[:len(l.orderedMaps)-1]
		l.pending = 0
	}
}

// MarkEntry says that the next mapping to open is the entry at index of the
// innermost "!!omap".
func (l *Ledger) MarkEntry(index int) { l.pending, l.pendingIndex = len(l.orderedMaps), index }

// UnmarkEntry forgets a mark no mapping took: an entry that is not a mapping,
// or a sequence opening inside one.
func (l *Ledger) UnmarkEntry() { l.pending = 0 }

// RecordEntry records text as the key of the "!!omap" entry MarkEntry marked,
// for an entry that opened no mapping of its own: an alias naming a one-entry
// mapping. The parser names the key from what the anchor recorded.
func (l *Ledger) RecordEntry(text string, kind token.KeyKind, pos token.Position) {
	if l.pending > 0 {
		l.recordInOrderedMap(l.orderedMaps[l.pending-1], text, kind, pos, l.pendingIndex)
	}
}

// recordInOrderedMap records a key among the entries of scope, and a repeat on
// its sequence.
func (l *Ledger) recordInOrderedMap(scope orderedMapScope, text string, kind token.KeyKind, pos token.Position, index int) {
	first, jsonOnly, defined := scope.keys.Record(0, text, kind, pos)
	if !defined || scope.seq == nil {
		return
	}
	scope.seq.Duplicates = append(scope.seq.Duplicates, ast.DuplicateKey{
		Name: text, At: pos, FirstAt: first, JSONNameOnly: jsonOnly, Index: index, Allowed: l.allowRepeats,
	})
}

// AllowRepeats marks every repeat recorded from now on as allowed, so a load
// keeps one of the entries instead of refusing the document.
func (l *Ledger) AllowRepeats(on bool) { l.allowRepeats = on }

// Base returns the index the keys of the mapping opening now start at.
func (l *Ledger) Base() int { return l.keys.Base() }

// Holds reports whether the mapping starting at base has written the name text itself.
// See [Set.Holds] for which of the two key questions it answers.
func (l *Ledger) Holds(base int, text string) bool { return l.keys.Holds(base, text) }

// UseJSONNames compares a key under the name JSON gives it as well as under the
// node it resolves to. The parser sets it from its WithJSONCompatible option.
func (l *Ledger) UseJSONNames(on bool) { l.keys.UseJSONNames(on) }

// InMapping reports whether a mapping is being read.
func (l *Ledger) InMapping() bool { return len(l.builtKeys) > 0 }

// Record records that the mapping starting at base uses text as a key, written
// at pos.
//
// It returns where text was first written, whether the two keys are one JSON
// member name written as two YAML nodes, and whether the mapping had already
// used the name.
func (l *Ledger) Record(base int, text string, kind token.KeyKind, pos token.Position) (token.Position, bool, bool) {
	if probe.Enabled {
		l.checkStackTail(base)
	}

	return l.keys.Record(base, text, kind, pos)
}

// RecordOnce records text among the keys of the mapping starting at base, and
// notes a repeat on the mapping being read.
func (l *Ledger) RecordOnce(base int, text string, kind token.KeyKind, pos token.Position) {
	first, jsonOnly, defined := l.Record(base, text, kind, pos)
	if defined {
		l.noteDuplicate(ast.DuplicateKey{Name: text, At: pos, FirstAt: first, JSONNameOnly: jsonOnly})

		return
	}

	n := len(l.openMaps)
	if n == 0 || l.openMaps[n-1].entryOf < 0 {
		return
	}
	top := l.openMaps[n-1]
	l.recordInOrderedMap(l.orderedMaps[top.entryOf], text, kind, pos, top.entryIndex)
}

// RecordBuilt records identity among the built keys of the mapping being read,
// noting a repeat under the name display gives it.
//
// Call it only where InMapping holds: it writes to the innermost mapping's map.
func (l *Ledger) RecordBuilt(identity, display string, pos token.Position) {
	top := len(l.builtKeys) - 1
	if first, repeated := l.builtKeys[top][identity]; repeated {
		l.noteDuplicate(ast.DuplicateKey{Name: display, At: pos, FirstAt: first})

		return
	}

	if l.builtKeys[top] == nil {
		l.builtKeys[top] = make(map[string]token.Position, 4)
	}
	l.builtKeys[top][identity] = pos
}

// noteDuplicate records dup on the mapping being read, which is the innermost
// one open.
func (l *Ledger) noteDuplicate(dup ast.DuplicateKey) {
	dup.Allowed = l.allowRepeats
	if n := len(l.openMaps); n > 0 {
		l.openMaps[n-1].node.Duplicates = append(l.openMaps[n-1].node.Duplicates, dup)
	}
}

// checkStackTail records whether the keys above base all belong to the
// mapping recording now.
//
// Mappings nest, so a mapping records keys only while it is the innermost one
// open: an outer mapping's next key waits for the inner one to close. If that
// holds, the keys above base are exactly one mapping's, and a duplicate can be
// found by scanning that tail instead of hashing base into an index shared by
// every open mapping.
//
// probeBases shadows the stack with the base each key was recorded under, which
// the entries themselves stopped carrying once the scan made it redundant --
// which is the very thing under test, so the probe keeps its own copy rather
// than reading the answer off the state it is checking.
//
// Nothing may raise this: a disagreement means an outer mapping recorded a key
// over an inner one's, and Ledger.Close would then drop a key the outer still
// owns. Set.Record reads the tail on that promise.
func (l *Ledger) checkStackTail(base int) {
	// A repeated key records no entry, so the shadow can stand one ahead of the
	// stack it shadows. Trim it back before reading either.
	l.probeBases = l.probeBases[:min(len(l.probeBases), len(l.keys.entries))]

	held := true
	for i := base; i < len(l.probeBases); i++ {
		if int(l.probeBases[i]) == base {
			continue
		}
		at, was, text := i, l.probeBases[i], l.keys.entries[i].text
		probe.Check("mapkey.stackTailIsOneMapping", false, func() string {
			return fmt.Sprintf("key %q at %d was recorded under mapping %d, recording now for %d",
				text, at, was, base)
		})
		held = false

		break
	}
	if held {
		probe.Check("mapkey.stackTailIsOneMapping", true, nil)
	}

	l.probeBases = append(l.probeBases, int32(base))
}

// Close drops the keys of the mapping that started at base.
func (l *Ledger) Close(base int) {
	if probe.Enabled {
		l.probeBases = l.probeBases[:min(base, len(l.probeBases))]
	}
	l.keys.Close(base)
}

// Open records the mapping being read, and returns what takes it off.
func (l *Ledger) Open(node *ast.MappingNode) func() {
	l.openMaps = append(l.openMaps, openMap{node: node, entryOf: l.pending - 1, entryIndex: l.pendingIndex})
	l.builtKeys = append(l.builtKeys, nil)
	l.pending = 0
	l.expectOrderedMap = false

	return func() {
		l.openMaps = l.openMaps[:len(l.openMaps)-1]
		l.builtKeys = l.builtKeys[:len(l.builtKeys)-1]
	}
}

// Reset forgets every key and every open mapping, and keeps the room the Ledger has grown.
//
// A parse that stops on an error leaves mappings open, so call Reset before the Ledger reads another stream.
func (l *Ledger) Reset() {
	l.keys.Reset()
	clear(l.openMaps[:cap(l.openMaps)])
	clear(l.builtKeys[:cap(l.builtKeys)])
	l.probeBases = l.probeBases[:0]
	l.openMaps = l.openMaps[:0]
	l.builtKeys = l.builtKeys[:0]
	clear(l.orderedMaps[:cap(l.orderedMaps)])
	l.orderedMaps = l.orderedMaps[:0]
	l.pending = 0
	l.expectOrderedMap = false
}
