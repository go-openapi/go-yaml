// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsonspike_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/jsonspike"
)

// TestEveryImplementationDefinedCaseIsTagged holds the tag vocabulary to the
// suite's own judgement of what is open.
//
// The suite named 35 documents implementation-defined. One of them carrying no
// tag means the vocabulary has a gap, and a gap does not announce itself: the
// document simply gets scored as though the question were settled, and whatever
// position our parser happens to hold becomes the expectation for everyone
// else's.
func TestEveryImplementationDefinedCaseIsTagged(t *testing.T) {
	forEachFixture(t, func(t *testing.T, short string, src []byte) {
		if !strings.HasPrefix(short, "i_") {
			return
		}

		if tags := jsonspike.Describe(short, src).Tags; len(tags) == 0 {
			t.Errorf("%s is implementation-defined and carries no tag, so it will be scored as settled", short)
		}
	})
}

// TestNoSettledCaseIsTaggedWithoutCause is the same guard pointing the other
// way, and the one that actually caught something.
//
// A tag on a document the suite settled is not a failure, it is a silent
// removal: the document becomes undecidable for any parser tolerating that
// property and quietly leaves the scored set. An over-eager tagger yields a
// corpus that looks complete and is not.
//
// The cases listed here really do carry what they are tagged with -- ill-formed
// UTF-8, or a byte order mark -- so the tag is right and the suite's insistence
// on rejection is a stricter position than the tag alone implies.
func TestNoSettledCaseIsTaggedWithoutCause(t *testing.T) {
	expected := map[string]bool{
		"n_array_a_invalid_utf8": true, "n_array_invalid_utf8": true,
		"n_number_invalid-utf-8-in-bigger-int": true, "n_number_invalid-utf-8-in-exponent": true,
		"n_number_invalid-utf-8-in-int": true, "n_number_real_with_invalid_utf8_after_e": true,
		"n_object_lone_continuation_byte_in_key_and_trailing_comma": true,
		"n_string_invalid-utf-8-in-escape":                          true,
		"n_string_invalid_utf8_after_escape":                        true,
		"n_structure_UTF8_BOM_no_data":                              true,
		"n_structure_incomplete_UTF8_BOM":                           true,
		"n_structure_lone-invalid-utf-8":                            true,
		"n_structure_single_eacute":                                 true,
	}

	var unexpected []string

	forEachFixture(t, func(_ *testing.T, short string, src []byte) {
		if strings.HasPrefix(short, "i_") || expected[short] {
			return
		}

		if tags := jsonspike.Describe(short, src).Tags; len(tags) > 0 {
			unexpected = append(unexpected, short+" -> "+join(tags))
		}
	})

	sort.Strings(unexpected)
	for _, u := range unexpected {
		t.Errorf("a settled case gained a tag, so it left the scored set: %s", u)
	}
}

func join(tags []stance.Tag) string {
	out := make([]string, len(tags))
	for i, tg := range tags {
		out[i] = string(tg)
	}

	return strings.Join(out, " ")
}

// forEachFixture runs over every document in the suite, skipping the whole
// suite if the sibling checkout holding it is not there.
func forEachFixture(t *testing.T, each func(t *testing.T, short string, src []byte)) {
	t.Helper()

	dir := suiteDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		src, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			t.Fatalf("cannot read %s: %v", name, rerr)
		}

		each(t, strings.TrimSuffix(name, ".json"), src)
	}
}
