// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package colorize_test

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/transform"
	"github.com/go-openapi/go-yaml/transform/colorize"
)

var escapes = regexp.MustCompile("\x1b\\[[0-9;]*m")

const sample = `# a specification
%YAML 1.2
---
openapi: "3.0.0"
info:
  title: &name Pet store   # the name
  version: 1.2
  retired: false
  contact: ~
paths:
  /pets:
    get:
      tags: [pets, list]
      summary: *name
      description: |
        Returns every pet.
      responses:
        "200":
          schema: !!map {}
...
`

// TestColoringChangesNothingButTheEscapes checks that stripping the escapes out
// of a colored document gives the source back.
//
// It is the property that says the colorizer is a decoration rather than a
// renderer: it never re-spells a scalar, moves an indent or drops a comment.
func TestColoringChangesNothingButTheEscapes(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, transform.Walk(&out, []byte(sample), colorize.New(colorize.Default())))
	require.Equal(t, sample, escapes.ReplaceAllString(out.String(), ""))
	require.NotEqual(t, sample, out.String())
}

// TestZeroThemeWritesTheSource checks that a theme naming no style is the
// identity transform.
func TestZeroThemeWritesTheSource(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, transform.Walk(&out, []byte(sample), colorize.New(colorize.Theme{})))
	require.Equal(t, sample, out.String())
}

// TestAScalarIsDrawnByWhatTheParseResolved checks the whole reason a colorizer
// walks the parse rather than the tokens: a quoted "1" is a string and a plain 1
// is a number, and only the parse tells them apart.
func TestAScalarIsDrawnByWhatTheParseResolved(t *testing.T) {
	theme := colorize.Theme{
		Integer: colorize.Style{Prefix: "<int>", Suffix: "</int>"},
		String:  colorize.Style{Prefix: "<str>", Suffix: "</str>"},
		Bool:    colorize.Style{Prefix: "<bool>", Suffix: "</bool>"},
		Key:     colorize.Style{Prefix: "<key>", Suffix: "</key>"},
	}

	var out bytes.Buffer
	require.NoError(t, transform.Walk(&out, []byte("a: 1\nb: \"1\"\nc: yes\n"), colorize.New(theme)))
	require.Equal(t,
		"<key>a</key>: <int>1</int>\n<key>b</key>: <str>\"1\"</str>\n<key>c</key>: <str>yes</str>\n",
		out.String())
}
