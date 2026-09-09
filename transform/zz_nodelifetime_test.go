// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"io"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/transform"
)

// TestANodeStillPointsWhereTheWalkNamedIt reads Piece.Node.GetToken() for every
// piece the parse opened a node on, and asks whether the node is still the one
// the walk named.
//
// The parse reclaims a node's cells behind the descent, so a piece written after
// its node has gone carries whichever node was built over it. That is not a
// crash and not a nil: it is another part of the document, read as this one.
// The walk labeled the string 'a: b: c' and the piece arrived carrying an anchor
// name 23 bytes further on.
//
// 610 of 88,473 labeled pieces failed this before Leave began writing a node's
// own piece out and labelPart began writing a part's from inside the parent's
// handover. Both changes are in walk.go and this is what holds them: a piece
// whose token has moved is a piece written too late.
func TestANodeStillPointsWhereTheWalkNamedIt(t *testing.T) {
	t.Parallel()

	var labeled, moved, missing int
	for _, src := range corpusSources(t) {
		err := transform.Walk(io.Discard, []byte(src.text), transform.Func(func(_ io.Writer, p transform.Piece) error {
			if p.Node == nil {
				return nil
			}
			labeled++

			tk := p.Node.GetToken()
			switch {
			case tk == nil:
				missing++
				t.Errorf("%s: the node on the piece at %d has no token", src.name, p.At)
			case int(tk.Position.Offset()) != p.At:
				moved++
				t.Errorf("%s: the piece at %d carries a %T now standing at %d, with the text %q",
					src.name, p.At, p.Node, tk.Position.Offset(), tk.Value)
			}

			return nil
		}))
		if err != nil {
			continue
		}
	}

	t.Logf("read %d labeled pieces, %d moved, %d without a token", labeled, moved, missing)
	require.Positive(t, labeled)
}
