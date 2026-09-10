// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// TestAStreamReadsBackAsItsDocuments is presentation invariance for a stream:
// several documents written into one text come back as the same values, in the
// same order, however the stream is spelled.
//
// One property rather than the six a single document gets. What a stream adds
// over a document is the separator and the scope of what a document declares,
// and both are answered here: a "---" against a "..." is the whole of
// Style.DocumentSuffix, and an anchor or a %TAG handle reaching past its own
// document would show up as a refusal rather than as a wrong value. The
// per-document properties are already held by the tests beside this one, since
// Streams draws each document from Values.
func TestAStreamReadsBackAsItsDocuments(t *testing.T) {
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		docs := yamlgen.Streams().Draw(rt, "stream")
		style := yamlgen.Styles().Draw(rt, "style")

		// A stream is excused by the entry any of its documents matches: the
		// documents share one text, so one that will not read takes the whole
		// stream with it and there is nothing left to compare.
		//
		// Parses and Decode are consulted beside StreamDecode because a
		// document the library refuses on its own is refused here too, and a
		// stream is the only place StreamDecode's own entries can go wrong.
		const applies = yamlgen.Parses | yamlgen.Decode | yamlgen.StreamDecode

		for _, v := range docs {
			if known := yamlgen.Known(applies, v, style); known != nil {
				tally.record(known.Name, true)

				return
			}
		}

		w := yamlgen.WriteStream(docs, style)

		if w.MeansUnclear {
			// The generator states no meaning for this stream, so there is
			// nothing to hold the library to -- and nothing to read, where a
			// document keys on a collection and denotes no Go value at all.
			// Checked before the read for that reason. See Written.
			return
		}

		got, err := readStream(w.Text)
		if err != nil {
			rt.Fatalf("%s: the stream does not read:\n%s\n---\n%v", style, w.Text, err)
		}

		want, isSlice := w.Means.([]any)
		if !isSlice {
			rt.Fatalf("WriteStream did not report a document per value: %#v", w.Means)
		}

		if len(got) != len(want) {
			rt.Fatalf("%s: %d documents written, %d read back:\n%s\n---\n%#v",
				style, len(want), len(got), w.Text, got)
		}

		for i := range want {
			if !sameValue(want[i], got[i]) {
				rt.Fatalf("%s: document %d read back differently:\n%s\n---\nexpected: %#v\ngot:      %#v",
					style, i, w.Text, want[i], got[i])
			}
		}
	})

	tally.report(t, yamlgen.Parses|yamlgen.Decode|yamlgen.StreamDecode)
}

// readStream reads every document of src, which is what a caller looping over
// codec.Decoder gets.
func readStream(src string) ([]any, error) {
	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var out []any

	for {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}

		out = append(out, v)
	}
}
