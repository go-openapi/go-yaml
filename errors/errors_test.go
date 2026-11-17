// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

func at(line, col int) *token.Token {
	return &token.Token{
		Value:    "value",
		Position: token.Position{Line: int32(line), Column: int32(col)},
	}
}

func TestErrorKind(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind error
		msg  string
	}{
		{
			name: "syntax",
			err:  errors.NewSyntax("could not find expected ':'", at(2, 3)),
			kind: errors.ErrSyntax,
			msg:  "could not find expected ':'",
		},
		{
			name: "type mismatch",
			err:  errors.NewTypeMismatch(reflect.TypeFor[int](), reflect.TypeFor[string](), at(1, 1)),
			kind: errors.ErrTypeMismatch,
			msg:  "cannot unmarshal string into Go value of type int",
		},
		{
			name: "overflow",
			err:  errors.NewOverflow(reflect.TypeFor[int8](), "300", at(1, 1)),
			kind: errors.ErrOverflow,
			msg:  "cannot unmarshal 300 into Go value of type int8 ( overflow )",
		},
		{
			name: "duplicate key",
			err:  errors.NewDuplicateKey(`duplicate key "a"`, at(3, 1)),
			kind: errors.ErrDuplicateKey,
			msg:  `duplicate key "a"`,
		},
		{
			name: "unknown field",
			err:  errors.NewUnknownField(`unknown field "b"`, at(4, 1)),
			kind: errors.ErrUnknownField,
			msg:  `unknown field "b"`,
		},
		{
			name: "unexpected node type",
			err:  errors.NewUnexpectedNodeType(ast.SequenceType, ast.MappingType, at(5, 1)),
			kind: errors.ErrUnexpectedNodeType,
			msg:  "sequence was used where mapping is expected",
		},
	}

	kinds := []error{
		errors.ErrSyntax, errors.ErrTypeMismatch, errors.ErrOverflow,
		errors.ErrDuplicateKey, errors.ErrUnknownField, errors.ErrUnexpectedNodeType,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var yerr *errors.Error
			if !stderrors.As(tt.err, &yerr) {
				t.Fatalf("As failed for %T", tt.err)
			}
			if got := yerr.GetMessage(); got != tt.msg {
				t.Errorf("GetMessage() = %q, want %q", got, tt.msg)
			}

			// the kind it carries matches, and no other kind does
			for _, kind := range kinds {
				want := kind == tt.kind //nolint:errorlint // compares two sentinels, not an error to a sentinel
				if got := stderrors.Is(tt.err, kind); got != want {
					t.Errorf("Is(err, %v) = %v, want %v", kind, got, want)
				}
			}
		})
	}
}

func TestErrorPosition(t *testing.T) {
	err := errors.NewSyntax("bad", at(7, 4))

	if got := err.GetToken().Position.Line; got != 7 {
		t.Errorf("line = %d, want 7", got)
	}
	if !strings.HasPrefix(err.Error(), "[7:4] bad") {
		t.Errorf("Error() = %q, want it to start with %q", err.Error(), "[7:4] bad")
	}

	// an error with no token prints the message alone
	bare := errors.NewSyntax("nowhere", nil)
	if got := bare.Error(); got != "nowhere" {
		t.Errorf("Error() = %q, want %q", got, "nowhere")
	}
	if bare.GetToken() != nil {
		t.Errorf("GetToken() = %v, want nil", bare.GetToken())
	}
}

func TestStructField(t *testing.T) {
	err := errors.NewTypeMismatch(reflect.TypeFor[int](), reflect.TypeFor[string](), at(1, 1))

	if got := err.StructField(); got != "" {
		t.Errorf("StructField() = %q, want empty", got)
	}

	err.SetStructField("T.A")
	if got := err.StructField(); got != "T.A" {
		t.Errorf("StructField() = %q, want %q", got, "T.A")
	}
	want := "cannot unmarshal string into Go struct field T.A of type int"
	if got := err.GetMessage(); got != want {
		t.Errorf("GetMessage() = %q, want %q", got, want)
	}

	// the decoder prefixes as it unwinds
	err.SetStructField("U." + err.StructField())
	want = "cannot unmarshal string into Go struct field U.T.A of type int"
	if got := err.GetMessage(); got != want {
		t.Errorf("GetMessage() = %q, want %q", got, want)
	}
}

func TestWithSource(t *testing.T) {
	const doc = "a: 1\nb: [\nc: 3\n"

	err := errors.NewSyntax("could not find expected ']'", at(2, 4))
	if strings.Contains(err.Error(), "b: [") {
		t.Fatalf("Error() drew source before WithSource: %q", err.Error())
	}

	if got := errors.WithSource(err, errors.Source{Text: doc, FirstLine: 1}); got != error(err) { //nolint:errorlint // WithSource must return the same error, not a wrapper
		t.Errorf("WithSource returned %v, want the error it was given", got)
	}
	if !strings.Contains(err.Error(), "b: [") {
		t.Errorf("Error() = %q, want it to draw the source line %q", err.Error(), "b: [")
	}

	// inclSource false keeps the message and drops the drawing
	if got := err.FormatError(false, false); strings.Contains(got, "b: [") {
		t.Errorf("FormatError(false, false) = %q, want no source", got)
	}

	// an error that cannot be told its source is returned untouched
	plain := stderrors.New("plain")
	if got := errors.WithSource(plain, errors.Source{Text: doc, FirstLine: 1}); got != plain { //nolint:errorlint // WithSource must return the same error, not a wrapper
		t.Errorf("WithSource returned %v, want the error it was given", got)
	}
}

func TestFormatError(t *testing.T) {
	const doc = "a: 1\nb: 2\n"

	err := errors.WithSource(
		errors.NewSyntax("bad", at(2, 1)),
		errors.Source{Text: doc, FirstLine: 1},
	)

	if got := errors.FormatError(err, false, true); !strings.Contains(got, "b: 2") {
		t.Errorf("FormatError(err, false, true) = %q, want it to draw %q", got, "b: 2")
	}
	if got := errors.FormatError(err, false, false); got != "[2:1] bad" {
		t.Errorf("FormatError(err, false, false) = %q, want %q", got, "[2:1] bad")
	}

	// wrapping still reaches the Error inside
	wrapped := fmt.Errorf("decoding: %w", err)
	if got := errors.FormatError(wrapped, false, false); got != "[2:1] bad" {
		t.Errorf("FormatError(wrapped, false, false) = %q, want %q", got, "[2:1] bad")
	}

	// an error this package did not raise prints as its own Error method returns it
	plain := stderrors.New("plain")
	if got := errors.FormatError(plain, false, true); got != "plain" {
		t.Errorf("FormatError(plain, false, true) = %q, want %q", got, "plain")
	}
}

func TestFormatErrorAtToken(t *testing.T) {
	const doc = "a: 1\nb: 2\n"

	got := errors.FormatErrorAtToken("custom message", at(2, 1), []byte(doc), false, true)
	if !strings.HasPrefix(got, "[2:1] custom message") {
		t.Errorf("FormatErrorAtToken = %q, want it to start with %q", got, "[2:1] custom message")
	}
	if !strings.Contains(got, "b: 2") {
		t.Errorf("FormatErrorAtToken = %q, want it to draw %q", got, "b: 2")
	}

	// no source: the position and the message alone
	got = errors.FormatErrorAtToken("custom message", at(2, 1), nil, false, true)
	if got != "[2:1] custom message" {
		t.Errorf("FormatErrorAtToken = %q, want %q", got, "[2:1] custom message")
	}

	// no token: the message alone
	got = errors.FormatErrorAtToken("custom message", nil, []byte(doc), false, true)
	if got != "custom message" {
		t.Errorf("FormatErrorAtToken = %q, want %q", got, "custom message")
	}
}
