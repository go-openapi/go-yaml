// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// Coverage records which productions a recognition entered, and in which
// context.
//
// # Why this is not the compiler's coverage
//
// The recognizer is a closure interpreter: every production is a slot holding
// an expr built from shared combinators, so one copy of choice's machine code
// serves around a hundred distinct choices. Go's edge instrumentation saturates
// on it almost at once and can offer no gradient at all toward a production
// nothing has reached. Exploring the recognizer's *code* is not exploring the
// *grammar*; the code has stopped telling them apart.
//
// So production coverage is measured semantically, at [invoke], which is the
// single funnel every production entry passes through.
//
// # Production by context
//
// The bucket is a production paired with the context it was entered in, not the
// production alone. The same rule genuinely decides differently in block-in and
// in flow-key, and a vector that could not tell those apart would call a
// grammar covered while a whole half of it had never run.
//
// The indentation parameters n and m are in it too, reduced to nine classes
// each by [indentBin], because they are unbounded and cannot be in it as
// themselves. That is what keeps two documents differing only in how far they
// are indented from looking identical to the minimizer -- see [indentBin] for
// why that mattered enough to pay four dimensions for.
//
// The two halves of the bucket are not used for the same thing, and the
// difference is worth holding on to. Progress is measured over production by
// context, because that is the part with a denominator [Reach] can compute; a
// document's route is fingerprinted over all four, because that is the part
// that decides whether two documents are the same test.
//
// # Attempts and successes are different questions
//
// invoke fires on backtracked attempts too. "The grammar considered this rule
// here" and "this rule matched here" are both worth knowing and are not the
// same thing, so they get separate counters rather than a shared one.
//
// # Indices are not portable
//
// A bucket index is a production id, and ids are assigned per compiled
// [Grammar]. They are stable across processes -- productions are numbered in
// name order for exactly this reason -- but they are not stable across grammar
// *revisions*. Persist documents, never vectors.
type Coverage struct {
	g         *Grammar
	attempts  []uint32
	successes []uint32
	// touched is every index this vector has entered, in first-entry order.
	//
	// The vector has around 120,000 slots for YAML and a document enters a few
	// hundred, so every operation that would otherwise walk it -- resetting,
	// merging, comparing, fingerprinting -- walks this instead. Without it the
	// binned vector would be about eighty times the work per document, which is
	// the whole reason the parameters could not simply be added.
	touched []int32
}

// NewCoverage returns a vector sized for this grammar, with every bucket empty.
func (g *Grammar) NewCoverage() *Coverage {
	n := len(g.slots) * numContexts * numIndentBins * numIndentBins

	return &Coverage{
		g:         g,
		attempts:  make([]uint32, n),
		successes: make([]uint32, n),
	}
}

// Reset empties every bucket, keeping the allocation, so that a builder asking
// about one document after another does not allocate per document.
func (c *Coverage) Reset() {
	for _, at := range c.touched {
		c.attempts[at] = 0
		c.successes[at] = 0
	}

	c.touched = c.touched[:0]
}

// Buckets is how many production-by-context pairs the vector has room for.
//
// This is not a denominator. The vector is indexed rather than mapped because
// indexing is cheap, so it holds a slot for every production in every context
// and most of those combinations do not exist -- 605 of YAML's 1,477 can be
// entered. Score against [Reach] instead; see [Coverage.Against].
func (c *Coverage) Buckets() int { return len(c.attempts) }

// bucketOf packs a production, the context it was entered in, and the classes
// of its two indentation parameters into one index.
func bucketOf(id int32, e env) int {
	at := int(id)*numContexts + int(e.c)
	at = at*numIndentBins + indentBin(e.n)

	return at*numIndentBins + indentBin(e.m)
}

// Reached is how many buckets have been entered at least once.
func (c *Coverage) Reached() int { return len(c.touched) }

// Matched is how many buckets have been entered and matched at least once.
//
// The gap between this and [Coverage.Reached] is the part of the grammar that
// has been *considered* but never satisfied, which is a different kind of hole
// and often the more interesting one.
func (c *Coverage) Matched() int {
	var n int

	for _, at := range c.touched {
		if c.successes[at] > 0 {
			n++
		}
	}

	return n
}

// Merge folds another vector into this one. Both must come from the same
// grammar.
func (c *Coverage) Merge(o *Coverage) {
	c.mustAgree(o)

	for _, at := range o.touched {
		if c.attempts[at] == 0 && c.successes[at] == 0 {
			c.touched = append(c.touched, at)
		}

		c.attempts[at] += o.attempts[at]
		c.successes[at] += o.successes[at]
	}
}

// AddsTo reports whether this vector reaches a bucket that seen does not.
//
// This is the greedy minimizer's whole decision: keep a candidate exactly when
// it takes the corpus somewhere the corpus has not been. It deliberately says
// nothing about how *often* a bucket was reached, because a document that
// enters a production a thousand times is not more valuable than one that
// enters it once -- and selecting on counts would quietly prefer large
// documents, which is the opposite of what a corpus wants.
func (c *Coverage) AddsTo(seen *Coverage) bool {
	c.mustAgree(seen)

	for _, at := range c.touched {
		if seen.attempts[at] == 0 {
			return true
		}

		if c.successes[at] > 0 && seen.successes[at] == 0 {
			return true
		}
	}

	return false
}

// Unreached names the productions no context has ever entered, in name order.
//
// A production here is one nothing in the corpus exercises at all, which is the
// strongest kind of gap: not a rule reached in too few contexts, but a rule the
// generator has never once produced a document for.
//
// The productions that nothing in the *grammar* refers to are left out. They
// can never be entered from any start symbol, so listing them would mean
// reporting nineteen gaps that no corpus can ever close and burying the real
// ones underneath -- see [Grammar.Unreferenced].
func (c *Coverage) Unreached() []string {
	var out []string

	for name, s := range c.g.slots {
		if !c.enteredAnywhere(s.id) && !slices.Contains(c.g.unreferenced, name) {
			out = append(out, name)
		}
	}

	slices.Sort(out)

	return out
}

// Reachable is how many productions any document could enter.
//
// It is the rule count less the productions that nothing refers to *and*
// nothing entered. That second condition is what tells a start symbol from a
// decorative rule: both are unreferenced, since nothing in a grammar refers to
// its root, and the difference is that a recognition begins at one of them.
//
// It is a production denominator, and it is measured rather than derived: a
// production counts as live once something has entered it, so a rule nothing
// has reached yet is indistinguishable from one nothing can. [Reach] answers
// the same question statically, per context, and without needing a corpus to
// have run first. Prefer it; this remains for the one-line summary, where a
// production count is what is wanted and a start symbol is not to hand.
func (c *Coverage) Reachable() int {
	dead := 0

	for _, name := range c.g.unreferenced {
		if !c.enteredAnywhere(c.g.slots[name].id) {
			dead++
		}
	}

	return len(c.g.slots) - dead
}

// Considered names the productions that were entered somewhere and never
// matched anywhere.
//
// These are the rules the grammar keeps trying and the corpus never satisfies.
// A rule that is always attempted and never matched is either genuinely
// unreachable from the start symbol, or names a shape nothing generates -- and
// the two look identical from here, which is why this reports rather than
// fails.
func (c *Coverage) Considered() []string {
	var out []string

	for name, s := range c.g.slots {
		if c.enteredAnywhere(s.id) && !c.matchedAnywhere(s.id) {
			out = append(out, name)
		}
	}

	slices.Sort(out)

	return out
}

// Entered reports how often a production was entered and matched in one
// context, naming the context the way the grammar does.
func (c *Coverage) Entered(rule, ctx string) (attempts, matches uint32) {
	s := c.g.slotFor(rule)
	code, ok := contextCode[ctx]

	if !ok {
		panic(fmt.Sprintf("grammar: no context named %q", ctx))
	}

	// Folded over the indentation classes: the caller asked about a production
	// in a context, and answering with one column's worth would be answering a
	// question nobody asked.
	from := (int(s.id)*numContexts + int(code)) * numIndentBins * numIndentBins

	for _, at := range c.attempts[from : from+numIndentBins*numIndentBins] {
		attempts += at
	}

	for _, at := range c.successes[from : from+numIndentBins*numIndentBins] {
		matches += at
	}

	return attempts, matches
}

// String summarizes what has been reached, for a test that wants one line.
//
// Entered and matched are reported apart because they behave differently, and
// the difference is the finding: entry saturates almost at once -- both the
// YAML and the JSON official suites enter every reachable production -- while
// matching does not. A corpus measured on entry alone would report itself
// finished while a dozen rules had never once been satisfied.
func (c *Coverage) String() string {
	var entered, matched int

	for _, s := range c.g.slots {
		if c.enteredAnywhere(s.id) {
			entered++
		}

		if c.matchedAnywhere(s.id) {
			matched++
		}
	}

	return fmt.Sprintf("%d/%d reachable productions entered, %d matched; %d buckets reached",
		entered, c.Reachable(), matched, c.Reached())
}

// Report lists what has not been reached, in the shape a work list wants.
func (c *Coverage) Report() string {
	var b strings.Builder

	b.WriteString(c.String())

	if missing := c.Unreached(); len(missing) > 0 {
		fmt.Fprintf(&b, "\n\nnever entered (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}

	if considered := c.Considered(); len(considered) > 0 {
		fmt.Fprintf(&b, "\n\nentered but never matched (%d):\n  %s",
			len(considered), strings.Join(considered, "\n  "))
	}

	return b.String()
}

// span is every index belonging to one production, over all contexts and both
// indentation classes. Folding it is how the coarse questions are answered from
// the fine vector.
func span(id int32) (from, to int) {
	width := numContexts * numIndentBins * numIndentBins

	return int(id) * width, (int(id) + 1) * width
}

// Signature digests which buckets were entered, so that two recognitions can be
// compared for having gone the same way.
//
// It is a fingerprint of the route rather than of the document. For a document
// the grammar refuses, that is close to a statement of *how* it is wrong, which
// is what makes it a usable equivalence class: two documents that fail
// identically are one test case wearing two disguises.
//
// A digest rather than the bitset it used to be. The vector is around 120,000
// slots once indentation is in it, so the bitset would be fifteen kilobytes per
// document -- and its one use is as a map key over every document a build
// draws, where that is the difference between a corpus that builds and one that
// does not.
func (c *Coverage) Signature() string {
	// Sorted rather than taken in entry order: two documents that entered the
	// same buckets in a different sequence took the same route through the
	// grammar, and reporting them as different would defeat the grouping.
	entered := slices.Clone(c.touched)
	slices.Sort(entered)

	var (
		sum  = sha256.New()
		word [4]byte
	)

	for _, at := range entered {
		binary.LittleEndian.PutUint32(word[:], uint32(at))
		sum.Write(word[:])
	}

	return string(sum.Sum(nil))
}

func (c *Coverage) attempt(id int32, e env) {
	at := bucketOf(id, e)
	if c.attempts[at] == 0 && c.successes[at] == 0 {
		c.touched = append(c.touched, int32(at))
	}

	c.attempts[at]++
}

// succeed never has to record a touch: a rule is attempted before it can
// succeed, and both land on the same bucket.
func (c *Coverage) succeed(id int32, e env) {
	c.successes[bucketOf(id, e)]++
}

func (c *Coverage) mustAgree(o *Coverage) {
	if c.g != o.g {
		panic("grammar: coverage vectors from different grammars")
	}
}

func (c *Coverage) enteredAnywhere(id int32) bool {
	from, to := span(id)

	return slices.ContainsFunc(c.attempts[from:to], func(at uint32) bool { return at > 0 })
}

func (c *Coverage) matchedAnywhere(id int32) bool {
	from, to := span(id)

	return slices.ContainsFunc(c.successes[from:to], func(at uint32) bool { return at > 0 })
}

// Names lists the grammar's productions in the order their ids run, which is
// name order.
func (g *Grammar) Names() []string {
	out := make([]string, 0, len(g.slots))
	for name := range g.slots {
		out = append(out, name)
	}

	slices.Sort(out)

	return out
}
