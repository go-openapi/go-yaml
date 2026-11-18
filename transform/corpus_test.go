// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/transform"
)

// TestIdentityRebuildsTheCorpus runs the identity transform over every document
// the test suite and the fuzz seeds hold, and asks for the source back.
//
// It is the one property the package stands on: the pieces tile the document,
// so a transform that changes nothing changes nothing. A document the parse
// refuses is not scored -- there is no walk to tile it -- and the count of
// those is printed so that a change refusing far more says so.
func TestIdentityRebuildsTheCorpus(t *testing.T) {
	t.Parallel()

	var rebuilt, refused int
	for _, src := range corpusSources(t) {
		var out bytes.Buffer
		if err := transform.Walk(&out, []byte(src.text), transform.Func(transform.Copy)); err != nil {
			refused++

			continue
		}
		require.Equalf(t, src.text, out.String(), "%s did not come back as it went in", src.name)
		rebuilt++
	}

	t.Logf("rebuilt %d documents, %d refused by the parse", rebuilt, refused)
	require.Positive(t, rebuilt)
}

type corpusSource struct {
	name string
	text string
}

func corpusSources(t *testing.T) []corpusSource {
	t.Helper()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	srcs := make([]corpusSource, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, corpusSource{"suite/" + s.Name, string(s.InYAML)})
	}
	for i, s := range seeds {
		srcs = append(srcs, corpusSource{fmt.Sprintf("seed/%04d", i), s})
	}

	return srcs
}
