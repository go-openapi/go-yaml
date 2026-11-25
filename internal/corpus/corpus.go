// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package corpus generates the synthetic documents the benchmarks run on.
//
// It lives under testdata so that it is not part of the published module, and
// it is shared so that a number measured in one package means the same thing as
// a number measured in another.
//
// The shapes are chosen for what they expose, not for realism:
//
//   - FlatMap is wide and shallow. Sibling count at one level is the axis that
//     super-linear parsing shows up on, and a depth guard does not bound it.
//   - NestedDoc is the shape of a real OpenAPI specification. It understates
//     the width effect -- the sibling count at any one level stays low -- which
//     is exactly why both shapes are needed.
//   - Anchored and BlockScalars cover the two constructs that constrain
//     streaming: anchors must be retained until the document ends, and block
//     scalars are where the scanner does its least regular work.
package corpus

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// FlatMap builds a mapping of n sibling keys at one level.
func FlatMap(n int) string {
	var b strings.Builder
	const entryBytes = 24 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: value%06d\n", i, i)
	}

	return b.String()
}

// FlatSequence builds a sequence of n entries at one level, the sequence
// counterpart of FlatMap.
func FlatSequence(n int) string {
	var b strings.Builder
	const entryBytes = 16 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "- value%06d\n", i)
	}

	return b.String()
}

// FlatFlowMap builds a flow mapping of n entries on one line, the flow counterpart of FlatMap.
func FlatFlowMap(n int) string {
	var b strings.Builder
	const entryBytes = 24 // the bytes one entry writes, rounded up
	b.Grow(n*entryBytes + 2)

	b.WriteByte('{')
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "key%06d: value%06d", i, i)
	}
	b.WriteString("}\n")

	return b.String()
}

// FlatFlowSequence builds a flow sequence of n entries on one line, the flow counterpart of FlatSequence.
func FlatFlowSequence(n int) string {
	var b strings.Builder
	const entryBytes = 13 // the bytes one entry writes, rounded up
	b.Grow(n*entryBytes + 2)

	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "value%06d", i)
	}
	b.WriteString("]\n")

	return b.String()
}

// NestedDoc builds an OpenAPI-shaped document with n sibling paths, each a
// small nested mapping.
func NestedDoc(n int) string {
	var b strings.Builder
	const entryBytes = 160 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)
	b.WriteString("openapi: 3.0.0\ninfo:\n  title: Corpus\n  version: 1.0.0\npaths:\n")

	for i := range n {
		fmt.Fprintf(&b,
			"  /res%d:\n    get:\n      operationId: getRes%d\n"+
				"      summary: a reasonably long summary line for realism\n"+
				"      responses:\n        '200':\n          description: ok\n", i, i)
	}

	return b.String()
}

// Anchored builds a document defining one anchor and referring to it n times,
// which is the shape that pins content in memory for a streaming parser.
func Anchored(n int) string {
	var b strings.Builder
	const entryBytes = 32 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)
	b.WriteString("defaults: &defaults\n  timeout: 30\n  retries: 3\n  verbose: true\nitems:\n")

	for i := range n {
		fmt.Fprintf(&b, "  - name: item%06d\n    <<: *defaults\n", i)
	}

	return b.String()
}

// BlockScalars builds a document of n literal block scalars, each a few lines.
func BlockScalars(n int) string {
	var b strings.Builder
	const entryBytes = 96 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "text%06d: |\n  first line of block %d\n  second line, a little longer\n  third\n", i, i)
	}

	return b.String()
}

// Quoted builds a mapping of n double-quoted values holding nothing to unescape.
//
// The scanner may hand a quoted scalar back as a window on the source when the
// text between the quotes is the value; this is the shape that says what that
// is worth.
func Quoted(n int) string {
	var b strings.Builder
	const entryBytes = 40 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: \"a plain enough value %06d\"\n", i, i)
	}

	return b.String()
}

// Escaped builds a mapping of n double-quoted values carrying escapes, one of
// them naming a code point.
//
// scanDoubleQuote walks these a character at a time and builds the value in a
// buffer, so this is the counterpart to Quoted: the difference between the two
// is what escaping costs.
func Escaped(n int) string {
	var b strings.Builder
	const entryBytes = 56 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: \"a\\tvalue\\nwith \\\"escapes\\\" and \\u00e9 %06d\"\n", i, i)
	}

	return b.String()
}

// Commented builds a mapping of n entries with a comment above each and after
// each, the shape a hand-written configuration file has.
func Commented(n int) string {
	var b strings.Builder
	const entryBytes = 72 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "# what key%06d is for\nkey%06d: value%06d # and why\n", i, i, i)
	}

	return b.String()
}

// Flow builds a mapping of n entries whose values are flow collections, which
// the scanner counts brackets through rather than reading indentation.
func Flow(n int) string {
	var b strings.Builder
	const entryBytes = 56 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: {name: item%06d, tags: [a, b, c], on: true}\n", i, i)
	}

	return b.String()
}

// CRLF is FlatMap written with the line endings a Windows editor leaves, which
// the scanner steps over as one break rather than two.
func CRLF(n int) string {
	return strings.ReplaceAll(FlatMap(n), "\n", "\r\n")
}

// EscapedDense builds a mapping of n double-quoted values that are mostly
// escapes -- eight in a scalar of about forty characters.
//
// A scalar holding one escape is written from the source up to it and rewritten
// after; one holding many is rewritten almost entirely. The two say different
// things about what an escape costs, so both are measured.
func EscapedDense(n int) string {
	var b strings.Builder
	const entryBytes = 64 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: \"a\\tb\\nc\\\"d\\\\e\\tf\\ng\\\"h %06d\"\n", i, i)
	}

	return b.String()
}

// EscapedSparse builds a mapping of n long double-quoted values carrying one
// escape each, near the end.
//
// The value is a window on the source until the escape is read, so this is what
// says whether the lazy copy is reached late or early.
func EscapedSparse(n int) string {
	var b strings.Builder
	const entryBytes = 96 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: \"a long enough run of plain text to be worth windowing\\t%06d\"\n", i, i)
	}

	return b.String()
}

// EscapedUnicode builds a mapping of n double-quoted values holding the escapes
// that name a code point by its digits: \xXX, \uXXXX, \UXXXXXXXX and a UTF-16
// surrogate pair.
//
// These are the ones scanDoubleQuote reads furthest ahead for, a surrogate pair
// settling how far it reaches only once its low half is read.
func EscapedUnicode(n int) string {
	var b strings.Builder
	const entryBytes = 72 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)

	for i := range n {
		fmt.Fprintf(&b, "key%06d: \"\\x41\\u00e9\\U0001F600\\uD83D\\uDE00 %06d\"\n", i, i)
	}

	return b.String()
}

// DeepIndent builds a document nested six levels deep, so that most lines open
// with a run of spaces long enough to be worth stepping over in bulk.
//
// The other shapes are shallow and their indentation runs are one to three
// spaces, which is not what a document people write looks like: over the
// analysis workloads the mean run is 2.4 to 10.8 spaces, and golang_source
// spends 52% of its bytes in runs of eight or more. A scanner change that pays
// only on a long run is invisible to every other shape here.
func DeepIndent(n int) string {
	var b strings.Builder
	const entryBytes = 128 // the bytes one entry writes, rounded up
	b.Grow(n * entryBytes)
	b.WriteString("root:\n  level1:\n    level2:\n      level3:\n        level4:\n          entries:\n")

	for i := range n {
		fmt.Fprintf(&b,
			"            - name: item%06d\n              kind: example\n"+
				"              spec:\n                value: %06d\n                enabled: true\n", i, i)
	}

	return b.String()
}

// Shapes names the document shapes worth measuring separately. The order is
// fixed so that benchmark output lines up run to run.
var Shapes = []struct {
	Name     string
	Generate func(int) string
}{
	{"flat", FlatMap},
	{"nested", NestedDoc},
	{"deepindent", DeepIndent},
	{"anchored", Anchored},
	{"blockscalars", BlockScalars},
}

// Sizes are held small enough that a full benchmark set runs in seconds. The
// super-linear behavior of wide mappings is charted in the analysis module,
// which is free to be slow; this set exists to notice a change, not to draw a
// curve.
var Sizes = []int{100, 1000}

// ScanShapes names the shapes that separate one part of the scanner from
// another. They are kept apart from Shapes so that a number measured against
// Shapes still means what it meant before this list existed.
//
// Shapes asks what a document costs; these ask what a construct costs. Quoted
// against Escaped is the price of an escape, Commented the price of a comment,
// Flow the price of counting brackets instead of columns, and CRLF the price of
// a two-byte line break.
var ScanShapes = []struct {
	Name     string
	Generate func(int) string
}{
	{"quoted", Quoted},
	{"escaped", Escaped},
	{"escaped-dense", EscapedDense},
	{"escaped-sparse", EscapedSparse},
	{"escaped-unicode", EscapedUnicode},
	{"commented", Commented},
	{"flow", Flow},
	{"crlf", CRLF},
}

// ForEachDocument runs fn over every shape in Shapes and every size, naming each
// sub-benchmark and reporting throughput and allocations for it.
func ForEachDocument(b *testing.B, fn func(*testing.B, []byte)) {
	b.Helper()
	forEach(b, Shapes, fn)
}

// ForEachScanDocument runs fn over Shapes and ScanShapes both. It is what the
// scanner's own benchmarks use: they want the document shapes for continuity
// and the construct shapes to tell one change from another.
func ForEachScanDocument(b *testing.B, fn func(*testing.B, []byte)) {
	b.Helper()
	forEach(b, Shapes, fn)
	forEach(b, ScanShapes, fn)
}

func forEach(b *testing.B, shapes []struct {
	Name     string
	Generate func(int) string
}, fn func(*testing.B, []byte),
) {
	b.Helper()

	for _, shape := range shapes {
		for _, size := range Sizes {
			src := []byte(shape.Generate(size))

			b.Run(shape.Name+"-"+strconv.Itoa(size), func(b *testing.B) {
				b.SetBytes(int64(len(src)))
				b.ReportAllocs()
				fn(b, src)
			})
		}
	}
}
