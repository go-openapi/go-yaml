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

// TestASuiteIsNamedWithForwardSlashes covers the name a fixture's suite takes, which golden files and digests record.
//
// The path is built with filepath.Join, so on Windows it holds "\\" and this is the case that broke there: the
// suite was named after the fixture's whole path.
func TestASuiteIsNamedWithForwardSlashes(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("some", "testdata")
	for path, want := range map[string]string{
		filepath.Join(dir, "aliases-in-block-sequence", "in.yaml"):            "aliases-in-block-sequence",
		filepath.Join(dir, "trailing-whitespace-in-streams", "02", "in.json"): "trailing-whitespace-in-streams/02",
	} {
		require.Equal(t, want, suiteName(dir, path), path)
	}
}
