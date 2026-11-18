// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The Go type a document is read into, built from the value the document was
// written from.
//
// # Why the destination is an axis
//
// Every property in this package decodes into an `any`, and so does every
// corpus-driven decode in yamlcorpus and in conformance/. Reading into an `any`
// and reading into a Go type are two paths through codec: one walks the token
// stream, the other gathers a tree and fills fields by reflection. They can
// disagree silently, and three defects were found living on the second one in a
// single afternoon of reading it.
//
// So [TargetFor] builds a type that fits the drawn value -- a struct with a
// field per mapping key, a slice of the item type, the obvious scalar -- and
// the property decodes the same document twice and compares. The value is
// already known; this reads it back through the other path.
//
// # What it will not build, and why that is not a hedge
//
// A struct field names its key in a `yaml:"..."` tag, and a tag is a string
// with commas for options. So a key holding a comma cannot be named at all, and
// neither can the empty key -- `yaml:""` asks for the field's own name. Two
// keys differing only in case cannot be named separately either, since field
// matching is case-insensitive.
//
// A mapping holding any of those falls back to map[string]any, which is a
// destination the decoder treats differently from `any` and worth reaching in
// its own right. The fallback is reported by [TargetFor] rather than silent:
// see [Target.Structs].

// TargetShape is which destination is built, where a document has more than
// one that fits.
//
// The shape is drawn rather than derived, because a document does not choose
// its destination -- a caller does, and the three below are the three a caller
// writes. Reading one document into all of them and comparing is what says
// whether the reflection path agrees with itself.
type TargetShape int

const (
	// ShapePlain fits the decoded value as closely as it can: a struct per
	// mapping, the item type per sequence, the obvious scalar.
	ShapePlain TargetShape = iota
	// ShapePointers is [ShapePlain] with every struct field a pointer, which
	// is the destination that makes the decoder allocate before it fills.
	ShapePointers
	// ShapeAnyKeyedMap reads every mapping into a map[any]any, the destination
	// gopkg.in/yaml.v2 made ordinary and the one that keeps a key's own type
	// instead of naming it.
	ShapeAnyKeyedMap
)

func (s TargetShape) String() string {
	switch s {
	case ShapePointers:
		return "pointer fields"
	case ShapeAnyKeyedMap:
		return "map[any]any"
	case ShapePlain:
		return "plain"
	default:
		return "plain"
	}
}

// Target is a Go type built to hold a decoded value.
type Target struct {
	// Type is the destination to decode into.
	Type reflect.Type
	// Shape is which destination was asked for.
	Shape TargetShape
	// Structs counts the mappings that became a struct. Zero means the
	// document reached none of the reflection path this axis exists for, which
	// is worth reporting rather than counting as coverage.
	Structs int
	// Maps counts the mappings that became a typed map -- a map[any]any under
	// [ShapeAnyKeyedMap]. Like Structs it is reflection reached, and the two
	// are counted apart because they are different code.
	Maps int
	// Fallbacks counts the mappings that could not become a struct.
	Fallbacks int
}

// Reached reports whether the destination puts any mapping through the
// reflection path, which is what this axis exists to exercise.
func (t Target) Reached() bool { return t.Structs > 0 || t.Maps > 0 }

var (
	anyType      = reflect.TypeFor[any]()
	stringType   = reflect.TypeFor[string]()
	boolType     = reflect.TypeFor[bool]()
	int64Type    = reflect.TypeFor[int64]()
	floatType    = reflect.TypeFor[float64]()
	anyMapType   = reflect.TypeFor[map[string]any]()
	timeType     = reflect.TypeFor[time.Time]()
	bytesType    = reflect.TypeFor[[]byte]()
	anyKeyType   = reflect.TypeFor[map[any]any]()
	bigIntType   = reflect.TypeFor[*big.Int]()
	bigFloatType = reflect.TypeFor[*big.Float]()
)

// TargetForDecoded builds the Go type a decoded value fits.
//
// It works from what the `any` path read rather than from the [Value] a
// document was written from, and that is the whole of why it is the only
// builder here. A destination built from the Value names its mapping fields by
// [KeyText], which is the core schema's spelling; a document declaring
// "%YAML 1.1" resolves "yes:" to the key "true", and a struct tagged `yaml:"yes"`
// then matches nothing. Building from the read that is already the yardstick
// cannot disagree with it.
//
// It also serves the enumerated shapes in yamlcorpus, which are written out as
// YAML and have no Value behind them -- and those are the documents the
// reflection path most needs, since two of the three defects found on it were
// merge keys, which the generator does not write.
func TargetForDecoded(v any) Target { return TargetForDecodedAs(v, ShapePlain) }

// TargetForDecodedAs builds the destination the shape asks for.
//
// [ShapePointers] and [ShapeAnyKeyedMap] are the two destinations a document
// cannot suggest on its own: the `any` path gives a map[string]any, or a
// map[any]any once a key is not a string, and nothing in the read says "this
// caller wanted pointers" or "this caller wanted a map[any]any for every
// mapping". They are drawn beside the value and the style, and the property
// reads one document into whichever came up.
func TargetForDecodedAs(v any, shape TargetShape) Target {
	t := Target{Shape: shape}
	t.Type = t.typeOfDecoded(v)

	return t
}

func (t *Target) typeOfDecoded(v any) reflect.Type {
	switch n := v.(type) {
	case bool:
		return boolType
	case string:
		return stringType
	case int, int64, uint64:
		return int64Type
	case float64:
		return floatType
	case time.Time:
		// `!!timestamp` and `!!binary` are the two tags that hand back a Go
		// type no schema resolves, so a field typed for them is the only
		// destination that reads one without an any.
		return timeType
	case []byte:
		return bytesType
	case []any:
		return reflect.SliceOf(t.decodedItemType(n))
	case map[string]any:
		return t.decodedMapType(n)
	case map[any]any:
		// A mapping the `any` path widened on a key that is not a string. A
		// destination names its keys as a string-keyed map does, by KeyText's
		// rule, which is how every mapping reached one before the widening.
		// Without this arm every widened reading built no target at all, and
		// the enumerated shapes reaching the reflection path fell from 162 to
		// 129 with nothing reported.
		named, distinct := keyedByName(n)
		if !distinct && t.Shape != ShapeAnyKeyedMap {
			// Two keys take one name -- uint64(1) and "1" -- and no struct
			// field or map[string]any entry holds both.
			t.Fallbacks++

			return anyKeyType
		}

		return t.decodedMapType(named)
	default:
		return anyType
	}
}

// keyedByName returns m keyed by the name each key takes in a string-keyed map,
// and whether every key took a name of its own.
func keyedByName(m map[any]any) (map[string]any, bool) {
	out := make(map[string]any, len(m))

	for key, value := range m {
		name := keyString(reflect.ValueOf(key))
		if _, taken := out[name]; taken {
			return out, false
		}
		out[name] = value
	}

	return out, true
}

func (t *Target) decodedItemType(items []any) reflect.Type {
	if len(items) == 0 {
		return anyType
	}

	first := t.typeOfDecoded(items[0])
	for _, item := range items[1:] {
		if t.typeOfDecoded(item) != first {
			return anyType
		}
	}

	return first
}

func (t *Target) decodedMapType(m map[string]any) reflect.Type {
	if t.Shape == ShapeAnyKeyedMap {
		// Every mapping, including the empty one: a map is a destination with
		// something to fill even when the document filled nothing, which is
		// the case nameable turns away for a struct.
		t.Maps++

		return anyKeyType
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if !nameable(keys) {
		t.Fallbacks++

		return anyMapType
	}

	fields := make([]reflect.StructField, 0, len(keys))
	for i, key := range keys {
		field := t.typeOfDecoded(m[key])
		if t.Shape == ShapePointers {
			field = reflect.PointerTo(field)
		}

		fields = append(fields, reflect.StructField{
			Name: "F" + strconv.Itoa(i),
			Type: field,
			Tag:  reflect.StructTag(`yaml:"` + key + `"`),
		})
	}

	t.Structs++

	return reflect.StructOf(fields)
}

// nameable reports whether every key can be written in a struct tag and matched
// back to one field.
func nameable(keys []string) bool {
	if len(keys) == 0 {
		// An empty struct is a destination with nothing to fill, so it says
		// nothing about the reflection path. A map does.
		return false
	}

	folded := make(map[string]struct{}, len(keys))

	for _, key := range keys {
		// "-" is the tag that asks for a field to be left out, so a key
		// spelled that way would name a field nothing ever fills.
		if key == "" || key == "-" || strings.ContainsAny(key, `,"'`+"\n\r\t\\`") {
			return false
		}

		lower := strings.ToLower(key)
		if _, clash := folded[lower]; clash {
			return false
		}
		folded[lower] = struct{}{}
	}

	return true
}

// NamedKeys returns v with every mapping keyed by name, as a map[string]any, so
// encoding/json can write it.
//
// A mapping decoded into an `any` widens to a map[any]any on its first key that
// is not a string, and encoding/json refuses a map[any]any. The stored corpus
// and every comparison against it name each key by [KeyText]'s rule, so a
// mapping whose keys are all strings encodes as it did before the widening. It
// is [Normalize] applied to a decoded value, which holds no struct for
// Normalize to turn into a map.
func NamedKeys(v any) any {
	return Normalize(reflect.ValueOf(v))
}

// Normalize turns a value read into a Go type back into the shape [Value.Decoded]
// speaks, so the two can be compared.
//
// A struct becomes a map keyed by each field's yaml tag; everything else keeps
// its own shape. Numbers are left as they are and compared numerically, because
// the destination decides the Go type -- an int64 field holds 1 where an `any`
// holds uint64(1), and that is the destination doing its job rather than a
// disagreement.
func Normalize(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}

		return Normalize(v.Elem())
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}

		if v.Type() == bigIntType || v.Type() == bigFloatType {
			// These two stay pointers because the `any` path hands them back
			// as pointers, and Normalize exists to make the two sides of the
			// comparison the same shape.
			//
			// It matters beyond the shape for a big.Float. Following the
			// pointer lands on the named struct below, which hands back the
			// big.Float itself -- and reflect's equality then compares its
			// Accuracy field, which records how the last rounding went rather
			// than what the number is. sameValue compares a *big.Float by Cmp
			// and had no chance to.
			//
			// Everything else is followed. A *time.Time field under
			// ShapePointers holds what an `any` holds by value, and leaving it
			// a pointer made the two compare unequal while printing alike --
			// fmt calls time.Time's GoString through the pointer.
			return v.Interface()
		}

		return Normalize(v.Elem())
	case reflect.Struct:
		if v.Type().Name() != "" {
			// A named struct is somebody else's type -- a big.Float, a
			// time.Time -- and its fields are unexported. The types this
			// package builds with reflect.StructOf have no name.
			return v.Interface()
		}

		out := make(map[string]any, v.NumField())
		for i := range v.NumField() {
			name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
			out[name] = Normalize(v.Field(i))
		}

		return out
	case reflect.Map:
		return normalizeMap(v)
	case reflect.Slice, reflect.Array:
		out := make([]any, 0, v.Len())
		for i := range v.Len() {
			out = append(out, Normalize(v.Index(i)))
		}

		return out
	default:
		return v.Interface()
	}
}

// Collision stands in a normalized map where two keys spell the same name.
//
// A map[any]any can hold uint64(1) and "1" at once -- two nodes under 3.2.1.1,
// and the document "1: x" over "\"1\": y" reads into one that way. [keyString]
// names both "1", because the `any` path names both "1" and the two have to
// compare.
//
// Writing one over the other lost an entry and let Go's map iteration order
// pick which, so a comparison over such a map passed or failed at random:
// TestTheEnumeratedShapesReadIntoAGoType agreed on three runs in five and
// disagreed on the other two, over the same document. Both values are kept
// here instead, ordered by their %v spelling so that two walks of the same map
// build the same Collision.
type Collision struct {
	Name   string
	Values []any
}

func normalizeMap(v reflect.Value) map[string]any {
	out := make(map[string]any, v.Len())

	// MapRange and not MapKeys then MapIndex: a NaN key never finds itself, so looking its value up read nil, and a
	// document keyed on ".nan" compared as having lost the value its key holds.
	var collided map[string][]any
	for iter := v.MapRange(); iter.Next(); {
		key := iter.Key()
		name := keyString(key)
		val := Normalize(iter.Value())

		if _, taken := out[name]; !taken {
			out[name] = val

			continue
		}

		if collided == nil {
			collided = map[string][]any{}
		}
		if _, seen := collided[name]; !seen {
			collided[name] = []any{out[name]}
		}
		collided[name] = append(collided[name], val)
	}

	for name, values := range collided {
		sort.Slice(values, func(i, j int) bool {
			return fmt.Sprintf("%v", values[i]) < fmt.Sprintf("%v", values[j])
		})
		out[name] = Collision{Name: name, Values: values}
	}

	return out
}

// keyString spells a map key the way [KeyText] spells one.
func keyString(v reflect.Value) string {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	if !v.IsValid() {
		return "null"
	}

	// The names below mirror KeyText, because a map[any]any keeps the key's own
	// type where the `any` path names it -- so normalizing one to compare
	// against the other has to spell the name the same way.
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return floatKeyText(v.Float())
	}

	if v.CanInterface() {
		// A time.Time is named in RFC 3339 in its own zone, as KeyText names a
		// Timestamp and ToJSON writes one. Its String is Go's own layout.
		if stamp, ok := reflect.TypeAssert[time.Time](v); ok {
			return stamp.Format(time.RFC3339Nano)
		}

		// A *big.Float is named as KeyText names a BigFloat. Its String keeps
		// ten significant digits, which names two keys alike.
		if f, ok := reflect.TypeAssert[*big.Float](v); ok {
			return bigFloatKeyText(f)
		}

		// A *big.Int names itself through String.
		if s, ok := reflect.TypeAssert[interface{ String() string }](v); ok {
			return s.String()
		}

		return fmt.Sprintf("%v", v.Interface())
	}

	return strconv.FormatFloat(math.NaN(), 'g', -1, 64)
}
