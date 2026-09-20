// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/go-openapi/conformance-suites/grammar"
	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"

	_ "embed"
)

// Generator names what produced a corpus, so a change here is as visible in an
// artifact's header as a change to the grammar.
const Generator = "yamlcorpus/46"

// Build is the recipe for a corpus: how much to draw, and how much to keep.
type Build struct {
	// Tier labels the result.
	Tier string
	// Seed reproduces the whole thing.
	Seed uint64
	// Documents is how many valid documents to draw.
	Documents int
	// MutantsEach is how many times to break each of them.
	MutantsEach int
	// PerSignature is how many documents to keep per coverage signature, and
	// zero means keep every one.
	PerSignature int
}

// Smoke is the corpus that lives in the repository.
//
// Every number here is measured, and the measurement changed which knob to
// turn. See TestMeasureK.
//
// The quota stops paying at sixteen: replaying each build and counting the
// *distinct* complaints the library makes gives 14 at K=1, 19 at 4, 22 at 16
// and 23 at 32. The last doubling buys one complaint for 37KB.
//
// Drawing more documents beats raising the quota outright, and not marginally.
// Keeping everything at 1500 documents finds 27 complaints in 535KB; twice the
// mutants with the quota still at sixteen finds 29 in 395KB. So the corpus
// minimizes hard and generates more, which is the opposite of what the quota
// curve on its own suggests.
//
// The reason is in the shape of what is being looked for. Most complaints are
// singletons -- one document in twenty thousand -- so whether a quota keeps one
// is luck rather than policy, and no cleverness in the sampling helps. A
// thinning sample that took every power-of-two member of a group was tried and
// found exactly nothing extra. What finds more of them is more documents.
// Re-measured on 2026-09-07, when the keep decision gained the construct set.
// Four configurations at the same quota, and 3,000 documents against 16 mutants
// wins on every axis but size:
//
//	docs   mutants   cases    size    construct sets   complaints   "- : |1"
//	1500        24   15,603   639KB            2,546           64          1
//	3000        16   20,954   955KB            3,419           70         52
//	4000        12   21,809  1057KB            3,645           68         49
//	6000         8   21,515  1167KB            4,097           66         41
//
// The last column is the shape that started this: an entry with no key whose
// value is a block scalar, drawn about one document in 588. At 1,500 documents
// the corpus held one by luck and had held none the build before. Fewer mutants
// each does not cost complaints -- 70 against 64 -- because what finds a
// complaint is more documents, which the paragraph above already said.
func Smoke() Build {
	return Build{Tier: "smoke", Seed: 1, Documents: 3000, MutantsEach: 16, PerSignature: 16}
}

// Full is the corpus that ships as a release artifact, where nothing is
// discarded.
//
// Bigger on both axes than the smoke tier, because nothing here has to fit in a
// repository: four times the documents and no minimizing at all, which finds 50
// distinct complaints against the smoke tier's 29, in 2.2MB.
//
// That number is still climbing. The library can make around 55 distinct
// complaints while reading a document, counted from its own source, so a corpus
// finding 50 has found nine tenths of them -- and the way to find the rest is
// more documents rather than a different recipe.
func Full() Build {
	return Build{Tier: "full", Seed: 1, Documents: 6000, MutantsEach: 12, PerSignature: 0}
}

// Write generates a corpus and writes it as an artifact.
func (b Build) Write(w io.Writer) error {
	cases := b.cases()

	header := suite.Header{
		Grammar:   grammar.YAML.Name(),
		Digest:    grammar.YAML.Digest(),
		Generator: Generator,
		Seed:      b.Seed,
		Tier:      b.Tier,
		Cases:     len(cases),
		Vocabulary: suite.SpecsFor(
			vocabularyOf(cases), Vocabulary(), allRules()),
		Features: featuresOf(cases),
		// Every reading readingsOfEntry and readingsOfResolution consult. The
		// JSON schema is deliberately absent -- see schema.go.
		Readings: []string{Reading, yamlgen.Reading11},
	}

	out, err := suite.NewWriter(w, header)
	if err != nil {
		return err
	}

	for _, c := range cases {
		if err := out.Add(c); err != nil {
			return err
		}
	}

	return out.Close()
}

func (b Build) minimizing() bool { return b.PerSignature > 0 }

// cases puts the enumerated families in first and never discards them, then
// draws documents and keeps the ones that earn a place.
//
// The order matters and the exemption matters more. No coverage signal points
// at an anchor pattern or a schema shape -- an alias reaches the same
// productions whether it resolves or not, and every plain scalar reaches the
// same ones whatever it denotes -- so a minimizer selecting on what the grammar
// saw would keep one of each family and call the rest duplicates.
func (b Build) cases() []suite.Case {
	out := Cases()

	rec := grammar.NewRecognizer(4096)
	seen := grammar.YAML.NewCoverage()
	one := grammar.YAML.NewCoverage()
	kept := map[string]int{}
	// The construct sets already held, so a document bringing a new combination
	// survives the quota. See the keep decision below.
	seenConstructs := map[string]bool{}

	for _, e := range Generate(b.Seed, b.Documents, b.MutantsEach) {
		one.Reset()
		rec.Cover(one)
		wellFormed := rec.Stream(e.Src).OK
		rec.Cover(nil)

		signature := fmt.Sprintf("%x", one.Signature())
		covers := one.AddsTo(seen)
		shape := ConstructSet(e.Src)
		meaning := meaningOfEntry(e, wellFormed)

		// A document carrying a meaning is exempt from the quota, for the same
		// reason the enumerated shapes are: the signature cannot see what makes
		// it worth keeping.
		//
		// A signature fingerprints the route the *grammar* took, and two
		// documents that took the same route and denote different things are
		// one test for a recognizer and two for a decoder. Before meanings were
		// carried this cost nothing, because the two were genuinely
		// interchangeable. Now it would throw away the entire value half of the
		// corpus -- fifteen hundred drawn documents came out as two hundred and
		// fifty-nine, and the ones discarded were not duplicates of anything.
		//
		// Mutants are a different case and keep the quota. What a mutant
		// carries *is* its route: nobody knows what it denotes, which is why it
		// carries no meaning, so two mutants that fail identically really are
		// one test wearing two disguises.
		//
		// The construct set is the second half of the decision, added
		// 2026-09-07, and it is what the route alone could not do. A signature
		// fingerprints which productions ran, and that axis is saturated -- the
		// corpus enters 605 of 605 buckets -- so it cannot tell a document
		// holding a tab from one holding a space, or an explicit key whose
		// content is on the next line from one whose content is on the same
		// line. Both take the same route. A document whose combination of
		// constructs nothing before it held is kept whatever its route was.
		//
		// It is an *or* rather than a replacement: the route still keeps
		// everything it kept, and this keeps what the route was blind to.
		novel := !seenConstructs[shape]
		if b.minimizing() && meaning == nil && !covers && !novel && kept[signature] >= b.PerSignature {
			continue
		}

		seenConstructs[shape] = true

		seen.Merge(one)
		kept[signature]++

		out = append(out, suite.Case{
			Name:       e.Name,
			Src:        e.Src,
			WellFormed: wellFormed,
			VerdictAt:  verdictAt(e),
			Opaque:     !utf8.Valid(e.Src),
			Tags:       append(append(encodingTags(e.Src), representationNames(e.Value)...), names(e.Tags)...),
			Features:   featureNames(e.Features),
			Meaning:    meaning,
			Meanings:   readingsOfEntry(e, meaning),
			Origin: suite.Origin{
				Document:  -1,
				Mutation:  e.Mutation,
				Signature: signature,
			},
		})
	}

	return out
}

// meaningOfEntry states what a generated document denotes, which is free
// because the value came before the text.
//
// This is the half of the corpus a verdict was always going to be enough for,
// and it costs nothing to carry more: the generator drew a value and asked the
// emitter to write it, so the expected decode is already in hand. A mutant
// carries none -- whatever the mutation left behind is exactly what nobody
// knows.
func meaningOfEntry(e Entry, wellFormed bool) *suite.Meaning {
	// Declined, and the fallback below must not undo it. An entry whose
	// generator would not say what the document means is not the same as one
	// that carries no meaning to begin with.
	if e.MeansUnclear {
		return nil
	}

	// Entry.Means rather than Entry.Value.Decoded(), because a document may
	// declare which schema reads it: yamlgen.Style.Version writes a
	// "%YAML 1.1" line, and "yes" is the boolean true under it. Nil where the
	// generator will not say what the document means.
	//
	// Through yamlgen.NamedKeys, here and below: a mapping with a key that is
	// not a string decodes to a map[any]any, which encoding/json refuses, and
	// the corpus names each key as KeyText does.
	if e.Means != nil && wellFormed {
		encoded, err := json.Marshal(yamlgen.NamedKeys(e.Means))
		if err != nil {
			return nil
		}

		return &suite.Meaning{Under: Reading, JSON: encoded}
	}

	if e.Value == nil || !wellFormed {
		return nil
	}

	encoded, err := json.Marshal(yamlgen.NamedKeys(e.Value.Decoded()))
	if err != nil {
		return nil
	}

	return &suite.Meaning{Under: Reading, JSON: encoded}
}

// readingsOfEntry states what a generated document denotes under each reading,
// and only where they disagree.
//
// The core answer is repeated among them so that a consumer matching on
// stance.Table.Reads looks in one place. Where every reading agrees there is
// nothing to choose between and the single Meaning says it.
func readingsOfEntry(e Entry, core *suite.Meaning) []suite.Meaning {
	if core == nil || len(e.Readings) == 0 {
		return nil
	}

	out := []suite.Meaning{*core}

	for _, name := range slices.Sorted(maps.Keys(e.Readings)) {
		encoded, err := json.Marshal(yamlgen.NamedKeys(e.Readings[name]))
		if err != nil {
			return nil
		}

		out = append(out, suite.Meaning{Under: name, JSON: encoded})
	}

	return out
}

// representationNames reports the representation questions a generated
// document raises, as the artifact spells them.
//
// It reads the drawn value, so a mutant contributes nothing: its Value is nil.
func representationNames(v yamlgen.Value) []string {
	if v == nil {
		return nil
	}

	return names(representationTags(v))
}

// encodingTags reports what a document's bytes exhibit, which for a generated
// one is usually nothing and for a mutant is occasionally a broken rune.
func encodingTags(src []byte) []string {
	var out []string

	if !utf8.Valid(src) {
		out = append(out, string(stance.TagNotUTF8))
	}

	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		out = append(out, string(stance.TagBOM))
	}

	return out
}

// featuresOf collects every feature name the corpus uses, sorted.
func featuresOf(cases []suite.Case) []string {
	var out []string

	for _, c := range cases {
		for _, f := range c.Features {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}

	slices.Sort(out)

	return out
}

func featureNames(features []stance.Feature) []string {
	if len(features) == 0 {
		return nil
	}

	out := make([]string, 0, len(features))
	for _, f := range features {
		out = append(out, string(f))
	}

	return out
}

// vocabularyOf collects every tag name the corpus uses.
func vocabularyOf(cases []suite.Case) []string {
	var out []string

	for _, c := range cases {
		for _, tag := range c.Tags {
			if !slices.Contains(out, tag) {
				out = append(out, tag)
			}
		}
	}

	slices.Sort(out)

	return out
}

// Reading names the resolution the corpus states a meaning under.
//
// The specification's own default, not ours. A consumer implementing the JSON
// schema or YAML 1.1 disagrees about some of these and is told which by the
// tags, rather than by the corpus trying to enumerate every implementation's
// answer.
const Reading = yamlgen.ReadingCore

// Cases turns the enumerated families into corpus cases.
//
// Both families here are ones a verdict cannot express, which is why they are
// the first to be built rather than the last. An anchor pattern is a document
// the grammar accepts and a conforming consumer may have to refuse; a schema
// shape is a document every reading accepts and they disagree about what it
// says. Neither is reachable by generating documents and asking the oracle, so
// neither would ever appear in a corpus that only did that.
//
// The verdict is still recorded, and it is still the grammar's. What the
// construction adds is the label the grammar could not produce.
//
// None of these carries a Feature, and the omission is deliberate. A feature is
// derived by the emitter from a Value and a Style, and these documents have
// neither -- somebody wrote the bytes. Reading them back out with a scanner is
// exactly the unsound step TestEveryMarkInTheBytesIsLabeled has to guard
// against, so a filter on features selects among the generated documents and
// leaves these to the tags, which say more about them anyway.
func Cases() []suite.Case {
	out := make([]suite.Case, 0,
		len(Patterns())+len(Resolutions())+len(TagShapes())+len(KeyShapes())+
			len(MergeShapes())+len(DirectiveShapes())+len(SeparationShapes())+len(ReachShapes()))

	rec := grammar.NewRecognizer(4096)
	a := Corpus()

	for i, p := range Patterns() {
		src := p.build(a)

		out = append(out, suite.Case{
			Name:       "shape/anchor/" + p.Name,
			Src:        src,
			WellFormed: rec.Stream(src).OK,
			VerdictAt:  stance.Construct.String(),
			Tags:       names(p.Exhibits),
			Meaning:    meaningOfPattern(p),
			Origin:     suite.Origin{Document: i, Mutation: "enumerated"},
		})
	}

	// Every family but the anchors and the resolutions builds a case the same
	// way, so one helper builds them all: a family wired in without its meaning
	// was the mistake worth making impossible.
	for _, fam := range []struct {
		prefix string
		shapes []stance.Shape
	}{
		{"shape/tag/", TagShapes()},
		{"shape/key/", KeyShapes()},
		{"shape/merge/", MergeShapes()},
		{"shape/directive/", DirectiveShapes()},
		{"shape/separation/", SeparationShapes()},
		// The reach shapes carry no tags and raise no question. They exist
		// because the grammar has corners the rest of the corpus does not turn.
		{"shape/reach/", ReachShapes()},
	} {
		out = append(out, shapeCases(fam.prefix, fam.shapes, rec)...)
	}

	for i, r := range Resolutions() {
		src := []byte("k: " + r.Scalar + "\n")

		out = append(out, suite.Case{
			Name:       "shape/schema/" + r.Scalar,
			Src:        src,
			WellFormed: rec.Stream(src).OK,
			VerdictAt:  stance.Construct.String(),
			Tags:       names(r.Exhibits),
			Meaning:    meaningOfResolution(r),
			Meanings:   readingsOfResolution(r),
			Origin:     suite.Origin{Document: i, Mutation: "enumerated"},
		})
	}

	return out
}

// shapeCases turns an enumerated family into cases.
//
// [stance.Shape.Means] becomes the case's meaning, where the specification
// settles what the document denotes. A shape leaving it nil states none, and
// the tags say which question the language leaves open instead.
func shapeCases(prefix string, shapes []stance.Shape, rec *grammar.Recognizer) []suite.Case {
	out := make([]suite.Case, 0, len(shapes))

	for i, s := range shapes {
		out = append(out, suite.Case{
			Name:       prefix + s.Name,
			Src:        s.Src,
			WellFormed: rec.Stream(s.Src).OK,
			VerdictAt:  stance.Construct.String(),
			Tags:       names(s.Intent),
			Meaning:    meaningOfShape(s),
			Origin:     suite.Origin{Document: i, Mutation: "enumerated"},
		})
	}

	return out
}

// meaningOfShape states what an enumerated shape denotes, and nil where the
// specification does not settle it.
func meaningOfShape(s stance.Shape) *suite.Meaning {
	if s.Means == nil {
		return nil
	}

	encoded, err := json.Marshal(s.Means)
	if err != nil {
		return nil
	}

	return &suite.Meaning{Under: Reading, JSON: encoded}
}

// meaningOfPattern states what an anchor pattern denotes, where it denotes
// anything.
//
// A pattern the specification says must be refused denotes nothing, so it
// carries no meaning -- recording one would be recording what a document would
// have meant had it been legal. A cyclic one carries the flag and no JSON,
// because there is no JSON to carry.
func meaningOfPattern(p Pattern) *suite.Meaning {
	if !p.Valid {
		return nil
	}

	if slices.Contains(p.Exhibits, TagCyclicMeaning) {
		return &suite.Meaning{Under: Reading, Cyclic: true}
	}

	return nil
}

// meaningOfResolution states what a plain scalar denotes under the core schema.
//
// This is the whole point of the schema family reaching the artifact. The
// document is valid under every reading, so the verdict says nothing at all,
// and the disagreement is entirely in this field.
func meaningOfResolution(r Resolution) *suite.Meaning {
	value, ok := coreValue(r)
	if !ok {
		return nil
	}

	encoded, err := json.Marshal(map[string]any{"k": value})
	if err != nil {
		return nil
	}

	return &suite.Meaning{Under: Reading, JSON: encoded}
}

// coreValue is what YAML 1.2's core schema resolves a scalar to.
//
// Written out rather than computed, because computing it would mean
// implementing the core schema here and then testing our implementation of it
// against itself. These are the specification's answers, read off its regular
// expressions by hand, and a reader who disagrees with one has something
// concrete to disagree with.
//
// The floats that JSON cannot write are absent rather than approximated:
// there is no JSON for infinity or a NaN, so a meaning stated in JSON has
// nothing to say about them and says nothing instead.
func coreValue(r Resolution) (any, bool) {
	switch r.Scalar {
	case "yes", "no", "on", "off", "y", "Yes":
		// Strings under 1.2. Booleans under 1.1, which is what the tag says.
		return r.Scalar, true
	case "0777":
		// Core's integer is decimal here: octal is spelled 0o777, and a
		// leading zero buys nothing. 1.1 read it as 511.
		return 777, true
	case "1_000", "0b1010", "1:30", "12:34:56", "2001-12-14":
		// None of these is a number in 1.2. All of them were in 1.1.
		return r.Scalar, true
	case "0x1A":
		return 26, true
	case "0o17":
		return 15, true
	case "1e3":
		// A float under core and under JSON, whatever 1.1 said.
		return 1000.0, true
	default:
		return nil, false
	}
}

// Corpus is the material the anchor patterns are composed around.
//
// Fixed for now, and deliberately not minimal: a pattern composed around a
// one-line document would not demonstrate that composing leaves the document
// alone. It becomes generated once the YAML generator can hand over a block
// mapping written at column zero, and the patterns do not have to change when
// it does.
func Corpus() Around {
	return Around{
		Document:   []byte("kept:\n  nested: 1\n  list:\n    - a\n    - b\nalso: |\n  literal\n  content\n"),
		Scalar:     []byte("plain text"),
		Other:      []byte("\"quoted\""),
		Collection: []byte("[1, 2]"),
	}
}

func names(tags []stance.Tag) []string {
	if len(tags) == 0 {
		return nil
	}

	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, string(t))
	}

	return out
}

//go:embed testdata/yaml-smoke.jsonl.gz
var smokeArtifact []byte

// SmokeSuite returns the corpus checked in beside this package.
//
// The second way an artifact is consumed. Released, the same file is read by
// anyone with a JSON reader and base64; embedded, it is a corpus a Go test gets
// with one call and no generation, no oracle and no fuzzing.
func SmokeSuite() (suite.Header, []suite.Case, error) {
	return suite.FromBytes(smokeArtifact)
}

// verdictAt says how far a generated case's verdict can be trusted.
//
// A document emitted whole is trustworthy all the way: it was written from a
// value the generator holds, so its anchors resolve and its keys are distinct
// by construction, and there is nothing for a later stage to discover.
//
// A mutant is not. The mutation may have broken an anchor or duplicated a key,
// leaving a document the grammar accepts and a composer must refuse, and
// nothing here can tell which mutations did that. So its acceptance is claimed
// for parsing and no further -- while its *refusal*, which is most of what a
// mutant is worth, still counts at every stage.
func verdictAt(e Entry) string {
	// A document emitted whole is trustworthy all the way, and so is one broken
	// on purpose: the break was made on the value, so what it violates is known
	// and carried as a tag rather than left for a later stage to discover.
	if e.Mutation == "" || len(e.Tags) > 0 {
		return stance.Construct.String()
	}

	return stance.Parse.String()
}

// allRules is everything the specification settles that this corpus can label.
func allRules() stance.Rules {
	out := AnchorRules()
	out = append(out, TagRules()...)
	out = append(out, KeyRules()...)

	return append(out, DirectiveRules()...)
}

// readingsOfResolution states what a plain scalar denotes under each reading
// that has an answer.
//
// This is the family the field exists for. The document is valid under every
// reading, so the verdict says nothing at all and the whole disagreement is
// here: "0777" is 777 under the core schema and 511 under YAML 1.1, and a
// corpus storing one of those scores a conforming reader of the other as
// broken.
//
// A reading with no JSON rendering is left out rather than approximated: ".inf"
// is a float under the core schema and under 1.1, and JSON writes neither, so
// both say nothing here and the tag carries the disagreement on its own.
//
// The JSON schema states nothing at all. See the note beside jsonValue's
// removal in schema.go: §10.2.2 may make those scalars a refusal rather than a
// string, and a refusal is a verdict that no meaning can express.
func readingsOfResolution(r Resolution) []suite.Meaning {
	answers := []struct {
		under string
		value any
		ok    bool
	}{
		{under: Reading},
		{under: yamlgen.Reading11},
	}

	core, hasCore := coreValue(r)
	answers[0].value, answers[0].ok = core, hasCore

	if r.Legacy == "" {
		// Empty means 1.1 and the core schema agree.
		answers[1].value, answers[1].ok = core, hasCore
	} else {
		answers[1].value, answers[1].ok = legacyValue(r)
	}

	var out []suite.Meaning

	for _, a := range answers {
		if a.ok {
			out = append(out, suite.Meaning{Under: a.under, JSON: mustEncodeScalar(a.value)})
		}
	}

	if len(out) == 0 || nothingToChoose(out) {
		return nil
	}

	return out
}

// nothingToChoose reports whether every reading with an answer gives the same
// one and the core reading is among them, which is the case [suite.Case.Meaning]
// already covers on its own.
func nothingToChoose(out []suite.Meaning) bool {
	var core bool

	for _, m := range out {
		if m.Under == Reading {
			core = true
		}

		if !bytes.Equal(m.JSON, out[0].JSON) {
			return false
		}
	}

	return core
}

// mustEncodeScalar renders one resolved scalar as the document it stands in.
//
// The shapes write the scalar as a mapping value, so the meaning is the mapping
// and not the scalar alone.
func mustEncodeScalar(v any) []byte {
	encoded, err := json.Marshal(map[string]any{"k": v})
	if err != nil {
		// Every value here is a Go string, bool, int or float, all of which
		// encode.
		panic("yamlcorpus: a resolved scalar that JSON cannot render: " + fmt.Sprint(v))
	}

	return encoded
}
