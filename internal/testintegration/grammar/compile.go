// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// hexCode matches the grammar's way of writing a character by code point.
//
// The trailing quantifier is what keeps it from swallowing the literal
// character 'x' that ns-esc-8-bit matches: one 'x' with nothing after it is not
// a code point.
var hexCode = regexp.MustCompile(`^x([0-9A-F][0-9A-F])+$`)

// slot holds one production. Rules refer to each other cyclically, so calls go
// through the slot pointer and read fn at call time rather than capturing it.
type slot struct {
	name   string
	id     int32
	params []string
	fn     expr  // the matching form
	val    value // the (flip) form: a function of the variables, matching nothing
}

type compiler struct {
	raw   map[string]any
	slots map[string]*slot
}

// Grammar is a compiled set of productions, ready to recognize with.
//
// Compiling is a few milliseconds of walking a decoded structure, and the
// result is immutable, so one Grammar is shared by every caller.
type Grammar struct {
	name   string
	digest string
	slots  map[string]*slot
	// raw is the patched rule bodies the slots were compiled from, kept because
	// a compiled rule is a closure and closures cannot be walked. Static
	// reachability needs to read the grammar's shape, not run it -- see Reach.
	raw          map[string]any
	unreferenced []string
}

// MustCompile is Compile for a grammar that is embedded rather than supplied,
// where a failure is a defect in this package rather than bad input.
func MustCompile(name string, spec []byte, patches ...Patch) *Grammar {
	g, err := Compile(name, spec, patches...)
	if err != nil {
		panic("grammar: " + err.Error())
	}

	return g
}

// Digest identifies the grammar file this was compiled from.
//
// A corpus is a cache of what a grammar said, so the corpus has to record which
// grammar. When this changes, every stored verdict is suspect and the corpus
// has to be regenerated and compared rather than trusted.
//
// The digest reads the file with its line breaks as LF. A Windows checkout
// with core.autocrlf writes the embedded JSON with CRLF, which is the same
// grammar, and hashing the bytes as they stood told every stored corpus it had
// been built against another one.
func (g *Grammar) Digest() string { return g.digest }

// Unreferenced names the productions no other production refers to, in name
// order.
//
// These cannot be entered from any start symbol, so they belong in no coverage
// denominator. YAML 1.2 has nineteen of them and they are all one thing: the
// spec names each indicator character as a production -- c-anchor is "&",
// c-mapping-key is "?" -- for the prose to refer to, and then every rule that
// uses one writes the character directly instead.
//
// Counting them makes a corpus look permanently incomplete and hides the real
// gaps behind nineteen that can never close.
func (g *Grammar) Unreferenced() []string { return g.unreferenced }

// Name is what the grammar was compiled as, and appears in the panic when a
// caller asks for a production it does not define.
func (g *Grammar) Name() string { return g.name }

// Rules reports how many productions the grammar defines, so a test can assert
// the whole file compiled rather than some prefix of it.
func (g *Grammar) Rules() int { return len(g.slots) }

// Patch is a departure from the grammar file, applied before compiling.
//
// Some rules a spec states only in prose never reach its published grammar, so
// a faithful compilation of the file is not a faithful compilation of the
// language. Rather than special-case those at match time, the raw body is
// rewritten here and the rest of the compiler stays honest.
//
// Apply reports whether the body was the shape it expected. A patch that does
// not recognize its target fails the compile: a newer grammar file which either
// fixes the omission or moves it then says so, instead of quietly dropping the
// patch and taking a class of document with it.
type Patch struct {
	// Rule is the production to rewrite.
	Rule string
	// Because says what the grammar file leaves out, and appears in the error
	// when Apply refuses.
	Because string
	// Apply rewrites the rule body in place, and reports whether it recognized
	// it.
	Apply func(body any) bool
}

// Compile builds a Grammar from a spec in the productions format, applying
// patches in order before any rule is compiled.
func Compile(name string, spec []byte, patches ...Patch) (*Grammar, error) {
	c, err := newCompiler(spec, patches)
	if err != nil {
		return nil, fmt.Errorf("compiling the %s grammar: %w", name, err)
	}

	return &Grammar{
		name:         name,
		digest:       fmt.Sprintf("sha256:%x", sha256.Sum256(bytes.ReplaceAll(spec, []byte("\r\n"), []byte("\n")))),
		slots:        c.slots,
		raw:          c.raw,
		unreferenced: c.unreferenced(),
	}, nil
}

// unreferenced finds the productions that appear in no other production's body.
//
// A name reaches a rule either as a bare string in matching position or as the
// key of a call form, so both are collected. A rule referring only to itself is
// still unreferenced: nothing outside it can start it.
func (c *compiler) unreferenced() []string {
	seen := make(map[string]bool, len(c.raw))

	var walk func(from string, body any)

	walk = func(from string, body any) {
		switch form := body.(type) {
		case string:
			if form != from && c.slots[form] != nil {
				seen[form] = true
			}
		case []any:
			for _, item := range form {
				walk(from, item)
			}
		case map[string]any:
			for key, arg := range form {
				if key != from && c.slots[key] != nil {
					seen[key] = true
				}

				walk(from, arg)
			}
		}
	}

	for name, body := range c.raw {
		walk(name, body)
	}

	out := make([]string, 0, len(c.raw)-len(seen))

	for name := range c.raw {
		if !seen[name] {
			out = append(out, name)
		}
	}

	slices.Sort(out)

	return out
}

func newCompiler(spec []byte, patches []Patch) (*compiler, error) {
	var raw map[string]any
	if err := json.Unmarshal(spec, &raw); err != nil {
		return nil, fmt.Errorf("decoding the grammar: %w", err)
	}

	c := &compiler{
		raw:   make(map[string]any, len(raw)),
		slots: make(map[string]*slot, len(raw)),
	}

	// The ":007"-style keys index rule numbers back to the spec. They are not
	// productions.
	names := make([]string, 0, len(raw))

	for name, body := range raw {
		if strings.HasPrefix(name, ":") {
			continue
		}
		c.raw[name] = body
		names = append(names, name)
	}

	// Numbered in name order rather than in map order, so that a production's
	// id is the same in every process. Coverage vectors are indexed by it, and
	// a greedy selection over an unstably-indexed vector would choose a
	// different corpus on every run -- which is not something a checked-in
	// corpus can afford.
	slices.Sort(names)

	for id, name := range names {
		c.slots[name] = &slot{name: name, id: int32(id), params: declaredParams(c.raw[name])}
	}

	for _, p := range patches {
		body, ok := c.raw[p.Rule]
		if !ok {
			return nil, fmt.Errorf("no production named %q to patch, though %s", p.Rule, p.Because)
		}

		if !p.Apply(body) {
			return nil, fmt.Errorf("%s is not the shape it is patched into, where %s", p.Rule, p.Because)
		}
	}

	for name := range c.raw {
		c.compileRule(c.slots[name])
	}

	return c, nil
}

// declaredParams reads a rule's (...) form, which names the variables the rule
// takes. Every call site passes them explicitly -- no production in the grammar
// refers to a parameterized rule bare -- so this is the only place binding
// happens.
func declaredParams(body any) []string {
	m, ok := body.(map[string]any)
	if !ok {
		return nil
	}
	decl, ok := m["(...)"]
	if !ok {
		return nil
	}
	if one, ok := decl.(string); ok {
		return []string{one}
	}

	list, _ := decl.([]any)
	params := make([]string, 0, len(list))
	for _, p := range list {
		params = append(params, asType[string](p))
	}

	return params
}

func (c *compiler) compileRule(s *slot) {
	body := c.raw[s.name]

	if m, ok := body.(map[string]any); ok {
		if flip, ok := m["(flip)"]; ok {
			s.val = c.flip(flip)

			return
		}
	}

	s.fn = c.matcher(strip(body))
}

// strip removes the parameter declaration from a rule body, leaving the single
// form that does the matching.
func strip(body any) any {
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if _, ok := m["(...)"]; !ok {
		return body
	}

	out := make(map[string]any, len(m)-1)
	for k, v := range m {
		if k != "(...)" {
			out[k] = v
		}
	}

	return out
}

// matcher compiles a form appearing in matching position.
//
// Matching position is what disambiguates the grammar's scalars, and it has to:
// once encoded as JSON, the literal character 'n' that ns-esc-line-feed matches
// is indistinguishable from the variable n. Variables occur only as arguments,
// so a scalar reached through here is never one.
func (c *compiler) matcher(v any) expr {
	switch form := v.(type) {
	case string:
		return c.matchString(form)
	case []any:
		// A bare pair in matching position is a code point range.
		return c.charRange(form)
	case map[string]any:
		return c.matchForm(form)
	default:
		panic(fmt.Sprintf("grammar: unexpected form in matching position: %#v", v))
	}
}

func (c *compiler) matchString(form string) expr {
	switch form {
	case "<start-of-line>":
		return startOfLine
	case "<end-of-stream>":
		return endOfStream
	case "<empty>":
		return matchEmpty
	}

	if s, ok := c.slots[form]; ok {
		return c.call(s, nil)
	}

	if hexCode.MatchString(form) {
		return literalRune(decodeHex(form))
	}

	return literalString(form)
}

func (c *compiler) matchForm(form map[string]any) expr {
	// (if) and (set) are the one two-key form in the grammar, so they are
	// handled before the arity check rather than in the switch below.
	if assignment, ok := form["(set)"]; ok {
		return c.set(assignment, form["(if)"])
	}

	if len(form) != 1 {
		panic(fmt.Sprintf("grammar: expected a single form, got %d keys: %#v", len(form), form))
	}

	for key, arg := range form {
		switch key {
		case "(any)":
			return choice(c.each(arg))
		case "(all)":
			return c.allForm(arg)
		case "(+++)":
			return repeat(c.matcher(arg), 1, -1)
		case "(***)":
			return repeat(c.matcher(arg), 0, -1)
		case "(???)":
			return repeat(c.matcher(arg), 0, 1)
		case "(---)":
			return c.subtract(arg)
		case "(===)":
			return lookahead(c.matcher(arg), true)
		case "(!==)":
			return lookahead(c.matcher(arg), false)
		case "(<==)":
			return lookbehind(c.matcher(arg))
		case "(case)":
			return c.caseOf(arg)
		case "(max)":
			return limitTo(int(asType[float64](arg)))
		case "(exclude)":
			return c.exclude(arg)
		case "(<<<)":
			return capture(c.matcher(arg))
		case "(<)":
			return c.compare(arg, false)
		case "(<=)":
			return c.compare(arg, true)
		}

		if strings.HasPrefix(key, "({") && strings.HasSuffix(key, "})") {
			return c.repeatExactly(key[2:len(key)-2], c.matcher(arg))
		}

		if s, ok := c.slots[key]; ok {
			return c.call(s, arg)
		}

		panic(fmt.Sprintf("grammar: unknown form %q", key))
	}

	panic("unreachable")
}

// allForm compiles (all), giving the optional steps inside it the meaning the
// spec's notation gives them rather than the one a PEG would.
//
// A PEG's optional is possessive: in "A? B", an A that matched is kept even
// when B then fails, and the sequence fails with it. The spec writes BNF, where
// "A? B" reads as "(A B) | B" and both are to be tried. s-l+block-collection is
// where the difference is visible: an anchor written on a mapping key matches
// as the collection's own properties, the comment that must follow them is not
// there, and the sequence fails without ever trying the reading where the
// anchor belongs to the key. That is a valid document refused.
//
// So a sequence holding k optionals compiles to 2^k sequences, the one taking
// every optional first, which leaves the greedy reading in front wherever it
// already worked. The grammar has 25 sequences with one optional and 6 with
// two, so k is never above two and the choice never wider than four.
func (c *compiler) allForm(arg any) expr {
	list, ok := arg.([]any)
	if !ok {
		return sequence([]expr{c.matcher(arg)})
	}

	var optional []int

	for i, item := range list {
		if optionalBody(item) != nil {
			optional = append(optional, i)
		}
	}

	if len(optional) == 0 {
		return sequence(c.each(list))
	}

	alts := make([]expr, 0, 1<<len(optional))
	for mask := 1<<len(optional) - 1; mask >= 0; mask-- {
		steps := make([]expr, 0, len(list))
		taken := 0

		for i, item := range list {
			if taken < len(optional) && optional[taken] == i {
				// The first optional is the high bit, so counting the mask
				// down tries them greedily from the left.
				bit := len(optional) - 1 - taken
				taken++

				if mask&(1<<bit) != 0 {
					steps = append(steps, c.matcher(optionalBody(item)))
				}

				continue
			}

			steps = append(steps, c.matcher(item))
		}

		alts = append(alts, sequence(steps))
	}

	return choice(alts)
}

// optionalBody reports the body of a (???) form, or nil for anything else.
func optionalBody(item any) any {
	form, ok := item.(map[string]any)
	if !ok || len(form) != 1 {
		return nil
	}

	return form["(???)"]
}

func (c *compiler) each(arg any) []expr {
	list, ok := arg.([]any)
	if !ok {
		return []expr{c.matcher(arg)}
	}

	out := make([]expr, 0, len(list))
	for _, item := range list {
		out = append(out, c.matcher(item))
	}

	return out
}

// call binds a rule's declared parameters from the call site's arguments and
// invokes it through the memo table.
//
// A parameter the call site passed as the bare variable of the same name is
// passed by reference: whatever the callee left it at comes back. That is the
// only way c-b-block-header(m,t) can report a header, and it is narrow enough
// to be safe -- l-literal-content(n+m, t) passes an expression for n, so the
// callee's n stays the callee's.
func (c *compiler) call(s *slot, arg any) expr {
	args := c.arguments(s, arg)

	return func(st *state, e env) (env, bool) {
		callee := e
		for i, a := range args {
			v := a.of(st, e)

			// An indentation auto-detection reports the spec's error cases as
			// an impossible column, and this is where that has to bite: there
			// is no node at a column no document can have, so the rule is not
			// entered. Left to the rule, a body that is optional throughout
			// would match nothing and report success.
			if n, ok := v.(int); ok && impossible(n) {
				return e, false
			}

			assign(&callee, s.params[i], v)
		}

		out, ok := invoke(s, st, callee)
		if !ok {
			return e, false
		}

		result := e
		for i, a := range args {
			if a.byName {
				assign(&result, s.params[i], read(out, s.params[i]))
			}
		}

		return result, true
	}
}

// argument is one compiled call-site argument, and whether it was written as
// the bare variable the callee names it by.
type argument struct {
	of     value
	byName bool
}

// arguments compiles a call site's argument list. A rule taking one parameter
// receives the whole value; a rule taking several receives a sequence, and the
// two cannot be confused because the arity comes from the callee.
func (c *compiler) arguments(s *slot, arg any) []argument {
	if len(s.params) == 0 {
		return nil
	}

	if len(s.params) == 1 {
		return []argument{c.argument(s.params[0], arg)}
	}

	list, ok := arg.([]any)
	if !ok || len(list) != len(s.params) {
		panic(fmt.Sprintf("grammar: %s takes %d arguments, call site passes %#v", s.name, len(s.params), arg))
	}

	out := make([]argument, 0, len(list))
	for i, item := range list {
		out = append(out, c.argument(s.params[i], item))
	}

	return out
}

func (c *compiler) argument(param string, arg any) argument {
	name, _ := arg.(string)

	return argument{of: c.value(arg), byName: name == param}
}

// value compiles a form appearing in argument position, where scalars name
// variables rather than characters to match.
func (c *compiler) value(v any) value {
	switch form := v.(type) {
	case nil:
		return constant(nNull)
	case float64:
		return constant(int(form))
	case string:
		return c.valueString(form)
	case map[string]any:
		return c.valueForm(form)
	default:
		panic(fmt.Sprintf("grammar: unexpected form in argument position: %#v", v))
	}
}

func (c *compiler) valueString(form string) value {
	switch form {
	case "n":
		return func(_ *state, e env) any { return e.n }
	case "m":
		return func(_ *state, e env) any { return e.m }
	case "c":
		return func(_ *state, e env) any { return e.c }
	case "t":
		return func(_ *state, e env) any { return e.t }
	case "(match)":
		return func(s *state, _ env) any { return string(s.src[s.mark:s.pos]) }
	case "auto-detect":
		// A block scalar header that states no width. Detecting it here is too
		// early -- the content has not been reached -- so it travels as a
		// sentinel and arithmetic resolves it where n+m is computed.
		return constant(mAuto)
	case "<auto-detect-indent>":
		return func(s *state, e env) any { return detectCollectionIndent(s, e.n) }
	case "<auto-detect-compact-indent>":
		return func(s *state, _ env) any { return detectCompactIndent(s) }
	}

	// A context or chomping name: "flow-in", "keep", and the like. Resolved to
	// its code here, once, rather than on every step that compares it.
	if code, ok := contextCode[form]; ok && form != "" {
		return constant(code)
	}
	if code, ok := chompCode[form]; ok && form != "" {
		return constant(code)
	}

	panic(fmt.Sprintf("grammar: %q is not a variable, context or chomping mode", form))
}

func (c *compiler) valueForm(form map[string]any) value {
	if len(form) != 1 {
		panic(fmt.Sprintf("grammar: expected a single argument form, got %#v", form))
	}

	for key, arg := range form {
		switch key {
		case "(+)":
			return c.arithmetic(arg, 1)
		case "(-)":
			return c.arithmetic(arg, -1)
		case "(len)":
			inner := c.value(arg)

			return func(s *state, e env) any { return len(asType[string](inner(s, e))) }
		case "(ord)":
			return c.ordinal(arg)
		}

		if s, ok := c.slots[key]; ok {
			return c.callValue(s, arg)
		}

		panic(fmt.Sprintf("grammar: unknown argument form %q", key))
	}

	panic("unreachable")
}

func (c *compiler) arithmetic(arg any, sign int) value {
	list := asType[[]any](arg)
	left, right := c.value(list[0]), c.value(list[1])

	return func(s *state, e env) any {
		l := toInt(left(s, e))
		if l == nNull {
			return nNull
		}

		r := toInt(right(s, e))
		if r == mAuto {
			// n + auto-detect. Only c-l+literal and c-l+folded compute this,
			// and only after s-b-comment has consumed the header's line break,
			// so the position is the first line of the content -- which is the
			// one place the width can be read off.
			return detectScalarIndent(s, l)
		}

		return l + sign*r
	}
}

// callValue invokes a (flip) rule, which matches nothing and returns a value
// derived from the variables it is passed.
//
// This is the one production entry that does not go through [invoke], because a
// (flip) rule has no matching form to run. So it records coverage itself:
// leaving it out made in-flow and seq-spaces look unreachable when both are
// used on every flow collection in the grammar.
func (c *compiler) callValue(s *slot, arg any) value {
	args := c.arguments(s, arg)

	return func(st *state, e env) any {
		callee := e
		for i, a := range args {
			assign(&callee, s.params[i], a.of(st, e))
		}

		if st.cover != nil {
			// A (flip) rule cannot fail: it maps its arguments to a value and
			// returns it, so being entered and being satisfied are the same
			// event here.
			st.cover.attempt(s.id, callee)
			st.cover.succeed(s.id, callee)
		}

		return s.val(st, callee)
	}
}

// flip compiles the (flip) form: a switch on one variable whose arms are values
// rather than matchers.
//
// Every (flip) in the grammar switches on c, and the arms are indexed by
// context code so that dispatching is an array load.
func (c *compiler) flip(arg any) value {
	form := asType[map[string]any](arg)
	if name := asType[string](form["var"]); name != "c" {
		panic(fmt.Sprintf("grammar: (flip) switches on %q, only c was expected", name))
	}

	var arms [numArms]value
	for k, v := range form {
		if k == "var" {
			continue
		}
		arms[armIndex(k)] = c.value(v)
	}

	return func(s *state, e env) any {
		arm := arms[e.c]
		if arm == nil {
			panic(fmt.Sprintf("grammar: (flip) has no arm for c=%d", e.c))
		}

		return arm(s, e)
	}
}

// caseOf compiles the (case) form: a switch on one variable whose arms are
// matchers.
func (c *compiler) caseOf(arg any) expr {
	form := asType[map[string]any](arg)
	name := asType[string](form["var"])

	var arms [numArms]expr
	if name == "t" {
		arms = [numArms]expr{}
	}
	for k, v := range form {
		if k == "var" {
			continue
		}
		arms[armIndex(k)] = c.matcher(v)
	}

	if name == "t" {
		return func(s *state, e env) (env, bool) {
			arm := arms[e.t]
			if arm == nil {
				// The grammar leaves arms out where the spec says the
				// combination cannot arise. Reaching one means we arrived
				// wrongly, so say so rather than quietly failing to match.
				panic(fmt.Sprintf("grammar: (case) has no arm for t=%d", e.t))
			}

			return arm(s, e)
		}
	}

	return func(s *state, e env) (env, bool) {
		arm := arms[e.c]
		if arm == nil {
			panic(fmt.Sprintf("grammar: (case) has no arm for c=%d", e.c))
		}

		return arm(s, e)
	}
}

// armIndex resolves a (case) or (flip) arm's name to the code it switches on.
// Contexts and chomping modes never collide, so one table serves both.
func armIndex(name string) uint8 {
	if code, ok := contextCode[name]; ok && name != "" {
		return uint8(code)
	}
	if code, ok := chompCode[name]; ok && name != "" {
		return uint8(code)
	}

	panic(fmt.Sprintf("grammar: %q is not a context or chomping mode", name))
}

// subtract compiles (---): the first set, minus every set after it.
func (c *compiler) subtract(arg any) expr {
	list := asType[[]any](arg)
	first := c.matcher(list[0])

	minus := make([]expr, 0, len(list)-1)
	for _, item := range list[1:] {
		minus = append(minus, c.matcher(item))
	}

	return func(s *state, e env) (env, bool) {
		start := s.pos
		out, ok := first(s, e)
		if !ok {
			return e, false
		}
		end := s.pos

		for _, m := range minus {
			s.pos = start
			if _, ok := m(s, e); ok {
				s.pos = start

				return e, false
			}
		}
		s.pos = end

		return out, true
	}
}

// repeatExactly compiles ({n}) and its constant forms, ({2}), ({4}), ({8}).
func (c *compiler) repeatExactly(count string, inner expr) expr {
	if fixed, err := strconv.Atoi(count); err == nil {
		return repeat(inner, fixed, fixed)
	}

	name := count

	return func(s *state, e env) (env, bool) {
		n := readInt(e, name)
		if n == nNull || n < 0 {
			// s-indent(n) with a negative n matches nothing, successfully:
			// there is no indentation to consume.
			return e, n != nNull
		}

		return repeat(inner, n, n)(s, e)
	}
}

// compare compiles (<) and (<=), which assert a relation between two argument
// expressions and consume nothing.
func (c *compiler) compare(arg any, orEqual bool) expr {
	list := asType[[]any](arg)
	left, right := c.value(list[0]), c.value(list[1])

	return func(s *state, e env) (env, bool) {
		l, r := toInt(left(s, e)), toInt(right(s, e))
		if impossible(l) || impossible(r) {
			return e, false
		}

		if orEqual {
			return e, l <= r
		}

		return e, l < r
	}
}

func assign(e *env, param string, v any) {
	switch param {
	case "n":
		e.n = toInt(v)
	case "m":
		e.m = toInt(v)
	case "c":
		e.c = asType[ctxCode](v)
	case "t":
		e.t = asType[chompCodeT](v)
	default:
		panic(fmt.Sprintf("grammar: unknown parameter %q", param))
	}
}

// read is assign's inverse, for handing an out-parameter back to the call site.
func read(e env, param string) any {
	switch param {
	case "n":
		return e.n
	case "m":
		return e.m
	case "c":
		return e.c
	case "t":
		return e.t
	default:
		panic(fmt.Sprintf("grammar: unknown parameter %q", param))
	}
}

func readInt(e env, name string) int {
	switch name {
	case "n":
		return e.n
	case "m":
		return e.m
	default:
		panic(fmt.Sprintf("grammar: %q is not an integer variable", name))
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		panic(fmt.Sprintf("grammar: expected an integer, got %#v", v))
	}
}

func constant(v any) value {
	return func(_ *state, _ env) any { return v }
}

func decodeHex(code string) rune {
	n, err := strconv.ParseInt(code[1:], 16, 32)
	if err != nil {
		panic(fmt.Sprintf("grammar: bad code point %q: %v", code, err))
	}

	return rune(n)
}

// asType asserts v to T. The values it reads come from the grammar file this package embeds and from the codes the
// compiler itself puts in an env, so a value of another type is a malformed grammar or a compiler defect, and a panic
// is the right failure for either.
func asType[T any](v any) T {
	return v.(T) //nolint:forcetypeassert // see above: a wrong type here is a bug, and panicking names it
}
