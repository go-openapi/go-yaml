// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"slices"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
)

// holdsACollectionKey reports whether a mapping's key is a collection anywhere
// in v.
func holdsACollectionKey(v yamlgen.Value) bool {
	switch n := v.(type) {
	case yamlgen.Map:
		for _, p := range n.Pairs {
			// Peeled, because a key carries an anchor and a tag like any other
			// node: "&a1 !!map" over a mapping still writes its "?". Asking the
			// key's own type missed that and reported the label as outrunning
			// the style.
			switch k := peelKey(p.Key).(type) {
			case yamlgen.Seq:
				if len(k.Items) > 0 {
					return true
				}
			case yamlgen.Map:
				if len(k.Pairs) > 0 {
					return true
				}
			}

			if holdsACollectionKey(p.Key) || holdsACollectionKey(p.Val) {
				return true
			}
		}
	case yamlgen.Seq:
		return slices.ContainsFunc(n.Items, holdsACollectionKey)
	case yamlgen.Anchored:
		return holdsACollectionKey(n.V)
	case yamlgen.Alias:
		return holdsACollectionKey(n.V)
	case yamlgen.Tagged:
		return holdsACollectionKey(n.V)
	}

	return false
}

// peelKey returns the node an anchor and a tag decorate.
func peelKey(v yamlgen.Value) yamlgen.Value {
	for {
		switch n := v.(type) {
		case yamlgen.Anchored:
			v = n.V
		case yamlgen.Tagged:
			v = n.V
		default:
			return v
		}
	}
}
