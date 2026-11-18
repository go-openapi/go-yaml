// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"math/rand/v2"

	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// Generating documents for the corpus.
//
// # Drawing from yamlgen, deterministically
//
// The values and the styles are yamlgen's, drawn through rapid's Example, which
// is documented to produce a value deterministically from a seed. That is what
// an artifact needs and what a *rapid.T inside a property check could not give:
// a corpus claims a seed reproduces it byte for byte, and rapid owning the
// stream would make the claim unverifiable.
//
// Example carries a caveat -- it is meant for examples rather than for property
// tests -- and one more this file has to live with: nothing promises the values
// are the same across rapid versions. That is survivable here because it is
// *detected*. The stored corpus is regenerated and diffed on every run, so a
// rapid upgrade that moved the documents fails loudly and gets a deliberate
// regeneration and a Generator bump, rather than quietly producing a different
// corpus for whoever built it last.
//
// The toolchain used to be a second undetected axis. rapid.String draws from the
// standard library's unicode tables, which carry a Unicode version, so Go 1.27
// drew different characters from Go 1.25. yamlgen.Runes owns its ranges by
// number now -- see yamlgen/runes.go -- and yamlgen's own digest test fails
// before this one does.
//
// # Why yamlgen's generator and not one written here
//
// The first version of this file drew its own values, from an alphabet chosen
// for characters the grammar finds awkward. It was worse in the way that
// matters: it produced no anchors at all. yamlgen's Values wraps its trees in
// withAliases, so about a quarter of the documents carry an anchor and an alias
// -- and its awkwardStrings covers everything that alphabet did and adds the
// schema boundaries and the document markers besides.
//
// The mutations keep their own generator. Breaking a document is not something
// yamlgen has a seeded form of, and a plain PCG is the whole of what it needs.

// Entry is one generated document, with where it came from.
type Entry struct {
	// Name identifies it, and is stable for a given seed.
	Name string
	// Src is the document.
	Src []byte
	// Value is what it means, known because the value came first. Nil for a
	// mutant, which means whatever the mutation left behind.
	Value yamlgen.Value
	// Mutation names how it was broken, empty for a document emitted whole.
	Mutation string
	// Features are the constructs the emitter wrote into it.
	//
	// Empty for a byte mutant, and that is the honest answer rather than a
	// missing one: the mutation may have deleted the bracket the document was
	// labeled for, and nothing here can tell which mutations did. It is the
	// same reasoning that caps a mutant's VerdictAt at parsing.
	Features []stance.Feature
	// Readings is what the document denotes under each reading that disagrees
	// with the core schema, empty where they all agree.
	Readings map[string]any
	// Means is what the document denotes given the version it declares, and
	// Value.Decoded() for one that declares none. Nil where the generator will
	// not say -- see yamlgen.Written.MeansUnclear.
	Means any
	// MeansUnclear says the generator declined to state a meaning, as opposed
	// to an entry that simply carries none.
	//
	// The two have to be told apart, and telling them apart is a fix rather
	// than a nicety: meaningOfEntry falls back to Value.Decoded() for an entry
	// with no Means, which is right for an enumerated shape and wrong here --
	// it put the declined answer back. Nothing noticed until the merge axis,
	// because the only other shape that declines is a legacy spelling split
	// under "%YAML 1.1" and the corpus has few of those.
	MeansUnclear bool
	// Tags are the rules this document breaks, where it was broken on purpose
	// and the break is therefore known.
	//
	// Only the value-level breaks carry them. A byte mutation cannot say what
	// it broke, which is why its acceptance is claimed for parsing and no
	// further -- see suite.Case.VerdictAt.
	Tags []stance.Tag
}

// meansOf is what a written document denotes, and nil where the generator will
// not say.
func meansOf(w yamlgen.Written) any {
	if w.MeansUnclear {
		return nil
	}

	return w.Means
}

// Generate draws documents from a seed and breaks each of them.
//
// Meaning first, then presentation: a value is drawn, a style is drawn, and the
// document is what the emitter writes. That order is what makes the expectation
// free -- generating text first would leave nothing to compare a decode against.
func Generate(seed uint64, documents, mutantsEach int) []Entry {
	rng := rand.New(rand.NewPCG(seed, 0x59414d4c)) //nolint:gosec // a seeded generator, so a corpus build is reproducible; nothing here is secret

	values := yamlgen.Values()
	styles := yamlgen.Styles()
	streams := yamlgen.Streams()

	out := make([]Entry, 0, documents*(1+mutantsEach))

	for i := range documents {
		// One stream of example seeds per corpus seed, so that two corpora
		// built from different seeds share no documents rather than sharing a
		// prefix.
		at := int(seed)*1_000_003 + i

		value := values.Example(at)
		style := styles.Example(at)
		written := yamlgen.Write(value, style)
		src := []byte(written.Text)

		out = append(out, Entry{
			Name:         "generated/" + digits(i),
			Src:          src,
			Value:        value,
			Features:     written.Features,
			Readings:     written.Readings,
			Means:        meansOf(written),
			MeansUnclear: written.MeansUnclear,
		})

		// One document in eight also yields a stream, which is the only way
		// this corpus reaches a document suffix, a second document, or the
		// grammar's l-document-prefix past the first one.
		//
		// It states no meaning. A stream denotes a *sequence* of documents and
		// suite.Case.Meaning is what one document denotes, so writing one in
		// would be answering a different question. What a stream is here for is
		// the shape: the buckets it enters, the complaints its mutants provoke,
		// and the verdict on whether it is YAML at all.
		if i%8 == 0 {
			docs := streams.Example(at)
			asStream := yamlgen.WriteStream(docs, style)

			out = append(out, Entry{
				Name:     "generated/" + digits(i) + "/stream",
				Src:      []byte(asStream.Text),
				Features: asStream.Features,
			})
		}

		// Broken on purpose, on the value, so the break is labeled rather
		// than guessed at. These are the only generated documents that violate
		// a rule the grammar cannot see.
		//
		// They state no meaning either, and for a sharper reason than the
		// stream above. The break is on the value, so meansOf would answer
		// from the rewritten tree -- and that tree has already resolved the
		// question the break opens. A repeated key collapses in a Go map, so
		// the meaning would record last-write-wins as settled; an alias
		// pointing at no anchor still carries the node it was copied from, so
		// the meaning would record a reference nothing defines. TagDuplicateKey
		// and TagAliasUndefined are on the case to let a consumer say what it
		// does about the rule. A stated meaning would answer for it.
		for _, b := range breakRules(value) {
			broken := yamlgen.Write(b.Value, style)

			out = append(out, Entry{
				Name:     "generated/" + digits(i) + "/" + b.How,
				Src:      []byte(broken.Text),
				Mutation: b.How,
				Features: broken.Features,
				Tags:     b.Tags,
			})
		}

		for j := range mutantsEach {
			broken, how := mutate(rng, src)
			out = append(out, Entry{
				Name:     "generated/" + digits(i) + "/" + how + "/" + digits(j),
				Src:      broken,
				Mutation: how,
			})
		}
	}

	return out
}

// digits renders a number without pulling in a formatter, so that a name is
// built the same way every time regardless of locale or format verb.
func digits(n int) string {
	if n == 0 {
		return "0"
	}

	var out []byte

	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}

	return string(out)
}
