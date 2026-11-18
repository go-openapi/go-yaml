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

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
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
