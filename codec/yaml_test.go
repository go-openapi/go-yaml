// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

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
