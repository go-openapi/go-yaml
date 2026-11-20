// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
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

// TestADecoderKeepsTheErrorThatStoppedItsStream holds every decode path to one
// answer once the source fails to parse: the same error on every Decode after,
// as encoding/json.Decoder gives, and never io.EOF, which a Decode loop takes
// for the stream's clean end.
//
// The walk kept its own parse error. The tree, and a walk that had handed the
// document to the tree before the parse failed, read the drained reader again
// as an empty source and returned io.EOF.
func TestADecoderKeepsTheErrorThatStoppedItsStream(t *testing.T) {
	// The walk cannot set an embedded pointer to an unexported struct, so it
	// hands the decode to the tree at the first key the embedding holds.
	type hidden struct {
		A []int `yaml:"a"`
	}
	type small struct {
		*hidden `yaml:",inline"`
	}
	type walked struct {
		A []int `yaml:"a"`
		B []int `yaml:"b"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	routes := map[string]struct {
		into func() any
		opts []codec.DecodeOption
	}{
		"a walk that falls back first": {into: func() any { return new(small) }},
		"a walk":                       {into: func() any { return new(walked) }},
		"an any":                       {into: func() any { return new(any) }},
		"the tree":                     {into: func() any { return new(walked) }, opts: []codec.DecodeOption{tree}},
	}
	for _, src := range []string{"a: [\n", "a: *missing\n", "a: [1]\n---\nb: [\n"} {
		for name, route := range routes {
			t.Run(name+"/"+src, func(t *testing.T) {
				dec := codec.NewDecoder(strings.NewReader(src), route.opts...)

				var first error
				for range 4 {
					err := dec.Decode(route.into())
					if first == nil {
						first = err

						continue
					}
					require.Error(t, first, "the source does not parse")
					require.NotErrorIs(t, err, io.EOF)
					require.EqualError(t, err, first.Error())
				}
			})
		}
	}
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

// TestATypeErrorFailsOnlyItsOwnDocument holds both decode paths to one answer
// for a document holding a value that does not fit: its Decode returns the
// type error, and the next Decode reads the next document.
//
// The tree decoder left its stream index on the document, so every Decode
// after it returned the same document's error and never reached the end.
func TestATypeErrorFailsOnlyItsOwnDocument(t *testing.T) {
	type doc struct {
		A int    `yaml:"a"`
		B string `yaml:"b"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	for name, opts := range map[string][]codec.DecodeOption{"walk": nil, "tree": {tree}} {
		t.Run(name, func(t *testing.T) {
			dec := codec.NewDecoder(strings.NewReader("a: x\nb: one\n---\na: 2\nb: two\n"), opts...)

			var first doc
			require.ErrorIs(t, dec.Decode(&first), yamlerrors.ErrTypeMismatch)
			assert.Equal(t, doc{B: "one"}, first)

			var second doc
			require.NoError(t, dec.Decode(&second))
			assert.Equal(t, doc{A: 2, B: "two"}, second)

			require.ErrorIs(t, dec.Decode(new(doc)), io.EOF)
		})
	}
}

// TestADocumentErrorFailsOnlyItsOwnDocument holds both decode paths to one
// answer for a document that stops decoding at any depth -- a repeated key, or
// the alias budget spent: its Decode returns the error, and the next Decode
// reads the next document, and no field after the error decodes. The budget covers the whole stream, so once it is
// spent the next document fails at its first node, and the stream still ends.
//
// The walk kept the error for the whole stream, so every Decode after it
// returned it again and never reached io.EOF. The tree took a repeated key
// below the root for a field error and decoded the fields after it.
//
// The parser records a mapping's repeats as it closes, so the walk finds a
// repeat in the root mapping only once every field of it is written.
func TestADocumentErrorFailsOnlyItsOwnDocument(t *testing.T) {
	type inner struct {
		S string `yaml:"s"`
	}
	type doc struct {
		A inner            `yaml:"a"`
		L []inner          `yaml:"l"`
		M map[string]inner `yaml:"m"`
		X any              `yaml:"x"`
		B string           `yaml:"b"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	// Each alias of the chain names the one before nine times: 9^9 values out
	// of a few hundred bytes.
	var laughs strings.Builder
	laughs.WriteString("x:\n  l0: &l0 [lol, lol, lol, lol, lol, lol, lol, lol, lol]\n")
	for i := 1; i < 10; i++ {
		fmt.Fprintf(&laughs, "  l%d: &l%d [*l%d, *l%d, *l%d, *l%d, *l%d, *l%d, *l%d, *l%d, *l%d]\n",
			i, i, i-1, i-1, i-1, i-1, i-1, i-1, i-1, i-1, i-1)
	}

	docs := map[string]struct {
		src   string
		kind  error
		spent bool
	}{
		"a repeat in a struct field":   {src: "a: {s: x, s: y}\nb: after\n", kind: yamlerrors.ErrDuplicateKey},
		"a repeat in the root mapping": {src: "a: {s: x}\na: {s: y}\n", kind: yamlerrors.ErrDuplicateKey},
		"a repeat in a slice element":  {src: "l: [{s: x, s: y}]\nb: after\n", kind: yamlerrors.ErrDuplicateKey},
		"a repeat in a map value":      {src: "m: {k: {s: x, s: y}}\nb: after\n", kind: yamlerrors.ErrDuplicateKey},
		"a repeat in an any":           {src: "x: {s: x, s: y}\nb: after\n", kind: yamlerrors.ErrDuplicateKey},
		"the alias budget":             {src: laughs.String() + "b: after\n", kind: yamlerrors.ErrExcessiveAliasing, spent: true},
	}
	for name, first := range docs {
		for path, opts := range map[string][]codec.DecodeOption{"walk": nil, "tree": {tree}} {
			t.Run(name+"/"+path, func(t *testing.T) {
				dec := codec.NewDecoder(strings.NewReader(first.src+"---\nb: two\n"), opts...)

				var broken doc
				require.ErrorIs(t, dec.Decode(&broken), first.kind)
				assert.Empty(t, broken.B)

				var second doc
				if first.spent {
					require.ErrorIs(t, dec.Decode(&second), yamlerrors.ErrExcessiveAliasing)
				} else {
					require.NoError(t, dec.Decode(&second))
					assert.Equal(t, doc{B: "two"}, second)
				}

				require.ErrorIs(t, dec.Decode(new(doc)), io.EOF)
			})
		}
	}
}
