// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package group

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/go-openapi/go-yaml/internal/probe"
)

// deadSeq and deadGroup stamp a cell the grouper has handed back.
//
// The stamp goes in a field the cell already has rather than in a set beside
// it: a map lookup on every accessor, under the lock the parallel gates need,
// took the workloads past ten minutes. It is also chosen not to destroy the
// cell -- raw and Group stay as they were -- because a cleared cell makes the
// descent fall over on a nil group somewhere below rather than reporting the
// read, and then one pass finds one violation instead of all of them.
//
// No token the scanner reads has a negative sequence, and no group is built
// with this type.
const (
	deadSeq   int32          = -0x0BAD
	deadGroup TokenGroupType = 0xFF
)

// readerOf names the parser frames that led to a read of a released cell. Only
// the first few failures of an invariant are described, so this is paid for a
// handful of times.
func readerOf() string {
	var pcs [12]uintptr
	n := runtime.Callers(4, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var b strings.Builder
	for {
		f, more := frames.Next()
		if strings.Contains(f.Function, "go-yaml/parser.") && !strings.Contains(f.Function, "checkLive") {
			fmt.Fprintf(&b, "        %s:%d\n", f.Function, f.Line)
		}
		if !more {
			break
		}
	}

	return b.String()
}

func poisonLeaves(cells []TapeToken) {
	probe.Count("grouper.leaf.released", int64(len(cells)))
	for i := range cells {
		cells[i].seq = deadSeq
	}
}

func poisonGroups(cells []TokenGroup) {
	probe.Count("grouper.released", int64(len(cells)))
	for i := range cells {
		cells[i].Type = deadGroup
	}
}

// reviveLeaf and reviveGroup do nothing: take zeroes the cell it hands out, so
// a cell in use again carries no stamp.
func reviveLeaf(_ *TapeToken) {}

func reviveGroup(_ *TokenGroup) {}

// checkLive records that t was read after the grouper handed its cell back. at
// names the accessor, so the report says what the read was after.
func (t *TapeToken) checkLive(at string) {
	if t == nil || t.seq != deadSeq {
		return
	}
	probe.Check("grouper.leaf.live", false, func() string {
		return fmt.Sprintf("%s read a leaf the grouper had taken back, from\n%s", at, readerOf())
	})
}

func (g *TokenGroup) checkLive(at string) {
	if g == nil || g.Type != deadGroup {
		return
	}
	probe.Check("grouper.live", false, func() string {
		var keyed int32
		if g.a != nil {
			keyed = g.a.seq
		}

		return fmt.Sprintf("%s read a group of %d keyed at %d, from\n%s", at, g.n, keyed, readerOf())
	})
}
