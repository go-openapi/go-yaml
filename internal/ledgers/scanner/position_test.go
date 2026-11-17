// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/go-yaml/internal/ledgers"
)

// positionDefects counts the two ways a token position can be malformed.
type positionDefects struct {
	// zeroColumns counts tokens whose Column is below 1. Columns are 1-based everywhere else, so 0 is out of range
	// rather than merely surprising.
	zeroColumns int
	// backwards counts tokens positioned before the token that precedes them.
	backwards int
}

func (d positionDefects) String() string {
	return fmt.Sprintf("{zeroColumns:%d, backwards:%d}", d.zeroColumns, d.backwards)
}

func (d positionDefects) empty() bool { return d == positionDefects{} }

// positionLedger records position defects the scanner is known to produce, per YAML Test Suite case.
//
// It is empty, and the two invariants it guards hold over the whole suite:
//
//   - No token reports a column below 1. Columns count from 1, so 0 addresses nothing.
//   - No token is positioned before the token in front of it.
//
// It held twelve entries until 2026-09-06, every one a block scalar whose content token carried column 0. They were
// one defect at one site, not twelve cases: Scanner.bufferedToken worked the position out again for a block a dedent
// had ended, where the block had already recorded it. See Scanner.multiLinePosition.
//
// An entry added here has to carry a comment saying why the defect stands, and "not fixed yet" is not one.
var positionLedger = map[string]positionDefects{} //nolint:gochecknoglobals // an immutable map is fine as a global

// TestPositionLedger measures token positions over the whole YAML Test Suite, and holds the known defects to their
// recorded extent.
func TestPositionLedger(t *testing.T) {
	measured := make(map[string]positionDefects)

	for test := range yamlTests(t) {
		var (
			defects           positionDefects
			prevLine, prevCol int32
		)

		for _, tk := range scanAll(t, string(test.InYAML)) {
			pos := tk.Position

			if pos.Column < 1 {
				defects.zeroColumns++
			}
			if pos.Line < prevLine || (pos.Line == prevLine && pos.Column < prevCol) {
				defects.backwards++
			}
			prevLine, prevCol = pos.Line, pos.Column
		}

		if !defects.empty() {
			measured[test.Name] = defects
		}
	}

	ledgers.Compare(t, "position defects", measured, positionLedger)
}
