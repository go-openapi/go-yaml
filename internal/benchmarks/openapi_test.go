// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package benchmarks

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/spec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

func azureSrc(tb testing.TB) []byte {
	tb.Helper()

	all, err := workloads.All()
	require.NoError(tb, err)
	for _, w := range all {
		if w.Name == "azure_swagger" {
			return w.Data
		}
	}
	tb.Fatal("azure_swagger is not among the workloads")

	return nil
}

// countSpecs totals what a decoded bundle holds, which is how one route's
// result is compared with another's.
func countSpecs(m map[string]spec.Swagger) (specs, paths, definitions int) {
	for _, sw := range m {
		specs++
		if sw.Paths != nil {
			paths += len(sw.Paths.Paths)
		}
		definitions += len(sw.Definitions)
	}

	return specs, paths, definitions
}

// TestReadingAnOpenAPISpecification records which options fill
// github.com/go-openapi/spec from YAML, and which do not.
//
// azure_swagger is a bundle of fifteen Azure specifications keyed by filename,
// so the destination is a map of them. spec's types carry `json` tags alone and
// embed without tagging, which is what codec.UseJSONTags reads; several of them
// are also union types -- spec.StringOrArray takes a string or a sequence, and
// only its UnmarshalJSON knows that -- which no tag can express.
//
// So the option is necessary and not sufficient here, and once
// UseJSONUnmarshaler is added spec.Swagger's own UnmarshalJSON takes the whole
// document through JSON and the tags stop being consulted at all. Reading an
// OpenAPI specification is a JSON conversion, whichever route is taken.
func TestReadingAnOpenAPISpecification(t *testing.T) {
	src := azureSrc(t)

	// The route go-openapi takes today, and the yardstick for the rest.
	jsonBytes, err := yaml.ToJSON(src)
	require.NoError(t, err)
	var viaJSON map[string]spec.Swagger
	require.NoError(t, json.Unmarshal(jsonBytes, &viaJSON))
	wantSpecs, wantPaths, wantDefs := countSpecs(viaJSON)
	require.Positive(t, wantPaths, "the yardstick decoded nothing")

	t.Run("the default mode fills nothing", func(t *testing.T) {
		var v map[string]spec.Swagger
		require.NoError(t, yaml.Unmarshal(src, &v))

		specs, paths, defs := countSpecs(v)
		assert.Equal(t, wantSpecs, specs, "the outer map is filled")
		assert.Zero(t, paths, "and every specification in it is left zero")
		assert.Zero(t, defs)
	})

	t.Run("UseJSONTags reaches the fields and stops at a union type", func(t *testing.T) {
		var v map[string]spec.Swagger
		err := codec.UnmarshalWithOptions(src, &v, codec.UseJSONTags(true))

		// It fails deep inside definitions, on spec.StringOrArray: "type:
		// string" against a []string. Reaching that far is the option working.
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sequence is expected")
	})

	t.Run("with UseJSONUnmarshaler it reads the document", func(t *testing.T) {
		var v map[string]spec.Swagger
		require.NoError(t, codec.UnmarshalWithOptions(src, &v,
			codec.UseJSONTags(true), codec.UseJSONUnmarshaler()))

		specs, paths, defs := countSpecs(v)
		assert.Equal(t, wantSpecs, specs)
		assert.Equal(t, wantPaths, paths)
		assert.Equal(t, wantDefs, defs)
	})

	t.Run("UseJSONUnmarshaler alone reads it too", func(t *testing.T) {
		// spec.Swagger implements UnmarshalJSON, so each specification is
		// handed over as JSON whole and no tag of it is ever read. UseJSONTags
		// adds nothing to this type, and would to one without the method.
		var v map[string]spec.Swagger
		require.NoError(t, codec.UnmarshalWithOptions(src, &v, codec.UseJSONUnmarshaler()))

		specs, paths, defs := countSpecs(v)
		assert.Equal(t, wantSpecs, specs)
		assert.Equal(t, wantPaths, paths)
		assert.Equal(t, wantDefs, defs)
	})
}
