// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

func reading(t *testing.T, src string) any {
	t.Helper()

	var v any
	if err := codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v); err != nil {
		t.Fatalf("%q: %v", src, err)
	}

	return v
}

// TestFixedAVersionDirectiveReachesTheRootScalar covers the one position a
// "%YAML" directive did not reach.
//
// 6.8.1: the directive applies to the document that follows it. It reached
// every scalar of that document except the one written directly under the
// "---", so "%YAML 1.1" over "N" read the string "N" where the same document
// read false at every other position.
//
// The scanner resolves a plain scalar as it cuts it, and the grouping reads one
// token past the directive to know the directive's own document has ended. For
// a document whose body is a bare scalar that one token is the body, already
// cut and typed by 1.2 before the parser read the directive at all; inside any
// collection a "-", a key or a "[" stands between the two, so the schema was in
// place by the time the scalar was cut. Parser.retypeAhead reads what was cut
// too early again, which token.ScalarType makes cheap: it is a pure function of
// the text and the schema.
func TestFixedAVersionDirectiveReachesTheRootScalar(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "%YAML 1.1\n---\nN\n", want: false},
		{src: "%YAML 1.1\n---\ny\n", want: true},
		{src: "%YAML 1.1\n---\nyes\n", want: true},
		{src: "%YAML 1.1\n---\n0777\n", want: uint64(511)},
		{src: "%YAML 1.1\n---\n1_000\n", want: uint64(1000)},
	} {
		if v := reading(t, tc.src); v != tc.want {
			t.Errorf("%q reads %#v, want %#v", tc.src, v, tc.want)
		}
	}
}

// TestAQuotedRootScalarIsUntouched holds the line the fix must not cross.
//
// A quoted or folded scalar is a string whatever it spells, so reading it again
// against another schema would be wrong. The scanner gives it a type of its own
// and retypeAhead leaves those alone.
func TestAQuotedRootScalarIsUntouched(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "%YAML 1.1\n---\n\"N\"\n", want: "N"},
		{src: "%YAML 1.1\n---\n'y'\n", want: "y"},
		{src: "%YAML 1.1\n---\n\"0777\"\n", want: "0777"},
		{src: "%YAML 1.1\n---\n!!str N\n", want: "N"},

		// And a document naming no version still reads 1.2.
		{src: "---\nN\n", want: "N"},
		{src: "%YAML 1.2\n---\nN\n", want: "N"},
	} {
		if v := reading(t, tc.src); v != tc.want {
			t.Errorf("%q reads %#v, want %#v", tc.src, v, tc.want)
		}
	}
}

// TestAVersionDirectiveReachesEverywhereElse is the other half, and it is what
// makes the case above a defect rather than a library that ignores directives.
func TestAVersionDirectiveReachesEverywhereElse(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "%YAML 1.1\n---\n- N\n", want: []any{false}},
		{src: "%YAML 1.1\n---\n[N]\n", want: []any{false}},
		{src: "%YAML 1.1\n---\nk: N\n", want: map[string]any{"k": false}},
		{src: "%YAML 1.1\n---\n{k: N}\n", want: map[string]any{"k": false}},
		{src: "%YAML 1.1\n---\n- 0777\n", want: []any{uint64(511)}},
		{src: "%YAML 1.1\n---\nk: 0777\n", want: map[string]any{"k": uint64(511)}},
	} {
		got := reading(t, tc.src)
		if !equalAny(got, tc.want) {
			t.Errorf("%q reads %#v, want %#v", tc.src, got, tc.want)
		}
	}
}

// TestTheOptionReachesTheRootScalar names the route that works, which is where
// a fix would take its answer from.
func TestTheOptionReachesTheRootScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "N\n", want: "Bool"},
		{src: "0777\n", want: "Integer"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src), parser.WithYAMLVersion(parser.YAML11))
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}

		if got := f.Docs[0].Body.Type().String(); got != tc.want {
			t.Errorf("parser.WithYAMLVersion on %q gives %s, want %s", tc.src, got, tc.want)
		}
	}
}

func equalAny(a, b any) bool {
	switch x := a.(type) {
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}

		for i := range x {
			if !equalAny(x[i], y[i]) {
				return false
			}
		}

		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}

		for k, v := range x {
			if !equalAny(v, y[k]) {
				return false
			}
		}

		return true
	default:
		return a == b
	}
}

// TestFixedADocumentCarriesMoreThanOneDirective covers the commonest header
// YAML has.
//
// §6.8 puts no limit on how many directives a document may carry, and a "%YAML"
// beside a "%TAG" is the ordinary prelude. Every document with two was refused,
// whatever the two were: "unexpected directive value. document not started".
//
// The grouping read the '%' of the second one as a value belonging to the
// first, and the reader then handed both to one document body, which holds a
// single node. So the fix is in two places -- a '%' on a new line ends the
// directive it follows, and the next directive ends the document that carried
// the previous one.
func TestFixedADocumentCarriesMoreThanOneDirective(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "%YAML 1.2\n%TAG !e! tag:example.com,2000:\n---\nk: 1\n", want: map[string]any{"k": uint64(1)}},
		{src: "%TAG !e! tag:example.com,2000:\n%YAML 1.2\n---\nk: 1\n", want: map[string]any{"k": uint64(1)}},
		{src: "%TAG !e! tag:a,2000:\n%TAG !f! tag:b,2000:\n---\nk: !e!x 1\n", want: map[string]any{"k": "1"}},
		{src: "%YAML 1.2\n%FOO bar\n---\nk: 1\n", want: map[string]any{"k": uint64(1)}},

		// The version still reaches the root scalar through two directives.
		{src: "%YAML 1.1\n%TAG !e! tag:a,2000:\n---\nN\n", want: false},
	} {
		if v := reading(t, tc.src); !equalAny(v, tc.want) {
			t.Errorf("%q reads %#v, want %#v", tc.src, v, tc.want)
		}

		f, err := parser.ParseBytes([]byte(tc.src))
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)

			continue
		}
		if got := f.String(); got != tc.src {
			t.Errorf("%q renders as %q", tc.src, got)
		}
	}
}

// TestADirectiveIsStillDeclaredOnce holds the two rules the specification does
// state about repeating one, both of which the parser could not reach while any
// second directive was refused at the grouping.
//
// §6.8.1 for the version and §6.8.2.2 for the handle: "It is an error to
// specify more than one '%TAG' directive for the same handle in the same
// document.".
func TestADirectiveIsStillDeclaredOnce(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "%YAML 1.2\n%YAML 1.1\n---\nk: 1\n", want: "YAML version has already been specified"},
		{
			src:  "%TAG !e! tag:a,2011:\n%TAG !e! tag:b,2011:\n---\na: 1\n",
			want: "tag handle !e! has already been declared",
		},
	} {
		_, err := parser.ParseBytes([]byte(tc.src))
		if err == nil {
			t.Errorf("%q is read, and the specification says it is an error", tc.src)

			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q reports %v, want %q", tc.src, err, tc.want)
		}
	}

	// A handle declared again in another document is a different document's
	// handle: the scope of a directive is the document that follows it.
	const twice = "%TAG !e! tag:a,2011:\n---\na: 1\n...\n%TAG !e! tag:b,2011:\n---\nb: 2\n"
	if _, err := parser.ParseBytes([]byte(twice)); err != nil {
		t.Errorf("%q: %v", twice, err)
	}
}
