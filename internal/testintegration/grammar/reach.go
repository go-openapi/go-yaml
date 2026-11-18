// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	"fmt"
	"slices"
	"strings"
)

// Reach is the set of production-by-context buckets a recognition starting
// somewhere can enter, computed from the grammar rather than measured from a
// corpus.
//
// # Why a denominator has to be computed
//
// A coverage vector holds one bucket per production per context, and nearly all
// of them are empty because nearly every production is only ever entered in one
// or two contexts: nothing calls s-l+block-collection in flow-key, and nothing
// ever will. YAML 1.2 has 211 productions and seven contexts, so the vector has
// 1,477 slots and a corpus reaching every bucket it could would still show as a
// third full.
//
// Quoted against that, "559 buckets reached" says nothing at all -- not whether
// the corpus is nearly done or barely started, and not which direction work
// should go. A target needs the number of buckets that exist.
//
// # Exact rather than approximate
//
// This is a fixed point over the call graph, carrying the context along each
// edge, and it is exact because the grammar leaves it no room not to be: the
// context at a call site is written as one of three things, a literal context
// name, the variable c, or in-flow(c), and all three are decidable without
// running anything. A rule that does not declare c inherits its caller's, which
// is what the compiled call does too.
//
// Two places need care, and both narrow the answer rather than widening it. A
// (case) on the context takes exactly the arm for the context it is in, so the
// other arms' calls are not reachable through it. And a (flip) with no arm for
// the context it is handed cannot return one, so nothing past it is reachable
// -- the recognizer panics there, which is the same statement said louder.
//
// # It is checked, not trusted
//
// A static denominator that is wrong is worse than none, because it reports
// progress that is not there. [Coverage.Impossible] is the falsification: any
// bucket a real recognition entered that this says is unreachable is a defect
// here, and the corpus is large enough to find one.
type Reach struct {
	g     *Grammar
	start string
	from  string
	in    []bool
	count int
}

// Reach computes the buckets reachable from a start production entered in a
// given context.
//
// The answer depends on both. l-yaml-stream in block-in is the question a
// document asks; ns-flow-node in flow-out is a much smaller grammar, and
// measuring one against the other's denominator would be measuring nothing.
func (g *Grammar) Reach(start, from string) *Reach {
	code, ok := contextCode[from]
	if !ok {
		panic(fmt.Sprintf("grammar: no context named %q", from))
	}

	r := &Reach{
		g:     g,
		start: start,
		from:  from,
		in:    make([]bool, len(g.slots)*numContexts),
	}

	r.enter(start, code)

	return r
}

// Buckets is how many production-by-context pairs a recognition from here can
// enter. This is the denominator.
func (r *Reach) Buckets() int { return r.count }

// calls walks a rule body in one context and reports every production it can
// enter, with the context it would be entered in.
func (g *Grammar) calls(body any, c ctxCode, emit func(string, ctxCode)) {
	switch form := body.(type) {
	case string:
		// A bare name in matching position is a call to a rule taking no
		// arguments, which inherits the context it was called in.
		if _, ok := g.slots[form]; ok {
			emit(form, c)
		}

	case []any:
		for _, item := range form {
			g.calls(item, c, emit)
		}

	case map[string]any:
		// A (flip) rule has no matching form: it maps its arguments to a value
		// and returns, so it enters nothing.
		if _, ok := form["(flip)"]; ok {
			return
		}

		if arg, ok := form["(case)"]; ok {
			g.caseCalls(arg, c, emit)

			return
		}

		for key, arg := range form {
			// The parameter declaration names variables, not productions.
			if key == "(...)" {
				continue
			}

			if _, ok := g.slots[key]; ok {
				if to, ok := g.contextFor(key, arg, c); ok {
					emit(key, to)
				}
			}

			// The argument is walked whether or not the key was a call, because
			// an argument can hold one: l+block-sequence(seq-spaces(n,c))
			// enters seq-spaces, and callValue records it.
			g.calls(arg, c, emit)
		}
	}
}

// caseCalls walks the one arm a (case) can take.
//
// A (case) on the context is the one place the analysis would be badly wrong if
// it walked everything: c-l-block-map-implicit-value has an arm per context,
// and taking all of them would put every arm's productions in every context's
// denominator. A (case) on the chomping mode is a different matter -- t is not
// what buckets are indexed by, so all of its arms are live.
func (g *Grammar) caseCalls(arg any, c ctxCode, emit func(string, ctxCode)) {
	form, ok := arg.(map[string]any)
	if !ok {
		return
	}

	on, _ := form["var"].(string)

	for key, body := range form {
		if key == "var" {
			continue
		}

		if on == "c" {
			code, ok := contextCode[key]
			if !ok || code != c {
				continue
			}
		}

		g.calls(body, c, emit)
	}
}

// contextFor resolves the context a call site enters its callee in, reporting
// false where no call can happen at all.
func (g *Grammar) contextFor(callee string, arg any, c ctxCode) (ctxCode, bool) {
	s := g.slots[callee]

	at := slices.Index(s.params, "c")
	if at < 0 {
		// The compiled call copies the caller's env and overwrites only the
		// parameters the callee declares, so a callee with no c keeps the
		// caller's.
		return c, true
	}

	if len(s.params) > 1 {
		list, ok := arg.([]any)
		if !ok || len(list) != len(s.params) {
			panic(fmt.Sprintf("grammar: %s takes %d arguments, call site passes %#v",
				callee, len(s.params), arg))
		}

		arg = list[at]
	}

	return g.contextValue(arg, c)
}

// contextValue evaluates an argument written in context position.
//
// The grammar writes exactly three things there and this refuses anything else,
// so a grammar that grows a fourth says so rather than being quietly guessed
// at. A wrong guess here does not fail: it produces a denominator that is a
// little wrong, which is the failure mode this whole file exists to avoid.
func (g *Grammar) contextValue(arg any, c ctxCode) (ctxCode, bool) {
	switch form := arg.(type) {
	case string:
		if form == "c" {
			return c, true
		}

		if code, ok := contextCode[form]; ok && form != "" {
			return code, true
		}

	case map[string]any:
		for key, inner := range form {
			if _, ok := g.slots[key]; !ok {
				continue
			}

			in, ok := g.contextValue(inner, c)
			if !ok {
				return 0, false
			}

			return g.flipContext(key, in)
		}
	}

	panic(fmt.Sprintf("grammar: %#v is not a context", arg))
}

// flipContext reads a (flip) rule's arms to resolve what it returns for one
// context, reporting false where it has no arm for it.
//
// The arms are read out of the grammar rather than known here, so that in-flow
// changing its mind is a change in one place. No arm means the compiled flip
// would panic, which makes everything past this call site unreachable -- and
// saying so is the point, since a denominator counting it would be counting
// buckets no document can ever fill.
func (g *Grammar) flipContext(rule string, in ctxCode) (ctxCode, bool) {
	body, ok := g.raw[rule].(map[string]any)
	if !ok {
		return 0, false
	}

	form, ok := body["(flip)"].(map[string]any)
	if !ok {
		panic(fmt.Sprintf("grammar: %s is used as a context and is not a (flip)", rule))
	}

	for key, arm := range form {
		code, ok := contextCode[key]
		if key == "var" || !ok || code != in {
			continue
		}

		name, ok := arm.(string)
		if !ok {
			return 0, false
		}

		out, ok := contextCode[name]

		return out, ok && name != ""
	}

	return 0, false
}

// Rules is how many distinct productions a recognition from here can enter, in
// any context.
func (r *Reach) Rules() int {
	var n int

	for _, s := range r.g.slots {
		if r.anyContext(s.id) {
			n++
		}
	}

	return n
}

// Has reports whether one bucket is reachable.
func (r *Reach) Has(rule, ctx string) bool {
	code, ok := contextCode[ctx]
	if !ok {
		panic(fmt.Sprintf("grammar: no context named %q", ctx))
	}

	return r.in[int(r.g.slotFor(rule).id)*numContexts+int(code)]
}

// Unreachable names the productions no context can enter from this start, in
// name order.
//
// For YAML 1.2 from l-yaml-stream these are the indicator characters the spec
// names for its prose and then writes literally everywhere it uses them, plus
// whatever start symbols this one is not. Nothing a corpus does will move them,
// which is why they are named rather than counted.
func (r *Reach) Unreachable() []string {
	var out []string

	for name, s := range r.g.slots {
		if !r.anyContext(s.id) {
			out = append(out, name)
		}
	}

	slices.Sort(out)

	return out
}

func (r *Reach) String() string {
	return fmt.Sprintf("%s from %s in %s: %d buckets over %d productions, %d productions unreachable",
		r.g.name, r.start, quoteContext(r.from), r.count, r.Rules(), len(r.Unreachable()))
}

// enter marks a bucket and follows everything it can call, stopping when a
// bucket is already marked. The grammar is cyclic, so that is what makes this
// terminate.
func (r *Reach) enter(name string, c ctxCode) {
	s := r.g.slotFor(name)

	at := int(s.id)*numContexts + int(c)
	if r.in[at] {
		return
	}

	r.in[at] = true
	r.count++

	r.g.calls(r.g.raw[name], c, r.enter)
}

func (r *Reach) anyContext(id int32) bool {
	return slices.Contains(r.in[int(id)*numContexts:(int(id)+1)*numContexts], true)
}

func quoteContext(c string) string {
	if c == "" {
		return "no context"
	}

	return c
}

// Against scores a coverage vector on the buckets that exist rather than on the
// buckets the vector has room for.
func (c *Coverage) Against(r *Reach) (reached, matched, total int) {
	c.mustReach(r)

	// r is indexed by production and context; the vector adds the two
	// indentation classes underneath. So each live slot folds a square of the
	// vector: the question here is whether a rule ran in a context, not how far
	// in it was indented.
	const fold = numIndentBins * numIndentBins

	for i, live := range r.in {
		if !live {
			continue
		}

		total++

		if slices.ContainsFunc(c.attempts[i*fold:(i+1)*fold], positive) {
			reached++
		}

		if slices.ContainsFunc(c.successes[i*fold:(i+1)*fold], positive) {
			matched++
		}
	}

	return reached, matched, total
}

func positive(n uint32) bool { return n > 0 }

// Missing names the buckets a recognition could have entered and none did, in
// name order.
//
// This is the work list. Unlike [Coverage.Unreached] it names a context as well
// as a production, so a rule reached everywhere but flow-key reports the one
// hole rather than looking finished.
func (c *Coverage) Missing(r *Reach) []string {
	return c.buckets(r, func(live, entered bool) bool { return live && !entered })
}

// Impossible names the buckets something entered that [Reach] says cannot be
// entered, in name order.
//
// It should always be empty, and a test should say so. A static denominator
// nobody falsifies is a number that can drift wrong in the safe-looking
// direction -- reporting a corpus as complete because the buckets it never
// filled were quietly left out of the count.
func (c *Coverage) Impossible(r *Reach) []string {
	return c.buckets(r, func(live, entered bool) bool { return !live && entered })
}

// Unmatched names the buckets something entered and nothing ever satisfied, in
// name order.
//
// The other half of the number [Coverage.Against] reports. Entering a
// production only means the recognizer tried it there; matching means a
// document in the corpus was one it accepted. So a bucket here is a shape the
// corpus makes the grammar consider and never writes -- a work list of the same
// kind as [Coverage.Missing], one rung further up.
//
// Read it whenever the matched count drops. A value kind added to the generator
// takes its share of the draws from every other kind, and this says which
// productions paid for it.
func (c *Coverage) Unmatched(r *Reach) []string {
	return c.satisfied(r, func(live, matched bool) bool { return live && !matched })
}

// ReportAgainst is [Coverage.Report] with a denominator that means something.
func (c *Coverage) ReportAgainst(r *Reach) string {
	reached, matched, total := c.Against(r)

	var b strings.Builder

	fmt.Fprintf(&b, "%d/%d buckets entered, %d matched (%s)", reached, total, matched, r)

	if missing := c.Missing(r); len(missing) > 0 {
		fmt.Fprintf(&b, "\n\nnever entered (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}

	return b.String()
}

func (c *Coverage) buckets(r *Reach, want func(live, entered bool) bool) []string {
	return c.over(r, c.attempts, want)
}

func (c *Coverage) satisfied(r *Reach, want func(live, matched bool) bool) []string {
	return c.over(r, c.successes, want)
}

func (c *Coverage) over(r *Reach, counts []uint32, want func(live, hit bool) bool) []string {
	c.mustReach(r)

	const fold = numIndentBins * numIndentBins

	names := c.g.Names()

	var out []string

	for i, live := range r.in {
		entered := slices.ContainsFunc(counts[i*fold:(i+1)*fold], positive)
		if !want(live, entered) {
			continue
		}

		out = append(out, names[i/numContexts]+"@"+quoteContext(contextName(ctxCode(i%numContexts))))
	}

	slices.Sort(out)

	return out
}

// contextName is contextCode's inverse, for reporting.
func contextName(code ctxCode) string {
	for name, c := range contextCode {
		if c == code {
			return name
		}
	}

	panic(fmt.Sprintf("grammar: no context with code %d", code))
}

func (c *Coverage) mustReach(r *Reach) {
	if c.g != r.g {
		panic("grammar: a coverage vector and a reachability set from different grammars")
	}
}
