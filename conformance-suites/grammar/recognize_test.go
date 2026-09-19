// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar_test

import (
	"testing"
	"time"

	"github.com/go-openapi/conformance-suites/grammar"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestCompiles asserts the whole grammar file turned into closures, rather than
// some prefix of it that happened not to hit an unimplemented form.
func TestCompiles(t *testing.T) {
	assert.Equal(t, 211, grammar.Rules())
}

// flowCases are documents that are exactly one flow node, and documents that
// are not. They are the flow half of the correctness gate: not a conformance
// claim, just enough to show the engine agrees with the spec where it can be
// checked by hand.
var flowCases = []struct {
	name  string
	src   string
	valid bool
}{
	// Flow collections.
	{"empty sequence", `[]`, true},
	{"empty mapping", `{}`, true},
	{"plain entries", `[a, b, c]`, true},
	{"trailing comma", `[a, b, c, ]`, true},
	{"nested", `[a, [b, {c: d}], e]`, true},
	{"mapping pair", `{a: 1, b: 2}`, true},
	{"spaces around", `[ a , b ]`, true},
	{"multi line", "[\n  a,\n  b\n ]", true},
	{"unclosed sequence", `[a, b`, false},
	{"mismatched close", `[a, b}`, false},
	{"stray close", `]`, false},
	{"double comma", `[a,, b]`, false},

	// Scalar styles.
	{"plain scalar", `hello world`, true},
	{"single quoted", `'it''s here'`, true},
	{"double quoted", `"a \n b"`, true},
	{"double quoted escape", `"☺"`, true},
	{"bad escape", `"\q"`, false},
	{"unterminated single", `'abc`, false},
	{"unterminated double", `"abc`, false},

	// Plain scalars are where the context sensitivity lives.
	{"colon in plain out", `a:b`, true},
	{"hash after text", `a#b`, true},
	{"comment starts", `a #b`, false},
	{"plain in flow with colon", `[a:b]`, true},
	{"plain cannot start with comma in flow", `[,a]`, false},

	// Node properties.
	{"anchor", `&a [1]`, true},
	{"tag", `!!str x`, true},
	{"tag and anchor", `!!str &a x`, true},
	{"alias", `*a`, true},
	{"alias cannot take content", `*a b`, false},
	{"empty node with tag", `!!str`, true},
}

func TestFlowNode(t *testing.T) {
	for _, tc := range flowCases {
		t.Run(tc.name, func(t *testing.T) {
			got := grammar.FlowNode([]byte(tc.src))
			assert.Equalf(t, tc.valid, got.OK, "%q: %s", tc.src, got)
		})
	}
}

// streamCases are whole documents, which is what the harness actually asks
// about. They exercise the forms flow context never reaches: indentation that
// is measured rather than stated, block scalar headers, and the markers that
// end one document and start the next.
//
// Each expectation is read off the spec by hand. That is the point of having
// them: the generative cross-check says the oracle and the library agree, which
// is worth nothing if they agree on the wrong thing.
var streamCases = []struct {
	name  string
	src   string
	valid bool
}{
	// Streams and documents.
	{"empty stream", ``, true},
	{"one break", "\n", true},
	{"only a comment", "# nothing else\n", true},
	{"bare scalar", "a\n", true},
	{"explicit document", "---\na\n", true},
	{"directive", "%YAML 1.2\n---\na\n", true},
	{"suffix", "---\na\n...\n", true},
	{"two bare documents", "a\n---\nb\n", true},
	{"two sequences", "---\n- a\n---\n- b\n", true},

	// Block mappings.
	{"one pair", "a: b\n", true},
	{"two pairs", "a: b\nc: d\n", true},
	{"empty value", "a:\n", true},
	{"nested mapping", "a:\n  b: c\n", true},
	{"explicit key", "? a\n: b\n", true},
	{"explicit key alone", "? a\n", true},
	{"blank line between", "a:\n\n  b: c\n", true},
	{"anchor on its own line", "a: &x\n  b: c\n", true},
	{"flow value", "a: [1, {b: c}]\n", true},
	{"a value cannot outdent", "a: b\n  c: d\n", false},
	{"a key needs a colon", "a: b\nb\n", false},
	{"two colons", "a: b: c\n", false},
	{"tabs do not indent", "a: b\n\tc: d\n", false},

	// Block sequences.
	{"two entries", "- a\n- b\n", true},
	{"sequence under a key", "a:\n- x\n- y\n", true},
	{"compact sequence", "- - a\n", true},
	{"compact mapping", "- a: 1\n  b: 2\n", true},

	// A compact collection is indented by an m of its own, counted from the
	// dash it follows. These three differ only in what the enclosing sequence
	// detected, which is what the rule inherits if nothing detects it here.
	{"compact under a wider parent", "- &z\n  - a: x\n", true},
	{"compact after two spaces", "-  - a: x\n", true},
	{"compact three deep", "&c\n- &b\n  - \"\": &a null\n", true},

	// Plain scalars fold across lines, which is why the two above are not
	// simply "a" followed by junk.
	{"plain over two lines", "a\nb\n", true},
	{"an indented dash continues a plain scalar", "- a\n - b\n", true},

	// Block scalars. The indentation indicator counts from the enclosing node,
	// and at the root that node sits at -1, so |1 is content in column 0.
	{"literal", "a: |\n  x\n", true},
	{"literal strip", "a: |-\n  x\n", true},
	{"literal keep", "a: |+\n  x\n\n", true},
	{"folded", "a: >\n  x\n", true},
	{"folded paragraphs", "a: >\n  one\n\n  two\n", true},
	{"stated indent", "a: |2\n   x\n", true},
	{"stated indent one", "a: |1\n x\n", true},
	{"stated indent at the root counts from -1", "|2\n a\n", true},
	{"leading empty line", "a: |\n\n  x\n", true},
	{"leading empty line indented less", "a: |\n \n  x\n", true},
	{"empty scalar before a sibling", "a: |\nb: c\n", true},
	{"trailing blank line under a stated indent", "|2\n a\n\n", true},
	{"stated wider than the content", "a: |2\nx\n", false},
	{"leading empty line indented more", "a: |\n    \n  a\n", false},

	// A block header takes its two indicators in either order, and the comment
	// that follows has to be tried against both.
	{"indent then chomp", "- |2-\n  x\n", true},
	{"chomp then indent", "- |-2\n  x\n", true},

	// Zero is not an indentation indicator: a block scalar's content is always
	// more indented than the node holding it.
	{"stated indent of zero", "|0\n", false},
	{"stated indent of zero after a marker", "--- |0\n", false},

	// Properties standing alone in key position. The node they name is empty,
	// and the reading where they belong to the enclosing collection instead has
	// to be given up when the comment it would need is not there.
	{"anchor as a whole key", "&a : a\n", true},
	{"tag as a whole key", "!!str : a\n", true},
	{"anchor and tag as a key", "&a !!str : a\n", true},
	{"anchor on an entry with a sibling", "- &a\n- a\n", true},

	// Directives, where the opposite holds: the first reading that matches is
	// the only one, or every malformed directive would parse as a reserved one.
	{"extra words on a yaml directive", "%YAML 1.2 foo\n---\n", false},
	{"comment run into a yaml directive", "%YAML 1.1#...\n---\n", false},
	{"a reserved directive takes parameters", "%FOO bar baz\n---\n", true},

	// Commitment covers only a YAML or TAG directive whose own production
	// matched. One that fails on its first parameter falls through to
	// ns-reserved-directive, whose name matches "TAG" as well as any other, so
	// patchReservedDirective keeps the two defined names out of it.
	{"a tag prefix opening on a flow indicator", "%TAG !e! [x\n---\n", false},
	{"a tag prefix opening on a comma", "%TAG !e! ,x\n---\n", false},
	{"a tag prefix with a flow indicator after its first character", "%TAG !e! x[1],y\n---\n", true},
	{"a tag directive with no prefix", "%TAG !e!\n---\n", false},
	{"a tag directive alone", "%TAG\n---\n", false},
	{"a yaml directive alone", "%YAML\n---\n", false},
	{"a yaml directive with a bad version", "%YAML .12\n---\n", false},
	{"a reserved directive named like tag", "%TAGS x\n---\n", true},
	{"a reserved directive named like yaml", "%YAMLX 1\n---\n", true},

	// A byte order mark stands in front of a document without putting anything
	// on the line, so what follows it still begins one. l-document-prefix
	// admits one before every document, not only the first.
	{"marked document", bom + "a: 1\n", true},
	{"marked comment", bom + "# c\n", true},
	{"marked marker", bom + "---\na: 1\n", true},
	{"mark alone", bom, true},
	{"mark after a suffix", "a: 1\n...\n" + bom + "b: 2\n", true},
	{"two marks", bom + bom + "a: 1\n", true},

	// Inside a document it is not a prefix, and nb-char excludes it from
	// content, so there is nowhere for it to be.
	{"mark inside a document", "---\n" + bom + "a: 1\n", false},

	// An indicator with the constraint on it left in the prose. Each pair is
	// the same bytes with and without the space the indicator needs, and both
	// are valid: "?a" is a plain scalar and "? a" is an explicit key, so the
	// lookahead decides which document this is rather than whether it is one.
	//
	// Nothing here would fail without the patches, and that is the measurement
	// rather than an oversight -- every one of these indicators is followed by
	// a rule wanting a separation, which refuses the same documents one step
	// later. They are asserted so that the pairs stay distinguishable.
	{"a question mark opens a plain scalar", "?a: b\n", true},
	{"a question mark alone is an explicit key", "? a: b\n", true},
	{"a question mark opens a flow scalar", "{?a: b}\n", true},
	{"a question mark alone in flow", "{? a: b}\n", true},
	{"three dashes with content behind them", "---foo\n", true},
	{"three dashes on their own line", "---\nfoo\n", true},
	{"a block header ends its line", "a: |2x\n  x\n", false},
	{"a block header with a comment", "a: |2 # c\n   x\n", true},

	// A flow collection carrying properties is still a flow node, which
	// ns-flow-yaml-node did not say: it offered only the plain and quoted
	// kinds after properties, so an anchored sequence had no reading at all.
	{"an anchored flow sequence as a key", "{&a [a]: b}\n", true},
	{"an anchored flow mapping", "&a {a: b}\n", true},

	// Properties are the collection's only if the line ends after them, and
	// the alternatives matter: c-ns-properties reads a tag and an anchor
	// together, so where only the tag belongs to the collection something has
	// to offer the shorter reading.
	{"a tag on its own line before a mapping", "!!map\n&a !!str k: v\n", true},
	{"an anchor on its own line before a mapping", "&a\n!!str k: v\n", true},
	{"properties on their own line before a mapping", "&a !!map\n&b !!str k: v\n", true},

	// An auto-detected indentation the spec calls an error, rather than a
	// width. The reading to refuse is not the obvious one: taking the first
	// non-empty line's width accepts the document, and so does giving up and
	// letting the scalar match empty -- which quietly hands its lines to
	// whatever encloses it.
	{"a leading empty line wider than a comment", "a: >\n \n  \n   \n # c\n", false},
	{"a leading empty line wider than the content", "a: >\n   \n \nb: 1\n", false},
}

// bom is written as a code point because a byte order mark in Go source is a
// compile error, which is its own small demonstration of the problem.
var bom = string(rune(0xFEFF))

func TestStream(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.name, func(t *testing.T) {
			got := grammar.Stream([]byte(tc.src))
			assert.Equalf(t, tc.valid, got.OK, "%q: %s", tc.src, got)
		})
	}
}

// TestMemoAgrees checks that memoization is an optimization and not a change of
// behavior. A parameterized PEG makes this worth asserting: if the memo key
// missed one of the four variables, this is where it would show.
func TestMemoAgrees(t *testing.T) {
	for _, tc := range flowCases {
		t.Run(tc.name, func(t *testing.T) {
			memo := grammar.Match("ns-flow-node", []byte(tc.src), 0, "flow-out")
			plain := grammar.MatchNoMemo("ns-flow-node", []byte(tc.src), 0, "flow-out")
			require.Equalf(t, memo.OK, plain.OK, "%q: memo=%s plain=%s", tc.src, memo, plain)
		})
	}

	// Block context is where the key could go wrong now: a rule that reports an
	// m or a t decides differently under a different one, and a rule inside a
	// bare document is asked with less input than the same rule outside one.
	for _, tc := range streamCases {
		t.Run(tc.name, func(t *testing.T) {
			memo := grammar.Match("l-yaml-stream", []byte(tc.src), -1, "block-in")
			plain := grammar.MatchNoMemo("l-yaml-stream", []byte(tc.src), -1, "block-in")
			require.Equalf(t, memo.OK, plain.OK, "%q: memo=%s plain=%s", tc.src, memo, plain)
		})
	}
}

// throughputCases are the documents throughput is measured against: small, but
// shaped like the ones a generator would produce.
var throughputCases = []string{
	`[a, b, c]`,
	`{name: value, other: thing}`,
	`[a, [b, {c: d}], e, [f, [g, [h]]]]`,
	`"a double quoted scalar with \t escapes and spaces"`,
	`[&anchor !!str value, *anchor, {key: [1, 2, 3]}]`,
	`a fairly long plain scalar that keeps going for a while yet`,
}

// TestThroughput reports how long a hundred thousand oracle calls would take,
// and what memoization is worth.
//
// It reports rather than asserts. The number is the input to a decision, not a
// property to defend.
func TestThroughput(t *testing.T) {
	skipTimings(t)

	const runs = 2000

	measure := func(match func(string) grammar.Result) (time.Duration, int64, int64) {
		start := time.Now()
		var steps, hits int64
		for range runs {
			for _, src := range throughputCases {
				r := match(src)
				steps += r.Steps
				hits += r.Hits
			}
		}

		return time.Since(start), steps, hits
	}

	fresh, steps, hits := measure(func(s string) grammar.Result {
		return grammar.Match("ns-flow-node", []byte(s), 0, "flow-out")
	})

	rec := grammar.NewRecognizer(64)
	reused, _, _ := measure(func(s string) grammar.Result {
		return rec.FlowNode([]byte(s))
	})

	docs := int64(runs * len(throughputCases))

	report := func(name string, total time.Duration) {
		per := total / time.Duration(docs)
		t.Logf("%-18s %8s/doc   100k documents in %s", name, per, (per * 100_000).Truncate(time.Millisecond))
	}

	t.Logf("%d steps/doc, %.0f%% of them answered by the memo table", steps/docs, 100*float64(hits)/float64(steps))
	report("table per call", fresh)
	report("table reused", reused)
	t.Logf("reusing the table is worth %.1fx", float64(fresh)/float64(reused))
}

func BenchmarkFlowNode(b *testing.B) {
	src := []byte(`[&anchor !!str value, *anchor, {key: [1, 2, 3]}]`)

	b.ReportAllocs()
	for b.Loop() {
		grammar.FlowNode(src)
	}
}
