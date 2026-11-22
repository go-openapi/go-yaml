// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package codec converts between YAML documents and Go values, with options.
//
// [Marshal] and [Unmarshal] convert one document. [NewEncoder] and [NewDecoder] write and read a stream of documents,
// one for each call to [Encoder.Encode] or [Decoder.Decode]. [ValueToNode] and [NodeToValue] convert between a Go value
// and an [github.com/go-openapi/go-yaml/ast] tree.
//
// These functions take options: an [EncodeOption] or a [DecodeOption] sets them for one call or one stream.
// The root package's Marshal and Unmarshal take none.
//
// # Struct fields
//
// A struct maps to a mapping of its exported fields. A field is named after its `yaml` tag, or after its Go name
// lowercased, as in go.yaml.in/yaml/v3. [Marshal] lists the tag's flags.
//
// Three option pairs read and write struct fields as encoding/json does:
//
//   - [UseJSONTags] and [WriteJSONTags] read the `json` tag where a field has no `yaml` tag, and promote the fields of
//     an untagged embedded struct.
//   - [UseInferredNames] and [WriteInferredNames] name an untagged field after its Go name, verbatim.
//   - [OmitEmpty] skips the fields encoding/json's omitempty skips, and [OmitZero] the fields its omitzero skips.
//
// # Types that encode or decode themselves
//
// A type implements [Marshaler] to return its YAML text, and [Unmarshaler] to receive it. [NodeUnmarshaler] receives
// the [ast.Node] instead, with its comments and positions. [GoYAMLMarshaler] and [GoYAMLUnmarshaler] have the shapes
// github.com/go-yaml/yaml expects, so that types written for that library work unchanged.
//
// To change how a type you do not own is encoded or decoded, pass [CustomMarshaler] or [CustomUnmarshaler], or call
// [RegisterCustomMarshaler] or [RegisterCustomUnmarshaler] once for every encoder and decoder.
//
// # Values with no Go equivalent
//
// A Go map loses the order of a mapping, and cannot tell apart some keys that YAML holds apart. [MapSlice] keeps both,
// and [UseOrderedMap] decodes every untyped mapping into one. [MapSliceSeq] reads and writes the "!!omap" tag.
// [Base64] holds a "!!binary" scalar, and [RawMessage] holds an encoded value whose decoding waits.
//
// A Go value has no place for a comment. [CommentToMap] collects a document's comments into a [CommentMap] while
// decoding, and [WithComment] writes them back while encoding.
//
// # JSON
//
// [ToJSON] converts a YAML document to the JSON holding the same values, and [ToJSONTokens] hands the same conversion
// over token by token. [FromJSON] converts JSON to YAML. None of them goes through a Go value, so a number keeps its
// digits and a mapping its order.
//
// # Errors
//
// Errors carry a position in the document, and match the kinds the [github.com/go-openapi/go-yaml/errors] package
// declares, such as errors.ErrDuplicateKey or errors.ErrTypeMismatch, through [errors.Is].
package codec
