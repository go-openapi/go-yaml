// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_Stream(t *testing.T) {
	yml := `
---
a: b
c: d
---
e: f
g: h
---
i: j
k: l
`
	dec := codec.NewDecoder(strings.NewReader(yml))
	values := []map[string]string{}
	for {
		var v map[string]string
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		values = append(values, v)
	}
	require.Len(t, values, 3)
	require.Equal(t, "b", values[0]["a"])
	require.Equal(t, "f", values[1]["e"])
	require.Equal(t, "j", values[2]["i"])
}

func TestDecoder_DecodeFromFile(t *testing.T) {
	yml := `
a: b
c: d
`
	file, err := parser.ParseBytes([]byte(yml))
	require.NoError(t, err)
	var v map[string]string
	require.NoError(t, codec.NewDecoder(file).Decode(&v))
	require.Len(t, v, 2)
	require.Equal(t, "b", v["a"])
	require.Equal(t, "d", v["c"])
}

func TestSetNullValue(t *testing.T) {
	for tc := range setNullValueCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("set null", func(t *testing.T) {
				var v any = 0x1
				require.NoError(t, codec.Unmarshal([]byte(tc.src), &v))
				require.Nil(t, v)
			})
			t.Run("invalid value", func(t *testing.T) {
				var v *struct{}
				require.NoError(t, codec.Unmarshal([]byte(tc.src), v))
				require.Nil(t, v)
			})
		})
	}
}

// setNullValueCase holds a document that decodes to null.
type setNullValueCase struct {
	name string
	src  string
}

func setNullValueCases() iter.Seq[setNullValueCase] {
	return slices.Values([]setNullValueCase{
		{
			name: "empty document",
			src:  "",
		},
		{
			name: "null value",
			src:  "null",
		},
	})
}

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
// built for them goes over to a walk now. Nor is anything built: the scanner
// reads a '&' or a '*' in a directive line as a character of it, so "*AML"
// after "%&AML 1.2" still resolves to nothing.
//
// libfyaml 1.0.0b1 reads the document in every case and the reference parser
// passes them; go.yaml.in/yaml/v3 refuses the directive line outright, which is
// the other defensible answer. Found on 2026-09-07 by a mutation of the "%YAML"
// line Style.Version writes -- a "Y" turned into a "&".

// TestFixedADirectiveNamedLikeAnAnchorIsStillADirective holds the three readers
// to the document the directive stands in front of.
func TestFixedADirectiveNamedLikeAnAnchorIsStillADirective(t *testing.T) {
	for tc := range fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCases() {
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

// fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCase holds a document behind an "&"-named directive and how the walk and ToJSON read it.
type fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCase struct {
	src, reads, writes string
}

func fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCases() iter.Seq[fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCase] {
	return slices.Values([]fixedADirectiveNamedLikeAnAnchorIsStillADirectiveCase{
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
	})
}

// TestFixedADirectiveNamedLikeAnAliasIsIgnored: "%*x y" is a well-formed
// directive named "*x", and 6.8 makes an unknown directive one to ignore.
//
// All three readers refused it with `could not find alias "x"` -- a message
// about a node, for a line that holds none -- because the scanner read the '*'
// as an alias. libfyaml 1.0.0b1 reads the document and the reference parser
// passes it; go.yaml.in/yaml/v3 refuses the directive line. Fred ruled on
// 2026-09-11 to ignore it, as every other reserved directive is ignored here.
func TestFixedADirectiveNamedLikeAnAliasIsIgnored(t *testing.T) {
	for _, src := range []string{"%*x y\n---\n-8.5\n", "%*x\n---\n-8.5\n", "%FOO *x\n---\n-8.5\n"} {
		var got any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "the walk: %q", src)
		assert.InDeltaf(t, -8.5, got, 0, "the walk: %q", src)

		require.NoErrorf(t, codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap()), "the tree: %q", src)
		assert.InDeltaf(t, -8.5, got, 0, "the tree: %q", src)

		out, err := codec.ToJSON([]byte(src))
		require.NoErrorf(t, err, "ToJSON: %q", src)
		assert.Equalf(t, "-8.5", string(out), "ToJSON: %q", src)
	}
}

// decodeStream reads every document of src.
func decodeStream(t *testing.T, src string, opts ...codec.DecodeOption) []any {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewBufferString(src), opts...)

	var out []any
	for range 32 {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out = append(out, v)
	}

	require.FailNowf(t, "the decoder never reached the end of the stream", "%q", src)

	return nil
}

// TestDecodeEmptyDocumentsKeepTheirPlace checks that a document holding nothing
// decodes to null and does not shift the documents after it.
//
// "---" opens a document and "..." closes one, so a document written with
// either marker holds the empty node whatever stands between them. One written
// "---\n...\n" was dropped from the stream: every document after it moved down
// one, and the last was lost.
//
// A run of comments carries no marker and is no document at all --
// l-yaml-stream puts those in a document prefix -- so a stream of them decodes
// to nothing.
func TestDecodeEmptyDocumentsKeepTheirPlace(t *testing.T) {
	for tc := range decodeEmptyDocumentsKeepTheirPlaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeStream(t, tc.src))
		})
	}
}

// decodeEmptyDocumentsKeepTheirPlaceCase holds a stream and the documents it decodes to.
type decodeEmptyDocumentsKeepTheirPlaceCase struct {
	name string
	src  string
	want []any
}

func decodeEmptyDocumentsKeepTheirPlaceCases() iter.Seq[decodeEmptyDocumentsKeepTheirPlaceCase] {
	return slices.Values([]decodeEmptyDocumentsKeepTheirPlaceCase{
		{
			// The spec's own stream example, spec-example-9-6-stream.
			name: "an empty document between two others",
			src:  "Document\n---\n# Empty\n...\n%YAML 1.2\n---\nmatches %: 20\n",
			want: []any{"Document", nil, map[string]any{"matches %": uint64(20)}},
		},
		{name: "two markers with nothing between", src: "a: 1\n---\n---\nb: 2\n", want: []any{map[string]any{"a": uint64(1)}, nil, map[string]any{"b": uint64(2)}}},
		{name: "a header on its own", src: "---\n", want: []any{nil}},
		{name: "a header and a suffix", src: "---\n...\n", want: []any{nil}},
		{name: "an empty document last", src: "a: 1\n---\n...\n", want: []any{map[string]any{"a": uint64(1)}, nil}},
		{name: "a directive over an empty document", src: "%YAML 1.2\n---\n", want: []any{nil}},

		// No marker, so no document.
		{name: "comments alone", src: "# just a comment\n", want: nil},
		{name: "nothing at all", src: "", want: nil},
		// l-yaml-stream admits a run of suffixes; only the first closes
		// anything.
		{name: "two suffixes in a row", src: "a\n...\n...\n", want: []any{"a"}},
		{name: "a suffix opening the stream", src: "...\na: 1\n", want: []any{map[string]any{"a": uint64(1)}}},
	})
}

// decodeDocumentCases covers document markers, line breaks, and documents whose entries end on a null value.
func decodeDocumentCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "document/bare-int",
			source: "123\n",
			value:  123,
		},
		{
			name:   "document/one-entry",
			source: "hello: world\n",
			value:  map[string]string{"hello": "world"},
		},
		{
			name:   "line-break/crlf",
			source: "hello: world\r\n",
			value:  map[string]string{"hello": "world"},
		},
		{
			name:   "line-break/cr",
			source: "hello: world\rGo: Gopher",
			value:  map[string]string{"hello": "world", "Go": "Gopher"},
		},
		{
			name:   "document/spaces-around-key",
			source: "       a       :          b        \n",
			value:  map[string]string{"a": "b"},
		},
		{
			name:   "document/trailing-comment",
			source: "a: b # comment\nb: c\n",
			value: map[string]string{
				"a": "b",
				"b": "c",
			},
		},
		{
			name:   "document/header",
			source: "---\na: b\n",
			value:  map[string]string{"a": "b"},
		},
		{
			name:   "document/end-marker",
			source: "a: b\n...\n",
			value:  map[string]string{"a": "b"},
		},
		// "---" opens a document holding the empty node, which decodes to
		// null rather than ending the stream. go.yaml.in/yaml/v3 and PyYAML
		// both read one document here.
		{
			name:   "document/yaml-directive-only",
			source: "%YAML 1.2\n---\n",
			value:  (*struct{})(nil),
		},
		{
			name:   "document/header-only",
			source: "---\n",
			value:  (*struct{})(nil),
		},
		{
			name:   "document/end-marker-only-is-eof",
			source: "...",
			value:  (*struct{})(nil),
			eof:    true,
		},
		{
			name:   "document/empty-is-eof",
			source: "",
			value:  (*struct{})(nil),
			eof:    true,
		},
		{
			name: "document/sequence-of-mappings-with-null",
			source: `---
- a:
    b:
- c: d
`,
			value: []map[string]any{
				{
					"a": map[string]any{
						"b": nil,
					},
				},
				{
					"c": "d",
				},
			},
		},
		{
			name: "document/null-nested-value",
			source: `---
a:
  b:
c: d
`,
			value: map[string]any{
				"a": map[string]any{
					"b": nil,
				},
				"c": "d",
			},
		},
		{
			name: "document/all-null-values",
			source: `---
a:
b:
c:
`,
			value: map[string]any{
				"a": nil,
				"b": nil,
				"c": nil,
			},
		},
		{
			name: "document/null-values-after-command",
			source: `---
a: go test ./...
b:
c:
`,
			value: map[string]any{
				"a": "go test ./...",
				"b": nil,
				"c": nil,
			},
		},
		{
			name: "document/null-values-after-literal-with-dots",
			source: `---
a: |
  hello
  ...
  world
b:
c:
`,
			value: map[string]any{
				"a": "hello\n...\nworld\n",
				"b": nil,
				"c": nil,
			},
		},
	})
}
