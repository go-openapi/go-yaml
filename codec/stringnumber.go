// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/structfields"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// This file reads and writes a `json:",string"` field: a number held in a string, as encoding/json/v2 does.

var stringType = reflect.TypeFor[string]()

// stringOptionError returns the error for `json:",string"` on a field whose type holds no number.
func stringOptionError(sf *structfields.Info, fieldType reflect.Type) error {
	return fmt.Errorf("option ,string may only be used on a number field: %s is %s", sf.FieldName, fieldType)
}

// decodeStringNumber decodes node into fieldValue, a number or a pointer to one, from a string that holds a JSON
// number.
//
// A plain 12 resolves to an integer and is rejected, as encoding/json/v2 rejects the JSON number 12. So is any text
// outside the JSON number grammar, such as "0x1", "+1" or " 12", and a fraction or an exponent for an integer
// field. The caller decodes a null as usual.
func (d *Decoder) decodeStringNumber(fieldValue reflect.Value, node ast.Node) error {
	base := fieldValue.Type()
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}

	v, err := d.nodeToValue(node)
	if err != nil {
		return err
	}
	s, ok := v.(string)
	if !ok {
		return yamlerrors.NewTypeMismatch(base, reflect.TypeOf(v), node.GetToken())
	}

	number := reflect.New(base).Elem()
	if err := setStringNumber(number, s); err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return yamlerrors.NewOverflow(base, s, node.GetToken())
		}

		return yamlerrors.NewTypeMismatch(base, stringType, node.GetToken())
	}

	dst := fieldValue
	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}
	dst.Set(number)

	return nil
}

// setStringNumber parses s into v, a number, and returns a [strconv.NumError] when s is no JSON number or
// does not fit.
func setStringNumber(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !isJSONInteger(s) {
			return &strconv.NumError{Func: "ParseInt", Num: s, Err: strconv.ErrSyntax}
		}
		n, err := strconv.ParseInt(s, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if !isJSONInteger(s) || s[0] == '-' {
			return &strconv.NumError{Func: "ParseUint", Num: s, Err: strconv.ErrSyntax}
		}
		n, err := strconv.ParseUint(s, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetUint(n)
	default: // a float, since StringTakesNumber holds
		if !isJSONNumber(s) {
			return &strconv.NumError{Func: "ParseFloat", Num: s, Err: strconv.ErrSyntax}
		}
		f, err := strconv.ParseFloat(s, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetFloat(f)
	}

	return nil
}

// isJSONInteger reports whether s is a JSON number with no fraction and no exponent.
func isJSONInteger(s string) bool {
	return isJSONNumber(s) && !strings.ContainsAny(s, ".eE")
}

// encodeStringNumber encodes v, a number or a pointer to one, as a string holding the number. The number is spelled
// as encoding/json spells it, so encoding/json/v2 reads it back.
//
// A nil pointer encodes as null whatever it points to, as in encoding/json/v2. Any other value of a type that holds
// no number returns the error of the option.
func (e *Encoder) encodeStringNumber(sf *structfields.Info, v reflect.Value, column int) (ast.Node, error) {
	number := v
	for number.Kind() == reflect.Pointer {
		if number.IsNil() {
			return e.encodeValue(v, column)
		}
		number = number.Elem()
	}
	if !structfields.StringTakesNumber(number.Type()) {
		return nil, stringOptionError(sf, v.Type())
	}

	var text string
	switch number.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		text = strconv.FormatInt(number.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		text = strconv.FormatUint(number.Uint(), 10)
	default: // a float, since StringTakesNumber holds
		f := number.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("option ,string cannot write %v: JSON has no number for it", f)
		}
		text = string(appendJSONFloat(nil, f, number.Type().Bits()))
	}

	return e.encodeString(text, column), nil
}

// appendJSONFloat appends f as encoding/json writes a float of the given bit size: the shortest decimal that reads
// back as f, with an exponent only below 1e-6 or from 1e21 on, and "e-7" for "e-07".
func appendJSONFloat(b []byte, f float64, bits int) []byte {
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 && (bits == 64 && (abs < 1e-6 || abs >= 1e21) ||
		bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21)) {
		format = 'e'
	}
	b = strconv.AppendFloat(b, f, format, -1, bits)
	if format == 'e' {
		// Clean up e-09 to e-9.
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}

	return b
}

// decodesStringField decodes the node of a `json:",string"` field, and reports whether it did. It records an error
// in foundErr, as for any field error. It returns false for a null the option leaves to the usual decode.
func (d *Decoder) decodesStringField(
	sf *structfields.Info, fieldValue reflect.Value, node ast.Node, foundErr *error,
	fieldName func(*yamlerrors.Error) string,
) bool {
	err := d.decodeStringField(sf, fieldValue, node)
	if err != nil {
		recordFieldError(foundErr, err, fieldName)

		return true
	}

	return !ast.IsNull(node)
}

// decodeStringField decodes the node of a `json:",string"` field. It leaves a null to the caller, which decodes it as
// for any field.
//
// It returns the error for a field whose type holds no number, except for a null into a pointer: encoding/json/v2
// sets the pointer to nil before it looks at the option.
func (d *Decoder) decodeStringField(sf *structfields.Info, fieldValue reflect.Value, node ast.Node) error {
	null := ast.IsNull(node)
	if null && fieldValue.Kind() == reflect.Pointer {
		return nil
	}
	if !structfields.StringTakesNumber(fieldValue.Type()) {
		return stringOptionError(sf, fieldValue.Type())
	}
	if null {
		return nil
	}

	return d.decodeStringNumber(fieldValue, node)
}
