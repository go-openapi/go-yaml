// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/go-yaml"
)

// TestPresentationInvariance is the property this package exists for: the same
// value, written down several different ways, has to read back the same every
// time.
//
// It needs no grammar, no oracle and no reference implementation. The invariant
// is internal, so every failure is unambiguous -- either two presentations of
// one value disagree, or one of them does not read back at all.
func TestPresentationInvariance(t *testing.T) {
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		styles := yamlgen.DistinctStyles(rt, 4)

		for _, style := range styles {
			// Write rather than Emit, because one style changes what the
			// document means: Style.Version writes a "%YAML 1.1" directive,
			// and a document that says which schema reads it means what that
			// schema makes of it. Written.Means is that answer, and
			// Value.Decoded() for every style that declares nothing.
			w := yamlgen.Write(value, style)
			if w.MeansUnclear {
				// The generator will not say what this document means, so
				// there is nothing to hold the library to. See Written.
				continue
			}

			src, expected := w.Text, w.Means
			known := yamlgen.Known(ledger, yamlgen.Decode, value, style)

			var got any
			err := yaml.Unmarshal([]byte(src), &got)
			diverged := err != nil || !sameValue(expected, got)

			if known != nil {
				tally.record(known.Name, diverged)

				continue
			}

			if err != nil {
				rt.Fatalf("style %s did not read back:\n%s\n---\nerror: %v", style, src, err)
			}
			if diverged {
				rt.Fatalf("style %s read back differently:\n%s\n---\nexpected: %#v\ngot:      %#v",
					style, src, expected, got)
			}
		}
	})

	tally.report(t, yamlgen.Decode)
}
