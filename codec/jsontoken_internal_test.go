// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding/json"
	"iter"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// rebuildJSON writes the JSON a run of tokens stands for, putting back the
// separators the token stream leaves out.
func rebuildJSON(toks []JSONToken) []byte {
	var (
		out   []byte
		comma bool
	)
	for _, tk := range toks {
		switch tk.Kind {
		case JSONObjectStart, JSONArrayStart:
			if comma {
				out = append(out, ',')
			}
			if tk.Kind == JSONObjectStart {
				out = append(out, '{')
			} else {
				out = append(out, '[')
			}
			comma = false
		case JSONObjectEnd, JSONArrayEnd:
			if tk.Kind == JSONObjectEnd {
				out = append(out, '}')
			} else {
				out = append(out, ']')
			}
			comma = true
		case JSONKey:
			if comma {
				out = append(out, ',')
			}
			out = appendJSONString(out, tk.Value)
			out = append(out, ':')
			comma = false
		default:
			if comma {
				out = append(out, ',')
			}
			switch tk.Kind {
			case JSONString:
				out = appendJSONString(out, tk.Value)
			case JSONNumber:
				out = append(out, tk.Value...)
			case JSONBool:
				out = strconv.AppendBool(out, tk.Bool)
			default:
				out = append(out, "null"...)
			}
			comma = true
		}
	}

	return out
}

func collectJSONTokens(src []byte, opts ...parser.Option) ([]JSONToken, error) {
	s := ToJSONTokens(src, opts...)
	var toks []JSONToken
	for tk := range s.Tokens() {
		toks = append(toks, tk)
	}

	return toks, s.Err()
}

// TestJSONTokensRebuildWhatToJSONWrites holds ToJSON's text against a rebuild
// of the tokens it is written from, and against JSON itself, over every
// document of the corpus.
//
// ⚠️ It compared two readings of a document until ToJSON became one of them.
// ToJSON walked the document, recorded the text each anchor wrote and answered
// a merge by reading its own output back, where the token converter follows
// ast.AliasNode.Target and asks ast.MergeOf; that walk is gone. What is left to
// compare is the separators -- the commas and colons no token carries --
// written twice, in rebuildJSON here and in appendJSONToken.
//
// json.Valid is the assertion that outlived the two readings. ToJSON used to
// write text that is no JSON document at all for two shapes, and both were
// skipped here rather than asserted: an explicit "? <<" merge key, which wrote
// a key with no value, and an anchor the parse read as a node of its own, which
// wrote two root values. The emitter refuses the second and names the first
// "<<", so every document ToJSON accepts is a JSON document and the skip is
// gone.
func TestJSONTokensRebuildWhatToJSONWrites(t *testing.T) {
	var compared, refused int

	for _, src := range corpusSources() {
		want, wantErr := ToJSON([]byte(src.text))
		toks, gotErr := collectJSONTokens([]byte(src.text))

		if wantErr != nil {
			if assert.Errorf(t, gotErr, "%s: ToJSON refused %q and the tokens did not", src.name, src.text) {
				// One emitter refuses once, so the two carry one message. They
				// refused 97 corpus documents with different messages while
				// each walked the document itself -- "a mapping entry holds one
				// value" against the parse error that followed it.
				assert.Equalf(t, wantErr.Error(), gotErr.Error(), "%s: %q", src.name, src.text)
			}
			refused++

			continue
		}
		if !assert.NoErrorf(t, gotErr, "%s: ToJSON converted %q and the tokens did not", src.name, src.text) {
			continue
		}
		assert.Truef(t, json.Valid(want),
			"%s: ToJSON wrote %q for %q, which is not a JSON document", src.name, want, src.text)

		assert.Equalf(t, string(want), string(rebuildJSON(toks)), "%s: %q", src.name, src.text)
		compared++
	}

	t.Logf("%d documents converted alike, %d refused alike", compared, refused)
}

// TestATagDecidesWhetherAnInfinityConverts holds both converters to one answer
// for an infinity or a NaN under a tag.
//
// JSON has no number for either, so an untagged ".inf" and a "!!float .inf" are
// refused. A tag that makes the scalar a string leaves JSON a spelling for it:
// "k: !!str .inf" is the string ".inf", exactly as "k: \".inf\"" is. A "!!int"
// cannot read ".inf" and is refused as the decoder refuses it, unless
// parser.WithLaxTags keeps the text.
func TestATagDecidesWhetherAnInfinityConverts(t *testing.T) {
	convert := func(t *testing.T, src string, opts ...parser.Option) (string, string) {
		t.Helper()

		out, err := ToJSON([]byte(src), opts...)
		toks, tokErr := collectJSONTokens([]byte(src), opts...)
		if err != nil {
			require.Errorf(t, tokErr, "ToJSON refused %q and the tokens did not", src)

			return err.Error(), tokErr.Error()
		}
		require.NoErrorf(t, tokErr, "ToJSON converted %q and the tokens did not", src)

		return string(out), string(rebuildJSON(toks))
	}

	t.Run("a string tag converts", func(t *testing.T) {
		for tc := range anInfinityUnderAStringTagCases() {
			t.Run(tc.src, func(t *testing.T) {
				fromBytes, fromTokens := convert(t, tc.src)
				assert.Equal(t, tc.want, fromBytes)
				assert.Equal(t, tc.want, fromTokens)
			})
		}
	})

	t.Run("a number is refused", func(t *testing.T) {
		for tc := range anInfinityAsANumberCases() {
			t.Run(tc.src, func(t *testing.T) {
				fromBytes, fromTokens := convert(t, tc.src)
				assert.Contains(t, fromBytes, tc.want)
				assert.Contains(t, fromTokens, tc.want)
			})
		}
	})

	t.Run("and WithLaxTags keeps the text", func(t *testing.T) {
		fromBytes, fromTokens := convert(t, "k: !!int .inf\n", parser.WithLaxTags())
		assert.Equal(t, `{"k":".inf"}`, fromBytes)
		assert.Equal(t, `{"k":".inf"}`, fromTokens)
	})
}

// aTagDecidesWhetherAnInfinityConvertsCase holds a document and the JSON or the error text both converters give for it.
type aTagDecidesWhetherAnInfinityConvertsCase struct {
	src  string
	want string
}

func anInfinityUnderAStringTagCases() iter.Seq[aTagDecidesWhetherAnInfinityConvertsCase] {
	return slices.Values([]aTagDecidesWhetherAnInfinityConvertsCase{
		{src: "!!str .inf: v\n", want: `{".inf":"v"}`},
		{src: "[!!str .nan]\n", want: `[".nan"]`},
		{src: "k: !!str &x .inf\n", want: `{"k":".inf"}`},
		{src: "k: !!str &x .inf\nj: *x\n", want: `{"k":".inf","j":".inf"}`},
		{src: "k: !!str -.inf\n", want: `{"k":"-.inf"}`},
		{src: "k: !!str .inf\n", want: `{"k":".inf"}`},
		{src: "k: !!str .nan\n", want: `{"k":".nan"}`},
		{src: "k: !a .inf\n", want: `{"k":".inf"}`},
		{src: "k: \".inf\"\n", want: `{"k":".inf"}`},
		{src: "k: &x !!str .inf\n", want: `{"k":".inf"}`},
		{src: "{!!str .inf: v}\n", want: `{".inf":"v"}`},
	})
}

func anInfinityAsANumberCases() iter.Seq[aTagDecidesWhetherAnInfinityConvertsCase] {
	return slices.Values([]aTagDecidesWhetherAnInfinityConvertsCase{
		{src: "!!float .inf: v\n", want: "JSON has no number for .inf"},
		{src: "k: !!float &x .nan\n", want: "JSON has no number for .nan"},
		{src: "k: !!float -.inf\n", want: "JSON has no number for -.inf"},
		{src: "k: !!float .inf\n", want: "JSON has no number for .inf"},
		{src: "k: !!int .inf\n", want: `cannot read ".inf" as !!int`},
		{src: "k: &x !!float .nan\n", want: "JSON has no number for .nan"},
		{src: "k: .inf\n", want: "JSON has no number for .inf"},
	})
}

// TestJSONTokensAllocateNothingPerValue holds the token walk to a fixed number
// of allocations on top of its parse, whatever the document's length.
//
// A fresh buffer for every number and a keys slice grown from nothing for every
// mapping made the token walk allocate 166,790 times on golang_source, where
// ToJSON allocates 207. ToJSON is to run on this walk, so an allocation per
// value here is one in every conversion. The parse allocates in proportion to
// the document by itself, so a walk handing its nodes to a visitor that does
// nothing is measured on the same source and taken away.
func TestJSONTokensAllocateNothingPerValue(t *testing.T) {
	overWalk := func(src []byte) float64 {
		tokens := testing.AllocsPerRun(5, func() {
			s := ToJSONTokens(src)
			for range s.Tokens() {
			}
		})
		walk := testing.AllocsPerRun(5, func() {
			_, _ = parser.New(parser.WithOmitNodePaths(), parser.WithJSONCompatible()).Walk(src, discardVisitor{})
		})

		return tokens - walk
	}

	for gen := range jsonTokensAllocateNothingPerValueCases() {
		t.Run(gen.name, func(t *testing.T) {
			small := overWalk([]byte(gen.doc(10)))
			large := overWalk([]byte(gen.doc(2000)))
			t.Logf("allocations over the walk: %.0f at 10, %.0f at 2000", small, large)

			// A few frames and a buffer grow once as the document widens; one per
			// value would add thousands.
			assert.LessOrEqualf(t, large-small, 16.0,
				"the token walk allocates %.0f more at 2000 than at 10", large-small)
		})
	}
}

// jsonTokensAllocateNothingPerValueCase holds a generator of documents of n values.
type jsonTokensAllocateNothingPerValueCase struct {
	name string
	doc  func(int) string
}

func jsonTokensAllocateNothingPerValueCases() iter.Seq[jsonTokensAllocateNothingPerValueCase] {
	return slices.Values([]jsonTokensAllocateNothingPerValueCase{
		{"flat mapping", corpus.FlatMap},
		{"flat sequence", corpus.FlatSequence},
		{"nested mappings", corpus.NestedDoc},
		{"numbers, bools and nulls", scalarSequence},
	})
}

// scalarSequence is a block sequence of n entries cycling through an integer, a
// float, a bool and a null, each spelled as JSON spells it.
//
// The corpus generators write strings, which the token walk takes from the node
// and never writes out, so they leave the scalar path unmeasured. A number JSON
// spells differently -- "0x1F" is 31 -- costs its token a string of its own.
func scalarSequence(n int) string {
	spellings := [...]string{"- 12\n", "- 1.5\n", "- true\n", "- null\n"}

	var b strings.Builder
	for i := range n {
		b.WriteString(spellings[i%len(spellings)])
	}

	return b.String()
}

// discardVisitor is handed a walk's nodes and does nothing with them.
type discardVisitor struct{}

func (discardVisitor) Enter(ast.Node, parser.Cursor) error { return nil }

func (discardVisitor) Leave(ast.Node, parser.Closing) error { return nil }
