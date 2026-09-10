// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// treeDecoder returns a Decoder that reads src by building a tree.
//
// Decoding into an any takes the walking path, so the side the walk is compared
// against has to be pushed off it. Registering an unmarshaler for chan int does
// that and nothing else: canWalk refuses a Decoder carrying custom
// unmarshalers, no document decodes into a channel, and the parse is left
// alone. CommentToMap would also keep the tree, but reading the comments
// changes which runs of text count as documents.
func treeDecoder(src string) *Decoder {
	return NewDecoder(
		bytes.NewReader([]byte(src)),
		CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }),
	)
}

// decodedStream is every document Decode hands back from a tree, which the walk
// has to reproduce for it to stand in.
func decodedStream(src string) ([]any, error) {
	dec := treeDecoder(src)
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
}

// TestReferenceStreamBuildsATree holds treeDecoder to its name. Without it a
// change to canWalk would leave TestWalkMatchesTheStream comparing the walk
// against itself, and passing.
func TestReferenceStreamBuildsATree(t *testing.T) {
	dec := treeDecoder("a: 1\n")

	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if dec.walkedOK {
		t.Error("the reference decoder walked the source")
	}
	if dec.parsedFile == nil {
		t.Error("the reference decoder built no tree")
	}
}

// corpusSource is one document of the corpus the codec is swept over.
type corpusSource struct{ name, text string }

// corpusSources returns the YAML test suite and the fuzz seeds together.
func corpusSources() []corpusSource {
	var srcs []corpusSource
	suites, _ := yamltestsuite.TestSuites()
	for _, s := range suites {
		srcs = append(srcs, corpusSource{"suite/" + s.Name, string(s.InYAML)})
	}
	seeds, _ := fuzzseeds.All()
	for i, s := range seeds {
		srcs = append(srcs, corpusSource{fmt.Sprintf("seed/%04d", i), s})
	}

	return srcs
}

// directiveNamedLikeAnAnchor reports whether src opens a line with a directive
// whose name begins with "&". 6.8 puts "&" in ns-char, so the name is
// well-formed and the directive is one an implementation ignores.
func directiveNamedLikeAnAnchor(src string) bool {
	// Split on both breaks and take a byte order mark off each line. 5.4 makes
	// a lone "\r" a line break as much as "\n" is, and 5.2 puts a mark in front
	// of the directives -- so a split on "\n" alone read
	// "\ufeff%&YAML 1.2\r---\r!foo \"\"\r" as a single line beginning with the
	// mark, found no directive in it, and let the document through.
	for line := range strings.FieldsFuncSeq(src, isBreak) {
		if strings.HasPrefix(strings.TrimPrefix(line, "\ufeff"), "%&") {
			return true
		}
	}

	return false
}

// mergesNothing reports whether the document writes a "<<" entry with nothing
// after the colon on its line.
//
// The value may still arrive on a following line, so this over-matches -- and
// deliberately: what it guards is a disagreement between two decode paths, and
// a document wrongly held out here is one the rest of the suite still scores.
// A predicate that under-matched would let the disagreement through.
func mergesNothing(src string) bool {
	for line := range strings.FieldsFuncSeq(src, isBreak) {
		// A byte order mark ahead of it and a comment after it, both of which
		// this missed on the first try: "\ufeff<<:" and "<<: # c2" are the same
		// entry as "<<:". A leading mark has now broken three line predicates
		// in this suite -- see directiveNamedLikeAnAnchor for the second.
		line = strings.TrimPrefix(line, "\ufeff")
		if cut := strings.IndexByte(line, '#'); cut >= 0 {
			line = line[:cut]
		}

		if strings.TrimRight(strings.TrimLeft(line, " \t-"), " \t") == "<<:" {
			return true
		}
	}

	return false
}

// isBreak reports the characters 5.4 makes a line break.
func isBreak(r rune) bool { return r == '\n' || r == '\r' }

func TestWalkMatchesTheStream(t *testing.T) {
	srcs := corpusSources()

	var same, differ, bothErr, oneErr, skipped, heldOut int
	for _, src := range srcs {
		if strings.Contains(src.text, "&!") {
			skipped++

			continue
		}
		if strings.Contains(src.text, "? <<") {
			// A merge key written the long way. In flow the tree merges it and
			// the walk hands "<<" back, so the two paths give different values.
			// yamlgen_test.TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge
			// pins both spellings and both paths. Held out until they agree.
			skipped++

			continue
		}
		if strings.Contains(src.text, "<<") && (strings.Contains(src.text, ",-") || strings.Contains(src.text, ", -")) {
			// A "-" inside a flow merge sequence: the tree sees a sequence
			// where the walk sees the mapping, so one refuses and the other
			// merges. Held by the last subtest of
			// yamlgen_test.TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree.
			skipped++

			continue
		}
		if mergesNothing(src.text) {
			// "<<:" with no value asks to merge null. The walk drops the entry
			// and hands back the mapping without it; the tree refuses the
			// document with "null was used where mapping is expected".
			// yamlgen_test.TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree
			// pins both. Held out here until the two paths agree.
			skipped++

			continue
		}
		if directiveNamedLikeAnAnchor(src.text) {
			// "%&AML 1.2" is a directive named "&AML", and the walk reads the
			// "&AML" as an anchor and hands the "1.2" back as the whole
			// document. The tree ignores the directive and reads the document
			// under it. codec.TestDefectADirectiveNamedLikeAPropertyIsReadAsOne
			// pins both. Held out here until the walk is fixed.
			skipped++

			continue
		}

		want, wantErr := decodedStream(src.text)
		got, gotErr := WalkValues([]byte(src.text))

		switch {
		case wantErr != nil && gotErr != nil:
			bothErr++
		case wantErr != nil || gotErr != nil:
			oneErr++
			if oneErr <= 5 {
				t.Logf("%s: stream err=%v walk err=%v src=%q", src.name, wantErr, gotErr, src.text)
			}
		case fmt.Sprintf("%#v", want) == fmt.Sprintf("%#v", got):
			same++
		case walkStreamHoldOuts[src.text] != "":
			heldOut++
		default:
			differ++
			if differ <= 5 {
				t.Logf("%s:\n  src  %q\n  want %#v\n  got  %#v", src.name, src.text, want, got)
			}
		}
		if reason, held := walkStreamHoldOuts[src.text]; held &&
			fmt.Sprintf("%#v", want) == fmt.Sprintf("%#v", got) {
			t.Errorf("%s: %q agrees now, so %s has closed: delete the hold-out", src.name, src.text, reason)
		}
	}
	t.Logf("same=%d differ=%d bothErr=%d oneErr=%d skipped=%d heldOut=%d", same, differ, bothErr, oneErr, skipped, heldOut)
	if differ > 0 || oneErr > 0 {
		t.Errorf("the walk and the tree disagree on %d documents (%d only one failed)", differ, oneErr)
	}
}

// TestStreamSwitchesFromWalkToTree covers a Decoder handed an any for one
// document of a stream and a struct for the next. The first decode read the
// source by walking it, and the second needs the tree the walk never built.
func TestStreamSwitchesFromWalkToTree(t *testing.T) {
	const src = "a: 1\n---\nb: two\n---\nc: 3\n"

	dec := NewDecoder(bytes.NewReader([]byte(src)))

	var first any
	if err := dec.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if !dec.walkedOK {
		t.Fatal("the first document was not walked")
	}
	if got, want := fmt.Sprintf("%v", first), "map[a:1]"; got != want {
		t.Errorf("first document: got %s, want %s", got, want)
	}

	var second struct {
		B string `yaml:"b"`
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if dec.walkedOK {
		t.Error("the Decoder stayed on the walking path")
	}
	if second.B != "two" {
		t.Errorf("second document: got %q, want %q", second.B, "two")
	}

	// The stream carries on from where the walk left it rather than starting
	// again: the tree is read from the same source and indexed the same way.
	var third any
	if err := dec.Decode(&third); err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprintf("%v", third), "map[c:3]"; got != want {
		t.Errorf("third document: got %s, want %s", got, want)
	}

	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Errorf("after the last document: got %v, want io.EOF", err)
	}
}

// walkStreamHoldOuts are the documents the walk and the tree disagree about,
// with the defect that explains each.
//
// Keyed on the source text and not on the corpus name, because a regeneration
// renames every seed. A held-out document that starts agreeing fails, so the
// entry reports the fix instead of outliving it.
var walkStreamHoldOuts = map[string]string{
	"%TAG !x! tag:yaml.org,2002:\r---\r&a1 !x!pairs\r- !x!omap [:]\r": "defect 109: a null key in an " +
		"!!omap is named \"null\" by the walk and nil by the tree",
}
