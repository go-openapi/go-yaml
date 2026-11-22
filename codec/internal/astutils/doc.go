// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package astutils holds the codec's helpers that read or edit an [ast] tree.
//
// These helpers are unaware of the codec's own types.
//
// The helpers come in five groups:
//
//   - Keys. [MapKeyString] names a decoded mapping key, [KeyText] names a scalar key for a JSON object,
//     and [StandsOnNothing] finds a key with no node under it.
//   - Entries. [EachEntryOwnFirst] and [MapEntriesOwnFirst] return a mapping's own entries before the ones a "<<"
//     merges in, and [EachMergedEntry] reads the merged ones. A reader that fills a field only once then gives an
//     own key precedence over a merged one.
//   - Repeated keys. The parser records a repeated key and rejects nothing. [RefuseDuplicateKeys], [RefuseRepeats]
//     and [RefuseOrderedMapDuplicates] return an error for the first repeat it recorded, and [NewAllowedRepeat] and
//     [AllowedRepeatEntries] find the repeats [github.com/go-openapi/go-yaml/codec.AllowDuplicateMapKey] lets through.
//   - Comments. [EachComment] reads a tree's comments for [github.com/go-openapi/go-yaml/codec.CommentToMap], and
//     [PlaceHeadComment], [PlaceLineComment] and [PlaceFootComment] write them back for
//     [github.com/go-openapi/go-yaml/codec.WithComment].
//   - Values and positions. [JSONScalarOf], [ScalarNumber] and [AnchorName] read a node. [NodeAt],
//     [ValidationErrorToken] and [SequenceEntryNode] find the source position an error points at. [CountNodes] sizes
//     the alias budget of a tree decoded without its source.
//
// A question about what a piece of YAML means usually belongs in package ast itself, e.g [ast.Unwrap], [ast.IsNull], [ast.OrderedMapOf].
// We add a helper here when it answers a question that only the codec currently asks.
package astutils
