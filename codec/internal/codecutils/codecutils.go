// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package codecutils holds the codec's helpers that read and convert Go values: reflection on destinations,
// numbers that do not fit a machine word, the identity of a map key, the alias budget, reading a source whole.
//
// They know nothing of the codec's own types. The helpers that name a key by its Base64 text stay in the codec.
package codecutils

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// aliasBudgetFloor is the budget of an empty document, which AliasBudget adds 32 a byte to.
const aliasBudgetFloor = 1024

// AliasBudget bounds what a decode may build from a document of n bytes.
//
// An alias names a node and the value it stands for is built again wherever the
// alias appears, so a chain of them multiplies rather than adds: 259 bytes of
// YAML name 100,000 values and 423 bytes name 387 million. maxDecodeDepth does
// not see it -- nine levels of aliases is nine deep and as wide as it likes.
//
// Reading a document that holds no alias costs well under one step a byte, and
// the widest of the measured workloads reaches 0.19, so 32 a byte leaves a
// document written by a person alone and closes early on one written to exhaust
// memory.
//
// It counts what a decode builds and not what an alias builds, because an alias
// is the only thing that can build more than the document holds. Reading into an
// interface hands the same value to every alias of one anchor and so builds
// nothing extra, which is why that path does not reach this and why it reads a
// 423-byte document naming 387 million values in no time at all.
func AliasBudget(n int) int {
	return aliasBudgetFloor + 32*n
}

// CastToFloatValue is castToFloat without a decoder, for the walking builder.
func CastToFloatValue(v any) any {
	switch vv := v.(type) {
	case int:
		return float64(vv)
	case int8:
		return float64(vv)
	case int16:
		return float64(vv)
	case int32:
		return float64(vv)
	case int64:
		return float64(vv)
	case uint:
		return float64(vv)
	case uint8:
		return float64(vv)
	case uint16:
		return float64(vv)
	case uint32:
		return float64(vv)
	case uint64:
		return float64(vv)
	case float32:
		return float64(vv)
	case float64:
		return vv
	case *big.Int:
		f, _ := new(big.Float).SetInt(vv).Float64()

		return f
	case *big.Float:
		if f, exact := floatOf(vv); exact {
			return f
		}

		// The number outgrew float64 in one direction or the other, so it keeps
		// the width it was read at, as castToInteger keeps a big.Int and as the
		// same number untagged already does. Narrowed, "!!float 1e+310" came
		// back as +Inf and "!!float 1e-400" as zero.
		return vv
	case string:
		// The text came from a node "!!float" stands over, which the resolver
		// may have left as a string: under a "%YAML 1.1" directive "1e-330" is
		// a string, because 1.1 spells the exponent form with a "." in it.
		f, err := strconv.ParseFloat(vv, 64)
		if err == nil && (f != 0 || spellsZero(vv)) {
			return f
		}
		// Outside float64's range in one direction or the other. strconv
		// reports ErrRange and an infinity for an overflow, and a plain zero
		// with no error at all for an underflow, so the two are told apart by
		// the digits.
		if b, ok := token.ParseBigFloat(vv, token.ScalarType(vv, token.Schema12)); ok {
			return b
		}

		return f
	}
	return 0
}

// spellsZero reports whether text writes the number zero, which is what tells
// nought from a number too small for a float64: strconv.ParseFloat returns zero
// and no error for both.
func spellsZero(text string) bool {
	for _, r := range text {
		if r >= '1' && r <= '9' {
			return false
		}
	}

	return true
}

// floatOf narrows a big.Float, and reports false where float64 has no room for
// it: an overflow gives an infinity and an underflow a zero, and neither is the
// number.
func floatOf(v *big.Float) (float64, bool) {
	f, _ := v.Float64()
	if math.IsInf(f, 0) && !v.IsInf() {
		return 0, false
	}
	if f == 0 && v.Sign() != 0 {
		return 0, false
	}

	return f, true
}

// IsPointer reports whether rv holds a pointer a decode can write through.
// A nil target holds nothing, and reflect panics on asking its type.
func IsPointer(rv reflect.Value) bool {
	return rv.IsValid() && rv.Kind() == reflect.Pointer
}

// HashableKey reports whether a decoded key can stand as a Go map key.
//
// reflect.Type.Comparable and not a switch on the kind: a slice, a map and a
// func are the kinds Go cannot hash, but a struct or an array holding one of
// them cannot be hashed either, and the kind alone reads "struct". A MapSlice
// is such a struct, so a mapping keyed on a mapping read under UseOrderedMap
// passed the switch and panicked comparing two of them.
func HashableKey(key any) bool {
	if key == nil {
		return true
	}

	return reflect.TypeOf(key).Comparable()
}

// NumberKey returns the kind and the canonical name a Go number is keyed by in
// YAML, and whether v is a number at all. The names are the ones the parser
// gives the same number written in a document: [token.KeyNameOfFloat] and
// [token.KeyNameOfBigFloat] for a float, the decimal digits for an integer.
func NumberKey(v any) (token.KeyKind, string, bool) {
	switch n := v.(type) {
	case *big.Int:
		if n == nil {
			return token.KeyOther, "", false
		}

		return token.KeyInt, n.String(), true
	case *big.Float:
		if n == nil {
			return token.KeyOther, "", false
		}

		return token.KeyFloat, token.KeyNameOfBigFloat(n), true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return token.KeyInt, strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return token.KeyInt, strconv.FormatUint(rv.Uint(), 10), true
	case reflect.Float32:
		return token.KeyFloat, token.KeyNameOfFloat(rv.Float(), 32), true
	case reflect.Float64:
		return token.KeyFloat, token.KeyNameOfFloat(rv.Float(), 64), true
	default:
		return token.KeyOther, "", false
	}
}

// ByteArray reports whether t is an array of bytes, which a "!!binary" fills as
// it fills a byte slice.
func ByteArray(t reflect.Type) bool {
	return t.Kind() == reflect.Array && t.Elem().Kind() == reflect.Uint8
}

// CopyIntoByteArray fills a byte array from b, and reports a length the array
// cannot hold rather than writing part of it.
func CopyIntoByteArray(dst reflect.Value, b []byte, src ast.Node) error {
	if len(b) != dst.Len() {
		return yamlerrors.NewSyntax(
			fmt.Sprintf("a %d byte array cannot hold the %d bytes this !!binary carries", dst.Len(), len(b)),
			src.GetToken(),
		)
	}
	reflect.Copy(dst, reflect.ValueOf(b))

	return nil
}

// ByteSlice reports whether t is a slice of bytes a []byte converts to. A
// []MyByte is not one: Go converts between slice types only where the element
// types are identical.
func ByteSlice(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && reflect.TypeFor[[]byte]().ConvertibleTo(t)
}

// IsBigNumber reports whether v holds a number wider than a native type.
func IsBigNumber(v reflect.Value) bool {
	switch v.Interface().(type) {
	case *big.Int, *big.Float:
		return true
	default:
		return false
	}
}

func IsFloatKind(k reflect.Kind) bool { return k == reflect.Float32 || k == reflect.Float64 }

// ConvertBigNumber converts a number the AST read into a [big.Int] or a
// [big.Float], which it does for one no native type holds.
//
// A float destination takes the nearest float64, and is refused where the
// number reaches past what one holds. strconv.ParseFloat answers the same
// values and returns ErrRange with them, and dropping that gave a document
// holding 1e400 a field reading +Inf with nothing said. A caller who wants the
// number whole decodes into a *big.Int or a *big.Float, which take it exactly.
//
// ±Inf is a float64 like any other and a document naming one gets it: ".inf"
// resolves to an ast.InfinityNode, which holds a float64 already and never
// reaches here. Only a finite big.Int or big.Float does, and a float64 of ±Inf
// from finite digits has lost the number rather than named it.
//
// A string destination takes the digits. Anything else is left to convertValue's
// own rules, which end in a type mismatch.
func ConvertBigNumber(v reflect.Value, typ reflect.Type) (reflect.Value, bool) {
	var (
		f    float64
		text string
	)
	switch n := v.Interface().(type) {
	case *big.Int:
		f, _ = new(big.Float).SetInt(n).Float64()
		text = n.String()
	case *big.Float:
		f, _ = n.Float64()
		text = n.Text('g', -1)
	default:
		return reflect.Value{}, false
	}

	switch typ.Kind() {
	case reflect.Float32, reflect.Float64:
		if outOfFloatRange(f, text) {
			return reflect.Value{}, false
		}

		return reflect.ValueOf(f).Convert(typ), true
	case reflect.String:
		return reflect.ValueOf(text).Convert(typ), true
	default:
		return reflect.Value{}, false
	}
}

// outOfFloatRange reports whether the float64 nearest to a finite number has
// lost it rather than rounded it: infinite, or zero from digits that are not.
func outOfFloatRange(f float64, text string) bool {
	if math.IsInf(f, 0) {
		return true
	}

	return f == 0 && strings.ContainsFunc(text, func(r rune) bool {
		return r >= '1' && r <= '9'
	})
}

// ConvertToKind converts v to typ, a type that is not a string kind.
//
// A string converts to a float kind when strconv.ParseFloat reads it, which
// is how a quoted "1e3" fills a float field. The result has typ itself, so a
// named float type such as `type Ratio float64` is filled as well.
func ConvertToKind(v reflect.Value, typ reflect.Type, src ast.Node) (reflect.Value, error) {
	if v.Type().ConvertibleTo(typ) {
		return v.Convert(typ), nil
	}
	if IsFloatKind(typ.Kind()) && v.Kind() == reflect.String {
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return reflect.ValueOf(f).Convert(typ), nil
		}
	}

	return reflect.Zero(typ), yamlerrors.NewTypeMismatch(typ, v.Type(), src.GetToken())
}

// CastToAssignableValue returns value in a form a target of type target can hold:
// value itself where it is assignable, or, for a pointer target, its address,
// taken up to five times. It reports a type mismatch at src otherwise. This is
// how an alias reuses its anchor's value under ShareAliases.
func CastToAssignableValue(value reflect.Value, target reflect.Type, src ast.Node) (reflect.Value, error) {
	if target.Kind() != reflect.Pointer {
		if !value.Type().AssignableTo(target) {
			return reflect.Value{}, yamlerrors.NewTypeMismatch(target, value.Type(), src.GetToken())
		}
		return value, nil
	}

	const maxAddrCount = 5

	for range maxAddrCount {
		if value.Type().AssignableTo(target) {
			break
		}
		if !value.CanAddr() {
			break
		}
		value = value.Addr()
	}
	if !value.Type().AssignableTo(target) {
		return reflect.Value{}, yamlerrors.NewTypeMismatch(target, value.Type(), src.GetToken())
	}
	return value, nil
}

// SetDecoded writes v, what createDecodedNewValue decoded, into dst. v is
// invalid only where nothing decoded at all, and dst then keeps what it held.
func SetDecoded(dst, v reflect.Value) {
	if v.IsValid() {
		dst.Set(v)
	}
}

// ReadAll returns everything r holds, without copying it where r already has
// it in one piece.
//
// Unmarshal wraps the caller's slice in a bytes.Buffer, so copying it into
// another buffer and then calling String on that made three copies of the
// document before the parse began. The parser keeps windows into what it is
// given, so handing it the caller's own bytes costs nothing and holds nothing
// twice.
//
// A reader that is not backed by a slice still has to be read into one. That
// buffer grows by doubling, so what it hands back may have spare capacity, and
// the tree pins the whole block; the alternative is a copy the size of the
// document, which is what this exists to avoid.
func ReadAll(r io.Reader) ([]byte, error) {
	switch b := r.(type) {
	case *bytes.Buffer:
		return b.Bytes(), nil
	case *bytes.Reader:
		src := make([]byte, b.Len())
		if _, err := io.ReadFull(b, src); err != nil {
			return nil, err
		}

		return src, nil
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DynamicTypeOf returns the type inside an interface value, or v's own type
// where v is not one. It names what a key actually holds in an error about a
// map[any]any.
func DynamicTypeOf(v reflect.Value) reflect.Type {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		return v.Elem().Type()
	}

	return v.Type()
}

// AppendPointerToken writes one segment of a JSON pointer, escaping the two
// characters RFC 6901 §3 gives a meaning to.
func AppendPointerToken(dst []byte, s string) []byte {
	if !strings.ContainsAny(s, "~/") {
		return append(dst, s...)
	}
	for i := range len(s) {
		switch s[i] {
		case '~':
			dst = append(dst, '~', '0')
		case '/':
			dst = append(dst, '~', '1')
		default:
			dst = append(dst, s[i])
		}
	}

	return dst
}

// NamesOrderedMap reports the "!!omap" tag, whatever handle the document spells
// it with.
func NamesOrderedMap(uri string) bool {
	keyword, ok := token.ReservedTagOf(uri)

	return ok && keyword == token.OrderedMapTag
}

// NamesScalarType reports whether the tag says what a scalar is worth, rather
// than naming a kind and leaving the node to say.
func NamesScalarType(uri string) bool {
	keyword, ok := token.ReservedTagOf(uri)
	if !ok {
		return false
	}

	switch keyword {
	case token.StringTag, token.IntegerTag, token.FloatTag, token.BooleanTag,
		token.NullTag, token.BinaryTag, token.TimestampTag:
		return true
	default:
		return false
	}
}

// CustomArg returns v as the T a custom (un)marshaler was registered for.
//
// The custom (un)marshaler maps are keyed by the reflect.Type of T and looked
// up by the type of the value at hand, so v is always a T.
func CustomArg[T any](v any) T {
	return v.(T) //nolint:forcetypeassert // the maps are keyed by T's type and looked up by v's
}

// UnusableKey reports a key that cannot address an entry.
func UnusableKey(key any) error {
	return fmt.Errorf("cannot use %T as a key: %w", key, yamlerrors.ErrUnhashableKey)
}

func SetZero(dst reflect.Value) {
	if dst.IsValid() && dst.CanSet() {
		dst.Set(reflect.Zero(dst.Type()))
	}
}

// HoldsAnything reports a destination that takes whatever the document writes.
func HoldsAnything(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.Type().NumMethod() == 0
}

// PointedKind returns the kind of t past its pointers.
func PointedKind(t reflect.Type) reflect.Kind {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Kind()
}

// FieldAt walks the index path an embedded field stands at, making the pointers
// between as it goes. reflect.Value.FieldByIndex panics on a nil one and
// FieldByIndexErr refuses rather than filling it, and a document writing through
// an embedded pointer expects it filled.
//
// It returns false where a pointer on the way cannot be set, or does not point to a struct.
func FieldAt(v reflect.Value, at []int) (reflect.Value, bool) {
	for i, step := range at {
		if i > 0 {
			for v.Kind() == reflect.Pointer {
				if v.IsNil() {
					if !v.CanSet() {
						return reflect.Value{}, false
					}
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
			if v.Kind() != reflect.Struct {
				return reflect.Value{}, false
			}
		}
		v = v.Field(step)
	}

	return v, true
}

// IsMapValue reports whether v is a mapping the walk built.
func IsMapValue(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any:
		return true
	default:
		return false
	}
}
