// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestRenderAllocatesInProportionToTheSource bounds what Renderer.Render allocates on a deeply nested document.
//
// Renderer.String once built each child's text in full and indented it line by line, so every byte was copied once
// per level above it: a document nested 1280 deep, 1.6 MB of YAML, rendered in 0.98 s and allocated 2.9 GB (1757x its
// size). It renders in about 6 ms and allocates about 1.7x its size since afcd8843. The ceiling of 8x leaves room for
// noise and fails long before the quadratic shape comes back.
func TestRenderAllocatesInProportionToTheSource(t *testing.T) {
	const (
		depth   = 1280
		ceiling = 8.0
	)

	var b strings.Builder
	for i := range depth {
		b.WriteString(strings.Repeat("  ", i))
		fmt.Fprintf(&b, "k%d:\n", i)
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("v\n")
	src := []byte(b.String())

	f, err := parser.ParseBytes(src)
	require.NoError(t, err)

	r := ast.NewRenderer()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	for _, doc := range f.Docs {
		require.NoError(t, r.Render(io.Discard, doc))
	}

	runtime.ReadMemStats(&after)

	allocated := after.TotalAlloc - before.TotalAlloc
	ratio := float64(allocated) / float64(len(src))
	t.Logf("depth %d, %d bytes of YAML, %d bytes allocated (%.1fx)", depth, len(src), allocated, ratio)

	require.LessOrEqualf(t, ratio, ceiling,
		"rendering %d bytes nested %d deep allocated %d bytes, %.1fx its size", len(src), depth, allocated, ratio)
}
