// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import "errors"

var (
	// ErrInvalidQuery marks a path that asks the document for something it cannot hold, such as an index into a
	// mapping.
	ErrInvalidQuery = errors.New("invalid query")
	// ErrInvalidPath marks a Path that neither PathString nor PathBuilder built, which holds no step.
	ErrInvalidPath = errors.New("invalid path instance")
	// ErrInvalidPathString marks text that is not a YAML path.
	ErrInvalidPathString = errors.New("invalid path string")
	// ErrNotFoundNode marks a path that addresses no node of the document.
	ErrNotFoundNode = errors.New("node not found")
)
