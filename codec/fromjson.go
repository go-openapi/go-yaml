// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// fromJSON implements [FromJSON]. It checks src as JSON text when syntax is set.
//
// Without the check it reads any YAML with a JSON form, as [ToJSONTokens] does. The encoder converts the output of a
// MarshalJSON method without the check. json.RawMessage returns its bytes unchecked, and a RawMessage holding YAML
// has always been written as that YAML.
func fromJSON(src []byte, syntax bool, opts ...ast.RenderOption) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(len(src) + len(src)/4)

	stream := ast.NewRenderer(opts...).Stream(&out)
	// A zero Encoder styles a string as Marshal does with no options.
	var enc Encoder

	var parseOpts []parser.Option
	if syntax {
		parseOpts = append(parseOpts, parser.WithJSONSyntax())
	}
	tokens := ToJSONTokens(src, parseOpts...)
	for tok := range tokens.Tokens() {
		if err := streamJSONToken(stream, &enc, tok); err != nil {
			return nil, err
		}
	}
	if err := tokens.Err(); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// streamJSONToken hands one JSON token to the stream.
func streamJSONToken(stream *ast.Stream, enc *Encoder, tok JSONToken) error {
	switch tok.Kind {
	case JSONObjectStart:
		return stream.BeginMapping()
	case JSONArrayStart:
		return stream.BeginSequence()
	case JSONObjectEnd, JSONArrayEnd:
		return stream.End()
	case JSONKey:
		return stream.Key(enc.encodeString(tok.Value, 0))
	case JSONString:
		return stream.Value(enc.encodeString(tok.Value, 0))
	case JSONNumber:
		tk := token.New(tok.Value, tok.Value, token.Position{})
		if strings.ContainsAny(tok.Value, ".eE") {
			return stream.Value(ast.Float(tk))
		}

		return stream.Value(ast.Integer(tk))
	case JSONBool:
		value := strconv.FormatBool(tok.Bool)

		return stream.Value(ast.Bool(token.New(value, value, token.Position{})))
	default:
		return stream.Value(ast.Null(token.New("null", "null", token.Position{})))
	}
}
