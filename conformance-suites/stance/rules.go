// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stance

import (
	"slices"
	"strings"
)

// Rule is a requirement a specification states outright and its grammar cannot
// express.
//
// # Why this is not a Stand
//
// A [Tag] normally names something the specification left open, and a [Table]
// says what this parser decided. That is the wrong model for a rule the
// specification settled: YAML says an alias must refer to an anchor that
// already occurred, and a parser reading one that does not is wrong rather than
// differently opinionated. Left in the stance model, a table declaring Accepts
// would score such a parser as conformant, which is the one outcome the corpus
// must not produce.
//
// So a settled rule fixes the outcome before any stance is consulted, and a
// table cannot vote it away. It can decline it -- see [Table.Expect] -- because
// a parser that never resolves aliases genuinely cannot check this, and
// unscored is the honest answer there. What it cannot do is claim a pass.
//
// # Why the grammar cannot express them
//
// Every one of these needs something a production does not have: a table of
// what has been seen, a count, or an identity comparison after resolution. They
// are the fourth row of the taxonomy this whole package exists for -- stated by
// the spec, invisible to the oracle, and therefore carried as a label put on a
// document by whatever constructed it.
type Rule struct {
	// Tag is the property a document carrying this rule's violation exhibits.
	Tag Tag
	// Because says what the specification requires, and where.
	Because string
	// Then is what a conforming parser must do with a document exhibiting it.
	//
	// Only Reject is forced. An Accept here records that a construct is legal,
	// which is not the same as the document being legal: the document may be
	// malformed for some entirely unrelated reason, and the grammar is still
	// the one to say so.
	Then Outcome
}

// Rules is one language's set of them.
//
// The mechanism is here and the entries are not: which rules a specification
// states outside its grammar is exactly the language-specific part.
type Rules []Rule

// Of returns the rule for a tag, and whether there is one.
func (r Rules) Of(tag Tag) (Rule, bool) {
	for _, rule := range r {
		if rule.Tag == tag {
			return rule, true
		}
	}

	return Rule{}, false
}

// Tags lists the tags these rules settle, sorted.
func (r Rules) Tags() []string {
	out := make([]string, 0, len(r))
	for _, rule := range r {
		out = append(out, string(rule.Tag))
	}

	slices.Sort(out)

	return out
}

// Contradicted names the settled rules a table claims to accept, sorted.
//
// Declaring Accepts on a rule the specification settled is not a position, it
// is a mistake -- most likely a tag copied into the wrong half of a table. It
// is reported rather than honored, and reported separately from a gap because
// the two want different fixes: a gap wants a decision, this wants a deletion.
func (r Rules) Contradicted(t Table) []string {
	var out []string

	for _, rule := range r {
		if rule.Then == Reject && t.Stand(rule.Tag) == Accepts {
			out = append(out, string(rule.Tag)+": "+rule.Because)
		}
	}

	slices.Sort(out)

	return out
}

// Describe renders the rules as a list, for a report that has to say what a
// corpus is holding a parser to beyond its grammar.
func (r Rules) Describe() string {
	lines := make([]string, 0, len(r))
	for _, rule := range r {
		lines = append(lines, string(rule.Tag)+" -> "+rule.Then.String()+": "+rule.Because)
	}

	slices.Sort(lines)

	return strings.Join(lines, "\n")
}
