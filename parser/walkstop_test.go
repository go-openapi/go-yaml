// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"errors"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// stopSpy answers each node with what answers[node type] holds, and records every handover.
type stopSpy struct {
	// on names the node type to answer at, and answer what Enter or Leave returns there.
	on     ast.NodeType
	answer error
	// atLeave sends the answer from Leave instead of Enter.
	atLeave bool

	// log holds every handover in order, as "enter Mapping" or "leave Integer",
	// so a test reads both what arrived and what arrived after the answer.
	log []string
	// answered marks where in log the answer was given.
	answered int
}

func (s *stopSpy) Enter(node ast.Node, _ parser.Cursor) error {
	s.log = append(s.log, "enter "+node.Type().String())
	if !s.atLeave && node.Type() == s.on {
		s.answered = len(s.log)

		return s.answer
	}

	return nil
}

func (s *stopSpy) Leave(node ast.Node, _ parser.Cursor) error {
	s.log = append(s.log, "leave "+node.Type().String())
	if s.atLeave && node.Type() == s.on {
		s.answered = len(s.log)

		return s.answer
	}

	return nil
}

// after returns the handovers that arrived once the spy had answered.
func (s *stopSpy) after() []string { return s.log[s.answered:] }

// errRefused is a visitor's own complaint, which is not the parse's.
var errRefused = errors.New("this visitor has read enough")

// TestAVisitorStopsTheWalk checks what Enter and Leave can tell the walk.
//
// A visitor had no way to report an error: Enter returned a bool and Leave returned nothing, so every consumer
// kept an error field and a flag of its own and re-tested them at the top of both methods, and the walk read on.
// walkState.err was read by Walk and written by nothing.
func TestAVisitorStopsTheWalk(t *testing.T) {
	t.Parallel()

	// Two entries, so a stop at the first is visible in what the second never receives.
	const src = "a:\n  b: 1\nc:\n  d: 2\n"

	t.Run("a visitor that answers nil sees the rest of the document", func(t *testing.T) {
		t.Parallel()

		// The control. Every case below asserts that after() is empty, which says nothing
		// unless the same spy answering nil has something in it.
		s := &stopSpy{on: ast.IntegerType}
		_, err := parser.New().Walk([]byte(src), s)

		require.NoError(t, err)
		assert.NotEmpty(t, s.after(), "the first integer is not the last handover of this document")
		assert.Equal(t, 2, countOf(s.log, "enter Integer"), "both integers arrive")
	})

	t.Run("an error from Enter stops the walk and reaches the caller", func(t *testing.T) {
		t.Parallel()

		s := &stopSpy{on: ast.IntegerType, answer: errRefused}
		_, err := parser.New().Walk([]byte(src), s)

		require.ErrorIs(t, err, errRefused)
		assert.Empty(t, s.after(), "nothing is handed over once Enter has failed")
	})

	t.Run("an error from Leave stops the walk", func(t *testing.T) {
		t.Parallel()

		s := &stopSpy{on: ast.IntegerType, answer: errRefused, atLeave: true}
		_, err := parser.New().Walk([]byte(src), s)

		require.ErrorIs(t, err, errRefused)
		assert.Empty(t, s.after(), "nothing is handed over once Leave has failed")
	})

	t.Run("StopWalk stops the walk with no error", func(t *testing.T) {
		t.Parallel()

		s := &stopSpy{on: ast.IntegerType, answer: parser.StopWalk}
		file, err := parser.New().Walk([]byte(src), s)

		require.NoError(t, err)
		require.NotNil(t, file, "the file still comes back, holding the documents without their bodies")
		assert.Empty(t, s.after(), "not even the Leave of a node the walk had open")
	})

	t.Run("SkipNode drops the content and the node's own Leave", func(t *testing.T) {
		t.Parallel()

		// The sequence is skipped and the entry after it still arrives, which tells SkipNode from a stop.
		s := &stopSpy{on: ast.SequenceType, answer: parser.SkipNode}
		_, err := parser.New().Walk([]byte("a: [1, 2]\nb: 3\n"), s)

		require.NoError(t, err)
		assert.NotContains(t, s.log, "leave Sequence", "Leave is not called for a node Enter skipped")
		assert.Equal(t, 1, countOf(s.log, "enter Integer"), "3 arrives; the 1 and 2 inside the sequence do not")
		assert.Contains(t, s.after(), "leave Mapping", "the walk carries on past the skipped node")
	})

	t.Run("the parse's refusal wins over a visitor's error", func(t *testing.T) {
		t.Parallel()

		// The visitor gives up on the first scalar, and the document is invalid further on.
		// A caller is told why the document was refused, not that the visitor stopped reading it.
		s := &stopSpy{on: ast.IntegerType, answer: errRefused}
		_, err := parser.New().Walk([]byte("a: 1\n- b\n"), s)

		require.Error(t, err)
		assert.NotErrorIs(t, err, errRefused)
	})
}

func countOf(all []string, want string) int {
	var n int
	for _, s := range all {
		if s == want {
			n++
		}
	}

	return n
}
