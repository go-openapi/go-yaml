// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestATabIsSeparationAndNotIndentation holds one rule to one answer.
//
// s-indent(n) is spaces and nothing else, so a tab among a block entry's
// indentation leaves the entry with nothing to sit on. A tab is separation, and
// is admitted wherever separation is: in front of a flow node, after a ':', and
// inside a flow collection.
//
// Three scanner sites asked this and answered it three ways. Two cut the origin
// with strings.TrimPrefix(origin, " ") and tested the next byte for a tab,
// which trims one space -- so " \ta: 1" and "  \ta: 1" drew different messages
// for one fault -- and the third read Scanner.indentHasTab. The origin form
// also missed a quoted key, which resets the buffer, and knew nothing about
// flow mode, so it refused "{\ta: 1}" that every oracle reads.
//
// Scanner.tabStandsWhereAnEntryNeedsIndent is the one question now, over the
// two runs that can hold the tab: the line's own indentation, and the
// separation since a token already cut on this line.
func TestATabIsSeparationAndNotIndentation(t *testing.T) {
	t.Run("a block entry is refused, whatever stands in front of the tab", func(t *testing.T) {
		// Every one of these drew "tab character cannot use as a map key
		// directly" or "...cannot stand for the indentation..." depending on
		// the space count and on whether the key was quoted.
		for _, src := range []string{
			"\ta: 1\n",
			" \ta: 1\n",
			"  \ta: 1\n",
			"   \ta: 1\n",
			"\t\"a\": 1\n",
			" \t\"a\": 1\n",
			"a:\n \tb: 1\n",
			"a:\n  \tb: 1\n",

			// The tab is in the separation the '-' left, not in the line's
			// indentation, which is the second run.
			"- \ta: 1\n",
			"-\ta: 1\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(),
				"tab character cannot stand for the indentation a mapping entry needs", "%q", src)
		}
	})

	t.Run("and a sequence entry the same way", func(t *testing.T) {
		for _, src := range []string{"\t- 1\n", " \t- 1\n", "  \t- 1\n", "- \t- 1\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "tab character cannot use as a sequence delimiter", "%q", src)
		}
	})

	t.Run("a flow collection admits it, and used to be refused", func(t *testing.T) {
		// grammar.NewRecognizer, go.yaml.in/yaml/v3 v3.0.5 and libfyaml
		// 1.0.0b1 all read these three. The retired check had no flow test.
		for _, src := range []string{
			"{\ta: 1}\n",
			"{ \ta: 1}\n",
			"{a: 1,\tb: 2}\n",
			"{a:\t1}\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("and separation elsewhere was always allowed", func(t *testing.T) {
		for _, src := range []string{"\t{}\n", "\t{a: 1}\n", "[\t1]\n", "[a,\tb]\n", "a: \tb\n", "a:\t1\n"} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}
