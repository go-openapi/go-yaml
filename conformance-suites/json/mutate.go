// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import (
	"encoding/json"
	"math/rand/v2"
	"slices"
	"strconv"
	"unicode"
)

// Mutate breaks a document, and claims nothing about the result.
//
// Most ways of disturbing a document leave another good one, so the recognizer
// is what makes this affordable: one call sorts the mutants worth asking a
// parser about from the ones that are still perfectly valid JSON.
//
// # Every mutation here is generic on purpose
//
// Not one of them mentions a colon, a closing brace, or any other particular
// character of the language. They delete a byte, duplicate a byte, swap two,
// truncate, or substitute from an alphabet **read out of the grammar file**
// rather than written down here.
//
// That constraint is the point, and it is meant to be checked by reading the
// code. This spike exists to find out whether generative testing would have
// caught a bug that a mature hand-written suite missed, and the author already
// knows what that bug is. A mutation aimed at the answer would prove nothing.
// A one-byte deletion that happens to find it proves the method.
func Mutate(rng *rand.Rand, src []byte) ([]byte, string) {
	if len(src) == 0 {
		return src, "nothing to break"
	}

	m := mutations[rng.IntN(len(mutations))]

	return m.apply(rng, slices.Clone(src)), m.name
}

// MutationNames lists the mutations, so a hunt can report which of them ever
// produced anything.
func MutationNames() []string {
	out := make([]string, 0, len(mutations))
	for _, m := range mutations {
		out = append(out, m.name)
	}

	return out
}

type mutation struct {
	name  string
	apply func(*rand.Rand, []byte) []byte
}

var mutations = []mutation{
	{
		name: "delete a byte",
		apply: func(rng *rand.Rand, src []byte) []byte {
			at := rng.IntN(len(src))

			return slices.Delete(src, at, at+1)
		},
	},
	{
		name: "delete a run",
		apply: func(rng *rand.Rand, src []byte) []byte {
			at := rng.IntN(len(src))
			end := min(at+1+rng.IntN(4), len(src))

			return slices.Delete(src, at, end)
		},
	},
	{
		name: "duplicate a byte",
		apply: func(rng *rand.Rand, src []byte) []byte {
			at := rng.IntN(len(src))

			return slices.Insert(src, at, src[at])
		},
	},
	{
		name: "swap two neighbors",
		apply: func(rng *rand.Rand, src []byte) []byte {
			if len(src) < 2 {
				return src
			}

			at := rng.IntN(len(src) - 1)
			src[at], src[at+1] = src[at+1], src[at]

			return src
		},
	},
	{
		name: "truncate",
		apply: func(rng *rand.Rand, src []byte) []byte {
			return src[:rng.IntN(len(src))]
		},
	},
	{
		name: "substitute a byte the grammar knows",
		apply: func(rng *rand.Rand, src []byte) []byte {
			src[rng.IntN(len(src))] = grammarBytes[rng.IntN(len(grammarBytes))]

			return src
		},
	},
	{
		name: "insert a byte the grammar knows",
		apply: func(rng *rand.Rand, src []byte) []byte {
			return slices.Insert(src, rng.IntN(len(src)), grammarBytes[rng.IntN(len(grammarBytes))])
		},
	},
	{
		name: "substitute an arbitrary byte",
		apply: func(rng *rand.Rand, src []byte) []byte {
			src[rng.IntN(len(src))] = byte(rng.IntN(256))

			return src
		},
	},
}

// grammarBytes is the alphabet the mutations substitute from, taken from the
// grammar rather than chosen here.
//
// Reading it out of the specification is what keeps the mutation catalog honest:
// an alphabet written by hand would be a list of characters somebody thought
// were interesting, and in a spike whose answer is already known that is not a
// list anyone should trust.
var grammarBytes = literalBytesOfTheGrammar()

func literalBytesOfTheGrammar() []byte {
	seen := make(map[byte]bool)

	var walk func(any)

	walk = func(body any) {
		switch form := body.(type) {
		case string:
			// Single characters written literally, and the code points the
			// grammar spells as xNN.
			if len(form) == 1 {
				seen[form[0]] = true

				return
			}

			if r, ok := hexPoint(form); ok && r < 0x80 {
				seen[byte(r)] = true
			}
		case []any:
			for _, item := range form {
				walk(item)
			}
		case map[string]any:
			for _, arg := range form {
				walk(arg)
			}
		}
	}

	walk(rawGrammar())

	out := make([]byte, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}

	slices.Sort(out)

	return out
}

// rawGrammar decodes the embedded specification, for the one caller that wants
// the text of it rather than the compiled form.
func rawGrammar() map[string]any {
	var raw map[string]any
	if err := json.Unmarshal(spec, &raw); err != nil {
		panic("jsoncorpus: the embedded grammar does not decode: " + err.Error())
	}

	return raw
}

// hexPoint decodes the grammar's xNN spelling of a code point.
func hexPoint(s string) (rune, bool) {
	if len(s) < 3 || s[0] != 'x' || len(s)%2 == 0 {
		return 0, false
	}

	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil || n > unicode.MaxRune {
		return 0, false
	}

	return rune(n), true
}
