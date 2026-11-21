// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

func TestStructValidator(t *testing.T) {
	type Inner struct {
		Required string `validate:"required"`
		Lt10     int    `validate:"lt=10"`
	}

	cases := []struct {
		TestName    string
		YAMLContent string
		ExpectedErr string
		Instance    any
	}{
		{
			TestName: "Test Simple Validation",
			YAMLContent: `---
- name: john
  age: 20
- name: tom
  age: -1
- name: ken
  age: 10`,
			ExpectedErr: `[5:8] Key: 'Age' Error:Field validation for 'Age' failed on the 'gte' tag
   2 | - name: john
   3 |   age: 20
   4 | - name: tom
>  5 |   age: -1
              ^
   6 | - name: ken
   7 |   age: 10`,
			Instance: &[]struct {
				Name string `validate:"required"     yaml:"name"`
				Age  int    `validate:"gte=0,lt=120" yaml:"age"`
			}{},
		},
		{
			TestName: "Test Missing Required Field",
			YAMLContent: `---
- name: john
  age: 20
- age: 10`,
			// The caret stands on the entry's first key rather than on its "-".
			// A parse that was not asked for comments builds no
			// ast.SequenceEntryNode, so the sequence keeps no "-" to point at
			// and codec.missingFieldToken falls back to the first key.
			ExpectedErr: `[4:3] Key: 'Name' Error:Field validation for 'Name' failed on the 'required' tag
   1 | ---
   2 | - name: john
   3 |   age: 20
>  4 | - age: 10
         ^
`,
			Instance: &[]struct {
				Name string `validate:"required"     yaml:"name"`
				Age  int    `validate:"gte=0,lt=120" yaml:"age"`
			}{},
		},
		{
			TestName: "Test Nested Validation Missing Internal Required",
			YAMLContent: `---
name: john
age: 10
addr:
  number: seven`,
			ExpectedErr: `[4:5] Key: 'State' Error:Field validation for 'State' failed on the 'required' tag
   1 | ---
   2 | name: john
   3 | age: 10
>  4 | addr:
           ^
   5 |   number: seven`,
			Instance: &struct {
				Name string `validate:"required"     yaml:"name"`
				Age  int    `validate:"gte=0,lt=120" yaml:"age"`
				Addr struct {
					Number string `validate:"required" yaml:"number"`
					State  string `validate:"required" yaml:"state"`
				} `yaml:"addr"`
			}{},
		},
		{
			TestName: "Test nested Validation with unknown field",
			YAMLContent: `---
name: john
age: 20
addr:
  number: seven
  state: washington
  error: error
`,
			ExpectedErr: `[7:3] unknown field "error"
   4 | addr:
   5 |   number: seven
   6 |   state: washington
>  7 |   error: error
         ^
`,
			Instance: &struct {
				Name string `validate:"required"     yaml:"name"`
				Age  int    `validate:"gte=0,lt=120" yaml:"age"`
				Addr *struct {
					Number string `validate:"required" yaml:"number"`
					State  string `validate:"required" yaml:"state"`
				} `validate:"required" yaml:"addr"`
			}{},
		},
		{
			TestName: "Test Validation with wrong field type",
			YAMLContent: `---
name: myDocument
roles:
  name: myRole
  permissions:
    - hello
    - how
    - are
    - you
`,
			ExpectedErr: `[4:7] mapping was used where sequence is expected
   1 | ---
   2 | name: myDocument
   3 | roles:
>  4 |   name: myRole
             ^
   5 |   permissions:
   6 |     - hello
   7 |     - how`,
			Instance: &struct {
				Name  string `yaml:"name"`
				Roles []struct {
					Name        string   `yaml:"name"`
					Permissions []string `yaml:"permissions"`
				} `yaml:"roles"`
			}{},
		},
		{
			TestName: "Test inline validation missing required",
			YAMLContent: `---
name: john
age: 20
`,
			// The field is missing, so there is no entry to point at and the
			// error goes to the mapping that should have held it. A field
			// promoted from Inner is reported the same way as one of the outer
			// struct's own.
			ExpectedErr: `[2:1] Key: 'Inner.Required' Error:Field validation for 'Required' failed on the 'required' tag
   1 | ---
>  2 | name: john
       ^
   3 | age: 20`,
			Instance: &struct {
				Name  string           `validate:"required"     yaml:"name"`
				Age   int              `validate:"gte=0,lt=120" yaml:"age"`
				Inner `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows the regular ones on purpose: the fixture covers that order
			}{},
		},
		{
			TestName: "Test inline validation field error",
			YAMLContent: `---
name: john
age: 20
required: present
lt10: 20
`,
			ExpectedErr: `[5:7] Key: 'Inner.Lt10' Error:Field validation for 'Lt10' failed on the 'lt' tag
   2 | name: john
   3 | age: 20
   4 | required: present
>  5 | lt10: 20
             ^
`,
			Instance: &struct {
				Name  string           `validate:"required"     yaml:"name"`
				Age   int              `validate:"gte=0,lt=120" yaml:"age"`
				Inner `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows the regular ones on purpose: the fixture covers that order
			}{},
		},
		{
			TestName: "Test map",
			YAMLContent: `
map:
  hello: hello
  hello2: hello`,
			ExpectedErr: `[2:4] Key: 'Map' Error:Field validation for 'Map' failed on the 'eq' tag
>  2 | map:
          ^
   3 |   hello: hello
   4 |   hello2: hello`,
			Instance: &struct {
				// Make sure that this map is not set
				Map map[string]string `validate:"eq=0" yaml:"map"`
			}{},
		},
		{
			TestName: "Test sequence",
			YAMLContent: `
map:
  - value
`,
			ExpectedErr: `[2:4] Key: 'Map' Error:Field validation for 'Map' failed on the 'eq' tag
>  2 | map:
          ^
   3 |   - value`,
			Instance: &struct {
				// Make sure that this map is not set
				Map []string `validate:"eq=0" yaml:"map"`
			}{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.TestName, func(t *testing.T) {
			validate := validator.New()
			dec := codec.NewDecoder(
				strings.NewReader(tc.YAMLContent),
				codec.Validator(validate.Struct),
				codec.Strict(),
			)
			err := dec.Decode(tc.Instance)
			switch {
			case tc.ExpectedErr != "" && err == nil:
				t.Fatal("expected error")
			case tc.ExpectedErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.ExpectedErr != "" && tc.ExpectedErr != err.Error():
				t.Fatalf("expected `%s` but actual `%s`", tc.ExpectedErr, err.Error())
			}
		})
	}
}

// TestAValidationErrorWrapsTheValidatorsOwn decodes a struct go-playground/validator refuses. The error carries
// yamlerrors.ErrValidation, and errors.As reaches the validator's ValidationErrors through it.
func TestAValidationErrorWrapsTheValidatorsOwn(t *testing.T) {
	type person struct {
		Age int `validate:"gte=0" yaml:"age"`
	}

	var p person
	err := codec.Unmarshal([]byte("age: -1\n"), &p, codec.Validator(validator.New().Struct))
	if !errors.Is(err, yamlerrors.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	var fields validator.ValidationErrors
	if !errors.As(err, &fields) || len(fields) != 1 || fields[0].StructField() != "Age" {
		t.Fatalf("want the validator's ValidationErrors naming Age, got %v", err)
	}
}
