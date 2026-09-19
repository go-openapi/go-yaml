// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Writer builds an artifact.
//
// Cases are written as they arrive rather than collected, so producing a corpus
// of any size costs one case of memory.
type Writer struct {
	gz      *gzip.Writer
	enc     *json.Encoder
	header  Header
	written int
	closed  bool
}

// NewWriter starts an artifact with the given header.
//
// The header states how many cases follow, and Close refuses if a different
// number arrived: a corpus whose count does not match its contents is either
// truncated or built by something that lost track, and both are worth failing
// on rather than shipping.
func NewWriter(w io.Writer, h Header) (*Writer, error) {
	if h.Format == 0 {
		h.Format = Format
	}

	if err := validate(h); err != nil {
		return nil, err
	}

	// The vocabulary is sorted rather than taken as given, because two runs
	// that discovered the same tags in a different order must still produce
	// identical bytes.
	h.Vocabulary = slices.Clone(h.Vocabulary)
	slices.SortFunc(h.Vocabulary, func(a, b TagSpec) int { return strings.Compare(a.Tag, b.Tag) })

	gz := gzip.NewWriter(w)

	// A gzip header carries a modification time and an operating system byte,
	// and both would make the same corpus compress to different bytes on
	// different days or machines. Regenerate-and-diff needs neither to happen.
	gz.ModTime = zeroTime
	gz.OS = unknownOS

	enc := json.NewEncoder(gz)

	if err := enc.Encode(h); err != nil {
		return nil, fmt.Errorf("writing the artifact header: %w", err)
	}

	return &Writer{gz: gz, enc: enc, header: h}, nil
}

// Add appends one case.
func (w *Writer) Add(c Case) error {
	if w.closed {
		return errors.New("suite: the artifact is closed")
	}

	if c.Name == "" {
		return errors.New("suite: a case needs a name, since a finding has to be reportable")
	}

	// Sorted for the same reason the vocabulary is: the same corpus has to
	// come out as the same bytes.
	c.Tags = slices.Sorted(slices.Values(c.Tags))

	if err := w.enc.Encode(c); err != nil {
		return fmt.Errorf("writing case %q: %w", c.Name, err)
	}

	w.written++

	return nil
}

// Close finishes the artifact and checks it against its own header.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}

	w.closed = true

	if w.written != w.header.Cases {
		return fmt.Errorf("suite: the header says %d cases and %d were written",
			w.header.Cases, w.written)
	}

	if err := w.gz.Close(); err != nil {
		return fmt.Errorf("finishing the artifact: %w", err)
	}

	return nil
}

func validate(h Header) error {
	switch {
	case h.Format != Format:
		return fmt.Errorf("suite: format %d, this package writes %d", h.Format, Format)
	case h.Grammar == "":
		return errors.New("suite: an artifact has to name the grammar its verdicts came from")
	case h.Digest == "":
		return errors.New("suite: an artifact has to identify the grammar file, or it cannot be shown to be current")
	case h.Tier == "":
		return errors.New("suite: an artifact has to say which tier it is")
	case h.Cases < 0:
		return errors.New("suite: a negative case count")
	}

	return nil
}
