// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

type Child struct {
	B int
	C int `yaml:"-"`
}

type TestString string

// decodeCase is a document, a value of the type to decode it into, and whether the decoder returns io.EOF instead.
type decodeCase struct {
	name   string
	source string
	value  any
	eof    bool
}

// TestDecoder decodes the source of each case into a new value of the type of the case's value, and compares
// the two.
//
// Each case is named "theme/what", so go test -run 'TestDecoder/double/' runs one theme.
func TestDecoder(t *testing.T) {
	seen := make(map[string]bool)
	for tc := range decoderCases() {
		require.Falsef(t, seen[tc.name], "two cases are named %q", tc.name)
		seen[tc.name] = true

		t.Run(tc.name, testDecode(tc))
	}
}

func testDecode(tc decodeCase) func(*testing.T) {
	return func(t *testing.T) {
		buf := bytes.NewBufferString(tc.source)
		dec := codec.NewDecoder(buf)
		typ := reflect.ValueOf(tc.value).Type()
		value := reflect.New(typ)
		if err := dec.Decode(value.Interface()); err != nil {
			if tc.eof && errors.Is(err, io.EOF) {
				return
			}
			require.NoError(t, err)
		}
		require.False(t, tc.eof, "decode returned no io.EOF")
		actual := fmt.Sprintf("%+v", value.Elem().Interface())
		expect := fmt.Sprintf("%+v", tc.value)
		require.Equal(t, expect, actual)
	}
}

func decoderCases() iter.Seq[decodeCase] {
	return concatCases(
		decodeBasicScalarCases(),
		decodeNumberFormCases(),
		decodeBoolCases(),
		decodeIntegerRangeCases(),
		decodeFloatRangeCases(),
		decodeTimeCases(),
		decodeTagCases(),
		decodeSingleQuotedCases(),
		decodeDoubleQuotedCases(),
		decodeKeyAndPlainCases(),
		decodeNullCases(),
		decodeCollectionCases(),
		decodeDocumentCases(),
		decodeBlockScalarCases(),
		decodeStructCases(),
	)
}
