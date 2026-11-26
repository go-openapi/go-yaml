// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package parser_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser"
)

// keyLedger records how often each mapping-key invariant of the parser fails over the fuzz corpus,
// as the scanner's state ledger does for the scanner.
//
// TestKeyLedger checks each count with [ledgers.Compare], in both directions.
// An entry that starts failing has lost an invariant.
// An entry that stops failing may guard a check that can go, and its count comes down.
var keyLedger = map[string]int64{ //nolint:gochecknoglobals // ok to store an immutable map as a global
	// Mappings nest, so a mapping records keys only while it is the innermost one open.
	// Then the key set's entries above base are one mapping's keys,
	// and a duplicate is found by scanning the tail instead of hashing base into a document-wide map.
	//
	// This count stays 0: a failure means an outer mapping recorded a key over an inner one's,
	// and key.Ledger.Close would drop a key it still owns.
	"mapkey.stackTailIsOneMapping": 0,
}

// TestKeyLedger checks the parser's mapping-key invariants against the counts keyLedger records.
//
//	go test -tags yamlprobe -run TestKeyLedger ./internal/ledgers/parser/
func TestKeyLedger(t *testing.T) {
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	require.NotEmpty(t, seeds)

	probe.Reset()
	for _, src := range seeds {
		_, _ = parser.ParseBytes([]byte(src))
		_, _ = parser.ParseBytes([]byte(src), parser.WithComments())
	}

	checks := probe.Checks()
	require.NotEmpty(t, checks, "the probe recorded nothing; is the yamlprobe tag on?")

	// probe.Checks holds one ledger for the whole library,
	// and a parse also runs the scanner, whose checks internal/ledgers/scanner holds to its own ledger.
	// This test reads only the parser's "mapkey." names: the ones that disagreed, and the ones the ledger records.
	measured := make(map[string]int64)
	for name, inv := range checks {
		if _, recorded := keyLedger[name]; strings.HasPrefix(name, "mapkey.") && (inv.Failed > 0 || recorded) {
			measured[name] = inv.Failed
			t.Logf("%s: %d of %d %v", name, inv.Failed, inv.Tested, inv.Samples)
		}
	}

	ledgers.Compare(t, "disagreements", measured, keyLedger)

	for _, name := range slices.Sorted(maps.Keys(keyLedger)) {
		assert.Positivef(t, checks[name].Tested, "%s is in the ledger and nothing checked it", name)
	}
}
