// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// Scoring the library under each reading it implements.
//
// The tables differ in one field. Every verdict is the same -- "0777" is a
// valid document whichever schema resolves it -- so nothing about
// accept-or-refuse moves, and a corpus that only recorded verdicts would score
// both consumers identically and find nothing. What moves is the value.

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
			// reach it. See yamlgen.Ledger's two entries and
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

			// A "<<" the library does not resolve as a merge, under two open
			// defects: TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge, where
			// the entry is written "? <<" over ": *a", and
			// TestDefectATabBesideTheMergeKeySuppressesTheMerge, where a tab
			// stands beside the "<<" or its ":". The corpus states the merge
			// and the library hands "<<" back as a key.
			//
			// Both spellings, not every merge: a "<<" written plainly with a
			// space is scored, and the second defect was found by scoring it.
			if aMergeKeyTheLibraryLeavesAlone(string(c.Src)) {
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

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Errorf("%s: %s read something JSON cannot write: %v", c.Name, table.Name, err)

				continue
			}

			if string(encoded) != string(meaning.JSON) {
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
	for i := 0; i < len(src); i++ {
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

// aMergeKeyTheLibraryLeavesAlone reports whether a "<<" entry is written one of
// the two ways the library does not resolve as a merge.
//
// The long form is "? <<", with a space or a tab after the "?" -- explicitKey
// writes both. The tab form is a tab touching the "<<" or the ":" after it,
// which Style.TabSeparation puts there.
func aMergeKeyTheLibraryLeavesAlone(src string) bool {
	for _, spelling := range []string{"? <<", "?\t<<", "<<\t", "<<:\t"} {
		if strings.Contains(src, spelling) {
			return true
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
