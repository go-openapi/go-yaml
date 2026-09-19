// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"slices"
	"strings"

	"github.com/go-openapi/conformance-suites/stance"
)

// SpecsFor builds an artifact's vocabulary from a language's tags and rules,
// keeping only the tags that appear in used.
//
// The header states what the artifact uses rather than everything the language
// defines, so that [Header.Unknown] stays a real check: a consumer is asked to
// understand the questions this corpus actually raises, not every question the
// language could raise.
//
// This is the one place the on-disk form meets the stance types. The wire
// format stays plain strings, so nothing about consuming an artifact depends on
// having these packages -- which is the whole reason the stages are written
// into the file instead of being looked up in our source.
func SpecsFor(used []string, vocabulary stance.Vocabulary, rules stance.Rules) []TagSpec {
	out := make([]TagSpec, 0, len(used))

	for _, name := range used {
		tag := stance.Tag(name)

		spec := TagSpec{Tag: name, Stage: stance.Parse.String()}

		if at, ok := vocabulary.Of(tag); ok {
			spec.Stage = at.String()
		}

		if rule, ok := rules.Of(tag); ok {
			spec.Settled = rule.Then.String()
			spec.Because = rule.Because
		}

		out = append(out, spec)
	}

	slices.SortFunc(out, func(a, b TagSpec) int { return strings.Compare(a.Tag, b.Tag) })

	return out
}

// StageOf reads a stage name back, defaulting to [stance.Parse].
//
// The default is the point rather than a convenience. A case that says nothing
// about how far its verdict reaches is claiming the least, so a name this does
// not recognize -- an older artifact, a newer stage, a typo -- costs a scored
// case and never produces a wrong expectation.
func StageOf(name string) stance.Stage {
	switch name {
	case stance.Compose.String():
		return stance.Compose
	case stance.Construct.String():
		return stance.Construct
	default:
		return stance.Parse
	}
}
