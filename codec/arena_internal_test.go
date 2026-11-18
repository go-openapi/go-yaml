// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// pointsInto reports whether s reads bytes of src.
//
// Comparing the addresses is the only way to ask. A copy and a window compare
// equal as strings, so only the address of the first byte tells them apart.
func pointsInto(s string, src []byte) bool {
	if len(s) == 0 || len(src) == 0 {
		return false
	}
	at := uintptr(unsafe.Pointer(unsafe.StringData(s)))
	first := uintptr(unsafe.Pointer(&src[0]))

	return at >= first && at < first+uintptr(len(src))
}

// arenaSrc writes a string in every shape the decoder reads one from: plain,
// quoted, single-quoted, literal, folded, tagged, a key, a sequence entry, and
// one long enough to be copied on its own.
var arenaSrc = "plain: hello world\n" +
	"quoted: \"a quoted value\"\n" +
	"single: 'single quoted'\n" +
	"literal: |\n  block text\n" +
	"folded: >\n  folded text\n" +
	"escaped: \"tab\\there\"\n" +
	"tagged: !!str 0x10\n" +
	"anchored: &a anchor text\n" +
	"aliased: *a\n" +
	"seq:\n  - first entry\n  - second entry\n" +
	"nested:\n  inner key: inner value\n" +
	"long: " + longScalar + "\n"

var longScalar = string(bytes.Repeat([]byte("x"), 4<<10))

// walkStrings collects every string a decoded value holds, keys included.
func walkStrings(v any, out *[]string) {
	switch t := v.(type) {
	case string:
		*out = append(*out, t)
	case map[string]any:
		for k, e := range t {
			*out = append(*out, k)
			walkStrings(e, out)
		}
	case []any:
		for _, e := range t {
			walkStrings(e, out)
		}
	}
}

func TestDecodedStringsLeaveTheDocument(t *testing.T) {
	t.Run("walk", func(t *testing.T) {
		src := []byte(arenaSrc)
		v, err := WalkValue(src)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		walkStrings(v, &got)
		if len(got) < 20 {
			t.Fatalf("read only %d strings, the document writes more", len(got))
		}
		for _, s := range got {
			if pointsInto(s, src) {
				t.Errorf("%q still reads the document", s)
			}
		}
	})

	t.Run("tree", func(t *testing.T) {
		src := []byte(arenaSrc)
		dec := treeDecoder(string(src))

		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
		if dec.walkedOK {
			t.Fatal("did not take the tree path")
		}
		var got []string
		walkStrings(v, &got)
		if len(got) < 20 {
			t.Fatalf("read only %d strings, the document writes more", len(got))
		}
		for _, s := range got {
			// The Decoder read the reader into a buffer of its own, which is
			// the document these strings must not be reading.
			if pointsInto(s, dec.src) {
				t.Errorf("%q still reads the document", s)
			}
		}
	})

	t.Run("reflection", func(t *testing.T) {
		src := []byte(arenaSrc)

		var dst struct {
			Plain   string            `yaml:"plain"`
			Quoted  string            `yaml:"quoted"`
			Literal string            `yaml:"literal"`
			Tagged  string            `yaml:"tagged"`
			Seq     []string          `yaml:"seq"`
			Nested  map[string]string `yaml:"nested"`
			Long    string            `yaml:"long"`
		}
		if err := Unmarshal(src, &dst); err != nil {
			t.Fatal(err)
		}

		got := []string{dst.Plain, dst.Quoted, dst.Literal, dst.Tagged, dst.Long}
		got = append(got, dst.Seq...)
		for k, v := range dst.Nested {
			got = append(got, k, v)
		}
		for _, s := range got {
			if pointsInto(s, src) {
				t.Errorf("%q still reads the document", s)
			}
		}
		if dst.Plain != "hello world" || dst.Tagged != "0x10" || len(dst.Long) != 4<<10 {
			t.Errorf("decoded the wrong values: %+v", dst)
		}
	})
}

// TestArenaBoxHoldsWhatItWasGiven checks the hand-built interface behaves like
// one the compiler made: it asserts, compares and keys a map.
func TestArenaBoxHoldsWhatItWasGiven(t *testing.T) {
	var a arena

	want := make([]string, 0, 3*arenaSlab)
	got := make([]any, 0, 3*arenaSlab)
	for i := range 3 * arenaSlab {
		s := string(rune('a'+i%26)) + string(rune('0'+i%10)) + longScalar[:i%64]
		want = append(want, s)
		got = append(got, a.value(s))
	}

	// The interface points into a slab the arena no longer holds, so the
	// collector has to keep that slab alive on the strength of the interface
	// alone. Dropping the arena and collecting twice checks that it does.
	a = arena{}
	runtime.GC()
	runtime.GC()

	m := map[any]int{}
	for i, v := range got {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("%d: holds %T, not a string", i, v)
		}
		if s != want[i] {
			t.Fatalf("%d: holds %q, want %q", i, s, want[i])
		}
		if v != any(want[i]) {
			t.Errorf("%d: does not compare equal to the same string", i)
		}
		m[v] = i
	}
	if len(m) == 0 {
		t.Error("boxed strings do not key a map")
	}
}

// pinnedStrings returns every string in v that reads src.
func pinnedStrings(v any, src []byte) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			if pointsInto(t, src) {
				out = append(out, t)
			}
		case map[string]any:
			for k, e := range t {
				if pointsInto(k, src) {
					out = append(out, k)
				}
				walk(e)
			}
		case MapSlice:
			for k, v := range t.All() {
				walk(k)
				walk(v)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)

	return out
}

// TestNoDecodedStringReadsTheDocument sweeps the YAML test suite and the fuzz
// seeds, on both paths and with the ordered map. One string left pointing at
// the document holds the whole of it, so the check is over every document
// rather than the handful arenaSrc writes.
func TestNoDecodedStringReadsTheDocument(t *testing.T) {
	for _, src := range corpusSources() {
		if strings.Contains(src.text, "&!") {
			continue
		}

		if v, err := WalkValue([]byte(src.text)); err == nil {
			if held := pinnedStrings(v, []byte(src.text)); len(held) > 0 {
				t.Errorf("%s: walk left %q reading the document", src.name, held)
			}
		}

		for _, mode := range []struct {
			name string
			opts []DecodeOption
		}{
			{"tree", []DecodeOption{CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })}},
			{"ordered", []DecodeOption{UseOrderedMap()}},
		} {
			dec := NewDecoder(bytes.NewReader([]byte(src.text)), mode.opts...)
			var v any
			if err := dec.Decode(&v); err != nil {
				continue
			}
			if held := pinnedStrings(v, dec.src); len(held) > 0 {
				t.Errorf("%s: %s left %q reading the document", src.name, mode.name, held)
			}
		}
	}
}
