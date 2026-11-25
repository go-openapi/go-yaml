// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"unsafe"

	lexer "github.com/go-openapi/core/json/lexers/default-lexer"
	coretoken "github.com/go-openapi/core/json/lexers/token"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
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

	// The YAML parser already rejects a key repeated in one object, so the tokens need no second check.
	return renderJSONTokens(tokens.Tokens(), len(src), tokens.Err, false, opts...)
}

// FromJSONTokens writes the YAML for a sequence of JSON tokens, as [FromJSON] writes it for the JSON text they come
// from.
//
// It takes the tokens of any JSON reader that yields them in document order, as [JSONTokens.Tokens] does, with no
// token for a "," or a ":". It checks their order, and returns an error wrapping
// [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey] for a key repeated in one object, as FromJSON does: YAML has
// no mapping with two equal keys. Keys are compared as the tokens hold them, after unescaping, with no Unicode
// normalization. A reader that stops on an error leaves a collection open, and FromJSONTokens then returns
// [ast.ErrStreamOrder]: check the reader's own error first, since it names the fault.
//
// opts lay the YAML out: [ast.WithIndent] and [ast.WithIndentSequence].
func FromJSONTokens(tokens iter.Seq[JSONToken], opts ...ast.RenderOption) ([]byte, error) {
	return renderJSONTokens(tokens, 0, nil, true, opts...)
}

// renderJSONTokens writes tokens through an [ast.Stream]. sizeHint presizes the output. sourceErr, when set, returns
// the error that stopped the source, and is checked before the stream closes. checkKeys rejects a key repeated in one
// object, for a source that does not.
func renderJSONTokens(
	tokens iter.Seq[JSONToken], sizeHint int, sourceErr func() error, checkKeys bool, opts ...ast.RenderOption,
) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(sizeHint + sizeHint/4)

	stream := ast.NewRenderer(opts...).Stream(&out)
	// A zero Encoder styles a string as Marshal does with no options.
	var enc Encoder

	// depth counts the collections open around a token, to give each key the scratch set of its own depth.
	var depth int
	var keys jsonKeySets
	for tok := range tokens {
		if checkKeys {
			if err := keys.track(tok); err != nil {
				return nil, err
			}
		}
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

// jsonKeySets holds the keys seen in each open object, so that a repeat is rejected before it is written.
//
// The sets are kept by depth and cleared for reuse, so a document allocates one set per level of object nesting, not
// one per object.
type jsonKeySets struct {
	// sets holds one set per open collection, nil for an array.
	sets []map[string]struct{}
	// spare holds the set last used at each depth, nil where no object has opened at that depth yet.
	spare []map[string]struct{}
}

func (k *jsonKeySets) track(tok JSONToken) error {
	depth := len(k.sets)
	switch tok.Kind {
	case JSONObjectStart:
		for len(k.spare) <= depth {
			// An array leaves its depth without a set, so the slice can be more than one short.
			k.spare = append(k.spare, nil)
		}
		set := k.spare[depth]
		if set == nil {
			set = make(map[string]struct{})
			k.spare[depth] = set
		} else {
			clear(set)
		}
		k.sets = append(k.sets, set)
	case JSONArrayStart:
		k.sets = append(k.sets, nil)
	case JSONObjectEnd, JSONArrayEnd:
		if depth > 0 {
			k.sets = k.sets[:depth-1]
		}
	case JSONKey:
		if depth == 0 || k.sets[depth-1] == nil {
			// Out of place: the stream rejects it.
			return nil
		}
		set := k.sets[depth-1]
		if _, repeated := set[tok.Value]; repeated {
			return fmt.Errorf("mapping key %q already defined in this object: %w", tok.Value, yamlerrors.ErrDuplicateKey)
		}
		set[tok.Value] = struct{}{}
	default:
	}

	return nil
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

// fromJSONText implements [FromJSON]: it reads src with the JSON lexer of github.com/go-openapi/core/json, which is
// several times faster than the YAML parser's walk, and writes the YAML through [FromJSONTokens].
//
// A lexer error, and a token sequence the YAML stream cannot take, such as an empty input or two values in a row,
// return an error wrapping [yamlerrors.ErrNotJSONSyntax]. A repeated member name returns one wrapping
// [yamlerrors.ErrDuplicateKey].
func fromJSONText(src []byte, opts ...ast.RenderOption) ([]byte, error) {
	l, release := lexer.BorrowLexerWithBytes(src)
	defer release()

	lexErr := func() error {
		if err := l.Err(); err != nil {
			return fmt.Errorf("%w: %w", yamlerrors.ErrNotJSONSyntax, err)
		}

		return nil
	}
	out, err := renderJSONTokens(coreJSONTokens(l, src), len(src), lexErr, true, opts...)
	if errors.Is(err, ast.ErrStreamOrder) {
		return nil, fmt.Errorf("%w: the tokens do not form one JSON value", yamlerrors.ErrNotJSONSyntax)
	}

	return out, err
}

// coreJSONTokens adapts the core lexer's tokens to [JSONToken], dropping the "," and ":" tokens.
//
// src is the text l reads. A key, a string or a number the lexer returns as a window into src is used in place; one it
// decoded into its own buffer, which it reuses, is copied. Nearly every value of a JSON document is a window: all of
// them on the benchmark workloads but one with escaped strings. The tokens are valid while src is neither changed nor
// freed, which holds for the call of [FromJSON] that reads them.
func coreJSONTokens(l *lexer.L, src []byte) iter.Seq[JSONToken] {
	lo := uintptr(unsafe.Pointer(unsafe.SliceData(src)))
	hi := lo + uintptr(len(src))
	text := func(v []byte) string {
		if len(v) == 0 {
			return ""
		}
		if p := uintptr(unsafe.Pointer(unsafe.SliceData(v))); p >= lo && p+uintptr(len(v)) <= hi {
			return unsafe.String(unsafe.SliceData(v), len(v))
		}

		return string(v)
	}

	return func(yield func(JSONToken) bool) {
		for t := range l.Tokens() {
			var out JSONToken
			switch t.Kind() {
			case coretoken.Delimiter:
				switch t.Delimiter() {
				case coretoken.OpeningBracket:
					out.Kind = JSONObjectStart
				case coretoken.ClosingBracket:
					out.Kind = JSONObjectEnd
				case coretoken.OpeningSquareBracket:
					out.Kind = JSONArrayStart
				case coretoken.ClosingSquareBracket:
					out.Kind = JSONArrayEnd
				default:
					continue
				}
			case coretoken.Key:
				out.Kind, out.Value = JSONKey, text(t.Value())
			case coretoken.String:
				out.Kind, out.Value = JSONString, text(t.Value())
			case coretoken.Number:
				out.Kind, out.Value = JSONNumber, text(t.Value())
			case coretoken.Boolean:
				out.Kind, out.Bool = JSONBool, t.Bool()
			case coretoken.Null:
				out.Kind = JSONNull
			default:
				continue
			}
			if !yield(out) {
				return
			}
		}
	}
}
