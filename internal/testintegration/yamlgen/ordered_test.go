// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// TestAnOrderedMapAgreesWithCodec holds yamlgen.OrderedMap to codec.MapSliceSeq, which it replaces in the
// generator's value model.
//
// The stored corpus holds three "!!omap" meanings, all empty, so regenerating it cannot show that the keys are
// named, ordered and deduplicated as before. This draws "!!omap" documents and checks both: codec refuses the same
// entry lists as duplicates, and the JSON object holds the same keys, in the same order.
//
// The values are not compared with codec's. The codec-typed meaning went through go-yaml's own JSON encoder, which
// wrote the float 0 as 0.0 and a float past float64's range as a string. OrderedMap writes its values as MeaningJSON
// writes every other value: 0, and the number 1e+330.
func TestAnOrderedMapAgreesWithCodec(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pairs := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) yamlgen.Pair {
			return yamlgen.Pair{Key: yamlgen.Keys().Draw(t, "key"), Val: yamlgen.Values().Draw(t, "value")}
		}), 0, 4).Draw(t, "pairs")

		items := make([]yamlgen.Value, 0, len(pairs))
		for _, pair := range pairs {
			items = append(items, yamlgen.Map{Pairs: []yamlgen.Pair{pair}})
		}

		decoded := yamlgen.Tagged{Tag: yamlgen.TagOMap, V: yamlgen.Seq{Items: items}}.Decoded()

		ordered, isOrdered := decoded.(yamlgen.OrderedMap)
		if !isOrdered {
			// The generator read the sequence as plain, so codec must refuse its entries.
			entries, ok := singleEntries(decoded)
			if !ok {
				return
			}
			if _, err := codec.NewMapSliceSeq(entries...); err == nil {
				t.Fatalf("codec accepts entries yamlgen calls duplicates: %#v", entries)
			}

			return
		}

		var items2 []codec.MapItem
		for key, value := range ordered.All() {
			items2 = append(items2, codec.MapItem{Key: toCodec(key), Value: toCodec(value)})
		}

		want, err := codec.NewMapSliceSeq(items2...)
		if err != nil {
			t.Fatalf("codec refuses entries yamlgen accepts: %v", err)
		}

		wantJSON, err := codecMeaningJSON(want)
		if err != nil {
			t.Skipf("codec cannot write this meaning: %v", err)
		}

		gotJSON, err := yamlgen.MeaningJSON(ordered)
		if err != nil {
			t.Fatalf("MeaningJSON: %v", err)
		}

		if !sameKeys(gotJSON, wantJSON) {
			t.Fatalf("JSON differs\n  yamlgen: %s\n  codec:   %s", gotJSON, wantJSON)
		}
	})
}

// singleEntries returns the entries of a decoded sequence of one-entry mappings.
func singleEntries(decoded any) ([]codec.MapItem, bool) {
	seq, ok := decoded.([]any)
	if !ok {
		return nil, false
	}

	out := make([]codec.MapItem, 0, len(seq))

	for _, item := range seq {
		switch m := item.(type) {
		case map[string]any:
			for k, v := range m {
				out = append(out, codec.MapItem{Key: k, Value: toCodec(v)})
			}
		case map[any]any:
			for k, v := range m {
				out = append(out, codec.MapItem{Key: toCodec(k), Value: toCodec(v)})
			}
		default:
			return nil, false
		}
	}

	return out, true
}

// toCodec converts the generator's own value types to the codec types they replace.
func toCodec(v any) any {
	switch n := v.(type) {
	case yamlgen.Base64:
		return codec.Base64(n)
	case yamlgen.OrderedMap:
		var items []codec.MapItem
		for key, value := range n.All() {
			items = append(items, codec.MapItem{Key: toCodec(key), Value: toCodec(value)})
		}
		out, err := codec.NewMapSliceSeq(items...)
		if err != nil {
			panic(err)
		}

		return out
	case []any:
		out := make([]any, 0, len(n))
		for _, item := range n {
			out = append(out, toCodec(item))
		}

		return out
	case map[string]any:
		out := make(map[string]any, len(n))
		for key, value := range n {
			out[key] = toCodec(value)
		}

		return out
	case map[any]any:
		out := make(map[any]any, len(n))
		for key, value := range n {
			out[toCodec(key)] = toCodec(value)
		}

		return out
	}

	return v
}

// codecJSONNumbers is the jsonNumbers step of MeaningJSON as it stood while meanings held codec types.
func codecJSONNumbers(v any) any {
	switch n := v.(type) {
	case yamlgen.Inexact:
		return json.Number(n.Text)
	case *big.Float:
		return json.Number(n.Text('g', -1))
	case []any:
		out := make([]any, 0, len(n))
		for _, item := range n {
			out = append(out, codecJSONNumbers(item))
		}

		return out
	case map[string]any:
		out := make(map[string]any, len(n))
		for key, value := range n {
			out[key] = codecJSONNumbers(value)
		}

		return out
	case codec.MapSliceSeq:
		var items []codec.MapItem
		for key, value := range n.All() {
			items = append(items, codec.MapItem{Key: key, Value: codecJSONNumbers(value)})
		}
		ordered, err := codec.NewMapSliceSeq(items...)
		if err != nil {
			return n
		}

		return ordered
	}

	return v
}

// sameKeys reports whether two JSON objects hold the same keys, in the same order.
func sameKeys(a, b []byte) bool {
	ka, errA := objectKeys(a)
	kb, errB := objectKeys(b)

	return errA == nil && errB == nil && slices.Equal(ka, kb)
}

// objectKeys returns the keys of a JSON object, in the order they are written.
func objectKeys(doc []byte) ([]string, error) {
	d := json.NewDecoder(bytes.NewReader(doc))
	if _, err := d.Token(); err != nil {
		return nil, err
	}

	var keys []string

	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}

		name, ok := key.(string)
		if !ok {
			return nil, errors.New("a key that is not a string")
		}

		keys = append(keys, name)

		var value json.RawMessage // skipped: only the keys are compared
		if err := d.Decode(&value); err != nil {
			return nil, err
		}

	}

	return keys, nil
}

// codecMeaningJSON writes a codec-typed meaning as MeaningJSON wrote it before the generator had its own types.
//
// It returns an error where codec's encoder panics. codec.Marshal panics on a map[any]any holding a nil key,
// because encodeMap passes reflect.ValueOf(nil) to MapIndex, and a mapping keyed by null decodes to one.
func codecMeaningJSON(m codec.MapSliceSeq) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("codec panics: %v", r)
		}
	}()

	return json.Marshal(codecJSONNumbers(yamlgen.NamedKeys(m)))
}
