// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	"github.com/go-openapi/go-yaml/parser"
)

// Shapes the generator found that still diverge.
//
// A case here pins today's behavior rather than the correct behavior, so that a
// fix breaks the test that says it was broken, and the matching entry in
// [yamlgen.Ledger] is what keeps the property tests from failing on it
// meanwhile. When both go the case moves to fixed_test.go with its assertions
// inverted, which is where all of them are now.
//
// Write the next one here. The two helpers below are what a case needs, and
// they are kept for it rather than moved.

// wellFormed asserts src is a YAML 1.2 document before anything is asked of the
// library, so that a case here is a claim about the library and not about a
// document nobody has to read.
func wellFormed(t *testing.T, src string) {
	t.Helper()

	require.True(t, grammar.NewRecognizer(1024).Stream([]byte(src)).OK,
		"the document is not YAML 1.2, so there is nothing to hold the library to")
}

// renderOnce reads a document with comments kept and writes it back.
func renderOnce(t *testing.T, src string) string {
	t.Helper()

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	return file.String()
}

// The three defects Style.ExplicitKeys found on its first deep run.
//
// A mapping entry has two spellings -- "key: value" and "? key" over
// ": value" -- and the generator had only ever written the short one. All three
// of these are the long form and nothing else: the same document written short
// reads, libfyaml 1.0.0b1 reads every one, the reference parser passes them,
// and grammar.NewRecognizer accepts them.

// secondDocument reads a two-document stream and returns what the second one
// holds, which is where this defect shows.
func secondDocument(t *testing.T, src string) string {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var last any

	for i := 0; ; i++ {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoErrorf(t, err, "document %d of %q", i, src)

		last = v
	}

	text, isText := last.(string)
	require.Truef(t, isText, "the last document of %q is %#v", src, last)

	return text
}

// readTheStream reads every document of src, which is what a caller looping
// over codec.Decoder gets.
func readTheStream(t *testing.T, src string) []any {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var out []any

	for {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoErrorf(t, err, "%q", src)

		out = append(out, v)
	}
}

// TestDefectAPropertiedKeyRefusesABlockScalarValue pins the combination.
//
// An anchor or a tag on an *implicit* key, over a block scalar value, is refused
// with "value is not allowed in this context. map key-value is pre-defined".
// Nothing about how the value is written is the key's business: 8.2.2 puts an
// implicit key at ns-s-block-map-implicit-key, a flow node, and a flow node
// carries its properties.
//
// Four controls place it exactly. The same key with a plain value reads, the
// same key over a block collection reads, the same block scalar under a bare key
// reads, and -- the sharpest -- the same entry written the long way reads:
// "? &a1 k" over ": >-" over " x" is the mapping, where "&a1 k: >-" is refused.
//
// Reached on 2026-09-08 by TestAStreamReadsBackAsItsDocuments, on the first run
// after the tagger and the aliaser began walking keys.
func TestDefectAPropertiedKeyRefusesABlockScalarValue(t *testing.T) {
	t.Run("today a property on the key refuses the block scalar", func(t *testing.T) {
		for _, src := range []string{
			"&a1 k: >-\n x\n",
			"&a1 k: |-\n x\n",
			"!!str k: >-\n x\n",
			"&a1 \"k\": &a2 !!str >-\n x\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "value is not allowed in this context", "%q", src)
		}
	})

	t.Run("and every adjacent shape reads", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			// The block scalar under a bare key.
			{src: "k: >-\n x\n", want: map[string]any{"k": "x"}},
			// The propertied key over a plain value.
			{src: "&a1 k: v\n", want: map[string]any{"k": "v"}},
			{src: "!!str k: v\n", want: map[string]any{"k": "v"}},
			// The propertied key over a block collection.
			{src: "&a1 k:\n  a: 1\n", want: map[string]any{"k": map[string]any{"a": uint64(1)}}},
			{src: "&a1 k:\n  - 1\n", want: map[string]any{"k": []any{uint64(1)}}},
			// And the same entry written the long way, which is the control
			// that places the fault on the implicit key: "? &a1 k" over
			// ": >-" reads where "&a1 k: >-" is refused.
			{src: "? &a1 k\n: >-\n x\n", want: map[string]any{"k": "x"}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestDefectAPropertiedEmptyKeyIsMishandled pins the three arrangements.
//
// No generated document reaches these: the tagger leaves keys untagged until
// the family is fixed, since a ledger entry wide enough to excuse it would
// excuse every document holding a tagged key. The emitter can write them --
// keyIn puts a key's properties in front of it since 2026-09-08 -- so the draw
// is one line away once the parser reads them.
//
// A tag written verbatim on a key that writes nothing swallows the entry: the
// document comes back as the tagged empty node instead of as a mapping holding
// it. The shorthand spelling of the same tag reads correctly, and so does the
// verbatim tag on a key that writes something -- so it is the two spellings
// parting company over an empty key.
//
// Giving the entry a value turns the loss into an error, which is the sharper
// half: `!<tag:yaml.org,2002:null> : 1` is refused outright.
//
// Reached on 2026-09-08 by TestPresentationInvariance, which read two
// presentations of one value differently once the tagger began walking keys.
func TestDefectAPropertiedEmptyKeyIsMishandled(t *testing.T) {
	t.Run("the shorthand spelling reads the mapping", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("!!null :\n"), &got))
		assert.Equal(t, map[any]any{nil: nil}, got)
	})

	t.Run("and so does the verbatim spelling on a key that writes something", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("!<tag:yaml.org,2002:null> null:\n"), &got))
		assert.Equal(t, map[any]any{nil: nil}, got)
	})

	t.Run("today the verbatim spelling on an empty key gives the scalar", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{src: "!<tag:yaml.org,2002:null> :\n", want: nil},
			{src: "!<tag:yaml.org,2002:str> :\n", want: ""},
			{src: "!<!foo> :\n", want: nil},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "today: the mapping is gone: %q", tc.src)
		}
	})

	t.Run("today an entry with a value is refused outright", func(t *testing.T) {
		var got any
		err := codec.Unmarshal([]byte("!<tag:yaml.org,2002:null> : 1\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is not allowed in this context")

		err = codec.Unmarshal([]byte("a: 1\n!<tag:yaml.org,2002:null> :\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-map value is specified")
	})

	t.Run("today a tag ahead of an anchor is refused", func(t *testing.T) {
		// Style.PropertyOrder writes this one. The other order reads.
		var got any
		err := codec.Unmarshal([]byte("!!null &a1 : 1\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support this kind of node")

		require.NoError(t, codec.Unmarshal([]byte("&a1 !!null : 1\n"), &got))
		assert.Equal(t, map[any]any{nil: uint64(1)}, got)
	})

	t.Run("today an anchor and a tag together are refused below another entry", func(t *testing.T) {
		var got any
		err := codec.Unmarshal([]byte("a: 1\n&a1 !!null : 2\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-map value is specified")

		// Either property alone, in the same position, reads.
		for _, src := range []string{"a: 1\n!!null : 2\n", "a: 1\n&a1 : 2\n"} {
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[any]any{"a": uint64(1), nil: uint64(2)}, got, "%q", src)
		}
	})
}

// TestDefectTwoBareColonLinesInARowAreRefused pins it.
//
// 8.2.2 lets an explicit entry's value be the empty node and lets an entry's key
// be the empty node, so `? a` over `:` over `: v` is two entries and two bare
// ":" lines. It is refused with `found an invalid key for this map`.
//
// grammar.NewRecognizer accepts it, the reference parser passes it, and libfyaml
// 1.0.0b1 reads {a: null, null: "v"}. go.yaml.in/yaml/v3 v3.0.5 refuses it,
// which is its own refusal of a bare ":" as an empty key rather than agreement.
//
// Reached on 2026-09-08 by Keys drawing a collection: a collection key forces
// the explicit form, which put a bare ":" where nothing had put one before. The
// collection is not needed to reproduce it, which the neighbors below show.
func TestDefectTwoBareColonLinesInARowAreRefused(t *testing.T) {
	t.Run("today the pair of bare colon lines is refused", func(t *testing.T) {
		for _, src := range []string{"? a\n:\n: v\n", "?\n \"\": 0\n:\n: v\n"} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "found an invalid key for this map", "%q", src)
		}
	})

	// Change any one thing and it reads, which is what makes the pair of lines
	// the shape rather than the explicit key, the empty value or the empty key.
	t.Run("each neighbor reads", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"a:\n: v\n", map[any]any{"a": nil, nil: "v"}},
			{"? a\n: 1\n: v\n", map[any]any{"a": uint64(1), nil: "v"}},
			{"? a\n:\nk: v\n", map[string]any{"a": nil, "k": "v"}},
			{"? a\n:\n", map[string]any{"a": nil}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestDefectACommentAboveABlankLineAddsALeadingBreak pins it.
//
// A sequence entry carrying a comment, with a blank line before its content,
// renders with a blank line at the head of the document. Rendering again drops
// it, so the first pass does not settle and the second does.
//
// The value survives every pass, and the tab is not part of the shape: a space
// does it too, and without the blank line it settles on the first pass.
func TestDefectACommentAboveABlankLineAddsALeadingBreak(t *testing.T) {
	t.Run("today the first render adds a leading break", func(t *testing.T) {
		for _, src := range []string{"-\t#\n\n e\n", "- #\n\n e\n", "-\t# c\n\n e\n"} {
			f, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, err, "%q", src)

			once := f.String()
			assert.Truef(t, strings.HasPrefix(once, "\n"), "today: %q renders to %q", src, once)

			g, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoErrorf(t, err, "%q", once)
			assert.Equalf(t, strings.TrimPrefix(once, "\n"), g.String(),
				"the second render settles: %q", once)
		}
	})

	t.Run("the value survives every pass", func(t *testing.T) {
		const src = "-\t#\n\n e\n"

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, []any{"e"}, got)
	})

	t.Run("without the blank line it settles on the first pass", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("-\t#\n e\n"), parser.WithComments())
		require.NoError(t, err)
		assert.Equal(t, "- e #\n", f.String())
	})
}
