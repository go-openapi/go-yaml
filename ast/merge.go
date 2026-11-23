// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "fmt"

// Verdict classifies a mapping entry whose key is "<<".
//
// Every reader must tell the three verdicts apart, and tell them apart the same
// way. Thirteen places asked [MapKeyNode.IsMergeKey]
// and each drew its own conclusion from a bare yes: the decoder folded, the
// converter deferred to the mapping's close, the typed walk gave up on the
// document, and none of them agreed about a "<<" whose value is not a mapping.
type Verdict int

const (
	// NotAMerge marks an entry whose key is an ordinary one.
	//
	// Whether a "<<" is a merge key is settled before a node exists. The
	// scanner recognizes the characters and types them, unconditionally,
	// because it cannot see a "!!merge" standing in front of them -- a "%TAG"
	// line can repoint the secondary handle and the scanner never reads one.
	// The parser holds the directives and the document's version, so the parser
	// chooses the node: a MergeKeyNode where the merge resolves, an ordinary key
	// where it does not. [MergeOf] reads the answer from the node's type.
	NotAMerge Verdict = iota
	// Folds marks an entry that brings in the mappings of [MergeEntry.Sources], earliest
	// first, and contributes no key of its own.
	Folds
	// Refused marks an entry whose value is not a mapping and not a sequence of
	// mappings, so the merge type has nothing to fold. [MergeEntry.Offender] holds the value.
	Refused
)

// MergeEntry records what one "<<" entry contributes to the mapping holding it.
//
// It is not named Merge: [Merge] already folds one tree into another, which is
// a different operation on the same word.
type MergeEntry struct {
	// Verdict is one of [NotAMerge], [Folds] and [Refused].
	Verdict Verdict
	// Sources are the mappings to fold, in the order they win: an earlier
	// mapping of a "<<" sequence beats a later one. It is nil unless Verdict is
	// [Folds].
	Sources []MapNode
	// Offender is the node a "<<" cannot fold, and is nil unless Verdict is
	// [Refused].
	//
	// Offender holds the node instead of an error because each caller words the
	// failure differently -- the parser reports a position, the decoder reports
	// a Go type it cannot fill -- and errors imports ast, so ast cannot import
	// errors.
	Offender Node
}

// MergeOf reads what a mapping entry contributes to the mapping holding it.
//
// It accepts every entry, not only a "<<", and returns [NotAMerge] for an
// ordinary key. A caller calls it once per entry and switches on the verdict,
// instead of testing the key and then working out what the test implied.
//
// It takes no schema. The parser settles which spellings of "<<" merge, by
// building a [MergeKeyNode] or an ordinary key. Checking again here would
// repeat that choice with less context, since a tag handle is
// expanded by the parser and a node does not carry the version it was read
// under.
//
// MergeOf does not apply precedence either. The merge type gives a mapping's own
// keys precedence over the ones a "<<" brings in, which is a rule about the
// mapping and not about the entry, so a caller reads its own entries first and
// writes a merged key only where none stands. [MergeEntry.Sources] is ordered
// so that reading them in turn and keeping the first writer gives the
// earlier-wins rule for free.
func MergeOf(entry *MappingValueNode) MergeEntry {
	if entry == nil || entry.Key == nil || !entry.Key.IsMergeKey() {
		return MergeEntry{Verdict: NotAMerge}
	}

	sources, offender := mergeSources(entry.Value)
	if offender != nil {
		return MergeEntry{Verdict: Refused, Offender: offender}
	}

	return MergeEntry{Verdict: Folds, Sources: sources}
}

// mergeSources are the mappings a "<<" names, earliest first.
//
// The value is a mapping, an alias or an anchor naming one, or a sequence of
// those. mergeSources returns anything else as the offender, as
// go.yaml.in/yaml/v3 v3.0.5 does -- "map merge requires map or sequence of
// maps as the value". libfyaml 1.0.0b1 is inconsistent: in 1.1 mode it accepts
// an empty merge where the mapping ends and rejects the same one where the
// mapping continues.
func mergeSources(value Node) (sources []MapNode, offender Node) {
	switch n := Unwrap(value).(type) {
	case MapNode:
		return []MapNode{n}, nil
	case *SequenceNode:
		out := make([]MapNode, 0, len(n.Values))
		for _, item := range n.Values {
			m, ok := Unwrap(item).(MapNode)
			if !ok {
				return nil, item
			}
			out = append(out, m)
		}

		return out, nil
	default:
		return nil, value
	}
}

type InvalidMergeTypeError struct {
	dst Node
	src Node
}

func (e *InvalidMergeTypeError) Error() string {
	return fmt.Sprintf("cannot merge %s into %s", e.src.Type(), e.dst.Type())
}

// Merge merge document, map, sequence node.
func Merge(dst Node, src Node) error {
	if doc, ok := src.(*DocumentNode); ok {
		src = doc.Body
	}
	err := &InvalidMergeTypeError{dst: dst, src: src}
	switch dst.Type() {
	case DocumentType:
		node, _ := dst.(*DocumentNode)
		return Merge(node.Body, src)
	case MappingType:
		node, _ := dst.(*MappingNode)
		target, ok := src.(*MappingNode)
		if !ok {
			return err
		}
		node.Merge(target)
		return nil
	case SequenceType:
		node, _ := dst.(*SequenceNode)
		target, ok := src.(*SequenceNode)
		if !ok {
			return err
		}
		node.Merge(target)
		return nil
	default:
		// nothing to do
	}
	return err
}
