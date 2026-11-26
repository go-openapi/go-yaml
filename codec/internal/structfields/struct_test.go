// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package structfields

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestStructFieldMapIsReadOncePerType(t *testing.T) {
	typ := reflect.TypeFor[cachedFields]()

	first, err := MapOf(typ, YAMLTags)
	require.NoError(t, err)
	second, err := MapOf(typ, YAMLTags)
	require.NoError(t, err)

	require.Len(t, first, 3)
	// The same Info, not an equal one: a second read would allocate a
	// new one and the cache would be doing nothing.
	assert.Same(t, first["Name"], second["Name"], "the type was read twice")
	assert.Equal(t, "name", first["Name"].RenderName)
	assert.Equal(t, "tags", first["Tags"].RenderName)
}

func TestDuplicatedFieldNameIsRefusedEveryTime(t *testing.T) {
	typ := reflect.TypeFor[twoFieldsOneName]()
	const repeat = 3

	for i := range repeat {
		fields, err := MapOf(typ, YAMLTags)
		require.Errorf(t, err, "read %d: the duplicated name was accepted", i)
		assert.Nilf(t, fields, "read %d: fields came back with the error", i)
		assert.ErrorContainsf(t, err, "same", "read %d: the error does not name the field", i)
	}
}

func TestNamesUnderEachMode(t *testing.T) {
	t.Run("yaml tags", func(t *testing.T) {
		assert.Equal(t, "goname", renderName[namedThreeWays](t, YAMLTags, "GoName"))
		assert.Equal(t, "t", renderName[namedThreeWays](t, YAMLTags, "Tagged"))
		assert.Equal(t, "jsononly", renderName[namedThreeWays](t, YAMLTags, "JSONOnly"), "the json tag is not read")
		assert.Equal(t, "yb", renderName[namedThreeWays](t, YAMLTags, "Both"))
	})

	t.Run("json tags", func(t *testing.T) {
		assert.Equal(t, "j", renderName[namedThreeWays](t, JSONTags, "JSONOnly"))
		assert.Equal(t, "yb", renderName[namedThreeWays](t, JSONTags, "Both"), "the yaml tag wins")
		assert.Equal(t, "goname", renderName[namedThreeWays](t, JSONTags, "GoName"))
	})

	t.Run("inferred names", func(t *testing.T) {
		assert.Equal(t, "GoName", renderName[namedThreeWays](t, InferredNames, "GoName"))
		assert.Equal(t, "t", renderName[namedThreeWays](t, InferredNames, "Tagged"))
	})

	t.Run("skipped fields", func(t *testing.T) {
		fields := fieldsOf[namedThreeWays](t, YAMLTags)
		assert.NotContains(t, fields.ByGoName, "Skipped")
		assert.NotContains(t, fields.ByGoName, "private")
	})
}

func TestIgnored(t *testing.T) {
	typ := reflect.TypeFor[namedThreeWays]()
	field := func(name string) reflect.StructField {
		f, ok := typ.FieldByName(name)
		require.True(t, ok)

		return f
	}

	assert.True(t, Ignored(field("Skipped"), YAMLTags))
	assert.True(t, Ignored(field("private"), YAMLTags))
	assert.False(t, Ignored(field("Tagged"), YAMLTags))
}

func TestYAMLFlags(t *testing.T) {
	fields := fieldsOf[flagged](t, YAMLTags).ByGoName

	all := fields["All"]
	assert.True(t, all.IsOmitEmpty)
	assert.True(t, all.IsOmitZero)
	assert.True(t, all.IsFlow)
	assert.False(t, all.IsInline)

	assert.Equal(t, "a1", fields["Anchor"].AnchorName)
	assert.Equal(t, "a1", fields["Alias"].AliasName)
	assert.True(t, fields["Auto"].IsAutoAnchor)
	assert.True(t, fields["Auto"].IsAutoAlias)
}

func TestYAMLFlagsRejectAnUnknownFlag(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			A string `yaml:"a,bogus"`
		}](),
		reflect.TypeFor[struct {
			A string `yaml:"a,"`
		}](),
		reflect.TypeFor[struct {
			A string `yaml:"a,anchor="`
		}](),
	} {
		_, err := Of(typ, YAMLTags)
		assert.Errorf(t, err, "%s", typ.Field(0).Tag)
	}
}

func TestJSONFlagsIgnoreAnUnknownFlag(t *testing.T) {
	type jsonFlagged struct {
		A string `json:"a,omitempty,string,bogus"` //nolint:staticcheck // the unknown option is the input under test
	}
	sf := fieldsOf[jsonFlagged](t, JSONTags).ByGoName["A"]

	assert.Equal(t, "a", sf.RenderName)
	assert.True(t, sf.IsOmitEmpty)
}

func TestLookupThroughAnInlinedStruct(t *testing.T) {
	fields := fieldsOf[withInline](t, YAMLTags)

	x, ok := fields.Lookup("x")
	require.True(t, ok)
	assert.Equal(t, []int{0, 0}, x.At)
	assert.Equal(t, "X", x.SF.FieldName)

	own, ok := fields.Lookup("own")
	require.True(t, ok)
	assert.Equal(t, []int{1}, own.At)

	_, ok = fields.Lookup("missing")
	assert.False(t, ok)
	assert.True(t, fields.Claims("x"))
	assert.False(t, fields.Claims("missing"))

	name, ok := fields.RenderNameOf("X")
	require.True(t, ok, "a promoted field is found by its Go name")
	assert.Equal(t, "x", name)
}

func TestLookupMatchesExactlyInEveryMode(t *testing.T) {
	type cased struct {
		Name string `json:"Name" yaml:"Name"`
	}

	for _, mode := range []Mode{YAMLTags, JSONTags} {
		_, ok := fieldsOf[cased](t, mode).Lookup("NAME")
		assert.False(t, ok, "mode %d matches a key exactly", mode)

		ff, ok := fieldsOf[cased](t, mode).Lookup("Name")
		require.True(t, ok)
		assert.Equal(t, "Name", ff.SF.FieldName)
	}
}

func TestConflictRuleUnderJSONTags(t *testing.T) {
	t.Run("two fields at one depth reach no field", func(t *testing.T) {
		fields := fieldsOf[twoBranches](t, JSONTags)

		assert.False(t, fields.Claims("x"))
		assert.True(t, fields.Claims("l"))
		assert.True(t, fields.Claims("r"))
		assert.False(t, fields.WritesPromoted("x", 0), "a dropped name is not written")
		assert.True(t, fields.WritesPromoted("l", 0))
		assert.True(t, fields.WritesPromoted("extra", 0), "a name no field declares is written")
	})

	t.Run("the shallower field wins", func(t *testing.T) {
		ff, ok := fieldsOf[shallowerWins](t, JSONTags).Lookup("x")
		require.True(t, ok)
		assert.Equal(t, []int{1}, ff.At)
		assert.False(t, fieldsOf[shallowerWins](t, JSONTags).WritesPromoted("x", 0),
			"the embedded struct's x is hidden by the outer one")
	})

	t.Run("the tagged field wins at one depth", func(t *testing.T) {
		ff, ok := fieldsOf[taggedWins](t, JSONTags).Lookup("x")
		require.True(t, ok)
		assert.Equal(t, []int{0, 0}, ff.At)
	})
}

func TestConflictOverAYAMLNameIsAnError(t *testing.T) {
	for _, mode := range []Mode{YAMLTags, JSONTags} {
		_, err := Of(reflect.TypeFor[yamlConflict](), mode)
		assert.Errorf(t, err, "mode %d", mode)
	}
}

func TestHasMergeProperty(t *testing.T) {
	type merging struct {
		Base map[string]any `yaml:",omitempty,inline,alias"`
	}
	type plain struct {
		Base map[string]any `yaml:",inline"`
	}

	assert.True(t, fieldsOf[merging](t, YAMLTags).ByGoName.HasMergeProperty())
	assert.False(t, fieldsOf[plain](t, YAMLTags).ByGoName.HasMergeProperty())
}

func TestInlineTargets(t *testing.T) {
	assert.True(t, InlineTakesUnclaimedEntries(reflect.TypeFor[map[string]any]()))
	assert.False(t, InlineTakesUnclaimedEntries(reflect.TypeFor[inlinedLeaf]()))

	_, err := Of(reflect.TypeFor[struct {
		A int `yaml:",inline"`
	}](), YAMLTags)
	assert.Error(t, err, "an int cannot be inlined")

	_, err = Of(reflect.TypeFor[struct {
		A map[int]any `yaml:",inline"`
	}](), YAMLTags)
	assert.Error(t, err, "an inlined map needs string keys")

	_, err = Of(reflect.TypeFor[struct {
		A map[string]any `yaml:",inline"`
		B map[string]any `yaml:",inline"`
	}](), YAMLTags)
	assert.Error(t, err, "a second inlined map has no entries to take")
}

// selfDecoder stands for one of the codec's unmarshaler interfaces.
type selfDecoder interface{ DecodeItself() }

type decodesItself struct{}

func (*decodesItself) DecodeItself() {}

func TestInlineReadsItsOwnNode(t *testing.T) {
	previous := selfDecoding
	t.Cleanup(func() { selfDecoding = previous })
	SelfDecoding(reflect.TypeFor[selfDecoder]())

	assert.True(t, InlineReadsItsOwnNode(reflect.TypeFor[decodesItself]()), "through its pointer")
	assert.False(t, InlineReadsItsOwnNode(reflect.TypeFor[inlinedLeaf]()))
}

type cachedFields struct {
	Name  string   `yaml:"name"`
	Count int      `yaml:"count"`
	Tags  []string `yaml:"tags"`
}

// twoFieldsOneName renders both fields as "same", which readStructFields
// refuses.
type twoFieldsOneName struct {
	First  string `yaml:"same"`
	Second string `yaml:"same"`
}

// fieldsOf returns the fields of T under mode, and fails the test on an error.
func fieldsOf[T any](t *testing.T, mode Mode) *Fields {
	t.Helper()
	fields, err := Of(reflect.TypeFor[T](), mode)
	require.NoError(t, err)

	return fields
}

// renderName returns the name a field of T is written under, by Go field name.
func renderName[T any](t *testing.T, mode Mode, goName string) string {
	t.Helper()
	sf, ok := fieldsOf[T](t, mode).ByGoName[goName]
	require.Truef(t, ok, "no field %s", goName)

	return sf.RenderName
}

type namedThreeWays struct {
	GoName   string
	Tagged   string `yaml:"t"`
	JSONOnly string `json:"j"`
	Both     string `json:"jb" yaml:"yb"`
	Skipped  string `yaml:"-"`
	private  string //nolint:unused // checks that an unexported field is skipped
}

type flagged struct {
	All    string `yaml:"all,omitempty,omitzero,flow"`
	Anchor string `yaml:"anchor,anchor=a1"`
	Alias  string `yaml:"alias,alias=a1"`
	Auto   string `yaml:"auto,anchor,alias"`
}

type inlinedLeaf struct {
	X int
}

type withInline struct {
	inlinedLeaf `yaml:",inline"`

	Own int
}

type leftBranch struct{ X, L int }

type rightBranch struct{ X, R int }

type twoBranches struct {
	leftBranch
	rightBranch
}

type shallowerWins struct {
	leftBranch

	X int
}

type taggedLeft struct {
	X int `json:"x"`
}

type taggedWins struct {
	taggedLeft
	rightBranch
}

type yamlLeft struct {
	X int `yaml:"x"`
}

type yamlRight struct {
	X int `yaml:"x"`
}

type yamlConflict struct {
	yamlLeft  `yaml:",inline"`
	yamlRight `yaml:",inline"`
}
