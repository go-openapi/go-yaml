// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package ledgers holds the ratchet the defect ledgers are compared with.
//
// A ledger records the defects a package is known to produce, keyed by the case or the invariant that shows them.
// [Compare] holds a fresh measurement against one, in both directions: a defect that is not recorded fails, and a
// recorded defect that no longer shows fails too, so a fix is landed by deleting the entry.
// [CompareNames] does the same for a ledger whose entries record why a case diverges, comparing the names only.
//
// One subdirectory per package under measurement, holding nothing but its test files. Nothing in the library imports
// them, so this tree can be given its own go.mod when its dependencies need separating from the library's.
//
// See the README for what belongs in a ledger and how to move one here.
package ledgers
