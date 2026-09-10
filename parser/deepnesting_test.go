// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
)

func TestNestingParse(t *testing.T) {
	for _, src := range []string{
		"a: 1\n",           // propNone alone
		"a: &x 1\n",        // propSawAnchor, propHaveAnchor
		"a: &x 1\nb: *x\n", // propSawAlias
		"a: !!str 1\n",     // propSawTag
		"a: &x !!str 1\n",  // propAnchorAndTag
		"a: !!str &x 1\n",  // propTagSawAnchor, propTagHaveAnchor
		"a: &x\nb: 1\n",    // an anchor naming the empty node
		"!!map\na: 1\n",    // a tag alone on its line
	} {
		p := New()
		_, err := p.Parse([]byte(src)) // TODO: this test is just a smoke test and nothing is really asserted
		require.NoError(t, err)
	}
}
