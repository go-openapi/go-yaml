// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

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
