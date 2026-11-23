// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

var errTooYoung = errors.New("too young")

type person struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
}

type team struct {
	Lead  person `yaml:"lead"`
	Title string `yaml:"title"`
}

func checkAge(v any) error {
	if p, ok := v.(person); ok && p.Age < 18 {
		return errTooYoung
	}

	return nil
}

// TestValidatorErrorsCarryErrValidation decodes structs a validator refuses. The error carries ErrValidation, not
// ErrSyntax, stands at the struct's mapping, and keeps the validator's own error for errors.Is.
func TestValidatorErrorsCarryErrValidation(t *testing.T) {
	t.Run("at the root", func(t *testing.T) {
		var p person
		err := codec.Unmarshal([]byte("name: tim\nage: 12\n"), &p, codec.Validator(checkAge))
		require.ErrorIs(t, err, yamlerrors.ErrValidation)
		require.ErrorIs(t, err, errTooYoung)
		assert.NotErrorIs(t, err, yamlerrors.ErrSyntax)

		var e *yamlerrors.Error
		require.ErrorAs(t, err, &e)
		assert.Equal(t, "too young", e.GetMessage())
		assert.Equal(t, int32(1), e.GetToken().Position.Line)
	})

	t.Run("in a field, which the fields after it still fill", func(t *testing.T) {
		var tm team
		err := codec.Unmarshal([]byte("lead:\n  name: tim\n  age: 12\ntitle: core\n"), &tm, codec.Validator(checkAge))
		require.ErrorIs(t, err, yamlerrors.ErrValidation)
		require.ErrorIs(t, err, errTooYoung)
		assert.Equal(t, "core", tm.Title)

		var e *yamlerrors.Error
		require.ErrorAs(t, err, &e)
		assert.Equal(t, int32(2), e.GetToken().Position.Line)
	})
}

// missingFieldError reports a struct field the document left out, as a validator such as go-playground/validator does:
// an error that is a slice of field errors.
type missingFieldError struct{ field string }

func (e missingFieldError) StructField() string { return e.field }

type missingFieldErrors []missingFieldError

func (missingFieldErrors) Error() string { return "size is required" }

type sizedItem struct {
	Name string `yaml:"name"`
	Size int    `yaml:"size"`
}

func requireSize(v any) error {
	if it, ok := v.(sizedItem); ok && it.Size == 0 {
		return missingFieldErrors{{"Size"}}
	}

	return nil
}

// TestAMissingFieldIsReportedAtTheSamePlaceWithComments decodes a struct that leaves out a field the validator
// requires, and checks that the error position does not depend on CommentToMap.
//
// The parser keeps the "- " node of a sequence entry only when it reads comments, so the error once pointed at the
// "-" with CommentToMap and at the struct's first key without it.
func TestAMissingFieldIsReportedAtTheSamePlaceWithComments(t *testing.T) {
	for tc := range missingFieldCases() {
		for path := range decodePaths() {
			for _, comments := range []bool{false, true} {
				opts := append([]codec.DecodeOption{codec.Validator(requireSize)}, path.opts...)
				name := path.name + "/comments off"
				if comments {
					opts = append(opts, codec.CommentToMap(codec.CommentMap{}))
					name = path.name + "/comments on"
				}

				t.Run(tc.name+"/"+name, func(t *testing.T) {
					var v struct {
						Items []sizedItem `yaml:"items"`
						Item  sizedItem   `yaml:"item"`
					}
					err := codec.Unmarshal([]byte(tc.source), &v, opts...)
					require.ErrorIs(t, err, yamlerrors.ErrValidation)

					var e *yamlerrors.Error
					require.ErrorAs(t, err, &e)
					assert.Equal(t, tc.line, e.GetToken().Position.Line)
					assert.Equal(t, tc.column, e.GetToken().Position.Column)
				})
			}
		}
	}
}

// missingFieldCase holds a document that leaves out a required field, and where the error points.
type missingFieldCase struct {
	name         string
	source       string
	line, column int32
}

func missingFieldCases() iter.Seq[missingFieldCase] {
	return slices.Values([]missingFieldCase{
		{
			name:   "in a sequence entry, at the first key",
			source: "items:\n  - name: a\n",
			line:   2, column: 5,
		},
		{
			name:   "in a mapping entry, at its key",
			source: "item:\n  name: a\n",
			line:   1, column: 1,
		},
	})
}
