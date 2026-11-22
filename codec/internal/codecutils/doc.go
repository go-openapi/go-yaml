// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package codecutils holds the codec's helpers that read and convert Go values: reflection on destinations,
// numbers that do not fit a machine word, the identity of a map key, the alias budget, reading a source whole.
//
// They know nothing of the codec's own types. The helpers that name a key by its Base64 text stay in the codec.
package codecutils
