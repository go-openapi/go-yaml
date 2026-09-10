// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !yamlprobe

package probe

// ReuseReleased says a chunk the grouper handed back may be filled again, which
// is the whole point of handing it back. A probe build keeps released chunks
// aside instead, so that a cell read after it went back still holds what it did
// and the read is reported rather than the document being misparsed.
const ReuseReleased = true
