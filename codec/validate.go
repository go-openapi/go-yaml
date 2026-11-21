// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

// fieldError is an element of a validation error that names the struct field it is about, as each
// github.com/go-playground/validator/v10 FieldError does. See [Validator].
type fieldError interface {
	StructField() string
}
