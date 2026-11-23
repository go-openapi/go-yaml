// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/transform"
)

// TestTrimmedAndFillerSplitTheText cuts the blanks and line breaks off the end of a piece's text: Trimmed returns
// what stands before them and Filler the blanks, so the two together give the text back.
func TestTrimmedAndFillerSplitTheText(t *testing.T) {
	for text, want := range map[string][2]string{
		"2  ":   {"2", "  "},
		"# c\n": {"# c", "\n"},
		"x\r\n": {"x", "\r\n"},
		"a\tb":  {"a\tb", ""},
		" \n\t": {"", " \n\t"},
		"":      {"", ""},
	} {
		p := transform.Piece{Text: []byte(text)}
		assert.Equalf(t, want[0], string(p.Trimmed()), "Trimmed(%q)", text)
		assert.Equalf(t, want[1], string(p.Filler()), "Filler(%q)", text)
	}
}

// TestEveryRoleHasAName requires a distinct name for each Role.
func TestEveryRoleHasAName(t *testing.T) {
	seen := map[string]transform.Role{}
	for r := transform.RoleFill; r <= transform.RoleText; r++ {
		name := r.String()
		assert.NotEmptyf(t, name, "%d", r)
		if prev, dup := seen[name]; dup {
			t.Errorf("%d and %d are both %q", prev, r, name)
		}
		seen[name] = r
	}
}
