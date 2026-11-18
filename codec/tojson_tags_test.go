// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestToJSONReadsTheTagsItKnows records what each tag of the YAML type
// repository converts to, and that a tag it does not know is transparent.
//
// A tag written out reaches the converter differently from one left implicit,
// and "!!merge <<: *base" is where that mattered: the merge machinery looked
// for an ast.MergeKeyNode and the key arrived wrapped in a tag, so the "<<"
// was written as a key with no value and the JSON did not parse.
func TestToJSONReadsTheTagsItKnows(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"str", "a: !!str 12\n", `{"a":"12"}`},
		{"int", `a: !!int "12"` + "\n", `{"a":12}`},
		{"float", "a: !!float 12\n", `{"a":12.0}`},
		{"bool", "a: !!bool yes\n", `{"a":true}`},
		{"null", "a: !!null ~\n", `{"a":null}`},
		{"binary", "a: !!binary aGk=\n", `{"a":"aGk="}`},

		// Transparent: the value is written as it stands.
		{"seq", "a: !!seq [1,2]\n", `{"a":[1,2]}`},
		{"map", "a: !!map {x: 1}\n", `{"a":{"x":1}}`},
		{"set", "a: !!set {x, y}\n", `{"a":{"x":null,"y":null}}`},
		// Not transparent: the tag names an ordered map, and JSON has no
		// ordering to lose, so the entries are written as one object holding
		// the order the sequence gave them.
		{"omap", "a: !!omap [{x: 1},{y: 2}]\n", `{"a":{"x":1,"y":2}}`},

		// A timestamp is written as the instant it names, in RFC 3339, which is
		// what the value converter writes for the time.Time the decoder builds.
		// yaml.org/type/timestamp.html spells one several ways this does not.
		{"timestamp", "a: !!timestamp 2001-12-14\n", `{"a":"2001-12-14T00:00:00Z"}`},
		// ⚠️ A tag the core schema does not resolve leaves its scalar as text,
		// digits and all. The scanner types a scalar by its own grammar and a
		// tag it cannot resolve is not that grammar, so "!thing 12" is the
		// string "12" and not the number.
		{"local", "a: !thing 12\n", `{"a":"12"}`},
		{"unknown namespace", "a: !<tag:example.com,2020:thing> 12\n", `{"a":"12"}`},

		// ⚠️ And so are the three tags of the 1.1 type repository nothing here
		// resolves. They are parsed and carried on the node; the value under
		// them stands as it was written.
		{"pairs", "a: !!pairs [{x: 1},{x: 2}]\n", `{"a":[{"x":1},{"x":2}]}`},
		{"value", "a: !!value =\n", `{"a":"="}`},
		{"yaml", "a: !!yaml '!'\n", `{"a":"!"}`},

		// The long form of a tag the core schema resolves. "!!int" is
		// shorthand for tag:yaml.org,2002:int, so the two mean the same.
		{"long form int", "a: !<tag:yaml.org,2002:int> \"12\"\n", `{"a":12}`},
		{"long form str", "a: !<tag:yaml.org,2002:str> 12\n", `{"a":"12"}`},
		{"long form bool", "a: !<tag:yaml.org,2002:bool> yes\n", `{"a":true}`},
		{"long form seq", "a: !<tag:yaml.org,2002:seq> [1,2]\n", `{"a":[1,2]}`},
		{"long form map", "a: !<tag:yaml.org,2002:map> {x: 1}\n", `{"a":{"x":1}}`},
		{"long form merge", "b: &b {x: 1}\na:\n  !<tag:yaml.org,2002:merge> <<: *b\n", `{"b":{"x":1},"a":{"x":1}}`},

		// The merge tag written out reads as the "<<" it stands on.
		{"merge", "b: &b {x: 1}\na:\n  !!merge <<: *b\n", `{"b":{"x":1},"a":{"x":1}}`},
		{"merge overridden", "b: &b {x: 1}\na:\n  !!merge <<: *b\n  x: 9\n", `{"b":{"x":1},"a":{"x":9}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			require.Truef(t, json.Valid(got), "wrote %s, which is not JSON", got)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
