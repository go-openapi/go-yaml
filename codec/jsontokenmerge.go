// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"slices"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// collectMerge reads what a "<<" names, without handing any of it over.
//
// The value is a mapping, an alias naming one, or a sequence of those -- the
// reading [ast.MergeOf] gives, applied here one node at a time because a walk
// hands a mapping's entries over rather than the entry. What it brings in goes
// over at the end of the mapping, where the keys the mapping writes itself are
// known: an own key beats a merged one whatever their order in the document.
func (t *jsonTokener) collectMerge(node ast.Node, at parser.Cursor) error {
	frame := t.frame()

	switch n := node.(type) {
	case *ast.SequenceNode:
		if frame.mergeSeq >= 0 {
			// A sequence inside the sequence of merge sources. "<<" takes
			// mappings, and the entries of both were handed over as one.
			t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))

			return t.halted()
		}
		// "<<: [*a, *b]" merges each in turn, earliest first, so the flag
		// stands until the sequence closes.
		frame.mergeSeq = at.Depth()

		return nil
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return t.halted()
		}
		if frame.mergeSeq < 0 {
			frame.mergeValue = false
		}
		t.collectRun(func() {
			name := anchorName(n.Value)
			t.expanding = append(t.expanding, name)
			t.emitTree(target, nodeAt(n))
			t.expanding = t.expanding[:len(t.expanding)-1]
		})

		return t.skipped()
	case *ast.MappingNode:
		// "<<: {a: 1}" merges a mapping written out. The walk hands its entries
		// over one at a time and the tree holds none of them, so it is read by
		// collecting what it hands over rather than from the node.
		if frame.mergeSeq < 0 {
			frame.mergeValue = false
		}
		t.buffer = new([]JSONToken)
		t.bufDepth = at.Depth()
		t.bufOwner = len(t.maps) - 1
		t.open(JSONObjectStart, nodeAt(n))
		t.pushMap(nil)

		return nil
	default:
		t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

		return t.halted()
	}
}

// failMerge refuses what a "<<" names, or the key the mapping repeats where it
// repeats one.
func (t *jsonTokener) failMerge(frame *tokenMapFrame, err error) {
	if frame.node != nil {
		if repeat := refuseDuplicateKeys(frame.node); repeat != nil {
			err = repeat
		}
	}
	t.fail(err)
}

// collectRun runs write with everything it hands over going to a run of its
// own, which is then kept for the mapping the "<<" stands in.
func (t *jsonTokener) collectRun(write func()) {
	owner := len(t.maps) - 1
	held := t.buffer
	t.buffer = new([]JSONToken)

	write()

	run := *t.buffer
	t.buffer = held
	if owner >= 0 && !t.stopped {
		t.maps[owner].merged = append(t.maps[owner].merged, run)
	}
}

// closeMapping hands over what the mapping's "<<" entries bring in, and closes
// it.
//
// A merged key goes in only where the mapping writes none itself, and an
// earlier merge beats a later one, so the runs are read in the order they were
// collected and the first writer of a name wins. JSON has no way to name a
// member twice.
func (t *jsonTokener) closeMapping(at parser.Cursor, end *token.Token) {
	if f := &t.maps[len(t.maps)-1]; f.hasPending {
		// A key whose value never began. Every entry has one, a null at least,
		// so this only guards the order the tokens go over in.
		f.hasPending = false
		f.keys = append(f.keys, f.pending.Value)
		t.emit(f.pending)
	}
	frame := t.maps[len(t.maps)-1]
	t.maps = t.maps[:len(t.maps)-1]

	for _, run := range frame.merged {
		for _, member := range mergedMembers(run) {
			if slices.Contains(frame.keys, member.key) {
				continue
			}
			frame.keys = append(frame.keys, member.key)
			for _, tok := range run[member.from:member.to] {
				t.emit(tok)
			}
		}
	}

	t.close(JSONObjectEnd, t.closeAt(end))

	if t.buffer != nil && at.Depth() == t.bufDepth {
		run := *t.buffer
		t.buffer = nil
		if t.bufOwner >= 0 && t.bufOwner < len(t.maps) && !t.stopped {
			t.maps[t.bufOwner].merged = append(t.maps[t.bufOwner].merged, run)
		}
	}
}

// jsonTokenMember is one member of a collected object run: its name, and where
// the key and the value stand in the run.
type jsonTokenMember struct {
	key      string
	from, to int
}

// mergedMembers reads the members of a run holding one object.
//
// The run opens with a [JSONObjectStart] and closes with the matching
// [JSONObjectEnd]; only the members between them at that level are read, so a
// key inside a nested collection is not one of them.
func mergedMembers(run []JSONToken) []jsonTokenMember {
	if len(run) < 2 || run[0].Kind != JSONObjectStart {
		return nil
	}

	var (
		members []jsonTokenMember
		open    int
	)
	for i := 1; i < len(run)-1; i++ {
		if open == 0 && run[i].Kind == JSONKey {
			member := jsonTokenMember{key: run[i].Value, from: i}
			i++
			member.to = valueEnd(run, i)
			i = member.to - 1
			members = append(members, member)

			continue
		}
		switch run[i].Kind {
		case JSONObjectStart, JSONArrayStart:
			open++
		case JSONObjectEnd, JSONArrayEnd:
			open--
		}
	}

	return members
}

// valueEnd is the index just past the value beginning at from.
func valueEnd(run []JSONToken, from int) int {
	var open int
	for i := from; i < len(run); i++ {
		switch run[i].Kind {
		case JSONObjectStart, JSONArrayStart:
			open++
		case JSONObjectEnd, JSONArrayEnd:
			open--
			if open == 0 {
				return i + 1
			}
		default:
			if open == 0 {
				return i + 1
			}
		}
	}

	return len(run)
}
