// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
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
}

func TestJSONTokensRefuseWhatJSONCannotHold(t *testing.T) {
	for name, src := range map[string]string{
		"an infinity":      "a: .inf\n",
		"a NaN":            "a: .nan\n",
		"a sequence key":   "? [1, 2]\n: v\n",
		"an unknown alias": "a: *nope\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := codec.ToJSONTokens([]byte(src))
			for range s.Tokens() {
			}
			assert.Error(t, s.Err())
		})
	}
}

func TestJSONTokensReadTheFirstDocumentOfAStream(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"the first of two": {"a: 1\n---\nb: 2\n", `{"a":1}`},
		"an empty first":   {"---\n---\nb: 2\n", `null`},
		"past a directive": {"%YAML 1.2\n---\na: 1\n", `{"a":1}`},
		"nothing at all":   {"", `null`},
	} {
		t.Run(name, func(t *testing.T) {
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
	for name, tc := range map[string]struct{ src, key string }{
		"a bare key":      {"1e3: x\n", "1000.0"},
		"a key under a ?": {"? 1e3\n: x\n", "1000.0"},
		"a tagged key":    {"!!float 1e3: x\n", "1000.0"},
		// A tag under an anchor or an alias says what the value is, so the
		// tagged value names the entry and not the digits.
		"an anchored tagged key":        {"&a !!float 1e3: x\n", "1000.0"},
		"an alias to a tagged key":      {"a: &a1 !!float 1e3\n*a1 : v\n", "1000.0"},
		"a timestamp under a ?":         {"? !!timestamp 2001-12-14\n: x\n", "2001-12-14T00:00:00Z"},
		"an anchored timestamp":         {"&a !!timestamp 2001-12-14: x\n", "2001-12-14T00:00:00Z"},
		"an alias to a zoned timestamp": {"a: &a1 !!timestamp 2001-12-14t21:59:43.10-05:00\n*a1 : v\n", "2001-12-14T21:59:43.1-05:00"},
		"an anchored key":               {"&a 1e3: x\n", "1e3"},
		"an alias as a key":             {"a: &a1 1e3\n*a1 : v\n", "1e3"},
		"a bare small float":            {"0.00003: x\n", "3e-05"},
		"an anchored one":               {"&a 0.00003: x\n", "0.00003"},
	} {
		t.Run(name, func(t *testing.T) {
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

func TestJSONTokensRefuseAStreamWhenAskedTo(t *testing.T) {
	for name, tc := range map[string]struct {
		src     string
		refused bool
	}{
		"one document":        {"a: 1\n", false},
		"one past directives": {"%YAML 1.2\n---\na: 1\n", false},
		"two documents":       {"a: 1\n---\nb: 2\n", true},
		"an empty first":      {"---\n---\nb: 2\n", true},
	} {
		t.Run(name, func(t *testing.T) {
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

// TestJSONTokensCloseWhereTheCollectionEnds holds that a closing token never
// stands before what it closes.
//
// A flow collection wrote a "}" or a "]" and the closer stands on it. A block
// collection wrote neither, so it stands at the last token it encloses. A
// consumer ordering tokens by position needs that: reading the node's own
// position puts the closer back at the collection's first token.
func TestJSONTokensCloseWhereTheCollectionEnds(t *testing.T) {
	for name, tc := range map[string]struct{ src, closers string }{
		"a block mapping":  {"a: 1\nb: 2\n", "L2C4"},
		"a block sequence": {"items:\n  - x\n  - y\n", "L3C5 L3C5"},
		"a flow mapping":   {"{a: 1}\n", "L1C6"},
		"a flow sequence":  {"[1, 2]\n", "L1C6"},
	} {
		t.Run(name, func(t *testing.T) {
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
	for name, tc := range map[string]struct {
		src string
		key string
	}{
		"a bare key":      {"k: v\n", "k"},
		"an anchored key": {"&a k: v\n", "k"},
		"a tagged key":    {"!!str k: v\n", "k"},
		"a key under a ?": {"? k\n: v\n", "k"},
	} {
		t.Run(name, func(t *testing.T) {
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
