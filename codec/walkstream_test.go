// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type streamDoc struct {
	A int
	B int
}

// streamRead is what one Decode of a stream returned.
type streamRead struct {
	Value streamDoc
	EOF   bool
	Err   bool
}

// readStructStream decodes src document by document into a streamDoc whose B
// the caller set beforehand, until io.EOF or an error.
func readStructStream(src string, opts ...codec.DecodeOption) []streamRead {
	dec := codec.NewDecoder(strings.NewReader(src), opts...)

	var reads []streamRead
	for range 8 {
		v := streamDoc{B: 99}
		err := dec.Decode(&v)
		read := streamRead{Value: v, EOF: errors.Is(err, io.EOF), Err: err != nil && !errors.Is(err, io.EOF)}
		reads = append(reads, read)
		if err != nil {
			break
		}
	}

	return reads
}

// treeOnly turns the walk off without changing the decode, as agDecode does:
// canWalk refuses a decoder holding a custom unmarshaler, and a chan int
// stands nowhere in streamDoc.
func treeOnly() codec.DecodeOption {
	return codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })
}

// TestAStreamDecodesOneDocumentIntoEachStruct holds a Decode loop into a struct to
// the tree: one document per Decode, in order, then io.EOF.
//
// The walk read every document of the stream into the first destination, so
// "a: 1" over "a: 2" came back as {A:2}, and every Decode after it returned a
// zero value and no error, so a loop waiting for io.EOF never ended.
func TestAStreamDecodesOneDocumentIntoEachStruct(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"a: 1\n---\na: 2\n",
		"a: 1\n",
		"",
		"---\n---\na: 2\n",
		"a: 1\n---\nb: 2\n",
		"# only a comment\n",
		"%YAML 1.2\n---\na: 1\n---\na: 2\n",
		// The second document holds an anchor, which the walk hands to the tree.
		"a: 1\n---\na: &x 2\nb: *x\n",
		"a: &x 1\n---\na: 2\n",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			walked := readStructStream(src)
			treed := readStructStream(src, treeOnly())

			assert.Equal(t, treed, walked)
			require.NotEmpty(t, walked)
			assert.True(t, walked[len(walked)-1].EOF, "the stream ends on io.EOF")
		})
	}
}

// TestADocumentKeepsTheDefaultsOfItsDestination checks that each document goes
// into the value its own Decode hands over: a field the document does not write
// keeps what the caller set.
func TestADocumentKeepsTheDefaultsOfItsDestination(t *testing.T) {
	t.Parallel()

	reads := readStructStream("a: 1\n---\na: 2\n")
	require.Len(t, reads, 3)
	assert.Equal(t, streamDoc{A: 1, B: 99}, reads[0].Value)
	assert.Equal(t, streamDoc{A: 2, B: 99}, reads[1].Value)
}

// TestUnmarshalReadsTheFirstDocumentOfAStream checks that Unmarshal fills its
// destination from the first document, and still refuses a stream the parse
// refuses further on, as the tree does.
func TestUnmarshalReadsTheFirstDocumentOfAStream(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"a: 1\n---\na: 2\n", "a: 1\n---\nb: 2\n"} {
		var walked, treed streamDoc
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &treed, treeOnly()))

		assert.Equal(t, streamDoc{A: 1}, walked, "%q", src)
		assert.Equal(t, treed, walked, "%q", src)
	}

	var v streamDoc
	require.Error(t, codec.Unmarshal([]byte("a: 1\n---\nb: [\n"), &v),
		"a refused second document fails the stream")
}

// TestADecoderReportsABrokenDocumentOnItsOwnDecode checks that a Decoder reads a
// document only when its Decode asks for it, as go.yaml.in/yaml/v3 and
// encoding/json do: the documents before a broken one decode.
func TestADecoderReportsABrokenDocumentOnItsOwnDecode(t *testing.T) {
	t.Parallel()

	dec := codec.NewDecoder(strings.NewReader("a: 1\n---\nb: [\n"))

	var first streamDoc
	require.NoError(t, dec.Decode(&first))
	assert.Equal(t, streamDoc{A: 1}, first)

	var second streamDoc
	require.Error(t, dec.Decode(&second))
}

// TestADecoderSwitchesToTheTreeForADestinationTheWalkCannotFill checks that a
// Decoder handed a destination the walk cannot fill, after the walk has read
// some of the stream, carries on from the next document through the tree.
func TestADecoderSwitchesToTheTreeForADestinationTheWalkCannotFill(t *testing.T) {
	t.Parallel()

	dec := codec.NewDecoder(strings.NewReader("a: 1\n---\n1: one\n---\na: 3\n"))

	var first streamDoc
	require.NoError(t, dec.Decode(&first))
	assert.Equal(t, streamDoc{A: 1}, first)

	// A map keyed by integers is not a destination the walk fills.
	var second map[int]string
	require.NoError(t, dec.Decode(&second))
	assert.Equal(t, map[int]string{1: "one"}, second)

	var third streamDoc
	require.NoError(t, dec.Decode(&third))
	assert.Equal(t, streamDoc{A: 3}, third)

	var past streamDoc
	require.ErrorIs(t, dec.Decode(&past), io.EOF)
}

// TestADroppedDecoderReleasesItsWalk checks that a Decoder dropped before the end
// of its stream does not leave its walk paused for ever. The walk runs in a
// coroutine, which counts as a goroutine until it is stopped.
func TestADroppedDecoderReleasesItsWalk(t *testing.T) {
	// Not parallel: it counts the process's goroutines.
	const decoders = 50

	before := runtime.NumGoroutine()
	for range decoders {
		dec := codec.NewDecoder(strings.NewReader("a: 1\n---\na: 2\n"))
		var v streamDoc
		require.NoError(t, dec.Decode(&v))
	}

	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	assert.LessOrEqual(t, runtime.NumGoroutine(), before,
		"%d decoders dropped halfway through their streams", decoders)
}
