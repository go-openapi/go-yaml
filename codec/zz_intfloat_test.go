// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFixedAFloatUnderAnIntegerTagIsRefused holds both readers to the complete
// representation.
//
// tag:yaml.org,2002:int names the whole numbers, so a float under "!!int" is a
// tag that does not hold. The library used to resolve it and truncate: "1.9"
// came back 1. That lenience carried the infinities and the NaN in with it,
// where there is nothing to truncate to -- castToInteger converted them with
// Go's int(v), and the Go specification leaves the result outside the integer
// range to the implementation. On amd64 ".inf", "-.inf" and ".nan" all decoded
// to -9223372036854775808, so a caller could not tell the three apart, and
// "1e400" decoded to 0.
func TestFixedAFloatUnderAnIntegerTagIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, says string }{
		{src: "k: !!int 1.9\n", says: `cannot read "1.9" as !!int`},
		{src: "k: !!int -1.9\n", says: `cannot read "-1.9" as !!int`},
		{src: "k: !!int 1e3\n", says: `cannot read "1e3" as !!int`},
		{src: "k: !!int 1e400\n", says: `cannot read "1e400" as !!int`},
		{src: "k: !!int .inf\n", says: `cannot read ".inf" as !!int`},
		{src: "k: !!int -.inf\n", says: `cannot read "-.inf" as !!int`},
		{src: "k: !!int .nan\n", says: `cannot read ".nan" as !!int`},
		{src: "!!int .inf: v\n", says: `cannot read ".inf" as !!int`},
		{src: "!!int 1.9: v\n", says: `cannot read "1.9" as !!int`},
	} {
		var v any
		err := codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v)
		require.Errorf(t, err, "%q should be refused", tc.src)
		assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
	}

	t.Run("and a whole number under the tag still reads, in any base", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{src: "k: !!int 16\n", want: 16},
			{src: "k: !!int 0x10\n", want: 16},
			{src: "k: !!int 0o17\n", want: 15},
			{src: "k: !!int -5\n", want: -5},
			// A tag standing on nothing keeps taking the tag's own default.
			{src: "k: !!int\n", want: uint64(0)},
		} {
			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	t.Run("and WithLaxTags reads the text instead", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{src: "k: !!int 1.9\n", want: "1.9"},
			{src: "k: !!int .inf\n", want: ".inf"},
			{src: "k: !!int .nan\n", want: ".nan"},
			{src: "k: !!int 1e400\n", want: "1e400"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithLaxTags())
			require.NoErrorf(t, err, "%q", tc.src)

			var v map[string]any
			require.NoErrorf(t,
				codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	// ToJSON reads the same verdict, so it refuses the same documents. The
	// house rule is strict by default, and parser.WithLaxTags relaxes both.
	t.Run("and the JSON converter refuses them too", func(t *testing.T) {
		for _, src := range []string{"k: !!int 1.9\n", "k: !!int 1e3\n", "k: !!int 1e400\n"} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "as !!int", "%q", src)

			got, laxErr := codec.ToJSON([]byte(src), parser.WithLaxTags())
			require.NoErrorf(t, laxErr, "%q under WithLaxTags", src)
			assert.Containsf(t, string(got), `"k":"`, "%q reads the text under WithLaxTags", src)
		}
	})
}

// TestTheTwoReadersAgreeOnATaggedNumber holds ToJSON to the decoder, spelling
// by spelling and version by version.
//
// Both read ast.Resolution, so they refuse the same documents. They disagreed
// about what the ones they accept mean: codec.taggedInteger and
// codec.taggedFloat sniffed Resolution.Text again with
// token.ScalarType(text, token.Schema12), a third copy of the fault ast had, so
// they never saw the base the document's own schema gave the digits. Under
// %YAML 1.1 "!!int 0b101" decoded to 5 and ToJSON wrote 0, "!!int 190:20:30"
// decoded to 685230 and ToJSON wrote 0, and "!!int 017" decoded to 15 and
// ToJSON wrote 17. They read Resolution.Type now.
func TestTheTwoReadersAgreeOnATaggedNumber(t *testing.T) {
	const under11 = "%YAML 1.1\n---\n"

	for _, tc := range []struct {
		body        string
		value, json string
	}{
		{body: "k: !!int 017", value: "15", json: "15"},
		{body: "k: !!int 0b101", value: "5", json: "5"},
		{body: "k: !!int 1_000", value: "1000", json: "1000"},
		{body: "k: !!int 190:20:30", value: "685230", json: "685230"},
		{body: "k: !!float 190:20:30.5", value: "685230.5", json: "685230.5"},
		{body: "k: !!float 1_0.5", value: "10.5", json: "10.5"},
		{body: "k: !!float 017", value: "15", json: "15.0"},
	} {
		src := under11 + tc.body + "\n"

		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v), "%q", src)
		assert.Equalf(t, tc.value, fmt.Sprintf("%v", v["k"]), "decoded %q", tc.body)

		got, err := codec.ToJSON([]byte(src))
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, `{"k":`+tc.json+"}", strings.TrimSpace(string(got)), "ToJSON %q", tc.body)
	}

	// A number no float64 holds keeps its magnitude in JSON, which writes the
	// digits and has no float64 to overflow.
	t.Run("and a number past float64 keeps its magnitude", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("k: !!float 1e400\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"k":1e+400}`, strings.TrimSpace(string(got)))
	})
}

// TestQuotingAScalarDoesNotChangeWhatItsTagMakesOfIt.
//
// YAML 1.2.2 §3.3.2 gives a non-specific tag -- "!" to a non-plain scalar, "?"
// to everything else -- only to a node *lacking* an explicit tag. A node
// carrying one has no non-specific tag to resolve, so `!!int "017"` and
// `!!int 017` are one node: the same tag over the same content. Nothing in the
// spec lets them differ.
//
// Three readings told them apart. ast.readsAs keyed the spelling rules on the
// type the scanner gave the scalar, which types a plain scalar and not a quoted
// one; Decoder.taggedValue resolved the node rather than the tagged text; and
// codec.castToInteger read that text under a fixed 1.2 schema. Under
// %YAML 1.1, `!!int "017"` came back 17 where `!!int 017` was 15 -- both
// integers, no error, two units gone -- and `!!int "0b101"` was 0 where the
// plain spelling was 5.
func TestQuotingAScalarDoesNotChangeWhatItsTagMakesOfIt(t *testing.T) {
	texts := []string{
		"0x10", "0o17", "017", "09", "0b101", "1_000", "190:20:30", "0X10", "-0x10",
		"1", "1.9", "1e3", "1.0e3", "1.0e+3", "1e400", "1_0.5", "0x1p-2", ".inf", ".nan", "-0",
	}

	for _, prefix := range []string{"", "%YAML 1.1\n---\n"} {
		for _, tag := range []string{"!!int", "!!float"} {
			for _, text := range texts {
				plain := prefix + "k: " + tag + " " + text + "\n"
				quoted := prefix + "k: " + tag + ` "` + text + "\"\n"

				var got, want any
				plainErr := codec.NewDecoder(strings.NewReader(plain)).Decode(&want)
				quotedErr := codec.NewDecoder(strings.NewReader(quoted)).Decode(&got)

				require.Equalf(t, plainErr == nil, quotedErr == nil,
					"%q and its quoted spelling must both read or both be refused", plain)
				if plainErr != nil {
					continue
				}
				assert.Equalf(t, fmt.Sprintf("%v", want), fmt.Sprintf("%v", got),
					"quoting %q under %q changed what the tag made of it", text, tag)
			}
		}
	}
}

// TestAnInfinityUnderAFloatTagKeepsItsValue guards the branch that reads one.
//
// token.ParseFloat goes through shapeOfTypedNumber, which has no case for
// InfinityType or NanType and reports false for both, so a taggedFloat routing
// them through it answered 0 -- the incomplete representation ruled out for
// "!!int" on 2026-09-09, arriving by another door.
func TestAnInfinityUnderAFloatTagKeepsItsValue(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want func(float64) bool
	}{
		{src: "k: !!float .inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .Inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float -.inf\n", want: func(f float64) bool { return math.IsInf(f, -1) }},
		{src: `k: !!float ".inf"` + "\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .nan\n", want: math.IsNaN},
		{src: `k: !!float ".nan"` + "\n", want: math.IsNaN},
	} {
		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)

		f, ok := v["k"].(float64)
		require.Truef(t, ok, "%q should read as a float64, got %T", tc.src, v["k"])
		assert.Truef(t, tc.want(f), "%q read as %v", tc.src, f)
	}
}
