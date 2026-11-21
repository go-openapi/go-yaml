// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml

import "github.com/go-openapi/go-yaml/codec"

// Marshal serializes a go value v into a YAML document.
//
// See [codec.Marshal] for how a Go value is mapped onto YAML and what the
// struct tags mean.
func Marshal(v any) ([]byte, error) {
	return codec.Marshal(v)
}

// Unmarshal decodes a YAML document into the go value pointed to by v.
//
// See [codec.Unmarshal] for how a YAML document is mapped onto a Go value.
func Unmarshal(data []byte, v any) error {
	return codec.Unmarshal(data, v)
}

// ToJSON converts a YAML document to the JSON that holds the same values.
//
// See [codec.ToJSON] for advanced usage with options.
func ToJSON(bytes []byte) ([]byte, error) {
	return codec.ToJSON(bytes)
}

// FromJSON converts a JSON document to the YAML that holds the same values, and
// rejects input that is not JSON.
//
// See [codec.FromJSON] for what is written, and for the options that lay it out.
func FromJSON(bytes []byte) ([]byte, error) {
	return codec.FromJSON(bytes)
}
