// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// stressSources are the documents built to strain what a token store that
// reclaims behind the parse finds hardest. Each is named for what it strains.
//
// The five workloads read like documents people write, which is what makes them
// the right thing to measure ordinary cost against -- and the wrong thing to
// size a reclaiming store on. Not one of them holds an anchor, a flow
// collection or a comment, and the widest level in any of them is 1,789
// entries against 293,142 tokens. They would make any store look good.
var stressSources = map[string]func() string{
	"flow_wide":         flowWide,
	"flow_nested":       flowNested,
	"flow_long_scalars": flowLongScalars,
	"anchors_far":       anchorsFar,
	"anchors_many":      anchorsMany,
	"anchors_nested":    anchorsNested,
	"map_wide":          mapWide,
	"comments_dense":    commentsDense,
}

// stress writes the stress documents into dir.
func stress(dir string) error {
	for name, build := range stressSources {
		src := []byte(build())
		if err := writeGzip(filepath.Join(dir, name+".yaml.gz"), src); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}

		fmt.Fprintf(os.Stdout, "%-18s %8d bytes, %d lines\n", name, len(src), strings.Count(string(src), "\n"))
	}

	return nil
}

// flowWide is one flow sequence and nothing else.
//
// Its tokens run to tens of thousands and the collection does not close until
// the last of them, so a store reclaiming behind the parse has nothing to
// reclaim until the document ends. One collection, many blocks.
func flowWide() string {
	const members = 30000

	var b strings.Builder
	b.Grow(members * 10)
	b.WriteString("wide: [")

	for i := range members {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "v%06d", i)
	}
	b.WriteString("]\n")

	return b.String()
}

// flowNested nests flow collections deeper than a hand-written document ever
// goes.
//
// Each open level holds a token the parse cannot let go of until that level
// closes, so what stands is the depth rather than the width. It is also the
// deepest the descent's stack of token references is asked to grow.
func flowNested() string {
	const depth, entries = 120, 300

	opening, closing := strings.Repeat("[", depth), strings.Repeat("]", depth)

	var b strings.Builder
	for i := range entries {
		fmt.Fprintf(&b, "k%04d: %s%d%s\n", i, opening, i, closing)
	}

	return b.String()
}

// flowLongScalars is a flow sequence of scalars that are long rather than many.
//
// A store sized in tokens is blind to this: the document is 500 kB in a few
// thousand tokens, and every one of them points at a scalar running the width
// of the line it was read from.
func flowLongScalars() string {
	const members, width = 1200, 380

	filler := strings.Repeat("long scalar text ", 1+width/17)[:width]

	var b strings.Builder
	b.WriteString("scalars: [")
	for i := range members {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", fmt.Sprintf("%04d %s", i, filler))
	}
	b.WriteString("]\n")

	return b.String()
}

// anchorsFar defines an anchor in the first entry and refers to it in the last.
//
// An anchored subtree has to outlive the document that refers to it, so the
// distance between the two is how long the store has to hold what the anchor
// covers. Here that distance is the whole document.
func anchorsFar() string {
	const entries = 8000

	var b strings.Builder
	b.WriteString("base: &base\n  name: shared\n  value: 1\n")
	for i := range entries {
		fmt.Fprintf(&b, "k%05d:\n  a: %d\n  b: text%05d\n", i, i, i)
	}
	b.WriteString("tail: *base\n")

	return b.String()
}

// anchorsMany spreads anchors evenly through a document, each referred to just
// after it is defined.
//
// Every anchor marks the block it falls in, so a store that keeps a whole block
// because one anchor sits in it keeps the whole document. The alias arriving
// straight away is the friendly case: nothing has to be held for long, and a
// store that holds it anyway has the wrong rule.
func anchorsMany() string {
	const anchors = 4000

	var b strings.Builder
	for i := range anchors {
		fmt.Fprintf(&b, "a%05d: &n%05d\n  v: %d\n  t: text%05d\n", i, i, i, i)
		fmt.Fprintf(&b, "r%05d: *n%05d\n", i, i)
	}

	return b.String()
}

// anchorsNested puts anchors inside anchored subtrees, four deep, and refers to
// the inner ones both inside the outer subtree and outside it.
//
// A store marking blocks with a flag cannot read this: the marks nest, so what
// it needs is a count of the anchors still open, and a block falling inside two
// of them has to survive both.
func anchorsNested() string {
	const groups = 800

	var b strings.Builder
	for i := range groups {
		fmt.Fprintf(&b, `g%04d: &o%04d
  i: &i%04d
    d: &d%04d [%d, %d]
    rd: *d%04d
  ri: *i%04d
r%04d: *o%04d
`, i, i, i, i, i, i+1, i, i, i, i)
	}

	return b.String()
}

// mapWide is one mapping of fifty thousand keys.
//
// The parse gathers a level's entries before it builds the node above them, so
// the widest level of a document is what stands at once. No workload written by
// a person is this wide; a generated one is.
func mapWide() string {
	const keys = 50000

	var b strings.Builder
	const bytesPerKey = len("key000000: value000000\n")
	b.Grow(keys * bytesPerKey)
	for i := range keys {
		fmt.Fprintf(&b, "key%06d: value%06d\n", i, i)
	}

	return b.String()
}

// commentsDense puts long runs of comment lines between entries.
//
// The descent looks past a comment to the token after it, and the look is not
// bounded: a run of five hundred comments is read through to find what follows
// and every one of them stands while it is read.
func commentsDense() string {
	const blocks, lines = 60, 300

	var b strings.Builder
	for i := range blocks {
		fmt.Fprintf(&b, "k%04d: %d\n", i, i)
		for j := range lines {
			fmt.Fprintf(&b, "# a comment standing between two entries, number %d of block %d\n", j, i)
		}
	}
	b.WriteString("last: done\n")

	return b.String()
}
