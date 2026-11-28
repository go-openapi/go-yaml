// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"io"
	"reflect"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/yamlpath"
	"github.com/go-openapi/go-yaml/parser"
)

// DecodeOption sets an option of a [Decoder], or of a call to [Unmarshal] or [NodeToValue].
type DecodeOption func(d *Decoder) error

// ReferenceReaders reads the anchors declared in readers, so that the decoded document can alias them.
func ReferenceReaders(readers ...io.Reader) DecodeOption {
	return func(d *Decoder) error {
		d.referenceReaders = append(d.referenceReaders, readers...)
		return nil
	}
}

// ReferenceFiles reads the anchors declared in files, so that the decoded document can alias them.
func ReferenceFiles(files ...string) DecodeOption {
	return func(d *Decoder) error {
		d.referenceFiles = files
		return nil
	}
}

// ReferenceDirs reads the anchors declared in the .yml and .yaml files of dirs, so that the decoded document can alias
// them. Use [RecursiveDir] to read their subdirectories too.
func ReferenceDirs(dirs ...string) DecodeOption {
	return func(d *Decoder) error {
		d.referenceDirs = dirs
		return nil
	}
}

// RecursiveDir makes [ReferenceDirs] read the subdirectories of its directories too.
func RecursiveDir(isRecursive bool) DecodeOption {
	return func(d *Decoder) error {
		d.isRecursiveDir = isRecursive
		return nil
	}
}

// Validator calls validate with each struct the decoder fills, once it is filled, passing the struct by value.
// An error validate returns comes back as [github.com/go-openapi/go-yaml/errors.ErrValidation],
// with a position and validate's message. [errors.Is] and [errors.As] reach validate's own error through it.
//
// An error that is a slice whose elements have a StructField() string method points at the value of the first field
// it names, or at the field's key when the value is a mapping or a sequence. When the document leaves that field out,
// the error points at the key of the entry that writes the struct, or at the struct's first key. Any other error points
// at the struct's mapping. github.com/go-playground/validator/v10 returns such errors, so pass its Struct method:
//
//	validate := validator.New()
//	dec := codec.NewDecoder(r, codec.Validator(validate.Struct))
func Validator(validate func(any) error) DecodeOption {
	return func(d *Decoder) error {
		d.validator = validate
		return nil
	}
}

// Strict is [DisallowUnknownField].
func Strict() DecodeOption {
	return func(d *Decoder) error {
		d.disallowUnknownField = true
		return nil
	}
}

// DisallowUnknownField returns an error for a mapping key that names no exported field of the struct it decodes into.
// A field tagged "-" names nothing. Use [AllowFieldPrefixes] to let some keys through.
//
// Under [UseJSONTags], a name that two embedded structs offer at the same depth, and that neither tag settles, reaches
// no field. Its key is then unknown and fails the decode, as with encoding/json/v2's RejectUnknownMembers.
func DisallowUnknownField() DecodeOption {
	return func(d *Decoder) error {
		d.disallowUnknownField = true
		return nil
	}
}

// AllowFieldPrefixes lets [DisallowUnknownField] accept an unknown key that starts with one of prefixes, such as "x-".
func AllowFieldPrefixes(prefixes ...string) DecodeOption {
	return func(d *Decoder) error {
		d.allowedFieldPrefixes = append(d.allowedFieldPrefixes, prefixes...)
		return nil
	}
}

// AllowDuplicateMapKey accepts a document that repeats a mapping key, and keeps the last value written under it.
//
// By default a repeated key fails the decode with [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey],
// as YAML 1.2.2 section 3.2.1.1 requires.
// The decoder finds the repeat when the mapping closes, so a struct, a map or an any still holds the whole
// mapping, with the last value and without its "<<" merges, and nothing after it in the document is decoded.
// Under this option a Go map and a struct field hold the last value,
// and a [MapSlice] holds it at the place the key was first written.
//
// Two keys that YAML tells apart but that share one name, such as 1 and "1", count as a repeat for a destination keyed
// by name: a struct, a map[string]T, an inline map, or an any under [UseStringKeys].
// An any and a [MapSlice] key their entries by value, and hold both keys with or without this option.
//
// [ToJSON] and [ToJSONTokens] keep the first value instead, under [parser.WithAllowDuplicateMapKey].
// They hand each token over as the parser reaches it, so the caller has the first value before the repeat is read.
// A document read both ways under the option can therefore give two values for one repeated key.
// Leave the option off where a decode and a conversion must agree.
func AllowDuplicateMapKey() DecodeOption {
	return func(d *Decoder) error {
		d.allowDuplicateMapKey = true
		return nil
	}
}

// WithParserOptions passes options straight to the parser the decode runs.
//
// The decoder already turns some decode options into parser options: [AllowDuplicateMapKey] sets
// [parser.WithAllowDuplicateMapKey], and [CommentToMap] sets [parser.WithComments]. Use WithParserOptions for the rest
// of the parser's options.
//
// Most callers want the YAML version a document is read under:
//
//	codec.Unmarshal(src, &v,
//		codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
//
// Without it, only a "%YAML 1.1" directive selects the 1.1 schema, and a document without one is read as YAML 1.2.
//
// The options apply after the ones the decoder sets, in the order given, so where two options set the same thing the
// later one wins. Each parser option the decoder sets only turns a feature on, and no parser option turns one off,
// so WithParserOptions can add to them but cannot undo them.
func WithParserOptions(opts ...parser.Option) DecodeOption {
	return func(d *Decoder) error {
		d.extraParserOptions = append(d.extraParserOptions, opts...)

		return nil
	}
}

// ShareAliases gives every alias of one anchor the same Go value, instead of a value of its own.
//
// By default the aliases are independent: "first: *b" and "second: *b" give two maps, and writing through one leaves
// the other alone. Under ShareAliases both names hold one map, and writing through either changes both.
//
// Use it for two reasons:
//
//   - The anchors survive a round trip through a Go value. [MarshalAnchor] and the ",anchor" and ",alias" struct tags
//     find an anchor by the address of its value, so the encoder writes "*name" only where one value stands under two
//     names. Without this option, a decoded document encoded again writes the anchored node in full at each alias.
//     A document read into an [ast] tree and rendered keeps its anchors either way: this concerns the Go value only.
//   - An alias costs nothing. A document written to exhaust memory relies on each alias building its own value
//     (see [errors.ErrExcessiveAliasing]). A shared value is built once, however many aliases name it.
//
// Do not use it on a value the program writes to afterwards, unless you want the writes shared.
func ShareAliases() DecodeOption {
	return func(d *Decoder) error {
		d.shareAliases = true

		return nil
	}
}

// UseOrderedMap decodes a mapping into a [MapSlice] wherever the destination does not name a type, as for an any,
// so that the mapping keeps the order of its keys.
func UseOrderedMap() DecodeOption {
	return func(d *Decoder) error {
		d.useOrderedMap = true
		return nil
	}
}

// UseStringKeys reads every mapping key as text, so a map with an interface key type holds strings instead of the Go
// types the keys resolve to.
//
// Without it, "1.5: a" decoded into a map[any]any gives the key float64(1.5), "true: a" gives bool(true), and a key the
// map's own type cannot hold is an error. With it both keys are strings, spelled as a map[string]any spells them, as a
// JSON object needs. A map whose key type is named, such as a map[float64]any, is left alone, because the caller asked
// for that type.
//
// It applies to a [MapSlice] too, whose MapItem.Key otherwise holds the resolved key. There "1: a" and "\"1\": b"
// become two entries under one name, because a MapSlice can hold both.
//
// It does not make a collection usable as a key. Go cannot hash a slice or a map, so "? [a]" is an error either way.
// Use [parser.WithJSONCompatible] to reject such keys when the document is parsed.
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
// The `yaml` tag always wins, so the option adds a way to name a field and takes none away. A field carrying both tags
// reads the same with or without the option, and `yaml:"-"` still hides a field that the `json` tag names.
//
// Without the option, the decoder reads the `yaml` tag alone, as go.yaml.in/yaml/v3 does, and promotes an embedded
// struct only where a tag says `,inline`. A type tagged for JSON alone then takes the lowercased Go name for every
// field, and promotes nothing, because encoding/json needs no inline marker. github.com/go-openapi/spec is such a type:
// without this option, decoding an OpenAPI document into spec.Swagger fills the outer map and leaves every
// specification in it zero.
//
// The option does not make the decoder call UnmarshalJSON: [UseJSONUnmarshaler] does that, after converting the
// document to JSON. UseJSONTags fills the fields directly, so a type whose UnmarshalJSON does more than assign them
// misses that work. spec.Swagger, for one, gathers its "x-" keys into its extensions.
//
// [ToJSON] reads no Go type at all, so no tag option applies to it. To decode a document into a type through its
// UnmarshalJSON, convert it with [ToJSON] and pass the result to encoding/json.
//
// A mapping key matches a field name exactly or matches nothing, in every mode, as in go.yaml.in/yaml/v3 and
// encoding/json/v2. encoding/json's v1 also accepted a key that differs from a name only in case; this codec does not.
//
// The `json` tag's `,string` option reads a number from a string, as encoding/json/v2 does: `n: "12"` fills an int
// and `n: 12` is rejected. It applies to a number or a pointer to one, and fails the decode of any other field.
//
// A field that neither tag names keeps its lowercased Go name. [UseInferredNames] changes that name.
//
// [WriteJSONTags] does the same for the encoder. Pass both, and a value read from a document is written back under
// the names it was read from.
func UseJSONTags(use bool) DecodeOption {
	return func(d *Decoder) error {
		d.useJSONTags = use

		return nil
	}
}

// UseInferredNames names a field that no tag names after its Go name, verbatim, as encoding/json does.
//
// Without it, such a field takes its Go name lowercased, as in go.yaml.in/yaml/v3: a field declared FieldName is
// written "fieldname", and a document writing "FieldName" reaches nothing. Under the option, the two swap over.
//
// The option changes names and nothing else. [UseJSONTags] decides which fields a decode considers, how an anonymous
// embedded struct is read, and how a repeated name is resolved. The two options are independent: a struct with no
// tags at all needs only UseInferredNames.
func UseInferredNames(use bool) DecodeOption {
	return func(d *Decoder) error {
		d.useInferredNames = use

		return nil
	}
}

// UseJSONUnmarshaler decodes a type that implements [encoding/json.Unmarshaler], and none of this package's
// unmarshaler interfaces, by converting its node to JSON and calling UnmarshalJSON.
func UseJSONUnmarshaler() DecodeOption {
	return func(d *Decoder) error {
		d.useJSONUnmarshaler = true
		return nil
	}
}

// CustomUnmarshaler decodes every T with unmarshaler, which receives the YAML text of the node, in place of any other
// decoding of T. The text is the one an [Unmarshaler] receives.
//
// It takes precedence over a function registered for the same type with [RegisterCustomUnmarshaler].
func CustomUnmarshaler[T any](unmarshaler func(*T, []byte) error) DecodeOption {
	return func(d *Decoder) error {
		var typ *T
		d.customUnmarshalerMap[reflect.TypeOf(typ)] = func(v any, b []byte) error {
			return unmarshaler(codecutils.CustomArg[*T](v), b)
		}
		return nil
	}
}

// EncodeOption sets an option of an [Encoder], or of a call to [Marshal] or [ValueToNode].
type EncodeOption func(e *Encoder) error

// Indent sets the number of spaces of each indentation level. The default is [DefaultIndentSpaces].
func Indent(spaces int) EncodeOption {
	return func(e *Encoder) error {
		e.indentNum = spaces
		return nil
	}
}

// IndentSequence indents a block sequence under a mapping key by one level, as set by [Indent]. By default the "- "
// of each entry stands in the key's column.
func IndentSequence(indent bool) EncodeOption {
	return func(e *Encoder) error {
		e.indentSequence = indent
		return nil
	}
}

// UseSingleQuote quotes a string that needs quoting with single quotes. A string holding a character that only an
// escape can write is still double-quoted.
func UseSingleQuote(sq bool) EncodeOption {
	return func(e *Encoder) error {
		e.singleQuote = sq
		return nil
	}
}

// Flow writes every mapping and sequence in flow style: {k: v} and [a, b].
func Flow(isFlowStyle bool) EncodeOption {
	return func(e *Encoder) error {
		e.isFlowStyle = isFlowStyle
		return nil
	}
}

// WithSmartAnchor writes an anchor on the first map value that a pointer reaches, and an alias at every later value
// with the same pointer. The anchor takes the name of the map key, with a suffix where two keys share a name.
// The option is experimental, and cannot be combined with the anchor and alias struct tags.
//
// A value decoded by [Unmarshal] shares a pointer only where the decode gave two names one value, so pass
// [ShareAliases] to the decode. Without it, every alias has a value of its own and the option finds no pointer to
// share.
func WithSmartAnchor() EncodeOption {
	return func(e *Encoder) error {
		e.enableSmartAnchor = true
		return nil
	}
}

// UseLiteralStyleIfMultiline writes a string that spans several lines as a literal block scalar ("|").
//
// Two kinds of string stay double-quoted, because the encoder has no block scalar that reads back as the same string.
// A string holding a carriage return or a byte order mark is one: a reader turns "\r\n" into "\n".
// A string whose first line of text starts with a space is the other:
// a reader takes the block's indentation from that line, and the encoder writes no indentation indicator.
func UseLiteralStyleIfMultiline(useLiteralStyleIfMultiline bool) EncodeOption {
	return func(e *Encoder) error {
		e.useLiteralStyleIfMultiline = useLiteralStyleIfMultiline
		return nil
	}
}

// JSON writes the document in flow style with every string quoted, so that the output is also JSON.
func JSON() EncodeOption {
	return func(e *Encoder) error {
		e.isJSONStyle = true
		e.isFlowStyle = true
		return nil
	}
}

// MarshalAnchor calls callback for each anchor the encoder writes, with the anchor node and the value it anchors.
//
// The encoder finds an anchor by the address of its value. A decoded document encoded again therefore keeps its
// anchors only where the decode gave every name of an anchor one value. Pass [ShareAliases] to [Unmarshal] for that.
// Without it, each alias has a value of its own, and the anchored node is written in full at every alias.
func MarshalAnchor(callback func(*ast.AnchorNode, any) error) EncodeOption {
	return func(e *Encoder) error {
		e.anchorCallback = callback
		return nil
	}
}

// UseJSONMarshaler encodes a type that implements [encoding/json.Marshaler], and neither this package's marshaler
// interfaces nor [encoding.TextMarshaler], by calling MarshalJSON and converting the JSON it returns to YAML.
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
// The `json` tag names a field that carries no `yaml` tag, and an anonymous embedded struct that no tag names has its
// fields written into the mapping around it, not under a key of its own. The `yaml` tag still wins wherever a field
// carries one.
//
// The `json` tag's flags follow encoding/json/v2. `,string` writes a number as a string, such as `n: "12"`.
// `,omitempty` drops a field that encodes as null, an empty string, an empty mapping or an empty sequence, and keeps
// false and 0. `yaml:",omitempty"` keeps the rule of go.yaml.in/yaml/v3, which drops false and 0 too.
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

// CustomMarshaler encodes every T with marshaler, which returns the YAML text to write, in place of any other encoding
// of T.
//
// Where T implements MarshalYAML on a pointer receiver, give *T as the type parameter. The option takes precedence over
// a function registered for the same type with [RegisterCustomMarshaler].
func CustomMarshaler[T any](marshaler func(T) ([]byte, error)) EncodeOption {
	return func(e *Encoder) error {
		var typ T
		e.customMarshalerMap[reflect.TypeOf(typ)] = func(v any) ([]byte, error) {
			return marshaler(codecutils.CustomArg[T](v))
		}
		return nil
	}
}

// AutoInt writes a float with no fractional part as an integer: 1.0 is written 1.
func AutoInt() EncodeOption {
	return func(e *Encoder) error {
		e.autoInt = true
		return nil
	}
}

// OmitEmpty skips every struct field that encoding/json v1's omitempty would skip: a false bool, a zero number, an
// empty string, a nil pointer or interface, and an empty slice or map.
//
// It applies to every field, tagged or not. This package's omitempty tag also skips a value whose IsZero method
// returns true, and a struct whose fields would all be skipped (see [Marshal]).
//
// Under [WriteJSONTags], a field with a `json` tag follows encoding/json/v2's omitempty instead, as its own
// `,omitempty` flag does: it is skipped when it encodes as null, an empty string, an empty mapping or an empty
// sequence, and a false bool or a zero number is written.
func OmitEmpty() EncodeOption {
	return func(e *Encoder) error {
		e.omitEmpty = true
		return nil
	}
}

// OmitZero skips every struct field that the omitzero tag would skip, as if each field carried it. See [Marshal] for
// the rule.
func OmitZero() EncodeOption {
	return func(e *Encoder) error {
		e.omitZero = true
		return nil
	}
}

// SkipDuplicateMapKey writes the first of two map keys that YAML reads as one key, and drops the rest.
//
// A Go map can hold two keys that YAML reads as one: int(1) and uint64(1) in a map[any]any, two NaN in a
// map[float64]T, or one instant in two zones in a map[time.Time]T. Without this option, the encoder returns an error
// wrapping [errors.ErrDuplicateKey] for such a map, as the decoder does for a document that writes one key twice.
//
// The encoder orders a map's keys by their text and then by their Go type, so the same key comes first whatever order
// the map iterates in. [AllowDuplicateMapKey] keeps the last value on decoding instead: the decoder has every entry
// before it fills a map, while the encoder writes each key as it comes to it.
func SkipDuplicateMapKey() EncodeOption {
	return func(e *Encoder) error {
		e.skipDuplicateMapKey = true
		return nil
	}
}

// WithComment writes the comments cm holds into the document being encoded.
//
// Each key is parsed as a YAML path, and the comment is written at the value the path addresses. A key that addresses
// no value in the document is skipped without an error, so a map read from one document can be written to another
// that lacks some of its values.
//
// The keys are full path expressions, so "$..a" and "$[*]" parse. A comment goes in one place, so only the first value
// such a key reaches takes the comment. Prefer keys that address one value each.
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
// Each comment is filed under the path of the value it belongs to, so [WithComment] can write the map back. cm must not
// be nil: the decoder fills the caller's map and does not return one.
//
// A Go value has nowhere to hold a comment, so use CommentToMap to keep a document's comments when you decode it.
func CommentToMap(cm CommentMap) DecodeOption {
	return func(d *Decoder) error {
		if cm == nil {
			return yamlerrors.ErrInvalidCommentMapValue
		}
		d.toCommentMap = cm
		return nil
	}
}
