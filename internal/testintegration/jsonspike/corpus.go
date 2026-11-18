// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsonspike

import (
	"fmt"
	"math/rand/v2"

	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
)

// Entry is one corpus document with everything a replay needs and nothing a
// replay should be told.
//
// It carries the grammar's verdict and the implementation-defined tags. It
// deliberately does not carry an expected outcome: that is a [stance.Table]'s
// job at replay time, so one corpus can score parsers holding different
// positions without any of them being written into it.
type Entry struct {
	// Name says where the document came from, in enough detail to reproduce it.
	Name string
	// Doc is the bytes, the grammar's verdict and the tags.
	Doc stance.Doc
	// Mutation names how it was broken, empty for a document generated whole.
	Mutation string
}

// Generate builds a corpus from a seed.
//
// It is deterministic: the same seed yields the same documents, byte for byte.
// A corpus that is checked in has to be reproducible or nobody can tell a
// regenerated one from a tampered one.
//
// Each generated document is emitted from a [Value] in a [Style], so it is
// valid by construction; then each is broken repeatedly. The mutants that stay
// valid are dropped -- accepting a valid document is not news -- and what
// remains is the set worth asking a parser about.
func Generate(seed uint64, documents, mutantsEach int) []Entry {
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)) //nolint:gosec // a seeded generator, so a corpus build is reproducible; nothing here is secret
	rec := NewRecognizer(1024)

	var (
		out   []Entry
		valid int
	)

	for d := range documents {
		src := Emit(RandomValue(rng, 0), RandomStyle(rng))

		if !rec.Text(src).OK {
			// The emitter is meant to write only valid JSON, so this is a
			// defect in it rather than an interesting document. The test that
			// gates the emitter reports it; here it is simply not corpus.
			continue
		}

		valid++
		out = append(out, Entry{
			Name: fmt.Sprintf("generated/%04d", d),
			Doc:  Describe(fmt.Sprintf("generated/%04d", d), src),
		})

		for m := range mutantsEach {
			mutant, how := Mutate(rng, src)
			if len(mutant) == 0 || rec.Text(mutant).OK {
				continue // still JSON, so it says nothing about a refusal
			}

			name := fmt.Sprintf("generated/%04d/mutant/%02d", d, m)
			out = append(out, Entry{
				Name:     name,
				Doc:      Describe(name, mutant),
				Mutation: how,
			})
		}
	}

	return out
}

// Broken counts the entries the grammar refuses, which are the ones a laxity
// hunt is about.
func Broken(entries []Entry) int {
	var n int

	for _, e := range entries {
		if !e.Doc.WellFormed {
			n++
		}
	}

	return n
}
