// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_ScientificNotation(t *testing.T) {
	for test := range decoderScientificNotationCases() {
		t.Run(test.source, func(t *testing.T) {
			buf := bytes.NewBufferString(test.source)
			dec := codec.NewDecoder(buf)
			typ := reflect.ValueOf(test.value).Type()
			value := reflect.New(typ)
			if err := dec.Decode(value.Interface()); err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				require.NoError(t, err)
			}
			actual := fmt.Sprintf("%+v", value.Elem().Interface())
			expect := fmt.Sprintf("%+v", test.value)
			require.Equal(t, expect, actual)
		})
	}
}

// decoderScientificNotationCase holds a number in scientific notation and the value it decodes to.
type decoderScientificNotationCase struct {
	source string
	value  any
}

func decoderScientificNotationCases() iter.Seq[decoderScientificNotationCase] {
	return slices.Values([]decoderScientificNotationCase{
		{
			"v: 1e3",
			map[string]uint{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]uint{"v": 0},
		},
		{
			"v: 1e3",
			map[string]int{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]int{"v": 0},
		},
		{
			"v: 1e3",
			map[string]float32{"v": 1000},
		},
		{
			"v: 1.0e3",
			map[string]float64{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]float64{"v": 0.001},
		},
		{
			"v: 1.0e-3",
			map[string]float64{"v": 0.001},
		},
		{
			"v: 1.0e+3",
			map[string]float64{"v": 1000},
		},
		{
			"v: 1.0e+3",
			map[string]float64{"v": 1000},
		},
	})
}

// A number past big.Float's exponent decoded to zero.
//
// The parser reads a number no machine type holds as a big.Int or a big.Float.
// A big.Float keeps its exponent in an int32, so past 1e2147483647 there was no
// big.Float to build -- and the decoder fell back to a float64, which cannot
// hold it either and came back as zero. Nothing was reported.
//
// A float whose decimal exponent is past ±1000 now reads as an infinity of its
// sign, or zero, before any big.Float is built: see token.FloatPastRange.
//
// ToJSON writes what the document said, and JSON puts no bound on the
// magnitude of a number. Found by regenerating the corpus on 2026-09-10,
// through the seed set fuzzseeds.All() draws from it.

func decoded(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v), "%q", src)

	return v.(map[string]any)["a"]
}

// TestFixedANumberPastBigFloatIsAnInfinity: each of these came back as zero.
func TestFixedANumberPastBigFloatIsAnInfinity(t *testing.T) {
	for src, want := range map[string]float64{
		"a: 1e2147483647\n":        math.Inf(1),
		"a: -3.96068E4059375226\n": math.Inf(-1),
		"a: -36.74E69647216797\n":  math.Inf(-1),
	} {
		assert.Equal(t, want, decoded(t, src), "%q", src)
	}
}

// TestAWideNumberBelowThatBoundIsKept holds the numbers inside the ±1000 bound,
// which keep the width they need.
func TestAWideNumberBelowThatBoundIsKept(t *testing.T) {
	for tc := range aWideNumberBelowThatBoundIsKeptCases() {
		assert.Equal(t, tc.want, typeName(decoded(t, tc.src)), "%q", tc.src)
	}
}

// aWideNumberBelowThatBoundIsKeptCase holds a document and the Go type its number decodes to.
type aWideNumberBelowThatBoundIsKeptCase struct {
	src, want string
}

func aWideNumberBelowThatBoundIsKeptCases() iter.Seq[aWideNumberBelowThatBoundIsKeptCase] {
	return slices.Values([]aWideNumberBelowThatBoundIsKeptCase{
		{src: "a: 1e308\n", want: "float64"},
		{src: "a: 1e309\n", want: "*big.Float"},
		{src: "a: 1E400\n", want: "*big.Float"},
		{src: "a: 1e-400\n", want: "*big.Float"},
		{src: "a: 123456789012345678901234567890\n", want: "*big.Int"},
	})
}

// TestToJSONKeepsTheNumberWhateverItsWidth holds the half that is right, so a
// fix to the decoder does not take it along.
func TestToJSONKeepsTheNumberWhateverItsWidth(t *testing.T) {
	for tc := range toJSONKeepsTheNumberWhateverItsWidthCases() {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoError(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out),
			"ToJSON writes what the document said; JSON bounds no number's magnitude")
	}
}

// toJSONKeepsTheNumberWhateverItsWidthCase holds a document and the JSON ToJSON writes for it.
type toJSONKeepsTheNumberWhateverItsWidthCase struct {
	src, want string
}

func toJSONKeepsTheNumberWhateverItsWidthCases() iter.Seq[toJSONKeepsTheNumberWhateverItsWidthCase] {
	return slices.Values([]toJSONKeepsTheNumberWhateverItsWidthCase{
		{src: "a: 1e2147483647\n", want: `{"a":1e2147483647}`},
		{src: "a: -3.96068E4059375226\n", want: `{"a":-3.96068E4059375226}`},
		{src: "a: 1E400\n", want: `{"a":1E400}`},
	})
}

func typeName(v any) string {
	switch v.(type) {
	case float64:
		return "float64"
	case *big.Float:
		return "*big.Float"
	case *big.Int:
		return "*big.Int"
	default:
		return "other"
	}
}

// TestFixedAnIntTagOnAWideIntegerKeepsItsDigits: `!!int` on an integer past a
// machine word converts to the number, as the same integer untagged always did.
//
// taggedInteger read the text with strconv.ParseInt and the converter wrote the
// int64 it got back, so every wide integer came out as math.MinInt64 whatever
// its value and whatever its sign. It reads token.ParseBigInteger now, which is
// what ast.IntegerNode.GetValue reads for the untagged spelling.
func TestFixedAnIntTagOnAWideIntegerKeepsItsDigits(t *testing.T) {
	for tc := range fixedAnIntTagOnAWideIntegerKeepsItsDigitsCases() {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out), "%q", tc.src)
	}

	t.Run("the decoder reads it as a big.Int either way", func(t *testing.T) {
		for _, src := range []string{"!!int 123456789012345678901\n", "123456789012345678901\n"} {
			var v any
			require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v))
			assert.IsType(t, new(big.Int), v, "%q", src)
		}
	})
}

// fixedAnIntTagOnAWideIntegerKeepsItsDigitsCase holds an integer document and the JSON ToJSON writes for it.
type fixedAnIntTagOnAWideIntegerKeepsItsDigitsCase struct {
	src, want string
}

func fixedAnIntTagOnAWideIntegerKeepsItsDigitsCases() iter.Seq[fixedAnIntTagOnAWideIntegerKeepsItsDigitsCase] {
	return slices.Values([]fixedAnIntTagOnAWideIntegerKeepsItsDigitsCase{
		{src: "!!int 123456789012345678901\n", want: "123456789012345678901"},
		{src: "!!int -123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int 18446744073709551616\n", want: "18446744073709551616"},
		{src: "123456789012345678901\n", want: "123456789012345678901"},
		{src: "-123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int -5\n", want: "-5"},
		{src: "!!int 0x1f\n", want: "31"},
	})
}

// TestBigNumberReachesTheDecoder holds what a document's number does when no
// native type has room for it.
//
// YAML 1.2 puts no bound on an integer -- "arbitrary sized finite mathematical
// integers" -- and none on a float beyond the implementation's. So a number is
// read exactly and handed to the decoder, which decides what to do with it,
// rather than truncated on the way. "!!int 18446744073709551616" used to
// decode as math.MaxInt64, silently.
//
// The plain, untagged forms still resolve to a string until the scanner types
// scalars by their grammar; the tag is how one reaches the integer node today.
func TestBigNumberReachesTheDecoder(t *testing.T) {
	const (
		pastUint64 = "18446744073709551616" // 2^64
		pastInt64  = "-9223372036854775809" // one below the smallest int64
	)

	t.Run("into any, the number arrives whole", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastUint64, n.String())
	})

	t.Run("into a native integer, it overflows and says so", func(t *testing.T) {
		var m map[string]int64
		err := codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overflow")
		assert.Contains(t, err.Error(), pastUint64)
	})

	t.Run("below the smallest int64", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastInt64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastInt64, n.String())
	})

	t.Run("into a float, it takes the nearest one", func(t *testing.T) {
		var m map[string]float64
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, math.Pow(2, 64), m["v"])
	})

	t.Run("into a string, it keeps its digits", func(t *testing.T) {
		var m map[string]string
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, pastUint64, m["v"])
	})
}

// TestAWideNumberIsRefusedAndAnInfinityIsNot holds the line between the two.
//
// ±Inf is a float64 like any other, and a document naming one gets it: ".inf"
// resolves to an ast.InfinityNode, which holds a float64 already. A number
// whose digits are finite and too wide resolves to a *big.Float, and a float64
// of ±Inf from those has lost the number rather than named it -- that one is
// refused, where before it was answered with +Inf and nothing said.
//
// The two are told apart by where they arrive from and not by the value, which
// is why this is worth a test: the routing is what makes it work.
func TestAWideNumberIsRefusedAndAnInfinityIsNot(t *testing.T) {
	t.Run("an infinity the document named", func(t *testing.T) {
		for _, src := range []string{"a: .inf\n", "a: .Inf\n", "a: -.inf\n", "a: !!float .inf\n"} {
			var into struct{ A float64 }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &into), "%q", src)
			assert.Truef(t, math.IsInf(into.A, 0), "%q read as %v", src, into.A)
		}
	})

	t.Run("a number no float64 holds", func(t *testing.T) {
		for _, src := range []string{"a: 1e400\n", "a: -1e400\n", "a: 1e-400\n"} {
			var f64 struct{ A float64 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f64), "%q read as %v", src, f64.A)

			var f32 struct{ A float32 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f32), "%q read as %v", src, f32.A)

			// Whole, where the caller asks for it whole.
			var wide struct{ A *big.Float }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &wide), "%q", src)
			assert.Falsef(t, wide.A.IsInf(), "%q read as %v", src, wide.A)
		}
	})

	t.Run("a number a float64 only rounds", func(t *testing.T) {
		var into struct{ A float64 }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.InEpsilon(t, 1.2345678901234568e+29, into.A, 1e-15)
	})

	t.Run("a wide integer whole", func(t *testing.T) {
		var into struct{ A *big.Int }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.Equal(t, "123456789012345678901234567890", into.A.String())
	})
}

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
	for tc := range fixedAFloatUnderAnIntegerTagIsRefusedCases() {
		var v any
		err := codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v)
		require.Errorf(t, err, "%q should be refused", tc.src)
		assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
	}

	t.Run("and a whole number under the tag still reads, in any base", func(t *testing.T) {
		for tc := range wholeNumberUnderAnIntTagCases() {
			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	t.Run("and a tagged and an untagged integer are one key", func(t *testing.T) {
		// "!!int 1" decoded to int(1) and "1" to uint64(1): two keys to a Go
		// map, so an own "!!int 1" did not beat the "1" a "<<" brought in and
		// the mapping held both entries, whichever side carried the tag.
		for _, pair := range [][2]string{{"1", "!!int 1"}, {"!!int 1", "1"}, {"-1", "!!int -1"}} {
			src := "%YAML 1.1\n---\na: &m\n  ? " + pair[0] + "\n  : merged\n" +
				"b:\n  <<: *m\n  ? " + pair[1] + "\n  : own\n"

			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v), "%q", src)
			b, isMap := v["b"].(map[any]any)
			require.Truef(t, isMap, "%q: b is %T", src, v["b"])
			assert.Lenf(t, b, 1, "%q", src)
			for _, value := range b {
				assert.Equalf(t, "own", value, "%q", src)
			}
		}
	})

	t.Run("and WithLaxTags reads the text instead", func(t *testing.T) {
		for tc := range laxIntTagReadsTheTextCases() {
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

// fixedAFloatUnderAnIntegerTagIsRefusedCase holds a float under !!int and the text its error contains.
type fixedAFloatUnderAnIntegerTagIsRefusedCase struct {
	src, says string
}

func fixedAFloatUnderAnIntegerTagIsRefusedCases() iter.Seq[fixedAFloatUnderAnIntegerTagIsRefusedCase] {
	return slices.Values([]fixedAFloatUnderAnIntegerTagIsRefusedCase{
		{src: "k: !!int 1.9\n", says: `cannot read "1.9" as !!int`},
		{src: "k: !!int -1.9\n", says: `cannot read "-1.9" as !!int`},
		{src: "k: !!int 1e3\n", says: `cannot read "1e3" as !!int`},
		{src: "k: !!int 1e400\n", says: `cannot read "1e400" as !!int`},
		{src: "k: !!int .inf\n", says: `cannot read ".inf" as !!int`},
		{src: "k: !!int -.inf\n", says: `cannot read "-.inf" as !!int`},
		{src: "k: !!int .nan\n", says: `cannot read ".nan" as !!int`},
		{src: "!!int .inf: v\n", says: `cannot read ".inf" as !!int`},
		{src: "!!int 1.9: v\n", says: `cannot read "1.9" as !!int`},
	})
}

// wholeNumberUnderAnIntTagCase holds a whole number under !!int and the value it decodes to.
type wholeNumberUnderAnIntTagCase struct {
	src  string
	want any
}

func wholeNumberUnderAnIntTagCases() iter.Seq[wholeNumberUnderAnIntTagCase] {
	return slices.Values([]wholeNumberUnderAnIntTagCase{
		// The Go type the untagged number decodes to.
		{src: "k: !!int 16\n", want: uint64(16)},
		{src: "k: !!int 0x10\n", want: uint64(16)},
		{src: "k: !!int 0o17\n", want: uint64(15)},
		{src: "k: !!int -5\n", want: int64(-5)},
		// A tag standing on nothing keeps taking the tag's own default.
		{src: "k: !!int\n", want: uint64(0)},
	})
}

// laxIntTagReadsTheTextCase holds a float under !!int and the text WithLaxTags reads it as.
type laxIntTagReadsTheTextCase struct {
	src, want string
}

func laxIntTagReadsTheTextCases() iter.Seq[laxIntTagReadsTheTextCase] {
	return slices.Values([]laxIntTagReadsTheTextCase{
		{src: "k: !!int 1.9\n", want: "1.9"},
		{src: "k: !!int .inf\n", want: ".inf"},
		{src: "k: !!int .nan\n", want: ".nan"},
		{src: "k: !!int 1e400\n", want: "1e400"},
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

	for tc := range theTwoReadersAgreeOnATaggedNumberCases() {
		src := under11 + tc.body + "\n"

		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v), "%q", src)
		assert.Equalf(t, tc.value, fmt.Sprintf("%v", v["k"]), "decoded %q", tc.body)

		got, err := codec.ToJSON([]byte(src))
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, `{"k":`+tc.json+"}", strings.TrimSpace(string(got)), "ToJSON %q", tc.body)
	}

	// A number no float64 holds keeps its magnitude in JSON, which writes the
	// digits and has no float64 to overflow -- the digits the document wrote,
	// where reading them into a *big.Float first respelled them as 1e+400.
	t.Run("and a number past float64 keeps its magnitude", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("k: !!float 1e400\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"k":1e400}`, strings.TrimSpace(string(got)))
	})
}

// theTwoReadersAgreeOnATaggedNumberCase holds a tagged number under 1.1, the value it decodes to and the JSON ToJSON writes.
type theTwoReadersAgreeOnATaggedNumberCase struct {
	body        string
	value, json string
}

func theTwoReadersAgreeOnATaggedNumberCases() iter.Seq[theTwoReadersAgreeOnATaggedNumberCase] {
	return slices.Values([]theTwoReadersAgreeOnATaggedNumberCase{
		{body: "k: !!int 017", value: "15", json: "15"},
		{body: "k: !!int 0b101", value: "5", json: "5"},
		{body: "k: !!int 1_000", value: "1000", json: "1000"},
		{body: "k: !!int 190:20:30", value: "685230", json: "685230"},
		{body: "k: !!float 190:20:30.5", value: "685230.5", json: "685230.5"},
		{body: "k: !!float 1_0.5", value: "10.5", json: "10.5"},
		{body: "k: !!float 017", value: "15", json: "15.0"},
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
	for tc := range anInfinityUnderAFloatTagKeepsItsValueCases() {
		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)

		f, ok := v["k"].(float64)
		require.Truef(t, ok, "%q should read as a float64, got %T", tc.src, v["k"])
		assert.Truef(t, tc.want(f), "%q read as %v", tc.src, f)
	}
}

// anInfinityUnderAFloatTagKeepsItsValueCase holds an infinity or NaN under !!float and a check on the float it decodes to.
type anInfinityUnderAFloatTagKeepsItsValueCase struct {
	src  string
	want func(float64) bool
}

func anInfinityUnderAFloatTagKeepsItsValueCases() iter.Seq[anInfinityUnderAFloatTagKeepsItsValueCase] {
	return slices.Values([]anInfinityUnderAFloatTagKeepsItsValueCase{
		{src: "k: !!float .inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .Inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float -.inf\n", want: func(f float64) bool { return math.IsInf(f, -1) }},
		{src: `k: !!float ".inf"` + "\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .nan\n", want: math.IsNaN},
		{src: `k: !!float ".nan"` + "\n", want: math.IsNaN},
	})
}

type (
	ratio   float64
	ratio32 float32
)

// TestAStringFillsANamedFloatType covers a string read into a float field
// through strconv.ParseFloat, which is how a quoted "1e3" fills one.
//
// The value used to come back as a plain float64 or float32, and setting it
// on a field of a named float type panicked with "reflect.Set: value of type
// float64 is not assignable to type ratio".
func TestAStringFillsANamedFloatType(t *testing.T) {
	for _, src := range []string{`v: "1e3"`, `v: !!str 1e3`} {
		t.Run(src, func(t *testing.T) {
			var named struct{ V ratio }
			require.NoError(t, codec.Unmarshal([]byte(src), &named))
			assert.Equal(t, ratio(1000), named.V)

			var named32 struct{ V ratio32 }
			require.NoError(t, codec.Unmarshal([]byte(src), &named32))
			assert.Equal(t, ratio32(1000), named32.V)

			var plain struct{ V float64 }
			require.NoError(t, codec.Unmarshal([]byte(src), &plain))
			assert.InDelta(t, 1000.0, plain.V, 0)
		})
	}

	// A string ParseFloat does not read is still a type mismatch.
	var named struct{ V ratio }
	require.Error(t, codec.Unmarshal([]byte(`v: "abc"`), &named))
}

// TestASignedInfinityResolves covers the "+" spellings of an infinity, which
// the 1.2 core schema admits and this library read as text.
//
// The float production is `[-+]? ( \.inf | \.Inf | \.INF )`, so "+.inf" names
// the same value ".inf" does. token.reservedInfKeywords listed the six unsigned
// and "-" spellings and left the three "+" ones out, so the scalar never
// resolved and every destination saw a string: an "any" held "+.inf", a
// float64 field took the zero, and ToJSON wrote {"a":"+.inf"} where the same
// value spelled ".inf" is refused outright.
//
// go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1 both read +.inf and +.INF as
// infinity. A NaN is the control: 1.2 spells it `\.nan | \.NaN | \.NAN` with no
// sign, so "+.nan" is a string in all three.
func TestASignedInfinityResolves(t *testing.T) {
	t.Run("into an any", func(t *testing.T) {
		for tc := range aSignedInfinityResolvesCases() {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got["a"], "%q", tc.src)
		}
	})

	t.Run("into a float64", func(t *testing.T) {
		var into struct {
			A float64 `yaml:"a"`
		}
		require.NoError(t, codec.Unmarshal([]byte("a: +.inf\n"), &into))
		assert.Equal(t, math.Inf(1), into.A)
	})

	t.Run("ToJSON refuses it, as it refuses the unsigned spelling", func(t *testing.T) {
		for _, src := range []string{"a: +.inf\n", "a: .inf\n", "a: +.INF\n"} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.ErrorIsf(t, err, yamlerrors.ErrNotJSON, "%q", src)
		}
	})

	t.Run("and it is the same key as the unsigned spelling", func(t *testing.T) {
		// 3.2.1.1 keys on the resolved node, and both spell +Inf.
		var got any
		err := codec.Unmarshal([]byte("+.inf: a\n.inf: b\n"), &got)
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}

// aSignedInfinityResolvesCase holds an infinity spelling and the value it decodes to in an any.
type aSignedInfinityResolvesCase struct {
	src  string
	want any
}

func aSignedInfinityResolvesCases() iter.Seq[aSignedInfinityResolvesCase] {
	return slices.Values([]aSignedInfinityResolvesCase{
		{"a: +.inf\n", math.Inf(1)},
		{"a: +.Inf\n", math.Inf(1)},
		{"a: +.INF\n", math.Inf(1)},
		{"a: .inf\n", math.Inf(1)},
		{"a: -.inf\n", math.Inf(-1)},

		// The control: a NaN admits no sign, so this one stays text.
		{"a: +.nan\n", "+.nan"},
		{"a: -.nan\n", "-.nan"},

		// And a quoted infinity is text whatever it spells.
		{"a: \"+.inf\"\n", "+.inf"},
	})
}

// decodeNumberFormCases covers the number forms of the YAML 1.2 core schema, and the YAML 1.1 forms it reads as strings.
func decodeNumberFormCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		// YAML 1.1 wrote binary integers as "0b...". The 1.2 core schema has
		// no such form, so these resolve to strings until WithYAML11 exists.
		{
			name:   "number/binary-is-string",
			source: "v: 0b10",
			value:  map[string]any{"v": "0b10"},
		},
		{
			name:   "number/negative-binary-is-string",
			source: "v: -0b101010",
			value:  map[string]any{"v": "-0b101010"},
		},
		{
			name:   "number/long-negative-binary-is-string",
			source: "v: -0b1000000000000000000000000000000000000000000000000000000000000000",
			value:  map[string]any{"v": "-0b1000000000000000000000000000000000000000000000000000000000000000"},
		},
		{
			name:   "number/hex",
			source: "v: 0xA",
			value:  map[string]any{"v": 10},
		},
		// The "_" digit separator is YAML 1.1's. The 1.2 core schema has no
		// separator, so a number written with one is a string.
		{
			name:   "number/separator-in-float-is-string",
			source: "v: 685.230_15e+03",
			value:  map[string]any{"v": "685.230_15e+03"},
		},
		{
			name:   "number/separator-in-decimal-float-is-string",
			source: "v: 685_230.15",
			value:  map[string]any{"v": "685_230.15"},
		},
		{
			name:   "number/separator-into-string",
			source: "v: 685_230.15",
			value:  map[string]string{"v": "685_230.15"},
		},
		{
			name:   "number/decimal",
			source: "v: 685230",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/signed-separator-is-string",
			source: "v: +685_230",
			value:  map[string]any{"v": "+685_230"},
		},
		// A leading zero made a number octal in YAML 1.1. In 1.2 the decimal
		// form is "[-+]? [0-9]+", which reads the zero and nothing into it.
		{
			name:   "number/leading-zero-is-decimal",
			source: "v: 02472256",
			value:  map[string]any{"v": 2472256},
		},
		{
			name:   "number/octal",
			source: "v: 0o2472256",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/hex-with-separator-is-string",
			source: "v: 0x_0A_74_AE",
			value:  map[string]any{"v": "0x_0A_74_AE"},
		},
		{
			name:   "number/hex-uppercase",
			source: "v: 0x0A74AE",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/binary-with-separators-is-string",
			source: "v: 0b1010_0111_0100_1010_1110",
			value:  map[string]any{"v": "0b1010_0111_0100_1010_1110"},
		},
		{
			name:   "number/signed-separator-into-string",
			source: "v: +685_230",
			value:  map[string]string{"v": "+685_230"},
		},
	})
}

// decodeIntegerRangeCases covers the bounds of each integer type, and an integer written with an exponent.
func decodeIntegerRangeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "uint/into-uint",
			source: "v: 42",
			value:  map[string]uint{"v": 42},
		},
		{
			name:   "uint/uint64-above-32-bits",
			source: "v: 4294967296",
			value:  map[string]uint64{"v": uint64(4294967296)},
		},
		{
			name:   "int/max-int32",
			source: "v: 2147483647",
			value:  map[string]int{"v": math.MaxInt32},
		},
		{
			name:   "int/min-int32",
			source: "v: -2147483648",
			value:  map[string]int{"v": math.MinInt32},
		},
		{
			name:   "int64/max",
			source: "v: 9223372036854775807",
			value:  map[string]int64{"v": math.MaxInt64},
		},
		// The 1.2 core schema writes a base-16 integer as "0x [0-9a-fA-F]+",
		// with no sign in front of the prefix, so "-0x7F" is a string.
		{
			name:   "int64/max-as-hex",
			source: "v: 0x7FFFFFFFFFFFFFFF",
			value:  map[string]int64{"v": math.MaxInt64},
		},
		{
			name:   "int64/negative-hex-is-string",
			source: "v: -0x7FFFFFFFFFFFFFFF",
			value:  map[string]string{"v": "-0x7FFFFFFFFFFFFFFF"},
		},
		{
			name:   "int64/min",
			source: "v: -9223372036854775808",
			value:  map[string]int64{"v": math.MinInt64},
		},
		{
			name:   "uint/zero",
			source: "v: 0",
			value:  map[string]uint{"v": 0},
		},
		{
			name:   "uint/max-uint32",
			source: "v: 4294967295",
			value:  map[string]uint{"v": math.MaxUint32},
		},
		{
			name:   "uint/exponent",
			source: "v: 1e3",
			value:  map[string]uint{"v": 1000},
		},
		{
			name:   "uint64/zero",
			source: "v: 0",
			value:  map[string]uint64{"v": 0},
		},
		{
			name:   "uint64/max",
			source: "v: 18446744073709551615",
			value:  map[string]uint64{"v": math.MaxUint64},
		},
		{
			name:   "uint64/max-as-hex",
			source: "v: 0xFFFFFFFFFFFFFFFF",
			value:  map[string]uint64{"v": math.MaxUint64},
		},
		{
			name:   "uint64/max-int64",
			source: "v: 9223372036854775807",
			value:  map[string]uint64{"v": math.MaxInt64},
		},
		{
			name:   "uint64/exponent",
			source: "v: 1e3",
			value:  map[string]uint64{"v": 1000},
		},
	})
}

// decodeFloatRangeCases covers the bounds of float32 and float64, and the spellings of infinity and NaN.
func decodeFloatRangeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "float32/max",
			source: "v: 3.40282346638528859811704183484516925440e+38",
			value:  map[string]float32{"v": math.MaxFloat32},
		},
		{
			name:   "float32/smallest-nonzero",
			source: "v: 1.401298464324817070923729583289916131280e-45",
			value:  map[string]float32{"v": math.SmallestNonzeroFloat32},
		},
		{
			name:   "float32/max-uint64",
			source: "v: 18446744073709551615",
			value:  map[string]float32{"v": float32(math.MaxUint64)},
		},
		{
			name:   "float32/above-max-uint64",
			source: "v: 18446744073709551616",
			value:  map[string]float32{"v": float32(math.MaxUint64 + 1)},
		},
		{
			name:   "float32/small-exponent",
			source: "v: 1e-06",
			value:  map[string]float32{"v": 1e-6},
		},
		{
			name:   "float64/max",
			source: "v: 1.797693134862315708145274237317043567981e+308",
			value:  map[string]float64{"v": math.MaxFloat64},
		},
		{
			name:   "float64/smallest-nonzero",
			source: "v: 4.940656458412465441765687928682213723651e-324",
			value:  map[string]float64{"v": math.SmallestNonzeroFloat64},
		},
		{
			name:   "float64/max-uint64",
			source: "v: 18446744073709551615",
			value:  map[string]float64{"v": float64(math.MaxUint64)},
		},
		{
			name:   "float64/above-max-uint64",
			source: "v: 18446744073709551616",
			value:  map[string]float64{"v": float64(math.MaxUint64 + 1)},
		},
		{
			name:   "float64/small-exponent",
			source: "v: 1e-06",
			value:  map[string]float64{"v": 1e-06},
		},
		{
			name:   "inf/lowercase",
			source: "v: .inf\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/capitalized",
			source: "v: .Inf\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/uppercase",
			source: "v: .INF\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/negative-lowercase",
			source: "v: -.inf\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "inf/negative-capitalized",
			source: "v: -.Inf\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "inf/negative-uppercase",
			source: "v: -.INF\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "nan/lowercase",
			source: "v: .nan\n",
			value:  map[string]any{"v": math.NaN()},
		},
		{
			name:   "nan/mixed-case",
			source: "v: .NaN\n",
			value:  map[string]any{"v": math.NaN()},
		},
		{
			name:   "nan/uppercase",
			source: "v: .NAN\n",
			value:  map[string]any{"v": math.NaN()},
		},
	})
}
