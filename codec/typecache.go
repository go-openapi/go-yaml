// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"reflect"
	"sync"
)

// typeCache is a [sync.Map] from a reflect.Type to a V, safe for concurrent
// use. It holds the one type assertion its callers would otherwise repeat.
type typeCache[V any] struct {
	m sync.Map
}

// Load returns the value stored for t, and whether there is one.
func (c *typeCache[V]) Load(t reflect.Type) (V, bool) {
	v, ok := c.m.Load(t)
	if !ok {
		var zero V

		return zero, false
	}

	return v.(V), true //nolint:forcetypeassert // Store and LoadOrStore only ever store a V
}

// LoadOrStore returns the value already stored for t if there is one, and
// otherwise stores and returns v.
func (c *typeCache[V]) LoadOrStore(t reflect.Type, v V) V {
	actual, _ := c.m.LoadOrStore(t, v)

	return actual.(V) //nolint:forcetypeassert // Store and LoadOrStore only ever store a V
}

// Store records v for t.
func (c *typeCache[V]) Store(t reflect.Type, v V) {
	c.m.Store(t, v)
}
