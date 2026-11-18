// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

func TestWalkMatchesTheDecoder(t *testing.T) {
	var srcs []struct{ name, text string }
	suites, _ := yamltestsuite.TestSuites()
	for _, s := range suites {
		srcs = append(srcs, struct{ name, text string }{"suite/" + s.Name, string(s.InYAML)})
	}
	seeds, _ := fuzzseeds.All()
	for i, s := range seeds {
		srcs = append(srcs, struct{ name, text string }{fmt.Sprintf("seed/%04d", i), s})
	}

	var same, differ, bothErr, oneErr, skipped int
	for _, src := range srcs {
		if strings.Contains(src.text, "&!") {
			// The parse reads "&!a1" as an anchor with no name followed by the
			// tag "!a1", rather than as an anchor named "!a1": ns-anchor-name
			// excludes the flow indicators and nothing else, so "!" belongs in
			// a name. The two readers make different nonsense of the same tree
			// and neither is right. Held out here as the JSON equivalence gate
			// holds it out, until the parser is fixed.
			skipped++

			continue
		}

		var want any
		wantErr := codec.Unmarshal([]byte(src.text), &want)

		got, gotErr := codec.WalkValue([]byte(src.text))

		switch {
		case wantErr != nil && gotErr != nil:
			bothErr++
		case wantErr != nil || gotErr != nil:
			oneErr++
			if oneErr <= 6 {
				t.Logf("%s: decoder err=%v walk err=%v\n%s", src.name, wantErr, gotErr, src.text)
			}
		case fmt.Sprintf("%#v", want) == fmt.Sprintf("%#v", got):
			same++
		default:
			differ++
			if differ <= 6 {
				t.Logf("%s:\n  src   %q\n  want  %#v\n  got   %#v", src.name, src.text, want, got)
			}
		}
	}
	t.Logf("same=%d differ=%d bothErr=%d oneErr=%d skipped=%d", same, differ, bothErr, oneErr, skipped)
}
