// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec/internal/structfields"
)

// TestAPlainKeyNeedsNoQuotesUnderAnyOption checks fieldPlan.plainKey against encodeString, which encodeStruct calls
// for every other key: a name planned as plain comes back unquoted under each combination of the quoting options but
// JSON, and a name that takes quotes under one of them is not planned as plain.
func TestAPlainKeyNeedsNoQuotesUnderAnyOption(t *testing.T) {
	type keys struct {
		Plain     int `yaml:"plain"`
		Dashed    int `yaml:"with-dash"`
		Spaced    int `yaml:"two words"`
		Colon     int `yaml:"a:b"`
		URL       int `yaml:"http://x"`
		Bracket   int `yaml:"a]b"`
		Brace     int `yaml:"a}b"`
		Quote     int `yaml:"it's"`
		DQuote    int `yaml:"say \"hi\""`
		Hash      int `yaml:"a #b"`
		Bool      int `yaml:"yes"`
		Null      int `yaml:"null"`
		Number    int `yaml:"12"`
		Indicator int `yaml:"-a"`
		Unicode   int `yaml:"clé"`
	}
	plan, err := planOf(reflect.TypeFor[keys](), structfields.YAMLTags)
	require.NoError(t, err)
	require.Len(t, plan.write, reflect.TypeFor[keys]().NumField())

	var plain int
	for _, fp := range plan.write {
		name := fp.info.RenderName
		quotedSomewhere := false
		for mask := range 16 {
			e := &Encoder{
				isFlowStyle:                mask&1 != 0,
				useLiteralStyleIfMultiline: mask&2 != 0,
				singleQuote:                mask&4 != 0,
			}
			if got := e.encodeString(name, 0).Value; got != name {
				quotedSomewhere = true
				assert.Falsef(t, fp.plainKey, "%q is planned plain, but encodeString writes %q under %+v", name, got, e)
			}
		}
		if fp.plainKey {
			plain++
		}
		assert.Equalf(t, !quotedSomewhere, fp.plainKey || name == "http://x",
			"%q: plainKey %v, quoted under some option %v", name, fp.plainKey, quotedSomewhere)
	}
	assert.Positive(t, plain)
}
