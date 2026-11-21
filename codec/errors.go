// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

var (
	// ErrUnknownCommentPositionType reports a Comment whose Position is none of
	// head, line or foot.
	ErrUnknownCommentPositionType = errors.New("unknown comment position type")
	// ErrInvalidCommentMapValue reports a nil CommentMap handed to CommentToMap,
	// which has nowhere to put what it collects.
	ErrInvalidCommentMapValue = errors.New("invalid comment map value. it must be not nil value")
	// ErrDecodeRequiredPointerType reports a decode into a value the decoder
	// cannot write through.
	ErrDecodeRequiredPointerType = errors.New("required pointer type value")
	// ErrExceededMaxDepth reports a document nested deeper than the parser
	// reads. It is [yamlerrors.ErrExceededMaxDepth].
	ErrExceededMaxDepth = yamlerrors.ErrExceededMaxDepth
	// ErrKeyNotComparable reports a key no map may hold: Go hashes no slice,
	// map or function.
	ErrKeyNotComparable = errors.New("key is not comparable")
)
