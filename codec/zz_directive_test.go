// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// A directive whose name begins with "&" or "*" was read as a node property.
//
// 6.8 makes a directive a "%", a name of ns-chars, and its parameters, and says
// an unknown one is ignored with a warning. "&" and "*" are ns-chars, so
// "%&AML 1.2" is a well-formed directive named "&AML".
//
// stageProperties runs before stageDirectives, so it had already folded the
// "&AML" and the "1.2" into an anchor group by the time the directive was
// recognized. parseDirectiveName then built an anchor from that group and
// handed it to the walk, at depth 0 and before the directive itself, so the
// walk took it for the document's root: "%&AML 1.2" over "---" over "k: v"
// walked to 1.2 with the mapping gone, and ToJSON wrote 1.2. The tree read the
// mapping, which is what made it the walk's defect and not the scanner's.
//
// A directive's name and parameters are not nodes of the document, so nothing
// built for them goes over to a walk now. The anchor is still built and still
// registers nothing -- "*AML" after "%&AML 1.2" has never resolved -- so this
// is the hand-over alone.
//
// libfyaml 1.0.0b1 reads the document in every case and the reference parser
// passes them; go.yaml.in/yaml/v3 refuses the directive line outright, which is
// the other defensible answer. Found on 2026-09-07 by a mutation of the "%YAML"
// line Style.Version writes -- a "Y" turned into a "&".

// TestFixedADirectiveNamedLikeAnAnchorIsStillADirective holds the three readers
// to the document the directive stands in front of.
func TestFixedADirectiveNamedLikeAnAnchorIsStillADirective(t *testing.T) {
	for _, tc := range []struct{ src, reads, writes string }{
		{src: "%&AML 1.2\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		{src: "%&AML 1.2\n---\nk: v\n", reads: `map[string]interface {}{"k":"v"}`, writes: `{"k":"v"}`},
		{src: "%&x y\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// No parameter, so the anchor named the empty node.
		{src: "%&AML\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// Two of them, and one standing after a "%TAG".
		{src: "%&AML 1.2\n%&BML 2.0\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		{src: "%TAG !e! tag:yaml.org,2002:\n%&AML 1.2\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// The parameter is a collection, which the walk used to hand over whole.
		{src: "%&AML [1, 2]\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
	} {
		var walked any
		require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &walked), "%q", tc.src)
		assert.Equalf(t, tc.reads, fmt.Sprintf("%#v", walked), "the walk reads past the directive: %q", tc.src)

		out, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.writes, string(out), "%q", tc.src)
	}

	t.Run("and the name registers no anchor", func(t *testing.T) {
		var v any
		err := codec.Unmarshal([]byte("%&AML 1.2\n---\nk: *AML\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "AML"`)
	})

	t.Run("any other name is a directive and is ignored", func(t *testing.T) {
		for _, src := range []string{
			"%FOO 1.2\n---\n-8.5\n",
			"%!x y\n---\n-8.5\n",
			// The indicator has to lead: "A&ML" is an ordinary name.
			"%A&ML 1.2\n---\n-8.5\n",
		} {
			out, err := codec.ToJSON([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			assert.Equal(t, "-8.5", string(out), "%q", src)

			assert.Equalf(t, "-8.5", fmt.Sprintf("%#v", treeRead(t, src)), "%q", src)
		}
	})
}

// TestDefectADirectiveNamedLikeAnAliasIsRefused is the half still open.
//
// "%*x y" is a well-formed directive named "*x", and all three readers refuse
// it with `could not find alias "x"` -- a message about a node, for a line that
// holds none. libfyaml 1.0.0b1 reads the document and the reference parser
// passes it; go.yaml.in/yaml/v3 refuses the directive line, so refusing is
// defensible and the message is not. Unlike the anchor half this was never a
// silent loss: the three paths agree, and they agree on an error.
func TestDefectADirectiveNamedLikeAnAliasIsRefused(t *testing.T) {
	for _, src := range []string{"%*x y\n---\n-8.5\n", "%*x\n---\n-8.5\n"} {
		var got any
		err := codec.Unmarshal([]byte(src), &got)
		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), `could not find alias "x"`, "today: %q", src)

		terr := codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap())
		require.Errorf(t, terr, "%q", src)
		assert.Containsf(t, terr.Error(), `could not find alias "x"`, "today: %q", src)

		_, jerr := codec.ToJSON([]byte(src))
		assert.Errorf(t, jerr, "%q", src)
	}
}
