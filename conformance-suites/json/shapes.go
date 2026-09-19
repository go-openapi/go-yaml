// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import (
	"github.com/go-openapi/conformance-suites/stance"
)

// around is what JSON contributes to the shared encoding shapes: a small valid
// document, a way to put bytes inside a string, and its whitespace.
//
// The document is deliberately the smallest thing that is unambiguously a
// document. A larger one would exercise the same encoding question and make
// every failure harder to read.
func around() stance.Around {
	return stance.Around{
		Document:   []byte(`{"a":1}`),
		Whitespace: []byte(" \t\r\n"),
		InString: func(b []byte) []byte {
			out := make([]byte, 0, len(b)+10)
			out = append(out, `{"a":"`...)
			out = append(out, b...)

			return append(out, `"}`...)
		},
	}
}

// EncodingShapes is the cross-product of every byte order mark against every
// position it can occupy, as JSON documents.
func EncodingShapes() []stance.Shape { return stance.EncodingShapes(around()) }
