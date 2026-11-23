// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
)

func TestIsNeedQuoted(t *testing.T) {
	needQuotedTests := []string{
		"",
		"true",
		"1.234",
		"0b11111111111111111111111111111111111111111111111111111111111111111",
		"0o7777777777777777777777777777777777777777",
		"999999999999999999999999999999999999999999",
		"0xffffffffffffffffffffffffffffffffffffffff",
		"1:1",
		"2001-12-15T02:59:43.1Z",
		"2001-12-14t21:59:43.10-05:00",
		"2001-12-15 2:59:43.10",
		"2002-12-14",
		"hoge # comment",
		"\\0",
		"#a b",
		"*a b",
		"&a b",
		"{a b",
		"}a b",
		"[a b",
		"]a b",
		",a b",
		"!a b",
		"|a b",
		">a b",
		">a b",
		"%a b",
		`'a b`,
		`"a b`,
		"a:",
		"a: b",
		"y",
		"Y",
		"yes",
		"Yes",
		"YES",
		"n",
		"N",
		"no",
		"No",
		"NO",
		"on",
		"On",
		"ON",
		"off",
		"Off",
		"OFF",
		"@test",
		":0",
		":8080",
		":value",
		" a",
		" a ",
		"a ",
		"null",
		"Null",
		"NULL",
		"~",
		"-",
		"- --foo",
		// A document marker, where the scalar starts its line.
		"---",
		"...",
		"... x",
		"---\tx",
		"...\tx",
		// The spaced example of https://yaml.org/type/timestamp.html, a timestamp under YAML 1.1.
		"2001-12-14 21:59:43.10 -5",
	}
	for _, test := range needQuotedTests {
		assert.Truef(t, token.IsNeedQuoted(test), "expected %q to need quoting", test)
	}

	notNeedQuotedTests := []string{
		"Hello World",
		// A date alone takes two-digit month and day fields: https://yaml.org/type/timestamp.html
		"2001-1-2",
	}
	for _, test := range notNeedQuotedTests {
		assert.Falsef(t, token.IsNeedQuoted(test), "expected %q not to need quoting", test)
	}
}
