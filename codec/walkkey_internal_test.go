// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"reflect"
	"testing"
)

// TestAKeptKeyKeepsTheWalk reads documents whose keys are written with an
// explicit "?" or a tag, with and without ShareAliases, and requires the value
// the tree gives without the decode building a tree. The walk holds such a key
// whole and names the entry by it, as the tree does.
func TestAKeptKeyKeepsTheWalk(t *testing.T) {
	docs := map[string]string{
		"a block key":                "? name\n: top\n",
		"a block key in a field":     "leaf:\n  ? s\n  : x\n  i: 2\n",
		"a flow key":                 "leaf: {? s : x, i: 2}\n",
		"a key with no value":        "leaf: {? s, i: 2}\n",
		"into a map":                 "plain: {? k : v, j: w}\n",
		"a number key into a map":    "plain: {? 1 : v}\n",
		"a key no field claims":      "? nope\n: x\nname: m\n",
		"a collection key skipped":   "? [a, b]\n: x\nname: m\n",
		"a merge key":                "leaf: {? << : {s: a}, i: 1}\n",
		"in a sequence entry":        "list:\n- ? s\n  : one\n- s: two\n",
		"in a flow sequence":         "list: [{? s : one}, {s: two}]\n",
		"replayed for an alias":      "ptr: &p {? s : x}\nleaf: *p\n",
		"a tag inside the key":       "plain: {? !!str 1 : v}\n",
		"a tagged key into a map":    "plain: {!!str 1: v, !!int 2: w}\n",
		"a tagged key into a field":  "leaf: {!!str s: x}\n",
		"a local tag on a key":       "leaf: {!k s: x}\n",
		"a tagged key in a sequence": "list: [{!!str s: one}, {s: two}]\n",
		"inside an any":              "any: {? k : v, !!str 1: w}\n",
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

// TestAKeptKeyTheWalkCannotReadGoesToTheTree pins the keys the walk still hands
// to the tree, so each one that starts to walk is noticed and moved above. It
// requires the tree's value or its error from both.
func TestAKeptKeyTheWalkCannotReadGoesToTheTree(t *testing.T) {
	for name, src := range map[string]string{
		"an anchor inside a key":      "? &a name\n: top\nplain: {k: *a}\n",
		"an alias inside a key":       "name: &n k\nplain: {? *n : v}\n",
		"a collection key into a map": "plain: {? [a] : v}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if (r.walkErr == nil) != (r.treeErr == nil) {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr != nil {
				if r.walkErr.Error() != r.treeErr.Error() {
					t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
				}

				return
			}
			if !reflect.DeepEqual(r.walked, r.built) {
				t.Errorf("the two paths disagree:\n  walk %+v\n  tree %+v", r.walked, r.built)
			}
			if r.dec.parsedFile == nil {
				t.Error("the walk read it: move the case to TestAKeptKeyKeepsTheWalk")
			}
		})
	}
}

// TestAKeptKeyRefusesARepeatAsTheTreeDoes requires the tree's error, word for
// word, from a key written with "?" or a tag that repeats another: a repeat the
// parse records, and two keys that one name makes one, which a reader keyed by
// name refuses. Where the refusal comes from is left out: a repeat written
// with plain keys reaches the tree too.
func TestAKeptKeyRefusesARepeatAsTheTreeDoes(t *testing.T) {
	for name, src := range map[string]string{
		"a repeat in a field":      "leaf: {? s : x, s: y}\n",
		"one name into a map":      "plain: {? 1 : x, \"1\": y}\n",
		"a tagged key of one name": "plain: {!!str 1: x, 1: y}\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := decodeBothWays(t, src)
			if r.walkErr == nil || r.treeErr == nil {
				t.Fatalf("walk err=%v, tree err=%v", r.walkErr, r.treeErr)
			}
			if r.walkErr.Error() != r.treeErr.Error() {
				t.Errorf("the two paths refuse differently:\n  walk %v\n  tree %v", r.walkErr, r.treeErr)
			}
		})
	}
}
