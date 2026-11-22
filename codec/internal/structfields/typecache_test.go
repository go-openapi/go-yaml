// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package structfields

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
)

func TestCache(t *testing.T) {
	var c Cache[string]
	intType, stringType := reflect.TypeFor[int](), reflect.TypeFor[string]()

	_, ok := c.Load(intType)
	assert.False(t, ok)

	assert.Equal(t, "first", c.LoadOrStore(intType, "first"))
	assert.Equal(t, "first", c.LoadOrStore(intType, "second"), "the first value stored stays")

	c.Store(stringType, "s")
	v, ok := c.Load(stringType)
	assert.True(t, ok)
	assert.Equal(t, "s", v)

	c.Clear()
	_, ok = c.Load(intType)
	assert.False(t, ok)
}
