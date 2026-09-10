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
	// read. One pointer per mapping open at once, which is the document's
	// nesting and not its width.
	openMaps []*ast.MappingNode
	// builtKeys holds, for each mapping being read, the identity of every key a
	// single token could not name, under the position it was first written at.
	// It stands beside openMaps and is pushed and popped with it.
	builtKeys []map[string]token.Position
}

// Base returns the index the keys of the mapping opening now start at.
func (l *Ledger) Base() int { return l.keys.Base() }

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
	if !defined {
		return
	}

	l.noteDuplicate(ast.DuplicateKey{Name: text, At: pos, FirstAt: first, JSONNameOnly: jsonOnly})
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
	if n := len(l.openMaps); n > 0 {
		l.openMaps[n-1].Duplicates = append(l.openMaps[n-1].Duplicates, dup)
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
	l.openMaps = append(l.openMaps, node)
	l.builtKeys = append(l.builtKeys, nil)

	return func() {
		l.openMaps = l.openMaps[:len(l.openMaps)-1]
		l.builtKeys = l.builtKeys[:len(l.builtKeys)-1]
	}
}
