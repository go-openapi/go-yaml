// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_DecodeFromNode(t *testing.T) {
	t.Run("has reference", func(t *testing.T) {
		str := `
anchor: &map
  text: hello
map: *map`
		var buf bytes.Buffer
		dec := codec.NewDecoder(&buf)
		f, err := parser.ParseBytes([]byte(str))
		require.NoError(t, err, "parse")
		type T struct {
			Map map[string]string
		}
		var v T
		require.NoError(t, dec.DecodeFromNode(f.Docs[0].Body, &v), "decode")
		actual := fmt.Sprintf("%+v", v)
		expect := fmt.Sprintf("%+v", T{map[string]string{"text": "hello"}})
		require.Equal(t, expect, actual)
	})
	t.Run("with reference option", func(t *testing.T) {
		const reference = `
map: &map
  text: hello`
		var buf bytes.Buffer
		dec := codec.NewDecoder(&buf, codec.ReferenceReaders(strings.NewReader(reference)))

		// "map: *map" names an anchor it does not declare, so the parse that
		// builds the node has to be told where that anchor comes from. The
		// decoder reads the reference for the value; the parser reads it for
		// the name.
		ref, err := parser.ParseBytes([]byte(reference))
		require.NoError(t, err, "parse the reference")
		f, err := parser.ParseBytes([]byte("map: *map"), parser.WithAnchors(ref.Docs[0].Anchors))
		require.NoError(t, err, "parse")
		type T struct {
			Map map[string]string
		}
		var v T
		require.NoError(t, dec.DecodeFromNode(f.Docs[0].Body, &v), "decode")
		actual := fmt.Sprintf("%+v", v)
		expect := fmt.Sprintf("%+v", T{map[string]string{"text": "hello"}})
		require.Equal(t, expect, actual)
	})
	t.Run("value is not pointer", func(t *testing.T) {
		var buf bytes.Buffer
		var v bool
		err := codec.NewDecoder(&buf).DecodeFromNode(nil, v)
		require.ErrorIs(t, err, yamlerrors.ErrDecodeRequiredPointerType)
	})
}

func TestDecoder_DecodeWithNode(t *testing.T) {
	t.Run("abstract node", func(t *testing.T) {
		type T struct {
			Text ast.Node `yaml:"text"`
		}
		var v T
		require.NoError(t, codec.Unmarshal([]byte(`text: hello`), &v))
		require.Equal(t, "hello", v.Text.String())
	})
	t.Run("concrete node", func(t *testing.T) {
		type T struct {
			Text *ast.StringNode `yaml:"text"`
		}
		var v T
		require.NoError(t, codec.Unmarshal([]byte(`text: hello`), &v))
		require.Equal(t, "hello", v.Text.String())
	})
}
