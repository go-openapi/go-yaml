// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"testing"
)

type aliasPair struct {
	N int `yaml:"n"`
}

const aliasSrc = "base: &b\n  n: 1\nfirst: *b\nsecond: *b\n"

// TestAliasesAreIndependent covers every destination that used to hand two
// aliases one value. Whether it did turned on whether the destination declared
// a field for the anchor itself: with one, the decoded value was recorded and
// every alias got it; without one, each alias read the node afresh.
func TestAliasesAreIndependent(t *testing.T) {
	t.Run("into an any, walking", func(t *testing.T) {
		var v any
		if err := Unmarshal([]byte(aliasSrc), &v); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through second")
		}
		if got := m["base"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through the anchor")
		}
	})

	t.Run("into an any, tree", func(t *testing.T) {
		var v any
		if err := treeDecoder(aliasSrc).Decode(&v); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got == 999 {
			t.Error("writing through first wrote through second")
		}
	})

	t.Run("into pointers, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		if err := Unmarshal([]byte(aliasSrc), &dst); err != nil {
			t.Fatal(err)
		}
		if dst.First == dst.Second || dst.First == dst.Base {
			t.Fatal("two aliases stand at one address")
		}
		dst.First.N = 999
		if dst.Second.N == 999 || dst.Base.N == 999 {
			t.Error("writing through first wrote through the others")
		}
	})

	t.Run("into maps, the anchor decoded too", func(t *testing.T) {
		var dst struct {
			Base   map[string]int `yaml:"base"`
			First  map[string]int `yaml:"first"`
			Second map[string]int `yaml:"second"`
		}
		if err := Unmarshal([]byte(aliasSrc), &dst); err != nil {
			t.Fatal(err)
		}
		dst.First["n"] = 999
		if dst.Second["n"] == 999 || dst.Base["n"] == 999 {
			t.Error("writing through first wrote through the others")
		}
	})
}

// TestShareAliasesGivesOneValue is the other half: the option puts the two
// names back at one address, which is what MarshalAnchor and WithSmartAnchor
// read to write an alias back out.
func TestShareAliasesGivesOneValue(t *testing.T) {
	t.Run("into an any, walking", func(t *testing.T) {
		var v any
		if err := UnmarshalWithOptions([]byte(aliasSrc), &v, ShareAliases()); err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		m["first"].(map[string]any)["n"] = 999
		if got := m["second"].(map[string]any)["n"]; got != 999 {
			t.Errorf("second reads %v, want the value first was given", got)
		}
	})

	t.Run("into pointers", func(t *testing.T) {
		var dst struct {
			Base   *aliasPair `yaml:"base"`
			First  *aliasPair `yaml:"first"`
			Second *aliasPair `yaml:"second"`
		}
		dec := NewDecoder(bytes.NewReader([]byte(aliasSrc)), ShareAliases())
		if err := dec.Decode(&dst); err != nil {
			t.Fatal(err)
		}
		if dst.First != dst.Second || dst.First != dst.Base {
			t.Error("the three names do not stand at one address")
		}
	})
}

// TestSharingSurvivesTheBudget checks that the option also puts back the
// immunity: an alias that builds nothing extra cannot run past the budget, so a
// document naming 10^9 values reads in no time.
func TestSharingSurvivesTheBudget(t *testing.T) {
	src := aliasBomb(9, 9)

	var v any
	if err := UnmarshalWithOptions([]byte(src), &v, ShareAliases()); err != nil {
		t.Fatalf("%d bytes: %v", len(src), err)
	}
}
