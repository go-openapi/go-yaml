// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// insertMarkers puts a synthetic "x-mark: <n>" entry into every block mapping,
// at the front or at the end, and returns how many it put in.
//
// It is the shape a specification loader wants: a caller walks a parsed
// document and adds a key, and everything it did not touch has to come back the
// way it was written.
func insertMarkers(t *testing.T, n ast.Node, front bool, count *int) {
	t.Helper()

	switch v := n.(type) {
	case nil:
		return
	case *ast.DocumentNode:
		insertMarkers(t, v.Body, front, count)
	case *ast.MappingValueNode:
		insertMarkers(t, v.Value, front, count)
	case *ast.SequenceNode:
		for _, e := range v.Values {
			insertMarkers(t, e, front, count)
		}
	case *ast.MappingNode:
		for _, e := range v.Values {
			insertMarkers(t, e, front, count)
		}
		if v.IsFlowStyle {
			return
		}
		built, err := codec.ValueToNode(map[string]any{"x-mark": *count})
		require.NoError(t, err)
		mapping, ok := built.(*ast.MappingNode)
		require.True(t, ok)
		require.Len(t, mapping.Values, 1)
		*count++

		if front {
			v.Values = append([]*ast.MappingValueNode{mapping.Values[0]}, v.Values...)

			return
		}
		v.Values = append(v.Values, mapping.Values[0])
	}
}

// withoutMarkers drops the lines an inserted entry wrote, so that what is left
// can be held against the document that went in.
func withoutMarkers(text string) string {
	var kept []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "x-mark:") {
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}

// TestVerbatimKeepsWhatWasNotTouched renders a parsed document that a caller has
// inserted nodes into.
//
// Two things have to hold at once, and they are what the whole verbatim design
// is for: an inserted node is laid out, and every line the caller did not touch
// comes back byte for byte -- its own indentation, its comments, and the spacing
// the author chose. Renderer.Render cannot do the second; it lays the whole tree
// out by depth and normalises a four-space document to two.
func TestVerbatimKeepsWhatWasNotTouched(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		"a flat mapping":            "a: 1\nb: 2\n",
		"a comment and odd spacing": "# lead\ninfo:\n  title: Pet   # trailing\n  version: 1.2\n",
		"four-space indentation":    "root:\n    deep:\n        value: 1\n",
		"a one-space sequence":      "a:\n - 1\n - 2\nb: x\n",
		"a blank line between":      "a: 1\n\nb: 2\n",
	} {
		for _, front := range []bool{true, false} {
			where := "at the end"
			if front {
				where = "at the front"
			}

			t.Run(name+", inserted "+where, func(t *testing.T) {
				t.Parallel()

				file, err := parser.ParseBytes([]byte(src), parser.WithComments())
				require.NoError(t, err)

				var inserted int
				for _, doc := range file.Docs {
					insertMarkers(t, doc, front, &inserted)
				}
				require.Positive(t, inserted)

				var out bytes.Buffer
				require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))

				// It reads back, and it holds every key the caller added.
				var read map[string]any
				require.NoErrorf(t, codec.Unmarshal(out.Bytes(), &read), "rendered:\n%s", out.String())
				require.Contains(t, read, "x-mark")

				// And what the caller did not touch is what the document wrote.
				require.Equalf(t, strings.TrimRight(src, "\n"),
					strings.TrimRight(withoutMarkers(out.String()), "\n"),
					"the untouched lines changed\nrendered:\n%s", out.String())
			})
		}
	}
}

// TestRenderNormalisesWhereVerbatimDoesNot is the control: the same tree through
// Renderer.Render loses the document's own indentation, which is why Verbatim
// exists.
func TestRenderNormalisesWhereVerbatimDoesNot(t *testing.T) {
	t.Parallel()

	const src = "root:\n    deep:\n        value: 1\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	require.Equal(t, "root:\n  deep:\n    value: 1\n", file.String(),
		"Render lays out by depth, so four spaces come back as two")

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, src, out.String())
}

// TestVerbatimPlacesAnInsertedEntry is the insertion matrix: an entry put at the
// front of a collection, between two entries, and after the last one.
//
// Byte for byte, because the placement is the whole point -- the break in front
// of the entry, the indentation it opens with, the "-" a sequence element needs
// back, the ", " a flow collection separates with. A comparison that trimmed
// either end would have missed the blank line an append used to leave behind.
func TestVerbatimPlacesAnInsertedEntry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		src    string
		nested bool
		at     int
		want   string
	}{
		{"a mapping, at the front", "a: 1\nb: 2\n", false, 0, "x: 9\na: 1\nb: 2\n"},
		{"a mapping, in between", "a: 1\nb: 2\n", false, 1, "a: 1\nx: 9\nb: 2\n"},
		{"a mapping, after the last", "a: 1\nb: 2\n", false, 2, "a: 1\nb: 2\nx: 9\n"},

		{"a sequence, at the front", "- 1\n- 2\n", false, 0, "- 9\n- 1\n- 2\n"},
		{"a sequence, in between", "- 1\n- 2\n", false, 1, "- 1\n- 9\n- 2\n"},
		{"a sequence, after the last", "- 1\n- 2\n", false, 2, "- 1\n- 2\n- 9\n"},

		{"a nested mapping, at the front", "root:\n  a: 1\n  b: 2\n", true, 0, "root:\n  x: 9\n  a: 1\n  b: 2\n"},
		{"a nested mapping, in between", "root:\n  a: 1\n  b: 2\n", true, 1, "root:\n  a: 1\n  x: 9\n  b: 2\n"},
		{"a nested mapping, after the last", "root:\n  a: 1\n  b: 2\n", true, 2, "root:\n  a: 1\n  b: 2\n  x: 9\n"},
		{"a nested sequence, after the last", "root:\n  - 1\n  - 2\n", true, 2, "root:\n  - 1\n  - 2\n  - 9\n"},

		{"four-space indentation", "root:\n    a: 1\n", true, 1, "root:\n    a: 1\n    x: 9\n"},
		{"a comment beside the last entry", "a: 1\nb: 2  # note\n", false, 2, "a: 1\nb: 2  # note\nx: 9\n"},
		{"a blank line between entries", "a: 1\n\nb: 2\n", false, 2, "a: 1\n\nb: 2\nx: 9\n"},
		{"no trailing line break", "a: 1", false, 1, "a: 1\nx: 9\n"},
		{"carriage returns", "a: 1\r\nb: 2\r\n", false, 2, "a: 1\r\nb: 2\r\nx: 9\n"},
		// A lone CR ends the line the entry before stands on. Read as spacing, it
		// sent the search for the end of that line on through the next document,
		// and the entry went in after that document.
		{"lines broken by a lone CR", "a: 1 # c\r---\rb: 2\r", false, 1, "a: 1 # c\rx: 9\n---\rb: 2\r"},
		{"outside a nested mapping", "root:\n  a: 1\n", false, 1, "root:\n  a: 1\nx: 9\n"},

		{"a flow mapping, at the front", "{a: 1, b: 2}\n", false, 0, "{x: 9, a: 1, b: 2}\n"},
		{"a flow mapping, in between", "{a: 1, b: 2}\n", false, 1, "{a: 1, x: 9, b: 2}\n"},
		{"a flow mapping, after the last", "{a: 1, b: 2}\n", false, 2, "{a: 1, b: 2, x: 9}\n"},
		{"a flow sequence, after the last", "[1, 2]\n", false, 2, "[1, 2, 9]\n"},
		{"a flow mapping with nothing in it", "{}\n", false, 0, "{x: 9}\n"},
		{"a flow sequence with nothing in it", "[]\n", false, 0, "[9]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)

			body := file.Docs[0].Body
			if tc.nested {
				body = onlyValueOf(t, body)
			}
			insertInto(t, body, tc.at)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())
		})
	}
}

// TestVerbatimBracesAPairThatGainsAnEntry covers an entry inserted into a pair of a flow sequence, as in "[a: b]".
//
// The pair is a mapping of one entry written without braces. The entry went in on a block line inside the brackets,
// which no parser reads, and a pair standing under a block key had it go in below the key instead. A pair holding a
// second entry is the flow mapping it has become, so it is written in braces and the entry goes in as a flow entry.
//
// A flow entry inserted in front of a comment landed against the "#", where "1# note" reads as one scalar and the
// comment is lost; the last case holds that.
func TestVerbatimBracesAPairThatGainsAnEntry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		src     string
		element int
		at      int
		want    string
	}{
		{"the only pair, after its entry", "[a: b]\n", 0, 1, "[{a: b, x: 9}]\n"},
		{"the only pair, in front of its entry", "[a: b]\n", 0, 0, "[{x: 9, a: b}]\n"},
		{"a pair before another element", "[a: b, c]\n", 0, 1, "[{a: b, x: 9}, c]\n"},
		{"a pair after another element", "[c, a: b]\n", 1, 1, "[c, {a: b, x: 9}]\n"},
		{"a pair whose value is a collection", "[a: [1, 2], c]\n", 0, 1, "[{a: [1, 2], x: 9}, c]\n"},
		{"a pair on a line of its own", "[\n  a: b,\n  c\n]\n", 0, 1, "[\n  {a: b, x: 9},\n  c\n]\n"},
		{"a pair with a comment beside it", "[a: b # note\n, c]\n", 0, 1, "[{a: b, x: 9} # note\n, c]\n"},
		{"a pair under a block key", "k: [a: b]\n", -1, 1, "k: [{a: b, x: 9}]\n"},
		{"a flow mapping with a comment beside its last entry", "{a: 1 # note\n}\n", -2, 1, "{a: 1, x: 9 # note\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)

			body := file.Docs[0].Body
			switch tc.element {
			case -2: // the document is the mapping
			case -1: // the pair is the first element of the only key's flow sequence
				body = onlyValueOf(t, body).(*ast.SequenceNode).Values[0]
			default:
				body = body.(*ast.SequenceNode).Values[tc.element]
			}
			insertInto(t, body, tc.at)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())

			_, err = parser.ParseBytes(out.Bytes())
			require.NoError(t, err)
		})
	}
}

// insertInto puts an entry into a collection at index at.
func insertInto(t *testing.T, body ast.Node, at int) {
	t.Helper()

	switch c := body.(type) {
	case *ast.MappingNode:
		built, err := codec.ValueToNode(map[string]any{"x": 9})
		require.NoError(t, err)
		added := built.(*ast.MappingNode).Values[0]
		c.Values = append(c.Values[:at], append([]*ast.MappingValueNode{added}, c.Values[at:]...)...)
	case *ast.SequenceNode:
		added, err := codec.ValueToNode(9)
		require.NoError(t, err)
		c.Values = append(c.Values[:at], append([]ast.Node{added}, c.Values[at:]...)...)
	default:
		t.Fatalf("%T is not a collection", body)
	}
}

// onlyValueOf is the collection held by a mapping with one entry.
func onlyValueOf(t *testing.T, body ast.Node) ast.Node {
	t.Helper()

	mapping, isMapping := body.(*ast.MappingNode)
	require.True(t, isMapping)
	require.Len(t, mapping.Values, 1)

	return mapping.Values[0].Value
}

// placement records how an entry inserted at one position fared over the corpus:
// how many documents were measured, and how many came out wrong.
//
// The denominator is recorded with the failures because the two answer different
// questions, and a failure count on its own answers neither. tested follows the
// corpus -- it counts documents holding a block mapping whose entries open lines
// of their own, so a parser fix that accepts one more adds it here with the
// renderer standing still. unreadable and disturbed follow the placement. On
// 2026-09-10 a rebase took front and middle from 60 to 59 with nothing changed
// in writeEntry, and reading which of the two had moved needed the diff.
//
// With both on the page: unreadable up and tested flat is the placement; both up
// together is the parser; unreadable up and tested down is the harness.
type placement struct {
	tested     int
	unreadable int
	disturbed  int
}

// insertionCensus holds what each position measures. tested may not fall; the
// other two are held exactly while it stands still and capped while it grows,
// which is what mustHold does.
//
// What still fails are shapes the placement does not reach: a mapping standing
// as the key of an explicit "?" pair, one written compactly after a "-", and
// comments around a block scalar, where the line one entry ends on is not the
// line the next one begins.
//
// Appending was 86 unreadable and 1293 disturbed until writeEntry took the rest
// of the previous entry's line from the cursor instead of from its extent -- a
// token's extent runs to the end of its tile, which can be a line further on.
//
// 1747 to 1750 when the scanner stopped refusing a block scalar whose last line
// holds nothing but spaces. All three new documents end on such a line, so
// appending after it disturbs one, which took back from 187 to 190; front and
// middle did not move.
// Re-baselined on 2026-09-11: tested 1750 -> 1764, back 190 -> 191 disturbed,
// front and middle 57 unreadable throughout. Two things moved it together --
// be17078 lets three more render sources parse, and the "!!omap" corpus shapes
// arrived. Read the ratios: 10.86% -> 10.83% disturbed at the back and 3.26% ->
// 3.23% unreadable at the front, so both counts followed the corpus and the
// placement stood still.
//
// 1775 -> 1776 when a tab inside a plain scalar began to move the column, and
// the one document it adds is unreadable at the front and the middle and
// disturbed at the back. seed/17697 writes `Nul<TAB>l, 0841: ...` inside a
// flow sequence. The key 0841 now stands at its true column, which passes the
// column test in firstPlainMapping, and that test compares the column with
// everything in front of the key on its line, blanks or not. It admits a
// single pair inside a flow sequence, where an inserted entry has no line of
// its own to go on.
//
// 1776 -> 1775 when a "<<" after a plain scalar's text stayed in that text.
// seed/10138 writes ` i    <<:` under a sequence entry, and read only because
// the scanner cut "<<" out as a merge key and dropped the "i". It is refused
// now, as go.yaml.in/yaml/v3 refuses it, and it read at every placement, so
// the other counts stand.
//
// 1775 -> 1776 when a tab between a quoted key and its ':' stopped being read
// as indentation. seed/10532 writes `"\n"<TAB>: ` over a block scalar, was
// refused, and now reads as v3 reads it; an entry inserts cleanly at every
// placement, so the other counts stand.
//
// 1776 stays 1776 when the compact corpus was regenerated as yamlcorpus/46, with
// bigFloatMaxExp capped at 995, and the documents behind the count changed:
// every seed reshuffled. 58 -> 56 unreadable at the front and the middle and
// 192 -> 189 disturbed at the back followed the new documents, and the renderer
// did not move. An equal count does not mean the same documents.
//
// At the back, 13 -> 12 unreadable and 189 -> 190 disturbed is one document:
// seed/3300, written with lone "\r" breaks, whose entry went in after its second
// document until a "\r" began to end the line, and now lands in the first. It
// counts as disturbed with the 60 other documents written that way, since
// withoutMarkerLines splits on "\n" and cannot take the entry's line out of one.
//
// 1776 -> 1779 when a plain scalar inside a flow collection began to run onto a
// line left of the last ':', and a flow mapping entry stopped taking a second
// pair as its value. Three corpus documents holding such a continued value read
// now. Unreadable went 56 -> 63 at the front and the middle and 12 -> 59 at the
// back, and back disturbed 190 -> 135, for the reason seed/17697 gives above:
// the entry goes in as a block line inside a single pair of a flow sequence.
// The parser read that rendering as a nested pair and refuses it now, and
// yaml/yaml-reference-parser refuses each of the 51 renderings that moved.
//
// 1779 stays 1779 when an entry inserted into a pair of a flow sequence began to
// go in as a flow entry, the pair braced into the flow mapping it becomes, and
// an entry inserted before a comment in a flow collection stopped landing
// against the "#". Unreadable fell 63 -> 1 at the front and the middle and
// 59 -> 4 at the back. Disturbed rose 0 -> 64, 17 -> 81 and 135 -> 190, and
// that is the measure: withoutMarkerLines takes out the line the marker stands
// on, and a flow entry shares its line with the entries around it, so each
// document that reads now counts as disturbed -- every one of the 64 at the
// front, 79 of the 81 in the middle and 110 of the 190 at the back. What still
// fails is seed/10848, a mapping standing as an explicit "?" key, and at the
// back an entry after a "|+" scalar holding one empty line and after a
// double-quoted scalar over two lines. yaml/yaml-reference-parser reads both of
// those renderings, and the parser refuses them.
//
// At the back, 4 -> 2 unreadable is the parser and not the renderer: a keep
// scalar holding only empty lines, "keep: |+" over a blank line, was refused
// once a key followed it, and the two documents ending on one now read with the
// entry after them. Neither changed a line, so disturbed stands.
//
// 1779 -> 1781 when a tab inside a double-quoted scalar stopped moving the
// column twice. Two seeds holding one read now, and at the back 2 -> 0
// unreadable: the two documents ending on "bar" over " \t \t baz \t \t \"" took
// the entry after them as standing on the scalar's line. Nothing else moved.
//
// 1781 -> 1780 when a node property standing before an explicit key's "?" on
// the same line began to be refused. seed/10848 writes "!!null ?", which
// yaml/yaml-reference-parser and libfyaml refuse, and it was the document left
// unreadable at the front and the middle: the entry went in after the tag and
// the tag was copied in front of the line it pushed down. Nothing fails now.
//
// 1780 -> 1784 when a property in front of an implicit key stopped moving the
// column its value is measured from. Four documents that were refused now read,
// which is the parser and not the renderer, and they insert cleanly at every
// placement: unreadable and disturbed stand.
//
// 1784 -> 1812 on the regeneration to yamlcorpus/47, which draws a float
// written with more digits than a float64 keeps and reshuffled every seed
// behind it. The counts follow the corpus: 28 more documents reach the
// insertion, unreadable stands at 0, disturbed stands at 64 and 81 in front and
// in the middle, and at the back moves 190 -> 192, which is 10.65% -> 10.60% of
// the documents measured.
var insertionCensus = map[string]placement{
	"front":  {tested: 1812, unreadable: 0, disturbed: 64},
	"middle": {tested: 1812, unreadable: 0, disturbed: 81},
	"back":   {tested: 1812, unreadable: 0, disturbed: 192},
}

// TestInsertingIntoTheCorpus puts one entry into every document the corpus holds
// and asks for it back.
//
// Three things are counted: the rendered document still parses, it holds the
// entry that was put in, and every line the caller did not touch is unchanged.
// The third is the one the design exists for, and the one a trimmed comparison
// cannot see.
func TestInsertingIntoTheCorpus(t *testing.T) {
	t.Parallel()

	for _, where := range []string{"front", "middle", "back"} {
		recorded := insertionCensus[where]

		t.Run("inserted at the "+where, func(t *testing.T) {
			t.Parallel()

			var tried, unreadable, lost, disturbed int
			for _, src := range renderSources(t) {
				file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
				if err != nil || len(file.Docs) == 0 {
					continue
				}
				mapping := firstPlainMapping(file.Docs[0].Body, src.text)
				if mapping == nil {
					continue
				}

				at := 0
				switch where {
				case "middle":
					at = len(mapping.Values) / 2
				case "back":
					at = len(mapping.Values)
				}
				insertMarkerAt(t, mapping, at)
				tried++

				var out bytes.Buffer
				require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src.text))).VerbatimFile(&out, file))

				if _, err := parser.ParseBytes(out.Bytes(), parser.WithComments()); err != nil {
					unreadable++

					continue
				}
				if !strings.Contains(out.String(), corpusMarker) {
					lost++

					continue
				}
				if withoutMarkerLines(out.String()) != src.text {
					disturbed++
				}
			}

			require.Positive(t, tried)
			t.Logf("inserted at the %s: %d documents measured (recorded %d), %d no longer parse, %d lost the entry, %d changed a line the caller did not touch",
				where, tried, recorded.tested, unreadable, lost, disturbed)

			require.Zerof(t, lost, "%d documents dropped the entry that was put in", lost)
			require.GreaterOrEqualf(t, tried, recorded.tested,
				"%d documents reach the insertion where %d did: fewer documents are being measured, which is the harness or the parser and not the placement",
				tried, recorded.tested)
			mustHold(t, "the count of documents that no longer parse",
				unreadable, recorded.unreadable, tried, recorded.tested)
			mustHold(t, "the count of documents that changed a line the caller did not touch",
				disturbed, recorded.disturbed, tried, recorded.tested)
		})
	}
}

// mustHold checks a measured count against the recorded one: exactly where the
// same documents were measured, and as a ceiling where more were.
//
// The exact side is the ratchet. A ceiling alone passes when a count falls, so
// the fix that took appending from 86 unreadable to 13 would have left 86
// standing and over-stating the defect from then on. Where the denominator moved
// a fall cannot be read -- fewer failures over more documents says nothing by
// itself -- so a ceiling is the honest bound there rather than a compromise.
//
// Exact in both directions everywhere would go red on every parser fix that
// accepts one more document, and a guard that fails on good news gets its
// numbers bumped without being read.
func mustHold(t *testing.T, what string, got, recorded, tested, testedRecorded int) {
	t.Helper()

	if tested == testedRecorded {
		require.Equalf(t, recorded, got,
			"the same %d documents were measured and %s moved from %d to %d: nothing but the renderer can have done that, so record the new number",
			tested, what, recorded, got)

		return
	}

	require.LessOrEqualf(t, got, recorded,
		"%s is %d of %d where %d of %d was recorded: the corpus moved as well, so re-measure before reading this as a regression",
		what, got, tested, recorded, testedRecorded)
}

const corpusMarker = "x-mark-9"

// firstPlainMapping is the first block mapping of a document whose entries open
// lines of their own, which is where an inserted entry has somewhere to go.
func firstPlainMapping(n ast.Node, src string) *ast.MappingNode {
	var found *ast.MappingNode
	walkEveryNode(n, func(n ast.Node) {
		if found != nil {
			return
		}
		mapping, isMapping := n.(*ast.MappingNode)
		if !isMapping || mapping.IsFlowStyle || len(mapping.Values) == 0 {
			return
		}
		key := mapping.Values[0].Key.GetToken()
		if key == nil || int(key.Position.Column) != len(indentBefore(src, key.Position.Offset()))+1 {
			return
		}
		found = mapping
	})

	return found
}

// indentBefore is what stands between the last line break and at.
func indentBefore(src string, at int32) string {
	end := min(int(at), len(src))
	for i := end - 1; i >= 0; i-- {
		if src[i] == '\n' {
			return src[i+1 : end]
		}
	}

	return src[:end]
}

func insertMarkerAt(t *testing.T, mapping *ast.MappingNode, at int) {
	t.Helper()

	built, err := codec.ValueToNode(map[string]any{corpusMarker: 1})
	require.NoError(t, err)
	added := built.(*ast.MappingNode).Values[0]
	mapping.Values = append(mapping.Values[:at], append([]*ast.MappingValueNode{added}, mapping.Values[at:]...)...)
}

// withoutMarkerLines drops every line the marker was written on.
func withoutMarkerLines(rendered string) string {
	lines := strings.Split(rendered, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.Contains(line, corpusMarker) {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}

// TestVerbatimWritesAnInsertedCollectionInFlow puts a collection into a flow
// collection, where the block layout it asks for would end the collection at
// its first line break.
//
// The node is written in flow style however its own IsFlowStyle fields are set,
// and the caller's tree is left as it was: a renderer copy carries the decision,
// not a walk marking nodes on the way past.
func TestVerbatimWritesAnInsertedCollectionInFlow(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		src   string
		value any
		want  string
	}{
		{"a mapping", "{a: 1}\n", map[string]any{"x": map[string]any{"p": 1, "q": 2}}, "{a: 1, x: {p: 1, q: 2}}\n"},
		{"a sequence", "{a: 1}\n", map[string]any{"x": []any{1, 2}}, "{a: 1, x: [1, 2]}\n"},
		{"nested two deep", "{a: 1}\n", map[string]any{"x": map[string]any{"p": map[string]any{"r": []any{1, 2}}}}, "{a: 1, x: {p: {r: [1, 2]}}}\n"},
		{"a scalar", "{a: 1}\n", map[string]any{"x": 9}, "{a: 1, x: 9}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			built, err := codec.ValueToNode(tc.value)
			require.NoError(t, err)

			added := built.(*ast.MappingNode).Values[0]
			mapping := file.Docs[0].Body.(*ast.MappingNode)
			mapping.Values = append(mapping.Values, added)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			require.Equal(t, tc.want, out.String())

			// The caller's node still says what it said: nothing marked it flow.
			if collection, isCollection := added.Value.(*ast.MappingNode); isCollection {
				require.False(t, collection.IsFlowStyle, "the inserted node was marked flow style")
			}

			// And the document means what the tree means.
			var read map[string]any
			require.NoError(t, codec.Unmarshal(out.Bytes(), &read))
			require.Contains(t, read, "x")
		})
	}
}

// TestVerbatimRefusesAMultilineNodeInAFlowCollection is the one shape flow
// rendering cannot reach: a scalar written over several lines keeps its block
// header wherever it is put, and the first break would close the collection.
//
// Refused rather than written, because the alternative is a document that no
// longer parses and an error that never came.
func TestVerbatimRefusesAMultilineNodeInAFlowCollection(t *testing.T) {
	t.Parallel()

	const src = "{a: 1}\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	built, err := codec.ValueToNode(map[string]any{"x": "one\ntwo"})
	require.NoError(t, err)

	mapping := file.Docs[0].Body.(*ast.MappingNode)
	mapping.Values = append(mapping.Values, built.(*ast.MappingNode).Values[0])

	var out bytes.Buffer
	err = ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file)
	require.ErrorIs(t, err, ast.ErrInsert)
	require.ErrorContains(t, err, "spans lines")
}
