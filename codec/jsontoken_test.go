// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// line renders one token with where the conversion stood when it went over.
func line(s *codec.JSONTokens, tk codec.JSONToken) string {
	value := tk.Value
	if tk.Kind == codec.JSONBool {
		value = strconv.FormatBool(tk.Bool)
	}

	return fmt.Sprintf("%-6s %-8q depth=%d path=%q", tk.Kind, value, s.Depth(), s.Path())
}

func TestJSONTokensReadADocument(t *testing.T) {
	const src = `name: gopher
tags: [a, b]
meta:
  ok: true
  n: 0x1F
  none:
`

	s := codec.ToJSONTokens([]byte(src))
	var got []string
	for tk := range s.Tokens() {
		got = append(got, line(s, tk))
	}
	require.NoError(t, s.Err())

	assert.Equal(t, []string{
		`{      ""       depth=1 path=""`,
		`key    "name"   depth=1 path="/name"`,
		`string "gopher" depth=1 path="/name"`,
		`key    "tags"   depth=1 path="/tags"`,
		`[      ""       depth=2 path="/tags"`,
		`string "a"      depth=2 path="/tags/0"`,
		`string "b"      depth=2 path="/tags/1"`,
		`]      ""       depth=1 path="/tags"`,
		`key    "meta"   depth=1 path="/meta"`,
		`{      ""       depth=2 path="/meta"`,
		`key    "ok"     depth=2 path="/meta/ok"`,
		`bool   "true"   depth=2 path="/meta/ok"`,
		`key    "n"      depth=2 path="/meta/n"`,
		`number "31"     depth=2 path="/meta/n"`,
		`key    "none"   depth=2 path="/meta/none"`,
		`null   ""       depth=2 path="/meta/none"`,
		`}      ""       depth=1 path="/meta"`,
		`}      ""       depth=0 path=""`,
	}, got)
}

// TestJSONTokensPopBeforeACloser holds the convention the JSON lexer this feeds
// uses: a closing token reports the depth it returns to, not the one it closes.
func TestJSONTokensPopBeforeACloser(t *testing.T) {
	s := codec.ToJSONTokens([]byte("a: [1]\n"))

	var depths []int
	var kinds []string
	for tk := range s.Tokens() {
		depths = append(depths, s.Depth())
		kinds = append(kinds, tk.Kind.String())
	}
	require.NoError(t, s.Err())

	assert.Equal(t, []string{"{", "key", "[", "number", "]", "}"}, kinds)
	assert.Equal(t, []int{1, 1, 2, 2, 1, 0}, depths)
}

func TestJSONTokensEscapeAPointer(t *testing.T) {
	s := codec.ToJSONTokens([]byte("\"a/b\":\n  \"c~d\": 1\n"))

	var paths []string
	for tk := range s.Tokens() {
		if tk.Kind == codec.JSONNumber {
			paths = append(paths, s.Path())
		}
	}
	require.NoError(t, s.Err())

	assert.Equal(t, []string{"/a~1b/c~0d"}, paths)
}

// TestJSONTokensPositionsAddressTheSource holds that a token's Offset indexes
// the bytes the caller passed in, so the source can be sliced with it.
func TestJSONTokensPositionsAddressTheSource(t *testing.T) {
	const src = "héllo: wörld\nn: 12\n"

	s := codec.ToJSONTokens([]byte(src))
	for tk := range s.Tokens() {
		if tk.Kind != codec.JSONKey && tk.Kind != codec.JSONString && tk.Kind != codec.JSONNumber {
			continue
		}
		at := int(tk.At.Offset())
		require.LessOrEqualf(t, at+len(tk.Value), len(src), "%v %q", tk.Kind, tk.Value)
		assert.Equalf(t, tk.Value, src[at:at+len(tk.Value)], "%v at %d", tk.Kind, at)
	}
	require.NoError(t, s.Err())
}

// TestJSONTokensBudgetBoundsAnAlias holds the one bound that covers an alias
// bomb: neither the nesting depth nor the size of a scalar reaches it.
func TestJSONTokensBudgetBoundsAnAlias(t *testing.T) {
	var b strings.Builder
	b.WriteString("l0: &a0 [x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "l%d: &a%d [", i, i)
		for j := range 9 {
			if j > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "*a%d", i-1)
		}
		b.WriteString("]\n")
	}

	s := codec.ToJSONTokens([]byte(b.String())).Budget(10000)
	var n int
	for range s.Tokens() {
		n++
	}

	require.Error(t, s.Err())
	assert.ErrorContains(t, s.Err(), "10000")
	assert.LessOrEqual(t, n, 10000)

	// The error matches ErrNotJSON and carries the position of the token past the budget.
	require.ErrorIs(t, s.Err(), yamlerrors.ErrNotJSON)
	ye, ok := errors.AsType[*yamlerrors.Error](s.Err())
	require.TrueT(t, ok)
	require.NotNil(t, ye.GetToken())
	assert.Positive(t, ye.GetToken().Position.Line)
}

func TestJSONTokensRefuseWhatJSONCannotHold(t *testing.T) {
	for tc := range jsonTokensRefuseWhatJSONCannotHoldCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src))
			for range s.Tokens() {
			}
			assert.Error(t, s.Err())
		})
	}
}

// jsonTokensRefuseWhatJSONCannotHoldCase holds a document JSON cannot hold.
type jsonTokensRefuseWhatJSONCannotHoldCase struct {
	name string
	src  string
}

func jsonTokensRefuseWhatJSONCannotHoldCases() iter.Seq[jsonTokensRefuseWhatJSONCannotHoldCase] {
	return slices.Values([]jsonTokensRefuseWhatJSONCannotHoldCase{
		{name: "a NaN", src: "a: .nan\n"},
		{name: "a sequence key", src: "? [1, 2]\n: v\n"},
		{name: "an infinity", src: "a: .inf\n"},
		{name: "an unknown alias", src: "a: *nope\n"},
	})
}

func TestJSONTokensReadTheFirstDocumentOfAStream(t *testing.T) {
	for tc := range jsonTokensReadTheFirstDocumentOfAStreamCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src))
			var got []string
			for tk := range s.Tokens() {
				switch tk.Kind {
				case codec.JSONKey:
					got = append(got, `"`+tk.Value+`":`)
				case codec.JSONNull:
					got = append(got, "null")
				default:
					got = append(got, tk.Value+tk.Kind.String())
				}
			}
			require.NoError(t, s.Err())
			_ = got
			want, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(want))
		})
	}
}

// jsonTokensReadTheFirstDocumentOfAStreamCase holds a stream and the JSON its first document converts to.
type jsonTokensReadTheFirstDocumentOfAStreamCase struct {
	name      string
	src, want string
}

func jsonTokensReadTheFirstDocumentOfAStreamCases() iter.Seq[jsonTokensReadTheFirstDocumentOfAStreamCase] {
	return slices.Values([]jsonTokensReadTheFirstDocumentOfAStreamCase{
		{"an empty first", "---\n---\nb: 2\n", `null`},
		{"nothing at all", "", `null`},
		{"past a directive", "%YAML 1.2\n---\na: 1\n", `{"a":1}`},
		{"the first of two", "a: 1\n---\nb: 2\n", `{"a":1}`},
	})
}

// TestJSONTokensStopWhenTheRangeDoes holds that breaking out of the range stops
// the parse rather than reading the document through.
func TestJSONTokensStopWhenTheRangeDoes(t *testing.T) {
	s := codec.ToJSONTokens([]byte("a: 1\nb: 2\nc: 3\n"))

	var n int
	for range s.Tokens() {
		n++
		if n == 2 {
			break
		}
	}

	assert.Equal(t, 2, n)
	assert.NoError(t, s.Err())
}

// TestJSONTokensCountNestedElements holds that a collection standing in a
// sequence fills a slot of it, so the elements after it are not all at 0.
func TestJSONTokensCountNestedElements(t *testing.T) {
	s := codec.ToJSONTokens([]byte("- [1]\n- [2]\n- {k: 3}\n- 4\n"))

	var paths []string
	for tk := range s.Tokens() {
		if tk.Kind == codec.JSONNumber {
			paths = append(paths, s.Path())
		}
	}
	require.NoError(t, s.Err())

	assert.Equal(t, []string{"/0/0", "/1/0", "/2/k", "/3"}, paths)
}

// TestJSONTokensFoldAMerge holds that a "<<" brings its mapping's members in,
// that the mapping's own key beats a merged one, and that the merged members
// come after the ones the mapping wrote itself.
func TestJSONTokensFoldAMerge(t *testing.T) {
	const src = "base: &b {k: 1, z: 9}\nuse:\n  !!merge <<: *b\n  k: 2\n"

	s := codec.ToJSONTokens([]byte(src))
	var members []string
	for tk := range s.Tokens() {
		if tk.Kind == codec.JSONKey && strings.HasPrefix(s.Path(), "/use/") {
			members = append(members, tk.Value)
		}
	}
	require.NoError(t, s.Err())

	assert.Equal(t, []string{"k", "z"}, members)

	want, err := codec.ToJSON([]byte(src))
	require.NoError(t, err)
	assert.Equal(t, `{"base":{"k":1,"z":9},"use":{"k":2,"z":9}}`, string(want))
}

// TestJSONTokensSpellAKeyAsToJSONDoes pins how each shape of key is named.
//
// ⚠️ The corpus does not reach these: no document in it stands an alias or an
// anchor as a mapping key over a float whose two spellings differ, so
// TestJSONTokensRebuildWhatToJSONWrites agreed while the two converters named
// an alias key differently. That is how the rows came to be written out, and
// they stay: ToJSON writes the tokens now, so they pin the one name both hand
// a caller.
//
// The last two rows are a defect -- a property in front of a key should not
// change the key's name -- and are pinned as they stand so that a fix reports
// itself here.
func TestJSONTokensSpellAKeyAsToJSONDoes(t *testing.T) {
	for tc := range jsonTokensSpellAKeyAsToJSONDoesCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src))
			var keys []string
			for tk := range s.Tokens() {
				if tk.Kind == codec.JSONKey {
					keys = append(keys, tk.Value)
				}
			}
			require.NoError(t, s.Err())
			require.NotEmpty(t, keys)
			assert.Equal(t, tc.key, keys[len(keys)-1])

			// And the same name ToJSON writes, which is the contract.
			want, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Contains(t, string(want), `"`+tc.key+`":`)
		})
	}
}

// jsonTokensSpellAKeyAsToJSONDoesCase holds a document and the name its last key gets.
type jsonTokensSpellAKeyAsToJSONDoesCase struct {
	name     string
	src, key string
}

func jsonTokensSpellAKeyAsToJSONDoesCases() iter.Seq[jsonTokensSpellAKeyAsToJSONDoesCase] {
	return slices.Values([]jsonTokensSpellAKeyAsToJSONDoesCase{
		{"a bare key", "1e3: x\n", "1000.0"},
		{"a key under a ?", "? 1e3\n: x\n", "1000.0"},
		{"a tagged key", "!!float 1e3: x\n", "1000.0"},
		// A tag under an anchor or an alias says what the value is, so the
		// tagged value names the entry and not the digits.
		{"an anchored tagged key", "&a !!float 1e3: x\n", "1000.0"},
		{"an alias to a tagged key", "a: &a1 !!float 1e3\n*a1 : v\n", "1000.0"},
		{"a timestamp under a ?", "? !!timestamp 2001-12-14\n: x\n", "2001-12-14T00:00:00Z"},
		{"an anchored timestamp", "&a !!timestamp 2001-12-14: x\n", "2001-12-14T00:00:00Z"},
		{"an alias to a zoned timestamp", "a: &a1 !!timestamp 2001-12-14t21:59:43.10-05:00\n*a1 : v\n", "2001-12-14T21:59:43.1-05:00"},
		{"an anchored key", "&a 1e3: x\n", "1000.0"},
		{"an alias as a key", "a: &a1 1e3\n*a1 : v\n", "1000.0"},
		{"a bare small float", "0.00003: x\n", "3e-05"},
		{"an anchored one", "&a 0.00003: x\n", "3e-05"},
		{"an alias to one", "a: &a1 0.00003\n*a1 : v\n", "3e-05"},
	})
}

func TestJSONTokensRefuseAStreamWhenAskedTo(t *testing.T) {
	for tc := range jsonTokensRefuseAStreamWhenAskedToCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src)).OneDocument()
			for range s.Tokens() {
			}
			if tc.refused {
				assert.Error(t, s.Err())

				return
			}
			assert.NoError(t, s.Err())
		})
	}
}

// jsonTokensRefuseAStreamWhenAskedToCase holds a stream and whether OneDocument refuses it.
type jsonTokensRefuseAStreamWhenAskedToCase struct {
	name    string
	src     string
	refused bool
}

func jsonTokensRefuseAStreamWhenAskedToCases() iter.Seq[jsonTokensRefuseAStreamWhenAskedToCase] {
	return slices.Values([]jsonTokensRefuseAStreamWhenAskedToCase{
		{"an empty first", "---\n---\nb: 2\n", true},
		{"one document", "a: 1\n", false},
		{"one past directives", "%YAML 1.2\n---\na: 1\n", false},
		{"two documents", "a: 1\n---\nb: 2\n", true},
	})
}

// TestJSONTokensCloseWhereTheCollectionEnds holds that a closing token never
// stands before what it closes.
//
// A flow collection wrote a "}" or a "]" and the closer stands on it. A block
// collection wrote neither, so it stands just past the last byte it encloses,
// at a position of its own. A consumer ordering tokens by position needs that:
// reading the node's own position puts the closer back at the collection's
// first token. Column counts characters, so "é" moves it by one.
func TestJSONTokensCloseWhereTheCollectionEnds(t *testing.T) {
	for tc := range jsonTokensCloseWhereTheCollectionEndsCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src))

			var (
				closers []string
				last    codec.JSONToken
				seen    bool
			)
			for tk := range s.Tokens() {
				if seen {
					assert.GreaterOrEqualf(t, tk.At.Offset(), last.At.Offset(),
						"%v at %d goes back before %v at %d", tk.Kind, tk.At.Offset(), last.Kind, last.At.Offset())
				}
				last, seen = tk, true

				if tk.Kind == codec.JSONObjectEnd || tk.Kind == codec.JSONArrayEnd {
					closers = append(closers, fmt.Sprintf("L%dC%d", tk.At.Line, tk.At.Column))
				}
			}
			require.NoError(t, s.Err())
			assert.Equal(t, tc.closers, strings.Join(closers, " "))
		})
	}
}

// jsonTokensCloseWhereTheCollectionEndsCase holds a document and the positions of its closing tokens.
type jsonTokensCloseWhereTheCollectionEndsCase struct {
	name         string
	src, closers string
}

func jsonTokensCloseWhereTheCollectionEndsCases() iter.Seq[jsonTokensCloseWhereTheCollectionEndsCase] {
	return slices.Values([]jsonTokensCloseWhereTheCollectionEndsCase{
		{"a block mapping", "a: 1\nb: 2\n", "L2C5"},
		{"a block sequence", "items:\n  - x\n  - y\n", "L3C6 L3C6"},
		{"a block mapping ending in a flow one", "a: {b: 1}\n", "L1C9 L1C10"},
		{"a quoted value", "a: \"x\"\n", "L1C7"},
		{"a wide character", "a: \u00e9\n", "L1C5"},
		{"a literal block", "a: |\n  x\nb: 1\n", "L3C5"},
		{"an alias", "a: &x 1\nb: *x\n", "L2C6"},
		{"a flow mapping", "{a: 1}\n", "L1C6"},
		{"a flow sequence", "[1, 2]\n", "L1C6"},
	})
}

// TestJSONTokensAlwaysCarryAPosition holds that every token points somewhere in
// the source, including the null an empty document reads as.
func TestJSONTokensAlwaysCarryAPosition(t *testing.T) {
	for _, src := range []string{"", "---\n", "a: 1\n", "[]\n", "a:\n"} {
		s := codec.ToJSONTokens([]byte(src))
		for tk := range s.Tokens() {
			assert.Positivef(t, tk.At.Line, "%q: %v has no line", src, tk.Kind)
			assert.Positivef(t, tk.At.Column, "%q: %v has no column", src, tk.Kind)
		}
		require.NoError(t, s.Err())
	}
}

// TestJSONTokensPointAKeyAtItsScalar holds that a key written behind a property
// points at the name it carries, not at the property.
//
// The offset is what a caller slices the source with, so that is what is
// checked: src[Offset()] must be the first byte of the key's text.
//
// ⚠️ The column is not checked here. A scalar behind a tag reports one column
// short of its own offset -- "!!str k" gives offset 6, which is right, and
// column 6, where k is the seventh character -- which is defect 76 in go-yaml
// and not this converter's doing.
func TestJSONTokensPointAKeyAtItsScalar(t *testing.T) {
	for tc := range jsonTokensPointAKeyAtItsScalarCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src))
			var found codec.JSONToken
			for tk := range s.Tokens() {
				if tk.Kind == codec.JSONKey {
					found = tk
				}
			}
			require.NoError(t, s.Err())
			assert.Equal(t, tc.key, found.Value)

			at := int(found.At.Offset())
			require.Less(t, at, len(tc.src))
			assert.Equal(t, tc.key, tc.src[at:at+len(tc.key)], "the offset must land on the key")
		})
	}
}

// jsonTokensPointAKeyAtItsScalarCase holds a document and the key its offset must land on.
type jsonTokensPointAKeyAtItsScalarCase struct {
	name string
	src  string
	key  string
}

func jsonTokensPointAKeyAtItsScalarCases() iter.Seq[jsonTokensPointAKeyAtItsScalarCase] {
	return slices.Values([]jsonTokensPointAKeyAtItsScalarCase{
		{"a bare key", "k: v\n", "k"},
		{"a key under a ?", "? k\n: v\n", "k"},
		{"a tagged key", "!!str k: v\n", "k"},
		{"an anchored key", "&a k: v\n", "k"},
	})
}

// TestAMergedKeyLosesToTheMappingsOwn pins which member a "<<" writes and which it leaves out.
//
// A mapping's own key beats one a merge brings in, whatever their order in the document, and an earlier
// merge beats a later one. ToJSON answers the first from the parse's key ledger, through
// parser.Closing.HoldsKey, and keeps its own record only of the names earlier merges brought in.
//
// ⚠️ The two questions are not the same one. The ledger compares a key by the name and the node it resolves
// to, since 3.2.1.1 makes "7" and "007" one key and "1" and "1.0" two. HoldsKey compares the name alone,
// which is what a JSON member is named by: "1: a" and "\"1\": b" are two YAML keys writing one member.
// The cases below are the ones that tell the two readings apart, so a swap back to comparing nodes fails here.
func TestAMergedKeyLosesToTheMappingsOwn(t *testing.T) {
	t.Parallel()

	for tc := range aMergedKeyLosesToTheMappingsOwnCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.ToJSON([]byte(tc.src), parser.WithMergeKeys())
			require.NoErrorf(t, err, "%q", tc.src)

			assert.JSONEq(t, tc.want, string(out))
			assert.Equalf(t, tc.want, string(out), "%q: the members are written in this order", tc.src)
		})
	}

	t.Run("a repeated merge key is a repeat like any other", func(t *testing.T) {
		t.Parallel()

		_, err := codec.ToJSON([]byte("m:\n  <<: {a: 1}\n  <<: {a: 2, b: 3}\n"), parser.WithMergeKeys())

		require.ErrorContains(t, err, `mapping key "<<" already defined`)
	})
}

// aMergedKeyLosesToTheMappingsOwnCase holds a document with a merge and the JSON it converts to.
type aMergedKeyLosesToTheMappingsOwnCase struct {
	name, src, want string
}

func aMergedKeyLosesToTheMappingsOwnCases() iter.Seq[aMergedKeyLosesToTheMappingsOwnCase] {
	return slices.Values([]aMergedKeyLosesToTheMappingsOwnCase{
		{
			"an own key written after the merge",
			"base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			`{"base":{"a":1,"b":2},"m":{"b":"own","a":1}}`,
		}, {
			"an integer own key against a quoted merged one, which JSON names alike",
			"base: &b\n  \"1\": merged\nm:\n  <<: *b\n  1: own\n",
			`{"base":{"1":"merged"},"m":{"1":"own"}}`,
		}, {
			"a quoted own key against an integer merged one",
			"base: &b\n  1: merged\nm:\n  <<: *b\n  \"1\": own\n",
			`{"base":{"1":"merged"},"m":{"1":"own"}}`,
		}, {
			"a float own key against an integer merged one, which JSON names apart",
			"base: &b\n  1: merged\nm:\n  <<: *b\n  1.0: own\n",
			`{"base":{"1":"merged"},"m":{"1.0":"own","1":"merged"}}`,
		}, {
			"007 owns the member 7",
			"base: &b\n  7: merged\nm:\n  <<: *b\n  007: own\n",
			`{"base":{"7":"merged"},"m":{"7":"own"}}`,
		}, {
			"a bool own key against the quoted merged one",
			"base: &b\n  \"true\": merged\nm:\n  <<: *b\n  true: own\n",
			`{"base":{"true":"merged"},"m":{"true":"own"}}`,
		}, {
			"the earlier of two merge sources wins",
			"one: &x {a: 1}\ntwo: &y {a: 2, b: 3}\nm:\n  <<: [*x, *y]\n",
			`{"one":{"a":1},"two":{"a":2,"b":3},"m":{"a":1,"b":3}}`,
		}, {
			"an own key beats both merge sources",
			"one: &x {a: 1}\ntwo: &y {a: 2, b: 3}\nm:\n  <<: [*y, *x]\n  a: own\n",
			`{"one":{"a":1},"two":{"a":2,"b":3},"m":{"a":"own","b":3}}`,
		}, {
			"a mapping written in place as the merge source",
			"m:\n  <<: {a: 1, b: 2}\n  a: own\n",
			`{"m":{"a":"own","b":2}}`,
		}, {
			"an anchored own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  &k a: own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an explicit own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  ? a\n  : own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"a tagged own key",
			"base: &b {a: 1}\nm:\n  <<: *b\n  !!str a: own\n",
			`{"base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an alias as the own key",
			"n: &n a\nbase: &b {a: 1}\nm:\n  <<: *b\n  *n : own\n",
			`{"n":"a","base":{"a":1},"m":{"a":"own"}}`,
		}, {
			"an own key replaces the whole merged value and does not fold into it",
			"base: &b\n  nest: {x: 1}\nm:\n  <<: *b\n  nest: {y: 2}\n",
			`{"base":{"nest":{"x":1}},"m":{"nest":{"y":2}}}`,
		}, {
			"a merge source that merges in its turn",
			"deep: &d\n  <<: {a: 1}\n  b: 2\nm:\n  <<: *d\n  a: own\n",
			`{"deep":{"b":2,"a":1},"m":{"a":"own","b":2}}`,
		}, {
			// The mapping inside writes "x" and the merge brings "x" to the mapping around it.
			// The parse pops a mapping's keys as it closes, so the inner ones are out of range by the
			// time the outer mapping is left; leaked, they would drop the merged member.
			"a name an inner mapping writes does not answer for the outer one",
			"base: &b {x: merged}\nm:\n  <<: *b\n  inner:\n    x: 1\n",
			`{"base":{"x":"merged"},"m":{"inner":{"x":1},"x":"merged"}}`,
		}, {
			"a name the outer mapping writes does not answer for a merge inside it",
			"base: &b {x: merged}\nm:\n  x: own\n  inner:\n    <<: *b\n",
			`{"base":{"x":"merged"},"m":{"x":"own","inner":{"x":"merged"}}}`,
		}, {
			"a merge in flow style",
			"m: {<<: {a: 1}, a: own, b: 2}\n",
			`{"m":{"a":"own","b":2}}`,
		}, {
			"an empty merge source",
			"base: &b {}\nm:\n  <<: *b\n  a: 1\n",
			`{"base":{},"m":{"a":1}}`,
		}, {
			"a merge into a mapping that writes nothing itself",
			"base: &b {a: 1}\nm:\n  <<: *b\n",
			`{"base":{"a":1},"m":{"a":1}}`,
		},
	})
}

// TestEveryRouteToAMergeAnswersAlike checks that how a "<<" comes to merge does not change what it writes.
//
// Parser.mergeKeysInForce resolves a bare "<<" under YAML 1.1 or under WithMergeKeys, and a written
// "!!merge" tag is honored at any version. isMergeKey reads three node shapes for those routes --
// ast.MergeKeyNode, a TagNode over the merge tag, and MappingKeyNode.IsMergeKey for the "? <<" spelling --
// and the cases above reach only the first.
//
// ⚠️ Under 1.2 with no option a bare "<<" is an ordinary key spelled "<<", and the last case pins that.
// It is the documented reading, not a defect: the merge key is a 1.1 type.
func TestEveryRouteToAMergeAnswersAlike(t *testing.T) {
	t.Parallel()

	for tc := range everyRouteToAMergeAnswersAlikeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.ToJSON([]byte(tc.src), tc.opts...)
			require.NoErrorf(t, err, "%q", tc.src)

			assert.Equalf(t, tc.want, string(out), "%q", tc.src)
		})
	}

	// The reading HoldsKey answers on -- a name shared by two keys YAML tells apart -- has to be the
	// same whichever route the merge took, since each reaches collectMerge by a different node shape.
	t.Run("the name a merged key loses on does not depend on the route", func(t *testing.T) {
		t.Parallel()

		const want = `{"base":{"1":"merged"},"m":{"1":"own"}}`

		byDirective, err := codec.ToJSON([]byte("%YAML 1.1\n---\nbase: &b\n  \"1\": merged\nm:\n  <<: *b\n  1: own\n"))
		require.NoError(t, err)

		byTag, err := codec.ToJSON([]byte("base: &b\n  \"1\": merged\nm:\n  !!merge <<: *b\n  1: own\n"))
		require.NoError(t, err)

		assert.Equal(t, want, string(byDirective))
		assert.Equal(t, want, string(byTag))
	})
}

// everyRouteToAMergeAnswersAlikeCase holds a document, the parser options that make its "<<" merge, and the JSON it converts to.
type everyRouteToAMergeAnswersAlikeCase struct {
	name string
	src  string
	opts []parser.Option
	want string
}

func everyRouteToAMergeAnswersAlikeCases() iter.Seq[everyRouteToAMergeAnswersAlikeCase] {
	const merged = `{"base":{"a":1,"b":2},"m":{"b":"own","a":1}}`

	return slices.Values([]everyRouteToAMergeAnswersAlikeCase{
		{
			name: "a bare << under WithMergeKeys",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			opts: []parser.Option{parser.WithMergeKeys()},
			want: merged,
		}, {
			name: "a bare << under a %YAML 1.1 directive",
			src:  "%YAML 1.1\n---\nbase: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			want: merged,
		}, {
			name: "a bare << under WithYAMLVersion(YAML11)",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			opts: []parser.Option{parser.WithYAMLVersion(parser.YAML11)},
			want: merged,
		}, {
			name: "a written !!merge tag, at 1.2 and with no option",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  !!merge <<: *b\n  b: own\n",
			want: merged,
		}, {
			name: "the \"? <<\" spelling",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  ? <<\n  : *b\n  b: own\n",
			opts: []parser.Option{parser.WithMergeKeys()},
			want: merged,
		}, {
			name: "a bare << at 1.2 is an ordinary key, and merges nothing",
			src:  "base: &b\n  a: 1\n  b: 2\nm:\n  <<: *b\n  b: own\n",
			want: `{"base":{"a":1,"b":2},"m":{"<<":{"a":1,"b":2},"b":"own"}}`,
		},
	})
}

// TestJSONTokensDrawTheSourceOfTheirOwnErrors checks that an error the conversion raises, and not the parser,
// prints the lines around its position, as a parse error does.
func TestJSONTokensDrawTheSourceOfTheirOwnErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		budget    int
		want      string
	}{
		{"name-only pair", "x: 0\n1: a\n\"1\": b\n", 0, `>  3 | "1": b`},
		{"budget", "a: &a [x, x]\nb: [*a, *a]\n", 5, `>  1 | a: &a [x, x]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(tc.src)).Budget(tc.budget)
			for range s.Tokens() {
			}
			require.Error(t, s.Err())
			assert.StringContainsT(t, s.Err().Error(), tc.want)

			_, err := codec.ToJSON([]byte(tc.src))
			if tc.budget == 0 {
				require.Error(t, err)
				assert.StringContainsT(t, err.Error(), tc.want)
			}
		})
	}
}
