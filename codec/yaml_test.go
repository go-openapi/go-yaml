// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

func TestRoundTripWithComment(t *testing.T) {
	yml := `
# head comment
key: value # line comment
`
	var v struct {
		Key string
	}
	comments := codec.CommentMap{}

	require.NoError(t, codec.Unmarshal([]byte(yml), &v, codec.Strict(), codec.CommentToMap(comments)))
	out, err := codec.Marshal(v, codec.WithComment(comments))
	require.NoError(t, err)
	got := "\n" + string(out)
	require.Equal(t, yml, got)
}

func TestStreamDecodingWithComment(t *testing.T) {
	yml := `
# comment
---
a:
  b:
    c: # comment
---
foo: bar # comment
---
- a
- b
- c # comment
`
	cm := codec.CommentMap{}
	dec := codec.NewDecoder(strings.NewReader(yml), codec.CommentToMap(cm))
	var commentPathsWithDocIndex [][]string
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		paths := make([]string, 0, len(cm))
		for k := range cm {
			paths = append(paths, k)
		}
		commentPathsWithDocIndex = append(commentPathsWithDocIndex, paths)
		for k := range cm {
			delete(cm, k)
		}
	}
	require.Equal(t, [][]string{
		{"$.a.b.c"},
		{"$.foo"},
		{"$[2]"},
	}, commentPathsWithDocIndex)
}

func TestDecodeKeepAddress(t *testing.T) {
	data := `
a: &a [_]
b: &b [*a,*a]
c: &c [*b,*b]
d: &d [*c,*c]
`
	// ShareAliases: every alias of one anchor stands for one Go value, which is
	// what this checks. Without it each builds its own and the addresses differ.
	var v map[string]any
	require.NoError(t, codec.Unmarshal([]byte(data), &v, codec.ShareAliases()))
	a := v["a"]
	b := v["b"]
	for _, vv := range v["b"].([]any) {
		require.Equalf(t, fmt.Sprintf("%p", a), fmt.Sprintf("%p", vv), "an element of b does not keep the address of a")
	}
	for _, vv := range v["c"].([]any) {
		require.Equalf(t, fmt.Sprintf("%p", b), fmt.Sprintf("%p", vv), "an element of c does not keep the address of b")
	}
}

func TestSmartAnchor(t *testing.T) {
	data := `
a: &a [_,_,_,_,_,_,_,_,_,_,_,_,_,_,_]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f,*f]
h: &h [*g,*g,*g,*g,*g,*g,*g,*g,*g,*g]
i: &i [*h,*h,*h,*h,*h,*h,*h,*h,*h,*h]
`
	// WithSmartAnchor writes "*a" where two values stand at one address, so the
	// decode has to leave them there: ShareAliases is what puts them there.
	// Without it this document builds 10^9 values and the alias budget stops it.
	var v any
	require.NoError(t, codec.Unmarshal([]byte(data), &v, codec.ShareAliases()))
	got, err := codec.Marshal(v, codec.WithSmartAnchor())
	require.NoError(t, err)
	expected := `
a: &a
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
b: &b
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
c: &c
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
d: &d
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
e: &e
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
f: &f
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
g: &g
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
h: &h
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
i:
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
`
	require.Equal(t, strings.TrimPrefix(expected, "\n"), string(got))
}

func TestCustomErrorMessage(t *testing.T) {
	data := `
foo:
  bar:
    foo: 2
  baz:
    foo: 3
foo: 2
`
	// The document repeats a key. The parse reads it and records the repeat;
	// the load is what reports one, so the error with a position on it comes
	// from there.
	var into any
	err := codec.Unmarshal([]byte(data), &into)
	require.Error(t, err)
	var yamlErr *yamlerrors.Error
	require.ErrorAs(t, err, &yamlErr)
	expected := `
[7:1] custom message
   4 |     foo: 2
   5 |   baz:
   6 |     foo: 3
>  7 | foo: 2
       ^
`
	got := "\n" + yamlerrors.FormatErrorAtToken("custom message", yamlErr.GetToken(), []byte(data), false, true)
	require.Equal(t, expected, got)
}

func checkRawValue[T any](t *testing.T, v codec.RawMessage, expected T) {
	t.Helper()

	var actual T

	if !assert.NoError(t, codec.Unmarshal(v, &actual)) {
		return
	}

	assert.Equal(t, expected, actual)
}

func checkJSONRawValue[T any](t *testing.T, v json.RawMessage, expected T) {
	t.Helper()

	var actual T

	if !assert.NoError(t, json.Unmarshal(v, &actual)) {
		return
	}

	assert.Equal(t, expected, actual)

	checkRawValue(t, codec.RawMessage(v), expected)
}

func TestRawMessage(t *testing.T) {
	data := []byte(`
a: 1
b: "asdf"
c:
  foo: bar
`)

	var m map[string]codec.RawMessage
	require.NoError(t, codec.Unmarshal(data, &m))

	require.Len(t, m, 3)

	checkRawValue(t, m["a"], 1)
	checkRawValue(t, m["b"], "asdf")
	checkRawValue(t, m["c"], map[string]string{"foo": "bar"})

	dt, err := codec.Marshal(m)
	require.NoError(t, err)
	var m2 map[string]codec.RawMessage
	require.NoError(t, codec.Unmarshal(dt, &m2))

	checkRawValue(t, m2["a"], 1)
	checkRawValue(t, m2["b"], "asdf")
	checkRawValue(t, m2["c"], map[string]string{"foo": "bar"})

	dt, err = json.Marshal(m2)
	require.NoError(t, err)

	var m3 map[string]codec.RawMessage
	require.NoError(t, codec.Unmarshal(dt, &m3))
	checkRawValue(t, m3["a"], 1)
	checkRawValue(t, m3["b"], "asdf")
	checkRawValue(t, m3["c"], map[string]string{"foo": "bar"})

	var m4 map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(dt, &m4))
	checkJSONRawValue(t, m4["a"], 1)
	checkJSONRawValue(t, m4["b"], "asdf")
	checkJSONRawValue(t, m4["c"], map[string]string{"foo": "bar"})
}

type rawYAMLWrapper struct {
	StaticField  string           `json:"staticField"  yaml:"staticField"`
	DynamicField codec.RawMessage `json:"dynamicField" yaml:"dynamicField"`
}

type rawJSONWrapper struct {
	StaticField  string          `json:"staticField"  yaml:"staticField"`
	DynamicField json.RawMessage `json:"dynamicField" yaml:"dynamicField"`
}

func (w rawJSONWrapper) Equals(o *rawJSONWrapper) bool {
	if w.StaticField != o.StaticField {
		return false
	}
	return reflect.DeepEqual(w.DynamicField, o.DynamicField)
}

type dynamicField struct {
	A int               `json:"a" yaml:"a"`
	B string            `json:"b" yaml:"b"`
	C map[string]string `json:"c" yaml:"c"`
}

func (t dynamicField) Equals(o *dynamicField) bool {
	if t.A != o.A {
		return false
	}
	if t.B != o.B {
		return false
	}
	if len(t.C) != len(o.C) {
		return false
	}
	for k, v := range t.C {
		ov, exists := o.C[k]
		if !exists {
			return false
		}
		if v != ov {
			return false
		}
	}
	return true
}

func TestRawMessageJSONCompatibility(t *testing.T) {
	rawData := []byte(`staticField: value
dynamicField:
  a: 1
  b: abcd
  c:
    foo: bar
    something: else
`)

	expectedDynamicFieldValue := &dynamicField{
		A: 1,
		B: "abcd",
		C: map[string]string{
			"foo":       "bar",
			"something": "else",
		},
	}

	t.Run("UseJSONUnmarshaler and json.RawMessage", func(t *testing.T) {
		var wrapper rawJSONWrapper
		require.NoError(t, codec.Unmarshal(rawData, &wrapper, codec.UseJSONUnmarshaler()))
		require.Equal(t, "value", wrapper.StaticField)
		var dynamicFieldValue dynamicField
		require.NoError(t, codec.Unmarshal(wrapper.DynamicField, &dynamicFieldValue))
		require.Truef(t, dynamicFieldValue.Equals(expectedDynamicFieldValue), "dynamic field is %v", dynamicFieldValue)
	})

	t.Run("UseJSONUnmarshaler and codec.RawMessage", func(t *testing.T) {
		var wrapper rawYAMLWrapper
		require.NoError(t, codec.Unmarshal(rawData, &wrapper, codec.UseJSONUnmarshaler()))
		require.Equal(t, "value", wrapper.StaticField)
		var dynamicFieldValue dynamicField
		require.NoError(t, codec.Unmarshal(wrapper.DynamicField, &dynamicFieldValue))
		require.Truef(t, dynamicFieldValue.Equals(expectedDynamicFieldValue), "dynamic field is %v", dynamicFieldValue)
	})

	t.Run("UseJSONMarshaler and json.RawMessage", func(t *testing.T) {
		dynamicFieldBytes, err := codec.Marshal(expectedDynamicFieldValue)
		require.NoError(t, err)
		wrapper := rawJSONWrapper{
			StaticField:  "value",
			DynamicField: json.RawMessage(dynamicFieldBytes),
		}
		wrapperBytes, err := codec.Marshal(&wrapper, codec.UseJSONMarshaler())
		require.NoError(t, err)
		var unmarshaledWrapper rawJSONWrapper
		require.NoError(t, codec.Unmarshal(wrapperBytes, &unmarshaledWrapper, codec.UseJSONUnmarshaler()))
		require.Equal(t, wrapper.StaticField, unmarshaledWrapper.StaticField)
		var unmarshaledDynamicFieldValue dynamicField
		require.NoError(t, codec.Unmarshal(unmarshaledWrapper.DynamicField, &unmarshaledDynamicFieldValue, codec.UseJSONUnmarshaler()))
		require.Truef(t, unmarshaledDynamicFieldValue.Equals(expectedDynamicFieldValue), "dynamic field is %v", unmarshaledDynamicFieldValue)
	})

	t.Run("UseJSONMarshaler and codec.RawMessage", func(t *testing.T) {
		dynamicFieldBytes, err := codec.Marshal(expectedDynamicFieldValue)
		require.NoError(t, err)
		wrapper := rawYAMLWrapper{
			StaticField:  "value",
			DynamicField: codec.RawMessage(dynamicFieldBytes),
		}
		wrapperBytes, err := codec.Marshal(&wrapper, codec.UseJSONMarshaler())
		require.NoError(t, err)
		var unmarshaledWrapper rawYAMLWrapper
		require.NoError(t, codec.Unmarshal(wrapperBytes, &unmarshaledWrapper, codec.UseJSONUnmarshaler()))
		require.Equal(t, wrapper.StaticField, unmarshaledWrapper.StaticField)
		var unmarshaledDynamicFieldValue dynamicField
		require.NoError(t, codec.Unmarshal(unmarshaledWrapper.DynamicField, &unmarshaledDynamicFieldValue, codec.UseJSONUnmarshaler()))
		require.Truef(t, unmarshaledDynamicFieldValue.Equals(expectedDynamicFieldValue), "dynamic field is %v", unmarshaledDynamicFieldValue)
	})
}
