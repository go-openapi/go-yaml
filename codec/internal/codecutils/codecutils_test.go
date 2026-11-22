// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"bytes"
	"iter"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestReadAllDoesNotCopyWhatItIsGiven checks the decoder reads the caller's
// bytes rather than a copy of them.
//
// Unmarshal wraps the caller's slice in a bytes.Buffer, and the parser keeps
// windows into what it parses, so a copy here would be one the whole tree then
// holds. This fails the moment ReadAll starts allocating again.
func TestReadAllDoesNotCopyWhatItIsGiven(t *testing.T) {
	data := []byte("a: 1\nb: two\n")

	got, err := ReadAll(bytes.NewBuffer(data))
	require.NoError(t, err)
	require.Equal(t, string(data), string(got), "ReadAll changed the document")
	assert.Same(t, &data[0], &got[0], "ReadAll copied a bytes.Buffer's contents instead of reading them where they were")
}

// TestReadAllReadsWhatIsNotASlice checks the fallback still reads a plain
// reader through, since only a byte-backed one can be read in place.
func TestReadAllReadsWhatIsNotASlice(t *testing.T) {
	const doc = "a: 1\nb: two\n"

	got, err := ReadAll(strings.NewReader(doc))
	require.NoError(t, err)
	assert.Equal(t, doc, string(got), "ReadAll changed the document")
}

// TestReadAllReadsAByteReader covers *bytes.Reader, which holds the bytes but
// will not hand them over.
func TestReadAllReadsAByteReader(t *testing.T) {
	data := []byte("a: 1\n")

	got, err := ReadAll(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, string(data), string(got), "ReadAll changed the document")
	assert.NotSame(t, &data[0], &got[0], "bytes.Reader does not expose its slice; ReadAll now aliases it, which is worth knowing")
}

func TestCastToFloatValue(t *testing.T) {
	for tc := range castToFloatCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, CastToFloatValue(tc.in))
		})
	}

	for _, text := range []string{"1e400", "1e-400"} {
		t.Run("string past float64: "+text, func(t *testing.T) {
			got, ok := CastToFloatValue(text).(*big.Float)
			require.Truef(t, ok, "%q narrowed to a float64", text)

			assert.Zero(t, got.Cmp(bigFloat(text)))
		})
	}
}

func TestNumberKey(t *testing.T) {
	name := func(v any) string {
		_, n, ok := NumberKey(v)
		require.Truef(t, ok, "%T is not a number", v)

		return n
	}

	assert.Equal(t, name(int8(1)), name(uint64(1)), "one integer in two Go types")
	assert.Equal(t, name(float32(0.5)), name(0.5), "one float in two Go types")
	assert.Equal(t, "0.0", name(0.0))
	assert.Equal(t, "123456789012345678901234567890", name(func() *big.Int {
		n, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
		return n
	}()))

	kind, _, _ := NumberKey(1)
	assert.Equal(t, token.KeyInt, kind)
	kind, _, _ = NumberKey(1.0)
	assert.Equal(t, token.KeyFloat, kind)

	for _, v := range []any{"1", (*big.Int)(nil), (*big.Float)(nil), true} {
		_, _, ok := NumberKey(v)
		assert.Falsef(t, ok, "%T reads as a number", v)
	}
}

func TestHashableKey(t *testing.T) {
	type holdsASlice struct{ S []int }

	for _, key := range []any{nil, 1, "a", [2]int{}, struct{ n int }{}} {
		assert.Truef(t, HashableKey(key), "%T", key)
	}
	for _, key := range []any{[]int{}, map[string]int{}, holdsASlice{}, [1][]int{}} {
		assert.Falsef(t, HashableKey(key), "%T", key)
	}
}

func TestByteDestinations(t *testing.T) {
	type raw []byte

	assert.True(t, ByteArray(reflect.TypeFor[[4]byte]()))
	assert.False(t, ByteArray(reflect.TypeFor[[4]int]()))
	assert.False(t, ByteArray(reflect.TypeFor[[]byte]()))

	assert.True(t, ByteSlice(reflect.TypeFor[[]byte]()))
	assert.True(t, ByteSlice(reflect.TypeFor[raw]()))
	assert.False(t, ByteSlice(reflect.TypeFor[[]int]()))
	assert.False(t, ByteSlice(reflect.TypeFor[[4]byte]()))
}

func TestCopyIntoByteArray(t *testing.T) {
	src := scalarAt(t, "!!binary AQID")

	t.Run("fills an array of the right length", func(t *testing.T) {
		var dst [3]byte
		require.NoError(t, CopyIntoByteArray(reflect.ValueOf(&dst).Elem(), []byte{1, 2, 3}, src))
		assert.Equal(t, [3]byte{1, 2, 3}, dst)
	})

	t.Run("rejects a length the array does not have", func(t *testing.T) {
		var dst [4]byte
		err := CopyIntoByteArray(reflect.ValueOf(&dst).Elem(), []byte{1, 2, 3}, src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a 4 byte array cannot hold the 3 bytes")
		assert.Equal(t, [4]byte{}, dst, "the array is left as it was")
	})
}

func TestAppendPointerToken(t *testing.T) {
	assert.Equal(t, "/plain", string(AppendPointerToken([]byte("/"), "plain")))
	assert.Equal(t, "/a~1b~0c", string(AppendPointerToken([]byte("/"), "a/b~c")))
	assert.Equal(t, "~01", string(AppendPointerToken(nil, "~1")), "an escape in the input is escaped again")
}

func TestFieldAt(t *testing.T) {
	// The embedded types are exported: reflect cannot set an unexported embedded field.
	type Inner struct{ X int }
	type Mid struct{ Inner }
	type outer struct{ *Mid }

	t.Run("allocates the embedded pointer on the way", func(t *testing.T) {
		var o outer
		field, ok := FieldAt(reflect.ValueOf(&o).Elem(), []int{0, 0, 0})
		require.True(t, ok)

		field.SetInt(42)
		require.NotNil(t, o.Mid)
		assert.Equal(t, 42, o.X)
	})

	t.Run("stops at a nil pointer it cannot set", func(t *testing.T) {
		_, ok := FieldAt(reflect.ValueOf(outer{}), []int{0, 0, 0})
		assert.False(t, ok)
	})
}

func TestConvertBigNumber(t *testing.T) {
	big123 := reflect.ValueOf(big.NewInt(123))

	got, ok := ConvertBigNumber(big123, reflect.TypeFor[float64]())
	require.True(t, ok)
	assert.Equal(t, 123.0, got.Interface())

	got, ok = ConvertBigNumber(big123, reflect.TypeFor[string]())
	require.True(t, ok)
	assert.Equal(t, "123", got.Interface())

	_, ok = ConvertBigNumber(big123, reflect.TypeFor[int]())
	assert.False(t, ok, "an integer destination is not converted here")

	_, ok = ConvertBigNumber(reflect.ValueOf(bigFloat("1e400")), reflect.TypeFor[float64]())
	assert.False(t, ok, "a number past float64 is not narrowed to +Inf")

	_, ok = ConvertBigNumber(reflect.ValueOf(1), reflect.TypeFor[float64]())
	assert.False(t, ok, "a native number is not a big one")
}

func TestConvertToKind(t *testing.T) {
	src := scalarAt(t, "x")

	got, err := ConvertToKind(reflect.ValueOf(7), reflect.TypeFor[int64](), src)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got.Interface())

	got, err = ConvertToKind(reflect.ValueOf("1e3"), reflect.TypeFor[float32](), src)
	require.NoError(t, err)
	assert.Equal(t, float32(1000), got.Interface())

	got, err = ConvertToKind(reflect.ValueOf("x"), reflect.TypeFor[float64](), src)
	require.Error(t, err)
	assert.Equal(t, 0.0, got.Interface(), "the zero of the destination comes with the error")
}

func TestReservedTags(t *testing.T) {
	assert.True(t, NamesOrderedMap(token.YAMLTagPrefix+"omap"))
	assert.False(t, NamesOrderedMap(token.YAMLTagPrefix+"map"))
	assert.False(t, NamesOrderedMap("!omap"), "a local tag is not the reserved one")

	for _, name := range []string{"str", "int", "float", "bool", "null", "binary", "timestamp"} {
		assert.Truef(t, NamesScalarType(token.YAMLTagPrefix+name), "!!%s", name)
	}
	for _, name := range []string{"map", "seq", "omap", "merge"} {
		assert.Falsef(t, NamesScalarType(token.YAMLTagPrefix+name), "!!%s", name)
	}
}

func TestDynamicTypeOf(t *testing.T) {
	var held any = 3
	assert.Equal(t, reflect.TypeFor[int](), DynamicTypeOf(reflect.ValueOf(&held).Elem()))

	var empty any
	assert.Equal(t, reflect.TypeFor[any](), DynamicTypeOf(reflect.ValueOf(&empty).Elem()))
}

func TestSetZero(t *testing.T) {
	n := 5
	SetZero(reflect.ValueOf(&n).Elem())
	assert.Zero(t, n)

	assert.NotPanics(t, func() { SetZero(reflect.Value{}) })
	assert.NotPanics(t, func() { SetZero(reflect.ValueOf(5)) }, "a value it cannot set")
}

// ***************** cast to float test cases **************************

// castToFloatCase holds a decoded value and what CastToFloatValue returns for it.
type castToFloatCase struct {
	name string
	in   any
	want any
}

func castToFloatCases() iter.Seq[castToFloatCase] {
	huge := bigFloat("1e400")

	return slices.Values([]castToFloatCase{
		{"int8", int8(3), 3.0},
		{"uint64", uint64(7), 7.0},
		{"float32", float32(0.5), 0.5},
		{"big.Int", big.NewInt(10), 10.0},
		{"big.Float that a float64 holds", bigFloat("2.5"), 2.5},
		{"big.Float past float64, kept", huge, huge},
		{"string", "1e3", 1000.0},
		{"string spelling zero", "0.0", 0.0},
		{"unknown type", true, 0},
	})
}

// bigFloat returns text read as a *big.Float. It panics on text that is not a number: the cases spell fixed literals.
func bigFloat(text string) *big.Float {
	f, _, err := big.ParseFloat(text, 10, 0, big.ToNearestEven)
	if err != nil {
		panic(err)
	}

	return f
}

// scalarAt returns the node of a one-scalar document, for the helpers that place an error at a node.
func scalarAt(t *testing.T, src string) ast.Node {
	t.Helper()
	file, err := parser.ParseBytes([]byte(src))
	require.NoError(t, err)
	require.Len(t, file.Docs, 1)

	return file.Docs[0].Body
}
