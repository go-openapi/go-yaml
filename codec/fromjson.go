// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"iter"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// fromJSON implements [FromJSON]. It checks src as JSON text when syntax is set.
//
// Without the check it reads any YAML with a JSON form, as [ToJSONTokens] does. The encoder converts the output of a
// MarshalJSON method without the check. json.RawMessage returns its bytes unchecked, and a RawMessage holding YAML
// has always been written as that YAML.
func fromJSON(src []byte, syntax bool, opts ...ast.RenderOption) ([]byte, error) {
	var parseOpts []parser.Option
	if syntax {
		parseOpts = append(parseOpts, parser.WithJSONSyntax())
	}
	tokens := ToJSONTokens(src, parseOpts...)

	return renderJSONTokens(tokens.Tokens(), len(src), tokens.Err, opts...)
}

// FromJSONTokens writes the YAML for a sequence of JSON tokens, as [FromJSON] writes it for the JSON text they come
// from.
//
// It takes the tokens of any JSON reader that yields them in document order, as [JSONTokens.Tokens] does, with no
// token for a "," or a ":". It checks their order and nothing else: a key repeated in one object is written twice. A
// reader that stops on an error leaves a collection open, and FromJSONTokens then returns
// [ast.ErrStreamOrder]: check the reader's own error first, since it names the fault.
//
// opts lay the YAML out: [ast.WithIndent] and [ast.WithIndentSequence].
func FromJSONTokens(tokens iter.Seq[JSONToken], opts ...ast.RenderOption) ([]byte, error) {
	return renderJSONTokens(tokens, 0, nil, opts...)
}

// renderJSONTokens writes tokens through an [ast.Stream]. sizeHint presizes the output. sourceErr, when set, returns
// the error that stopped the source, and is checked before the stream closes.
func renderJSONTokens(
	tokens iter.Seq[JSONToken], sizeHint int, sourceErr func() error, opts ...ast.RenderOption,
) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(sizeHint + sizeHint/4)

	stream := ast.NewRenderer(opts...).Stream(&out)
	// A zero Encoder styles a string as Marshal does with no options.
	var enc Encoder

	// depth counts the collections open around a token, to give each key the scratch set of its own depth.
	var depth int
	for tok := range tokens {
		switch tok.Kind {
		case JSONObjectStart, JSONArrayStart:
			enc.scratch = nil
		case JSONObjectEnd, JSONArrayEnd:
			enc.scratch = nil
			depth--
		case JSONKey:
			enc.scratch = enc.keyScratchAt(max(depth-1, 0))
		default:
			enc.scratch = &enc.valueScratch
		}
		if err := streamJSONToken(stream, &enc, tok); err != nil {
			return nil, err
		}
		if tok.Kind == JSONObjectStart || tok.Kind == JSONArrayStart {
			depth++
		}
	}
	if sourceErr != nil {
		if err := sourceErr(); err != nil {
			return nil, err
		}
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// streamJSONToken hands one JSON token to the stream. It builds a scalar from enc's scratch set when one is set.
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
		tk := enc.newToken(tok.Value, 0)
		if strings.ContainsAny(tok.Value, ".eE") {
			return stream.Value(enc.floatNode(tk))
		}

		return stream.Value(enc.integerNode(tk))
	case JSONBool:
		return stream.Value(enc.boolNode(enc.newToken(strconv.FormatBool(tok.Bool), 0)))
	default:
		return stream.Value(enc.nullNode(enc.newToken("null", 0)))
	}
}
