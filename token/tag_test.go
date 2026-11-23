// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestReservedTagOfNamesOnlyTheYAMLNamespace maps a tag URI to its "!!" shorthand, and rejects a URI outside
// tag:yaml.org,2002 or a name the namespace does not reserve.
func TestReservedTagOfNamesOnlyTheYAMLNamespace(t *testing.T) {
	for uri, want := range map[string]token.ReservedTagKeyword{
		"tag:yaml.org,2002:int":   token.IntegerTag,
		"tag:yaml.org,2002:omap":  token.OrderedMapTag,
		"tag:yaml.org,2002:merge": token.MergeTag,
	} {
		got, ok := token.ReservedTagOf(uri)
		assert.Truef(t, ok, "%q", uri)
		assert.Equalf(t, want, got, "%q", uri)
	}
	for _, uri := range []string{"tag:yaml.org,2002:foo", "!int", "tag:example.com,2000:int", "", "!!int"} {
		_, ok := token.ReservedTagOf(uri)
		assert.Falsef(t, ok, "%q", uri)
	}
}
