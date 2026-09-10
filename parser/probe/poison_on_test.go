// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package probe_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/parser/probe"
)

// TestReuseReleasedIsOffUnderTheTag pins what the probe build is for: a
// released chunk is kept aside, so a cell read after it went back still carries
// the stamp and the read is reported.
//
// ⚠️ No CI job runs "go test -tags yamlprobe", so this only fails for whoever
// asks for it. See the default half in poison_test.go.
func TestReuseReleasedIsOffUnderTheTag(t *testing.T) {
	t.Parallel()

	assert.False(t, probe.ReuseReleased,
		"the yamlprobe build keeps released chunks aside so every stale read is reported",
	)
}
