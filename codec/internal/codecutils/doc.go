// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package codecutils holds the codec's helpers that read and convert Go values.
//
// They are unaware of the codec's own types, and only read AST nodes to place an error.
//
// The helpers come in five groups:
//
//   - Destinations. [SetDecoded], [SetZero], [FieldAt], [HoldsAnything], [IsPointer], [PointedKind] and
//     [DynamicTypeOf] inspect and set by reflection the destination of a decode. [ByteArray], [ByteSlice] and
//     [CopyIntoByteArray] fill a byte destination from a "!!binary".
//   - Conversions. [ConvertToKind], [CastToAssignableValue] and [CastToFloatValue] convert a decoded value to its
//     destination's type. [IsBigNumber] and [ConvertBigNumber] handle a number no native type holds.
//   - Map keys. [KeyIDOf], [SameMapKey], [SameKeyIn] and [ComparesByValue] decide when two decoded keys are one YAML
//     key, which Go's == misses for a timestamp in two zones or a wide number. [HashableKey] and [UnusableKey] reject
//     a key a Go map cannot hold, and a key with a Canonical method is named by what that method returns.
//   - Building an any. [KeyedMap] builds a mapping as a map[string]any and widens it to map[any]any on the first key
//     that is not a string. [Arena] copies decoded strings out of the document in bulk, so a decoded value never pins
//     the source.
//   - The rest. [AliasBudget] caps what aliases may build from a document of n bytes. [ReadAll] reads a source
//     whole. [QuoteWith] and [AppendPointerToken] quote and escape text. [NamesOrderedMap] and [NamesScalarType]
//     read a tag. [CustomArg] passes a value to a registered custom (un)marshaler.
package codecutils
