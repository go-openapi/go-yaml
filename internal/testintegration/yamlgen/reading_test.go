// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	"pgregory.net/rapid"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
	"github.com/go-openapi/go-yaml/parser"
)

// plain is the style that writes a scalar unquoted wherever it can, which is
// the only style under which a spelling raises a resolution question.
func plainStyle() yamlgen.Style {
	return yamlgen.Style{Quoting: yamlgen.QuotePlain, Indent: 1, NullSpelling: "null"}
}

// TestAPlainLegacyBooleanGetsASecondReading is the case the field exists for.
func TestAPlainLegacyBooleanGetsASecondReading(t *testing.T) {
	v := yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "k"}, Val: yamlgen.Str{V: "yes"}}}}

	w := yamlgen.Write(v, plainStyle())

	if w.Text != "k: yes\n" {
		t.Fatalf("wrote %q, want %q", w.Text, "k: yes\n")
	}

	want := map[string]any{"k": true}
	if got := w.Readings[yamlgen.Reading11]; !reflect.DeepEqual(got, want) {
		t.Errorf("YAML 1.1 reads %#v, want %#v", got, want)
	}

	if got := v.Decoded(); !reflect.DeepEqual(got, map[string]any{"k": "yes"}) {
		t.Errorf("the core reading moved: %#v", got)
	}
}

// TestQuotingSettlesTheQuestion holds the reason the emitter records and the
// Style is not read off.
//
// The same Value under three styles: quoted twice and written as a block
// scalar once, and none of the three resolves to anything but a string.
func TestQuotingSettlesTheQuestion(t *testing.T) {
	v := yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "k"}, Val: yamlgen.Str{V: "yes"}}}}

	for _, st := range []yamlgen.Style{
		{Quoting: yamlgen.QuoteDouble, Indent: 1, NullSpelling: "null"},
		{Quoting: yamlgen.QuoteSingle, Indent: 1, NullSpelling: "null"},
		{Quoting: yamlgen.QuotePlain, Indent: 1, NullSpelling: "null", Literal: true},
	} {
		w := yamlgen.Write(v, st)
		if len(w.Readings) != 0 {
			t.Errorf("%q raises a resolution question and should not: %v", w.Text, w.Readings)
		}
	}
}

// TestATagSettlesTheQuestionToo checks the other way a spelling stops
// resolving.
func TestATagSettlesTheQuestionToo(t *testing.T) {
	v := yamlgen.Map{Pairs: []yamlgen.Pair{
		{Key: yamlgen.Str{V: "k"}, Val: yamlgen.Tagged{Tag: yamlgen.TagStr, V: yamlgen.Str{V: "yes"}}},
	}}

	if w := yamlgen.Write(v, plainStyle()); len(w.Readings) != 0 {
		t.Errorf("%q is tagged and should raise nothing: %v", w.Text, w.Readings)
	}
}

// TestOneTextWrittenTwoWaysDropsTheReading is the conservative case.
//
// "yes" plain in one entry and as a literal block scalar in another has two
// answers under YAML 1.1 for one spelling, and this package will not guess
// which node the caller meant.
func TestOneTextWrittenTwoWaysDropsTheReading(t *testing.T) {
	// FlowFrom 2 leaves the outer mapping's own values in block style, where a
	// literal block scalar is reachable, and puts the nested mapping's values
	// in flow, where it is not.
	st := plainStyle()
	st.Literal = true
	st.Flow = true
	st.FlowFrom = 2

	v := yamlgen.Map{Pairs: []yamlgen.Pair{
		{Key: yamlgen.Str{V: "block"}, Val: yamlgen.Str{V: "yes"}},
		{Key: yamlgen.Str{V: "flow"}, Val: yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "k"}, Val: yamlgen.Str{V: "yes"}}}}},
	}}

	w := yamlgen.Write(v, st)
	if len(w.Readings) != 0 {
		t.Errorf("%q writes \"yes\" both ways and should state no second reading: %v", w.Text, w.Readings)
	}
}

// TestASecondReadingOnlyArrivesWithALegacySpelling is the property.
//
// A reading is stated for four reasons and no fifth. The document holds one of
// the sixteen spellings YAML 1.1 reads as a boolean and 1.2 does not, written
// plain; or one of the nine it reads as a number and 1.2 reads as the text; or
// it writes a number in a form 1.1 does not read, which Style.NumberForm
// decides; or it writes a "<<" entry, which 1.1 merges and the core schema
// hands back as a key named "<<".
func TestASecondReadingOnlyArrivesWithALegacySpelling(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		v := yamlgen.Values().Draw(rt, "value")
		st := yamlgen.Styles().Draw(rt, "style")

		w := yamlgen.Write(v, st)
		if len(w.Readings) == 0 {
			return
		}

		// A number's form is the style's, so the value alone cannot say
		// whether one was written -- only that there is a number to write.
		if divergentForm(st, v) && holdsNumber(v) {
			return
		}

		// A merge key is written bare whatever Style.Quoting asks for -- see
		// yamlgen.MergeKey -- so it explains a second reading on its own and
		// the quoting check below does not apply to it.
		if holdsAMergeKey(v) {
			return
		}

		if !holdsLegacySpelling(v) {
			rt.Fatalf("a second reading with no legacy spelling in the value\n%q\n%v", w.Text, w.Readings)
		}

		if st.Quoting != yamlgen.QuotePlain {
			rt.Fatalf("a second reading under %s, which quotes every string\n%q", st, w.Text)
		}
	})
}

// holdsAMergeKey reports whether v writes a "<<" entry anywhere.
//
// Restated here rather than exported from yamlgen, for the reason
// holdsLegacySpelling restates the tables: a test that reads the generator's
// own answer back cannot catch the generator being wrong.
func holdsAMergeKey(v yamlgen.Value) bool {
	switch n := v.(type) {
	case yamlgen.Map:
		for _, p := range n.Pairs {
			if _, isMerge := p.Key.(yamlgen.MergeKey); isMerge {
				return true
			}

			if holdsAMergeKey(p.Key) || holdsAMergeKey(p.Val) {
				return true
			}
		}
	case yamlgen.Seq:
		return slices.ContainsFunc(n.Items, holdsAMergeKey)
	case yamlgen.Anchored:
		return holdsAMergeKey(n.V)
	case yamlgen.Alias:
		return holdsAMergeKey(n.V)
	case yamlgen.Tagged:
		return holdsAMergeKey(n.V)
	}

	return false
}

// divergentForm reports the number forms YAML 1.1 may not read as core does.
//
// "0o37" is a string there, and so is a float whose exponent carries no sign.
// A leading zero diverges both ways: 1.1 reads "010" as the octal 8 where core
// reads 10, and reads "09" as the string "09" since 9 is no octal digit. The
// decimal and "+" forms it reads exactly as core does, and hex too.
//
// A BigFloat diverges under every form: its text comes from
// big.Float.Text('g', -1), which writes an exponent with no '.' before it.
func divergentForm(st yamlgen.Style, v yamlgen.Value) bool {
	switch st.NumberForm {
	case yamlgen.NumberOctal, yamlgen.NumberExponent, yamlgen.NumberLeadingZero:
		return true
	}

	return holdsBigFloat(v)
}

// holdsBigFloat reports whether v has a float past what a float64 holds in it.
func holdsBigFloat(v yamlgen.Value) bool {
	switch n := v.(type) {
	case yamlgen.BigFloat:
		return true
	case yamlgen.Seq:
		return slices.ContainsFunc(n.Items, holdsBigFloat)
	case yamlgen.Map:
		for _, p := range n.Pairs {
			if holdsBigFloat(p.Key) || holdsBigFloat(p.Val) {
				return true
			}
		}
	case yamlgen.Anchored:
		return holdsBigFloat(n.V)
	case yamlgen.Alias:
		return holdsBigFloat(n.V)
	case yamlgen.Tagged:
		return holdsBigFloat(n.V)
	}

	return false
}

// holdsNumber reports whether v has a number in it anywhere.
func holdsNumber(v yamlgen.Value) bool {
	switch n := v.(type) {
	case yamlgen.Int, yamlgen.Float, yamlgen.BigInt, yamlgen.BigFloat:
		return true
	case yamlgen.Seq:
		return slices.ContainsFunc(n.Items, holdsNumber)
	case yamlgen.Map:
		for _, p := range n.Pairs {
			if holdsNumber(p.Key) || holdsNumber(p.Val) {
				return true
			}
		}
	case yamlgen.Anchored:
		return holdsNumber(n.V)
	case yamlgen.Alias:
		return holdsNumber(n.V)
	case yamlgen.Tagged:
		return holdsNumber(n.V)
	}

	return false
}

func holdsLegacySpelling(v yamlgen.Value) bool {
	words := map[string]bool{
		"y": true, "Y": true, "yes": true, "Yes": true, "YES": true,
		"on": true, "On": true, "ON": true,
		"n": true, "N": true, "no": true, "No": true, "NO": true,
		"off": true, "Off": true, "OFF": true,
		// The numeric half: 1.1 reads each of these as a number and 1.2 as the
		// text. Written out here rather than read from yamlgen.legacyNumbers,
		// for the reason the booleans above are: a test restating the table is
		// a test that catches an entry going in without a draw to reach it.
		"1_000": true, "-1_0": true, "0b1010": true, "+0b11": true,
		"0x_1F": true, "1:30": true, "190:20:30": true,
		"685_230.15": true, "12:00.5": true,
	}

	switch n := v.(type) {
	case yamlgen.Str:
		return words[n.V]
	case yamlgen.Seq:
		return slices.ContainsFunc(n.Items, holdsLegacySpelling)
	case yamlgen.Map:
		for _, p := range n.Pairs {
			// Keys as well as values: a key is a node now, so a plain "yes:"
			// is the key "true" under YAML 1.1.
			if holdsLegacySpelling(p.Key) || holdsLegacySpelling(p.Val) {
				return true
			}
		}
	case yamlgen.Anchored:
		return holdsLegacySpelling(n.V)
	case yamlgen.Tagged:
		return holdsLegacySpelling(n.V)
	case yamlgen.Alias:
		return holdsLegacySpelling(n.V)
	}

	return false
}

// TestTheNumberFormsMeanUnder11WhatTheLibraryReads holds yamlgen's table of
// YAML 1.1 answers to the library's own 1.1 reader.
//
// The table in reading.go is written out rather than derived, so it can be
// wrong, and being wrong there would mean stating a meaning no implementation
// holds -- the one failure the whole readings mechanism exists to avoid. This
// asks the library the same question for every form the emitter can write.
//
// The library is not the specification, so a disagreement is a question rather
// than a verdict. It has been the right question twice: libfyaml overturned a
// triage done without it, and the reference parser settled the key departure.
func TestTheNumberFormsMeanUnder11WhatTheLibraryReads(t *testing.T) {
	values := []yamlgen.Value{
		yamlgen.Int{V: 0}, yamlgen.Int{V: 1}, yamlgen.Int{V: 31}, yamlgen.Int{V: 511},
		yamlgen.Int{V: 1000}, yamlgen.Int{V: -31},
		// The leading zero's own corners: 9 writes "09", which 1.1 reads as a
		// string, and 7 writes "07", which both read as 7.
		yamlgen.Int{V: 9}, yamlgen.Int{V: 7}, yamlgen.Int{V: 777},
		yamlgen.Float{V: 1.5}, yamlgen.Float{V: -1.5}, yamlgen.Float{V: 0.5},
		yamlgen.Float{V: 1000}, yamlgen.Float{V: 1e-320}, yamlgen.Float{V: 0},
	}

	forms := []yamlgen.NumberForm{
		yamlgen.NumberPlain, yamlgen.NumberSigned,
		yamlgen.NumberHex, yamlgen.NumberOctal, yamlgen.NumberExponent,
		yamlgen.NumberLeadingZero,
	}

	for _, form := range forms {
		st := yamlgen.Style{NullSpelling: "null", NumberForm: form}

		for _, v := range values {
			w := yamlgen.Write(yamlgen.Map{Pairs: []yamlgen.Pair{{Key: yamlgen.Str{V: "k"}, Val: v}}}, st)

			// The core answer first, so a form that changes the value at all
			// fails here rather than quietly in the 1.1 column.
			var core any
			require.NoError(t, yaml.Unmarshal([]byte(w.Text), &core), "%q", w.Text)
			assert.Equal(t, map[string]any{"k": v.Decoded()}, core,
				"%q does not read back as the value it was written from", w.Text)

			var legacy any
			require.NoError(t, yaml.Unmarshal([]byte("%YAML 1.1\n---\n"+w.Text), &legacy), "%q", w.Text)

			want := map[string]any{"k": v.Decoded()}
			if alt, differs := w.Readings[yamlgen.Reading11]; differs {
				want = alt.(map[string]any)
			}

			assert.Equal(t, want, legacy,
				"%q: yamlgen says %v under 1.1 and the library reads %v", w.Text, want, legacy)
		}
	}
}

// TestTheLegacyNumbersMeanUnderElevenWhatTheLibraryReads holds the other half
// of the table, the spellings YAML 1.1 reads as numbers and 1.2 reads as the
// text, to the library's own 1.1 reader.
//
// The same argument as the test above and the same risk: reading.go's
// legacyNumbers is written out from §10.3 and §10.4 of the 1.1 specification --
// the "_" separator, the "0b" prefix, base 60 -- and being wrong there would
// put a meaning in the corpus that no implementation holds.
//
// Each text is asked as a value and as a key, in block and in flow. A key is
// the harder half: "1:30" carries a ':' that is not the entry's, and the name
// the key ends up under moves with the reading, so "1:30: v" is keyed "1:30"
// under core and "90" under 1.1.
//
// A text added to legacyNumbers without a row here is caught by
// TestASecondReadingOnlyArrivesWithALegacySpelling, which restates the whole
// table and fails on a spelling it does not know.
func TestTheLegacyNumbersMeanUnderElevenWhatTheLibraryReads(t *testing.T) {
	for _, tc := range []struct {
		text  string
		under any    // what 1.1 reads the text as
		named string // the name a key resolving to that gets
	}{
		{"1_000", uint64(1000), "1000"},
		{"-1_0", int64(-10), "-10"},
		{"0b1010", uint64(10), "10"},
		{"+0b11", uint64(3), "3"},
		{"0x_1F", uint64(31), "31"},
		{"1:30", uint64(90), "90"},
		{"190:20:30", uint64(685230), "685230"},
		{"685_230.15", 685230.15, "685230.15"},
		{"12:00.5", 720.5, "720.5"},
	} {
		for _, flow := range []bool{false, true} {
			st := yamlgen.Style{NullSpelling: "null", Quoting: yamlgen.QuotePlain, Flow: flow}

			asValue := yamlgen.Map{Pairs: []yamlgen.Pair{
				{Key: yamlgen.Str{V: "k"}, Val: yamlgen.Str{V: tc.text}},
			}}
			readsAs(t, yamlgen.Write(asValue, st),
				map[string]any{"k": tc.text}, map[string]any{"k": tc.under})

			asKey := yamlgen.Map{Pairs: []yamlgen.Pair{
				{Key: yamlgen.Str{V: tc.text}, Val: yamlgen.Str{V: "v"}},
			}}
			readsAs(t, yamlgen.Write(asKey, st),
				map[string]any{tc.text: "v"}, map[string]any{tc.named: "v"})
		}
	}
}

// readsAs checks a written document against both readings: the library reads
// core on its own and 1.1 under a directive, and yamlgen states the second one
// only where it differs.
func readsAs(t *testing.T, w yamlgen.Written, core, under11 any) {
	t.Helper()

	var got any
	require.NoError(t, yaml.Unmarshal([]byte(w.Text), &got), "%q", w.Text)
	assert.Equal(t, core, got, "%q does not read back as the value it was written from", w.Text)

	assert.Equal(t, under11, w.Readings[yamlgen.Reading11],
		"%q: yamlgen states %v under 1.1", w.Text, w.Readings[yamlgen.Reading11])

	var legacy any
	require.NoError(t, yaml.Unmarshal([]byte("%YAML 1.1\n---\n"+w.Text), &legacy), "%q", w.Text)
	assert.Equal(t, under11, legacy,
		"%q: yamlgen says %v under 1.1 and the library reads %v", w.Text, under11, legacy)
}

// TestAMergedKeyIsBeatenByTheOwnKeyThatResolvesToIt guards the accepting side
// of the unique-key work against the merge rule.
//
// A mapping's own keys beat the ones a "<<" brings in, and naming a key by
// [ast.KeyIdentity] rather than by its spelling lets `? [ 1 ]` beat a merged
// `? [1]`: the two resolve to one node, so one overrides the other instead of
// standing beside it.
//
// It is the direction that fails loudly. Had the parser recorded a merged
// mapping's keys in the scope of the mapping holding the "<<", every override
// would be refused as a repeat -- a valid document turned into an error. The
// merged mapping is a node of its own with its own scope, so it is not, and
// this says so rather than leaving it to be inferred.
//
// The corpus draws merges that share a key with what they merge, 34 of 51 as of
// yamlgen/44, but it names those keys with scalars. A collection key and an
// alias key are the shapes this work introduced identity comparison for, and
// neither is drawn there.
//
// "%YAML 1.1" because a bare "<<" is an ordinary key under 1.2, where the merge
// type is not in the schema.
func TestAMergedKeyIsBeatenByTheOwnKeyThatResolvesToIt(t *testing.T) {
	// ⚠️ The three cases here keyed on a sequence, which reached the decoder
	// only while a collection key was named with Go's printing. A collection
	// cannot be a key in a Go map, so the vehicle is a scalar written two ways:
	// the question is whether an own key beats a merged one that *resolves* to
	// it, not whether the two are spelled alike.
	for _, tc := range []struct{ name, src string }{
		{"the same key respelled", "%YAML 1.1\n---\na: &m\n  ? 1\n  : from_merge\n  extra: kept\nb:\n  <<: *m\n  ? !!int 1\n  : own\n"},
		{"quoted against plain", "%YAML 1.1\n---\na: &m\n  ? k\n  : from_merge\n  extra: kept\nb:\n  <<: *m\n  ? \"k\"\n  : own\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)

			b, ok := got["b"].(map[string]any)
			require.Truef(t, ok, "%q", tc.src)
			assert.Lenf(t, b, 2, "%q read %v", tc.src, b)
			assert.Equalf(t, "kept", b["extra"], "the merge-only key is kept: %q", tc.src)
			for name, v := range b {
				if name != "extra" {
					assert.Equalf(t, "own", v, "the own entry wins: %q", tc.src)
				}
			}
		})
	}

	t.Run("and a collection key does not reach the question", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: &m\n  ? [1]\n  : from_merge\nb:\n  <<: *m\n  ? [1]\n  : own\n"

		_, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)

		var got any
		assert.Error(t, codec.Unmarshal([]byte(src), &got), "read %v", got)
	})
}
