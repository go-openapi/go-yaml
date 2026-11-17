// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

// StructValidator need to implement Struct method only
// ( see https://pkg.go.dev/github.com/go-playground/validator/v10#Validate.Struct ).
type StructValidator interface {
	Struct(value any) error
}

// FieldError need to implement StructField method only
// ( see https://pkg.go.dev/github.com/go-playground/validator/v10#FieldError ).
type FieldError interface {
	StructField() string
}
