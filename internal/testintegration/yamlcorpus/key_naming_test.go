// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// How a mapping key is named, and when two of them are one.
//
// The rule, implemented 2026-09-10: a key is named by the canonical spelling of
// its *type*, and two keys conflict when their type and their name agree.
//
//	an integer   expanded decimal      007 -> "7", 0x10 -> "16", +1 -> "1"
//	a float      shortest, with .0     1e3 -> "1000.0", 1.00 -> "1.0"
//	the specials YAML's own spelling   .Inf -> ".inf", .NaN -> ".nan"
//	null         "null"                ~ and an empty key alike
//	a boolean    "true" / "false"      True -> "true"
//
// Naming per type is what makes uniqueness per type expressible: a float always
// carries its ".0", so it never lands in the integers' namespace and 1 beside
// 1.0 is two keys rather than a collision.

func read(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v), "%q", src)

	return v
}

func refuses(t *testing.T, src string) string {
	t.Helper()

	var v any
	err := codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v)
	require.Error(t, err, "%q should be refused", src)

	return err.Error()
}

// TestFixedAKeyIsNamedByItsType is the naming half.
//
// Every one of these was named by the source text, or by Go's %v on the
// resolved value, before 2026-09-10. 1e3 keyed "1000" and 1.0 keyed "1", so a
// float lost the fact that it was one.
func TestFixedAKeyIsNamedByItsType(t *testing.T) {
	for _, tc := range []struct{ src, key string }{
		{src: "1.0: a\n", key: "1.0"},
		{src: "1e3: a\n", key: "1000.0"},
		{src: "1.5e3: a\n", key: "1500.0"},
		{src: "-0.0: a\n", key: "-0.0"},
		{src: "0.5: a\n", key: "0.5"},

		{src: "1: a\n", key: "1"},
		{src: "007: a\n", key: "7"},
		{src: "0x10: a\n", key: "16"},
		{src: "+1: a\n", key: "1"},

		{src: "~: a\n", key: "null"},
		{src: ": a\n", key: "null"},
		{src: "NULL: a\n", key: "null"},
		{src: "True: a\n", key: "true"},
		{src: "FALSE: a\n", key: "false"},

		{src: ".Inf: a\n", key: ".inf"},
		{src: ".INF: a\n", key: ".inf"},
		{src: "-.inf: a\n", key: "-.inf"},
		{src: ".NaN: a\n", key: ".nan"},
		{src: ".NAN: a\n", key: ".nan"},
	} {
		assert.Contains(t, read(t, tc.src), tc.key, "%q should be keyed %q", tc.src, tc.key)
	}
}

// TestFixedTwoKeysOfOneTypeAndName Conflict is the uniqueness half.
//
// Every one of these was read without complaint before 2026-09-10, silently
// keeping the last value: two spellings of one key, and a value gone.
func TestFixedTwoKeysOfOneTypeAndNameConflict(t *testing.T) {
	for _, src := range []string{
		"007: a\n7: b\n",
		"0x10: a\n16: b\n",
		"+1: a\n1: b\n",
		"1.0: a\n1.00: b\n",
		"~: a\nnull: b\n",
		"true: a\nTrue: b\n",
		".inf: a\n.Inf: b\n",
		".nan: a\n.NaN: b\n",
	} {
		assert.Contains(t, refuses(t, src), "already defined", "%q", src)
	}
}

// TestFixedTwoTypesAreTwoKeys is what the naming rule buys.
//
// A float and an integer of the same value are two nodes and stay two keys,
// which is what go.yaml.in/yaml/v3 does and what 3.2.1.1 requires. libfyaml
// merges them, and is lax here.
func TestFixedTwoTypesAreTwoKeys(t *testing.T) {
	got := read(t, "1.0: a\n1: b\n")

	assert.Equal(t, map[string]any{"1.0": "a", "1": "b"}, got,
		"a float and an integer of equal value are two keys")

	t.Run("and the infinities keep their sign", func(t *testing.T) {
		assert.Len(t, read(t, ".inf: a\n-.inf: b\n"), 2)
	})
}

// TestFixedAnExplicitFloatTagOnAKeyKeepsItsFloatness: writing the tag changes
// nothing about the name, which is what writing it should do.
//
// Found on 2026-09-10 by the two converters in this library disagreeing with
// each other: ToJSON wrote "226.0" for both spellings and the decoder "226" for
// the tagged one. They read one node→name walk now, [ast.KeyName], which
// resolves a tag rather than stepping over it -- so "!!float 226" is the float
// 226.0 and is named "226.0", where stripping the tag would have read the
// token "226" and named it after an integer.
func TestFixedAnExplicitFloatTagOnAKeyKeepsItsFloatness(t *testing.T) {
	assert.Contains(t, read(t, "226.0: x\n"), "226.0", "untagged, the float keeps its name")
	assert.Contains(t, read(t, "!!float 226.0: x\n"), "226.0",
		"the tag costs the key nothing")
	assert.Contains(t, read(t, "!!float 226: x\n"), "226.0",
		"and a whole number under !!float is named as the float it is")

	t.Run("the other tags name a key correctly", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{src: "!!int 226: x\n", key: "226"},
			{src: "!!str 226.0: x\n", key: "226.0"},
			{src: "!!bool True: x\n", key: "true"},
			{src: "!!null ~: x\n", key: "null"},
		} {
			assert.Contains(t, read(t, tc.src), tc.key, "%q", tc.src)
		}
	})
}

// TestFixedAnExplicitIntTagOnAKeyKeepsItsBase: a key written in hex or octal is
// named by the decimal it denotes, and writing "!!int" in front of it changes
// nothing.
//
// Found on 2026-09-09. Three walks reached a tagged key -- [ast.KeyName],
// [ast.KeyIdentity] and the parser's duplicate check -- and each carried its own
// copy of the tag switch, handing token.IntegerType over whatever base the
// scalar was written in. So "!!int 0x10: v" named the key "0x10" where
// "0x10: v" named it "16", and the two spellings passed the duplicate check as
// different keys and then collided in the decoder, which kept the second value
// and dropped the first with no error. They read [ast.TaggedKeyName] now.
func TestFixedAnExplicitIntTagOnAKeyKeepsItsBase(t *testing.T) {
	for _, tc := range []struct{ src, key string }{
		{src: "0x10: v\n", key: "16"},
		{src: "!!int 0x10: v\n", key: "16"},
		{src: "0o17: v\n", key: "15"},
		{src: "!!int 0o17: v\n", key: "15"},

		// The tag names the type and the quotes hide the base, so the text is
		// read again under the 1.2 core schema.
		{src: "!!int \"0x10\": v\n", key: "16"},
		{src: "!!int '0o17': v\n", key: "15"},

		// The base follows the schema that typed the scalar, not the tag: a
		// leading zero is octal in 1.1 and decimal in 1.2.
		{src: "!!int 017: v\n", key: "17"},
		{src: "%YAML 1.1\n---\n!!int 017: v\n", key: "15"},
		{src: "%YAML 1.1\n---\n!!int 0b101: v\n", key: "5"},

		// An anchor names the node and says nothing about its type.
		{src: "!!int &a 0x10: v\n", key: "16"},
		{src: "&a !!int 0x10: v\n", key: "16"},
	} {
		assert.Contains(t, read(t, tc.src), tc.key, "%q should be keyed %q", tc.src, tc.key)
	}

	t.Run("and the tagged spelling collides with the untagged one", func(t *testing.T) {
		for _, src := range []string{
			"0x10: a\n!!int 0x10: b\n",
			"!!int 0x10: a\n16: b\n",
			"0o17: a\n!!int 0o17: b\n",
			"{0x10: a, !!int 0x10: b}\n",
		} {
			assert.Contains(t, refuses(t, src), "already defined", "%q", src)
		}
	})
}

// TestAnIntegerTagReadsTheBaseTheSchemaTyped: "!!int" names the type and the
// document's schema says what base the digits are written in, so the same
// characters are one integer under 1.1 and no integer at all under 1.2.
//
// ast.readsAsInteger sniffed the base with strconv.ParseInt(text, 0, 64) --
// Go's rule, not YAML's. It takes a sign on a hex number, a capital "X", a "0b"
// prefix and "_" separators, so every one of these resolved whatever the
// document declared, and then decoded to 0 while the key walk named it after
// the text. The base comes from the type the scanner gave the scalar now, and
// the scanner typed it under the schema the document declared.
//
// The two paths are checked together on purpose: the value a document decodes
// to and the name its key is addressed by come from one reading, and 81 is what
// happens when they part.
func TestAnIntegerTagReadsTheBaseTheSchemaTyped(t *testing.T) {
	const under11 = "%YAML 1.1\n---\n"

	for _, tc := range []struct{ body, key11 string }{
		// 1.1 puts a sign on a hex integer, 1.2 does not.
		{body: "!!int -0x10: v", key11: "-16"},
		{body: "!!int +0x10: v", key11: "16"},
		// 1.1 has binary and "_" separators, 1.2 has neither.
		{body: "!!int 0b101: v", key11: "5"},
		{body: "!!int 1_000: v", key11: "1000"},
		// 1.1 has base 60, written between colons.
		{body: "!!int 190:20:30: v", key11: "685230"},
	} {
		assert.Containsf(t, refuses(t, tc.body+"\n"), "as !!int",
			"%q is no integer under the 1.2 core schema", tc.body)
		assert.Containsf(t, read(t, under11+tc.body+"\n"), tc.key11,
			"%q should be keyed %q under 1.1", tc.body, tc.key11)
	}

	t.Run("and a spelling neither schema writes is refused under both", func(t *testing.T) {
		// YAML writes the hexadecimal prefix in lower case at every version.
		for _, src := range []string{"!!int 0X10: v\n", under11 + "!!int 0X10: v\n"} {
			assert.Containsf(t, refuses(t, src), "as !!int", "%q", src)
		}
	})

	t.Run("and a leading zero follows the schema rather than the tag", func(t *testing.T) {
		assert.Contains(t, read(t, "!!int 017: v\n"), "17", "1.2 reads a leading zero as decimal")
		assert.Contains(t, read(t, under11+"!!int 017: v\n"), "15", "1.1 reads it as octal")
	})
}

// TestATagIsNoLaxerThanTheBareSpelling: where the schema reads a plain scalar
// as a number, writing the tag gives the same number; where it does not, the
// tag is refused.
//
// The tag names the type, not the spelling. ast.readsAsInteger and
// ast.readsAsFloat sniffed the text with Go's rules -- strconv.ParseInt at base
// 0 and strconv.ParseFloat -- so a tag accepted spellings the document's own
// schema had already read as strings, and the value that came back was 0 or
// Go's own reading of the characters.
func TestATagIsNoLaxerThanTheBareSpelling(t *testing.T) {
	const under11 = "%YAML 1.1\n---\n"

	t.Run("a spelling 1.1 does not have", func(t *testing.T) {
		// 1.1's octal is a leading zero and its decimal may not open with one,
		// so "09" is neither; "0o17" is 1.2's octal, which 1.1 has no form for.
		// Both fell back to a decimal parse: "!!int 09" was 9 and
		// "!!int 0o17" was 15, where the bare scalars are the strings.
		for _, text := range []string{"09", "0o17", "08", "0o0", "0O17", "0X10", "+0o17", "0_9"} {
			assert.Equalf(t, text, valueOf(t, under11+"k: "+text+"\n"),
				"%q is a string under 1.1", text)
			assert.Containsf(t, refuses(t, under11+"k: !!int "+text+"\n"), "as !!int",
				"so !!int over %q is a tag naming a type its scalar is not", text)
		}
	})

	t.Run("a float spelling only Go has", func(t *testing.T) {
		// A hexadecimal float is Go's, and no YAML schema writes one.
		for _, src := range []string{"k: !!float 0x1p-2\n", under11 + "k: !!float 0x1p-2\n"} {
			assert.Containsf(t, refuses(t, src), "as !!float", "%q", src)
		}
		// 1.1 makes the decimal point mandatory in a float, so an exponent with
		// no point is a string there and a float under the core schema.
		assert.Equal(t, float64(1000), valueOf(t, "k: !!float 1e3\n"), "1.2 reads an exponent with no point")
		assert.Contains(t, refuses(t, under11+"k: !!float 1e3\n"), "as !!float", "1.1 wants the point")
	})

	t.Run("and 1.1's sexagesimal reaches both tags", func(t *testing.T) {
		assert.Equal(t, 685230, valueOf(t, under11+"k: !!int 190:20:30\n"))
		assert.Equal(t, 685230.5, valueOf(t, under11+"k: !!float 190:20:30.5\n"))
		// Neither is a number under the core schema.
		assert.Contains(t, refuses(t, "k: !!int 190:20:30\n"), "as !!int")
	})
}

// valueOf reads src and returns what it holds under the key "k".
func valueOf(t *testing.T, src string) any {
	t.Helper()

	m, ok := read(t, src).(map[string]any)
	require.Truef(t, ok, "%q should read as a mapping", src)

	return m["k"]
}
