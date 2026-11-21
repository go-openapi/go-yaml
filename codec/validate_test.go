// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
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
