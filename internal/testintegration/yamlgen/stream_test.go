// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/go-yaml/codec"
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
			if known := yamlgen.Known(ledger, applies, v, style); known != nil {
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

// TestAStreamDecodesIntoAGoTypePerDocument reads a stream document by document,
// each into a Go type of its own, and holds the walk to a Decoder that reads the
// same stream through a tree.
//
// Neither TestAStreamReadsBackAsItsDocuments nor
// TestDecodingIntoAGoTypeGivesTheSameValue reaches this: the first reads a
// stream into `any`, the second one document through Unmarshal. What only a
// stream of typed destinations asks is how the walk pauses between documents,
// takes a destination of another type on each Decode, and switches to the tree
// in the middle of a stream where a destination or a document needs one.
//
// The yardstick is the tree and not the `any` read. A Decoder into `any` runs
// the same paused walk as one into a struct, so a defect in how the walk hands
// documents out would read the same on both sides and pass.
//
// Each struct destination also carries a field no document writes, set before
// Decode. A Decoder fills the value the caller hands over, so the field has to
// come back as it went in, on both sides.
func TestAStreamDecodesIntoAGoTypePerDocument(t *testing.T) {
	tally := newTally()

	var compared, structs, others int

	rapid.Check(t, func(rt *rapid.T) {
		docs := yamlgen.Streams().Draw(rt, "stream")
		style := yamlgen.Styles().Draw(rt, "style")

		// Values draws a mapping with keys a struct can name in about one
		// document in eleven, which leaves the struct walk mostly unexercised.
		// A document put under one plain key has a struct at its root and the
		// drawn value as its field.
		for i := range docs {
			if rapid.Bool().Draw(rt, "under a key") {
				docs[i] = yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "doc"}, Val: docs[i]}}}
			}
		}

		// A document no typed destination can hold says nothing here, and one
		// a property of its own excuses is excused here too. See
		// TestDecodingIntoAGoTypeGivesTheSameValue for the collection key.
		const applies = yamlgen.Parses | yamlgen.Decode | yamlgen.DecodeTyped |
			yamlgen.StreamDecode | yamlgen.StreamDecodeTyped

		for _, v := range docs {
			if holdsACollectionKey(v) {
				return
			}
			if known := yamlgen.Known(ledger, applies, v, style); known != nil {
				tally.record(known.Name, true)

				return
			}
		}

		w := yamlgen.WriteStream(docs, style)
		if w.MeansUnclear {
			return
		}

		// The destinations are built from the tree's own reading, so the walk
		// takes no part in setting its yardstick. A stream the tree will not
		// read into `any` is TestAStreamReadsBackAsItsDocuments's to answer.
		loose, err := treeStreamInto(w.Text, nil)
		if err != nil {
			return
		}

		types := make([]reflect.Type, len(loose))
		for i, doc := range loose {
			shape := yamlgen.TargetShape(rapid.IntRange(0, 2).Draw(rt, "shape"))
			types[i] = withPresetField(yamlgen.TargetForDecodedAs(doc.Interface(), shape).Type)
			if types[i].Kind() == reflect.Struct {
				structs++
			} else {
				others++
			}
		}

		walked, walkErr := streamInto(w.Text, types)
		treed, treeErr := treeStreamInto(w.Text, types)

		if (walkErr != nil) != (treeErr != nil) {
			rt.Fatalf("%s: one reading of the stream failed:\n%s\n---\nwalk: %v\ntree: %v",
				style, w.Text, walkErr, treeErr)
		}
		if treeErr != nil {
			// Both refused. The tree refuses the whole stream on the first
			// Decode and the walk on the document that breaks, so the values
			// before that are not comparable.
			return
		}
		compared++

		requireSameTypedStream(rt, style, w.Text, types, walked, treed)
	})

	t.Logf("%d streams compared, %d struct destinations and %d others", compared, structs, others)
	tally.report(t, yamlgen.StreamDecodeTyped)
}

// requireSameTypedStream fails rt unless the walk read every document of src as
// the tree did, and left each preset field as it was set.
func requireSameTypedStream(rt *rapid.T, style yamlgen.Style, src string, types []reflect.Type, walked, treed []reflect.Value) {
	if len(walked) != len(treed) {
		rt.Fatalf("%s: the walk read %d documents and the tree %d:\n%s", style, len(walked), len(treed), src)
	}
	for i := range treed {
		if preset, ok := presetOf(walked[i]); ok && preset != presetValue {
			rt.Fatalf("%s: document %d wrote over a field it never names, %q:\n%s", style, i, preset, src)
		}

		want := yamlgen.Normalize(treed[i])
		got := yamlgen.Normalize(walked[i])
		if !sameThroughAType(want, got) {
			rt.Fatalf("%s: document %d into %v read differently:\n%s\n---\ntree: %#v\nwalk: %#v",
				style, i, types[i], src, want, got)
		}
	}
}

const (
	// presetName and presetKey name the field withPresetField adds. The key is
	// one Strings never draws, so no document writes it.
	presetName  = "Preset"
	presetKey   = "\x00preset"
	presetValue = "set before Decode"
)

// withPresetField returns a struct type t with one more field, which no document
// writes. Any other type comes back as it is.
func withPresetField(t reflect.Type) reflect.Type {
	if t.Kind() != reflect.Struct {
		return t
	}

	fields := make([]reflect.StructField, 0, t.NumField()+1)
	for f := range t.Fields() {
		fields = append(fields, f)
	}
	fields = append(fields, reflect.StructField{
		Name: presetName,
		Type: reflect.TypeFor[string](),
		Tag:  reflect.StructTag(`yaml:"` + presetKey + `"`),
	})

	return reflect.StructOf(fields)
}

// presetOf returns the field withPresetField added, where v has one.
func presetOf(v reflect.Value) (string, bool) {
	if v.Kind() != reflect.Struct {
		return "", false
	}
	f := v.FieldByName(presetName)
	if !f.IsValid() {
		return "", false
	}

	return f.String(), true
}

// streamInto reads src with one Decoder, document i into a fresh value of
// types[i], with its preset field set first. It stops at io.EOF or the first
// error. A nil types reads every document into an `any`.
func streamInto(src string, types []reflect.Type, opts ...codec.DecodeOption) ([]reflect.Value, error) {
	dec := codec.NewDecoder(bytes.NewReader([]byte(src)), opts...)

	var out []reflect.Value
	for i := 0; ; i++ {
		typ := reflect.TypeFor[any]()
		if types != nil {
			if i >= len(types) {
				// One more Decode than the stream has documents, to read its
				// io.EOF.
				typ = reflect.TypeFor[any]()
			} else {
				typ = types[i]
			}
		}

		into := reflect.New(typ)
		if f := into.Elem(); f.Kind() == reflect.Struct {
			if preset := f.FieldByName(presetName); preset.IsValid() {
				preset.SetString(presetValue)
			}
		}

		err := dec.Decode(into.Interface())
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, into.Elem())
	}
}

// treeStreamInto is streamInto through a tree. A custom unmarshaler turns the
// walk off, and no destination here holds the chan int it names.
func treeStreamInto(src string, types []reflect.Type) ([]reflect.Value, error) {
	return streamInto(src, types, codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))
}
