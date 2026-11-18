// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// toJSONViaValues is the converter ToJSON was until it began writing the
// document as the parser finished it: unmarshal into an ordered map, marshal
// that back out. It is kept here as the oracle the folding converter is held
// to.
func toJSONViaValues(src []byte) ([]byte, error) {
	var v any
	if err := codec.UnmarshalWithOptions(src, &v, codec.UseOrderedMap()); err != nil {
		return nil, err
	}

	return codec.MarshalWithOptions(v, codec.JSON())
}

// TestToJSONMatchesTheValueConverter converts every document of the corpus both
// ways and compares the values that come back.
//
// The two are compared as JSON values rather than as bytes: the folding
// converter writes compactly and the value converter wrote a space after every
// ':' and ',', which is a difference in spelling and not in content.
//
// A document the value converter cannot write as JSON at all is skipped, named
// in intendedJSONDivergence with what the folding converter does instead.
//
//nolint:gocognit // converts each corpus document both ways and compares them, minus the excused divergences
func TestToJSONMatchesTheValueConverter(t *testing.T) {
	t.Parallel()

	var compared, skipped, refused int
	for _, src := range jsonSources(t) {
		if directiveNamedLikeAnAnchorInText(src.text) {
			// "%&AML 1.2" is a directive named "&AML", and the walk reads the
			// "&AML" as an anchor and hands the "1.2" back as the whole
			// document. ToJSON walks, so it converts the 1.2; the value
			// converter reads the document under the directive.
			// codec.TestDefectADirectiveNamedLikeAPropertyIsReadAsOne pins it.
			skipped++

			continue
		}
		if strings.Contains(src.text, "? <<") {
			// A merge key written the long way. In flow the tree merges it and
			// the walk does not, and ToJSON -- which walks -- writes a key with
			// no value for it: `{? <<: {x: 1}, ...}` comes out as `{"",...`,
			// which is not JSON at all. That is sharper than the value
			// disagreement and is recorded on the pin,
			// yamlgen_test.TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge.
			skipped++

			continue
		}
		if strings.Contains(src.text, "<<") && (strings.Contains(src.text, ",-") || strings.Contains(src.text, ", -")) {
			// A "-" inside a flow merge sequence. The value converter reads the
			// element as a sequence and refuses the document; the folding one
			// reads it as the mapping and merges. Held by the last subtest of
			// yamlgen_test.TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree,
			// and held out here for the same reason
			// codec.TestWalkMatchesTheStream holds it out: the two paths
			// disagree before either converter is reached.
			skipped++

			continue
		}
		if strings.Contains(src.text, "&!") {
			// The parse reads "&!a1" as an anchor with no name followed by the
			// tag "!a1", rather than as an anchor named "!a1" -- ns-anchor-name
			// excludes the flow indicators and nothing else, so "!" belongs in
			// a name. The tree the two converters read is the same; they make
			// different nonsense of it, and neither is right. Held out here
			// until the parser is fixed.
			skipped++

			continue
		}

		want, wantErr := toJSONViaValues([]byte(src.text))
		got, gotErr := codec.ToJSON([]byte(src.text))

		if errors.Is(gotErr, yamlerrors.ErrNotJSON) {
			// ToJSON parses with parser.WithJSONCompatible, so it refuses a
			// document JSON has no spelling for: a collection used as a mapping
			// key or an alias naming one, an infinity or NaN, and a cycle.
			//
			// The value converter answered the first three by inventing a
			// spelling, "[a b]" for the key and null for the number, so there
			// is nothing there to agree about, and a cycle it refuses too.
			//
			// It may also refuse for a reason of its own, and the reason is not
			// enumerated here. This assertion used to require a cycle and broke
			// on a mutant that is malformed twice over: ToJSON refused it for
			// the "-.inf" it carries and the value converter refused it for the
			// "!!bool tru" it also carries, both correctly. Listing the
			// permitted refusals means re-listing them every time the parser
			// learns to refuse something new.
			//
			// Nothing is lost by not constraining it. ToJSON has already
			// refused, so there is no output to compare; the direction that
			// matters -- one converter accepting what the other refuses -- is
			// asserted in the branch below.
			if wantErr != nil {
				t.Logf("%s: ToJSON refuses it as not JSON and the value converter refuses it too: %v",
					src.name, firstLine(wantErr.Error()))
			}
			refused++

			continue
		}
		if gotErr != nil && wantErr == nil && losesATaggedFlowKeyAlonesAnchor(src.text, gotErr) {
			// A recorded defect: ToJSON loses an anchor declared on a flow
			// entry written as a key alone whose tag stands before it. Pinned
			// in TestDefectToJSONLosesAnAnchorOnATaggedFlowKeyAlone.
			t.Logf("%s: a tagged flow key written alone loses its anchor", src.name)
			skipped++

			continue
		}

		if wantErr != nil || gotErr != nil {
			// Both must refuse, and for the same reason: a converter that
			// accepts what the other refuses is a different converter.
			assert.Equalf(t, wantErr != nil, gotErr != nil,
				"%s: value converter err=%v, folding converter err=%v", src.name, wantErr, gotErr)

			continue
		}

		wantValue, err := readJSON(want)
		if err != nil {
			// The value converter wrote something no JSON parser reads. There
			// is nothing to compare against; the folding converter is held to
			// writing JSON, which is the whole of the fix.
			_, gotErr := readJSON(got)
			require.NoErrorf(t, gotErr,
				"%s: value converter wrote %q, which is not JSON, and the folding converter wrote %q, which is not JSON either",
				src.name, want, got)
			skipped++

			continue
		}

		gotValue, err := readJSON(got)
		if err != nil && mergesAValueWrittenInPlace(src.text) {
			// A recorded defect: ToJSON performs the merge and writes the "<<"
			// entry's own slot as well, so the output carries a member with no
			// key. Pinned in TestDefectAMergeWrittenInPlaceMakesToJSONWriteBrokenJSON.
			t.Logf("%s: a merge written in place makes ToJSON write JSON that will not parse", src.name)
			skipped++

			continue
		}
		require.NoErrorf(t, err, "%s: folding converter wrote %q, which is not JSON", src.name, got)

		if directiveNamedLikeAProperty(src.text) {
			// A recorded defect: a directive whose name begins with "&" is
			// read as an anchor, and ToJSON writes the node it names as the
			// whole document. Pinned in
			// TestDefectADirectiveNamedLikeAPropertyIsReadAsOne.
			t.Logf("%s: a directive named like an anchor is read as one", src.name)
			skipped++

			continue
		}

		var excused []string
		if !sameJSON(wantValue, gotValue, &excused) {
			if reason, ok := knownJSONDivergence(wantValue, gotValue); ok {
				t.Logf("%s: %s", src.name, reason)
				skipped++

				continue
			}

			if opensWithAnEmptyDocument(src.text) {
				// A recorded defect: ToJSON skips an empty first document and
				// the decoder keeps it, so "---" over "---" over "b: 2"
				// converts to {"b":2} one way and null the other. Pinned in
				// TestDefectToJSONSkipsAnEmptyFirstDocument. They agree on
				// every stream whose first document has content.
				t.Logf("%s: ToJSON skips an empty first document", src.name)
				skipped++

				continue
			}

			if carriesATimestampOrBinaryTag(src.text) {
				// A recorded defect: ToJSON writes a !!timestamp as the
				// scalar's source text and the value converter writes the
				// time.Time the decoder resolved, so "2001-12-14t21:59:43.1Z"
				// and "2001-12-14T21:59:43.1Z" convert two ways. As a key both
				// tags diverge too -- the decoder names one by Go's %v and
				// ToJSON by the text. Pinned in
				// TestDefectToJSONWritesATimestampAsItWasSpelled; the key half
				// is yamlcorpus.Departures' "a key tagged !!timestamp".
				//
				// Asked only once the two have disagreed, so a document
				// carrying one of these tags that converts the same way both
				// ways is still compared. construct-binary is one.
				t.Logf("%s: a timestamp or a binary is written two ways", src.name)
				skipped++

				continue
			}
			assert.Failf(t, "converters disagree",
				"%s\nvalue converter: %s\nfolding converter: %s", src.name, want, got)
		}

		if len(excused) > 0 {
			t.Logf("%s: %s", src.name, excused[0])
			skipped++

			continue
		}

		compared++
	}

	t.Logf("%d documents converted the same way, %d diverge on purpose, %d refused as not JSON",
		compared, skipped, refused)
}

// knownJSONDivergence names a difference the folding converter makes on
// purpose, given the two values that came back.
//
// One so far, a place where the value converter lost something on its way
// through Go values: a number too wide for int64 or float64 went out as a
// quoted string. It is a number and is written as one.
//
// Two more are not visible here because the value converter's output is not
// JSON at all and the comparison never reaches this: a control character went
// out with YAML's "\a" escape, which JSON has no spelling for, and a merge key
// wrote every merged entry and then the mapping's own, so an overridden key was
// written twice.
//
// Two have stopped being divergences. Infinity and NaN were written bare as
// ".inf" and ".nan"; the folding converter refuses them. A collection reached
// through an alias key -- "? *x", where the anchor names one -- went out as Go
// printed it, "[a b]", against the key's own JSON ["a","b"]; the parser follows
// the alias to what it names and refuses the key, so neither converter writes
// it.
func knownJSONDivergence(want, got any) (string, bool) {
	wantText, isText := want.(string)

	gotNumber, isNumber := got.(json.Number)
	if !isText || !isNumber {
		return "", false
	}

	// Compared by value and not through a float64, because the numbers this
	// fires on are exactly the ones a float64 cannot hold: a big.Int of forty
	// digits, a big.Float of -2.91e+1267. Float64 conversion returned a range
	// error for those and the excuse never fired.
	if sameNumber(json.Number(wantText), gotNumber) {
		return "a number too wide for a machine word is written as a number, not a string", true
	}

	// A recorded defect rather than a spelling difference: an explicit "!!int"
	// on an integer past a machine word makes ToJSON write math.MinInt64,
	// whatever the value and whatever its sign. Untagged, it writes the number;
	// the decoder reads either as a big.Int. Pinned in
	// TestDefectAnIntTagOnAWideIntegerWritesMinInt64.
	if gotNumber.String() == "-9223372036854775808" && wantText != gotNumber.String() {
		return "an explicit !!int on a wide integer makes ToJSON write MinInt64", true
	}

	return "", false
}

// keyNamingTheSameNumber returns the entry of g whose key spells the same
// number as k, where the two texts are different.
//
// Both keys have to parse as a float for this to fire, so it never rescues a
// pair of keys that merely look alike.
func keyNamingTheSameNumber(g map[string]any, k string) (any, bool) {
	want, err := strconv.ParseFloat(k, 64)
	if err != nil {
		return nil, false
	}

	for other, v := range g {
		got, err := strconv.ParseFloat(other, 64)
		if err == nil && got == want {
			return v, true
		}
	}

	return nil, false
}

// opensWithAnEmptyDocument reports whether src's first document holds nothing,
// which is the shape the two converters disagree about.
//
// A crude scan and deliberately so: the first line that is not a directive, a
// comment or blank has to be a "---", and the line after it has to be another
// "---" or a "...". Anything else means the first document has content and the
// two converters agree.
//
// Split on either break character. Style.Break writes a lone "\r" for a third
// of the corpus, and a scan that only knows "\n" reads such a document as one
// line -- which is how this missed fuzzseed/3226 on its first try.
func opensWithAnEmptyDocument(src string) bool {
	var seen int

	for line := range strings.FieldsFuncSeq(src, func(r rune) bool { return r == '\n' || r == '\r' }) {
		// Trimmed before the prefixes are read: a comment may be indented, and
		// " # c" is as empty a document body as "" is.
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		seen++
		if seen == 1 {
			if !strings.HasPrefix(line, "---") || strings.TrimSpace(line) != "---" {
				return false
			}

			continue
		}

		return strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...")
	}

	return false
}

// carriesATimestampOrBinaryTag reports whether src tags a node !!timestamp or
// !!binary, in any of the three spellings a tag has.
//
// Matched on the suffix rather than on "!!timestamp", so the verbatim
// "!<tag:yaml.org,2002:timestamp>" and a handle declared by a %TAG line are
// caught too. It is wider than the defect -- a local tag named "!timestamp"
// would match and resolves to nothing -- and being wider only skips documents.
func carriesATimestampOrBinaryTag(src string) bool {
	return strings.Contains(src, "timestamp") || strings.Contains(src, "binary")
}

// numberBeyondBigFloat is the recorded defect, and it is the decoder's rather
// than the converter's.
//
// A big.Float holds its exponent in an int32, so a number past 1e2147483647
// cannot be parsed into one. The decoder falls back to a float64 there and
// hands back **zero**, with nothing reported -- so the value converter, which
// goes through the decoder, writes 0 where ToJSON writes what the document
// said.
//
// ToJSON is right and is not excused here. JSON puts no bound on the magnitude
// of a number: RFC 8259 §6 says an implementation *may* set limits and warns
// about interoperability, and forbids nothing. A reader holding the value in a
// big.Float or a json.Number reads it back exactly.
func numberBeyondBigFloat(want, got any) (string, bool) {
	gotNumber, isNumber := got.(json.Number)
	if !isNumber {
		return "", false
	}

	if _, ok := new(big.Float).SetPrec(512).SetString(gotNumber.String()); ok {
		return "", false
	}

	wantNumber, wantIsNumber := want.(json.Number)
	if !wantIsNumber {
		return "", false
	}

	if zero, err := wantNumber.Float64(); err != nil || zero != 0 {
		return "", false
	}

	return "the decoder reads a number past big.Float's exponent as zero, where ToJSON writes the text", true
}

// jsonCounterpart returns the value got holds for want's key k: under k
// itself, or under a name one of the two converters gives the key differently,
// which is a recorded defect rather than a disagreement about the document.
func jsonCounterpart(got map[string]any, k string, excused *[]string) (any, bool) {
	if other, found := got[k]; found {
		return other, true
	}

	// An explicit "!!float" on a key loses its float-ness through the decoder,
	// so the value converter writes "226" where ToJSON writes "226.0".
	// Untagged, both write "226.0".
	if float, renamed := got[k+".0"]; renamed {
		*excused = append(*excused,
			"an explicit !!float on a key is named as an integer through the decoder")

		return float, true
	}

	// The other way a key is named twice: a float key written the long way
	// keeps its source text through ToJSON. "? 1e3" over ": x" converts to
	// {"1e3":"x"} where "1e3: x" converts to {"1000.0":"x"}, and the decoder
	// names both "1000.0". Pinned in
	// TestDefectToJSONNamesALongFormFloatKeyByItsText.
	if number, renamed := keyNamingTheSameNumber(got, k); renamed {
		*excused = append(*excused,
			"a float key written the long way keeps its source text through ToJSON")

		return number, true
	}

	return nil, false
}

// sameJSON compares two decoded documents, with numbers compared by value.
//
// UseNumber keeps a number as the text it was written with, and the two
// converters legitimately spell one number two ways: 4.810363808109991e-10 and
// 0.0000000004810363808109991 are the same number and neither is wrong. So the
// comparison reads them as arbitrary-precision decimals rather than as text,
// which is the only way to be both magnitude-safe and spelling-safe.
func sameJSON(want, got any, excused *[]string) bool {
	switch w := want.(type) {
	case json.Number:
		g, ok := got.(json.Number)
		if !ok {
			return false
		}

		if sameNumber(w, g) {
			return true
		}

		// A leaf that differs may still be a recorded defect, and the
		// comparison has to say so here rather than at the document, which is
		// where the difference is not.
		if reason, known := numberBeyondBigFloat(w, g); known {
			*excused = append(*excused, reason)

			return true
		}

		return false
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for k, v := range w {
			other, found := jsonCounterpart(g, k, excused)
			if !found || !sameJSON(v, other, excused) {
				return false
			}
		}

		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(w) != len(g) {
			return false
		}

		for i := range w {
			if !sameJSON(w[i], g[i], excused) {
				return false
			}
		}

		return true
	case string:
		if reason, known := knownJSONDivergence(want, got); known {
			*excused = append(*excused, reason)

			return true
		}

		return assert.ObjectsAreEqual(want, got)
	default:
		return assert.ObjectsAreEqual(want, got)
	}
}

// sameNumber compares two JSON numbers, at the width the value actually has.
//
// A number both sides can hold in a float64 is compared as one, because that is
// what such a number denotes and the two converters reach it by different
// routes: one round-trips through a double and writes the shortest text back,
// the other writes what the document said. -18097.449829101555 and
// -18097.4498291015562 are one double spelled two ways, and neither is wrong.
//
// A number too wide for a double is compared exactly, as an arbitrary-precision
// decimal. That is the case this function exists for: JSON puts no bound on
// magnitude and ToJSON writes what the document said, so "1e400" has to survive
// the comparison rather than fail it.
func sameNumber(a, b json.Number) bool {
	const precision = 512

	af, aerr := a.Float64()
	bf, berr := b.Float64()

	if aerr == nil && berr == nil && !math.IsInf(af, 0) && !math.IsInf(bf, 0) {
		return af == bf
	}

	x, okA := new(big.Float).SetPrec(precision).SetString(a.String())
	y, okB := new(big.Float).SetPrec(precision).SetString(b.String())

	if !okA || !okB {
		return a.String() == b.String()
	}

	return x.Cmp(y) == 0
}

// readJSON decodes with UseNumber, so a number keeps the text it was written
// with instead of going through a float64.
//
// That matters here rather than being a nicety. JSON puts no bound on the
// magnitude of a number -- RFC 8259 §6 says an implementation *may* set limits
// and warns about interoperability, and forbids nothing -- and ToJSON writes
// what the document said on purpose, so "1e400" is a number it is right to
// emit. Unmarshalling into an `any` maps every number to a float64 and refuses
// that one, which would have made this test call a correct conversion a
// failure.
//
// It also makes the comparison stricter: two converters that write "1" and
// "1.0" for the same value now differ, where a float64 hid it.
func readJSON(text []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.UseNumber()

	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}

	return out, nil
}

type jsonSource struct{ name, text string }

// jsonSources is every document the converters are held to: the YAML test
// suite, the generated shapes and the fuzz seeds.
func jsonSources(t *testing.T) []jsonSource {
	t.Helper()

	var srcs []jsonSource

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, s := range suites {
		srcs = append(srcs, jsonSource{name: "suite/" + s.Name, text: string(s.InYAML)})
	}

	for _, c := range []struct {
		name string
		gen  func(int) string
	}{
		{"flat-map", corpus.FlatMap},
		{"flat-sequence", corpus.FlatSequence},
		{"nested-doc", corpus.NestedDoc},
		{"anchored", corpus.Anchored},
		{"block-scalars", corpus.BlockScalars},
	} {
		for _, n := range []int{1, 10, 200} {
			srcs = append(srcs, jsonSource{
				name: fmt.Sprintf("corpus/%s-%d", c.name, n),
				text: c.gen(n),
			})
		}
	}

	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	for i, seed := range seeds {
		srcs = append(srcs, jsonSource{name: fmt.Sprintf("fuzzseed/%04d", i), text: seed})
	}

	return srcs
}

// firstLine keeps a log line to the parser's own words, since an error here
// carries the offending source and a caret under it.
// losesATaggedFlowKeyAlonesAnchor reports whether ToJSON refused src for the
// anchor it dropped from a flow entry written as a key alone.
//
// Both halves are required: the complaint, and a tag standing before an anchor
// inside a flow collection. The complaint on its own is one the parser makes
// about genuinely undefined aliases.
func losesATaggedFlowKeyAlonesAnchor(text string, err error) bool {
	return strings.Contains(err.Error(), "could not find alias") && taggedAnchorInFlow.MatchString(text)
}

var taggedAnchorInFlow = regexp.MustCompile(`[\[{][^\]}]*![^\s\[{]*\s+&`)

// directiveNamedLikeAProperty reports whether src opens with a directive whose
// name begins with the anchor or alias indicator.
func directiveNamedLikeAProperty(text string) bool {
	return strings.HasPrefix(text, "%&") || strings.HasPrefix(text, "%*")
}

// mergesAValueWrittenInPlace reports whether src gives a "<<" key a collection
// written where it stands rather than an alias to one.
//
// Deliberately crude, and it only ever runs on a document whose JSON already
// failed to parse: a false positive costs one excused case, where reproducing
// the parser here to be exact would cost a second parser to keep in step.
func mergesAValueWrittenInPlace(src string) bool {
	return mergeInPlace.MatchString(src)
}

var mergeInPlace = regexp.MustCompile(`(^|[\s{,])<<\s*:\s*[\[{]`)

func firstLine(text string) string {
	for i := range len(text) {
		if text[i] == '\n' {
			return text[:i]
		}
	}

	return text
}

// directiveNamedLikeAnAnchorInText is codec.directiveNamedLikeAnAnchor, which
// this file cannot reach across the package boundary.
//
// A byte order mark stands before the directives and a lone "\r" is a line
// break, so neither a split on "\n" nor a prefix test on the whole text finds
// the line -- the same two traps the other copy carries a comment about.
func directiveNamedLikeAnAnchorInText(src string) bool {
	for line := range strings.FieldsFuncSeq(src, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if strings.HasPrefix(strings.TrimPrefix(line, "\ufeff"), "%&") {
			return true
		}
	}

	return false
}

// TestToJSONSpelling checks the text ToJSON writes, not the value it stands for.
//
// A float keeps the digits the document wrote wherever JSON spells a number the
// same way, so "1e3" stays "1e3" and a value of twenty-five significant digits
// keeps all of them. Where JSON spells it differently -- ".5", "5.", "007.5",
// "+1.5" -- the text is rewritten to JSON's grammar, digit for digit. Written
// through a Go value, "+0.12345678901234567890123" was rounded to seventeen
// digits and ".5e10" came out as 5e+09. YAML 1.1's base-60 floats and its octal
// integers under "!!float" have no such rewrite and are written as their value.
//
// TestToJSONMatchesTheValueConverter compares what json.Unmarshal reads back,
// and JSON has one number type: it cannot tell 1.0 from 1, so a converter that
// stopped writing the fractional part passed it. A document that wrote a float
// and converts back to YAML should still hold one, since a bare "1" reads as an
// integer.
func TestToJSONSpelling(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a: 1.0\n", `{"a":1.0}`},
		{"a: 1e3\n", `{"a":1e3}`},
		{"a: 1.230\n", `{"a":1.230}`},
		{"a: 0.1234567890123456789012345\n", `{"a":0.1234567890123456789012345}`},
		{"a: .5\n", `{"a":0.5}`},
		{"a: 5.\n", `{"a":5.0}`},
		{"a: 007.5\n", `{"a":7.5}`},
		{"a: +1.5\n", `{"a":1.5}`},
		{"a: +0.12345678901234567890123\n", `{"a":0.12345678901234567890123}`},
		{"a: .5e10\n", `{"a":0.5e10}`},
		{"a: !!float +1.5\n", `{"a":1.5}`},
		{"a: !!float 0.12345678901234567890123\n", `{"a":0.12345678901234567890123}`},
		{"a: !!float .5e10\n", `{"a":0.5e10}`},
		{"%YAML 1.1\n---\na: 1_000.5\n", `{"a":1000.5}`},
		// A key is a string named after its value, as a bare "1e3" key names
		// its entry 1000.0, so an anchored float written from its digits as a
		// value still names the entry an alias keys by its value.
		{"a: &a1 !!float 1e3\n*a1 : v\n", `{"a":1e3,"1000.0":"v"}`},
		{"!!float &a 1e3: x\nb: *a\n", `{"1000.0":"x","b":1e3}`},
		// No text rewrite spells these, so they are written as their value.
		{"%YAML 1.1\n---\na: 190:20:30.15\n", `{"a":685230.15}`},
		{"%YAML 1.1\n---\na: !!float 017\n", `{"a":15.0}`},
		// Past and below what a float64 holds, both written as numbers. JSON
		// bounds neither, and what a reader makes of them is the reader's:
		// encoding/json refuses the first into a float64 and rounds the second
		// to zero, and json.Number reads both.
		{"a: 1e400\n", `{"a":1e400}`},
		{"a: 1e-400\n", `{"a":1e-400}`},
		{"a: 1.5\n", `{"a":1.5}`},
		{"a: -0.0\n", `{"a":-0.0}`},
		{"a: 3\n", `{"a":3}`},
		// "-0" is a JSON number and keeps its sign, as "-0.0" above does. Reading
		// it through strconv wrote it back as "0", so one converter dropped a sign
		// the other kept.
		{"a: -0\n", `{"a":-0}`},
		{"a: -9223372036854775808\n", `{"a":-9223372036854775808}`},
		{"a: 18446744073709551615\n", `{"a":18446744073709551615}`},
		{"a: 0x1F\n", `{"a":31}`},
		{"a: !!float 12\n", `{"a":12.0}`},
		{"a: 18446744073709551616\n", `{"a":18446744073709551616}`},
		{"a: 123456789012345678901234567890\n", `{"a":123456789012345678901234567890}`},
		{"a: 07\n", `{"a":7}`},
		{"a: \"07\"\n", `{"a":"07"}`},
		{"a: yes\n", `{"a":"yes"}`},
		{"a: |\n  x\n", `{"a":"x\n"}`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// TestToJSONRefusesWhatJSONCannotSpell records the documents ToJSON stops on
// rather than inventing a spelling for.
//
// It parses with parser.WithJSONCompatible. A collection used as a mapping key
// used to be written as the key's own JSON text -- {"[\"a\",\"b\"]":1} -- which
// no reader takes apart again, and the infinities and NaN used to be written as
// null, which loses them without saying so.
//
// An alias naming a collection goes with them: "a: &x [1, 2]" then "? *x" wrote
// {"a":[1,2],"[1,2]":3}, and the decoder wrote "[1 2]" for the same key. The
// parser follows the alias to what its anchor names, however deep in the
// document the anchor sits.
//
// So does a cycle, which JSON cannot hold anywhere: the conversion read the
// alias as naming an anchor it had not finished writing and reported it as
// missing, which said nothing about the cycle.
func TestToJSONRefusesWhatJSONCannotSpell(t *testing.T) {
	for src, want := range map[string]string{
		"? [a, b]\n: 1\n":  "a sequence cannot be a JSON key",
		"? {a: 1}\n: 2\n":  "a mapping cannot be a JSON key",
		"{? [a]: 1}\n":     "a sequence cannot be a JSON key",
		"? &x [a]\n: 1\n":  "a sequence cannot be a JSON key",
		"? !!seq [a]\n: 1": "a sequence cannot be a JSON key",
		"a: .inf\n":        "JSON has no number for .inf",
		"a: -.inf\n":       "JSON has no number for -.inf",
		"a: .nan\n":        "JSON has no number for .nan",
		".inf: a\n":        "JSON has no number for .inf",

		"a: &x [1, 2]\n? *x\n: 3\n":               "a sequence cannot be a JSON key",
		"a: &x {p: 1}\n? *x\n: 3\n":               "a mapping cannot be a JSON key",
		"a: &x !!seq [1]\n? *x\n: 3\n":            "a sequence cannot be a JSON key",
		"outer:\n  inner: &x [1, 2]\n? *x\n: 3\n": "a sequence cannot be a JSON key",
		"a: &x [ *x ]\n":                          "a cycle cannot be written as JSON",
		"a: &x {b: *x}\n":                         "a cycle cannot be written as JSON",
		"a: &x\n  - *x\n":                         "a cycle cannot be written as JSON",
		"a: &x [ &y [ *x ], *y ]\n":               "a cycle cannot be written as JSON",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := codec.ToJSON([]byte(src))
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrNotJSON)
			assert.Contains(t, err.Error(), want)

			// The same document parses, since it is well-formed YAML and only
			// the conversion is impossible.
			_, err = parser.ParseBytes([]byte(src))
			assert.NoError(t, err)
		})
	}
}

// TestToJSONConvertsTheFirstDocument holds ToJSON to the document Unmarshal
// reads, whatever the first document of the stream is.
//
// A document written as nothing between its markers hands over no node at all,
// so the writer counted the bodies it saw and made "---" over "---" over
// "b: 2" convert the second document as though it were the first --
// {"b":2} where the decoder reads null. Which document a node belongs to is
// parser.Step.Document's to say, and the writer reads it.
func TestToJSONConvertsTheFirstDocument(t *testing.T) {
	for _, tc := range []struct{ name, src, writes string }{
		{
			name:   "an empty first document is a null, not a skip",
			src:    "---\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "an explicit null reads the same way",
			src:    "---\nnull\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "a first document with a body",
			src:    "---\na: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "with no opening marker",
			src:    "a: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "a stream of empty documents",
			src:    "---\n---\n",
			writes: "null",
		},
		{
			name:   "one empty document",
			src:    "---\n",
			writes: "null",
		},
		{
			name:   "an empty stream",
			src:    "",
			writes: "null",
		},
		{
			name:   "a stream holding only a comment",
			src:    "# c\n",
			writes: "null",
		},
		{
			name:   "an empty last document does not reach the first",
			src:    "a: 1\n---\n",
			writes: `{"a":1}`,
		},

		// A "%YAML" or "%TAG" line is a document of the parse in its own
		// right, ahead of the one it applies to, so the document to convert is
		// the one past the directives.
		{
			name:   "a version directive",
			src:    "%YAML 1.2\n---\na: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "a tag directive",
			src:    "%TAG !e! tag:yaml.org,2002:\n---\na: !e!str 1\n",
			writes: `{"a":"1"}`,
		},
		{
			name:   "two directive lines",
			src:    "%YAML 1.2\n%TAG !e! tag:yaml.org,2002:\n---\na: !e!str 1\n",
			writes: `{"a":"1"}`,
		},
		{
			name:   "a directive over an empty document",
			src:    "%YAML 1.2\n---\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "a directive and nothing after it",
			src:    "%YAML 1.2\n---\n",
			writes: "null",
		},
		{
			name:   "a directive on a later document does not move the first",
			src:    "a: 1\n...\n%YAML 1.2\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			// The version is scoped to the document it opens, and that document
			// is the one converted.
			name:   "a 1.1 directive reaches the document it opens",
			src:    "%YAML 1.1\n---\na: yes\n---\nb: yes\n",
			writes: `{"a":true}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		})
	}
}

// TestToJSONReadsThePastTheFirstDocument is the other half of the contract: the
// rest of the stream is converted and thrown away, so a document JSON has no
// spelling for is refused wherever it stands rather than half-answered.
func TestToJSONReadsThePastTheFirstDocument(t *testing.T) {
	for _, tc := range []struct{ src, says string }{
		{src: "a: 1\n---\nb: .inf\n", says: "JSON has no number for .inf"},
		{src: "a: 1\n---\nb: *nope\n", says: `could not find alias "nope"`},
		{src: "a: 1\n---\n&x [ *x ]\n", says: "a cycle cannot be written as JSON"},

		// And it still reads past a first document that wrote nothing, which is
		// the case the skip used to hide.
		{src: "---\n---\nb: .inf\n", says: "JSON has no number for .inf"},
		{src: "---\n---\n[a]: 1\n", says: "a sequence cannot be a JSON key"},
	} {
		t.Run(tc.says, func(t *testing.T) {
			_, err := codec.ToJSON([]byte(tc.src))
			require.Errorf(t, err, "%q", tc.src)
			assert.Contains(t, err.Error(), tc.says, "%q", tc.src)
		})
	}

	t.Run("a duplicate key past the first document is still refused", func(t *testing.T) {
		_, err := codec.ToJSON([]byte("a: 1\n---\nb: 1\nb: 2\n"))
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}

// TestToJSONReadsTheTagsItKnows records what each tag of the YAML type
// repository converts to, and that a tag it does not know is transparent.
//
// A tag written out reaches the converter differently from one left implicit,
// and "!!merge <<: *base" is where that mattered: the merge machinery looked
// for an ast.MergeKeyNode and the key arrived wrapped in a tag, so the "<<"
// was written as a key with no value and the JSON did not parse.
func TestToJSONReadsTheTagsItKnows(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"str", "a: !!str 12\n", `{"a":"12"}`},
		{"int", `a: !!int "12"` + "\n", `{"a":12}`},
		{"float", "a: !!float 12\n", `{"a":12.0}`},
		{"bool", "a: !!bool yes\n", `{"a":true}`},
		{"null", "a: !!null ~\n", `{"a":null}`},
		{"binary", "a: !!binary aGk=\n", `{"a":"aGk="}`},

		// Transparent: the value is written as it stands.
		{"seq", "a: !!seq [1,2]\n", `{"a":[1,2]}`},
		{"map", "a: !!map {x: 1}\n", `{"a":{"x":1}}`},
		{"set", "a: !!set {x, y}\n", `{"a":{"x":null,"y":null}}`},
		// Not transparent: the tag names an ordered map, and JSON has no
		// ordering to lose, so the entries are written as one object holding
		// the order the sequence gave them.
		{"omap", "a: !!omap [{x: 1},{y: 2}]\n", `{"a":{"x":1,"y":2}}`},

		// A timestamp is written as the instant it names, in RFC 3339, which is
		// what the value converter writes for the time.Time the decoder builds.
		// yaml.org/type/timestamp.html spells one several ways this does not.
		{"timestamp", "a: !!timestamp 2001-12-14\n", `{"a":"2001-12-14T00:00:00Z"}`},
		// ⚠️ A tag the core schema does not resolve leaves its scalar as text,
		// digits and all. The scanner types a scalar by its own grammar and a
		// tag it cannot resolve is not that grammar, so "!thing 12" is the
		// string "12" and not the number.
		{"local", "a: !thing 12\n", `{"a":"12"}`},
		{"unknown namespace", "a: !<tag:example.com,2020:thing> 12\n", `{"a":"12"}`},

		// ⚠️ And so are the three tags of the 1.1 type repository nothing here
		// resolves. They are parsed and carried on the node; the value under
		// them stands as it was written.
		{"pairs", "a: !!pairs [{x: 1},{x: 2}]\n", `{"a":[{"x":1},{"x":2}]}`},
		{"value", "a: !!value =\n", `{"a":"="}`},
		{"yaml", "a: !!yaml '!'\n", `{"a":"!"}`},

		// The long form of a tag the core schema resolves. "!!int" is
		// shorthand for tag:yaml.org,2002:int, so the two mean the same.
		{"long form int", "a: !<tag:yaml.org,2002:int> \"12\"\n", `{"a":12}`},
		{"long form str", "a: !<tag:yaml.org,2002:str> 12\n", `{"a":"12"}`},
		{"long form bool", "a: !<tag:yaml.org,2002:bool> yes\n", `{"a":true}`},
		{"long form seq", "a: !<tag:yaml.org,2002:seq> [1,2]\n", `{"a":[1,2]}`},
		{"long form map", "a: !<tag:yaml.org,2002:map> {x: 1}\n", `{"a":{"x":1}}`},
		{"long form merge", "b: &b {x: 1}\na:\n  !<tag:yaml.org,2002:merge> <<: *b\n", `{"b":{"x":1},"a":{"x":1}}`},

		// The merge tag written out reads as the "<<" it stands on.
		{"merge", "b: &b {x: 1}\na:\n  !!merge <<: *b\n", `{"b":{"x":1},"a":{"x":1}}`},
		{"merge overridden", "b: &b {x: 1}\na:\n  !!merge <<: *b\n  x: 9\n", `{"b":{"x":1},"a":{"x":9}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			require.Truef(t, json.Valid(got), "wrote %s, which is not JSON", got)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// TestAFloatPastTheExponentBoundIsAnInfinityOrZero covers a float whose
// decimal exponent is past ±1000, which every path reads as an infinity of its
// sign, or zero.
//
// Two faults sat past the bound. A number past big.Float's own range decoded to
// 0 -- "1e2147483647" read as zero on the walk and the tree. And a number
// inside big.Float's range stalled every reader when written as a key: naming
// it wrote it as decimal, in time that grew about with the square of the
// exponent, so "1e10000000: a" took over a minute to parse.
func TestAFloatPastTheExponentBoundIsAnInfinityOrZero(t *testing.T) {
	inf, negInf := math.Inf(1), math.Inf(-1)

	for src, want := range map[string]float64{
		"a: 1e1001\n":          inf,
		"a: -1e1001\n":         negInf,
		"a: 1e2147483647\n":    inf,
		"a: -1e2147483647\n":   negInf,
		"a: 1e10000000\n":      inf,
		"a: 1e-1001\n":         0,
		"a: 1e-2147483647\n":   0,
		"a: !!float 1e2000\n":  inf,
		"a: !!float -1e2000\n": negInf,
	} {
		// An any goes down the walk, a MapSlice under UseOrderedMap builds the
		// tree, and a struct takes the typed walk.
		var walked any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &walked), "the walk: %q", src)
		asMap, isMap := walked.(map[string]any)
		require.Truef(t, isMap, "the walk: %q", src)
		assert.Equalf(t, want, asMap["a"], "the walk: %q", src)

		var tree codec.MapSlice
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &tree, codec.UseOrderedMap()), "the tree: %q", src)
		v, _ := tree.Get("a")
		assert.Equalf(t, want, v, "the tree: %q", src)

		var typed struct{ A float64 }
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "the typed walk: %q", src)
		assert.Equalf(t, want, typed.A, "the typed walk: %q", src)
	}

	t.Run("inside the bound a float too wide for a float64 stays a *big.Float", func(t *testing.T) {
		var walked any
		require.NoError(t, codec.Unmarshal([]byte("a: 1e1000\n"), &walked))
		asMap, isMap := walked.(map[string]any)
		require.True(t, isMap)
		assert.IsType(t, (*big.Float)(nil), asMap["a"], "the walk")
	})

	t.Run("as a key it is the key .inf is", func(t *testing.T) {
		var named map[string]string
		require.NoError(t, codec.Unmarshal([]byte("1e10000000: a\n"), &named))
		assert.Equal(t, map[string]string{".inf": "a"}, named)

		var keyed map[float64]string
		require.NoError(t, codec.Unmarshal([]byte("1e10000000: a\n"), &keyed))
		assert.Equal(t, map[float64]string{inf: "a"}, keyed)

		for _, src := range []string{"1e10000000: a\n.inf: b\n", "1e2000: a\n1e3000: b\n", "1e-2000: a\n0.0: b\n"} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})
}

// TestAFloatPastTheExponentBoundConvertsToTheNumberWritten covers ToJSON and
// ToJSONTokens on a float past ±1000.
//
// The decoder reads one as an infinity or zero, as Go's types require. JSON
// bounds no number, so both converters write the digits -- but only where the
// document spelled them as JSON does. "+1e1001", "1.e1001" and every spelling
// under "!!float" were read as the infinity and written as null, and
// "!!float 1e-1001" as 0.0.
func TestAFloatPastTheExponentBoundConvertsToTheNumberWritten(t *testing.T) {
	for src, want := range map[string]string{
		"k: !!float 1e1001\n":   "1e1001",
		"k: !!float -1e1001\n":  "-1e1001",
		"k: +1e1001\n":          "1e1001",
		"k: !!float +1e1001\n":  "1e1001",
		"k: 1.e1001\n":          "1e1001",
		"k: -1.e1001\n":         "-1e1001",
		"k: !!float 1e-1001\n":  "1e-1001",
		"k: !!float 1.5e1001\n": "1.5e1001",
		"k: !!float 007e1001\n": "7e1001",

		// Shapes outside the fault: JSON spells these as the document did.
		"k: 1e1001\n":  "1e1001",
		"k: 1E+1001\n": "1E+1001",
	} {
		t.Run(src, func(t *testing.T) {
			out, err := codec.ToJSON([]byte(src))
			require.NoError(t, err)
			assert.Equal(t, `{"k":`+want+`}`, string(out), "the digits as written")

			s := codec.ToJSONTokens([]byte(src))
			var got []string
			for tk := range s.Tokens() {
				got = append(got, fmt.Sprintf("%v:%s", tk.Kind, tk.Value))
			}
			require.NoError(t, s.Err())
			assert.Contains(t, got, "number:"+want)
		})
	}
}

// Both converters take the first document of a stream, empty or not.
//
// "---" over "---" over "b: 2" is a stream of two documents, the first empty.
// The decoder read the first and gave null; ToJSON wrote {"b":2}, which is the
// second -- so the library gave both answers to the question of what converting
// a stream should mean. They had always agreed on a stream whose first document
// has content, so it was the empty one they parted company over.
//
// ✅ Settled on 2026-09-13 in favor of the decoder's reading: ToJSON converts
// the first document whatever it holds. ToJSON counted the document bodies
// it saw, and a document written as nothing between its markers hands over no
// node, so the count never moved for it. parser.Step.Document now says which
// document a node belongs to, and the writer reads that instead.
// TestToJSONConvertsTheFirstDocument holds the whole contract; this test
// keeps the agreement between the two converters, which is what found it.
//
// Found on 2026-09-13 by the stream axis, once the corpus carried a document
// suffix into TestToJSONMatchesTheValueConverter.

// TestFixedBothConvertersTakeTheFirstDocument pins both sides.
func TestFixedBothConvertersTakeTheFirstDocument(t *testing.T) {
	t.Run("an empty first document is a null to both", func(t *testing.T) {
		const src = "---\n---\nb: 2\n"

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, "null", string(out), "ToJSON takes the first")

		assert.Equal(t, "null\n", throughValues(t, src), "and so does the decoder")
	})

	t.Run("a first document with content is taken by both", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "a: 1\n---\nb: 2\n", writes: `{"a":1}`},
			{src: "a: 1\n...\n---\nb: 2\n", writes: `{"a":1}`},
			{src: "1\n---\n2\n---\n3\n", writes: "1"},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.writes, string(out), "%q", tc.src)
		}
	})
}

// throughValues is the converter ToJSON is held against: decode, then marshal.
func throughValues(t *testing.T, src string) string {
	t.Helper()

	var v any
	require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()))

	out, err := codec.MarshalWithOptions(v, codec.JSON())
	require.NoError(t, err)

	return string(out)
}

// TestAKeyIsNamedByItsType covers the text a non-string mapping key addresses
// its entry by.
//
// The name is the type's own canonical spelling and not the document's. An
// integer is written in decimal whatever base it was read from, so "0x10" and
// "007" name 16 and 7 -- one key each, however many ways YAML spells it. A
// float always carries a '.' or an exponent, which keeps the floats out of the
// integers' namespace: "1" and "1.0" are an integer and a float, two keys, and
// two names.
//
// keyText wrote fmt.Sprint of the resolved value before this, which spells
// float64(1) and int(1) alike, so "1: a" beside "1.0: b" came out of ToJSON as
// {"1":"a","1":"b"} -- one name twice, which reads back with one of them gone.
func TestAKeyIsNamedByItsType(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want map[string]any
	}{
		// The two keys that started this.
		{src: "1: a\n1.0: b\n", want: map[string]any{"1": "a", "1.0": "b"}},

		// An integer in decimal, whatever base the document wrote.
		{src: "0x10: a\n", want: map[string]any{"16": "a"}},
		{src: "007: a\n", want: map[string]any{"7": "a"}},
		// ⚠️ "-0x1" is a string and not an integer: the core schema writes a
		// hexadecimal as "0x" and digits with no sign in front of the prefix,
		// so it resolves as text and is named by it. That is a separate open
		// question about the resolver, not about naming.
		{src: "-0x1: a\n", want: map[string]any{"-0x1": "a"}},

		// A float in the shortest text that reads back as the same float64,
		// with ".0" where that text has neither a point nor an exponent.
		{src: "1e3: a\n", want: map[string]any{"1000.0": "a"}},
		{src: "1.5e3: a\n", want: map[string]any{"1500.0": "a"}},
		{src: "1.00: a\n", want: map[string]any{"1.0": "a"}},
		{src: "0.1: a\n", want: map[string]any{"0.1": "a"}},

		// And the exponent stays where the shortest text keeps one, so a wide
		// float is not expanded to precision it never had.
		{src: "1e30: a\n", want: map[string]any{"1e+30": "a"}},
		{src: "1e23: a\n", want: map[string]any{"1e+23": "a"}},
		{src: "1e300: a\n", want: map[string]any{"1e+300": "a"}},
		{src: "1e-6: a\n", want: map[string]any{"1e-06": "a"}},

		// Every spelling of a null and of a boolean names one entry.
		{src: "~: a\n", want: map[string]any{"null": "a"}},
		{src: "NULL: a\n", want: map[string]any{"null": "a"}},
		{src: "True: a\n", want: map[string]any{"true": "a"}},
		{src: "FALSE: a\n", want: map[string]any{"false": "a"}},

		// A key written empty is a null, and a quoted empty key is not.
		{src: "\"\": a\nnull: b\n", want: map[string]any{"": "a", "null": "b"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			var decoded map[string]any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			assert.Equal(t, tc.want, decoded, "the decoder")

			var stringKeyed map[string]any
			require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &stringKeyed, codec.UseStringKeys()))
			assert.Equal(t, tc.want, stringKeyed, "UseStringKeys")

			converted, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)

			var fromJSON map[string]any
			require.NoError(t, json.Unmarshal(converted, &fromJSON))
			assert.Equal(t, tc.want, fromJSON, "ToJSON wrote %s", converted)

			// What it wrote holds as many names as the document wrote keys.
			// JSON with one name twice reads back with one of them gone.
			assert.Len(t, fromJSON, len(tc.want), "ToJSON wrote %s", converted)
		})
	}
}

// TestAnInfinityKeyTakesYAMLsSpelling covers the floats that are not finite.
//
// §10.2.1.4 gives ".inf" and ".nan" as the canonical forms, and they read back
// into YAML as the same value. Go's fmt gives "+Inf" and libfyaml gives
// "Infinity", which is JavaScript's spelling and follows no standard.
//
// codec.ToJSON never reaches them: parser.WithJSONCompatible refuses a document
// holding one, since JSON has no number for it either.
func TestAnInfinityKeyTakesYAMLsSpelling(t *testing.T) {
	for src, want := range map[string]string{
		".inf: a\n":  ".inf",
		"-.inf: a\n": "-.inf",
		".nan: a\n":  ".nan",
		".Inf: a\n":  ".inf",
		".NAN: a\n":  ".nan",
	} {
		t.Run(src, func(t *testing.T) {
			var decoded map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &decoded))
			assert.Equal(t, map[string]any{want: "a"}, decoded)

			_, err := codec.ToJSON([]byte(src))
			assert.Error(t, err, "JSON has no number for it")
		})
	}
}

// Two keys that resolve to different nodes and name themselves alike are read
// five ways, and only one of them is right.
//
// §3.2.1.1 makes two keys equal when they resolve to the same node, so `1: x`
// over `"1": y` is an integer key and a string key -- two nodes, two keys, two
// entries.
//
// The rule is general over the resolution table rather than a handful of cases,
// which `0x1f: x` over `"31": y` is the clearest way to see: those two keys
// share not one character, and the walk merges them, because a key is named by
// the canonical spelling of what it resolved to and 0x1f resolves to 31. The
// empty key reaches it the same way -- it resolves to null, whose spelling is
// "null" -- so `: x` over `"null": y` merges too.
//
// A fix wants to be general over that table rather than to add cases, and the
// one pair that looks like an exception is the reason to say so.
//
// `.inf: x` over `"+.inf": y` keeps both today, and NOT because the naming is
// careful there. It is that `+.inf` does not resolve at all: token.go's
// reservedInfKeywords lists `.inf`, `.Inf`, `.INF`, `-.inf`, `-.Inf`, `-.INF`
// and omits the three `+` spellings, where the 1.2 core schema float production
// is `[-+]? ( \.inf | \.Inf | \.INF )`. So the quoted key and the plain key are
// both strings and there is nothing to merge. `.nan` is the control and is
// right: 1.2 admits no sign there, and we, libfyaml and go.yaml.in/yaml/v3 all
// read `+.nan` as a string.
//
// ⚠️ **That row will start merging when the resolver is fixed**, and it should
// not be read as a regression here when it does. It is a separate defect
// standing in front of this one -- both loaders read `+.inf` and `+.INF` as
// infinity, and codec.ToJSON writes `{"a":"+.inf"}` where it refuses `.inf`
// outright, so the same value is a number or a string depending on which of two
// legal spellings the document used.
//
// What each destination does with that:
//
//	map[any]any     both, typed: uint64(1) => "x" and "1" => "y"   ✅
//	map[string]any  refused, `duplicate key "1"`
//	an `any`        one entry, {"1": "y"} -- "x" is gone, silently
//	UseOrderedMap   both entries, typed: uint64(1) and "1"         ✅
//	ToJSON          refused as ErrNotJSON                          ✅
//
// The map[any]any row is what makes this the naming rather than the read: the
// library holds the two apart wherever the destination can, so nothing is lost
// until a key is named by the canonical spelling of its type and both land in
// the strings' namespace. yamlcorpus.Departures records the reading; this pins
// which destination does what, since the five disagree and a caller has no way
// to know which one they are on.
//
// RFC 8259 section 4 says the names in a JSON object SHOULD be unique, so the
// JSON was not invalid -- but every reader collapses it, and "x" is lost one
// step later.
//
// ✅ ToJSON refuses these documents since 2026-09-13, as ErrNotJSON rather than
// ErrDuplicateKey: they are two keys, and it is JSON that cannot hold both. The
// parser records the pair the way it records a repeated key, under
// WithJSONCompatible.
//
// ✅ UseOrderedMap holds them apart since defect 69: a MapItem.Key is an
// interface{} and now carries what the key resolves to, as a map[any]any key
// does, so the integer 1 and the string "1" are two entries under two keys
// rather than two under one name.
//
// ✅ An `any` keeps both since the mapping built for one widens: it holds
// map[string]any while every key is a string and moves to map[any]any on the
// first key that is not, so the integer 1 and the string "1" are two entries
// under the values they resolve to. It kept one and reported nothing before,
// having lost a key inside the map before anything could complain.
//
// So one row is left, and it is not a defect: map[string]any refuses the pair,
// which is right, since a Go map keyed by a string cannot hold both.

// TestFixedATypedKeyKeepsItsOwnNamespace pins all five.
func TestFixedATypedKeyKeepsItsOwnNamespace(t *testing.T) {
	for name, tc := range map[string]struct {
		src string
		// merged is the one name the two keys land on wherever a destination
		// names a key by the canonical spelling of its type.
		merged string
		// resolved is what the plain key resolves to, which a MapSlice keeps
		// and a Go map cannot.
		resolved any
	}{
		"an integer and a string": {src: "1: x\n\"1\": y\n", merged: "1", resolved: uint64(1)},
		"a boolean and a string":  {src: "true: x\n\"true\": y\n", merged: "true", resolved: true},
		"a null and a string":     {src: "~: x\n\"null\": y\n", merged: "null", resolved: nil},
		"a float and a string":    {src: "1.0: x\n\"1.0\": y\n", merged: "1.0", resolved: float64(1)},
		// The one that shows what the rule is. "0x1f" and "31" share not one
		// character, and the walk merges them: a key is named by the canonical
		// spelling of what it resolved to, and 0x1f resolves to the integer 31.
		"a hexadecimal integer and the decimal string": {src: "0x1f: x\n\"31\": y\n", merged: "31", resolved: uint64(31)},
		// The empty key reaches it too, since it resolves to null and null's
		// canonical spelling is "null".
		"an empty key and the string \"null\"": {src: ": x\n\"null\": y\n", merged: "null", resolved: nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Run("a map[any]any keeps both, and is the only one that does", func(t *testing.T) {
				var got map[any]any
				require.NoError(t, codec.Unmarshal([]byte(tc.src), &got))
				assert.Len(t, got, 2)
				assert.Contains(t, got, tc.merged, "the quoted key stays a string")
			})

			t.Run("a typed string map refuses it as a duplicate", func(t *testing.T) {
				var got map[string]any
				err := codec.Unmarshal([]byte(tc.src), &got)
				require.Error(t, err)
				assert.Contains(t, err.Error(), `duplicate key "`+tc.merged+`"`)
			})

			t.Run("an `any` keeps both, under the values they resolve to", func(t *testing.T) {
				var got any
				require.NoError(t, codec.Unmarshal([]byte(tc.src), &got))
				assert.Equal(t, map[any]any{tc.resolved: "x", tc.merged: "y"}, got)
			})

			t.Run("an ordered map keeps both entries, under the keys they resolve to", func(t *testing.T) {
				var got any
				require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &got, codec.UseOrderedMap()))
				assert.Equal(t, mapSliceOf(
					item(tc.resolved, "x"),
					item(tc.merged, "y"),
				), got)
			})

			t.Run("ToJSON refuses it rather than writing the name twice", func(t *testing.T) {
				_, err := codec.ToJSON([]byte(tc.src))
				require.Error(t, err)
				assert.ErrorIs(t, err, yamlerrors.ErrNotJSON)
				assert.Contains(t, err.Error(), `two keys write the JSON member "`+tc.merged+`"`)
			})
		})
	}
}

// TestFixedAnAnchoredFloatKeyIsNamedByYAMLOnBothPaths: an anchor no longer
// changes the name a float key gets.
//
// A float whose YAML spelling differs from Go's formatting was named one way by
// the walk and another by the tree, and only behind an anchor: the walk called
// "&a .inf" "+Inf" where the tree called it ".inf", and "&a 1e3" "1000" against
// "1000.0". Naming 1e3 "1000" puts a float where the integer 1000 already is,
// which is the collision TestDefectATypedKeyIsNamedIntoTheStringsNamespace is
// about.
//
// parseAnchor attached the anchored node to ast.AnchorNode.Value after
// readAnchorValue returned, and readAnchorValue fires the walk's Leave on its
// way out -- so a walking reader was handed the anchor with Value still nil.
// unwrapKeyNode then unwrapped to nothing, keyName had no node to read a token
// from, and mapKeyString fell through to fmt.Sprint of the resolved float.
// readAnchorValue attaches the value before it returns now.
//
// Reached on 2026-09-07 by codec.TestWalkMatchesTheStream, once the corpus grew
// to 3,000 drawn documents. Closing it retired two hold-outs there: an
// input-keyed anchoredFloatKey, which was excusing 525 documents whatever
// happened to them, and the difference-keyed differOnlyByAFloatKeySpelling.
func TestFixedAnAnchoredFloatKeyIsNamedByYAMLOnBothPaths(t *testing.T) {
	t.Run("an anchor leaves the name alone", func(t *testing.T) {
		for _, tc := range []struct {
			src, name string
			resolved  float64
		}{
			{"&a .inf: c\n", ".inf", math.Inf(1)},
			{"&a 1e3: c\n", "1000.0", 1000},
			{"&a 1.0: c\n", "1.0", 1},
		} {
			// A string-keyed destination names the key, and the name is the
			// one being pinned here: the anchor does not change it.
			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &typed), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.name: "c"}, typed, "the tree: %q", tc.src)

			// An `any` keeps what the key resolves to, so the mapping widens.
			var walked any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &walked), "%q", tc.src)
			assert.Equalf(t, map[any]any{tc.resolved: "c"}, walked, "the walk: %q", tc.src)
		}
	})

	t.Run("and a NaN key is held but cannot be looked up", func(t *testing.T) {
		// NaN is not equal to itself, so a Go map keyed by one never gives the
		// entry back. The value is there and a range reaches it.
		// go.yaml.in/yaml/v3 v3.0.5 builds the same map for the same document.
		var typed map[string]any
		require.NoError(t, codec.Unmarshal([]byte("&a .nan: c\n"), &typed))
		assert.Equal(t, map[string]any{".nan": "c"}, typed)

		var walked any
		require.NoError(t, codec.Unmarshal([]byte("&a .nan: c\n"), &walked))
		widened, ok := walked.(map[any]any)
		require.True(t, ok)
		require.Len(t, widened, 1)
		for key, value := range widened {
			assert.True(t, math.IsNaN(key.(float64)))
			assert.Equal(t, "c", value)
		}
	})

	// ⚠️ A tag is a different fault and is still open: "!!float 1.0: c" is
	// named "1" on both paths, and "!!float 1: a" over "1: b" reads
	// {"1": "b"} -- two keys the parser tells apart, since it refuses
	// "!!float 1: a" over "1.0: b" as one key, collapsed into one Go map entry
	// with nothing reported. unwrapKeyNode does not unwrap an ast.TagNode, and
	// naming a tagged key wants the tag resolved rather than stripped.

	t.Run("a string-keyed destination names every one of them the same way", func(t *testing.T) {
		for _, tc := range []struct{ src, name string }{
			{".inf: c\n", ".inf"},
			{".nan: c\n", ".nan"},
			{"&a 1: c\n", "1"},
			{"&a true: c\n", "true"},
			{"&a x: c\n", "x"},
		} {
			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &typed), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.name: "c"}, typed, "%q", tc.src)
		}
	})

	t.Run("and a string key leaves the mapping keyed by string", func(t *testing.T) {
		var walked any
		require.NoError(t, codec.Unmarshal([]byte("&a x: c\n"), &walked))
		assert.Equal(t, map[string]any{"x": "c"}, walked)
	})
}

// ToJSON names a float key written the long way by its source text.
//
// "? 1e3" over ": x" converts to {"1e3":"x"}; the same mapping written
// "1e3: x" converts to {"1000.0":"x"}. So two spellings of one document give
// two JSON documents, and which one you get depends on where the "?" is.
//
// §3.2.1.1 makes a key equal to another when they resolve to the same node, so
// the name belongs to the value and not to the text. The decoder names both
// "1000.0". libfyaml 1.0.0b1 writes {"1000.0": "x"} for both and
// go.yaml.in/yaml/v3 v3.0.5 reads both as map[1000:x] -- the two spellings get
// one name from every implementation asked, this library's decoder included.
//
// Floats only. "? 007", "? 0x1f", "? ~" and "? true" are named 7, 31, null and
// true by both converters, so the resolution reaches the key -- only the
// canonical spelling of a float does not.
//
// Found on 2026-09-13, by Style.ExplicitKeys crossing a float once the
// yamlcorpus/25 draw put the two together.

// TestToJSONNamesAFloatKeyTheSameBothWays: "? 1e3" and "1e3: x" convert to one
// name.
//
// A key takes the canonical spelling of its type -- a float carries its "." or
// its exponent, which is what keeps 1 and 1.0 apart -- and a value keeps the
// digits the document wrote where JSON spells the number the same way. The
// scalar under a "?" arrives with at.Key false, since the "?" is the key and
// the scalar is what it stands around, so it was written as a value and
// closeKey quoted the result: "? 1e3" came out "1e3" against "1000.0".
//
// Only a float showed it. An integer, a null and a boolean have no source-text
// path to take, and 1.5 spells itself the same either way.
func TestToJSONNamesAFloatKeyTheSameBothWays(t *testing.T) {
	t.Run("the float, in every spelling of the key", func(t *testing.T) {
		for _, src := range []string{
			"? 1e3\n: x\n", "{? 1e3\n: x}\n", "1e3: x\n", "? &a 1e3\n: x\n",
		} {
			out, err := codec.ToJSON([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			assert.Equal(t, `{"1000.0":"x"}`, string(out), "%q", src)
		}
	})

	t.Run("a float JSON spells the same way is unchanged", func(t *testing.T) {
		// The canonical name and the source text agree here, so neither path
		// moved. The long digits are the case the value path exists for: read
		// into a float64 and written back they would lose their tail, and a key
		// keeps them because token.KeyName does.
		for _, tc := range []struct{ src, folds string }{
			{src: "1.5: x\n", folds: `{"1.5":"x"}`},
			{src: "? 1.5\n: x\n", folds: `{"1.5":"x"}`},
			{
				src:   "? 0.000000000000000000000000000000000022811873366677198\n: x\n",
				folds: `{"2.2811873366677198e-35":"x"}`,
			},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.folds, string(out), "%q", tc.src)
		}
	})

	t.Run("the decoder agrees with both spellings", func(t *testing.T) {
		for _, src := range []string{"1e3: x\n", "? 1e3\n: x\n"} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"1000.0": "x"}, got, "%q", src)
		}
	})

	t.Run("and every other scalar kind is named the same both ways", func(t *testing.T) {
		for _, tc := range []struct{ long, short, folds string }{
			{long: "? 007\n: x\n", short: "007: x\n", folds: `{"7":"x"}`},
			{long: "? 0x1f\n: x\n", short: "0x1f: x\n", folds: `{"31":"x"}`},
			{long: "? ~\n: x\n", short: "~: x\n", folds: `{"null":"x"}`},
			{long: "? true\n: x\n", short: "true: x\n", folds: `{"true":"x"}`},
		} {
			for _, src := range []string{tc.long, tc.short} {
				out, err := codec.ToJSON([]byte(src))
				require.NoErrorf(t, err, "%q", src)
				assert.Equalf(t, tc.folds, string(out), "%q", src)
			}
		}
	})
}

// A "!!timestamp" is written as the instant it names, by both converters.
//
// ToJSON wrote the scalar's source text until 2026-09-12, so
// "2001-12-14t21:59:43.1Z" went out with its lowercase "t" and
// "2001-12-14 21:59:43.1" with its space -- neither RFC 3339, and two documents
// denoting one instant converting to two JSON documents. The value converter
// goes through the decoder, which resolves the tag to a time.Time, and wrote
// "2001-12-14T21:59:43.1Z" for all of them.
//
// The field is split on which answer is right: libfyaml 1.0.0b1 writes the
// source text and go.yaml.in/yaml/v3 v3.0.5 writes the normalized instant. What
// settled it was inside this library rather than outside -- tagZeroJSON already
// wrote RFC 3339 for a "!!timestamp" standing on no value, so the text was the
// odd one out among ToJSON's own answers before it was a question about the
// field.
//
// "!!binary" has no such split: both converters write the decoded bytes as a
// JSON array of numbers. That differs from encoding/json, which writes a []byte
// as its base64 string, and from libfyaml, which writes "aGVsbG8=" -- recorded
// rather than pinned, since the two paths in this library agree.
//
// Found on 2026-09-13, once yamlgen drew a Timestamp and the corpus carried one
// into TestToJSONMatchesTheValueConverter.

// TestToJSONWritesATimestampAsTheInstantItNames holds the two converters to one
// answer.
func TestToJSONWritesATimestampAsTheInstantItNames(t *testing.T) {
	t.Run("every spelling converts to the instant", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "a: !!timestamp 2001-12-14\n", writes: `{"a":"2001-12-14T00:00:00Z"}`},
			{src: "a: !!timestamp 2001-12-14t21:59:43.1Z\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
			{src: "a: !!timestamp 2001-12-14T21:59:43.1Z\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
			{src: "a: !!timestamp 2001-12-14 21:59:43.1\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)

			var v any
			require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)
			through, err := codec.MarshalWithOptions(v, codec.JSON())
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, strings.ReplaceAll(tc.writes, `":"`, `": "`)+"\n", string(through),
				"%q: the value converter agrees", tc.src)
		}
	})

	t.Run("a binary value is the base64 text on both paths", func(t *testing.T) {
		const src = "a: !!binary aGVsbG8=\n"

		// JSON has no binary type and a JSON string is UTF-8, so the encoded
		// text travels -- the same thing encoding/json writes for a []byte.
		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(out))

		// And the decoder reads a codec.Base64, which holds that same text, so
		// writing the value as JSON writes it too. The two agree, which they
		// did not while the decoder read the bytes.
		var v any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()))
		through, err := codec.MarshalWithOptions(v, codec.JSON())
		require.NoError(t, err)
		assert.Equal(t, `{"a": "aGVsbG8="}`+"\n", string(through))
	})

	t.Run("a binary written over lines is canonical in JSON", func(t *testing.T) {
		// RFC 2045 lets the encoded stream carry line breaks; JSON takes the
		// canonical spelling, which is the same characters without them.
		out, err := codec.ToJSON([]byte("a: !!binary |\n  aGVs\n  bG8=\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(out))

		empty, err := codec.ToJSON([]byte("a: !!binary\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":""}`, string(empty))
	})

	t.Run("as a key a timestamp and a binary both agree now, which closes defect 30", func(t *testing.T) {
		// A MapItem.Key carries what the key resolves to, so a "!!timestamp"
		// key is a time.Time and the encoder writes RFC 3339 -- the instant
		// ToJSON already wrote. The two agree.
		//
		// "!!binary" agrees now too. ToJSON writes the base64 text, which is
		// what a JSON reader means by binary, and the decoder reads a
		// codec.Base64 holding that same text -- so the key is the same string
		// whichever path wrote it. It did not agree while the decoder resolved
		// the tag to []byte and ToJSON wrote the bytes as a JSON array.
		for _, tc := range []struct{ src, folds, values string }{
			{
				src:    "!!timestamp 2001-12-14: x\n",
				folds:  `{"2001-12-14T00:00:00Z":"x"}`,
				values: `{"2001-12-14T00:00:00Z": "x"}`,
			},
			{
				src:    "!!binary aGVsbG8=: x\n",
				folds:  `{"aGVsbG8=":"x"}`,
				values: `{"aGVsbG8=": "x"}`,
			},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.folds, string(out), "%q", tc.src)

			var v any
			require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)
			through, err := codec.MarshalWithOptions(v, codec.JSON())
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.values+"\n", string(through), "%q", tc.src)
		}
	})
}
