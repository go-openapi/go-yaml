// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// treeDecoder returns a Decoder that reads src by building a tree.
//
// Decoding into an any takes the walking path, so the side the walk is compared
// against has to be pushed off it. Registering an unmarshaler for chan int does
// that and nothing else: canWalk refuses a Decoder carrying custom
// unmarshalers, no document decodes into a channel, and the parse is left
// alone. CommentToMap would also keep the tree, but reading the comments
// changes which runs of text count as documents.
func treeDecoder(src string) *Decoder {
	return NewDecoder(
		bytes.NewReader([]byte(src)),
		treeOnly(),
	)
}

// decodedStream is every document Decode hands back from a tree, which the walk
// has to reproduce for it to stand in.
func decodedStream(src string) ([]any, error) {
	dec := treeDecoder(src)
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
}

// walkFirst reads the first document of src by walking it, and nil for an empty
// stream.
func walkFirst(src []byte) (any, error) {
	var v any
	err := Unmarshal(src, &v)

	return v, err
}

// walkedStream is every document a Decoder hands back by walking src, which
// TestWalkMatchesTheStream holds to decodedStream.
func walkedStream(src string) ([]any, error) {
	dec := NewDecoder(bytes.NewReader([]byte(src)))
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
}

// TestReferenceStreamBuildsATree holds treeDecoder to its name. Without it a
// change to canWalk would leave TestWalkMatchesTheStream comparing the walk
// against itself, and passing.
func TestReferenceStreamBuildsATree(t *testing.T) {
	dec := treeDecoder("a: 1\n")

	var v any
	require.NoError(t, dec.Decode(&v))
	assert.Nil(t, dec.stream, "the reference decoder walked the source")
	assert.NotNil(t, dec.parsedFile, "the reference decoder built no tree")
}

// corpusSource is one document of the corpus the codec is swept over.
type corpusSource struct{ name, text string }

// corpusSources returns the YAML test suite and the fuzz seeds together.
func corpusSources() []corpusSource {
	var srcs []corpusSource
	suites, _ := yamltestsuite.TestSuites()
	for _, s := range suites {
		srcs = append(srcs, corpusSource{"suite/" + s.Name, string(s.InYAML)})
	}
	seeds, _ := fuzzseeds.All()
	for i, s := range seeds {
		srcs = append(srcs, corpusSource{fmt.Sprintf("seed/%04d", i), s})
	}

	return srcs
}

func TestWalkMatchesTheStream(t *testing.T) {
	srcs := corpusSources()

	var same, bothErr, oneErr int
	differing := make(map[string]string)
	for _, src := range srcs {
		want, wantErr := decodedStream(src.text)
		got, gotErr := walkedStream(src.text)

		switch {
		case wantErr != nil && gotErr != nil:
			bothErr++
		case wantErr != nil || gotErr != nil:
			oneErr++
			if oneErr <= 5 {
				t.Logf("%s: stream err=%v walk err=%v src=%q", src.name, wantErr, gotErr, src.text)
			}
		case sameDecoded(want, got):
			same++
		default:
			differing[src.text] = fmt.Sprintf("%s: the tree decodes %#v, the walk %#v", src.name, want, got)
		}
	}
	t.Logf("same=%d differ=%d bothErr=%d oneErr=%d", same, len(differing), bothErr, oneErr)
	ledgers.CompareNames(t, "walk and tree difference", differing, walkStreamHoldOuts)
	assert.Zero(t, oneErr, "documents only one of the walk and the tree refuses")
}

// sameDecoded reports whether the walk and the tree decoded a document to the same values.
//
// The printed form compares most values, and a NaN among them, which equals nothing under reflect.DeepEqual. It
// prints a *big.Int as its address, though, so an integer too wide for a uint64 differed between two decodes that
// read the same number: five seeds holding a 31-digit integer under an anchored "!!omap" were counted as differing.
// reflect.DeepEqual follows the pointer and compares the numbers.
func sameDecoded(want, got any) bool {
	return fmt.Sprintf("%#v", want) == fmt.Sprintf("%#v", got) || reflect.DeepEqual(want, got)
}

// TestStreamSwitchesFromWalkToTree covers a Decoder handed an any for one
// document of a stream and a map keyed by integers for the next. The first
// decode read the source by walking it, and the second needs the tree the walk
// never built: the walk fills no map with keys other than strings.
func TestStreamSwitchesFromWalkToTree(t *testing.T) {
	const src = "a: 1\n---\n2: two\n---\nc: 3\n"

	dec := NewDecoder(bytes.NewReader([]byte(src)))

	var first any
	require.NoError(t, dec.Decode(&first))
	require.NotNil(t, dec.stream, "the first document was not walked")
	assert.Equal(t, "map[a:1]", fmt.Sprintf("%v", first), "first document")

	// A RawMessage holds its source text, which the walk drops, so it goes to the tree.
	var second RawMessage
	require.NoError(t, dec.Decode(&second))
	assert.Nil(t, dec.stream, "the Decoder stayed on the walking path")
	assert.Contains(t, string(second), "two", "second document")

	// The stream carries on from where the walk left it rather than starting
	// again: the tree is read from the same source and indexed the same way.
	var third any
	require.NoError(t, dec.Decode(&third))
	assert.Equal(t, "map[c:3]", fmt.Sprintf("%v", third), "third document")

	var extra any
	assert.ErrorIs(t, dec.Decode(&extra), io.EOF, "after the last document")
}

// walkStreamHoldOuts are the documents the walk and the tree disagree about,
// with the defect that explains each.
//
// Keyed on the source text and not on the corpus name, because a regeneration
// renames every seed. TestWalkMatchesTheStream checks it with
// [ledgers.CompareNames]: a document that starts differing fails, and a
// held-out document that starts agreeing fails too, so the entry reports the fix
// instead of outliving it.
//
// Empty since defect 109 closed: the walk kept a key's text where the tree kept
// what it resolved to, and the two now build the same mapping. Kept for the
// next entry, which should not have to write the mechanism again.
var walkStreamHoldOuts = map[string]string{}
