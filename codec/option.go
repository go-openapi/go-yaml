// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"io"
	"reflect"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/yamlpath"
	"github.com/go-openapi/go-yaml/parser"
)

// DecodeOption functional option type for Decoder.
type DecodeOption func(d *Decoder) error

// ReferenceReaders pass to Decoder that reference to anchor defined by passed readers.
func ReferenceReaders(readers ...io.Reader) DecodeOption {
	return func(d *Decoder) error {
		d.referenceReaders = append(d.referenceReaders, readers...)
		return nil
	}
}

// ReferenceFiles pass to Decoder that reference to anchor defined by passed files.
func ReferenceFiles(files ...string) DecodeOption {
	return func(d *Decoder) error {
		d.referenceFiles = files
		return nil
	}
}

// ReferenceDirs pass to Decoder that reference to anchor defined by files under the passed dirs.
func ReferenceDirs(dirs ...string) DecodeOption {
	return func(d *Decoder) error {
		d.referenceDirs = dirs
		return nil
	}
}

// RecursiveDir search yaml file recursively from passed dirs by ReferenceDirs option.
func RecursiveDir(isRecursive bool) DecodeOption {
	return func(d *Decoder) error {
		d.isRecursiveDir = isRecursive
		return nil
	}
}

// Validator set StructValidator instance to Decoder.
func Validator(v StructValidator) DecodeOption {
	return func(d *Decoder) error {
		d.validator = v
		return nil
	}
}

// Strict enable DisallowUnknownField.
func Strict() DecodeOption {
	return func(d *Decoder) error {
		d.disallowUnknownField = true
		return nil
	}
}

// DisallowUnknownField causes the Decoder to return an error when the destination
// is a struct and the input contains object keys which do not match any
// non-ignored, exported fields in the destination.
func DisallowUnknownField() DecodeOption {
	return func(d *Decoder) error {
		d.disallowUnknownField = true
		return nil
	}
}

// AllowFieldPrefixes, when paired with [DisallowUnknownField], allows fields
// with the specified prefixes to bypass the unknown field check.
func AllowFieldPrefixes(prefixes ...string) DecodeOption {
	return func(d *Decoder) error {
		d.allowedFieldPrefixes = append(d.allowedFieldPrefixes, prefixes...)
		return nil
	}
}

// AllowDuplicateMapKey ignore syntax error when mapping keys that are duplicates.
func AllowDuplicateMapKey() DecodeOption {
	return func(d *Decoder) error {
		d.allowDuplicateMapKey = true
		return nil
	}
}

// WithParserOptions passes options straight to the parser the decode runs.
//
// The decode options here name what a caller wants of the *decode*, and the
// decoder turns some of them into parser options on the caller's behalf --
// [AllowDuplicateMapKey] becomes
// [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey], and
// [CommentToMap] asks the parser for comments. This is the way through for the
// rest of the parser's surface, which the decoder has no opinion about.
//
// The one most callers want is the version a document is read under:
//
//	codec.UnmarshalWithOptions(src, &v,
//		codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
//
// Without it a "%YAML 1.1" directive is the only route into the 1.1 schema, and
// a document that does not carry one cannot be read under it at all.
//
// They are applied last and in order, so where two options set the same field
// the later one wins. Every parser option a decode option asks for today only
// turns something on and has no counterpart that turns it off, so this cannot
// take one back -- it adds to them.
func WithParserOptions(opts ...parser.Option) DecodeOption {
	return func(d *Decoder) error {
		d.extraParserOptions = append(d.extraParserOptions, opts...)

		return nil
	}
}

// ShareAliases makes every alias of one anchor stand for the same Go value,
// rather than each building its own.
//
// By default they are independent: "first: *b" and "second: *b" give two maps,
// so writing through one leaves the other alone. Sharing gives one map under
// both names, and a caller who writes through either writes through both.
//
// Two reasons to ask for it:
//
//   - The anchors survive a round trip through a Go value. [MarshalAnchor] and
//     the ",anchor" and ",alias" struct tags find an anchor by the address the
//     value stands at, so an encoder can only write "*name" where the decode
//     left one value under two names. Without this option, decoding a document
//     and encoding it again writes the anchored node out in full at each alias.
//     Reading a document into an [github.com/go-openapi/go-yaml/ast] tree and
//     rendering it keeps the anchors either way; this is about the Go value.
//   - An alias costs nothing. Building each alias its own value is what a
//     document written to exhaust memory abuses -- see [errors.ErrExcessiveAliasing]
//     -- and a shared value is built once however many aliases name it.
//
// Do not use it on a value the program then writes to, unless the aliasing is
// the point.
func ShareAliases() DecodeOption {
	return func(d *Decoder) error {
		d.shareAliases = true

		return nil
	}
}

// UseOrderedMap can be interpreted as a map,
// and uses MapSlice ( ordered map ) aggressively if there is no type specification.
func UseOrderedMap() DecodeOption {
	return func(d *Decoder) error {
		d.useOrderedMap = true
		return nil
	}
}

// UseStringKeys reads every mapping key as text, so a map with an interface key
// type holds strings rather than the Go types the keys resolve to.
//
// Without it, "1.5: a" decoded into a map[any]any gives the key float64(1.5),
// "true: a" gives bool(true), and a key the map's own type cannot take is an
// error. With it both give a string, the same spelling a map[string]any gets,
// which is what a JSON object needs. A map whose key type is named -- a
// map[float64]any -- is untouched, since the caller asked for that type.
//
// It reaches [MapSlice] too, whose MapItem.Key otherwise holds what the key
// resolves to. There it makes "1: a" over "\"1\": b" two entries under one
// name rather than an error, since a MapSlice can hold both.
//
// It does not make a collection usable as a key. Go cannot hash a slice or a
// map, so "? [a]" is an error either way; use
// [github.com/go-openapi/go-yaml/parser.WithJSONCompatible] to refuse those at
// parse time.
func UseStringKeys() DecodeOption {
	return func(d *Decoder) error {
		d.useStringKeys = true

		return nil
	}
}

// UseJSONTags reads a struct's fields the way encoding/json does wherever the
// `yaml` tag leaves off: the `json` tag names a field carrying no `yaml` tag,
// and an anonymous embedded struct that no tag names has its fields promoted
// into the outer one.
//
// The `yaml` tag always wins, so this adds a way to name a field and takes none
// away. A type carrying both tags on one field reads the same under the option
// as without it, and a `yaml:"-"` still hides a field the `json` tag names.
//
// Without it the decoder reads the `yaml` tag alone, as go.yaml.in/yaml/v3
// does, and promotes an embedded struct only where a tag says `,inline`. A type
// tagged for JSON alone then takes the lowercased Go name for every field, and
// carries no inline marker anywhere, because encoding/json never needed one.
// github.com/go-openapi/spec is such a type, and decoding an OpenAPI document
// into spec.Swagger without this option fills the outer map and leaves every
// specification in it zero.
//
// It does not make the decoder call UnmarshalJSON -- [UseJSONUnmarshaler] does
// that, converting the document to JSON first. This fills the fields directly,
// so a type whose UnmarshalJSON does more than assign them (spec.Swagger
// gathers "x-" keys into its extensions) will not get that.
//
// A mapping key matches a field name exactly where it can, and otherwise
// matches one differing only in case, which is the order encoding/json tries
// them in. Without the option a key matches exactly or matches nothing, as
// go.yaml.in/yaml/v3 has it.
//
// It names a field the `json` tag names, and leaves a field neither tag names
// under its lowercased Go name. [UseInferredNames] is what changes that one --
// though with case-insensitive matching on, both spellings of a Go name reach
// the field either way, and the option decides which of them an encoder writes.
//
// [WriteJSONTags] is the encoder's half of it. Pass both and a value read from
// a document is written back under the names it was read from.
func UseJSONTags(use bool) DecodeOption {
	return func(d *Decoder) error {
		d.useJSONTags = use

		return nil
	}
}

// UseInferredNames names a field that no tag names after its Go name, verbatim,
// the way encoding/json does.
//
// Without it such a field takes its Go name lowercased, which is what
// go.yaml.in/yaml/v3 does: a field declared FieldName is written "fieldname",
// and a document writing "FieldName" reaches nothing. Under the option the two
// swap over.
//
// It changes names and nothing else. Which fields a decode considers, how an
// anonymous embedded struct is read and how a repeated name is resolved are
// [UseJSONTags]'s to say, and the two options are independent: a struct with no
// tags at all reads under this one alone.
//
// With [UseJSONTags] set, a mapping key matches a field name but for case, so
// both spellings of a Go name reach the field whether this is set or not. What
// it still decides there is which of them is the exact match, and so which one
// [WriteInferredNames] writes.
func UseInferredNames(use bool) DecodeOption {
	return func(d *Decoder) error {
		d.useInferredNames = use

		return nil
	}
}

// UseJSONUnmarshaler if neither `Unmarshaler` nor `GoYAMLUnmarshaler` is implemented
// and `UnmashalJSON([]byte)error` is implemented, convert the argument from `YAML` to `JSON` and then call it.
func UseJSONUnmarshaler() DecodeOption {
	return func(d *Decoder) error {
		d.useJSONUnmarshaler = true
		return nil
	}
}

// CustomUnmarshaler overrides any decoding process for the type specified in generics.
//
// NOTE: If RegisterCustomUnmarshaler and CustomUnmarshaler of DecodeOption are specified for the same type,
// the CustomUnmarshaler specified in DecodeOption takes precedence.
func CustomUnmarshaler[T any](unmarshaler func(*T, []byte) error) DecodeOption {
	return func(d *Decoder) error {
		var typ *T
		d.customUnmarshalerMap[reflect.TypeOf(typ)] = func(v any, b []byte) error {
			return unmarshaler(customArg[*T](v), b)
		}
		return nil
	}
}

// EncodeOption functional option type for Encoder.
type EncodeOption func(e *Encoder) error

// Indent change indent number.
func Indent(spaces int) EncodeOption {
	return func(e *Encoder) error {
		e.indentNum = spaces
		return nil
	}
}

// IndentSequence causes sequence values to be indented the same value as Indent.
func IndentSequence(indent bool) EncodeOption {
	return func(e *Encoder) error {
		e.indentSequence = indent
		return nil
	}
}

// UseSingleQuote determines if single or double quotes should be preferred for strings.
func UseSingleQuote(sq bool) EncodeOption {
	return func(e *Encoder) error {
		e.singleQuote = sq
		return nil
	}
}

// Flow encoding by flow style.
func Flow(isFlowStyle bool) EncodeOption {
	return func(e *Encoder) error {
		e.isFlowStyle = isFlowStyle
		return nil
	}
}

// WithSmartAnchor when multiple map values share the same pointer,
// an anchor is automatically assigned to the first occurrence, and aliases are used for subsequent elements.
// The map key name is used as the anchor name by default.
// If key names conflict, a suffix is automatically added to avoid collisions.
// This is an experimental feature and cannot be used simultaneously with anchor tags.
//
// Encoding a value that came from Unmarshal writes an anchor only where the
// decode left two names at one address, so pass [ShareAliases] to the decode.
// Without it every alias builds its own value, nothing shares a pointer, and
// this option has nothing to find.
func WithSmartAnchor() EncodeOption {
	return func(e *Encoder) error {
		e.enableSmartAnchor = true
		return nil
	}
}

// UseLiteralStyleIfMultiline causes encoding multiline strings with a literal syntax,
// no matter what characters they include.
func UseLiteralStyleIfMultiline(useLiteralStyleIfMultiline bool) EncodeOption {
	return func(e *Encoder) error {
		e.useLiteralStyleIfMultiline = useLiteralStyleIfMultiline
		return nil
	}
}

// JSON encode in JSON format.
func JSON() EncodeOption {
	return func(e *Encoder) error {
		e.isJSONStyle = true
		e.isFlowStyle = true
		return nil
	}
}

// MarshalAnchor call back if encoder find an anchor during encoding.
//
// An anchor is found by the address its value stands at, so a document decoded
// and encoded again keeps its anchors only where the decode left one value
// under every name that points at it. Pass [ShareAliases] to Unmarshal for
// that; without it each alias builds its own value and the anchored node is
// written out in full at every one.
func MarshalAnchor(callback func(*ast.AnchorNode, any) error) EncodeOption {
	return func(e *Encoder) error {
		e.anchorCallback = callback
		return nil
	}
}

// UseJSONMarshaler if neither `Marshaler` nor `GoYAMLMarshaler`
// nor `encoding.TextMarshaler` is implemented and `MarshalJSON()([]byte, error)` is implemented,
// call `MarshalJSON` to convert the returned `JSON` to `YAML` for processing.
func UseJSONMarshaler() EncodeOption {
	return func(e *Encoder) error {
		e.useJSONMarshaler = true
		return nil
	}
}

// WriteJSONTags names a struct's fields the way encoding/json names them
// wherever the `yaml` tag leaves off, so that a value read under [UseJSONTags]
// is written back under the names it was read from.
//
// The `json` tag names a field carrying no `yaml` tag, and an anonymous
// embedded struct that no tag names has its fields written into the mapping
// around it rather than under a key of their own. The `yaml` tag still wins
// wherever a field carries one.
//
// Without it the encoder writes the `yaml` tag's name, or the lowercased Go
// name, as go.yaml.in/yaml/v3 does.
func WriteJSONTags(write bool) EncodeOption {
	return func(e *Encoder) error {
		e.writeJSONTags = write

		return nil
	}
}

// WriteInferredNames writes a field that no tag names under its Go name,
// verbatim, the way encoding/json writes it, and mirrors [UseInferredNames] on
// the decoder.
//
// Without it such a field is written under its lowercased Go name, which is
// what go.yaml.in/yaml/v3 writes.
func WriteInferredNames(write bool) EncodeOption {
	return func(e *Encoder) error {
		e.writeInferredNames = write

		return nil
	}
}

// CustomMarshaler overrides any encoding process for the type specified in generics.
//
// NOTE: If type T implements MarshalYAML for pointer receiver, the type specified in CustomMarshaler must be *T.
// If RegisterCustomMarshaler and CustomMarshaler of EncodeOption are specified for the same type,
// the CustomMarshaler specified in EncodeOption takes precedence.
func CustomMarshaler[T any](marshaler func(T) ([]byte, error)) EncodeOption {
	return func(e *Encoder) error {
		var typ T
		e.customMarshalerMap[reflect.TypeOf(typ)] = func(v any) ([]byte, error) {
			return marshaler(customArg[T](v))
		}
		return nil
	}
}

// AutoInt automatically converts floating-point numbers to integers when the fractional part is zero.
// For example, a value of 1.0 will be encoded as 1.
func AutoInt() EncodeOption {
	return func(e *Encoder) error {
		e.autoInt = true
		return nil
	}
}

// OmitEmpty behaves in the same way as the interpretation of the omitempty tag in the encoding/json library.
// set on all the fields.
// In the current implementation, the omitempty tag is not implemented in the same way as encoding/json,
// so please specify this option if you expect the same behavior.
func OmitEmpty() EncodeOption {
	return func(e *Encoder) error {
		e.omitEmpty = true
		return nil
	}
}

// OmitZero forces the encoder to assume an `omitzero` struct tag is
// set on all the fields. See `Marshal` commentary for the `omitzero` tag logic.
func OmitZero() EncodeOption {
	return func(e *Encoder) error {
		e.omitZero = true
		return nil
	}
}

// SkipDuplicateMapKey writes the first of two map keys that are one YAML key,
// and drops the rest.
//
// A Go map can hold two keys YAML holds as one: int(1) and uint64(1) in a
// map[any]any, two NaN in a map[float64]T, one instant in two zones in a
// map[time.Time]T. Without this option the encoder returns an error wrapping
// [errors.ErrDuplicateKey] for such a map, as the decoder refuses a document
// that writes one key twice.
//
// The encoder orders a map's keys by their text and then by their Go type, so
// the first is the same whatever order the map iterates in. [AllowDuplicateMapKey]
// keeps the last on the way in instead: the decoder has every entry before it
// fills a map, and the encoder writes each key as it comes to it.
func SkipDuplicateMapKey() EncodeOption {
	return func(e *Encoder) error {
		e.skipDuplicateMapKey = true
		return nil
	}
}

// CommentPosition says where a comment stands relative to the value it belongs
// to.
//
// This is the one thing [ast] does not record. An [ast.CommentGroupNode] holds
// the text of a run of comments and nothing about its placement: a node has a
// single comment slot, and which of the three a comment is follows from which
// node the group was attached to. The parser works the placement out and the
// tree does not keep it, so a caller reading comments out of a document or
// writing them into one needs this to say what the tree cannot.
type CommentPosition int

const (
	// CommentHeadPosition is a comment on the lines above its value.
	CommentHeadPosition CommentPosition = CommentPosition(iota)
	// CommentLinePosition is a comment sharing a line with its value, after it.
	CommentLinePosition
	// CommentFootPosition is a comment on the lines below its value.
	CommentFootPosition
)

func (p CommentPosition) String() string {
	switch p {
	case CommentHeadPosition:
		return "Head"
	case CommentLinePosition:
		return "Line"
	case CommentFootPosition:
		return "Foot"
	default:
		return ""
	}
}

// LineComment returns a comment to write after its value, on the same line.
func LineComment(text string) *Comment {
	return &Comment{
		Texts:    []string{text},
		Position: CommentLinePosition,
	}
}

// HeadComment returns a comment to write above its value, one line per text.
func HeadComment(texts ...string) *Comment {
	return &Comment{
		Texts:    texts,
		Position: CommentHeadPosition,
	}
}

// FootComment returns a comment to write below its value, one line per text.
func FootComment(texts ...string) *Comment {
	return &Comment{
		Texts:    texts,
		Position: CommentFootPosition,
	}
}

// Comment is the text of a comment and where it goes, apart from any document.
//
// It carries what [ast.CommentGroupNode] carries -- one line of text per entry
// in Texts -- plus the [CommentPosition] the tree drops. Build one with
// [HeadComment], [LineComment] or [FootComment] rather than by hand, so that
// the texts and the position agree.
//
// Nothing here addresses a document. A Comment says what to write and whether
// it goes above, beside or below; [CommentMap] says which value it belongs to.
type Comment struct {
	// Texts is one line of comment per entry, written without the '#'.
	Texts []string
	// Position places the comment relative to its value.
	Position CommentPosition
}

// CommentMap holds the comments of a document, against the path of the value
// each belongs to.
//
// A key is a YAML path as [PathString] parses it -- "$.foo.bar", "$.baz[1]".
// [CommentToMap] fills one in while decoding, taking each key from the node's
// own path, and [WithComment] reads one while encoding. So a map produced by
// decoding is a map the encoder can write back.
//
// A value may carry more than one comment: a head comment and a line comment
// stand in different places and are two entries under the same key.
type CommentMap map[string][]*Comment

// WithComment writes the comments cm holds into the document being encoded.
//
// Each key is parsed as a YAML path and the comment is written at the value the
// path addresses. A key addressing no value in the document is passed over
// rather than reported: a map read from one document may be written to another
// that does not hold every value.
//
// The keys are full path expressions, so "$..a" and "$[*]" parse. Only the
// first value such a key reaches takes the comment, because a comment goes in
// one place -- prefer a key that addresses one value.
func WithComment(cm CommentMap) EncodeOption {
	return func(e *Encoder) error {
		commentMap := map[nodeFilter][]*Comment{}
		for k, v := range cm {
			path, err := yamlpath.PathString(k)
			if err != nil {
				return err
			}
			commentMap[path] = v
		}
		e.commentMap = commentMap
		return nil
	}
}

// CommentToMap collects the comments of the document being decoded into cm.
//
// Each comment is filed under the path of the value it belongs to, so the map
// is one [WithComment] can write back. cm must not be nil; the decoder fills
// the map the caller keeps rather than returning one.
//
// Decoding a document reads its values; this is how the comments come out with
// them, since a Go value has nowhere to hold a comment.
func CommentToMap(cm CommentMap) DecodeOption {
	return func(d *Decoder) error {
		if cm == nil {
			return ErrInvalidCommentMapValue
		}
		d.toCommentMap = cm
		return nil
	}
}
