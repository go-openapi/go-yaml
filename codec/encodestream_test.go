// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// encodeBothWays encodes v once through the stream and once through the tree. streamed reports whether the stream
// wrote the document; false means it fell back, and Encode then writes the tree.
func encodeBothWays(t *testing.T, v any, opts ...EncodeOption) (fromStream, fromTree string, streamed bool, err error) {
	t.Helper()

	var tree bytes.Buffer
	te := NewEncoder(&tree, opts...)
	te.treeOnly = true
	treeErr := te.Encode(v)

	var out bytes.Buffer
	se := NewEncoder(&out, opts...)
	require.NoError(t, se.prepare())
	if !se.streams() {
		return "", tree.String(), false, treeErr
	}
	r := ast.NewRenderer(ast.WithIndent(se.indentNum), ast.WithIndentSequence(se.indentSequence))
	streamErr := se.encodeStream(r.Stream(&out), v)
	if errors.Is(streamErr, errStreamFallback) {
		return "", tree.String(), false, treeErr
	}
	if (streamErr == nil) != (treeErr == nil) {
		t.Errorf("the stream returned %v and the tree %v for %#v", streamErr, treeErr, v)
	}

	return out.String(), tree.String(), true, treeErr
}

// streamOptions are the option sets the stream is compared under. Flow and JSON never stream, and are left out.
func streamOptions() map[string][]EncodeOption {
	return map[string][]EncodeOption{
		"default":    nil,
		"indent4":    {Indent(4)},
		"indentseq":  {IndentSequence(true)},
		"literal":    {UseLiteralStyleIfMultiline(true)},
		"singlequot": {UseSingleQuote(true)},
		"omitempty":  {OmitEmpty()},
		"omitzero":   {OmitZero(), AutoInt()},
	}
}

// TestAStreamedEncodeWritesWhatTheTreeWrites decodes every YAML Test Suite document and fuzz seed into an any and
// encodes it through the stream and through the tree, under each option set, and requires the same bytes. It also
// checks that the stream wrote most of them instead of falling back.
func TestAStreamedEncodeWritesWhatTheTreeWrites(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	srcs := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, string(s.InYAML))
	}
	srcs = append(srcs, seeds...)

	var values []any
	for _, src := range srcs {
		var v any
		if err := Unmarshal([]byte(src), &v); err != nil || v == nil {
			continue
		}
		values = append(values, v)
	}
	require.Greater(t, len(values), 1000)

	for name, opts := range streamOptions() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var streamed, fellBack, differed int
			for _, v := range values {
				fromStream, fromTree, ok, err := encodeBothWays(t, v, opts...)
				if err != nil {
					continue
				}
				if !ok {
					fellBack++

					continue
				}
				streamed++
				if fromStream != fromTree {
					differed++
					if differed <= 3 {
						t.Errorf("the stream wrote\n%s\nthe tree wrote\n%s\nfor %#v", fromStream, fromTree, v)
					}
				}
			}
			t.Logf("%d values streamed, %d fell back to the tree", streamed, fellBack)
			assert.Zero(t, differed)
			assert.Greater(t, streamed, 10*fellBack)
		})
	}
}

type streamInner struct {
	A int    `yaml:"a"`
	B string `yaml:"b,omitempty"`
}

type streamMarshaler struct{ n int }

func (m streamMarshaler) MarshalYAML() ([]byte, error) {
	return fmt.Appendf(nil, "x: %d\n\ny: [%d, %d]\n", m.n, m.n, m.n+1), nil
}

type streamText struct{ s string }

func (m streamText) MarshalText() ([]byte, error) { return []byte("text-" + m.s), nil }

// TestAStreamedEncodeTakesEveryFieldShape encodes values built for each case streamStruct and streamMap treat apart:
// anchors and aliases, inline structs and maps, an inline anchor, flow fields, the omit tags, the MapSlice family,
// times, Base64, marshalers, keys that are collections or tagged, and empty and nil values. Each must write what the
// tree writes, under each option set. Some fall back by design; the test names them.
func TestAStreamedEncodeTakesEveryFieldShape(t *testing.T) {
	t.Parallel()

	shared := &streamInner{A: 7}
	type anchored struct {
		First  *streamInner `yaml:"first,anchor=x"`
		Second *streamInner `yaml:"second,alias=x"`
		Third  *streamInner `yaml:"third"`
	}
	type autoAnchored struct {
		First  *streamInner `yaml:"first,anchor"`
		Second *streamInner `yaml:"second"`
	}
	type inlined struct {
		streamInner `yaml:",inline"`

		C int `yaml:"c"`
	}
	type inlineMap struct {
		C    int            `yaml:"c"`
		Rest map[string]any `yaml:",inline"`
	}
	type inlineAnchor struct {
		*streamInner `yaml:",inline,anchor"`

		C int `yaml:"c"`
	}
	type flowed struct {
		List []int          `yaml:"list,flow"`
		Map  map[string]int `yaml:"map,flow"`
		Deep streamInner    `yaml:"deep"`
	}
	type omitted struct {
		Empty  string    `yaml:"empty,omitempty"`
		Zero   time.Time `yaml:"zero,omitzero"`
		Nil    *int      `yaml:"nil"`
		Slice  []string  `yaml:"slice"`
		Map    map[string]int
		Kept   int `yaml:"kept"`
		hidden int
	}
	type scalars struct {
		When     time.Time     `yaml:"when"`
		Took     time.Duration `yaml:"took"`
		Blob     Base64        `yaml:"blob"`
		Self     streamMarshaler
		Text     streamText
		Lines    string  `yaml:"lines"`
		NaN      float64 `yaml:"nan"`
		Inf      float64 `yaml:"inf"`
		Big      uint64  `yaml:"big"`
		Quoted   string  `yaml:"a:b"`
		LongName string  `yaml:"long"`
	}

	when := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	cases := map[string]any{
		"anchor and alias": anchored{First: shared, Second: shared, Third: shared},
		"auto anchor":      autoAnchored{First: shared, Second: shared},
		"inline struct":    inlined{streamInner: streamInner{A: 1, B: "b"}, C: 2},
		"inline map":       inlineMap{C: 1, Rest: map[string]any{"z": 1, "c": 9, "y": []any{1, "two"}}},
		"inline anchor":    inlineAnchor{streamInner: &streamInner{A: 1}, C: 2},
		"flow fields":      flowed{List: []int{1, 2}, Map: map[string]int{"a": 1}, Deep: streamInner{A: 3}},
		"omit tags":        omitted{Kept: 1, hidden: 2},
		"scalars": scalars{
			When: when, Took: time.Second, Blob: Base64("YWJj"), Self: streamMarshaler{1}, Text: streamText{"t"},
			Lines: "a\nb\n", NaN: math.NaN(), Inf: math.Inf(-1), Big: math.MaxUint64, Quoted: "yes",
			LongName: strings.Repeat("k", 1100),
		},
		"MapSlice":          MapSlice{items: []MapItem{{Key: "b", Value: 1}, {Key: "a", Value: []any{MapSlice{items: []MapItem{{Key: "x", Value: nil}}}}}}},
		"MapSliceSeq":       map[string]any{"o": MapSliceSeq{items: []MapItem{{Key: "k", Value: 1}}}},
		"time keys":         map[any]any{when: 1, "s": map[string]any{"t": when}},
		"collection key":    map[any]any{[2]int{1, 2}: "pair", "s": 1},
		"marshaler key":     map[streamText]int{{"a"}: 1, {"b"}: 2},
		"long key":          map[string]any{strings.Repeat("k", 1100): map[string]int{"a": 1}},
		"empty collections": map[string]any{"m": map[string]any{}, "s": []any{}, "n": nil, "z": []map[string]any{{}}},
		"nested sequences":  []any{[]any{[]any{1}, 2}, map[string]any{"a": []any{}}},
		"pointer chain":     &map[string]*[]*streamInner{"p": {{A: 1}, nil}},
		"marshaler entry":   []any{streamMarshaler{2}, map[string]any{"m": streamMarshaler{3}}},
		"a lone scalar":     "x",
		"a lone null":       nil,
	}
	wantFallback := map[string]bool{"collection key": true}

	for name, opts := range streamOptions() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for caseName, v := range cases {
				fromStream, fromTree, streamed, err := encodeBothWays(t, v, opts...)
				require.NoErrorf(t, err, "%s", caseName)
				if wantFallback[caseName] {
					assert.Falsef(t, streamed, "%s should fall back to the tree", caseName)

					continue
				}
				require.Truef(t, streamed, "%s fell back to the tree", caseName)
				assert.Equalf(t, fromTree, fromStream, "%s", caseName)

				_, err = parser.ParseBytes([]byte(fromStream))
				assert.NoErrorf(t, err, "%s: the stream wrote YAML that does not parse:\n%s", caseName, fromStream)
			}
		})
	}
}

// TestEncodeFallsBackForAHeldNode encodes a parsed node carrying comments: the stream drops comments, so Encode
// builds the tree, and the comments survive.
func TestEncodeFallsBackForAHeldNode(t *testing.T) {
	file, err := parser.ParseBytes([]byte("# above\na: 1 # beside\n"), parser.WithComments())
	require.NoError(t, err)

	_, fromTree, streamed, err := encodeBothWays(t, map[string]any{"k": file.Docs[0].Body})
	require.NoError(t, err)
	assert.False(t, streamed)

	out, err := Marshal(map[string]any{"k": file.Docs[0].Body})
	require.NoError(t, err)
	assert.Equal(t, fromTree, string(out))
	assert.Contains(t, string(out), "# beside")
}
