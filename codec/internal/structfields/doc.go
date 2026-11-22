// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package structfields reads the fields a Go struct type shows to the codec: the name each is written under, its
// `yaml` flags, and the fields an embedded or `,inline` struct promotes.
//
// [Mode] selects the tags read: `yaml` alone, as go.yaml.in/yaml/v3 does, or with encoding/json's rules added for
// [github.com/go-openapi/go-yaml/codec.UseJSONTags] and [github.com/go-openapi/go-yaml/codec.UseInferredNames].
//
// [Of] returns a type's [Fields]: each field by Go name, the field a mapping key names, and the fields its embedded
// structs promote. [MapOf] returns the fields by Go name alone. Both read a type once per mode and keep the result
// in a [Cache].
//
// The codec calls [SelfDecoding] once, at init, with its unmarshaler interfaces. An `,inline` field whose type
// implements one decodes its own node; see [InlineReadsItsOwnNode].
package structfields
