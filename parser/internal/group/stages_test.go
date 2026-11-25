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
