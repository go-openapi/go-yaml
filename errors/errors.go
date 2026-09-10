// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package errors reports what went wrong reading or writing a YAML document,
// with the position it happened at and the lines of source around it.
//
// Every failure raised by the scanner, the parser and the codec is an [Error].
// Match one against the kind it carries:
//
//	import (
//		"errors"
//
//		yamlerrors "github.com/go-openapi/go-yaml/errors"
//	)
//
//	if errors.Is(err, yamlerrors.ErrDuplicateKey) {
//		// ...
//	}
//
// To reach the position, unwrap to the [Error] itself:
//
//	var yerr *yamlerrors.Error
//	if errors.As(err, &yerr) {
//		fmt.Println(yerr.GetToken().Position)
//	}
//
// Error prints the message with the source drawn under it. Use [FormatError]
// to choose color and whether to draw the source, and [FormatErrorAtToken] to
// draw a message of your own at a token.
package errors

import (
	stderrors "errors"
	"fmt"
	"reflect"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/printer"
	"github.com/go-openapi/go-yaml/token"
)

// The kinds of failure an [Error] carries. Match one with [errors.Is].
var (
	// ErrSyntax reports a document the scanner or the parser could not read.
	ErrSyntax = stderrors.New("syntax error")
	// ErrTypeMismatch reports a value that does not fit the Go type it was
	// decoded into.
	ErrTypeMismatch = stderrors.New("type mismatch")
	// ErrOverflow reports a number too large for the Go type it was decoded
	// into.
	ErrOverflow = stderrors.New("overflow")
	// ErrDuplicateKey reports a mapping with the same key twice, found with
	// the strict duplicate-key check on.
	ErrDuplicateKey = stderrors.New("duplicate key")
	// ErrUnknownField reports a mapping key with no matching struct field,
	// found with the strict unknown-field check on.
	ErrUnknownField = stderrors.New("unknown field")
	// ErrUnexpectedNodeType reports a node of the wrong shape, such as a
	// sequence where a mapping is required.
	ErrUnexpectedNodeType = stderrors.New("unexpected node type")
	// ErrUnhashableKey reports a mapping key Go cannot use as a map key -- a
	// sequence or a mapping, decoded into a map whose key type admits one,
	// such as a map[any]any. A map key must be comparable, and setting one
	// that is not panicked with "hash of unhashable type" until it was
	// reported here.
	ErrUnhashableKey = stderrors.New("unhashable map key")
	// ErrNotJSON reports a well-formed YAML document that JSON has no spelling
	// for, found with
	// [github.com/go-openapi/go-yaml/parser.WithJSONCompatible] on. It is not a
	// syntax error: the document is valid YAML and only the conversion is
	// impossible.
	ErrNotJSON = stderrors.New("not convertible to JSON")
	// ErrUnknownAnchor reports an alias naming an anchor the document does not
	// declare before it. An anchor belongs to the document it was written in
	// and an alias names the most recent one of that name, so an anchor
	// declared later, or in another document, is not one this alias can name.
	ErrUnknownAnchor = stderrors.New("unknown anchor")
	// ErrRecursiveAlias reports an alias standing inside the node its own
	// anchor names, directly or through another anchor. The node is not
	// resolved yet, so there is nothing for the alias to stand for.
	ErrRecursiveAlias = stderrors.New("recursive alias")
	// ErrExcessiveAliasing reports a document whose aliases build far more than
	// the document could hold written out. An alias names a node and the value
	// it stands for is built again wherever the alias appears, so a chain of
	// them multiplies: 259 bytes of YAML can name 100,000 values and 423 bytes
	// 387 million.
	ErrExcessiveAliasing = stderrors.New("excessive aliasing")
)

const (
	defaultFormatColor   = false
	defaultIncludeSource = true
)

// Source is the text an error was found in, and the number of its first line.
//
// A parse holds the whole document, so the text costs nothing to keep: a Go
// string shares the bytes it is taken from. A scanner fed from a reader would
// keep a window around the failure instead, and FirstLine says which line that
// window starts at.
type Source struct {
	Text      string
	FirstLine int
}

// Error is a failure raised while reading or writing a YAML document.
//
// The kind it carries says what went wrong; [errors.Is] matches against
// [ErrSyntax] and its siblings. GetToken gives the position, and Error draws
// the lines of source around it once the document has been recorded with
// [WithSource].
type Error struct {
	kind error

	msg      string       // the message as given, for a syntax, duplicate key or unknown field error
	dst, src reflect.Type // the Go types, for a type mismatch or an overflow
	num      string       // the number that did not fit, for an overflow
	field    string       // the struct field path, for a type mismatch
	actual   string       // the YAML names of the node types, for an unexpected node type
	expected string

	token  *token.Token
	source Source
}

// NewSyntax reports msg as a malformed document at tk.
func NewSyntax(msg string, tk *token.Token) *Error {
	return &Error{kind: ErrSyntax, msg: msg, token: tk}
}

// NewUnhashableKey reports src as a mapping key Go cannot compare, at tk.
func NewUnhashableKey(src reflect.Type, tk *token.Token) *Error {
	return &Error{
		kind:  ErrUnhashableKey,
		msg:   fmt.Sprintf("cannot use %s as a map key: it is not comparable", src),
		token: tk,
	}
}

// NewUnnamedKey reports a non-scalar key, at tk.
//
// A Go map keyed by a string needs a key with a spelling, and a collection has
// none: [github.com/go-openapi/go-yaml/token.KeyName] writes the canonical text
// of every scalar type and nothing for a sequence or a mapping. A Go map keyed
// by any refuses one for a second reason -- Go hashes no slice or map.
//
// Go's own printing stood in before, naming the key after the value the decoder
// built rather than after the document.
func NewUnnamedKey(kind string, tk *token.Token) *Error {
	return &Error{
		kind:  ErrUnhashableKey,
		msg:   fmt.Sprintf("a %s cannot be a key in a Go map", kind),
		token: tk,
	}
}

// NewNotJSON reports msg as a document JSON cannot hold, at tk.
func NewNotJSON(msg string, tk *token.Token) *Error {
	return &Error{kind: ErrNotJSON, msg: msg, token: tk}
}

// NewUnknownAnchor reports the alias name as naming no anchor, at tk.
func NewUnknownAnchor(name string, tk *token.Token) *Error {
	return &Error{
		kind:  ErrUnknownAnchor,
		msg:   fmt.Sprintf("could not find alias %q", name),
		token: tk,
	}
}

// NewExcessiveAliasing reports a decode building more than the document can
// account for, at tk.
func NewExcessiveAliasing(built, budget int, tk *token.Token) *Error {
	return &Error{
		kind: ErrExcessiveAliasing,
		msg: fmt.Sprintf(
			"the document's aliases build more than it can account for: %d values against a budget of %d",
			built, budget,
		),
		token: tk,
	}
}

// NewRecursiveAlias reports the alias name as standing inside what its own
// anchor names, at tk.
func NewRecursiveAlias(name string, tk *token.Token) *Error {
	return &Error{
		kind:  ErrRecursiveAlias,
		msg:   fmt.Sprintf("alias %q names an anchor that is not resolved yet", name),
		token: tk,
	}
}

// NewTypeMismatch reports a value of type src decoded into a Go value of type
// dst, at tk.
func NewTypeMismatch(dst, src reflect.Type, tk *token.Token) *Error {
	return &Error{kind: ErrTypeMismatch, dst: dst, src: src, token: tk}
}

// NewOverflow reports the number num as too large for the Go type dst, at tk.
func NewOverflow(dst reflect.Type, num string, tk *token.Token) *Error {
	return &Error{kind: ErrOverflow, dst: dst, num: num, token: tk}
}

// NewDuplicateKey reports msg as a repeated mapping key at tk.
func NewDuplicateKey(msg string, tk *token.Token) *Error {
	return &Error{kind: ErrDuplicateKey, msg: msg, token: tk}
}

// NewUnknownField reports msg as a mapping key with no struct field at tk.
func NewUnknownField(msg string, tk *token.Token) *Error {
	return &Error{kind: ErrUnknownField, msg: msg, token: tk}
}

// NewUnexpectedNodeType reports a node of type actual where expected was
// required, at tk.
func NewUnexpectedNodeType(actual, expected ast.NodeType, tk *token.Token) *Error {
	return &Error{
		kind:     ErrUnexpectedNodeType,
		actual:   actual.YAMLName(),
		expected: expected.YAMLName(),
		token:    tk,
	}
}

// Unwrap returns the kind of failure, so that [errors.Is] matches an Error
// against [ErrSyntax] and its siblings.
func (e *Error) Unwrap() error { return e.kind }

// GetToken returns the token the failure was found at, nil if the failure
// carries no position.
func (e *Error) GetToken() *token.Token { return e.token }

// GetMessage returns the message without the position or the source.
func (e *Error) GetMessage() string { return e.message() }

// StructField returns the struct field path recorded by [Error.SetStructField],
// empty when none was.
func (e *Error) StructField() string { return e.field }

// SetStructField records name as the struct field a type mismatch was found
// decoding, replacing any path already there. The decoder builds the path as
// it unwinds, so a mismatch nested two structs deep reads "Outer.Inner.leaf".
func (e *Error) SetStructField(name string) { e.field = name }

// Error returns the message with its position, and the lines of source around
// it where the document has been recorded with [WithSource].
func (e *Error) Error() string {
	return e.FormatError(defaultFormatColor, defaultIncludeSource)
}

// FormatError renders the failure, coloring it when colored is set and
// drawing the source under it when inclSource is set and the document is
// known.
func (e *Error) FormatError(colored, inclSource bool) string {
	return formatError(e.message(), e.token, e.source, colored, inclSource)
}

func (e *Error) setSource(src Source) { e.source = src }

func (e *Error) message() string {
	switch e.kind {
	case ErrTypeMismatch:
		if e.field != "" {
			return fmt.Sprintf("cannot unmarshal %s into Go struct field %s of type %s", e.src, e.field, e.dst)
		}

		return fmt.Sprintf("cannot unmarshal %s into Go value of type %s", e.src, e.dst)
	case ErrOverflow:
		return fmt.Sprintf("cannot unmarshal %s into Go value of type %s ( overflow )", e.num, e.dst)
	case ErrUnexpectedNodeType:
		return fmt.Sprintf("%s was used where %s is expected", e.actual, e.expected)
	default:
		return e.msg
	}
}

// sourced is an error that can be told the text it was found in.
type sourced interface {
	setSource(Source)
}

// WithSource tells err, and every error wrapped inside it, the text it was
// found in. An error that never learns keeps its position and prints no
// document around it.
func WithSource(err error, src Source) error {
	for e := err; e != nil; e = stderrors.Unwrap(e) {
		if s, ok := e.(sourced); ok {
			s.setSource(src)
		}
	}

	return err
}

// FormatError renders err, drawing the source around it where err carries one
// and inclSource asks for it. An error this package did not raise prints as
// its own Error method returns it.
func FormatError(err error, colored, inclSource bool) string {
	var yerr *Error
	if stderrors.As(err, &yerr) {
		return yerr.FormatError(colored, inclSource)
	}

	return err.Error()
}

// FormatErrorAtToken renders msg as a failure reported at tk, drawing the
// lines of source around it.
//
// source is the document tk was read from. Drawing a document requires being
// given one: an [Error] renders its own context from the text it was found in,
// and this helper needs the same. Pass nil to print the position and the
// message alone.
func FormatErrorAtToken(msg string, tk *token.Token, source []byte, colored, inclSource bool) string {
	var src Source
	if len(source) > 0 {
		src = Source{Text: nocopy.String(source), FirstLine: 1}
	}

	return formatError(msg, tk, src, colored, inclSource)
}

func formatError(errMsg string, tk *token.Token, src Source, colored, inclSource bool) string {
	var pp printer.Printer
	if tk == nil {
		return pp.PrintErrorMessage(errMsg, colored)
	}

	pos := fmt.Sprintf("[%d:%d] ", tk.Position.Line, tk.Position.Column)
	msg := pp.PrintErrorMessage(pos+errMsg, colored)
	if inclSource && src.Text != "" {
		msg += "\n" + pp.PrintErrorSource(src.Text, src.FirstLine, tk, colored)
	}

	return msg
}
