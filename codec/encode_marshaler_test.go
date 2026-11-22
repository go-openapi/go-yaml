// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// encodeMarshalerCases covers time.Time, time.Duration and an encoding.TextMarshaler.
func encodeMarshalerCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"time/zero",
			"v: 0001-01-01T00:00:00Z\n",
			map[string]time.Time{"v": {}},
			nil,
		},
		{
			"time/pointer-to-zero",
			"v: 0001-01-01T00:00:00Z\n",
			map[string]*time.Time{"v": {}},
			nil,
		},
		{
			"time/nil-pointer",
			"v: null\n",
			map[string]*time.Time{"v": nil},
			nil,
		},
		{
			"duration/value",
			"v: 30s\n",
			map[string]time.Duration{"v": 30 * time.Second},
			nil,
		},
		{
			"duration/pointer",
			"v: 30s\n",
			map[string]*time.Duration{"v": new(30 * time.Second)},
			nil,
		},
		{
			"duration/nil-pointer",
			"v: null\n",
			map[string]*time.Duration{"v": nil},
			nil,
		},
		{
			"text-marshaler/word",
			"v: test\n",
			TestTextUnmarshalerContainer{V: "test"},
			nil,
		},
		{
			"text-marshaler/digit-quoted",
			"v: \"1\"\n",
			TestTextUnmarshalerContainer{V: "1"},
			nil,
		},
		{
			"text-marshaler/hash-quoted",
			"v: \"#\"\n",
			TestTextUnmarshalerContainer{V: "#"},
			nil,
		},
	})
}

type useJSONMarshalerTest struct{}

func (t useJSONMarshalerTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"a":[1, 2, 3]}`), nil
}

func TestEncoder_UseJSONMarshaler(t *testing.T) {
	got, err := codec.Marshal(useJSONMarshalerTest{}, codec.UseJSONMarshaler())
	require.NoError(t, err)
	expected := `
a:
- 1
- 2
- 3
`
	require.Equal(t, expected, "\n"+string(got))
}

func TestEncoder_CustomMarshaler(t *testing.T) {
	t.Run("override struct type", func(t *testing.T) {
		type T struct {
			Foo string `yaml:"foo"`
		}
		b, err := codec.Marshal(&T{Foo: "bar"}, codec.CustomMarshaler[T](func(_ T) ([]byte, error) {
			return []byte(`"override"`), nil
		}))
		require.NoError(t, err)
		require.Equal(t, []byte("\"override\"\n"), b)
	})
	t.Run("override bytes type", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		b, err := codec.Marshal(&T{Foo: []byte("bar")}, codec.CustomMarshaler[[]byte](func(v []byte) ([]byte, error) {
			require.Equal(t, []byte("bar"), v)
			return []byte(`override`), nil
		}))
		require.NoError(t, err)
		require.Equal(t, []byte("foo: override\n"), b)
	})
}

func TestEncoder_UnmarshallableTypes(t *testing.T) {
	for _, test := range []struct {
		desc        string
		input       any
		expectedErr string
	}{
		{
			desc:        "channel",
			input:       make(chan int),
			expectedErr: "unknown value type chan int",
		},
		{
			desc:        "function",
			input:       func() {},
			expectedErr: "unknown value type func()",
		},
		{
			desc:        "complex number",
			input:       complex(10, 11),
			expectedErr: "unknown value type complex128",
		},
		{
			desc:        "unsafe pointer",
			input:       unsafe.Pointer(&struct{}{}),
			expectedErr: "unknown value type unsafe.Pointer",
		},
		{
			desc:        "uintptr",
			input:       uintptr(0x1234),
			expectedErr: "unknown value type uintptr",
		},
		{
			desc:        "map with channel",
			input:       map[string]any{"key": make(chan string)},
			expectedErr: "unknown value type chan string",
		},
		{
			desc: "nested map with func",
			input: map[string]any{
				"a": map[string]any{
					"b": func(_ string) {},
				},
			},
			expectedErr: "unknown value type func(string)",
		},
		{
			desc:        "slice with channel",
			input:       []any{make(chan bool)},
			expectedErr: "unknown value type chan bool",
		},
		{
			desc:        "nested slice with complex number",
			input:       []any{[]any{complex(10, 11)}},
			expectedErr: "unknown value type complex128",
		},
		{
			desc: "struct with unsafe pointer",
			input: struct {
				Field unsafe.Pointer `yaml:"field"`
			}{},
			expectedErr: "unknown value type unsafe.Pointer",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var buf bytes.Buffer
			err := codec.NewEncoder(&buf).Encode(test.input)
			assert.EqualError(t, err, test.expectedErr)
		})
	}
}

type tMarshal []string

func (t *tMarshal) MarshalYAML() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("tags:\n")
	for i, v := range *t {
		if i == 0 {
			fmt.Fprintf(&buf, "- %s\n", v)
		} else {
			fmt.Fprintf(&buf, "  %s\n", v)
		}
	}
	return buf.Bytes(), nil
}

func Test_Marshaler(t *testing.T) {
	const expected = `- hello-world
`

	// sanity check
	var l []string
	require.NoError(t, codec.Unmarshal([]byte(expected), &l))

	buf, err := codec.Marshal(tMarshal{"hello-world"})
	require.NoError(t, err)

	require.Equal(t, expected, string(buf))

	t.Logf("%s", buf)
}

type SlowMarshaler struct {
	A string
	B int
}

type FastMarshaler struct {
	A string
	B int
}

type (
	TextMarshaler          int64
	TextMarshalerContainer struct {
		Field TextMarshaler `yaml:"field"`
	}
)

func (v SlowMarshaler) MarshalYAML() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("tags:\n")
	buf.WriteString("- slow-marshaler\n")
	buf.WriteString("a: " + v.A + "\n")
	buf.WriteString("b: " + strconv.FormatInt(int64(v.B), 10) + "\n")
	return buf.Bytes(), nil
}

func (v FastMarshaler) MarshalYAML() (any, error) {
	return mapSliceOf(
		item("tags", []string{"fast-marshaler"}),
		item("a", v.A),
		item("b", v.B),
	), nil
}

func (t TextMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(t), 8)), nil
}

type bytesMarshaler struct{}

func (b *bytesMarshaler) MarshalYAML() ([]byte, error) {
	return codec.Marshal(map[string]any{"d": "foo"})
}

func TestBytesMarshaler(t *testing.T) {
	b, err := codec.Marshal(map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": &bytesMarshaler{},
			},
		},
	})
	require.NoError(t, err)
	expected := `
a:
  b:
    c:
      d: foo
`
	got := "\n" + string(b)
	require.Equal(t, expected, got)
}

type customMapSliceOneItemMarshaler struct{}

func (m *customMapSliceOneItemMarshaler) MarshalYAML() ([]byte, error) {
	v := mapSliceOf(item("a", "b"))
	return codec.Marshal(v)
}

type customMapSliceTwoItemMarshaler struct{}

func (m *customMapSliceTwoItemMarshaler) MarshalYAML() ([]byte, error) {
	v := mapSliceOf(item("a", "b"), item("b", "c"))
	return codec.Marshal(v)
}

func TestCustomMapSliceMarshaler(t *testing.T) {
	type T struct {
		A *customMapSliceOneItemMarshaler `yaml:"a"`
		B *customMapSliceTwoItemMarshaler `yaml:"b"`
	}
	b, err := codec.Marshal(&T{
		A: &customMapSliceOneItemMarshaler{},
		B: &customMapSliceTwoItemMarshaler{},
	})
	require.NoError(t, err)
	expected := `
a:
  a: b
b:
  a: b
  b: c
`
	got := "\n" + string(b)
	require.Equal(t, expected, got)
}

type Issue174 struct {
	K string
	V []int
}

func (v Issue174) MarshalYAML() ([]byte, error) {
	return codec.Marshal(map[string][]int{v.K: v.V}, codec.Flow(true))
}

func TestIssue174(t *testing.T) {
	b, err := codec.Marshal(Issue174{"00:00:00-23:59:59", []int{1, 2, 3}})
	require.NoError(t, err)
	got := strings.TrimSuffix(string(b), "\n")
	require.Equal(t, `{"00:00:00-23:59:59": [1, 2, 3]}`, got)
}
