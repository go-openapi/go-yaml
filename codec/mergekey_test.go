// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"testing"
)

// underEleven is src read under YAML 1.1, where "<<" is the merge key.
//
// The merge key is tag:yaml.org,2002:merge, a 1.1 type, so the version decides
// whether a bare "<<" merges at all. These tests are about what a merge means,
// so they ask 1.1; what the same documents mean as written is asserted in
// codec_test's TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema.
//
// The package has its own copy because codec_test is a different package.
func underEleven(src string) string { return "%YAML 1.1\n---\n" + src }

type mergePair struct {
	A int `yaml:"a"`
	B int `yaml:"b"`
	C int `yaml:"c"`
}

// TestMergedKeyLosesToTheMappingsOwn covers the point of "<<": the mapping
// writes what the merged one already writes, and its own value stands.
//
// Both were validated against one another until now, so overriding a merged key
// was refused as a duplicate -- with the position of the anchor's key when the
// "<<" came last. The document writes each key once; the two stand in different
// mappings.
func TestMergedKeyLosesToTheMappingsOwn(t *testing.T) {
	for name, src := range map[string]string{
		"merge first": underEleven("base: &b\n  a: 1\n  b: 2\nuse:\n  <<: *b\n  b: 3\n"),
		"merge last":  underEleven("base: &b\n  a: 1\n  b: 2\nuse:\n  b: 3\n  <<: *b\n"),
	} {
		t.Run(name, func(t *testing.T) {
			var dst struct {
				Base mergePair `yaml:"base"`
				Use  mergePair `yaml:"use"`
			}
			if err := Unmarshal([]byte(src), &dst); err != nil {
				t.Fatal(err)
			}
			if dst.Use.A != 1 {
				t.Errorf("a: got %d, want 1 from the merge", dst.Use.A)
			}
			if dst.Use.B != 3 {
				t.Errorf("b: got %d, want 3, the mapping's own", dst.Use.B)
			}

			// The two paths read one document, so they answer the same.
			var walked any
			if err := Unmarshal([]byte(src), &walked); err != nil {
				t.Fatal(err)
			}
			use, _ := walked.(map[string]any)["use"].(map[string]any)
			if use["a"] != uint64(1) && use["a"] != int64(1) && use["a"] != 1 {
				t.Errorf("walk read a as %v (%T)", use["a"], use["a"])
			}
			if use["b"] != uint64(3) && use["b"] != int64(3) && use["b"] != 3 {
				t.Errorf("walk read b as %v (%T)", use["b"], use["b"])
			}
		})
	}
}

// TestEarlierMergeWinsOverLater covers a mapping merging two others that write
// the same key: the first named stands, as YAML 1.1's merge says.
func TestEarlierMergeWinsOverLater(t *testing.T) {
	src := underEleven("one: &one\n  a: 1\n  b: 1\ntwo: &two\n  b: 2\n  c: 2\nuse:\n  <<: [*one, *two]\n")

	var dst struct {
		Use mergePair `yaml:"use"`
	}
	if err := Unmarshal([]byte(src), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Use.A != 1 || dst.Use.C != 2 {
		t.Errorf("the merge did not take both: %+v", dst.Use)
	}
	if dst.Use.B != 1 {
		t.Errorf("b: got %d, want 1 from the mapping named first", dst.Use.B)
	}
}

// TestDuplicateKeyInOneMappingIsStillRefused holds the line the fix moves: two
// keys of one mapping still collide.
func TestDuplicateKeyInOneMappingIsStillRefused(t *testing.T) {
	var dst mergePair
	if err := Unmarshal([]byte("a: 1\na: 2\n"), &dst); err == nil {
		t.Error("the repeated key was accepted")
	}
}
