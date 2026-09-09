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

	yaml "github.com/go-openapi/go-yaml"
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

// TestDefectABlankLineBeforeACommentDoesNotSettle: the first rendering keeps a
// blank line written before a comment and the second drops it.
//
// Only the rendering wobbles -- the value is the same every time and no comment
// is lost. Found on 2026-09-07 by Style.Chomping's padding, which writes the
// blank lines a "-" or a clip indicator then discards.
func TestDefectABlankLineBeforeACommentDoesNotSettle(t *testing.T) {
	const src = "a:\n - x\n\n# c\nb: 1\n"
	wellFormed(t, src)

	once := renderOnce(t, src)
	assert.Equal(t, "a:\n- x\n\n# c\nb: 1\n", once, "the first rendering keeps the blank line")
	assert.Equal(t, "a:\n- x\n# c\nb: 1\n", renderOnce(t, once), "and the second drops it")

	t.Run("three things are needed", func(t *testing.T) {
		for _, tc := range []struct{ name, src string }{
			// Already at the renderer's own indentation.
			{name: "an indentation the renderer does not use", src: "a:\n- x\n\n# c\nb: 1\n"},
			// A mapping rather than a sequence.
			{name: "a nested sequence", src: "a:\n b: 1\n\n# c\nc: 2\n"},
			// Nothing after the comment.
			{name: "an entry after the comment", src: "a:\n - x\n\n# c\n"},
		} {
			once := renderOnce(t, tc.src)
			assert.Equalf(t, once, renderOnce(t, once), "without %s it settles: %q", tc.name, tc.src)
		}
	})

	t.Run("the value survives every rendering", func(t *testing.T) {
		want := map[string]any{"a": []any{"x"}, "b": uint64(1)}

		text := src
		for range 3 {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(text), &got))
			assert.Equal(t, want, got)
			text = renderOnce(t, text)
		}
	})
}

// TestDefectABlankLineBeforeASequenceEntryDoesNotSettle is the same wobble with
// no comment in it, which is what widens the entry above.
//
// ": &1" over a blank line over "-" over "? \"\"" renders to ": &1" over "- "
// over a blank over "? \"\"" over ":", moving the blank line past the "-", and
// renders again without it. yamlgen.Ledger's predicate for this asks for
// Style.Chomping's padding and a comment, and reaches neither shape here, so
// the property test met it as a plain failure. Found on 2026-09-11 at 30,000
// draws; 200,000 draws of TestRenderReachesAFixedPoint alone did not draw it
// again.
func TestDefectABlankLineBeforeASequenceEntryDoesNotSettle(t *testing.T) {
	const src = ": &1\n\n-\n? \"\"\n"
	wellFormed(t, src)

	once := renderOnce(t, src)
	assert.Equal(t, ": &1\n- \n\n? \"\"\n:\n", once, "the first rendering moves the blank line past the \"-\"")
	assert.Equal(t, ": &1\n- \n? \"\"\n:\n", renderOnce(t, once), "and the second drops it")

	t.Run("the value survives every rendering", func(t *testing.T) {
		want := map[string]any{"": nil, "null": []any{nil}}

		text := src
		for range 3 {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(text), &got))
			assert.Equal(t, want, got)
			text = renderOnce(t, text)
		}
	})
}

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
		assert.Equal(t, map[string]any{"null": nil}, got)
	})

	t.Run("and so does the verbatim spelling on a key that writes something", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("!<tag:yaml.org,2002:null> null:\n"), &got))
		assert.Equal(t, map[string]any{"null": nil}, got)
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
		assert.Contains(t, err.Error(), "names a kind this node is not")

		require.NoError(t, codec.Unmarshal([]byte("&a1 !!null : 1\n"), &got))
		assert.Equal(t, map[string]any{"null": uint64(1)}, got)
	})

	t.Run("today an anchor and a tag together are refused below another entry", func(t *testing.T) {
		var got any
		err := codec.Unmarshal([]byte("a: 1\n&a1 !!null : 2\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-map value is specified")

		// Either property alone, in the same position, reads.
		for _, src := range []string{"a: 1\n!!null : 2\n", "a: 1\n&a1 : 2\n"} {
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"a": uint64(1), "null": uint64(2)}, got, "%q", src)
		}
	})
}

// TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge pins the two spellings apart.
//
// The 1.1 merge type names the key `<<` and says nothing about how it is
// written, so `? <<` over `: *a` is the same key node as `<<: *a` and should
// merge the same way. It does not: the long form comes back as an ordinary key
// named "<<".
//
// Every document here declares `%YAML 1.1`, which is where the merge lives
// since 8acf11b. Without the directive neither spelling merges and the two
// agree, so the defect needs the directive to be reached at all --
// TestFixedAMergeKeyIsAnOrdinaryKeyUnderYAML12 holds that half.
//
// The key beside the merge is `w` and not `y`: 1.1 resolves `y` to the boolean
// true, so a document that declares 1.1 to reach the merge has to keep clear of
// 1.1's other spellings.
//
// go.yaml.in/yaml/v3 v3.0.5 merges both, in block and in flow, and is the oracle
// that answers here -- libfyaml 1.0.0b1 resolves no merge at all and hands "<<"
// back as a member name, so it cannot say which spelling is right.
func TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge(t *testing.T) {
	const base = "%YAML 1.1\n---\nb: &a {x: 1}\n"

	t.Run("the plain spelling merges", func(t *testing.T) {
		for _, src := range []string{
			base + "d:\n  <<: *a\n  w: 2\n",
			// A tag on the mapping changes nothing, which is what makes this
			// the key's presentation rather than the node's type.
			base + "d: !foo\n  <<: *a\n  w: 2\n",
			base + "d: !!map\n  <<: *a\n  w: 2\n",
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"x": uint64(1), "w": uint64(2)}, got["d"], "%q", src)
		}
	})

	t.Run("today the long spelling does not", func(t *testing.T) {
		for _, src := range []string{
			base + "d:\n  ? <<\n  : *a\n  w: 2\n",
			base + "d: {? <<\n  : *a, w: 2}\n",
			base + "d: !foo\n  ? <<\n  : *a\n  w: 2\n",
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t,
				map[string]any{"<<": map[string]any{"x": uint64(1)}, "w": uint64(2)},
				got["d"], "today: the long spelling is an ordinary key: %q", src)
		}
	})

	// The worse half: the answer depends on the destination. Written in flow
	// with the mapping in place, the walk hands "<<" back and the tree merges
	// it, so two callers reading the same document into different destinations
	// get different values.
	t.Run("today the two decode paths disagree in flow", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n{? <<: {x: 1}, w: 2}\n"

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t,
			map[string]any{"<<": map[string]any{"x": uint64(1)}, "w": uint64(2)},
			walked, "today: the walk does not merge it")

		var typed map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &typed))
		assert.Equal(t,
			map[string]any{"x": uint64(1), "w": uint64(2)},
			typed, "today: the tree does merge it")
	})

	t.Run("in block the two paths agree, and neither merges", func(t *testing.T) {
		const src = "%YAML 1.1\n---\nd:\n  ? <<\n  : {x: 1}\n  w: 2\n"

		want := map[string]any{"d": map[string]any{"<<": map[string]any{"x": uint64(1)}, "w": uint64(2)}}

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, want, walked)

		var typed map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &typed))
		assert.Equal(t, want, typed)
	})
}

// TestDefectAMergeKeyAloneInFlowEscapesTheDuplicateCheck pins the inconsistency.
//
// 3.2.1.1 makes two keys that resolve alike one key, and a flow entry written
// as a key alone is an entry like any other: `{a: 1, a}` is refused, and so is
// `{<<: {x: 1}, <<: {y: 2}}`. `{<<: {x: 1}, <<}` is read.
//
// Both readings are wrong here, in two different ways, and the bare "<<" is
// still special-cased under both. Under `%YAML 1.1` the first entry merges and
// the second becomes a literal key, so the mapping holds both the merged entry
// and a "<<" named nothing. Under the core schema, where 8acf11b makes "<<" an
// ordinary key, the two entries are one key spelled "<<" twice: the tree
// refuses the document -- with `duplicate key "<<"` rather than the
// `mapping key "<<" already defined at` an ordinary repeat gets -- and the walk
// reads it, keeping the second entry's null and dropping the first entry's
// mapping. A value goes missing with no error at all, which is the worse half.
//
// The quoted spelling is the control. `{"<<": {x: 1}, "<<"}` is refused by both
// paths under both versions, so the fault sits on the bare "<<" and nothing
// else -- the scanner types it MergeKeyType whatever the version, and the
// duplicate check reads that type. So is the colon: write the second entry
// `<<: ` and every path refuses the document today, under both versions, with
// the ordinary `mapping key "<<" already defined at [1:2]`. One character.
//
// # The ruling, 2026-09-08
//
// Under the core schema the two entries are one key spelled "<<" twice, so the
// duplicate check answers -- and codec.AllowDuplicateMapKey then reads the
// document with the last entry winning, which leaves the empty one standing
// rather than dropping it. Under "%YAML 1.1" the merge key requires its ":", so
// a "<<" written as a flow entry's key alone is invalid merge syntax and the
// document is refused before any duplicate question arises.
//
// Measured on the same day, and the ruling is the strictest of four answers:
// go.yaml.in/yaml/v3 v3.0.5 refuses it under both versions as a repeated key,
// libfyaml 1.0.0b1 reads it and drops an entry with nothing reported, and this
// library reads it and keeps "<<" as an ordinary key beside the merged entries
// -- which nobody else does, since the same two characters then resolve to the
// merge type in one entry and to a string in another.
//
// yamlcorpus.MergeShapes holds both documents and Departures records what we do
// instead, so the gap is written down at both ends.
//
// Found on 2026-09-07 when yamlcorpus's duplicateAKey landed on a merge key --
// the merge axis made that reachable for the first time.
func TestDefectAMergeKeyAloneInFlowEscapesTheDuplicateCheck(t *testing.T) {
	t.Run("an ordinary key alone is refused, and so are two merge keys with values", func(t *testing.T) {
		for _, src := range []string{
			"{a: 1, a}\n",
			"{\"<<\": {x: 1}, \"<<\"}\n",
			"{<<: {x: 1}, <<: {y: 2}}\n",
			"b: &r {x: 1}\nd:\n  <<: *r\n  <<: *r\n",
		} {
			for _, full := range []string{src, "%YAML 1.1\n---\n" + src} {
				var got any
				assert.Errorf(t, codec.Unmarshal([]byte(full), &got), "%q", full)
			}
		}
	})

	t.Run("today a merge key alone is read under YAML 1.1", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n{<<: {x: 1}, <<}\n"

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"<<": nil, "x": uint64(1)}, got,
			"today: the first merges and the second is a literal key")
	})

	t.Run("today a merge key alone is an ordinary key under YAML 1.1", func(t *testing.T) {
		// The 1.1 half with no duplicate in sight, which is where the ruling
		// bites: one "<<" written without its ":". go.yaml.in/yaml/v3 refuses
		// it -- "map merge requires map or sequence of maps as the value" --
		// and libfyaml drops the entry.
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{src: "%YAML 1.1\n---\n{a: 1, <<}\n", want: map[string]any{"a": uint64(1), "<<": nil}},
			{src: "%YAML 1.1\n---\n{<<}\n", want: map[string]any{"<<": nil}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got,
				"today: the merge key with no ':' is read as an ordinary key: %q", tc.src)
		}
	})

	t.Run("with the colon it is refused, which is the whole difference", func(t *testing.T) {
		for _, src := range []string{
			"{<<: {x: 1}, <<: }\n",
			"%YAML 1.1\n---\n{<<: {x: 1}, <<: }\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), `mapping key "<<" already defined`, "%q", src)
		}
	})

	t.Run("today the two paths part company under the core schema", func(t *testing.T) {
		// Nothing merges here, so the two entries are one key written twice
		// and the document should be refused on both paths.
		const src = "{<<: {x: 1}, <<}\n"

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, map[string]any{"<<": nil}, walked,
			"today: the walk reads it and the first entry's mapping is gone")

		var typed map[string]any
		err := codec.Unmarshal([]byte(src), &typed)
		require.Error(t, err, "today: the tree refuses it")
		assert.Contains(t, err.Error(), `duplicate key "<<"`,
			"today: and not the `already defined` an ordinary repeat gets")
	})
}

// TestDefectATabBesideTheMergeKeySuppressesTheMerge pins the tab.
//
// 6.1 puts a tab in s-white and s-separate-in-line is s-white+, so a tab
// separates an indicator from what follows it exactly as a space does. Beside
// the merge key it does not: `<<:<TAB>{m: 1}` comes back as a key named "<<"
// where `<<: {m: 1}` merges, and so does a tab written before the ":". Two
// spaces merge, so the width is not what decides.
//
// The same distinction one indicator earlier is TestFixedATabSeparatesAsASpaceDoes,
// which a0182a6 closed for an anchor, an alias and a tag shorthand. This is the
// piece of that family nobody had looked at.
//
// Older than the version rule: the tab suppressed the merge on master at
// 2abdd2f as well, where every document merged. Nothing could see it, because a
// merge document stated no meaning until 8acf11b made each reading answerable.
// Found on 2026-09-08 by yamlcorpus's
// TestTheLibraryMeansWhatTheCorpusSaysUnderEachReading on its first run after
// that.
func TestDefectATabBesideTheMergeKeySuppressesTheMerge(t *testing.T) {
	const base = "%YAML 1.1\n---\n"

	t.Run("a space merges, and so do two", func(t *testing.T) {
		for _, src := range []string{
			base + "<<: {m: 1}\nk: 1\n",
			base + "<<:  {m: 1}\nk: 1\n",
			base + "<<:\n  m: 1\nk: 1\n",
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"m": uint64(1), "k": uint64(1)}, got, "%q", src)
		}
	})

	t.Run("today a tab does not", func(t *testing.T) {
		for _, src := range []string{
			// After the ":", in block and in flow.
			base + "<<:\t{m: 1}\nk: 1\n",
			base + "{<<:\t{m: 1}, k: 1}\n",
			// Before the ":".
			base + "<<\t: {m: 1}\nk: 1\n",
			// And with the value on the lines below, which is the shape the
			// corpus drew.
			base + "<<:\t\n  m: 1\nk: 1\n",
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"<<": map[string]any{"m": uint64(1)}, "k": uint64(1)}, got,
				"today: the tab leaves an ordinary key named \"<<\": %q", src)
		}
	})

	t.Run("an alias behind the tab is no different", func(t *testing.T) {
		const src = base + "b: &a {m: 1}\nd:\n  <<:\t*a\n  k: 1\n"

		var got map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"<<": map[string]any{"m": uint64(1)}, "k": uint64(1)}, got["d"],
			"today: the alias resolves and the merge does not happen")
	})
}

// TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree pins the split.
//
// `<<:` with no value asks to merge null, which is not a mapping and so not a
// merge at all. The two decode paths answer differently: the walk drops the
// entry and hands back an empty mapping, the tree refuses the document with
// "null was used where mapping is expected".
//
// Every document declares `%YAML 1.1`, since 8acf11b merges under that version
// and no other. Without the directive there is no merge to fail: `<<:` is a key
// named "<<" holding null and both paths agree, which
// TestFixedAMergeKeyIsAnOrdinaryKeyUnderYAML12 pins. Every other non-mapping
// merge -- "<<: 1", "<<: x", "<<: [x]" -- is refused by both paths under 1.1,
// which TestFixedAMergeKeyResolvesUnderYAML11 pins; null and a "-" inside a
// flow sequence are the two shapes left over.
//
// Whichever answer is right, one document should not have two. yamlcorpus's
// MergeShapes holds "a merge key with no alias at all" under TagMergeNonMapping
// for the stance question of what merging a non-mapping means; this is the
// narrower fault of the two paths disagreeing about it.
func TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree(t *testing.T) {
	// A second shape, and a narrower one: the two paths read the element
	// differently rather than the merge. "<<: [{a: 1}, - {b: 2}]" merges both
	// mappings on the walk and is refused by the tree as "sequence was used
	// where mapping is expected", so the tree sees a sequence where the walk
	// sees the mapping.
	//
	// Reached on 2026-09-07 by a mutation that put a "-" inside a merge
	// sequence, once the corpus grew to 3,000 drawn documents.
	t.Run("a '-' inside a flow merge sequence", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n<<: [{a: 1}, - {b: 2}]\n"

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, map[string]any{"a": uint64(1), "b": uint64(2)}, walked,
			"today: the walk merges both")

		var typed map[string]any
		err := codec.Unmarshal([]byte(src), &typed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sequence was used where mapping is expected")
	})

	for _, src := range []string{"<<:\n", "<<: null\n", "a:\n  <<:\n"} {
		full := "%YAML 1.1\n---\n" + src

		var walked any
		assert.NoErrorf(t, codec.Unmarshal([]byte(full), &walked), "today: the walk reads it: %q", src)

		var typed map[string]any
		err := codec.Unmarshal([]byte(full), &typed)
		require.Errorf(t, err, "today: the tree refuses it: %q", src)
		assert.Contains(t, err.Error(), "null was used where mapping is expected")
	}
}

// TestDefectACollectionKeyWrittenAloneInFlowIsRefused pins it.
//
// 7.4.2 lets a flow mapping entry be a key with no value, and lets that key be
// any flow node. `{{"": 0}}` is one entry whose key is the mapping {"": 0}.
//
// The same key with a value parses, so it is the missing value and not the
// collection key. The whole measurement, including why libfyaml cannot answer,
// is on yamlgen.Strict's entry of the same name.
func TestDefectACollectionKeyWrittenAloneInFlowIsRefused(t *testing.T) {
	t.Run("today a collection key alone is refused", func(t *testing.T) {
		for _, src := range []string{"{{\"\": 0}}\n", "{[a]}\n"} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "could not find flow map content", "%q", src)
		}
	})

	t.Run("the same key with a value parses", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("{{a: 0}: v}\n"), &got))
		assert.Equal(t, map[string]any{"map[a:0]": "v"}, got)
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
			want map[string]any
		}{
			{"a:\n: v\n", map[string]any{"a": nil, "null": "v"}},
			{"? a\n: 1\n: v\n", map[string]any{"a": uint64(1), "null": "v"}},
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
