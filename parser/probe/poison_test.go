// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !yamlprobe

package probe_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/parser/probe"
)

// TestReuseReleasedIsOnWithoutTheTag pins the default: a normal build fills a
// released chunk again.
//
// The two halves of this guard live in different builds and cannot check each
// other, so each states its own value. See the yamlprobe half in
// poison_on_test.go. Turning the probe on for everyone would cost the reuse
// that [arena.Run] exists for -- the parser would allocate a fresh chunk per
// mapping run -- so the default is worth failing on rather than reading.
func TestReuseReleasedIsOnWithoutTheTag(t *testing.T) {
	t.Parallel()

	assert.True(t, probe.ReuseReleased,
		"a build with no yamlprobe tag fills released chunks again",
	)
}
