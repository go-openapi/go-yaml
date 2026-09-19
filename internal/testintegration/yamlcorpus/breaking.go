// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// Breaking a rule the grammar cannot see, on purpose and on the value.
//
// # The mutation this resurrects, and why it was dead
//
// yamlgen once had a mutation that left an alias pointing at no anchor. Its
// note records the result: over twenty thousand draws it produced not one
// document the recognizer refused, so it was deleted as dead weight. That was
// the right reading of the measurement and the wrong conclusion. The documents
// were not uninteresting -- they were *invalid in a way the oracle could not
// see*, which is the most interesting thing a document can be, and the corpus
// had no way to say so.
//
// # Why this works on the value and not on the text
//
// Doing it by editing bytes brings the labeling problem with it. Rewriting
// "*a1" to "*zz" breaks a reference only if those bytes were an alias, and
// inside a quoted scalar or a block scalar they are ordinary characters. A
// mutation that guessed would put a settled rejection on documents that do not
// deserve one -- the same trap that made a tagger the wrong answer for the
// generic mutations.
//
// So these work on the [yamlgen.Value] before it is written. An alias renamed
// in the tree is an alias, because the tree is what an alias means. The
// resulting document is invalid by construction, exactly as the enumerated
// families are, and carries the tag to say which rule it breaks.
//
// # What they are worth
//
// They are the only generated documents that break a rule beyond the grammar.
// Everything else the generator draws is valid, and everything the byte
// mutations produce is either invalid in a way the grammar sees or valid in a
// way nobody can label. These fill the gap between the two.

// broken is one document made invalid on purpose, with the rule it breaks.
type broken struct {
	// How names the break, for the case name.
	How string
	// Value is the rewritten tree.
	Value yamlgen.Value
	// Tags say which rule it breaks, put on by construction.
	Tags []stance.Tag
}

// breakRules rewrites a value into every rule-breaking variant it admits.
//
// Returns nothing for a value with no aliases and no mapping of two entries,
// which is most small ones. That is expected rather than a failure: a document
// with nothing to break cannot be broken.
func breakRules(v yamlgen.Value) []broken {
	var out []broken

	if renamed, ok := renameAnAlias(v); ok {
		out = append(out, broken{
			How:   "alias renamed to nothing",
			Value: renamed,
			Tags:  []stance.Tag{TagAliasUndefined},
		})
	}

	if duplicated, ok := duplicateAKey(v); ok {
		out = append(out, broken{
			How:   "a key repeated",
			Value: duplicated,
			Tags:  []stance.Tag{TagDuplicateKey},
		})
	}

	// TagDuplicateKey and not TagDuplicateAfterResolution: both keys are
	// written the same way, so a consumer comparing spellings catches it
	// without resolving anything. A repeat written some *other* way -- tagged,
	// anchored, aliased, or spelled as YAML 1.1 resolves it -- is the harder
	// half and needs an emitter that can write a key's properties, which
	// panics today. See duplicates.go.
	if repeated, ok := repeatAKey(v); ok {
		out = append(out, broken{
			How:   "a duplicate entry appended",
			Value: repeated,
			Tags:  []stance.Tag{TagDuplicateKey},
		})
	}

	if repeated, ok := repeatACollectionKey(v); ok {
		out = append(out, broken{
			How:   "a collection key written twice",
			Value: repeated,
			Tags:  []stance.Tag{TagDuplicateKey},
		})
	}

	return out
}

// renameAnAlias points the first alias at a name nothing anchors.
//
// The name is one the emitter never writes: it numbers its anchors a0, a1 and
// so on, so "unanchored" collides with nothing whatever the tree holds.
func renameAnAlias(v yamlgen.Value) (yamlgen.Value, bool) {
	var done bool

	var walk func(yamlgen.Value) yamlgen.Value

	walk = func(n yamlgen.Value) yamlgen.Value {
		if done {
			return n
		}

		switch t := n.(type) {
		case yamlgen.Alias:
			done = true

			return yamlgen.Alias{Name: "unanchored", V: t.V}
		case yamlgen.Anchored:
			return yamlgen.Anchored{Name: t.Name, V: walk(t.V)}
		case yamlgen.Seq:
			items := make([]yamlgen.Value, 0, len(t.Items))
			for _, item := range t.Items {
				items = append(items, walk(item))
			}

			return yamlgen.Seq{Items: items}
		case yamlgen.Map:
			pairs := make([]yamlgen.Pair, 0, len(t.Pairs))
			for _, p := range t.Pairs {
				pairs = append(pairs, yamlgen.Pair{Key: p.Key, Val: walk(p.Val)})
			}

			return yamlgen.Map{Pairs: pairs}
		default:
			return n
		}
	}

	out := walk(v)

	return out, done
}

// holdsAMergeKey reports whether a mapping's first entry is a "<<".
//
// duplicateAKey skips such a mapping, and not to keep the corpus green.
// Duplicating a "<<" writes a document whose break is a different rule than the
// tag claims: the entry says TagDuplicateKey, and what comes out is a merge key
// alone in flow, which this library reads where it refuses `{a: 1, a}`. That
// shape is held by
// yamlgen_test.TestDefectAMergeKeyAloneInFlowEscapesTheDuplicateCheck rather
// than by a corpus document whose label would be wrong.
func holdsAMergeKey(m yamlgen.Map) bool {
	for _, p := range m.Pairs {
		if _, isMerge := p.Key.(yamlgen.MergeKey); isMerge {
			return true
		}
	}

	return false
}

// duplicateAKey gives the second entry of the first mapping it finds the first
// entry's key.
//
// Adjacent and identical, so the two are certainly in the same mapping -- which
// a mutation working on the text could not have promised, since a line that
// looks like a mapping entry may be the content of a block scalar.
func duplicateAKey(v yamlgen.Value) (yamlgen.Value, bool) {
	var done bool

	var walk func(yamlgen.Value) yamlgen.Value

	walk = func(n yamlgen.Value) yamlgen.Value {
		if done {
			return n
		}

		switch t := n.(type) {
		case yamlgen.Anchored:
			return yamlgen.Anchored{Name: t.Name, V: walk(t.V)}
		case yamlgen.Seq:
			items := make([]yamlgen.Value, 0, len(t.Items))
			for _, item := range t.Items {
				items = append(items, walk(item))
			}

			return yamlgen.Seq{Items: items}
		case yamlgen.Map:
			if len(t.Pairs) >= 2 && !done && !holdsAMergeKey(t) {
				done = true

				pairs := make([]yamlgen.Pair, len(t.Pairs))
				copy(pairs, t.Pairs)
				pairs[1].Key = pairs[0].Key

				return yamlgen.Map{Pairs: pairs}
			}

			pairs := make([]yamlgen.Pair, 0, len(t.Pairs))
			for _, p := range t.Pairs {
				pairs = append(pairs, yamlgen.Pair{Key: p.Key, Val: walk(p.Val)})
			}

			return yamlgen.Map{Pairs: pairs}
		default:
			return n
		}
	}

	out := walk(v)

	return out, done
}
