// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/jsoncorpus"
)

// TestTheGrammarCompiledIntoWhatItSays is the check that runs before the suite
// score, so that a grammar which fails to compile at all says so in one line
// rather than as three hundred disagreements.
func TestTheGrammarCompiledIntoWhatItSays(t *testing.T) {
	if got := jsoncorpus.JSON.Rules(); got == 0 {
		t.Fatal("the grammar compiled to no productions")
	}

	for _, c := range []struct {
		src  string
		want bool
		why  string
	}{
		// The shapes.
		{`{"a":1}`, true, "an object"},
		{`[1,2,3]`, true, "an array"},
		{`"x"`, true, "a lone string, which RFC 8259 admits at the top level"},
		{`42`, true, "a lone number, likewise"},
		{`true`, true, "a lone literal"},
		{` { "k" : [ null ] } `, true, "whitespace wherever the grammar allows it"},

		// The bug this spike is about, and the case beside it that the suite covers.
		{`{"a":}`, false, "a colon followed by a closer"},
		{`{"a":`, false, "a colon followed by nothing"},

		// Places a hand-rolled parser tends to be generous.
		{`[1,]`, false, "a trailing comma"},
		{`01`, false, "a leading zero"},
		{`nul`, false, "a truncated literal"},
		{`"\q"`, false, "an escape the grammar does not define"},
		{"\"\t\"", false, "a raw control character in a string"},
		{`-0.5e+10`, true, "every optional part of a number at once"},
	} {
		if got := jsoncorpus.Text([]byte(c.src)).OK; got != c.want {
			t.Errorf("%-12q want %v, got %v -- %s", c.src, c.want, got, c.why)
		}
	}
}
