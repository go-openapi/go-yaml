// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"fmt"
	"iter"
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
		// A document ending without a line break still does, and one written with "\r\n" or "\r" gets its own
		// break around the entry: a "\n" there mixed two kinds of break in one document.
		{"no trailing line break", "a: 1", false, 1, "a: 1\nx: 9"},
		{"carriage returns", "a: 1\r\nb: 2\r\n", false, 2, "a: 1\r\nb: 2\r\nx: 9\r\n"},
		{"carriage returns, at the front", "a: 1\r\nb: 2\r\n", false, 0, "x: 9\r\na: 1\r\nb: 2\r\n"},
		// A lone CR ends the line the entry before stands on. Read as spacing, it
		// sent the search for the end of that line on through the next document,
		// and the entry went in after that document.
		{"lines broken by a lone CR", "a: 1 # c\r---\rb: 2\r", false, 1, "a: 1 # c\rx: 9\r---\rb: 2\r"},
		{"outside a nested mapping", "root:\n  a: 1\n", false, 1, "root:\n  a: 1\nx: 9\n"},
		// The lines of blanks after a block scalar are the scalar's: put in front of them, the entry took the
		// empty lines a "|+" keeps, and a line of spaces deeper than the indentation.
		{"after a keep scalar's empty lines", "k: |+\n a\n\n\n", false, 1, "k: |+\n a\n\n\nx: 9\n"},
		{"after a line of spaces under a block scalar", "foo: |\n  x\n   \n", false, 1, "foo: |\n  x\n   \nx: 9\n"},
		{"after a keep scalar, before a comment", "a: |+\n  x\n\n# c\n", false, 1, "a: |+\n  x\n\nx: 9\n# c\n"},

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

// insertableDocuments is how many corpus documents hold a mapping an entry can be inserted into. The count may not
// fall: fewer documents measured is the harness or the parser losing ground, and zero failures over them proves less.
const insertableDocuments = 3563

// TestInsertingIntoTheCorpus puts one entry into every document the corpus holds
// and asks for it back.
//
// Each document must still parse, hold the entry that was put in, be the source byte for byte once the entry is
// taken out again (see insertedCleanly), and decode to the source's value without it (see decodesAsTheSource).
// None may fail.
func TestInsertingIntoTheCorpus(t *testing.T) {
	t.Parallel()

	for _, where := range []string{"front", "middle", "back"} {
		t.Run("inserted at the "+where, func(t *testing.T) {
			t.Parallel()

			var tried int
			failed := map[insertionFault]int{}
			for _, src := range renderSources(t) {
				fault, measured := insertMarkerInto(t, src.text, where)
				if !measured {
					continue
				}
				tried++
				if fault != insertedFine {
					failed[fault]++
					if failed[fault] <= 3 {
						t.Logf("%s: %s", fault, src.name)
					}
				}
			}

			t.Logf("inserted at the %s: %d documents measured, %v", where, tried, failed)
			require.GreaterOrEqualf(t, tried, insertableDocuments,
				"%d documents reach the insertion where %d did: the harness or the parser lost ground", tried, insertableDocuments)
			require.Emptyf(t, failed, "insertions failed: %v", failed)
		})
	}
}

// insertionFault is the first check an insertion fails.
type insertionFault string

const (
	insertedFine        insertionFault = ""
	insertionUnreadable insertionFault = "no longer parses"
	insertionLost       insertionFault = "dropped the entry"
	insertionDisturbed  insertionFault = "changed a line the caller did not touch"
	insertionChanged    insertionFault = "decodes to another value"
)

// insertMarkerInto puts the marker entry into the first plain mapping of src, at where, and returns the first check the
// rendering fails. It returns false for a document with no mapping to insert into.
func insertMarkerInto(t *testing.T, src, where string) (insertionFault, bool) {
	t.Helper()

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	if err != nil || len(file.Docs) == 0 {
		return insertedFine, false
	}
	mapping := firstPlainMapping(file.Docs[0].Body, src)
	if mapping == nil {
		return insertedFine, false
	}

	at := 0
	switch where {
	case "middle":
		at = len(mapping.Values) / 2
	case "back":
		at = len(mapping.Values)
	}
	orderedEntry := isOrderedMapEntry(file.Docs[0].Body, mapping)
	insertMarkerAt(t, mapping, at)

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	rendered := out.String()

	switch {
	case parsesNot(rendered):
		return insertionUnreadable, true
	case !strings.Contains(rendered, corpusMarker):
		return insertionLost, true
	case !insertedCleanly(rendered, src):
		return insertionDisturbed, true
	case !orderedEntry && !decodesAsTheSource(rendered, src):
		return insertionChanged, true
	default:
		return insertedFine, true
	}
}

// parsesNot reports whether text fails to parse.
func parsesNot(text string) bool {
	_, err := parser.ParseBytes([]byte(text), parser.WithComments())

	return err != nil
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

// insertedCleanly reports whether rendered is src with the marker entry put in and nothing else changed.
//
// A block entry adds its line: the indentation, the entry and the break ending it, or the break in front of it where
// it is the document's last line and the document ends without one. In front of a mapping written after a "-", it
// takes the "-" from the entry it pushes down, which keeps the width. A flow entry adds itself and one comma. Put into
// a single pair of a flow sequence, "[a: 1]", it also adds the braces that make the pair the flow mapping it becomes,
// "[{a: 1, x: 9}]": a flow sequence entry holds one pair, and two need a mapping.
func insertedCleanly(rendered, src string) bool {
	left, cut := withoutMarker(rendered)
	if left == src {
		return true
	}

	return withoutBracesAround(left, cut) == src
}

// decodesAsTheSource reports whether rendered decodes to the value src decodes to, the marker entry taken out.
//
// A rendering can hold every line of the source and still mean something else: put between a "|+" scalar and the
// empty lines it keeps, the entry took them from it, and the lines were all still there. A source the decoder
// refuses has no value to compare, and passes.
func decodesAsTheSource(rendered, src string) bool {
	var want, got any
	if err := codec.Unmarshal([]byte(src), &want); err != nil {
		return true
	}
	if err := codec.Unmarshal([]byte(rendered), &got); err != nil {
		return false
	}

	// Printed with %#v, a map is written in key order and a NaN equals itself, which reflect.DeepEqual has not.
	return fmt.Sprintf("%#v", withoutMarkerKey(got)) == fmt.Sprintf("%#v", withoutMarkerKey(want))
}

// withoutMarkerKey returns v with the marker entry taken out of the mapping that holds it, wherever it is, and an
// ordered map written as the list of its pairs, so that two values compare by what they hold.
func withoutMarkerKey(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if k != corpusMarker {
				out[k] = withoutMarkerKey(e)
			}
		}

		return out
	case map[any]any:
		out := make(map[any]any, len(x))
		for k, e := range x {
			if k != corpusMarker {
				out[k] = withoutMarkerKey(e)
			}
		}

		return out
	case codec.MapSliceSeq:
		return pairsOf(x.All())
	case codec.MapSlice:
		return pairsOf(x.All())
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = withoutMarkerKey(e)
		}

		return out
	default:
		return v
	}
}

// pairsOf lists the pairs of an ordered map, the marker taken out of each value.
func pairsOf(all iter.Seq2[any, any]) []any {
	var out []any
	for k, e := range all {
		if k != corpusMarker {
			out = append(out, []any{k, withoutMarkerKey(e)})
		}
	}

	return out
}

// isOrderedMapEntry reports whether mapping is an entry of an "!!omap" or a "!!pairs" under doc. Each such entry
// holds one pair by the type's definition, so the marker put into it makes the collection another type: the value
// check does not apply.
func isOrderedMapEntry(doc ast.Node, mapping *ast.MappingNode) bool {
	var found bool
	ast.EachNode(doc, func(n ast.Node) {
		tag, isTag := n.(*ast.TagNode)
		if found || !isTag || (!strings.HasSuffix(tag.URI, ":omap") && !strings.HasSuffix(tag.URI, ":pairs")) {
			return
		}
		seq, isSeq := ast.Unwrap(tag.Value).(*ast.SequenceNode)
		if !isSeq {
			return
		}
		for _, entry := range seq.Values {
			if ast.Unwrap(entry) == ast.Node(mapping) {
				found = true
			}
		}
	})

	return found
}

// withoutBracesAround takes out the '{' that opens the flow mapping holding offset at, and the '}' closing it.
func withoutBracesAround(text string, at int) string {
	at = min(max(at, 0), len(text))
	open := -1
	for i, depth := at-1, 0; i >= 0; i-- {
		switch text[i] {
		case '}', ']':
			depth++
		case '[':
			depth--
		case '{':
			if depth == 0 {
				open = i
			}
			depth--
		}
		if open >= 0 || depth < 0 {
			break
		}
	}
	if open < 0 {
		return text
	}
	for i, depth := open+1, 0; i < len(text); i++ {
		switch text[i] {
		case '{', '[':
			depth++
		case ']':
			depth--
		case '}':
			if depth == 0 {
				return text[:open] + text[open+1:i] + text[i+1:]
			}
			depth--
		}
	}

	return text
}

// withoutMarker takes out the marker entry, its line or its comma, as insertedCleanly describes, and returns where
// the cut was.
func withoutMarker(rendered string) (string, int) {
	at := strings.Index(rendered, corpusMarker)
	if at < 0 {
		return rendered, 0
	}
	entry := corpusMarker + ": 1"
	for _, flow := range []string{entry + ", ", ", " + entry, entry} {
		i := strings.Index(rendered, flow)
		if i < 0 {
			continue
		}
		before := strings.TrimRight(rendered[:i], " \t-")
		if strings.HasSuffix(before, "{") || strings.HasSuffix(before, "[") || strings.HasSuffix(before, ",") ||
			strings.HasPrefix(rendered[i+len(flow):], "}") || strings.HasPrefix(rendered[i+len(flow):], "]") {
			return rendered[:i] + rendered[i+len(flow):], i
		}
	}

	start := strings.LastIndexAny(rendered[:at], "\r\n") + 1
	end := at + len(entry)
	switch {
	case strings.HasPrefix(rendered[end:], "\r\n"):
		end += 2
	case strings.HasPrefix(rendered[end:], "\n"), strings.HasPrefix(rendered[end:], "\r"):
		end++
	case end == len(rendered):
		// The last line, with no break after it: the break in front of it is the one it added.
		switch {
		case strings.HasSuffix(rendered[:start], "\r\n"):
			start -= 2
		case strings.HasSuffix(rendered[:start], "\n"), strings.HasSuffix(rendered[:start], "\r"):
			start--
		}
	}

	// Put in front of a mapping written after a "-", the entry takes the "-" and the entry it pushed down is
	// indented to the same width: "- a: 1" becomes "- x: 9" over "  a: 1". The "-" goes back to that line.
	lead := rendered[start:at]
	if rest := rendered[end:]; strings.TrimLeft(lead, " \t") != "" && strings.HasPrefix(rest, strings.Repeat(" ", len(lead))) {
		return rendered[:start] + lead + rest[len(lead):], start
	}

	return rendered[:start] + rendered[end:], start
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

// TestVerbatimInsertsInFrontOfAMappingAfterADash puts an entry in front of a mapping written after a "-". The new
// entry takes the "-", and the entry it pushes down keeps the width of the line. Written with the "-" again, it
// opened a second sequence entry: "- a: 1" became "- x: 9" over "- a: 1".
func TestVerbatimInsertsInFrontOfAMappingAfterADash(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		src  string
		at   int
		want string
	}{
		{"- a: 1\n  b: 2\n", 0, "- x: 9\n  a: 1\n  b: 2\n"},
		{" - a: 1\n", 0, " - x: 9\n   a: 1\n"},
		{"- a: 1\n  b: 2\n", 1, "- a: 1\n  x: 9\n  b: 2\n"},
		{"- a: 1\r\n- b: 2\r\n", 0, "- x: 9\r\n  a: 1\r\n- b: 2\r\n"},
	} {
		file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoError(t, err)
		insertInto(t, file.Docs[0].Body.(*ast.SequenceNode).Values[0], tc.at)

		var out bytes.Buffer
		require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
		require.Equalf(t, tc.want, out.String(), "%q at %d", tc.src, tc.at)
	}
}
