// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package ledgers provides the two ratchets the defect ledgers are checked with.
//
// A ledger records the defects a package is known to produce, keyed by the case or the invariant that shows them.
// [Compare] checks a fresh measurement against one, in both directions: a defect that is not recorded fails, and a
// recorded defect that no longer shows fails too, so a fix lands by deleting the entry.
// [CompareNames] does the same for a ledger whose entries record why a case diverges, comparing the names only.
//
// One subdirectory per package under measurement (scanner, parser), and one for the YAML Test Suite harnesses
// (conformance), each holding nothing but test files. Nothing in the library imports them.
//
// See the README for every ledger, what each measures, and how to re-baseline one.
package ledgers
