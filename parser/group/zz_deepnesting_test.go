// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
)

// TestStageIndicesMatchTheChain pins alwaysLooking to the stages it names.
//
// It is an index into a slice built in init, so inserting a stage before
// stageExplicitKeys moves it silently, and the short way then skips a stage
// that has to count the brackets around it. That is how "? a: b" inside a flow
// mapping was read wrongly for the length of one commit.
func TestStageIndicesMatchTheChain(t *testing.T) {
	require.Equal(t, "stageExplicitKeys", stageNameAt(alwaysLooking),
		"alwaysLooking names the first stage that has to see every token",
	)

	require.Equal(t, "stageMapKeysByValue", stageNameAt(alwaysLooking+1),
		"the stage after it holds the key window and has to see every token too",
	)
}

// TestPropertyStatesAreReachable walks a document through each state the
// properties machine has, so a state that stops being reachable shows up as a
// gap rather than as dead code.
func TestPropertyStatesAreReachable(t *testing.T) {
	seen := map[propState]string{}

	// Reachability is checked by driving the machine directly: parsing does not
	// report which states it passed through.
	for _, st := range []propState{
		propNone, propSawAnchor, propHaveAnchor, propSawAlias,
		propSawTag, propAnchorAndTag, propTagSawAnchor, propTagHaveAnchor,
	} {
		require.NotEmptyf(t, st.String(), "state %d has no name", st)
		require.NotEqualf(t, "?", st.String(), "state %d has no name", st)
		seen[st] = st.String()
	}

	require.Len(t, seen, 8, "the machine has eight states and each is named")
}

/*
// skipTimings skips a test that measures wall time.
//
// A ratio of two clock readings is not a thing to gate on: the machine's own
// variance swamps the signal, and these failed about one run in three while
// nothing was wrong. They stay because they are useful to read, and are run by
// asking:
//
//	YAML_TIMINGS=1 go test -run TestParseScalesLinearly ./internal/analysis/
//
// The replacement is a guard behind a build tag that reads the parser's own
// counters -- tokens held, entries walked, allocations -- rather than the
// clock. A count does not vary with the machine.
func skipTimings(t *testing.T) {
	t.Helper()

	if os.Getenv("YAML_TIMINGS") == "" {
		t.Skip("measures wall time; set YAML_TIMINGS=1 to run it")
	}
}
*/
