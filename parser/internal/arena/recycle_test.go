// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package arena_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser/internal/arena"
	"github.com/go-openapi/go-yaml/parser/internal/probe"
)

// TestRecycleTakesBackEveryChunk checks that Recycle poisons every cell handed out, the chunk being filled included,
// and that the cells taken next come from those chunks, zeroed.
func TestRecycleTakesBackEveryChunk(t *testing.T) {
	t.Parallel()

	// Three chunks of two cells, the last still the one being filled. Every cell is taken,
	// so each cell handed out after Recycle is one handed out before it.
	const taken = 6
	a := arena.Run[cell]{Size: 2}

	held := make(map[*cell]bool, taken)
	for seq := range int32(taken) {
		c := a.Take(seq + 1)
		*c = cell{n: int(seq), text: "stale"}
		held[c] = true
	}

	var poisoned int
	a.Recycle(func(cells []cell) { poisoned += len(cells) })
	assert.Equal(t, taken, poisoned)

	for seq := range int32(taken) {
		c := a.Take(seq + 1)
		assert.Equal(t, cell{}, *c)
		if !probe.ReuseReleased {
			assert.Falsef(t, held[c], "the probe build filled a recycled chunk again at take %d", seq)

			continue
		}
		require.Truef(t, held[c], "take %d allocated a new cell while recycled chunks remained", seq)
	}
}
