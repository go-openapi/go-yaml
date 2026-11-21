// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"
)

// TestReadAllDoesNotCopyWhatItIsGiven checks the decoder reads the caller's
// bytes rather than a copy of them.
//
// Unmarshal wraps the caller's slice in a bytes.Buffer, and the parser keeps
// windows into what it parses, so a copy here would be one the whole tree then
// holds. This fails the moment readAll starts allocating again.
func TestReadAllDoesNotCopyWhatItIsGiven(t *testing.T) {
	data := []byte("a: 1\nb: two\n")

	got, err := readAll(bytes.NewBuffer(data))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("readAll changed the document: %q", got)
	}
	if &got[0] != &data[0] {
		t.Fatal("readAll copied a bytes.Buffer's contents instead of reading them where they were")
	}
}

// TestReadAllReadsWhatIsNotASlice checks the fallback still reads a plain
// reader through, since only a byte-backed one can be read in place.
func TestReadAllReadsWhatIsNotASlice(t *testing.T) {
	const doc = "a: 1\nb: two\n"

	got, err := readAll(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != doc {
		t.Fatalf("readAll changed the document: %q", got)
	}
}

// TestReadAllReadsAByteReader covers *bytes.Reader, which holds the bytes but
// will not hand them over.
func TestReadAllReadsAByteReader(t *testing.T) {
	data := []byte("a: 1\n")

	got, err := readAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("readAll changed the document: %q", got)
	}
	if len(got) > 0 && uintptr(unsafe.Pointer(&got[0])) == uintptr(unsafe.Pointer(&data[0])) {
		t.Fatal("bytes.Reader does not expose its slice; this now aliases it, which is worth knowing")
	}
}
