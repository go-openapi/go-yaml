// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package key_test

import (
	"fmt"
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser/key"
	"github.com/go-openapi/go-yaml/token"
)

// padTo is how many keys one mapping holds before [key.Set] leaves the scan for
// its hash index. The threshold itself is unexported, so these cases reach it by
// recording keys, the way a document does.
const padTo = 64

// at returns a position on line, so a test can tell one recording of a key from
// another by where it stands.
func at(line int32) token.Position {
	return token.Position{Line: line, Column: 1}
}

// answer names the three values Record hands back.
type answer struct {
	first    token.Position
	jsonOnly bool
	repeat   bool
}

// record records text as a key of the mapping starting at base, written on line.
func record(s *key.Set, base int, text string, kind token.KeyKind, line int32) answer {
	first, jsonOnly, repeat := s.Record(base, text, kind, at(line))

	return answer{first: first, jsonOnly: jsonOnly, repeat: repeat}
}

// pad fills the mapping at base up to padTo keys, so that the next key recorded
// there goes through the index and not the scan. Keys the caller recorded first
// count towards the fill and are carried over by the spill.
func pad(t *testing.T, s *key.Set, base int) {
	t.Helper()

	for i := range padTo - (s.Base() - base) {
		got := record(s, base, fmt.Sprintf("pad%d", i), token.KeyString, int32(1000+i))
		require.Falsef(t, got.repeat, "pad%d is a key of its own", i)
	}
	require.Equal(t, base+padTo, s.Base())
}

// keyPair asks whether two keys written into one mapping are one key or two.
type keyPair struct {
	name string
	// aText and aKind are the key written first, bText and bKind the key
	// written after it.
	aText string
	aKind token.KeyKind
	bText string
	bKind token.KeyKind
	// jsonNames turns [key.Set.UseJSONNames] on for the case.
	jsonNames bool
	// repeat is whether the mapping had already used the second key, and
	// jsonOnly whether the two are one JSON member name and two YAML nodes.
	repeat   bool
	jsonOnly bool
}

func keyPairs() iter.Seq[keyPair] {
	return slices.Values([]keyPair{
		{
			name:  "one string written twice",
			aText: "a", aKind: token.KeyString,
			bText: "a", bKind: token.KeyString,
			repeat: true,
		},
		{
			name:  "two strings",
			aText: "a", aKind: token.KeyString,
			bText: "b", bKind: token.KeyString,
		},
		{
			// 3.2.1.1 makes a key's type half its identity, so the integer 1 and
			// the string "1" are two keys under the core schema.
			name:  "an integer and a string spelled alike",
			aText: "1", aKind: token.KeyInt,
			bText: "1", bKind: token.KeyString,
		},
		{
			name:  "an integer and a float",
			aText: "1", aKind: token.KeyInt,
			bText: "1.0", bKind: token.KeyFloat,
		},
		{
			// The filter packs the kind, the length and the first and last byte
			// into one word, so "abc" and "axc" stand for the same word. A
			// filter match settles nothing on its own: the text is compared
			// after it, and these are two keys.
			name:  "two keys the filter cannot tell apart",
			aText: "abc", aKind: token.KeyString,
			bText: "axc", bKind: token.KeyString,
		},
		{
			name:  "an integer and a string under JSON names",
			aText: "1", aKind: token.KeyInt,
			bText: "1", bKind: token.KeyString,
			jsonNames: true,
			repeat:    true,
			jsonOnly:  true,
		},
		{
			name:  "one string written twice under JSON names",
			aText: "a", aKind: token.KeyString,
			bText: "a", bKind: token.KeyString,
			jsonNames: true,
			repeat:    true,
		},
		{
			name:  "two strings under JSON names",
			aText: "a", aKind: token.KeyString,
			bText: "b", bKind: token.KeyString,
			jsonNames: true,
		},
	})
}

// TestSetReadsAPairOfKeysTheSameWayOnBothPaths records each pair twice: once
// under the scan, and once in a mapping padded past the spill so that the index
// answers.
//
// The index inherits the kind from the same [token.KeyKind] the scan filters on,
// so both paths read a pair the same way. An index that compared the written
// characters would pass every case below on the scan alone.
func TestSetReadsAPairOfKeysTheSameWayOnBothPaths(t *testing.T) {
	t.Parallel()

	for c := range keyPairs() {
		t.Run(c.name+", scanned", func(t *testing.T) {
			t.Parallel()

			var s key.Set
			s.UseJSONNames(c.jsonNames)

			require.False(t, record(&s, 0, c.aText, c.aKind, 1).repeat)

			got := record(&s, 0, c.bText, c.bKind, 3)
			assert.Equal(t, c.repeat, got.repeat)
			assert.Equal(t, c.jsonOnly, got.jsonOnly)
			if c.repeat {
				assert.Equal(t, at(1), got.first)
			} else {
				assert.Zero(t, got.first)
			}
		})

		t.Run(c.name+", indexed", func(t *testing.T) {
			t.Parallel()

			var s key.Set
			s.UseJSONNames(c.jsonNames)
			pad(t, &s, 0)

			require.False(t, record(&s, 0, c.aText, c.aKind, 1).repeat)

			got := record(&s, 0, c.bText, c.bKind, 3)
			assert.Equal(t, c.repeat, got.repeat)
			assert.Equal(t, c.jsonOnly, got.jsonOnly)
			if c.repeat {
				assert.Equal(t, at(1), got.first)
			} else {
				assert.Zero(t, got.first)
			}
		})
	}
}

// TestSetZeroValueRecordsFromEmpty checks that a set needs no construction: the
// parser holds one by value and reuses it for the whole document.
func TestSetZeroValueRecordsFromEmpty(t *testing.T) {
	t.Parallel()

	var s key.Set
	assert.Zero(t, s.Base())

	assert.False(t, record(&s, 0, "a", token.KeyString, 1).repeat)
	assert.Equal(t, 1, s.Base())
}

// TestSetKeepsOneMappingsKeysToItself checks the base: a mapping's keys are the
// tail from where it opened, so a key an outer mapping used is no repeat for an
// inner one.
func TestSetKeepsOneMappingsKeysToItself(t *testing.T) {
	t.Parallel()

	var s key.Set
	outer := s.Base()
	require.False(t, record(&s, outer, "a", token.KeyString, 1).repeat)

	inner := s.Base()
	assert.False(t, record(&s, inner, "a", token.KeyString, 2).repeat,
		"the inner mapping has not used \"a\"")

	s.Close(inner)

	got := record(&s, outer, "a", token.KeyString, 5)
	assert.True(t, got.repeat, "the outer mapping used \"a\" on line 1")
	assert.Equal(t, at(1), got.first)
}

// TestSetCloseDropsTheMappingsKeys checks that a mapping's keys go with it, so
// the next mapping opening at the same base starts empty.
func TestSetCloseDropsTheMappingsKeys(t *testing.T) {
	t.Parallel()

	var s key.Set
	require.False(t, record(&s, 0, "a", token.KeyString, 1).repeat)
	require.False(t, record(&s, 0, "b", token.KeyString, 2).repeat)

	s.Close(0)
	assert.Zero(t, s.Base())

	assert.False(t, record(&s, 0, "a", token.KeyString, 9).repeat,
		"the mapping that used \"a\" has closed")
}

// TestSetCloseClearsTheIndexPastTheSpill checks the half of Close a truncation
// cannot do.
//
// A mapping that never spilled is dropped by cutting the slices back, and its
// keys go with them. A spilled mapping has entries in the index too, keyed on
// its base -- and the next mapping to open at that base carries the same base,
// so an index left uncleared reports the closed mapping's keys as its own.
func TestSetCloseClearsTheIndexPastTheSpill(t *testing.T) {
	t.Parallel()

	var s key.Set
	pad(t, &s, 0)
	require.False(t, record(&s, 0, "a", token.KeyString, 1).repeat)

	s.Close(0)
	require.Zero(t, s.Base())

	pad(t, &s, 0)
	got := record(&s, 0, "a", token.KeyString, 500)
	assert.False(t, got.repeat, "the mapping that used \"a\" has closed")
	assert.Zero(t, got.first)
}

// TestSetKeepsOneMappingsKeysToItselfPastTheSpill checks the base through the
// index, where it is a field of the map's key and not a slice bound.
func TestSetKeepsOneMappingsKeysToItselfPastTheSpill(t *testing.T) {
	t.Parallel()

	var s key.Set
	outer := s.Base()
	pad(t, &s, outer)
	require.False(t, record(&s, outer, "a", token.KeyString, 1).repeat)

	inner := s.Base()
	pad(t, &s, inner)
	assert.False(t, record(&s, inner, "a", token.KeyString, 2).repeat,
		"the inner mapping has not used \"a\"")

	s.Close(inner)

	got := record(&s, outer, "a", token.KeyString, 5)
	assert.True(t, got.repeat)
	assert.Equal(t, at(1), got.first)
}

// TestSetCarriesTheKeysItHeldIntoTheIndex checks the one-time spill: a key
// recorded under the scan is still found once the mapping has grown past the
// threshold and the index answers instead.
func TestSetCarriesTheKeysItHeldIntoTheIndex(t *testing.T) {
	t.Parallel()

	var s key.Set
	require.False(t, record(&s, 0, "first", token.KeyString, 1).repeat)
	pad(t, &s, 0)

	got := record(&s, 0, "first", token.KeyString, 900)
	assert.True(t, got.repeat, "\"first\" was recorded before the spill")
	assert.Equal(t, at(1), got.first)
}

// TestSetUnderJSONNamesCarriesBothIndexEntriesOverTheSpill checks that the spill
// copies the name-only entry as well as the one keyed on the kind, so a pair
// written on either side of the threshold still meets.
func TestSetUnderJSONNamesCarriesBothIndexEntriesOverTheSpill(t *testing.T) {
	t.Parallel()

	var s key.Set
	s.UseJSONNames(true)
	require.False(t, record(&s, 0, "1", token.KeyInt, 1).repeat)
	pad(t, &s, 0)

	got := record(&s, 0, "1", token.KeyString, 900)
	assert.True(t, got.repeat)
	assert.True(t, got.jsonOnly, "the integer 1 and the string \"1\" write one JSON member")
	assert.Equal(t, at(1), got.first)
}

// TestSetCloseClearsBothIndexEntriesUnderJSONNames isolates the second delete.
//
// Past the spill a set under JSON names holds two index entries per key: one on
// the key's kind and one on its name alone. Recording "1" as a string after the
// mapping that recorded it as an integer has closed reaches the name entry and
// not the kind entry, so a Close that clears only the first reports a repeat
// against a mapping that is gone.
func TestSetCloseClearsBothIndexEntriesUnderJSONNames(t *testing.T) {
	t.Parallel()

	var s key.Set
	s.UseJSONNames(true)
	pad(t, &s, 0)
	require.False(t, record(&s, 0, "1", token.KeyInt, 1).repeat)

	s.Close(0)
	require.Zero(t, s.Base())

	pad(t, &s, 0)
	got := record(&s, 0, "1", token.KeyString, 500)
	assert.False(t, got.repeat, "the mapping that used the integer 1 has closed")
	assert.False(t, got.jsonOnly)
}
