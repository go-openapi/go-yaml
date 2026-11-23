// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// asking rewrites a document so that it asks for the version a table reads.
//
// A "%YAML" directive is the only route into 1.1 through codec.Decoder: the
// parser takes parser.WithYAMLVersion, and the decoder has no option that
// passes one through. So the 1.1 consumer is the library reading a document
// that asks for 1.1, which is a real consumer and worth scoring.
// asking returns src rewritten so the table's reading applies to it, and
// reports whether it could be asked at all.
//
// Two kinds cannot. A document that declares its own version: "%YAML 1.1" above
// a "%YAML 1.2" line is two directives, which the library refuses with "YAML
// version has already been specified". And a document opening with a byte order
// mark: 5.2 puts the mark in l-document-prefix, so prepending a directive above
// it leaves the mark inside the document, where it is refused with "found a
// byte order mark where no document begins".
//
// Style.Version and Style.ByteOrderMark write such documents, so the corpus
// holds them and they are left unscored rather than counted as a failure.
func asking(t stance.Table, src []byte) ([]byte, bool) {
	if t.Reads != "yaml-1.1" {
		return src, true
	}

	if bytes.HasPrefix(src, []byte("%YAML")) || bytes.HasPrefix(src, []byte("\ufeff")) {
		return nil, false
	}

	// A document already opening with a marker takes the directive above it;
	// anything else needs the marker too, since a directive has to be followed
	// by one.
	if bytes.HasPrefix(src, []byte("---")) {
		return append([]byte("%YAML 1.1\n"), src...), true
	}

	return append([]byte("%YAML 1.1\n---\n"), src...), true
}

// TestTheLibraryMeansWhatTheCorpusSaysUnderEachReading replays every case that
// states a meaning, against the table that reads it.
func TestTheLibraryMeansWhatTheCorpusSaysUnderEachReading(t *testing.T) {
	header, cases := storedCases(t)

	for _, table := range []stance.Table{yamlcorpus.GoYAML, yamlcorpus.GoYAML11} {
		scored, departed, declared := 0, 0, 0

		for _, c := range cases {
			meaning, states := header.MeaningFor(c, table.Reads)
			if !states || len(meaning.JSON) == 0 {
				continue
			}

			// Only the cases the readings disagree about are worth the rewrite,
			// and prepending a directive to eleven thousand documents would be
			// measuring the prelude rather than the scalars.
			if len(c.Meanings) == 0 {
				continue
			}

			// A root scalar under a 1.1 directive is a recorded departure: the
			// directive does not reach the one scalar directly under it. See
			// TestDefectAVersionDirectiveMissesTheRootScalar, which pins it
			// exactly, and Departures. A meaning that renders as neither an
			// object nor an array is a document whose root is that scalar.
			if table.Reads == yamlcorpus.GoYAML11.Reads && rootIsAScalar(meaning.JSON) {
				departed++

				continue
			}

			// A tab between a node's properties and the node: the parse refuses
			// it after a tag and loses the value after an anchor, where the
			// grammar, the reference parser, libfyaml and go.yaml.in/yaml/v3
			// all read it. The corpus's meaning is right and the library cannot
			// reach it. See the two entries of ledger in yamlgen/ledger_test.go and
			// yamlgen_test.TestDefectATabAfterANodesPropertiesIsMishandled.
			if propertyFollowedByTab(string(c.Src)) {
				declared++

				continue
			}

			// A collection standing as a key. Two open defects live there:
			// TestDefectACollectionKeyWrittenAloneInFlowIsRefused and
			// TestDefectTwoBareColonLinesInARowAreRefused. The library refuses
			// both documents.
			//
			// Three more closed. Two with [ast.KeyIdentity], which names a
			// collection key by what it holds: two block collection keys no
			// longer collide, and "[a]" beside "[ a ]" is now one key rather
			// than two -- see
			// yamlgen.TestFixedACollectionKeyIsNamedByWhatItResolvesTo. The
			// third is the nesting of two '?', see
			// yamlgen.TestFixedAnExplicitKeyInsideAnExplicitKeyReads.
			//
			// Wide on purpose: it holds out every document with a collection
			// key rather than the three shapes, because they are the same
			// region of the parser and it is being repaired. Narrow it when
			// they are closed.
			if holdsACollectionKeyInText(string(c.Src)) {
				declared++

				continue
			}

			src, askable := asking(table, c.Src)
			if !askable {
				declared++

				continue
			}

			scored++

			var got any

			dec := codec.NewDecoder(bytes.NewReader(src))
			if err := dec.Decode(&got); err != nil {
				t.Errorf("%s: %s cannot read it: %v", c.Name, table.Name, err)

				continue
			}

			// Named as the corpus names each key: a mapping with a key that is
			// not a string decodes to a map[any]any, which JSON cannot write.
			encoded, err := yamlgen.MeaningJSON(got)
			if err != nil {
				t.Errorf("%s: %s read something JSON cannot write: %v", c.Name, table.Name, err)

				continue
			}

			if !sameMeaning(meaning.JSON, encoded, slices.Contains(c.Tags, string(yamlbuild.TagInexactFloat))) {
				t.Errorf("%s under %s\n  document: %q\n  library:  %s\n  corpus:   %s",
					c.Name, table.Reads, strings.TrimRight(string(c.Src), "\n"), encoded, meaning.JSON)
			}
		}

		if scored == 0 {
			t.Errorf("%s scored no case at all, so nothing above was exercised", table.Name)
		}

		t.Logf("%s: %d cases scored under %s, %d left to the recorded departure, %d it cannot ask",
			table.Name, scored, table.Reads, departed, declared)
	}
}

// rootIsAScalar reports whether a stated meaning renders as neither an object
// nor an array, which is what a document with a bare scalar at its root
// denotes.
func rootIsAScalar(encoded []byte) bool {
	trimmed := bytes.TrimSpace(encoded)

	return len(trimmed) > 0 && trimmed[0] != '{' && trimmed[0] != '['
}

// TestTheTwoReadingsActuallyDisagree keeps the test above from passing because
// both tables read the same thing.
//
// Every case scored above is one the corpus says the readings disagree about.
// If the library gave the same answer to both, it would be implementing one
// schema and the replay would be scoring it twice.
func TestTheTwoReadingsActuallyDisagree(t *testing.T) {
	header, cases := storedCases(t)

	differed := 0

	for _, c := range cases {
		if len(c.Meanings) == 0 {
			continue
		}

		core, hasCore := header.MeaningFor(c, yamlcorpus.GoYAML.Reads)
		legacy, hasLegacy := header.MeaningFor(c, yamlcorpus.GoYAML11.Reads)

		if !hasCore || !hasLegacy {
			continue
		}

		if string(core.JSON) != string(legacy.JSON) {
			differed++
		}
	}

	if differed == 0 {
		t.Fatal("no case states two different answers, so the replay proves nothing")
	}

	t.Logf("%d cases state a different answer under the two readings", differed)
}

// propertyFollowedByTab reports whether a tag or an anchor ends at a tab.
//
// Crude on purpose, for the reason every hold-out in this suite is: one wrongly
// held out is a document the rest of the suite still scores, and one let through
// reports a known defect as a fresh disagreement.
func propertyFollowedByTab(src string) bool {
	for i := range len(src) {
		if src[i] != '&' && src[i] != '!' {
			continue
		}

		for j := i + 1; j < len(src); j++ {
			if src[j] == '\t' {
				return true
			}

			if src[j] == ' ' || src[j] == '\n' || src[j] == '\r' {
				break
			}
		}
	}

	return false
}

// holdsACollectionKeyInText reports whether a collection stands as a key.
//
// Two spellings reach it: a "?" next to a flow collection's opener, which is
// how a collection key is written inside a flow collection, and a line holding
// nothing but "?", which is where a block collection key goes -- explicitKey
// puts it on the lines below.
func holdsACollectionKeyInText(src string) bool {
	for _, pair := range []string{"{?", "[?", "? {", "? ["} {
		if strings.Contains(src, pair) {
			return true
		}
	}

	for line := range strings.FieldsFuncSeq(src, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if strings.TrimSpace(strings.TrimPrefix(line, "\ufeff")) == "?" {
			return true
		}
	}

	return false
}

// sameMeaning reports whether the library read a document as the corpus says it
// means.
//
// The two are compared as text, which is exact and cheap, and numerically where
// rounds is set: a document carrying more digits than a float64 keeps has a
// meaning this library cannot hold, and rounding it is the stand this table
// declares on representation/float-inexact. The rounding is confined to the
// numbers that differ, so a document holding a long mantissa beside a float
// past float64's range still has the second one compared whole.
func sameMeaning(corpus, library []byte, rounds bool) bool {
	if bytes.Equal(corpus, library) {
		return true
	}
	if !rounds {
		return false
	}

	want, ok := decodeNumbers(corpus)
	if !ok {
		return false
	}
	got, ok := decodeNumbers(library)
	if !ok {
		return false
	}

	return sameRounded(want, got)
}

// decodeNumbers reads JSON with every number left as text, so a number wider
// than a float64 survives the reading that compares it.
func decodeNumbers(src []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(src))
	decoder.UseNumber()

	var out any
	if err := decoder.Decode(&out); err != nil {
		return nil, false
	}

	return out, true
}

// sameRounded compares two decoded meanings, reading a number as the float64 a
// reader holds where the two spell it differently.
func sameRounded(want, got any) bool {
	switch w := want.(type) {
	case json.Number:
		g, ok := got.(json.Number)
		if !ok {
			return false
		}
		if w.String() == g.String() {
			return true
		}
		wf, werr := strconv.ParseFloat(w.String(), 64)
		gf, gerr := strconv.ParseFloat(g.String(), 64)

		return werr == nil && gerr == nil && wf == gf
	case []any:
		g, ok := got.([]any)
		if !ok || len(w) != len(g) {
			return false
		}
		for i := range w {
			if !sameRounded(w[i], g[i]) {
				return false
			}
		}

		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(w) != len(g) {
			return false
		}
		for key, value := range w {
			other, held := g[key]
			if !held || !sameRounded(value, other) {
				return false
			}
		}

		return true
	}

	return want == got
}
