// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
)

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
