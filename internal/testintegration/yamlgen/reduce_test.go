// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

func TestReduce(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		interesting func([]byte) bool
		want        string
	}{
		{
			name:        "drops the lines that do not matter",
			src:         "a: 1\nb: 2\nBANG: 3\nd: 4\n",
			interesting: func(b []byte) bool { return bytes.Contains(b, []byte("BANG")) },
			// Reduction does not stop at the line: once the other lines are
			// gone the byte pass takes the rest of this one with them. The
			// break the document ends on stays, because a document without one
			// is not one the generator would have written.
			want: "BANG\n",
		},
		{
			name:        "shortens a scalar once the lines are gone",
			src:         "key: aaaaaaaaaa\nother: bbbb\n",
			interesting: func(b []byte) bool { return bytes.Contains(b, []byte("aa")) },
			want:        "aa\n",
		},
		{
			name:        "leaves an already minimal document alone",
			src:         "x",
			interesting: func(b []byte) bool { return bytes.Contains(b, []byte("x")) },
			want:        "x",
		},
		{
			name:        "returns the input when it is not interesting to begin with",
			src:         "a: 1\n",
			interesting: func(_ []byte) bool { return false },
			want:        "a: 1\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(yamlgen.Reduce([]byte(tc.src), tc.interesting))
			assert.Equal(t, tc.want, got)
		})
	}
}

// interestingHere stands in for a defect while none is outstanding.
//
// The reducer and its report do not care what makes a document interesting,
// only that something does -- and wiring them to whichever defect happens to be
// open would leave them untested the moment it is fixed, which is exactly when
// the next one needs them.
func interestingHere(src []byte) bool {
	return bytes.Contains(src, []byte("BANG"))
}

// TestReduceKeepsThePredicateTrue is the property that matters for a reducer:
// whatever it returns must still be a case of the thing being reduced.
func TestReduceKeepsThePredicateTrue(t *testing.T) {
	src := "a: 1\nb: BANG\nc: 3\n"
	require.True(t, interestingHere([]byte(src)), "the fixture must be interesting to begin with")

	small := yamlgen.Reduce([]byte(src), interestingHere)

	require.True(t, interestingHere(small),
		"the reduced document is no longer interesting: %q", small)
	assert.Less(t, len(small), len(src), "and it should be smaller than what it started from")
	t.Logf("reduced %q to %q", src, string(small))
}

// TestReducedReportIsUseful exercises the whole failure path, because the
// report is only ever produced when something has already gone wrong -- which
// is precisely when nobody wants to discover that the reporting itself is
// broken.
func TestReducedReportIsUseful(t *testing.T) {
	report := reduced("Defect", "a: 1\nb: BANG\nc: 3\n", interestingHere)

	assert.Contains(t, report, "as generated")
	assert.Contains(t, report, "reduced to")
	assert.Contains(t, report, "renders to:")
	assert.Contains(t, report, "reads as:")
	assert.Contains(t, report, "func TestDefect(t *testing.T) {")
	t.Log(report)
}

// TestAValueChangedBeforeRenderingIsNotReportedAsRendering holds the document
// apart from the message that reports it.
//
// TestRenderPreservesValue compares Written.Means against a reading of the
// rendered text, so it fails when either the writing or the rendering moved the
// value. It reported both through reduced("RenderChangedTheValue", ...), whose
// predicate reads the document and reads the rendering of it -- a different
// comparison, and one that holds only for the rendering half.
//
// On the document below the predicate is false: the rendering preserves it
// exactly, and the value moved when the document was written. So the report named the
// renderer, printed "reads as" and "then as" byte-identical because both come
// after the step that moved the value, and emitted a reproducer asserting the
// value its own "// today:" line gave. yaml-transform read it on 2026-09-10 and
// could not tell what differed, which is the whole complaint.
//
// The document is the one the failfile held, kept verbatim. Reduction cannot
// shrink it -- Reduce keeps its predicate true, and this predicate is false at
// every step.
func TestAValueChangedBeforeRenderingIsNotReportedAsRendering(t *testing.T) {
	const src = "\ufeff&a3\n- &a2\n ?\tnull\n : &a1 !!binary\t\"AA==\"\n ?\t*a1\n :\t\"aliased\"\n"

	assert.False(t, renderChangesValue([]byte(src)),
		"rendering preserves this document, so a message naming the renderer names the wrong stage")

	// The "!!binary" value reads as a codec.Base64, the text the document
	// carries, since 752f09c. It was []byte{0} before that, which Go hashes
	// nowhere and which nothing marked as binary.
	var got any
	require.NoError(t, codec.Unmarshal([]byte(src), &got))
	assert.Equal(t, []any{map[string]any{"null": codec.Base64("AA=="), "AA==": "aliased"}}, got)
}

// TestReduceKeepsTheDocumentEndingInABreak: the byte pass will not remove the
// line break the document ends on.
//
// It is the most productive byte in the document to remove -- without it a
// block scalar means something else, so many predicates stay true -- and the
// result is a document no emitter here would produce. Reduction that leaves the
// generated space arrives at a different defect than the one that was found,
// which is worse than not reducing at all.
func TestReduceKeepsTheDocumentEndingInABreak(t *testing.T) {
	src := "k: |\n  g\n"
	// True of the reduced form that drops the final break, and false of the
	// document itself: exactly the drift being prevented.
	interesting := func(b []byte) bool { return bytes.Contains(b, []byte("g")) }

	got := yamlgen.Reduce([]byte(src), interesting)

	assert.True(t, bytes.HasSuffix(got, []byte("\n")), "reduced to %q", got)
}

func TestReproducerIsValidGo(t *testing.T) {
	got := yamlgen.Reproducer("Defect", "k: |+\n  a\n\n", "x", "y")

	assert.Contains(t, got, "func TestDefect(t *testing.T) {")
	assert.Contains(t, got, "yaml.Unmarshal")
	// A YAML document is far more readable backquoted, and every document the
	// generator emits can be.
	assert.Contains(t, got, "const src = `k: |+")
	assert.True(t, strings.HasSuffix(got, "}\n"))
}
