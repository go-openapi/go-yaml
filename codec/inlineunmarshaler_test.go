// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// unmarshalsItself reads the whole mapping it is given rather than letting the
// decoder fill its fields.
type unmarshalsItself struct {
	Name string `yaml:"name"`
	Seen bool   `yaml:"-"`
}

func (u *unmarshalsItself) UnmarshalYAML(b []byte) error {
	type plain unmarshalsItself

	var p plain
	if err := codec.Unmarshal(b, &p); err != nil {
		return err
	}
	*u = unmarshalsItself(p)
	u.Seen = true

	return nil
}

// inlinesAnUnmarshaler names the field, so the method does not promote to the
// outer type and only the decoder's own rule can call it.
type inlinesAnUnmarshaler struct {
	Leaf unmarshalsItself `yaml:",inline"`
	Tail string           `yaml:"tail"`
}

// TestAnInlineFieldThatUnmarshalsItselfIsRendered covers a `,inline` field whose
// type reads its own YAML.
//
// The decoder hands such a field the mapping it stands in, rendered back to
// text, and the mapping it builds for that is synthetic: its values are the
// document's nodes, its keys are new. Those keys carried no token, and
// ast.Renderer reads a scalar's token to decide whether to write it as a block,
// so rendering the mapping dereferenced nil and the decode panicked.
//
// go.yaml.in/yaml/v3 reads this shape the same way, calling the field's
// unmarshaler with the whole mapping and filling the rest of the struct
// besides.
func TestAnInlineFieldThatUnmarshalsItselfIsRendered(t *testing.T) {
	var v inlinesAnUnmarshaler
	require.NotPanics(t, func() {
		require.NoError(t, codec.Unmarshal([]byte("name: N\ntail: T\n"), &v))
	})

	assert.Equal(t, "N", v.Leaf.Name)
	assert.True(t, v.Leaf.Seen, "the field read the mapping itself")
	assert.Equal(t, "T", v.Tail, "and the rest of the struct is filled too")
}
