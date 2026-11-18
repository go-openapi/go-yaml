// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_AnchorReferenceDirs(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(buf, codec.ReferenceDirs("../testdata/codec"))
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

func TestDecoder_AnchorReferenceDirsRecursive(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(
		buf,
		codec.ReferenceDirs("../testdata/codec"),
	)
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

func TestDecoder_AnchorFiles(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(buf, codec.ReferenceFiles("../testdata/codec/anchor.yml"))
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

func TestDecoder_DecodeWithAnchorAnyValue(t *testing.T) {
	type Config struct {
		Env []string `json:"env"`
	}

	type Schema struct {
		Def    map[string]any `json:"def"`
		Config Config         `json:"config"`
	}

	data := `
def:
  myenv: &my_env
    - VAR1=1
    - VAR2=2
config:
  env: *my_env
`

	var cfg Schema
	if err := codec.NewDecoder(strings.NewReader(data)).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Config.Env, []string{"VAR1=1", "VAR2=2"}) {
		t.Fatalf("failed to decode value. actual = %+v", cfg)
	}
}

func TestRoundtripAnchorAlias(t *testing.T) {
	t.Run("irreversible", func(t *testing.T) {
		type foo struct {
			K1 string
			K2 string
		}

		type bar struct {
			K1 string
			K3 string
		}

		type doc struct {
			Foo foo
			Bar bar
		}
		yml := `%YAML 1.1
---
foo:
 <<: &test-anchor
   k1: "One"
 k2: "Two"

bar:
 <<: *test-anchor
 k3: "Three"
`
		var v doc
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		bytes, err := codec.Marshal(v)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		expected := `
foo:
  k1: One
  k2: Two
bar:
  k1: One
  k3: Three
`
		got := "\n" + string(bytes)
		if expected != got {
			t.Fatalf("expected:[%s] but got [%s]", expected, got)
		}
	})
	t.Run("reversible", func(t *testing.T) {
		type TestAnchor struct {
			K1 string
		}
		type foo struct {
			*TestAnchor `yaml:",inline,alias"`

			K2 string
		}
		type bar struct {
			*TestAnchor `yaml:",inline,alias"`

			K3 string
		}
		type doc struct {
			TestAnchor *TestAnchor `yaml:"test-anchor,anchor"`
			Foo        foo
			Bar        bar
		}
		// The body is compared on its own: the document is read under YAML 1.1,
		// where "<<" is the merge key, and the encoder writes no directive, so
		// the round trip is of the body and not of the header that selects the
		// version.
		body := `
test-anchor: &test-anchor
  k1: One
foo:
  <<: *test-anchor
  k2: Two
bar:
  <<: *test-anchor
  k3: Three
`
		var v doc
		if err := codec.Unmarshal([]byte("%YAML 1.1\n---"+body), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		bytes, err := codec.Marshal(v)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		got := "\n" + string(bytes)
		if body != got {
			t.Fatalf("expected:[%s] but got [%s]", body, got)
		}
	})
}

func TestDecodeWithSameAnchor(t *testing.T) {
	yml := `
a: &a 1
b: &a 2
c: &a 3
d: *a
`
	type T struct {
		A int
		B int
		C int
		D int
	}
	var v T
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, T{A: 1, B: 2, C: 3, D: 3}) {
		t.Fatalf("failed to decode same anchor: %+v", v)
	}
}

func TestDecoder_AnchorInUnmappedField(t *testing.T) {
	// Test that anchors defined in fields not mapped to struct
	// can be referenced using aliases (regression test for v1.16.0)
	type Config struct {
		Name string `yaml:"name"`
		Jobs []struct {
			Name     string `yaml:"name"`
			Defaults any    `yaml:"defaults"`
		} `yaml:"jobs"`
	}

	yml := `
shared:
  endpoint: &base_url "http://example.com"

name: Test Config
jobs:
- name: Job1
  defaults:
    http:
      url: *base_url
`

	var config Config
	if err := codec.Unmarshal([]byte(yml), &config); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if config.Name != "Test Config" {
		t.Fatalf("unexpected name: %v", config.Name)
	}

	if len(config.Jobs) != 1 {
		t.Fatalf("unexpected jobs length: %d", len(config.Jobs))
	}

	job := config.Jobs[0]
	if job.Name != "Job1" {
		t.Fatalf("unexpected job name: %v", job.Name)
	}

	defaults, ok := job.Defaults.(map[string]any)
	if !ok {
		t.Fatalf("defaults is not a map: %T", job.Defaults)
	}

	http, ok := defaults["http"].(map[string]any)
	if !ok {
		t.Fatalf("http is not a map: %T", defaults["http"])
	}

	url, ok := http["url"].(string)
	if !ok {
		t.Fatalf("url is not a string: %T", http["url"])
	}
	if url != "http://example.com" {
		t.Fatalf("unexpected url: %v", url)
	}
}

func TestDecoder_SiblingAnchorAlias(t *testing.T) {
	// Test that aliases to sibling anchors within a parent anchor resolve
	// correctly. An anchor counts as open only while its own node is decoded:
	// one left open after its node was done made a sibling alias look like an
	// alias inside its own anchor.
	t.Run("simple sibling alias", func(t *testing.T) {
		yml := `
a: &a
  b: &b value
  ref: *b
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		a, ok := result["a"].(map[string]any)
		if !ok {
			t.Fatalf("a is not a map: %T", result["a"])
		}
		if a["b"] != "value" {
			t.Fatalf("unexpected a.b: %v", a["b"])
		}
		if a["ref"] != "value" {
			t.Fatalf("unexpected a.ref: got %v, want \"value\"", a["ref"])
		}
	})

	t.Run("multiple sibling aliases", func(t *testing.T) {
		yml := `
config: &config
  db: &db postgres://localhost/mydb
  cache: &cache redis://localhost:6379
  app:
    database_url: *db
    cache_url: *cache
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		cfg, _ := result["config"].(map[string]any)
		app, _ := cfg["app"].(map[string]any)
		if app["database_url"] != "postgres://localhost/mydb" {
			t.Fatalf("unexpected database_url: got %v, want postgres://localhost/mydb", app["database_url"])
		}
		if app["cache_url"] != "redis://localhost:6379" {
			t.Fatalf("unexpected cache_url: got %v, want redis://localhost:6379", app["cache_url"])
		}
	})

	t.Run("nested map sibling alias", func(t *testing.T) {
		yml := `
service: &service
  auth: &auth
    required: true
    type: jwt
  endpoint:
    security: *auth
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		svc, _ := result["service"].(map[string]any)
		ep, _ := svc["endpoint"].(map[string]any)
		security, ok := ep["security"].(map[string]any)
		if !ok {
			t.Fatalf("security is not a map: %T (value: %v)", ep["security"], ep["security"])
		}
		if security["required"] != true {
			t.Fatalf("unexpected security.required: %v", security["required"])
		}
		if security["type"] != "jwt" {
			t.Fatalf("unexpected security.type: %v", security["type"])
		}
	})

	t.Run("self recursion is refused", func(t *testing.T) {
		// An alias standing inside the node its own anchor names makes a cycle,
		// which a Go value cannot hold. It used to decode to nil, which reported a mapping with a
		// null in it and no error at all; libfyaml and go.yaml.in/yaml/v3 both
		// refuse the document.
		yml := `
a: &a
  self: *a
`
		var result map[string]any
		err := codec.Unmarshal([]byte(yml), &result)
		if err == nil {
			t.Fatalf("a self-recursive alias should be refused, decoded to: %v", result)
		}
		if !strings.Contains(err.Error(), `alias "a" stands inside its own anchor`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestAliasInsideItsOwnAnchor holds the rule that a decode refuses a cycle.
//
// The parser resolves the alias and the tree holds the cycle, but a Go value has nowhere to put one.
//
// The alias used to decode to nil, which reported a mapping with a null in it
// and no error at all. libfyaml and go.yaml.in/yaml/v3 both refuse the
// document; PyYAML accepts it and builds the cycle. Nothing returns nil.
func TestAliasInsideItsOwnAnchor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "directly", src: "a: &x\n  b: *x\n", want: `alias "x" stands inside its own anchor`},
		{name: "through another anchor", src: "a: &p\n  q: &r\n    s: *p\n", want: `alias "p" stands inside its own anchor`},
		{name: "in a sequence", src: "a: &x\n  - *x\n", want: `alias "x" stands inside its own anchor`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var v map[string]any
			err := codec.Unmarshal([]byte(tc.src), &v)
			require.Error(t, err, "decoded to %v", v)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("an anchor named after it is resolved is fine", func(t *testing.T) {
		t.Parallel()

		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: &x\n  b: 1\nc: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": map[string]any{"b": uint64(1)},
			"c": map[string]any{"b": uint64(1)},
		}, v)
	})
}

// TestAnchorScopeIsOneDocument holds the rule that an anchor belongs to the
// document it was written in.
//
// A stream is a run of documents, each independent of the rest, so the books
// close at every boundary. libfyaml keeps a stream-scoped table and resolves
// across boundaries; go.yaml.in/yaml/v3 does the same by an accident it is
// removing (yaml/go-yaml#328). We refuse, because an alias may name any earlier
// anchor of its name, so carrying the table on would pin every anchored subtree
// of the stream until the stream ends.
func TestAnchorScopeIsOneDocument(t *testing.T) {
	t.Parallel()

	t.Run("an alias cannot name an anchor from an earlier document", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("nor from two documents earlier", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: 2\n---\nc: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("the same name may be anchored in each document", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: &x 1\nb: *x\n---\nc: &x 2\nd: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1), "b": uint64(1)},
			map[string]any{"c": uint64(2), "d": uint64(2)},
		}, docs)
	})

	t.Run("a document may anchor and alias on its own", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: 1\n---\nb: &x 2\nc: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1)},
			map[string]any{"b": uint64(2), "c": uint64(2)},
		}, docs)
	})

	t.Run("a name anchored twice takes the most recent", func(t *testing.T) {
		t.Parallel()

		// The specification allows the reuse: "an alias event refers to the most
		// recent event in the serialization having the specified anchor.
		// Therefore, anchors need not be unique within a serialization."
		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: &x 1\nb: *x\nc: &x 2\nd: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": uint64(1), "b": uint64(1), "c": uint64(2), "d": uint64(2),
		}, v)
	})
}

// decodeStreamAll reads every document of src, stopping at the first refusal.
func decodeStreamAll(t *testing.T, src string) ([]any, error) {
	t.Helper()

	dec := codec.NewDecoder(strings.NewReader(src))

	var docs []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return docs, err
		}
		docs = append(docs, v)
	}
}

// The walking reader lost an anchor declared on a flow entry written as a key
// alone, when the entry's tag stands before its anchor.
//
// Decoding into an any walks the source, so "{!!null &a1 null, k: *a1}" came
// back as `could not find alias "a1"`, and ToJSON, which walks too, refused it
// the same way. The tree holds the anchor: it reads {"null": null, "k": null}.
//
// The parse hands a flow key written alone over as its outermost property and
// nothing under it, so the anchor inside the tag never opened. The walk and
// ToJSON now record that anchor as the tag closes, with the tagged value.
//
// Three things had to be true at once, and changing any one of them made the
// walk agree with the tree: the entry is in a flow *mapping*, it is written as
// a key with no value, and its tag comes before its anchor. So
// "{&a1 !!null null, k: *a1}", "{!!str &a1 x: 1, k: *a1}" and
// "[!!null &a1 null, *a1]" kept the anchor on both paths.
//
// The reference parser reports the anchor -- "=VAL &a1 <tag:yaml.org,2002:null>
// :null" -- and libfyaml 1.0.0b1 and go.yaml.in/yaml/v3 both read the document.
// Found on 2026-09-11 by the generator, once the flow axes were weighted to
// reach a key written alone more than once in 350 documents.

// treeRead decodes src by building a tree. UseOrderedMap is what pushes the
// decode off the walking path; the ordering it also asks for is incidental.
func treeRead(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()), "%q", src)

	return v
}

// TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone holds the walk, ToJSON and
// the tree to one reading on both sides of that line.
func TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone(t *testing.T) {
	t.Run("the walk and ToJSON keep the anchor, as the tree does", func(t *testing.T) {
		for _, tc := range []struct{ src, walked, writes, tree string }{
			// The "!!null" key is the nil interface and not the text "null":
			// a MapItem.Key carries what the key resolves to.
			{
				src:    "{!!null &a1 null, k: *a1}\n",
				walked: `map[interface {}]interface {}{interface {}(nil):interface {}(nil), "k":interface {}(nil)}`,
				writes: `{"null":null,"k":null}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:interface {}(nil), Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:interface {}(nil)}}}`,
			},
			{
				src:    "{!!str &a1 x, k: *a1}\n",
				walked: `map[string]interface {}{"k":"x", "x":interface {}(nil)}`,
				writes: `{"x":null,"k":"x"}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:"x", Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:"x"}}}`,
			},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "the walk: %q", tc.src)
			assert.Equalf(t, tc.walked, fmt.Sprintf("%#v", got), "the walk: %q", tc.src)

			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "ToJSON: %q", tc.src)
			assert.Equalf(t, tc.writes, string(out), "ToJSON: %q", tc.src)

			assert.Equalf(t, tc.tree, fmt.Sprintf("%#v", treeRead(t, tc.src)), "the tree holds the anchor: %q", tc.src)
		}
	})

	t.Run("an alias to a tagged anchor names the tagged value on every path", func(t *testing.T) {
		// 6.9 gives the node both properties, so a1 names 5 and not "5".
		const src = "a: !!int &a1 \"5\"\nb: *a1\n"

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": uint64(5), "b": uint64(5)}, got, "the walk")

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":5,"b":5}`, string(out), "ToJSON")
	})

	t.Run("changing any one of the three makes the walk agree", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			// The anchor before the tag.
			{src: "{&a1 !!null null, k: *a1}\n", writes: `{"null":null,"k":null}`},
			// The entry carries a value.
			{src: "{!!str &a1 x: 1, k: *a1}\n", writes: `{"x":1,"k":"x"}`},
			// A sequence rather than a mapping.
			{src: "[!!null &a1 null, *a1]\n", writes: `[null,null]`},
			// A block mapping.
			{src: "a: !!null &a1 null\nk: *a1\n", writes: `{"a":null,"k":null}`},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)

			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.NotNilf(t, got, "%q", tc.src)
		}
	})
}
