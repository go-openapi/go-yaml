// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamltestsuite

import (
	"path/filepath"
	"testing"

	"github.com/go-openapi/testify/v2/require"
)

// TestSuitesInAMissingDirectoryReturnsTheError covers the walk callback, which
// filepath.Walk calls with a nil info when it cannot read a path.
//
// The callback asked info.IsDir() before it looked at the error, so a missing
// directory panicked instead of returning the error.
func TestSuitesInAMissingDirectoryReturnsTheError(t *testing.T) {
	t.Parallel()

	suites, err := suitesIn(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	require.Nil(t, suites)
}
