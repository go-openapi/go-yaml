// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"reflect"
	"testing"
)

// TestATagKeepsTheWalk reads documents holding a tagged node, with and without
// ShareAliases, and requires the value the tree gives without the decode
// building a tree for the document. The walk hands each tagged node to the
// tree decoder alone.
func TestATagKeepsTheWalk(t *testing.T) {
	docs := map[string]string{
		"a string on a number":          "name: !!str 1\n",
		"an int on a string":            "leaf: {i: !!int \"5\"}\n",
		"a float on an int":             "leaf: {f: !!float 1}\n",
		"a bool":                        "leaf: {b: !!bool true}\n",
		"a local tag on a scalar":       "name: !Ref x\n",
		"a local tag on a mapping":      "leaf: !thing {s: x, i: 2}\n",
		"a map tag":                     "leaf: !!map {s: x}\n",
		"a seq tag":                     "leaf: {xs: !!seq [1, 2]}\n",
		"a slice element":               "list: [!!map {s: one}, {s: two}]\n",
		"a map value":                   "plain: {k: !!str 7}\n",
		"a struct map value":            "byName: {a: !!map {s: aa}}\n",
		"a pointer":                     "ptr: !!map {s: p}\n",
		"a block root":                  "!!map\nname: top\n",
		"a flow root":                   "--- !!map {name: top}\n",
		"an any":                        "any: !!str 3\n",
		"inside an any":                 "any: {k: !!str 3}\n",
		"a field no field claims":       "nope: !!str x\nname: m\n",
		"a null on a pointer":           "ptr: !!null ~\n",
		"an empty string":               "name: !!str\n",
		"a merge inside":                "leaf: !!map {<<: {s: a}, i: 1}\n",
		"an anchor on a tagged scalar":  "name: &n !!str 1\nplain: {k: *n}\n",
		"an anchor on a tagged mapping": "leaf: &l !!map {s: x}\nptr: *l\nlist: [*l]\n",
		"a default kept":                "leaf: !!map {i: 4}\n",
		"a string tag into an int":      "leaf: {i: !!str 5}\n",
		"an alias to a tagged any":      "any: &m !!omap [{x: 1}]\nlist2: [*m, *m]\n",
		"an alias to a tagged scalar":   "anys: {a: &s !!str 1, b: *s}\n",
	}
	for _, share := range []bool{false, true} {
		for name, src := range docs {
			t.Run(fmt.Sprintf("share=%v/%s", share, name), func(t *testing.T) {
				var opts []DecodeOption
				if share {
					opts = append(opts, ShareAliases())
				}
				r := decodeBothWays(t, src, opts...)
				if r.walkErr != nil || r.treeErr != nil {
					t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
				}
				if !reflect.DeepEqual(r.walked, r.built) {
					t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
				}
				if r.dec.parsedFile != nil {
					t.Error("the decode built a tree")
				}
			})
		}
	}
}

// TestATagTheWalkRefusesAsTheTreeDoes requires the tree's error, word for
// word, from a tagged node the tree decoder refuses. The walk hands such a
// decode to the tree, which names the field and decodes the fields after it.
func TestATagTheWalkRefusesAsTheTreeDoes(t *testing.T) {
	for name, src := range map[string]string{
		"an int tag on text":      "leaf: {i: !!int x}\n",
		"a map tag on a sequence": "leaf: {xs: !!map {a: 1}}\n",
		"a sequence into a field": "name: !!seq [1]\nleaf: {s: after}\n",
		"a tagged root":           "!!seq [1]\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if r.walkErr == nil || r.treeErr == nil {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr.Error() != r.treeErr.Error() {
				t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths fill the value differently:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
		})
	}
}

// TestATagTheWalkCannotReadGoesToTheTree pins the tagged nodes the walk still
// hands to the tree, so each one that starts to walk is noticed and moved
// above.
func TestATagTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an anchor inside a tagged node": "leaf: !!map {s: &s x}\nname: *s\n",
		"an alias inside a tagged node":  "name: &n x\nleaf: !!map {s: *n}\n",
		"a tag on an anchor":             "name: !!str &n 1\nplain: {k: *n}\n",
		"a tagged key":                   "plain: {!!str 1: v}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if r.walkErr != nil || r.treeErr != nil {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile == nil {
				t.Error("the walk read it: move the case to TestATagKeepsTheWalk")
			}
		})
	}
}
