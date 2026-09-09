// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"sort"
	"testing"

	"github.com/go-openapi/go-yaml/internal/testintegration/goyaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	"github.com/go-openapi/go-yaml/internal/testintegration/libfyaml"
)

// Where the outside sources decline, and what the corpus may claim there.
//
// Three of the four sources answer a narrow question each. perlref and
// grammar.NewRecognizer answer syntax and nothing else. libfyaml 1.0.0b1 and
// go.yaml.in/yaml/v3 answer what a document denotes -- when they can: libfyaml
// dies on a collection key, since Python cannot hash a list, and yaml/v3
// refuses one for the same reason Go cannot.
//
// So there are documents the grammar accepts and no implementation will read,
// and on those the specification is the only judge. A meaning stated there
// rests on somebody's reading of the prose and nothing else, which is worth
// knowing before trusting one.
//
// # Silence and contradiction are not the same darkness
//
// This test counts both as "no value", which is right for the meaning question
// and hides a distinction the reader needs. yamlgen.Strict's two entries are
// the clean example, measured 2026-09-08:
//
//   - `{{"": 0}}`: libfyaml and yaml/v3 both decline *after* parsing, because
//     neither can hold a collection as a key. Nobody contradicts the grammar;
//     nobody can be asked. Read the specification.
//   - a flow mapping key spanning two lines: both refuse it *in the parser*, at
//     a position, against grammar.NewRecognizer and the reference parser, which
//     accept it. That is not silence -- it is two implementations reading 7.4.2
//     the other way, and the evidence is about 7.4.2 rather than about any
//     library.
//
// The first kind wants the prose read. The second wants the grammar
// questioned. Both leave a meaning uncorroborated, and only the first leaves it
// unopposed.

// uncorroborated names the shapes whose stated meaning no outside
// implementation can confirm, with the production that settles it.
//
// Two, and both are questions about what a key is. Adding a third means
// claiming a meaning on a reading alone, and the test below makes that a
// deliberate act rather than a quiet one.
var uncorroborated = map[string]string{
	"an explicit key whose own key is explicit": "8.2.2 puts an explicit entry's key at " +
		"s-l+block-indented(n, block-out), which is any block node -- a mapping written the long way " +
		"included, and a '?' of its own with it. The key it builds is a mapping, which libfyaml " +
		"cannot hash and yaml/v3 refuses, so neither can say what the document holds",
	"two collection keys in one mapping": "3.2.1.1 makes two keys equal when they resolve to the same " +
		"node, and two different mappings do not. libfyaml cannot hash either of them and yaml/v3 " +
		"refuses both, so neither can say the document holds two entries",
}

// TestEveryUncorroboratedMeaningIsNamed holds the corpus to naming the places
// where careful reading is the only instrument.
//
// The failure it exists to stop is quiet: a meaning added to a shape no
// implementation will read looks exactly like a meaning three of them agree on,
// and the corpus then asserts one person's reading of the specification with
// the authority of a measurement.
func TestEveryUncorroboratedMeaningIsNamed(t *testing.T) {
	if !libfyaml.Available() {
		t.Skipf("libfyaml is not installed at %s", libfyaml.Home())
	}

	rec := grammar.NewRecognizer(1024)

	var silent []string

	claimed := map[string]bool{}

	for family, shapes := range statedShapes() {
		for _, s := range shapes {
			if _, fromV3 := goyaml.Load(s.Src); fromV3 == nil {
				continue
			}

			if _, fromLibfy := libfyaml.Load(s.Src); fromLibfy == nil {
				continue
			}

			if !rec.Stream(s.Src).OK {
				// A document the grammar refuses is one every source answers,
				// by refusing it.
				continue
			}

			if s.Means == nil {
				silent = append(silent, family+": "+s.Name)

				continue
			}

			claimed[s.Name] = true

			if _, named := uncorroborated[s.Name]; !named {
				t.Errorf("%s states a meaning no implementation can confirm and is not named in "+
					"uncorroborated: %q\n  document: %q", s.Name, s.Name, string(s.Src))
			}
		}
	}

	for name := range uncorroborated {
		if !claimed[name] {
			t.Errorf("%q is named as uncorroborated and either states no meaning now or an "+
				"implementation reads it, so the entry can go", name)
		}
	}

	sort.Strings(silent)

	t.Logf("%d shapes the grammar accepts and no implementation reads; %d of them state a meaning",
		len(silent)+len(claimed), len(claimed))

	for _, name := range silent {
		t.Logf("  no meaning stated: %s", name)
	}
}
