// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

var (
	zero     = 0
	emptyStr = ""
)

type TestTextMarshaler string

func (t TestTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(t), nil
}

type TestTextUnmarshalerContainer struct {
	V TestTextMarshaler
}

type unexportedStruct struct{}

// encodeCase is a value, the options it is encoded with, and the document the encoder writes for it.
type encodeCase struct {
	name    string
	source  string
	value   any
	options []codec.EncodeOption
}

// TestEncoder encodes the value of each case and compares the document with the case's source.
//
// Each case is named "theme/what", so go test -run 'TestEncoder/omitzero-tag/' runs one theme.
func TestEncoder(t *testing.T) {
	seen := make(map[string]bool)
	for tc := range encoderCases() {
		require.Falsef(t, seen[tc.name], "two cases are named %q", tc.name)
		seen[tc.name] = true

		t.Run(tc.name, testEncode(tc))
	}
}

func testEncode(tc encodeCase) func(*testing.T) {
	return func(t *testing.T) {
		var buf bytes.Buffer
		enc := codec.NewEncoder(&buf, tc.options...)
		require.NoError(t, enc.Encode(tc.value))
		require.Equal(t, tc.source, buf.String())
	}
}

func encoderCases() iter.Seq[encodeCase] {
	return concatCases(
		encodeScalarCases(),
		encodeStringCases(),
		encodeMultiLineCases(),
		encodeCollectionCases(),
		encodeStructCases(),
		encodeOmitEmptyTagCases(),
		encodeOmitZeroTagCases(),
		encodeOmitOptionCases(),
		encodeFlowCases(),
		encodeMarshalerCases(),
	)
}

// concatCases runs the cases of each table in turn.
func concatCases[T any](tables ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, table := range tables {
			for tc := range table {
				if !yield(tc) {
					return
				}
			}
		}
	}
}

func TestEncoder_MultipleDocuments(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	require.NoError(t, enc.Encode(1))
	require.NoError(t, enc.Encode(2))
	assert.Equal(t, "1\n---\n2\n", buf.String())
}

// errWriterFull is the error failOnceWriter returns for the write that does not fit.
var errWriterFull = errors.New("writer full")

// failOnceWriter takes room bytes and returns errWriterFull for the write that goes past them.
// It then takes every later write, so an error dropped at that write does not resurface at the next one.
type failOnceWriter struct {
	room   int
	failed bool
}

func (w *failOnceWriter) Write(p []byte) (int, error) {
	if !w.failed && len(p) > w.room {
		w.failed = true

		return w.room, errWriterFull
	}
	w.room -= len(p)

	return len(p), nil
}

// TestEncodeReturnsTheWriterError fails one write at each of the three points where Encode writes, and checks
// that Encode returns the writer's error each time.
func TestEncodeReturnsTheWriterError(t *testing.T) {
	var one bytes.Buffer
	require.NoError(t, codec.NewEncoder(&one).Encode(map[string]int{"a": 1}))
	document := one.Len()

	for tc := range writerErrorCases(document) {
		t.Run(tc.name, func(t *testing.T) {
			enc := codec.NewEncoder(&failOnceWriter{room: tc.room})
			var err error
			for range tc.documents {
				if err = enc.Encode(map[string]int{"a": 1}); err != nil {
					break
				}
			}

			require.ErrorIs(t, err, errWriterFull)
		})
	}
}

// writerErrorCase holds how many bytes the writer takes, and how many documents are encoded to fill it.
type writerErrorCase struct {
	name      string
	room      int
	documents int
}

func writerErrorCases(document int) iter.Seq[writerErrorCase] {
	return slices.Values([]writerErrorCase{
		{name: "in the document", room: 0, documents: 1},
		{name: "in the newline after the document", room: document - 1, documents: 1},
		{name: "in the separator before the second document", room: document, documents: 2},
	})
}
