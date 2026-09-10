// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package probe

// ReuseReleased is false here: a released chunk is kept aside rather than
// filled again, so a cell read after it went back still carries the stamp and
// every read is reported, not only the ones that beat the reuse.
const ReuseReleased = false
