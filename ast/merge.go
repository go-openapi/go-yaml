// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

// Verdict says what a mapping entry whose key is "<<" means.
//
// The three answers are what a reader has to tell apart, and every reader has
// to tell them apart the same way. Thirteen places asked [MapKeyNode.IsMergeKey]
// and each drew its own conclusion from a bare yes: the decoder folded, the
// converter deferred to the mapping's close, the typed walk gave up on the
// document, and none of them agreed about a "<<" whose value is not a mapping.
type Verdict int

const (
	// NotAMerge says the entry's key is an ordinary one.
	//
	// Whether a "<<" is a merge key is settled before a node exists. The
	// scanner recognizes the characters and types them, unconditionally,
	// because it cannot see a "!!merge" standing in front of them -- a "%TAG"
	// line can repoint the secondary handle and the scanner never reads one.
	// The parser holds the directives and the document's version, so it decides
	// which node to build: a MergeKeyNode where the merge resolves, an ordinary
	// key where it does not. By the time this is asked the answer is the node's
	// own type.
	NotAMerge Verdict = iota
	// Folds says the entry brings in the mappings of [Merge.Sources], earliest
	// first, and contributes no key of its own.
	Folds
	// Refused says the value is not a mapping and not a sequence of mappings,
	// so the merge type has nothing to fold. [Merge.Err] names it.
	Refused
)

// MergeEntry is what one "<<" entry contributes to the mapping holding it.
//
// It is not named Merge: [Merge] already folds one tree into another, which is
// a different operation on the same word.
type MergeEntry struct {
	// Verdict says which of the three answers this entry has.
	Verdict Verdict
	// Sources are the mappings to fold, in the order they win: an earlier
	// mapping of a "<<" sequence beats a later one. It is nil unless Verdict is
	// [Folds].
	Sources []MapNode
	// Offender is the node a "<<" cannot fold, and is nil unless Verdict is
	// [Refused].
	//
	// The node rather than an error: what to say about it differs by caller --
	// the parser reports a position, the decoder reports a Go type it cannot
	// fill -- and errors imports ast, so ast cannot import errors.
	Offender Node
}

// MergeOf reads what a mapping entry contributes to the mapping holding it.
//
// It answers for every entry, not only for a "<<": an ordinary key is
// [NotAMerge], so a caller asks once per entry and switches on the verdict
// rather than testing the key and then working out what the test implied.
//
// It takes no schema. Which spellings of "<<" merge is the parser's to settle,
// and it settles it by building a [MergeKeyNode] or an ordinary key; asking
// again here would re-decide it with less context, since a tag handle is
// expanded by the parser and a node does not carry the version it was read
// under.
//
// Precedence is not decided here either. The merge type gives a mapping's own
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
// those. Anything else has nothing to fold and is refused, which is the reading
// go.yaml.in/yaml/v3 v3.0.5 gives -- "map merge requires map or sequence of
// maps as the value". libfyaml 1.0.0b1 is no guide: asked for 1.1 it accepts an
// empty merge where the mapping ends and refuses the same one where the mapping
// continues.
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
