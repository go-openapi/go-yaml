// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

// The generator crosses a written tag with a declared version and asserts what the
// scalar resolves to, which the presentation properties cannot reach.
//
// TestPresentationInvariance writes one value several ways and checks they read back the
// same. A tag there always names the type the value already has, so `!!int` sits on an
// Int and the document agrees with itself however it is written. What that never asks is
// whether the tag and the schema agree about the *base*: "!!int 0b101" is five under YAML
// 1.1 and not an integer at all under 1.2, and a property comparing a document only
// against itself reads both as the Int it started from.
//
// So these two hold a tagged scalar against the same scalar written bare, at the same
// version, which is a comparison the value model does not supply and cannot be satisfied
// by writing the document consistently.
//
// Both draw the scalar in a key position as well as a value one. The two walks resolved
// separately -- defect 81 was "!!int 0x10: v" naming its key "0x10" where "0x10: v" named
// it 16 -- so a shape drawn only as a value sees half of this, and a fix that corrected
// the naming alone would turn a two-key document into silent data loss with every
// property still green.

// intText draws the spelling of a scalar that some schema might read as an integer.
//
// The parts are drawn separately rather than sampled whole, so the cross reaches
// combinations nobody wrote down: a "+" on a hex body, an underscore against a base
// prefix, a sexagesimal run with a leading zero. Those are where the two schemas stop
// agreeing.
func intText() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		sign := rapid.SampledFrom([]string{"", "", "-", "+"}).Draw(t, "sign")
		prefix := rapid.SampledFrom([]string{"", "", "", "0x", "0X", "0o", "0b", "0"}).Draw(t, "prefix")

		var digits string
		switch prefix {
		case "0x", "0X":
			digits = rapid.StringOfN(rapid.RuneFrom([]rune("0123456789abcdefABCDEF")), 1, 4, -1).Draw(t, "hex")
		case "0b":
			digits = rapid.StringOfN(rapid.RuneFrom([]rune("01")), 1, 6, -1).Draw(t, "bin")
		case "0o", "0":
			digits = rapid.StringOfN(rapid.RuneFrom([]rune("01234567")), 1, 4, -1).Draw(t, "oct")
		default:
			digits = rapid.StringOfN(rapid.RuneFrom([]rune("0123456789")), 1, 5, -1).Draw(t, "dec")
		}

		body := prefix + digits
		if rapid.Bool().Draw(t, "sexagesimal") && prefix == "" {
			// 1.1 reads "190:20:30" as base 60 and core reads it as a string.
			body += ":" + rapid.StringOfN(rapid.RuneFrom([]rune("012345")), 1, 2, -1).Draw(t, "sexa")
		}
		if rapid.Bool().Draw(t, "underscore") {
			body = insertUnderscore(t, body)
		}

		return sign + body
	})
}

// insertUnderscore puts a "_" somewhere in text, including the placements 1.1 refuses.
//
// 1.1 wants a digit before the first "_", so "_1_0" is a string at both versions while
// "1_0_" is ten under 1.1. Drawing the position rather than a well-formed one is the
// point: the refusals are as much of the answer as the acceptances.
func insertUnderscore(t *rapid.T, text string) string {
	if text == "" {
		return text
	}
	at := rapid.IntRange(0, len(text)).Draw(t, "underscoreat")

	return text[:at] + "_" + text[at:]
}

// atVersion writes doc under an explicit %YAML directive.
func atVersion(version, doc string) string {
	return "%YAML " + version + "\n---\n" + doc + "\n"
}

// resolution is what one scalar came to, in one position, at one version.
type resolution struct {
	// Refused says the document did not read at all.
	Refused bool
	// Value is what it resolved to, rendered so two readings can be compared
	// without caring which integer width carried them.
	Value string
}

func (r resolution) String() string {
	if r.Refused {
		return "refused"
	}

	return r.Value
}

// asValue reads the scalar as a mapping's value.
func asValue(version, scalar string) resolution {
	var got map[string]any
	if err := yaml.Unmarshal([]byte(atVersion(version, "v: "+scalar)), &got); err != nil {
		return resolution{Refused: true}
	}

	return resolution{Value: render(got["v"])}
}

// asKey reads the scalar as a mapping's key and returns the name it is addressed by.
func asKey(version, scalar string) resolution {
	var got map[any]any
	if err := yaml.Unmarshal([]byte(atVersion(version, scalar+": v")), &got); err != nil {
		return resolution{Refused: true}
	}
	if len(got) != 1 {
		return resolution{Value: fmt.Sprintf("%d keys", len(got))}
	}
	for k := range got {
		return resolution{Value: render(k)}
	}

	return resolution{Value: "no key"}
}

// render writes a decoded scalar so two readings compare on the number rather than on the
// Go type that carried it.
func render(v any) string {
	switch n := v.(type) {
	case *big.Int:
		return "int:" + n.String()
	case big.Int:
		return "int:" + n.String()
	case int:
		return "int:" + strconv.Itoa(n)
	case int64:
		return "int:" + strconv.FormatInt(n, 10)
	case uint64:
		return "int:" + strconv.FormatUint(n, 10)
	default:
		return fmt.Sprintf("%T:%v", v, v)
	}
}

// isInt says the reading produced an integer.
func isInt(r resolution) bool {
	return !r.Refused && strings.HasPrefix(r.Value, "int:")
}

// isFloat says the reading produced a float.
func isFloat(r resolution) bool {
	return !r.Refused && strings.HasPrefix(r.Value, "float64:")
}

// isString says the schema read the characters and made a string of them.
func isString(r resolution) bool {
	return !r.Refused && strings.HasPrefix(r.Value, "string:")
}

// numberKind pairs a tag with the spellings to draw for it and the reading it names.
type numberKind struct {
	tag  string
	draw func(*rapid.T) string
	is   func(resolution) bool
}

// numberKinds are the two numeric tags whose base the schema decides.
//
//nolint:gochecknoglobals // ok as an immutable table
var numberKinds = []numberKind{
	{tag: "!!int", draw: func(t *rapid.T) string { return intText().Draw(t, "int") }, is: isInt},
	{tag: "!!float", draw: func(t *rapid.T) string { return floatText().Draw(t, "float") }, is: isFloat},
}

// floatText draws the spelling of a scalar that some schema might read as a float.
//
// YAML 1.1 makes the "." mandatory and the exponent's sign mandatory with it, so "1e3"
// and "1.0e3" are both strings there while "1.0e+3" is a number. YAML 1.2 core wants
// neither. Drawing the point and the sign separately crosses that line in both
// directions.
//
// "0x1p-2" is drawn because it is Go's hex float literal and no YAML schema's. It read
// 0.25 at both versions until 7e20582, through strconv.ParseFloat, which is the defect
// this axis was written to catch.
func floatText() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "special") {
			return rapid.SampledFrom([]string{
				".inf", "-.inf", ".nan", ".Inf", ".NaN", "0x1p-2", "0x1P+3",
			}).Draw(t, "specialtext")
		}

		sign := rapid.SampledFrom([]string{"", "", "-", "+"}).Draw(t, "fsign")
		whole := rapid.StringOfN(rapid.RuneFrom([]rune("0123456789")), 1, 3, -1).Draw(t, "whole")

		text := sign + whole
		if rapid.Bool().Draw(t, "point") {
			text += "." + rapid.StringOfN(rapid.RuneFrom([]rune("0123456789")), 0, 3, -1).Draw(t, "frac")
		}
		if rapid.Bool().Draw(t, "exponent") {
			text += rapid.SampledFrom([]string{"e", "E"}).Draw(t, "e") +
				rapid.SampledFrom([]string{"", "+", "-"}).Draw(t, "esign") +
				rapid.StringOfN(rapid.RuneFrom([]rune("0123456789")), 1, 2, -1).Draw(t, "exp")
		}

		return text
	})
}

// versions are the two schemas a document can declare.
//
// "1.2" is the control: it declares what the corpus already assumes, so a scalar under it
// must resolve exactly as the same scalar resolves with no directive at all.
var versions = []string{"1.1", "1.2"} //nolint:gochecknoglobals // ok as an immutable table

// TestATagIsNotLaxerThanTheBareSpelling holds "!!int x" against "x" at the same version.
//
// The tag names a type, and the schema decides what characters spell a value of that
// type. So the tag may not admit a spelling the schema refuses: where the bare scalar is
// an integer the tagged one has to be the same integer, and where the bare scalar is not
// an integer at all the tagged one has to be refused rather than guessed at.
//
// This is what caught the family. "!!float 0x1p-2" read 0.25 at both versions where plain
// "0x1p-2" is the string at both -- Go's hex float literal, which belongs to no YAML
// schema and reached the value through strconv. Every document was internally consistent,
// so nothing comparing a document against itself could see it.
//
// The relaxation is parser.WithLaxTags, and the default is strict, which is what this
// measures. Fred's ruling of 2026-09-09: tag:yaml.org,2002:int names the whole numbers,
// and an incomplete representation -- a truncation, or the zero a failed parse leaves --
// is not an acceptable answer for the decoder.
func TestATagIsNotLaxerThanTheBareSpelling(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		kind := rapid.SampledFrom(numberKinds).Draw(rt, "kind")
		text := kind.draw(rt)
		version := rapid.SampledFrom(versions).Draw(rt, "version")

		for _, position := range []struct {
			name string
			read func(string, string) resolution
		}{
			{"value", asValue},
			{"key", asKey},
		} {
			bare := position.read(version, text)
			tagged := position.read(version, kind.tag+" "+text)

			switch {
			case kind.is(bare):
				// The schema already reads these characters as the tag's own type,
				// so the tag has nothing left to decide.
				if tagged.Refused {
					rt.Fatalf("%%YAML %s, %s position: %q is %s and %q is refused;"+
						" a tag may not refuse a spelling its own schema reads",
						version, position.name, text, bare, kind.tag+" "+text)
				}
				if tagged.Value != bare.Value {
					rt.Fatalf("%%YAML %s, %s position: %q is %s and %q is %s;"+
						" the tag names the type, not a different reading of the characters",
						version, position.name, text, bare, kind.tag+" "+text, tagged)
				}
			case isString(bare):
				// The schema read the characters and made a string of them, which
				// settles that they do not spell a number at this version. A tag
				// that resolves one anyway is reading by some other rule --
				// strconv's, in every case found so far.
				if !tagged.Refused {
					rt.Fatalf("%%YAML %s, %s position: %q is %s and %q is %s;"+
						" the tag is laxer than the spelling, which is what WithLaxTags is for",
						version, position.name, text, bare, kind.tag+" "+text, tagged)
				}
			}
			// Anything else is deliberately unconstrained. An integer spelling is
			// also a float spelling under 1.2 core, where "[0-9]+" needs neither a
			// "." nor an exponent, so "!!float 017" resolving to 17 is correct and
			// refusing it would be correct under 1.1. That line is Fred's to draw,
			// and pinning either answer here would pin it by accident.
		}
	})
}

// TestAKeyAndAValueComeFromOneReading draws the same scalar in both positions.
//
// A document's value and the name its key is addressed by are one answer, so a scalar
// that reads as 16 in a value has to name its key 16 as well. The two walks resolved
// separately until 2026-09-09: "!!int 0x10: v" named its key "0x10" where "0x10: v" named
// it 16, so the tag changed the name and not the value (defect 81).
//
// The failure mode a naming-only fix leaves behind is why this draws both rather than
// trusting the value: correcting the name so that "0x10" and 16 agree would fold two keys
// of a two-key document into one, and a document losing an entry reads perfectly.
func TestAKeyAndAValueComeFromOneReading(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		text := intText().Draw(rt, "text")
		version := rapid.SampledFrom(versions).Draw(rt, "version")
		if rapid.Bool().Draw(rt, "tagged") {
			text = "!!int " + text
		}

		value, key := asValue(version, text), asKey(version, text)

		switch {
		case value.Refused != key.Refused:
			rt.Fatalf("%%YAML %s: %q is %s as a value and %s as a key;"+
				" one scalar, two answers about whether it is a document",
				version, text, value, key)
		case !value.Refused && value.Value != key.Value:
			rt.Fatalf("%%YAML %s: %q is %s as a value and names its key %s;"+
				" the value walk and the key walk read the same characters differently",
				version, text, value, key)
		}
	})
}

// TestADeclaredVersionOnlyChangesWhatItsSchemaChanges pins "1.2" as the control.
//
// A document declaring 1.2 must mean exactly what the same document means declaring
// nothing, since that is the schema everything else here assumes. If the two ever part,
// the directive is doing something beyond selecting a schema and every 1.1 measurement
// beside it is standing on sand.
func TestADeclaredVersionOnlyChangesWhatItsSchemaChanges(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		text := intText().Draw(rt, "text")
		if rapid.Bool().Draw(rt, "tagged") {
			text = "!!int " + text
		}

		var undeclared map[string]any
		bareErr := yaml.Unmarshal([]byte("v: "+text+"\n"), &undeclared)
		declared := asValue("1.2", text)

		if (bareErr != nil) != declared.Refused {
			rt.Fatalf("%q reads with no directive (err %v) and %s under %%YAML 1.2",
				text, bareErr, declared)
		}
		if bareErr == nil && render(undeclared["v"]) != declared.Value {
			rt.Fatalf("%q is %s with no directive and %s under %%YAML 1.2",
				text, render(undeclared["v"]), declared)
		}
	})
}

// TestTheNamedTagResolutionsHold pins the spellings Fred and go-yaml-perf named.
//
// The properties above explore; this states the answers, so a change to any of them shows
// up as this table moving rather than as a rapid seed nobody can reproduce. Every row was
// wrong before 420d69c, dbded8f and d906982 landed on 2026-09-09.
//
// The last two rows are the ruling: ".inf" and ".nan" under an "!!int" tag decoded to
// -9223372036854775808, which is Go's int(math.Inf(1)) and not a number the document
// wrote.
func TestTheNamedTagResolutionsHold(t *testing.T) {
	for _, tc := range []struct {
		scalar  string
		under12 string
		under11 string
	}{
		{"!!int -0x10", "refused", "int:-16"},
		{"!!int +0x10", "refused", "int:16"},
		{"!!int 0b101", "refused", "int:5"},
		{"!!int 1_000", "refused", "int:1000"},
		{"!!int 190:20:30", "refused", "int:685230"},
		{"!!int 0X10", "refused", "refused"},
		{"!!int 017", "int:17", "int:15"},
		{"!!int 1.9", "refused", "refused"},
		{"!!int .inf", "refused", "refused"},
		{"!!int -.inf", "refused", "refused"},
		{"!!int .nan", "refused", "refused"},
		// The float half, closed by 7e20582. "0x1p-2" is Go's hex float literal and
		// no YAML schema's; it read 0.25 at both versions through strconv.ParseFloat.
		// 1.1 makes the "." mandatory, so "1e3" is a string there and the tag agrees
		// with the bare spelling instead of being laxer than it.
		{"!!float 0x1p-2", "refused", "refused"},
		// Fred ruled on 2026-09-09 that a written !!int or !!float means its own
		// version's type, and that a refused value takes precedence over the tag
		// resolution, so this stays refused under 1.1. It is our stance and not
		// the specification's: 3.3.2 restricts application tag-resolution rules to
		// the "?" non-specific tag, so the schema regexes govern untagged nodes and
		// say nothing about what a written tag may spell. PyYAML 6.0.1 reads this
		// as 1000.0.
		{"!!float 1e3", "float64:1000", "refused"},
		{"!!float 1.5", "float64:1.5", "float64:1.5"},
	} {
		t.Run(tc.scalar, func(t *testing.T) {
			assert.Equal(t, tc.under12, asValue("1.2", tc.scalar).String(), "under %%YAML 1.2")
			assert.Equal(t, tc.under11, asValue("1.1", tc.scalar).String(), "under %%YAML 1.1")

			// The key walk answers the same or the row is only half stated.
			assert.Equal(t, tc.under12, asKey("1.2", tc.scalar).String(), "as a key under %%YAML 1.2")
			assert.Equal(t, tc.under11, asKey("1.1", tc.scalar).String(), "as a key under %%YAML 1.1")
		})
	}
}

// TestTheUnderscorePlacementsHold pins where "_" may stand in a number.
//
// Fred named the placements. 1.1 wants a digit before the first "_", and neither schema
// admits one against a base prefix the other owns -- "0o_17" is a string at both, since
// 1.1 has no "0o" form and 1.2's has no underscore.
//
// The bare column is what the schema reads; the tagged column follows from
// TestATagIsNotLaxerThanTheBareSpelling and is stated anyway, because it is the half that
// was wrong.
func TestTheUnderscorePlacementsHold(t *testing.T) {
	for _, tc := range []struct {
		text     string
		bare11   string
		bare12   string
		tagged11 string
	}{
		{"_1_0", "string:_1_0", "string:_1_0", "refused"},
		{"1_0_", "int:10", "string:1_0_", "int:10"},
		{"0_17", "int:15", "string:0_17", "int:15"},
		{"1__0", "int:10", "string:1__0", "int:10"},
		{"0x_10", "int:16", "string:0x_10", "int:16"},
		{"0b_1_0", "int:2", "string:0b_1_0", "int:2"},
		{"0o_17", "string:0o_17", "string:0o_17", "refused"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			require.Equal(t, tc.bare11, asValue("1.1", tc.text).String(), "bare under %%YAML 1.1")
			require.Equal(t, tc.bare12, asValue("1.2", tc.text).String(), "bare under %%YAML 1.2")
			require.Equal(t, tc.tagged11, asValue("1.1", "!!int "+tc.text).String(),
				"tagged under %%YAML 1.1")

			// Every one of these is a string under 1.2, so the tag has nothing to
			// name and must refuse rather than reach for Go's parser.
			require.Equal(t, "refused", asValue("1.2", "!!int "+tc.text).String(),
				"tagged under %%YAML 1.2")
		})
	}
}

// asJSON reads the scalar in a value position through codec.ToJSON.
//
// ToJSON walks the tree and writes the scalar itself, so it is a third reader beside the
// decoder and the tree. It carried its own copy of the 1.2 re-sniff in codec.taggedInteger
// and codec.taggedFloat until 7e20582, which is how it came to write 0 for a document the
// decoder read as 5.
func asJSON(version, scalar string) resolution {
	out, err := codec.ToJSON([]byte(atVersion(version, "v: "+scalar)))
	if err != nil {
		return resolution{Refused: true}
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		return resolution{Refused: true}
	}

	return resolution{Value: renderJSON(got["v"])}
}

// renderJSON writes what encoding/json built, which is a float64 for every number.
//
// The decoder answers with Go's integer types and ToJSON's number comes back through
// encoding/json as a float64, so the two cannot be compared as Go values. Comparing the
// number is the point, and a float64 carries every integer these documents spell.
func renderJSON(v any) string {
	if n, ok := v.(float64); ok {
		// A signed zero survives the round to an integer here. int64(-0.0) is 0,
		// and formatting that reported ToJSON as writing 0 for a document where it
		// wrote -0.0 -- the two readers were made to disagree by the harness, and
		// the ledger carried the direction backwards for a day because of it.
		// numberOf spells a zero the same way, so the two sides stay comparable.
		if n == 0 {
			if math.Signbit(n) {
				return "num:-0"
			}

			return "num:0"
		}
		if n == math.Trunc(n) && math.Abs(n) < 1<<53 {
			return "num:" + strconv.FormatInt(int64(n), 10)
		}

		return "num:" + strconv.FormatFloat(n, 'g', -1, 64)
	}

	return fmt.Sprintf("%T:%v", v, v)
}

// numberOf reduces a decoder reading to the same shape renderJSON writes, so the two
// readers are compared on the number and not on the Go type that carried it.
func numberOf(r resolution) (string, bool) {
	switch {
	case r.Refused:
		return "", false
	case strings.HasPrefix(r.Value, "int:"):
		return "num:" + strings.TrimPrefix(r.Value, "int:"), true
	case strings.HasPrefix(r.Value, "*big.Float:"), strings.HasPrefix(r.Value, "float64:"):
		_, digits, _ := strings.Cut(r.Value, ":")
		f, err := strconv.ParseFloat(digits, 64)
		if err != nil {
			return "", false
		}
		if f == 0 {
			// A signed zero is a value this library keeps deliberately -- see the
			// negative-zero fix in codec -- so the two zeros are not folded here.
			if math.Signbit(f) {
				return "num:-0", true
			}

			return "num:0", true
		}
		if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return "num:" + strconv.FormatInt(int64(f), 10), true
		}

		return "num:" + strconv.FormatFloat(f, 'g', -1, 64), true
	default:
		return "", false
	}
}

// TestToJSONAgreesWithTheDecoderOnATaggedNumber holds the two readers together.
//
// A tagged scalar is resolved once and read by three paths: the decoder, the tree and
// codec.ToJSON. Each carried its own copy of the base-and-schema decision, so they could
// disagree about one document without any of them looking wrong on its own -- under
// %YAML 1.1 "!!int 0b101" decoded to 5 while ToJSON wrote 0, and neither number is
// obviously the mistaken one until they are put side by side.
//
// ast.Resolution.Type is the single answer now, so this compares the readers rather than
// pinning what either of them says. Nothing here states a value: the property is that the
// two agree, which survives every ruling still to come about what a tag may spell.
//
// Only a number is compared. JSON has one number type, so a document the decoder refuses
// and ToJSON refuses agree by refusing, and a scalar that reads as a string on either side
// is left alone -- ToJSON's quoting rules are not this axis's question.
func TestToJSONAgreesWithTheDecoderOnATaggedNumber(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		kind := rapid.SampledFrom(numberKinds).Draw(rt, "kind")
		text := kind.tag + " " + kind.draw(rt)
		version := rapid.SampledFrom(versions).Draw(rt, "version")

		decoded, written := asValue(version, text), asJSON(version, text)

		if notJSON(decoded) {
			// JSON has no spelling for an infinity or a NaN, so ToJSON refuses a
			// document the decoder reads perfectly well. That is ErrNotJSON doing
			// its job and not a disagreement about the characters.
			return
		}

		if decoded.Refused != written.Refused {
			rt.Fatalf("%%YAML %s: %q is %s to the decoder and %s to ToJSON;"+
				" one document, two answers about whether it reads",
				version, text, decoded, written)
		}

		want, isNumber := numberOf(decoded)
		if !isNumber {
			return
		}
		if want != written.Value {
			rt.Fatalf("%%YAML %s: %q decodes to %s and ToJSON writes %s;"+
				" the two readers resolved the same characters differently",
				version, text, want, written.Value)
		}
	})
}

// notJSON says the decoder built a number JSON cannot write.
//
// ".inf", "-.inf" and ".nan" are floats YAML resolves and JSON has no spelling for, so
// codec.ToJSON refuses them by design. Comparing the two readers on such a document would
// measure that rule instead of the resolution the axis is about.
func notJSON(r resolution) bool {
	if !isFloat(r) {
		return false
	}
	f, err := strconv.ParseFloat(strings.TrimPrefix(r.Value, "float64:"), 64)

	return err != nil || math.IsInf(f, 0) || math.IsNaN(f)
}

// asQuoted reads the scalar double-quoted, which says "these characters are a string" and
// must not buy a different number under a tag.
func asQuoted(version, tagged string) resolution {
	tag, text, found := strings.Cut(tagged, " ")
	if !found {
		return resolution{Refused: true}
	}

	return asValue(version, tag+` "`+text+`"`)
}

// TestAQuotedScalarReadsAsThePlainOne is the property the three ratchets stood in for.
//
// 3.3.2 gives a non-specific tag -- "!" to a non-plain scalar and "?" to everything else --
// only to a node lacking an explicit tag. A node carrying one has no non-specific tag left
// to resolve, so `!!int "017"` and `!!int 017` are the same tag over the same content and
// nothing may tell them apart.
//
// Four readings did tell them apart until 183f3d0: ast.readsAs keyed on the scanner's
// type, Decoder.taggedValue resolved the node instead of the tagged text, codec's walk did
// the same, and castToInteger read that text under a fixed Schema12. Between them they
// produced three faults that looked separate -- a quoted scalar read under 1.2 whatever
// the document declared, `!!float "0x10"` reading 0.0 where the plain spelling read 16.0,
// and `!!float -0` dropping a sign the quoted spelling kept.
//
// The worst of them changed a number without refusing: under %YAML 1.1 `!!int 017` was 15
// written plainly and 17 written in quotes, both integers, so a document quietly lost two
// units when someone added quotes.
//
// This states no value, so a ruling about what a tag may spell moves the answers and not
// this property.
func TestAQuotedScalarReadsAsThePlainOne(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		kind := rapid.SampledFrom(numberKinds).Draw(rt, "kind")
		tagged := kind.tag + " " + kind.draw(rt)
		version := rapid.SampledFrom(versions).Draw(rt, "version")

		for _, position := range []struct {
			name  string
			plain func(string, string) resolution
		}{
			{"value", asValue},
			{"key", asKey},
		} {
			plain := position.plain(version, tagged)
			quoted := asQuoted(version, tagged)

			if plain.String() != quoted.String() {
				rt.Fatalf("%%YAML %s, %s position: %q is %s and the same tag over the same"+
					" characters quoted is %s; 3.3.2 leaves a tagged node no non-specific"+
					" tag to resolve, so quoting cannot change the reading",
					version, position.name, tagged, plain, quoted)
			}
		}
	})
}

// TestTheQuotedSpellingsThatUsedToDiffer pins the eight that did.
//
// A deterministic guard beside the drawn one, so a regression names a spelling instead of
// a seed. Every row read differently quoted before 183f3d0; `!!float "0x10"` was 0.0
// against 16.0 at both versions, and `!!int 017` was 17 against 15 under %YAML 1.1.
func TestTheQuotedSpellingsThatUsedToDiffer(t *testing.T) {
	for _, tagged := range []string{
		"!!int 017", "!!int 09", "!!int 0o17", "!!int 0b101", "!!int 1_000",
		"!!float 0x10", "!!float .inf", "!!float -0",
	} {
		for _, version := range versions {
			t.Run(version+" "+tagged, func(t *testing.T) {
				assert.Equal(t, asValue(version, tagged).String(), asQuoted(version, tagged).String(),
					"quoting changed the reading")
			})
		}
	}
}
