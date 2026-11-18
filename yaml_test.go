// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// The tests here cover the four functions of the top-level package. Their behaviour is tested in the package that
// implements it: codec for decoding and encoding, parser and internal/scanner for reading a document, conformance for
// the YAML Test Suite.

func TestMarshalAndUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	in := server{Host: "localhost", Port: 8080, Tags: []string{"api"}, Debug: true}

	out, err := yaml.Marshal(in)
	require.NoError(t, err)

	var back server
	require.NoError(t, yaml.Unmarshal(out, &back))
	assert.Equal(t, in, back)
}

func TestUnmarshalIntoAMap(t *testing.T) {
	t.Parallel()

	var m map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("a: 1\nb: [x, y]\nc: {d: true}\n"), &m))
	assert.Equal(t, map[string]any{
		"a": uint64(1),
		"b": []any{"x", "y"},
		"c": map[string]any{"d": true},
	}, m)
}

func TestUnmarshalReturnsASyntaxError(t *testing.T) {
	t.Parallel()

	var m map[string]any
	err := yaml.Unmarshal([]byte("a: [1, 2\n"), &m)
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
}

func TestToJSONAndFromJSONRoundTrip(t *testing.T) {
	t.Parallel()

	const doc = "a: 1\nb:\n- p\n- q\n"

	j, err := yaml.ToJSON([]byte(doc))
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1,"b":["p","q"]}`, string(j))

	y, err := yaml.FromJSON(j)
	require.NoError(t, err)
	assert.Equal(t, doc, string(y))
}

func TestToJSONReturnsASyntaxError(t *testing.T) {
	t.Parallel()

	_, err := yaml.ToJSON([]byte("a: [1, 2\n"))
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
}
