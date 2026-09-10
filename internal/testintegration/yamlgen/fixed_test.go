// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// Shapes the generator found that no longer diverge.
//
// A fixed defect leaves this much behind: the document that showed it, now
// asserting the behavior that is correct. It is worth keeping separately from
// the fix's own tests, because the generator reached these through a path
// nobody chose, and the same path will be walked again.

// TestFixedSingleQuotedKeyKeepsItsEscaping: a mapping key read from a
// single-quoted scalar is written back with its quote still doubled.
//
// The doubling used to be dropped, so a key holding one quote came back
// holding none of its escaping and the rendered document no longer parsed -- a
// valid document rendering to an invalid one. The same string in a value
// position was unaffected, which is what placed it in how keys were written
// rather than in single-quoted scalars, and the fix's own tests cover the value
// position alone.
func TestFixedSingleQuotedKeyKeepsItsEscaping(t *testing.T) {
	file, err := parser.ParseBytes([]byte("'a''b': false\n"), parser.WithComments())
	require.NoError(t, err)

	rendered := file.String()
	assert.Equal(t, "'a''b': false\n", rendered)

	reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
	require.NoError(t, err, "the rendered document must still parse")
	assert.Equal(t, rendered, reread.String(), "and rendering settles")
}

// TestFixedKeyThatIsOnlyAQuote is the smallest document of the same shape,
// which is what the reducer arrived at.
func TestFixedKeyThatIsOnlyAQuote(t *testing.T) {
	require.False(t, renderChangesValue([]byte("'''':\n")))
}

// TestFixedStripChompingKeepsTrailingSpaces: `|-` removes the trailing line
// break and nothing else.
//
// Chomping is defined over line breaks: b-chomped-last and l-chomped-empty say
// what happens to the break that ends the last line and to the empty lines
// after it. A space before that break is content -- l-nb-literal-text matches
// nb-char+, and nb-char includes a space -- so it survives every chomping mode.
// It used to be trimmed along with the break, and only under `|-`, so the same
// content read two ways gave two values.
func TestFixedStripChompingKeepsTrailingSpaces(t *testing.T) {
	values := map[string]any{
		"stripped":              "trailing ",
		"clipped":               "trailing \n",
		"not on the last line":  "a \nb",
		"a whole line of space": "a\n ",
	}
	sources := map[string]string{
		"stripped":              "|-\n  trailing \n",
		"clipped":               "|\n  trailing \n",
		"not on the last line":  "|-\n  a \n  b\n",
		"a whole line of space": "|-\n  a\n   \n",
	}

	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(src), &got))
			assert.Equal(t, values[name], got)
		})
	}

	// An empty line carries no content, so stripping takes it whole.
	t.Run("an empty trailing line is still chomped", func(t *testing.T) {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte("|-\n  a\n\n"), &got))
		assert.Equal(t, "a", got)
	})
}

// TestFixedBlockScalarsRenderTheirChomping: rendering writes back every line
// the chomping indicator settled on.
//
// A "|+" used to write the indicator without the blank lines it exists to
// preserve, so "a\n\n" came back as "a\n" -- the indicator was kept and its
// whole effect thrown away. Rendering took the content from the source text and
// trimmed the trailing whitespace off it, which is right for a "|" and wrong
// for the two indicators that exist to say what happens to that whitespace.
//
// The content now comes from the value, which is what chomping has already
// settled, so the indicator needs no arithmetic here at all.
func TestFixedBlockScalarsRenderTheirChomping(t *testing.T) {
	sources := map[string]string{
		"keep one blank line":    "k: |+\n  trail\n\n",
		"keep two":               "k: |+\n  trail\n\n\n",
		"keep with no content":   "k: |+\n\n\n",
		"clip":                   "k: |\n  trail\n",
		"strip":                  "k: |-\n  trail\n",
		"strip a trailing space": "k: |-\n  trail \n",
		"a stated indent":        "k: |2\n   x\n",
		"several lines":          "k: |\n  a\n  b\n",
		"a blank line inside":    "k: |\n  a\n\n  b\n",
		"in a sequence":          "- |+\n  keep\n\n- x\n",
	}

	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, src, file.String(), "the document is written back as it was read")

			var before, after any
			require.NoError(t, yaml.Unmarshal([]byte(src), &before))
			require.NoError(t, yaml.Unmarshal([]byte(file.String()), &after))
			assert.Equal(t, before, after, "and means the same")
		})
	}
}

// TestFixedCommentOnASequenceEntryStaysThere: a comment written on a sequence
// entry's own line is kept on that line.
//
// It is recorded on the entry rather than on its value, because an entry whose
// value is written below it -- or is not written at all -- has nothing on that
// line to carry it. Rendering read only the values, so such a comment was
// dropped; and the one on the first entry's dash was read a second time as the
// whole sequence's comment, which is how it also reappeared at the head of the
// document. Neither survived a second pass unchanged.
func TestFixedCommentOnASequenceEntryStaysThere(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"on an entry with no value":      {"-  # c\n", "-  # c\n"},
		"under a head comment":           {"# c1\n-  # c2\n", "# c1\n-  # c2\n"},
		"beside a sibling":               {"-  # c\n- y\n", "-  # c\n- y\n"},
		"on an entry holding a sequence": {"-  # c\n  - x\n", "- # c\n  - x\n"},
		"on an entry holding a mapping":  {"-  # c\n  a: 1\n", "- # c\n  a: 1\n"},
		"written below the dash":         {"-\n# c\n - x\n", "- # c\n  - x\n"},

		// A scalar on the entry's line carries its own comment, and a block
		// scalar's header is what shares the line -- a comment after it is
		// where YAML puts one.
		"on a scalar entry":      {"- x # c\n", "- x # c\n"},
		"after a block header":   {"- | # c\n  x\n", "- | # c\n  x\n"},
		"on a mapping entry key": {"k: # c\n  j: 1\n", "k: # c\n  j: 1\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)

			rendered := file.String()
			assert.Equal(t, test.want, rendered)

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err, "the rendered document must still parse")
			assert.Equal(t, rendered, reread.String(), "and rendering settles in one pass")
		})
	}
}

// TestFixedAnchoredEmptyEntryEndsWhereItsLineDoes: an entry whose line ends on
// an anchor that names nothing keeps the entries after it as siblings.
//
// Whatever such an anchor names has to be written inside the entry, further in
// than its '-' or its key. The parser looked only for the next entry of the
// same collection at the same column, so anything else -- a comment line, or
// the key of an enclosing mapping written level with the entry because a block
// sequence sits at its key's own column -- was taken as the anchor's value, and
// every later entry went down into it. Two siblings came back as one nested in
// the other: a document that still parses, still settles, and means something
// else.
func TestFixedAnchoredEmptyEntryEndsWhereItsLineDoes(t *testing.T) {
	values := map[string]any{
		"a comment then a sibling":            []any{nil, "x"},
		"an indented comment then a sibling":  []any{nil, "x"},
		"a mapping entry does the same":       map[string]any{"k": nil, "j": "x"},
		"an outer key level with the entry":   map[string]any{"": []any{nil}, " ": nil},
		"no comment":                          []any{nil, "x"},
		"nothing follows":                     []any{"x", nil},
		"the anchor names an indented value":  []any{[]any{"x"}},
		"the anchor names a mapping":          map[string]any{"k": map[string]any{"a": uint64(1)}, "j": "x"},
		"the anchor names an indented nested": []any{nil, "x"},
	}
	sources := map[string]string{
		"a comment then a sibling":            "- &a1\n#\n- x\n",
		"an indented comment then a sibling":  "- &a1\n  #\n- x\n",
		"a mapping entry does the same":       "k: &a1\n#\nj: x\n",
		"an outer key level with the entry":   "\"\": &a2\n - &a1\n\" \":\n",
		"no comment":                          "- &a1\n- x\n",
		"nothing follows":                     "- x\n- &a1\n#\n",
		"the anchor names an indented value":  "- &a1\n  - x\n",
		"the anchor names a mapping":          "k: &a1\n  a: 1\n#\nj: x\n",
		"the anchor names an indented nested": "- &a1\n  # c\n- x\n",
	}

	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			var before any
			require.NoError(t, yaml.Unmarshal([]byte(src), &before))
			assert.Equal(t, values[name], before, "reading is correct")

			file, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoError(t, err)
			rendered := file.String()

			var after any
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &after))
			assert.Equal(t, before, after, "and rendering keeps it")

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, rendered, reread.String(), "and settles in one pass")
		})
	}
}

// TestFixedBlankLineUnderAStatedIndent: a block scalar may state its
// indentation and keep its trailing blank lines at the same time.
//
// A blank line carries no indentation of its own, and YAML allows that:
// l-empty admits s-indent(<n), so an empty line may be indented less than the
// header states. Holding the last line to the stated width refused every such
// document -- and only when both features were present, since the padded and
// mid-content spellings were always accepted.
func TestFixedBlankLineUnderAStatedIndent(t *testing.T) {
	values := map[string]any{
		"a kept blank line":          map[string]any{"k": "one\n\n"},
		"two kept blank lines":       map[string]any{"k": "one\n\n\n"},
		"the blank line padded out":  map[string]any{"k": "one\n\n"},
		"a blank line in the middle": map[string]any{"k": "one\n\ntwo\n"},
		"without the indicator":      map[string]any{"k": "one\n\n"},
	}
	sources := map[string]string{
		"a kept blank line":          "k: |2+\n  one\n\n",
		"two kept blank lines":       "k: |2+\n  one\n\n\n",
		"the blank line padded out":  "k: |2+\n  one\n  \n",
		"a blank line in the middle": "k: |2\n  one\n\n  two\n",
		"without the indicator":      "k: |+\n  one\n\n",
	}

	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(src), &got))
			assert.Equal(t, values[name], got)
		})
	}

	// Content that really is indented less than the header states is still
	// refused: it is the empty line that is exempt, not the rule.
	for name, src := range map[string]string{
		"content short of the stated width": "k: |2\n x\n",
		"content short by more":             "k: |4\n  x\n",
	} {
		t.Run(name, func(t *testing.T) {
			var v any
			assert.Error(t, yaml.Unmarshal([]byte(src), &v))
		})
	}
}

// TestFixedStatedIndentFollowsTheContent: a block scalar that states its
// indentation has the header rewritten when the content moves.
//
// The width is counted from the indentation of whatever encloses the scalar,
// and the renderer writes the content at its own width, so carrying the
// source's number over left the header describing a layout that was no longer
// there -- the value gained a column on every cycle. A document encloses
// nothing, and the spec gives its node an indentation of -1, so at the root the
// same content is stated one higher than anywhere else.
func TestFixedStatedIndentFollowsTheContent(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"at the document root":         {"|2\n a\n", "|3\n  a\n"},
		"already at the root width":    {"|3\n  a\n", "|3\n  a\n"},
		"behind an anchor at the root": {"&a |2\n x\n", "&a |3\n  x\n"},
		"under a mapping key":          {"k: |2\n   x\n", "k: |2\n   x\n"},
		"under a sequence entry":       {"- |2\n   x\n", "- |2\n   x\n"},
		"narrower than the renderer":   {"k: |1\n  x\n", "k: |2\n   x\n"},

		// A property stays on the line the header ends, so what it names is
		// still indented from the start of that line and not from the property.
		"behind an anchor in a sequence": {"- &a |\n  x\n", "- &a |\n  x\n"},
		"behind a tag in a sequence":     {"- !!str |\n  x\n", "- !!str |\n  x\n"},
		"behind an anchor under a key":   {"k: &a |\n  x\n", "k: &a |\n  x\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var before any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &before))

			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			rendered := file.String()
			assert.Equal(t, test.want, rendered)

			var after any
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &after))
			assert.Equal(t, before, after, "the value survives the move")

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, rendered, reread.String(), "and settles in one pass")
		})
	}
}

// TestFixedKeepChompingKeepsItsBlankLinesWhenFolded: `>+` is written back with
// the trailing blank lines it exists to preserve.
//
// A folded scalar's value has lost its line structure, so the renderer writes
// its content back from the source text -- and the source ends the same way
// whatever the header says, since `>`, `>-` and `>+` differ only in what they
// make of the blank lines after the content. Reading the tail back from the
// source would have kept them under all three, so it was cut under all three,
// and keep chomping lost the only thing that distinguishes it. What the value
// ends on is where that decision has already been made, and the tail is
// rebuilt from there.
func TestFixedKeepChompingKeepsItsBlankLinesWhenFolded(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"keep":                     {"k: >+\n  trail\n\n", "k: >+\n  trail\n\n"},
		"keep, several":            {"k: >+\n  trail\n\n\n", "k: >+\n  trail\n\n\n"},
		"keep, nothing but blanks": {"k: >+\n\n\n", "k: >+\n\n\n"},
		"keep, folded over a gap":  {"k: >+\n  a\n\n  b\n\n", "k: >+\n  a\n\n  b\n\n"},
		"keep, stated width":       {"k: >2+\n   trail\n\n", "k: >2+\n   trail\n\n"},
		"keep, under a sequence":   {"- >+\n  trail\n\n", "- >+\n  trail\n\n"},
		"keep, at the root":        {">+\n  trail\n\n", ">+\n  trail\n\n"},

		// The other two chomping modes drop the blank lines when reading, so
		// the tail they are written with is empty -- which is what the source
		// text alone could not tell them apart by.
		"clip":  {"k: >\n  trail\n\n", "k: >\n  trail\n"},
		"strip": {"k: >-\n  trail\n\n", "k: >-\n  trail\n"},

		// A line of spaces is blank up to the width that introduced the block
		// and content past it: a folded scalar treats a line indented further
		// than its neighbors literally, so those spaces are the value.
		"a blank line of the block's own width": {"k: >+\n  a\n  \n", "k: >+\n  a\n\n"},
		"a blank line indented further":         {"k: >+\n  a\n     \n", "k: >+\n  a\n     \n"},
		"and one under strip chomping":          {"k: >-\n  a\n     \n", "k: >-\n  a\n     \n"},

		// Trailing spaces on a line of content are content under every mode,
		// the same way they are for the literal spelling.
		"a space before the break": {"k: >+\n  trail \n\n", "k: >+\n  trail \n\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var before any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &before))

			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			rendered := file.String()
			assert.Equal(t, test.want, rendered)

			var after any
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &after))
			assert.Equal(t, before, after, "the value survives being written out")

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, rendered, reread.String(), "and settles in one pass")
		})
	}
}

// TestFixedFoldedScalarWritesItsOwnBreak: a folded block scalar is written with
// "\n", whatever line break the document it came from used.
//
// The renderer used to copy the source's break into the scalar it wrote. A CRLF
// document then rendered to a mixture of both and was a third document on the
// next render; with a lone CR the content lines drifted a column right, so a
// fold stopped folding and "x y" came back as "x\n y"; and nested, the content
// landed at the parent's own column, which the grammar refuses outright.
//
// The break was doing two jobs: reading the origin, where it has to be the
// source's, and writing the output, where it has to be "\n". Splitting them
// left one thing to get right in the order -- the content is normalized before
// dedentBy takes it apart, that function looking for "\n".
func TestFixedFoldedScalarWritesItsOwnBreak(t *testing.T) {
	for _, test := range []struct {
		name, src, want string
		value           any
	}{
		{"CRLF", ">-\r\n x\r\n", ">-\n  x\n", "x"},
		{"lone CR, two lines", ">-\r x\r y\r", ">-\n  x\n  y\n", "x y"},
		{"LF, unchanged", ">-\n x\n y\n", ">-\n  x\n  y\n", "x y"},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.src), parser.WithComments())
			require.NoError(t, err)

			once := file.String()
			assert.Equal(t, test.want, once, "the renderer writes its own break")

			second, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, once, second.String(), "and the document settles")

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(once), &got))
			assert.Equal(t, test.value, got, "the fold still folds")
		})
	}
}

// TestFixedFoldedScalarNestedRendersYAML: a folded scalar under a mapping key
// renders content indented past the key, not level with it.
func TestFixedFoldedScalarNestedRendersYAML(t *testing.T) {
	const src = "a:\r  b: >\r   x\rc: 1\r"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	assert.Equal(t, "a:\n  b: >\n    x\nc: 1\n", file.String())

	var got any
	require.NoError(t, yaml.Unmarshal([]byte(file.String()), &got))
	// ">" clips rather than strips, so the value keeps one trailing break.
	assert.Equal(t, map[string]any{"a": map[string]any{"b": "x\n"}, "c": uint64(1)}, got)
}

// TestFixedCRLFComentDoesNotBlankALine: a comment closing a CRLF line leaves the
// token after it on the next line, not two down.
//
// scanComment stopped at the '\r' and left the '\n' to whatever came next,
// whose leading whitespace was then read as a second break. The document gained
// a blank line next to the comment when it was written back, and lost it again
// on the render after that, so it never settled.
//
// Any comment did it, at the end of a line as much as on one of its own. The
// ledger entry that recorded it asked for a standalone comment and so missed
// `- #\r\n -\r\n`, which failed TestRenderReachesAFixedPoint at a seed the
// earlier runs had not drawn; `- 1 # c1\r\n- 2\r\n` gained its blank line with
// no open entry anywhere. Both are below, because a fix is worth no more than
// the shapes it is held to.
func TestFixedCRLFComentDoesNotBlankALine(t *testing.T) {
	for _, test := range []struct{ name, src string }{
		{"CRLF", "# c1\r\n-\r\n# c2\r\n- 1\r\n"},
		{"lone CR", "# c1\r-\r# c2\r- 1\r"},
		{"LF", "# c1\n-\n# c2\n- 1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.src), parser.WithComments())
			require.NoError(t, err)

			once := file.String()
			assert.Equal(t, "# c1\n- \n# c2\n- 1\n", once,
				"every line break writes the same document")

			again, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, once, again.String(), "and it settles in one pass")
		})
	}

	// A comment at the end of a line, which the entry above did not ask for.
	for _, test := range []struct{ name, src, want string }{
		{"a line comment over an open entry", "- #\r\n -\r\n", "- #\n  - \n"},
		{"a line comment and no open entry", "- 1 # c1\r\n- 2\r\n", "- 1 # c1\n- 2\n"},
		{"a line comment introducing a block", "- # c1\r\n  - x\r\n", "- # c1\n  - x\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.src), parser.WithComments())
			require.NoError(t, err)

			once := file.String()
			assert.Equal(t, test.want, once, "no blank line either side of the comment")

			again, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, once, again.String(), "and it settles in one pass")
		})
	}
}

// TestFixedStrTagKeepsTheSpelling: `!!str` settles what a plain scalar is, so
// the spelling that went in is the one that comes back.
//
// The scalar used to be resolved first and the result turned into text, so
// `!!str null`, `!!str Null`, `!!str NULL` and `!!str ~` all read "" and
// `!!str True` and `!!str FALSE` read "true" and "false". The library
// disagreed with itself about it, which is what made it a defect rather than a
// reading of the spec: `!<tag:yaml.org,2002:str> null` is the same tag spelled
// verbatim and read "null", as did `! null`, `!foo null` and `!!str "null"`.
func TestFixedStrTagKeepsTheSpelling(t *testing.T) {
	for _, src := range []string{
		"!!str null\n", "!!str Null\n", "!!str NULL\n", "!!str ~\n",
		"!!str True\n", "!!str TRUE\n", "!!str False\n", "!!str FALSE\n",
		"!!str 5\n", "!!str on\n", "!!str true\n", "!!str x\n",
	} {
		wellFormed(t, src)

		var got any
		require.NoError(t, yaml.Unmarshal([]byte(src), &got))
		assert.Equal(t, strings.TrimSuffix(strings.TrimPrefix(src, "!!str "), "\n"), got, "%q", src)
	}

	t.Run("every spelling of the same tag gives the same text", func(t *testing.T) {
		for _, src := range []string{
			"!!str null\n", "!<tag:yaml.org,2002:str> null\n", "! null\n", "!foo null\n", "!!str \"null\"\n",
		} {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(src), &got))
			assert.Equal(t, "null", got, "%q", src)
		}
	})

	t.Run("and the tag on nothing is the empty string", func(t *testing.T) {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte("a: !!str\nb: 1\n"), &got))
		assert.Equal(t, map[string]any{"a": "", "b": uint64(1)}, got)
	})
}

// TestFixedACollectionTagBeforeAnAnchorParses covers a collection tag written
// in front of an anchor.
//
// YAML 1.2 lets a node's tag and anchor stand in either order and means the
// same by both. Written second the tag held; written first it stopped the parse
// outright -- "a: !!seq &a1 [1]" was refused with "value is not allowed in this
// context" and "a: !!map &a1 {b: 1}" with "could not find map", so a document
// carrying one could not be read, rendered or reformatted at all.
//
// Fixed on 2026-09-07 by parseTagValue reading whatever node follows a
// collection tag rather than insisting on the kind: what the tag made of the
// node is ast.TagNode.Resolve's to report, and the load refuses a kind the tag
// does not name. The tag before the anchor stopped being a parse question on
// the way.
//
// The other two shapes of that defect are still open, in
// TestDefectTagBeforeAnchorIsDropped: a tag on an empty node swallows what
// follows it, and any other tag is dropped from the node the anchor names.
func TestFixedACollectionTagBeforeAnAnchorParses(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "a: !!seq &a1 [1]\n", want: map[string]any{"a": []any{uint64(1)}}},
		{src: "a: !!map &a1 {b: 1}\n", want: map[string]any{"a": map[string]any{"b": uint64(1)}}},

		// The anchor written first, which always read, so the two orders agree.
		{src: "a: &a1 !!seq [1]\n", want: map[string]any{"a": []any{uint64(1)}}},
		{src: "a: &a1 !!map {b: 1}\n", want: map[string]any{"a": map[string]any{"b": uint64(1)}}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			wellFormed(t, tc.src)

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got)

			// And the anchor names the tagged node, so an alias to it is the
			// same value.
			aliased := strings.TrimSuffix(tc.src, "\n") + "\nb: *a1\n"

			var both map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(aliased), &both))
			assert.Equal(t, both["a"], both["b"], "%q", aliased)
		})
	}

	t.Run("and the document renders as it was written", func(t *testing.T) {
		for _, src := range []string{"a: !!seq &a1 [1]\n", "a: !!map &a1 {b: 1}\n"} {
			assert.Equal(t, src, renderOnce(t, src), "%q", src)
		}
	})
}

// TestFixedANullKeepsTheSpellingItWasWrittenWith covers a null rendering as the
// document spelled it.
//
// ast.NullNode.String wrote the four letters "null" whatever the token held, so
// "Null", "NULL" and "~" all came back as "null". Every other scalar node
// already rendered from its token -- "!!str True" keeps its capital T and
// "!!str .INF" its capitals -- and the null was the one that did not.
//
// Untagged it cost the spelling and not the value, since YAML resolves all four
// to the same node. Under "!!str" it cost the value too: the tag names the
// characters, and "Null" is not "null". Reading it was fixed first; writing it
// back is the half that was not.
func TestFixedANullKeepsTheSpellingItWasWrittenWith(t *testing.T) {
	for _, src := range []string{
		"k: !!str Null\n",
		"k: !!str NULL\n",
		"k: !!str null\n",
		"k: !!str ~\n",
		"!!str Null\n",
		"&a1 !!str Null\n",

		// Untagged, where only the spelling was at stake.
		"k: Null\n",
		"k: NULL\n",
		"k: ~\n",
		"k: null\n",
	} {
		t.Run(src, func(t *testing.T) {
			wellFormed(t, src)
			assert.Equal(t, src, renderOnce(t, src))
		})
	}

	t.Run("the value survives the write as well as the read", func(t *testing.T) {
		for _, src := range []string{"k: !!str Null\n", "k: !!str NULL\n", "k: !!str ~\n"} {
			want := strings.TrimSuffix(strings.TrimPrefix(src, "k: !!str "), "\n")

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, map[string]any{"k": want}, got, "the decode")

			var again any
			require.NoError(t, yaml.Unmarshal([]byte(renderOnce(t, src)), &again), "%q", src)
			assert.Equal(t, got, again, "the render, read back")
		}
	})

	t.Run("a null the document never wrote still has no text", func(t *testing.T) {
		// An implicit null is the absence of a value, so it renders as nothing.
		// Writing "null" for it would put a value where the document had none.
		for _, src := range []string{"k:\n", "k: !!str\n", "- \n"} {
			wellFormed(t, src)
			assert.NotContains(t, renderOnce(t, src), "null", "%q", src)
		}
	})
}

// TestFixedACommentAfterALineEndingTagSurvives covers a comment sitting after a
// tag that is the last thing on its line.
//
// The parse attached it to the tag node correctly; Renderer.tag rendered only
// n.Start.Value and the node it stands on, so the comment on the node itself
// went unread. "!!null # c1" and "- &a2 !!seq # c2" came back without theirs.
//
// An anchor never lost one, which is what said this was the tag and not
// comments on a property line in general: Renderer.anchor renders the name
// node, and the parse hangs the comment there. Nor did a tag with anything
// after it on the line -- "!!null null # c1" -- because the comment had been
// hung on the scalar instead.
func TestFixedACommentAfterALineEndingTagSurvives(t *testing.T) {
	for _, src := range []string{
		"!!null # c1\n",
		"&a1 !!null # c1\n",
		"k: !!null # c1\n",
		"- !!null # c1\n",
		"- &a2 !!seq # c2\n  - 1\n",
		"!!seq # c1\n- 1\n",
		"!!str # c1\n",

		// The shapes that always kept it, so the fix did not move them.
		"&a1 # c1\n",
		"!!null null # c1\n",
		"&a1 !!str x # c1\n",
		"k: !!str x # c1\n",
	} {
		t.Run(src, func(t *testing.T) {
			wellFormed(t, src)
			assert.Equal(t, src, renderOnce(t, src))
		})
	}

	t.Run("and it is still dropped when comments are off", func(t *testing.T) {
		// The renderer writes no comment without WithComments, tag or no tag.
		f, err := parser.ParseBytes([]byte("!!null # c1\n"))
		require.NoError(t, err)
		assert.NotContains(t, f.String(), "#")
	})
}

// TestFixedACommentAboveAPropertyLineStaysOnTheKey covers a comment on the line
// that introduces a node whose properties are written on the next one.
//
// A comment claims the rest of the line it sits on, so what follows the key
// cannot start there. Renderer.fitsOnKeyLine says yes to an anchor and a tag --
// they carry their own value and decide their own shape -- and the comment was
// then pushed past the whole value and written on the last line of it:
// "k: # c1" over "&a2" over "- 1" came back as "k: &a2" over "- 1 # c1".
//
// Harmless where the last line was a plain scalar and not harmless at all where
// it was a block scalar: the comment landed inside the content and the value
// changed with nothing reporting it.
func TestFixedACommentAboveAPropertyLineStaysOnTheKey(t *testing.T) {
	for _, src := range []string{
		"k: # c1\n  &a2\n  - 1\n",
		"k: # c1\n  !!seq\n  - 1\n",
		"k: # c1\n  &a2 |-\n    x\n",
		"k: # c1\n  &a2\n  - |-\n    trailing \n",

		// A sequence entry takes the same rule from Renderer.sequence, which
		// asked fitsOnKeyLine the same question and got the same wrong answer.
		// TestRenderPreservesValue found this one at 50,000 draws, after the
		// mapping half was fixed and the entry half was not.
		"- # c1\n  &a2\n  - |-\n    trailing \n",
		"- # c1\n  !!seq\n  - 1\n",

		// The shapes that always rendered correctly, so the fix did not move
		// them: a collection under a commented key already went below it, and a
		// block scalar keeps its header on the line because a comment after the
		// header is where YAML puts one.
		"k: # c1\n  a: 1\n",
		"k: # c1\n  [1, 2]\n",
		"k: |- # c1\n  x\n",
		"k: &a2 x # c1\n",
		"- |- # c1\n  x\n",
		"- 1 # c1\n",
	} {
		t.Run(src, func(t *testing.T) {
			wellFormed(t, src)
			assert.Equal(t, src, renderOnce(t, src))
		})
	}

	t.Run("the comment stays on its own line where the layout normalizes", func(t *testing.T) {
		// A block mapping under a property is indented from the property, with
		// or without a comment -- "!!map" over "a: 1" renders the same way. So
		// these do not come back byte for byte, and what this holds is the one
		// thing the defect moved: the comment is still on the line that
		// introduced the node, and not somewhere inside the value.
		for _, src := range []string{
			"- # c1\n  &a2\n  a: 1\n",
			"k: # c1\n  &a2\n  a: 1\n",
		} {
			wellFormed(t, src)

			got := renderOnce(t, src)
			assert.Equal(t, 1, strings.Count(got, "#"), "%q rendered %q", src, got)
			assert.Contains(t, strings.SplitN(got, "\n", 2)[0], "# c1", "%q rendered %q", src, got)
		}
	})

	t.Run("the block scalar keeps its value", func(t *testing.T) {
		const src = "k: # c1\n  &a2\n  - |-\n    trailing \n"

		var before, after any
		require.NoError(t, yaml.Unmarshal([]byte(src), &before))
		require.NoError(t, yaml.Unmarshal([]byte(renderOnce(t, src)), &after))

		assert.Equal(t, []any{"trailing "}, before.(map[string]any)["k"])
		assert.Equal(t, before, after, "the render changed the value")
	})
}

// TestFixedATagOnAnEmptyNodeKeepsWhatFollows covers a tag and an anchor written
// at the end of a line with nothing after them.
//
// The anchor went looking for a value and took the next entry of the collection
// around it. "- !!null &a1" over "- x" decoded to a one-item sequence -- the
// second entry gone, and no error at all -- and "k: !!null &a1" over "j: x"
// lost j the same way.
//
// Whatever a property at the end of a line names has to be written inside the
// entry holding it, which means further in than that entry's own column.
// parseMapValue and parseSequenceValue said exactly this for a bare anchor,
// which is why "- &a1" over "- x" never lost anything; a tag before the anchor
// sends the descent down parseTagValue, too far from either to repeat the test,
// so the parser now carries the column there.
func TestFixedATagOnAnEmptyNodeKeepsWhatFollows(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "- !!null &a1\n- x\n", want: []any{nil, "x"}},
		{src: "- !!str &a1\n- x\n", want: []any{"", "x"}},
		{src: "- !!seq &a1\n- x\n", want: []any{nil, "x"}},
		{src: "k: !!null &a1\nj: x\n", want: map[string]any{"k": nil, "j": "x"}},
		{src: "k: !!seq &a1\nj: x\n", want: map[string]any{"k": nil, "j": "x"}},

		// The anchor written first, which never lost anything.
		{src: "- &a1 !!null\n- x\n", want: []any{nil, "x"}},
		{src: "- &a1\n- x\n", want: []any{nil, "x"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			wellFormed(t, tc.src)

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("and a property still names what is written inside its entry", func(t *testing.T) {
		// The other side of the column rule: indented past the entry, what
		// follows belongs to the property and not to the collection around it.
		for _, tc := range []struct {
			src  string
			want any
		}{
			{src: "a: &a\n  foo: 1\nb: 2\n", want: map[string]any{"a": map[string]any{"foo": uint64(1)}, "b": uint64(2)}},
			{src: "a: !!map &m\n  foo: 1\nb: 2\n", want: map[string]any{"a": map[string]any{"foo": uint64(1)}, "b": uint64(2)}},

			// And at the document's root nothing encloses the property, so it
			// names what follows however far away it is written.
			{src: "!!str\n&a2\nscalar2\n", want: "scalar2"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestFixedAnAnchorAfterATagNamesTheTaggedNode covers the last shape of the
// tag-before-anchor defect.
//
// §6.9 lets a node's tag and anchor stand in either order and means the same by
// both. Written anchor first the tree is Anchor over Tag over the value, and the
// anchor names the tagged node; written tag first it is Tag over Anchor over the
// value, and the anchor named the value with the tag stripped off it. So
// "a: !!int &a1 \"5\"" read 5 at a and "5" at "b: *a1" -- one node, read as a
// number where it stands and as a string through an alias to it.
//
// The tree still keeps the order the document wrote, so it renders as it was
// written. Only what the name stands for changed.
func TestFixedAnAnchorAfterATagNamesTheTaggedNode(t *testing.T) {
	for _, tc := range []struct {
		tagFirst, anchorFirst string
		want                  any
	}{
		{
			tagFirst:    "a: !!int &a1 \"5\"\nb: *a1\n",
			anchorFirst: "a: &a1 !!int \"5\"\nb: *a1\n",
			want:        map[string]any{"a": 5, "b": 5},
		},
		{
			tagFirst:    "a: !!str &a1 5\nb: *a1\n",
			anchorFirst: "a: &a1 !!str 5\nb: *a1\n",
			want:        map[string]any{"a": "5", "b": "5"},
		},
		{
			tagFirst:    "a: !!float &a1 1\nb: *a1\n",
			anchorFirst: "a: &a1 !!float 1\nb: *a1\n",
			want:        map[string]any{"a": float64(1), "b": float64(1)},
		},
		{
			tagFirst:    "a: !!binary &a1 aGk=\nb: *a1\n",
			anchorFirst: "a: &a1 !!binary aGk=\nb: *a1\n",
			want:        map[string]any{"a": []byte("hi"), "b": []byte("hi")},
		},
	} {
		t.Run(tc.tagFirst, func(t *testing.T) {
			wellFormed(t, tc.tagFirst)
			wellFormed(t, tc.anchorFirst)

			var written, other any
			require.NoError(t, yaml.Unmarshal([]byte(tc.tagFirst), &written))
			require.NoError(t, yaml.Unmarshal([]byte(tc.anchorFirst), &other))

			assert.Equal(t, tc.want, written, "tag first")
			assert.Equal(t, tc.want, other, "anchor first")

			// And each order still renders as it was written.
			assert.Equal(t, tc.tagFirst, renderOnce(t, tc.tagFirst))
			assert.Equal(t, tc.anchorFirst, renderOnce(t, tc.anchorFirst))
		})
	}
}

// TestFixedFoldedScalarGainsNoBreakWhenRendered: a folded block scalar is
// written back with the blank lines the document gave it, and no more.
//
// token.Lookback.blankLineAbove asks linesSpannedBy how many lines a scalar
// occupies past its first, then subtracts that from the gap to the next token.
// linesSpannedBy counted the breaks in the scalar's value, which measures the
// source only for a literal block. Folding rewrites the line structure, so
// "  x\n\n  y\n" comes back as "x\ny\n", and the scalar measured one line short
// for every line its content folded away. The renderer read the leftover as a
// blank line the author had left, and wrote one.
//
// Under ">+" that blank line is content on the way back in, so the value gained
// a trailing break on every render and the document never settled. Clip and
// strip chomping discard it, which kept the same miscount out of sight.
//
// linesSpannedBy now measures the source through Token.EndLine and adds the
// blank lines chomping keeps.
func TestFixedFoldedScalarGainsNoBreakWhenRendered(t *testing.T) {
	for name, src := range map[string]string{
		// The shape the generator found, and the three the same miscount
		// reaches once the chomping indicator stops hiding it.
		"keep, folded over a gap":  "- >+\n  x\n\n  y\n- 1\n",
		"clip, folded over a gap":  "- >\n  x\n\n  y\n- 1\n",
		"strip, folded over a gap": "- >-\n  x\n\n  y\n- 1\n",
		"two lines folded to one":  "- >+\n  x\n  y\n\n- 1\n",

		// A literal block never showed it: its value keeps the source's lines.
		"literal, over a gap": "- |+\n  x\n\n  y\n- 1\n",

		// The miscount grew with the number of blank lines kept, so a scalar
		// ending on two of them gained two.
		"keep, two trailing blanks": "- >+\n  x\n\n  y\n\n\n- 1\n",
		"keep, one trailing blank":  "- >+\n  x\n\n  y\n\n- 1\n",
		"folded twice":              "- >+\n  a\n\n  b\n\n  c\n- 1\n",

		// Under a mapping key, and with a third entry after it.
		"under a mapping key": "a: >+\n  x\n\n  y\nb: 1\n",
		"two entries after":   "- >+\n  x\n\n  y\n\n- 1\n- 2\n",

		// Neither of these ever diverged: with nothing after the scalar there
		// is no gap to measure, and with no break inside it nothing folds.
		"nothing after the scalar": "- >+\n  x\n\n  y\n",
		"no break inside":          "- >+\n  x\n- 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			var before any
			require.NoError(t, yaml.Unmarshal([]byte(src), &before))

			file, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoError(t, err)

			rendered := file.String()
			assert.Equal(t, src, rendered, "the document is written back as it was read")

			var after any
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &after))
			assert.Equal(t, before, after, "the value survives being written out")

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, rendered, reread.String(), "and settles in one pass")
		})
	}
}

// TestFixedANonStringKeyNoLongerZeroesAWholeStruct: a key no field can be named
// after is skipped, and the entries around it read.
//
// ✅ Closed 2026-09-07 in two steps. 114e426 inverted decodeStruct, so the
// decode walks the document's entries and looks each field up rather than
// walking the fields and reading the mapping into a map first -- the map came
// back nil at the first key that was not a string, with no error, and every
// field kept its zero. entryName then named a key by its type's own canonical
// spelling, so a key a field can be named after reaches it: "true: a" writes a
// field tagged "true".
func TestFixedANonStringKeyNoLongerZeroesAWholeStruct(t *testing.T) {
	type named struct {
		Name string `yaml:"name"`
		True string `yaml:"true"`
	}

	for _, src := range []string{
		"1: a\nname: x\n",
		"name: x\n1: a\n",
		"1: a\nname: x\n2: b\n",
	} {
		wellFormed(t, src)

		var got named
		require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		assert.Equal(t, "x", got.Name, "%q: the entries around the key that cannot be named must read", src)
	}

	t.Run("a key a field can be named after reaches it", func(t *testing.T) {
		var got named
		require.NoError(t, yaml.Unmarshal([]byte("true: a\nname: x\n"), &got))
		assert.Equal(t, named{Name: "x", True: "a"}, got)
	})

	t.Run("the same documents read into an any", func(t *testing.T) {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte("1: a\nname: x\n"), &got))
		assert.Equal(t, map[string]any{"1": "a", "name": "x"}, got)
	})

	t.Run("and a string-keyed document reads into the struct", func(t *testing.T) {
		var got named
		require.NoError(t, yaml.Unmarshal([]byte("name: x\n"), &got))
		assert.Equal(t, named{Name: "x"}, got)
	})
}

// TestFixedATagTypesItsScalarWhateverItsSpelling: one tag gives one tree, in
// all three spellings.
//
// "!!float 7", "!<tag:yaml.org,2002:float> 7" and "!e!float 7" under a
// "%TAG !e! tag:yaml.org,2002:" line name the same tag, and each now puts an
// *ast.IntegerNode under its *ast.TagNode. Only the shorthand did: the scanner
// forced every other spelling to a string, because it matched the text the tag
// was written with against the reserved keywords instead of the URI it expands
// to -- and the URI is what only the parser knows, since a "%TAG" line can
// repoint a handle.
//
// Three values were lost by it rather than merely retyped. A string is read
// back against its tag afterwards, so "7", "0x1f" and "true" survived the
// detour; ".inf", "-.inf" and ".nan" decoded to the float64 zero and converted
// to the JSON "0.0", each reporting nothing.
func TestFixedATagTypesItsScalarWhateverItsSpelling(t *testing.T) {
	t.Run("the tree is the same in every spelling", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			node ast.Node
		}{
			{src: "!!float 7\n", node: &ast.IntegerNode{}},
			{src: "!!float 1e3\n", node: &ast.FloatNode{}},
			{src: "!!float .inf\n", node: &ast.InfinityNode{}},
			{src: "!<tag:yaml.org,2002:float> 7\n", node: &ast.IntegerNode{}},
			{src: "!<tag:yaml.org,2002:float> 1e3\n", node: &ast.FloatNode{}},
			{src: "!<tag:yaml.org,2002:float> .inf\n", node: &ast.InfinityNode{}},
			{src: "%TAG !e! tag:yaml.org,2002:\n---\n!e!float 1e3\n", node: &ast.FloatNode{}},
		} {
			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err, "%q", tc.src)

			tag, ok := file.Docs[len(file.Docs)-1].Body.(*ast.TagNode)
			require.True(t, ok, "%q: the body is not a tag node", tc.src)
			assert.Equal(t, "tag:yaml.org,2002:float", tag.URI, "%q", tc.src)
			assert.IsType(t, tc.node, tag.Value, "%q", tc.src)
		}
	})

	t.Run("the three specials keep their value in every spelling", func(t *testing.T) {
		for text, want := range map[string]float64{
			".inf":  math.Inf(1),
			"-.inf": math.Inf(-1),
			".nan":  math.NaN(),
		} {
			for _, src := range []string{
				"!!float " + text + "\n",
				"!<tag:yaml.org,2002:float> " + text + "\n",
			} {
				var got any
				require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
				assert.InDelta(t, want, got, 0, "%q", src)

				_, err := codec.ToJSON([]byte(src))
				require.Error(t, err, "JSON has no infinity or NaN, so refusing is right: %q", src)
			}
		}
	})

	t.Run("a tag that resolves to nothing still leaves its scalar as text", func(t *testing.T) {
		for _, src := range []string{
			"!foo 12\n",
			"! 12\n",
			"!<x:y> 12\n",
			// "%TAG !!" repoints the secondary handle, so "!!int" names
			// !local-int here and resolves to nothing. This is the spelling the
			// scanner could never have judged.
			"%TAG !! !local-\n---\n!!int 12\n",
		} {
			var got any
			require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, "12", got, "%q", src)
		}
	})
}

// TestFixedATaggedBlockMappingResolvesItsKeys: a tag on a block mapping types
// the keys inside it, as an untagged mapping does.
//
// The tag stands on the mapping; each key is a node of its own and resolves on
// its own, so "!foo" over "False: 1" is keyed by "false" -- the canonical
// spelling of the boolean -- and not by the text "False". The scanner used to
// force the token after a tag it did not recognize to a string, and the token
// after a tag that opens a block mapping is that mapping's first key.
func TestFixedATaggedBlockMappingResolvesItsKeys(t *testing.T) {
	for _, src := range []string{
		"!foo\nFalse: 1\n",
		"!\nFalse: 1\n",
		"!<tag:yaml.org,2002:map>\nFalse: 1\n",
		"!!map\nFalse: 1\n",
		"!foo {False: 1}\n",
		"False: 1\n",
	} {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		assert.Equal(t, map[string]any{"false": uint64(1)}, got, "%q", src)
	}
}

// TestFixedAKeyAfterALongTagOnAnEmptyValueResolves: an entry whose value is a
// tag with nothing after it no longer stops the next key from resolving.
//
// "a: !<tag:yaml.org,2002:null>" over "False: 1" is keyed by "false", as
// "a: !!null" over the same line always was. The scanner's rule reached past
// the tag's own node to whatever token came next, and where the tag stood alone
// at the end of a line that token was the following key.
func TestFixedAKeyAfterALongTagOnAnEmptyValueResolves(t *testing.T) {
	for _, src := range []string{
		"a: !<tag:yaml.org,2002:null>\nFalse: 1\n",
		"a: !!null\nFalse: 1\n",
		"%TAG !e! tag:yaml.org,2002:\n---\na: !e!null\nFalse: 1\n",
	} {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		assert.Equal(t, map[string]any{"a": nil, "false": uint64(1)}, got, "%q", src)
	}
}

// TestFixedAPropertyAloneAfterAColonNamesTheEmptyNode: a tag or an anchor
// written with nothing after it stands on the empty node, and the entries below
// it stay where the document put them.
//
// 8.2.2 needs a nested block mapping indented further than the key it belongs
// to, and 8.2.1 the same for a sequence, so a token back at the entry's own
// column opens the next entry rather than continuing this one. The parser said
// that for a bare anchor after "k:" and for the tags the core schema resolves,
// and nowhere else. Two shapes went the other way and restructured the
// document without reporting anything:
//
//   - "a: !foo" over "b: 1" over "c: 2" came back as {"a": {"b": 1, "c": 2}},
//     and "- !foo" over "- 1" as [[1]]. "!!null" and "!!str" on the same empty
//     value read flat, so it was the tags naming no known type.
//   - "? a" over ": &a1" over "? b" over ": &a2" came back as
//     {"a": {"b": nil}}. The short form "a: &a1" over "b: &a2" read flat, so it
//     was the anchor and the explicit key together: an explicit key's value is
//     read through parseMapKeyValue, which recorded no entry column for the
//     property to measure itself against.
func TestFixedAPropertyAloneAfterAColonNamesTheEmptyNode(t *testing.T) {
	t.Run("the entries below stay flat", func(t *testing.T) {
		for src, want := range map[string]any{
			"a: !foo\nb: 1\nc: 2\n":    map[string]any{"a": nil, "b": uint64(1), "c": uint64(2)},
			"a: !\nb: 1\n":             map[string]any{"a": nil, "b": uint64(1)},
			"- !foo\n- b\n":            []any{nil, "b"},
			"? a\n: &a1\n? b\n: &a2\n": map[string]any{"a": nil, "b": nil},
			"? a\n: !foo\n? b\n: 1\n":  map[string]any{"a": nil, "b": uint64(1)},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
			assert.Equal(t, src, renderOnce(t, src), "%q: the nesting is written back out", src)
		}
	})

	t.Run("an alias to the anchor reads it", func(t *testing.T) {
		// It used to report `alias "a1" names an anchor that is not resolved
		// yet`, because the anchor was inside the node still being built.
		var got any
		require.NoError(t, yaml.Unmarshal([]byte("? a\n: &a1\n? b\n: *a1\n"), &got))
		assert.Equal(t, map[string]any{"a": nil, "b": nil}, got)
	})

	t.Run("and a value written further in is still the property's node", func(t *testing.T) {
		for src, want := range map[string]any{
			// Indented past the key, so it is nested and always was.
			"a: !foo\n  b: 1\n":    map[string]any{"a": map[string]any{"b": uint64(1)}},
			"? a\n: &a1\n  b: 1\n": map[string]any{"a": map[string]any{"b": uint64(1)}},
			"- !foo\n  - 1\n":      []any{[]any{uint64(1)}},
			// 8.2.1 lets a block sequence stand at its key's own column, so
			// this is the value and not the next entry.
			"a: !foo\n- 1\n": map[string]any{"a": []any{uint64(1)}},
			"k: &a\n- 1\n":   map[string]any{"k": []any{uint64(1)}},
			// Written beside the property, so it is what the property names.
			"? a\n: &a1 x\n? b\n: *a1\n": map[string]any{"a": "x", "b": "x"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedAScalarUnderAnUnresolvedTagIsRead: two documents the generator found
// refused, both a scalar standing under a tag that names no type this library
// reads.
//
// Such a scalar keeps the text it was written with, and parseTagValue built
// that string without stepping past the token it had just read. The next reader
// found a token where the entry had already ended and reported "value is not
// allowed in this context". The two shapes look unrelated and are one fault: a
// tag written on its own line with a comment under it, and an unknown secondary
// tag on a root scalar under a "%YAML" directive.
func TestFixedAScalarUnderAnUnresolvedTagIsRead(t *testing.T) {
	for src, want := range map[string]any{
		"a:\n !\n # c\n 1\n":             map[string]any{"a": "1"},
		"a:\n !\n 1\n":                   map[string]any{"a": "1"},
		"%YAML 1.1\n---\n!!nulll Null\n": "Null",
		"!!nulll Null\n":                 "Null",
	} {
		var got any
		require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		assert.Equal(t, want, got, "%q", src)
	}
}

// TestFixedAnIntTagReadsIntoAGoInteger: "!!int" on a value reads into a Go
// integer, as the same value untagged always did.
//
// [yamlgen.Tagged.Decoded] says where the two parted company: an untagged
// non-negative integer comes back as a uint64 and a negative one as an int64,
// where "!!int" hands back a plain int for a number that fits one -- which is
// what strconv.Atoi gave. Decoder.decodeValue read a uint64, an int64, a
// float64 and a string into an integer field and had no case for an int, so it
// reported `cannot unmarshal int into Go struct field box.N of type int64`.
func TestFixedAnIntTagReadsIntoAGoInteger(t *testing.T) {
	type box struct {
		N  int64   `yaml:"n"`
		I  int     `yaml:"i"`
		I8 int8    `yaml:"i8"`
		U  uint64  `yaml:"u"`
		F  float64 `yaml:"f"`
		A  any     `yaml:"a"`
	}

	t.Run("into an integer field, whatever its width", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("n: !!int 5\ni: !!int 6\ni8: !!int 7\nu: !!int 8\n"), &got))
		assert.Equal(t, box{N: 5, I: 6, I8: 7, U: 8}, got)

		var negative box
		require.NoError(t, yaml.Unmarshal([]byte("n: !<tag:yaml.org,2002:int> -5\n"), &negative))
		assert.Equal(t, box{N: -5}, negative)
	})

	t.Run("into a slice and into a typed map", func(t *testing.T) {
		var items []int64
		require.NoError(t, yaml.Unmarshal([]byte("- !!int 5\n"), &items))
		assert.Equal(t, []int64{5}, items)

		var byName map[string]int64
		require.NoError(t, yaml.Unmarshal([]byte("n: !!int 5\n"), &byName))
		assert.Equal(t, map[string]int64{"n": 5}, byName)
	})

	t.Run("and a number too wide for the field still overflows", func(t *testing.T) {
		// The same complaint the untagged number draws, which is the point:
		// the tag changes what the node is and not how wide the field is.
		for _, src := range []string{"i8: !!int 300\n", "i8: 300\n", "u: !!int -5\n"} {
			var got box
			err := yaml.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Contains(t, err.Error(), "overflow", "%q", src)
		}
	})
}

// TestFixedAFloatTagOnAWideNumberKeepsItsWidth: "!!float" on a number no
// float64 holds reads as a big.Float, as the same number untagged always did.
//
// It failed two ways. Large, the parse stopped: ast.readsAsFloat took
// strconv.ParseFloat's ErrRange for "not a float", so "!!float 1e+310" was a
// tag naming a type its scalar is not and the document was refused. Small, it
// was worse than refused: "!!float 1e-400" came back as the float64 zero with
// nothing reported, because castToFloatValue narrowed the big.Float that the
// node held.
//
// codec.ToJSON lost the same numbers, writing "0.0" for both, and now writes
// them out in full.
func TestFixedAFloatTagOnAWideNumberKeepsItsWidth(t *testing.T) {
	t.Run("the decoder reads a big.Float, tagged or not", func(t *testing.T) {
		for _, src := range []string{
			"a: !!float 1e+310\n", "a: 1e+310\n",
			"a: !!float 1e-400\n", "a: 1e-400\n",
			"a: !!float -1e+310\n",
			"a: !<tag:yaml.org,2002:float> 1e+310\n",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.IsType(t, new(big.Float), got.(map[string]any)["a"], "%q", src)
		}
	})

	t.Run("ToJSON writes the number, tagged or not", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{src: "!!float 1e+310\n", want: "1e+310"},
			{src: "1e+310\n", want: "1e+310"},
			{src: "!!float 1e-400\n", want: "1e-400"},
			{src: "1e-400\n", want: "1e-400"},
			{src: "!!float -1e+310\n", want: "-1e+310"},
			// A float the machine word does hold still writes its fraction.
			{src: "!!float 1.5\n", want: "1.5"},
			{src: "!!float 7\n", want: "7.0"},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.want, string(out), "%q", tc.src)
		}
	})

	t.Run("and an int tag on a wide integer still reads as a big.Int", func(t *testing.T) {
		var got any
		require.NoError(t, yaml.Unmarshal([]byte("a: !!int 123456789012345678901\n"), &got))
		assert.IsType(t, new(big.Int), got.(map[string]any)["a"])
	})
}

// TestFixedAnAnchorBetweenATagAndItsScalarKeepsTheText: an anchor written
// between a tag and the scalar it types no longer changes the type.
//
// A tag that resolves to nothing leaves its scalar as the text it was written
// with, so "!foo true" is the string "true". parseTagValue applied that to the
// tag's own next token and an anchor may stand there, so "!foo &a1 true" read
// the boolean where "&a1 !foo true" -- the same two properties the other way
// round -- read the string. Only the tags naming no known type did it: "!!str"
// in the same place was unaffected.
func TestFixedAnAnchorBetweenATagAndItsScalarKeepsTheText(t *testing.T) {
	t.Run("the order of the two properties no longer decides", func(t *testing.T) {
		for _, src := range []string{
			"!foo true\n", "!foo &a1 true\n", "&a1 !foo true\n",
			"!<!foo> &a1 true\n", "! &a1 true\n", "!!str &a1 true\n",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, "true", got, "%q", src)
		}
	})

	t.Run("in a mapping, a sequence, a flow collection and through an alias", func(t *testing.T) {
		for src, want := range map[string]any{
			"a: !foo &a1 12\n":         map[string]any{"a": "12"},
			"- !foo &a1 12\n":          []any{"12"},
			"{a: !foo &a1 12}\n":       map[string]any{"a": "12"},
			"a: !foo &a1 12\nb: *a1\n": map[string]any{"a": "12", "b": "12"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("and a tag that does resolve still types its scalar", func(t *testing.T) {
		for src, want := range map[string]any{
			"!!int &a1 12\n":     12, // "!!int" hands back a plain int for a number that fits one.
			"!!float &a1 12\n":   float64(12),
			"!!seq &a1 [1, 2]\n": []any{uint64(1), uint64(2)},
			"!foo &a1 [1, 2]\n":  []any{uint64(1), uint64(2)},
			"!foo &a1 |\n  x\n":  "x\n",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedAQuotedExplicitKeyTakesABlockScalarValue: `? "a"` over `: >-` reads,
// as `? a` over the same two lines always did.
//
// The scanner measures the lines of a value against Scanner.lastDelimColumn,
// and scanMapValue picked the wrong column for it. A key already cut into
// tokens -- a quoted one, or an empty scalar carrying an anchor or a tag --
// sets the level from the key's own start, which is right while the key and its
// ":" stand on one line. Written the long way they do not: "? \"a\"" puts the
// quote in column 3, the ":" is in column 1 on the next line, and the block
// scalar's content in column 3 then read as level with its own delimiter --
// the end of the scalar rather than its first line. The header was cut short,
// an empty string went in as the value, and the content was left over for the
// document to complain about.
//
// The ":" is where the entry sits when the key was written above it, so that
// test goes first now and the key's own column is asked only for a key on the
// same line.
func TestFixedAQuotedExplicitKeyTakesABlockScalarValue(t *testing.T) {
	t.Run("the shapes that were refused", func(t *testing.T) {
		for _, src := range []string{
			"? \"a\"\n: >-\n  x\n",
			"? 'a'\n: >-\n  x\n",
			"? \"a\"\n: |\n  x\n",
			"a:\n  ? \"b\"\n  : >-\n    x\n",
			"- ? \"a\"\n  : >-\n    x\n",
			"? \"a\"\n: &an >-\n  x\n",
			"? \"a\"\n: !!str >-\n  x\n",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		}
	})

	t.Run("the value is the block scalar and not an empty string", func(t *testing.T) {
		for src, want := range map[string]any{
			"? \"a\"\n: >-\n  x\n":               map[string]any{"a": "x"},
			"? 'a'\n: >-\n  x\n":                 map[string]any{"a": "x"},
			"? \"a\"\n: |\n  x\n":                map[string]any{"a": "x\n"},
			"? a\n: >-\n  x\n":                   map[string]any{"a": "x"},
			"\"a\": >-\n  x\n":                   map[string]any{"a": "x"},
			"? \"a\"\n: >-\n  x\n? \"b\"\n: 1\n": map[string]any{"a": "x", "b": uint64(1)},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("a key on the ':' line still measures from the key", func(t *testing.T) {
		// The branch the fix moved: "&a :" cuts the key into tokens and the
		// value's lines are measured from the '&', not from the name after it.
		for src, want := range map[string]any{
			"&a :\n":              map[string]any{"null": nil},
			"\"a\": >-\n  x\n":    map[string]any{"a": "x"},
			"a: >-\n  x\n":        map[string]any{"a": "x"},
			"? \"a\"\n: [1]\n":    map[string]any{"a": []any{uint64(1)}},
			"? &a x\n: >-\n  y\n": map[string]any{"x": "y"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedACommentOnAnExplicitKeysColonLineIsKept: a comment on the ":" line
// of the long form, with the value below it, survives.
//
// newMappingValueNode returned early for every explicit key, on the reading
// that a comment on the token it was handed was the key's own and attached
// there already. That holds where parseMapKeyValue hands the key's own last
// token over, since an explicit key written in one group ends on the key rather
// than on a ":". It does not hold where the ":" is a token of its own: the
// comment then stands on the ":" line and belongs to the value, and returning
// early dropped it with nothing reported.
//
// It goes on the entry. Put on the value it collided with a head comment
// written under it -- the value begins on a later line, so the line comment
// degrades to a head comment and takes that slot -- and the entry is what the
// comment was written about anyway.
//
// The renderer writes an entry's line comment above the entry, so "? a" over
// ": # c3" over "  v" comes back as "# c3" over "? a" over ": v". Every
// rendering settles and no comment is lost; putting it back on the ':' line is
// the renderer's to do.
func TestFixedACommentOnAnExplicitKeysColonLineIsKept(t *testing.T) {
	for src, renders := range map[string]string{
		"? a\n: # c3\n  v\n":   "? a\n: # c3\n  v\n",
		"? a\n: # c3\n  - 1\n": "? a\n: # c3\n- 1\n",
		"?\n: #c1\n":           "?\n: #c1\n",
		// Every other position kept it before and still does.
		"a: # c3\n  v\n":   "a: v # c3\n",
		"a: # c3\n  - 1\n": "a: # c3\n- 1\n",
		"? a\n: v # c3\n":  "? a\n: v # c3\n",
		"? a # c3\n: v\n":  "? a # c3\n: v\n",
	} {
		once := renderOnce(t, src)
		assert.Equal(t, renders, once, "%q", src)

		assert.Equal(t, once, renderOnce(t, once), "%q: the rendering settles", src)
	}

	t.Run("the value is unchanged", func(t *testing.T) {
		for src, want := range map[string]any{
			"? a\n: # c3\n  v\n":   map[string]any{"a": "v"},
			"? a\n: # c3\n  - 1\n": map[string]any{"a": []any{uint64(1)}},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedALocalTagBeforeAnAnchorTypesItsScalar: a local or non-specific tag
// keeps its scalar as text whichever side of the anchor it is written on.
//
// ✅ Closed 2026-09-13 by the parser reaching the scalar an anchor group names
// and retyping the token before the node is built. It used to depend on the
// order: "!foo &a1 true" read the boolean where "!foo true" and
// "&a1 !foo true" read the string.
//
// The whole matrix is held rather than the one document that failed, because a
// fix that traded one spelling for another would otherwise look like a fix.
func TestFixedALocalTagBeforeAnAnchorTypesItsScalar(t *testing.T) {
	for _, src := range []string{
		// The four spellings, with the anchor after the tag.
		"!foo &a1 true\n",
		"! &a1 true\n",
		"!<!foo> &a1 true\n",
		// The anchor first, which always read.
		"&a1 !foo true\n",
		// No anchor at all.
		"!foo true\n",
		"! true\n",
		// A secondary tag, which was never affected.
		"!!str &a1 true\n",
	} {
		var got any
		require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		assert.Equalf(t, "true", got, "%q", src)
	}

	t.Run("in every context", func(t *testing.T) {
		for src, want := range map[string]any{
			"a: !foo &a1 true\n":   map[string]any{"a": "true"},
			"- !foo &a1 true\n":    []any{"true"},
			"{a: !foo &a1 true}\n": map[string]any{"a": "true"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, want, got, "%q", src)
		}
	})
}

// TestFixedABinaryTagReadsIntoAGoByteSlice: `!!binary` reads into the Go type
// the tag names.
//
// ✅ Closed 2026-09-13 by decodeSlice asking binaryBytes first. It used to be
// refused with "string was used where sequence is expected", while an `any`
// gave []uint8 and a string field gave the decoded bytes -- the one Go type the
// tag names was the one it could not reach.
func TestFixedABinaryTagReadsIntoAGoByteSlice(t *testing.T) {
	const src = "a: !!binary aGVsbG8=\n"

	t.Run("into a field, a typed map and a slice element", func(t *testing.T) {
		var into struct {
			A []byte `yaml:"a"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(src), &into))
		assert.Equal(t, []byte("hello"), into.A)

		var byName map[string][]byte
		require.NoError(t, yaml.Unmarshal([]byte(src), &byName))
		assert.Equal(t, map[string][]byte{"a": []byte("hello")}, byName)

		var items [][]byte
		require.NoError(t, yaml.Unmarshal([]byte("- !!binary aGVsbG8=\n"), &items))
		assert.Equal(t, [][]byte{[]byte("hello")}, items)
	})

	t.Run("and the destinations that always worked still do", func(t *testing.T) {
		var loose any
		require.NoError(t, yaml.Unmarshal([]byte(src), &loose))
		assert.Equal(t, map[string]any{"a": []byte("hello")}, loose)

		var timed struct {
			T time.Time `yaml:"t"`
		}
		require.NoError(t, yaml.Unmarshal([]byte("t: !!timestamp 2001-12-14\n"), &timed))
		assert.Equal(t, time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC), timed.T)
	})
}

// TestFixedAVersionDirectiveLeavesTheRootBlockScalarAlone: a "%YAML" line over
// a document whose body is a block scalar reads it, whatever the content
// spells.
//
// A block scalar is a string under every schema, so there was nothing there to
// resolve. Parser.retypeAhead reads the plain scalars the scan had already cut
// past the directive and types them again against the version it names, and a
// block scalar's content is cut as a plain String -- the one string a schema
// must not touch. "%YAML 1.1" over "---" over ">-" over " null" had the content
// retyped as a null, and parseLiteral then refused the document with
// "unexpected token. required string token".
//
// The tape is not grouped when retypeAhead runs, so the header and its content
// are still two tokens and the content is whatever follows the header -- which
// is how stageBlockScalars reads it too.
func TestFixedAVersionDirectiveLeavesTheRootBlockScalarAlone(t *testing.T) {
	t.Run("every content the schema would have resolved", func(t *testing.T) {
		for _, text := range []string{"null", "~", "True", "yes", "5", "1.5", "0100", "x", "x y", "null x"} {
			for _, header := range []string{">-", "|-"} {
				src := "%YAML 1.1\n---\n" + header + "\n " + text + "\n"
				wellFormed(t, src)

				var got any
				require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
				assert.Equal(t, text, got, "%q", src)
			}
		}
	})

	t.Run("under either version, and with the body nested", func(t *testing.T) {
		for src, want := range map[string]any{
			"%YAML 1.2\n---\n>-\n null\n":                   "null",
			">-\n null\n":                                   "null",
			"---\n>-\n null\n":                              "null",
			"%TAG !e! tag:yaml.org,2002:\n---\n>-\n null\n": "null",
			"%YAML 1.1\n---\nk: >-\n  null\n":               map[string]any{"k": "null"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("and the schema still reaches every plain scalar after the directive", func(t *testing.T) {
		// The retyping is what makes a directive work at all, so the fix must
		// not stop it: 1.1 reads "0100" as 64 where 1.2 reads 100.
		for src, want := range map[string]any{
			"%YAML 1.1\n---\n0100\n":    uint64(64),
			"%YAML 1.2\n---\n0100\n":    uint64(100),
			"%YAML 1.1\n---\nyes\n":     true,
			"%YAML 1.2\n---\nyes\n":     "yes",
			"%YAML 1.1\n---\nk: 0100\n": map[string]any{"k": uint64(64)},
			// A plain scalar after a block scalar still resolves, and after two.
			"%YAML 1.1\n---\na: >-\n  null\nb: 0100\n": map[string]any{"a": "null", "b": uint64(64)},
			"%YAML 1.1\n---\na: >-\n  null\nb: |-\n  yes\nc: 0100\n": map[string]any{
				"a": "null", "b": "yes", "c": uint64(64),
			},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedATagOnItsOwnLineTakesTheBlockScalarUnderIt: a tag written on a line
// of its own, over a block scalar, reads.
//
// 6.9.1 and 8.1 allow both: a node's properties may stand on a line of their
// own, and the node under them may be a block scalar. "!!null" over ">" was
// refused with "value is not allowed in this context", where "!!null >" on one
// line read and so did "!foo" over ">".
//
// The tag was what decided it, because of where the grouping puts the two. A
// tag is joined only to what stands on its own line, so "!!null >" arrives at
// parseTagValue as one scalar-tag group and "!!null" over ">" as a tag and a
// folded group. The branch that reads the second returned the literal without
// stepping past it, and the document then held a token nothing had read --
// the same fault as the scalar under an unresolved tag, one branch over.
func TestFixedATagOnItsOwnLineTakesTheBlockScalarUnderIt(t *testing.T) {
	t.Run("the shapes that were refused", func(t *testing.T) {
		for src, want := range map[string]any{
			"!!null\n>\n":           nil,
			"!!str\n>-\n x\n":       "x",
			"!!int\n>-\n 5\n":       5,
			"!!bool\n>-\n true\n":   true,
			"!!null\n|\n":           nil,
			"k: !!null\n  >\n":      map[string]any{"k": nil},
			"- !!str\n  >-\n   x\n": []any{"x"},
			// The version directive reaches the scalar under the tag as it
			// reaches any other, and a block scalar is a string either way.
			"%YAML 1.1\n---\n!!str\n>-\n null\n": "null",
		} {
			wellFormed(t, src)

			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("the spellings that read before still read", func(t *testing.T) {
		for src, want := range map[string]any{
			"!!null >\n":     nil,
			"!foo\n>\n":      "",
			"&a\n>\n":        "",
			"!!seq\n>\n":     "",
			"!!str >-\n x\n": "x",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("and what the tag cannot hold is still refused, at the tag", func(t *testing.T) {
		// A value the tag names a type for and cannot read, and a node of the
		// wrong kind entirely. Both parse; the refusal is resolution's.
		for src, says := range map[string]string{
			"!!null\n>-\n x\n": `cannot read "x" as !!null`,
			"!!null\n[1]\n":    "!!null names a kind this node is not",
		} {
			_, perr := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, perr, "%q parses", src)

			var got any
			err := yaml.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Contains(t, err.Error(), says, "%q", src)
		}
	})
}

// TestFixedAMappingKeyWrittenEmptyIsRead: an empty key reads in every position,
// and what precedes it no longer decides.
//
// Three shapes were refused with two messages, both naming the line above the
// key: "a:" over ": 2" reported `unexpected scalar value`, and "k: &a1" over
// ": 1" and "false: !!bool false" over ": &a1 !!null" both reported
// `mapping value is not allowed in this context`.
//
// keyWindow.hasNoKey took any candidate the grouping had already made
// something of as the key of the ':' that followed, whatever line it stood on.
// So the first entry's own key group, or an anchored value, became the key of
// the ':' below it. An implicit key stands on the line its ':' does -- 7.4.2 --
// so the line test applies to a grouped candidate too. An explicit key is the
// exception the rule needs: "? a" over ": 2" writes the two on separate lines
// by design, and it is told apart by opening with a "?".
func TestFixedAMappingKeyWrittenEmptyIsRead(t *testing.T) {
	t.Run("every position", func(t *testing.T) {
		for src, want := range map[string]any{
			// The three that were refused.
			"a:\n: 2\n":                           map[string]any{"a": nil, "null": uint64(2)},
			"k: &a1\n: 1\n":                       map[string]any{"k": nil, "null": uint64(1)},
			"false: !!bool false\n: &a1 !!null\n": map[string]any{"false": false, "null": nil},
			// The three that always read.
			": a\n":           map[string]any{"null": "a"},
			"a: 1\n: 2\n":     map[string]any{"a": uint64(1), "null": uint64(2)},
			"- k: 1\n  : 2\n": []any{map[string]any{"k": uint64(1), "null": uint64(2)}},
			// An anchored value with content, and nested.
			"k: &a1 x\n: 1\n":   map[string]any{"k": "x", "null": uint64(1)},
			"a:\n: 2\nb: 3\n":   map[string]any{"a": nil, "null": uint64(2), "b": uint64(3)},
			"x:\n  a:\n  : 2\n": map[string]any{"x": map[string]any{"a": nil, "null": uint64(2)}},
			"k: |\n  x\n: 1\n":  map[string]any{"k": "x\n", "null": uint64(1)},
		} {
			wellFormed(t, src)

			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("an explicit key still keys the ':' below it", func(t *testing.T) {
		for src, want := range map[string]any{
			"? a\n: 2\n":           map[string]any{"a": uint64(2)},
			"? a\n: 2\n? b\n: 3\n": map[string]any{"a": uint64(2), "b": uint64(3)},
			"? [a]\n: 1\n":         map[string]any{"[a]": uint64(1)},
			// A key on the ':' line is still the key, grouped or not.
			"!!str foo: 1\n": map[string]any{"foo": uint64(1)},
			"&a1 x: 1\n":     map[string]any{"x": uint64(1)},
			// Inside a flow collection a ':' may stand on its own line.
			"{a: 1, : 2}\n": map[string]any{"a": uint64(1), "null": uint64(2)},
			"{: 1}\n":       map[string]any{"null": uint64(1)},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})

	t.Run("and a node above a ':' is no longer taken as its key", func(t *testing.T) {
		// The laxity that went with it. grammar.NewRecognizer, the reference
		// parser, libfyaml 1.0.0b1 and go.yaml.in/yaml/v3 v3.0.5 all refuse
		// these; this library read them.
		for _, src := range []string{"a Null\n: 1\n", "!x Null\n: 1\n", "!!str Null\n: 1\n"} {
			var got any
			assert.Errorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
		}
	})
}

// TestFixedABlockScalarInASequenceKeepsAnEmptyKeyApart: the same fix, seen
// through the renderer.
//
// "a:" over " - |1-" over "   " over ":" rendered to "a:" over "- |2-    :",
// which reported `invalid header option` on the way back in -- a valid document
// rendering to one that is not YAML. The empty key was being read as part of
// the block scalar's entry, so the renderer wrote the two onto one line.
func TestFixedABlockScalarInASequenceKeepsAnEmptyKeyApart(t *testing.T) {
	for src, renders := range map[string]string{
		"a:\n - &a1 |1-\n   \n:\n":    "a:\n- &a1 |2-\n   \n:\n",
		"a:\n - |1-\n   \n:\n":        "a:\n- |2-\n   \n:\n",
		"a:\n - &a1 |1-\n   x\n:\n":   "a:\n- &a1 |2-\n   x\n:\n",
		"a:\n - &a1 |1\n   \n:\n":     "a:\n- &a1 |2\n   \n:\n",
		"a:\n - &a1 |1-\n   \nb: 1\n": "a:\n- &a1 |2-\n   \nb: 1\n",
	} {
		wellFormed(t, src)

		once := renderOnce(t, src)
		assert.Equal(t, renders, once, "%q", src)

		reread, err := parser.ParseBytes([]byte(once), parser.WithComments())
		require.NoErrorf(t, err, "%q: the rendering must parse", src)
		assert.Equal(t, once, reread.String(), "%q: and settle", src)
	}
}

// TestFixedABlockSequenceOnItsTagsLineIsRefused: "!foo - 1" is not a document
// and is no longer read as one.
//
// 8.2.1 puts s-l-comments between a node's properties and the collection under
// them, and s-l-comments requires a line break, so a block sequence cannot
// begin on the line its tag was written on. The grouping has always refused one
// on an anchor's line -- "sequence entries are not allowed after anchor on the
// same line" -- and left a tag to the parser, which caught only the tags the
// core schema resolves: "!!int - 8" was refused as `value is not allowed in
// this context` and "!foo - 1" was read as [1].
//
// This was yamlgen.Lax's last entry, and it is the one register whose emptiness
// says nothing: the mutation hunt is what fills it.
func TestFixedABlockSequenceOnItsTagsLineIsRefused(t *testing.T) {
	t.Run("every spelling of the tag, and one message", func(t *testing.T) {
		for _, src := range []string{
			"!foo - 1\n", "!!int - 8\n", "! - 1\n", "!<x:y> - 1\n",
			"a: !foo - 1\n", "- !foo - 1\n", "- ! -\n",
		} {
			var got any
			err := yaml.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Contains(t, err.Error(),
				"sequence entries are not allowed after a tag on the same line", "%q", src)
		}
	})

	t.Run("written correctly it reads, and a '-' that is not an entry still does", func(t *testing.T) {
		for src, want := range map[string]any{
			"!foo\n- 1\n":    []any{uint64(1)},
			"a: !foo\n- 1\n": map[string]any{"a": []any{uint64(1)}},
			"!foo [1]\n":     []any{uint64(1)},
			"!foo -1\n":      "-1",
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equal(t, want, got, "%q", src)
		}
	})
}

// TestFixedAVersionDirectiveIsScopedToOneDocument: a "%YAML" directive reaches
// the document it precedes and none of the next.
//
// It used to reach every document of the stream, and the library disagreed with
// itself: an anchor and a %TAG handle are both scoped to the document that
// declares them, and enforced, while a version directive was not. Documents are
// independent -- Fred, 2026-09-13 -- so the second document below is read under
// the core schema, where "yes" is the string.
//
// ✅ Fixed on 2026-09-13. parseDocument ended the scope only for a document
// closed by "...", and that reset did not take effect either: it set the schema
// back and left the tokens alone, though by the time a "..." is read the
// grouping has cut the whole of the next document. Parser.endVersionScope runs
// for every document and hands retypeAhead the first token the descent has not
// taken. parser/zz_version_test.go holds the shapes that separate the two.
func TestFixedAVersionDirectiveIsScopedToOneDocument(t *testing.T) {
	t.Run("the directive stops at the document it opens", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: yes\n---\nb: yes\n"

		wellFormed(t, src)

		got := readTheStream(t, src)
		require.Len(t, got, 2)
		assert.Equal(t, map[string]any{"a": true}, got[0], "the first document declares 1.1")
		assert.Equal(t, map[string]any{"b": "yes"}, got[1],
			"the second declares nothing and is read under the core schema")
	})

	t.Run("and a document end does not have to say so", func(t *testing.T) {
		// The "..." form was the one that looked fixed and was not: the reset
		// ran and the tokens had already been cut under 1.1.
		assert.Equal(t, []any{map[string]any{"a": true}, map[string]any{"b": "yes"}},
			readTheStream(t, "%YAML 1.1\n---\na: yes\n...\nb: yes\n"))
	})

	t.Run("which is what each document means on its own", func(t *testing.T) {
		// The same two documents, written apart.
		assert.Equal(t, []any{map[string]any{"a": true}}, readTheStream(t, "%YAML 1.1\n---\na: yes\n"))
		assert.Equal(t, []any{map[string]any{"b": "yes"}}, readTheStream(t, "b: yes\n"))
	})

	t.Run("an anchor and a tag handle are scoped the same way", func(t *testing.T) {
		var v any
		err := yaml.Unmarshal([]byte("a: &x 1\n---\nb: *x\n"), &v)
		require.Error(t, err, "an anchor does not reach the next document")
		assert.Contains(t, err.Error(), `could not find alias "x"`)

		_, herr := parser.ParseBytes(
			[]byte("%TAG !e! tag:yaml.org,2002:\n---\na: !e!str 1\n---\nb: !e!str 2\n"),
			parser.WithComments())
		require.Error(t, herr, "a handle does not reach the next document")
		assert.Contains(t, herr.Error(), "tag handle !e! is not defined")
	})

	t.Run("each document declaring its own reads under it", func(t *testing.T) {
		const both = "%YAML 1.1\n---\na: yes\n...\n%YAML 1.1\n---\nb: yes\n"

		wellFormed(t, both)
		assert.Equal(t, []any{map[string]any{"a": true}, map[string]any{"b": true}}, readTheStream(t, both))
	})
}

// TestFixedAMergeSequenceSharingAKeyReadsEverywhere closes the split.
//
// Two mappings in a merge sequence are expected to share keys -- that is what
// the earlier-wins rule of the 1.1 merge type is for, and the sequence has no
// other purpose. The walk applied it and a typed map refused the document,
// applying 3.2.1.1's uniqueness across mappings that are not one mapping.
//
// Decoder.decodeMap asked validateDuplicateKey for every key of the fold
// getMapNode makes of the sequence. It skips a repeat inside a fold now, so the
// earlier mapping wins there as it does on every other path.
//
// Filed by the peer session as defect 40 from hand-written shapes; reached by
// the merge axis on 2026-09-07 and closed the same day.
func TestFixedAMergeSequenceSharingAKeyReadsEverywhere(t *testing.T) {
	// Read under YAML 1.1, where "<<" is the merge key. It is a 1.1 type, so
	// under the core schema these documents hold a key named "<<" and merge
	// nothing.
	//
	// The other key is "w" and not "y": 1.1 resolves "y" to the boolean true,
	// so a test that switches version to reach the merge has to keep clear of
	// 1.1's other spellings.
	t.Run("a shared key reads the same into a typed map and an any", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n<<: [{x: 1}, {x: 2}]\nw: 3\n"

		want := map[string]any{"x": uint64(1), "w": uint64(3)}

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, want, walked, "the earlier mapping wins, which is the 1.1 rule")

		var typed map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &typed))
		assert.Equal(t, want, typed, "and the typed map agrees now")
	})

	t.Run("sharing no key, every destination still reads it", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n<<: [{x: 1}, {z: 2}]\nw: 3\n"

		want := map[string]any{"x": uint64(1), "w": uint64(3), "z": uint64(2)}

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, want, walked)

		var typed map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &typed))
		assert.Equal(t, want, typed)
	})

	t.Run("and a key repeated in one mapping is still refused", func(t *testing.T) {
		// The fold is what carries the earlier-wins rule. One mapping writing
		// a key twice is 3.2.1.1's repeat and has nothing to do with it.
		var typed map[string]any
		err := codec.Unmarshal([]byte("%YAML 1.1\n---\n<<: {x: 1, x: 2}\n"), &typed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"x" already defined`)
	})
}

// TestFixedAMergeKeyIsAnOrdinaryKeyUnderYAML12: with no "%YAML 1.1" directive,
// "<<" is a key spelled "<<" and nothing merges.
//
// tag:yaml.org,2002:merge is a YAML 1.1 type. 1.2 dropped it and left "<<" a
// plain scalar like any other, so the document holds a two-character key --
// which is how libfyaml 1.0.0b1 reads it in its own 1.2 mode. 8acf11b is where
// this library started saying so; before it, every document merged.
//
// This is the half of the ruling that costs a caller something, so it is
// pinned over the whole family: the value written as an alias, as a mapping in
// place, as a sequence, as a scalar and as nothing at all, in block and in
// flow, over both decode paths. A merge under the core schema would show up
// here as a key going missing.
func TestFixedAMergeKeyIsAnOrdinaryKeyUnderYAML12(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want map[string]any
	}{
		{
			name: "an alias to a mapping",
			src:  "base: &b {a: 1}\nd:\n  <<: *b\n  c: 2\n",
			want: map[string]any{
				"base": map[string]any{"a": uint64(1)},
				"d":    map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)},
			},
		},
		{
			name: "a mapping written in place",
			src:  "d:\n  <<: {a: 1}\n  c: 2\n",
			want: map[string]any{"d": map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)}},
		},
		{
			name: "the same mapping in flow",
			src:  "d: {<<: {a: 1}, c: 2}\n",
			want: map[string]any{"d": map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)}},
		},
		{
			// The key the merge would have brought in stays under "<<" and the
			// mapping's own "a" keeps its own value, so nothing collides. Under
			// 1.1 this document is the precedence rule and reads {"a": 9}.
			name: "a key the merge would have overridden",
			src:  "base: &b {a: 1}\nd:\n  <<: *b\n  a: 9\n",
			want: map[string]any{
				"base": map[string]any{"a": uint64(1)},
				"d":    map[string]any{"<<": map[string]any{"a": uint64(1)}, "a": uint64(9)},
			},
		},
		{
			name: "a sequence of mappings",
			src:  "d:\n  <<: [{a: 1}, {b: 2}]\n",
			want: map[string]any{"d": map[string]any{
				"<<": []any{map[string]any{"a": uint64(1)}, map[string]any{"b": uint64(2)}},
			}},
		},
		{
			// Refused under 1.1 -- "int was used where mapping is expected" --
			// and read without complaint here, which is the verdict half of the
			// ruling rather than the value half. See yamlcorpus.TagMergeNonMapping.
			name: "a scalar, which 1.1 has no merge for",
			src:  "<<: 1\n",
			want: map[string]any{"<<": uint64(1)},
		},
		{
			name: "no value at all",
			src:  "<<:\n",
			want: map[string]any{"<<": nil},
		},
		{
			name: "the long spelling, which reads the same",
			src:  "? <<\n: {a: 1}\nc: 2\n",
			want: map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, src := range []string{tc.src, "%YAML 1.2\n---\n" + tc.src} {
				var walked any
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &walked), "%q", src)
				assert.Equalf(t, tc.want, walked, "the walk: %q", src)

				var typed map[string]any
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
				assert.Equalf(t, tc.want, typed, "the tree: %q", src)
			}
		})
	}

	t.Run("a quoted merge key reads the same as a bare one", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("\"<<\": {a: 1}\nc: 2\n"), &got))
		assert.Equal(t, map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)}, got)
	})

	t.Run("a %YAML 1.2 directive overrides WithYAMLVersion(YAML11)", func(t *testing.T) {
		// The directive wins over the option in both directions, which is
		// libfyaml's model and the rule 8acf11b took. Without the directive the
		// same option merges -- see TestFixedAMergeKeyResolvesUnderYAML11.
		const src = "%YAML 1.2\n---\nd:\n  <<: {a: 1}\n  c: 2\n"

		var got any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &got,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, map[string]any{
			"d": map[string]any{"<<": map[string]any{"a": uint64(1)}, "c": uint64(2)},
		}, got)
	})
}

// TestFixedAMergeKeyResolvesUnderYAML11: under "%YAML 1.1" a "<<" entry merges,
// and a "<<" given something that is not a mapping is refused.
//
// The other half of 8acf11b's ruling. Three things reach the merge: the
// directive, parser.WithYAMLVersion(YAML11) on a document that declares no
// version, and a written "!!merge", which names the type outright and so does
// not depend on the version at all.
//
// The refusals are the part a value comparison would miss. A merging reader has
// to *reject* "<<: 1", since there is no operation for merging a scalar, so the
// same bytes are a valid document under 1.2 and an error under 1.1 -- one
// document, two verdicts, which is what yamlcorpus.TagMergeNonMapping carries.
func TestFixedAMergeKeyResolvesUnderYAML11(t *testing.T) {
	t.Run("the directive merges", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			src  string
			want map[string]any
		}{
			{
				name: "an alias to a mapping",
				src:  "base: &b {a: 1}\nd:\n  <<: *b\n  c: 2\n",
				want: map[string]any{
					"base": map[string]any{"a": uint64(1)},
					"d":    map[string]any{"a": uint64(1), "c": uint64(2)},
				},
			},
			{
				name: "a mapping written in place",
				src:  "d:\n  <<: {a: 1}\n  c: 2\n",
				want: map[string]any{"d": map[string]any{"a": uint64(1), "c": uint64(2)}},
			},
			{
				// 1.1 says an entry's own keys win over the ones a "<<" brings.
				name: "the mapping's own key winning",
				src:  "base: &b {a: 1}\nd:\n  <<: *b\n  a: 9\n",
				want: map[string]any{
					"base": map[string]any{"a": uint64(1)},
					"d":    map[string]any{"a": uint64(9)},
				},
			},
			{
				// And among the merged, the earlier wins: "a" comes from *x.
				name: "a sequence of mappings, the earlier winning",
				src:  "x: &x {a: 1}\nz: &z {a: 2, b: 3}\nd:\n  <<: [*x, *z]\n",
				want: map[string]any{
					"x": map[string]any{"a": uint64(1)},
					"z": map[string]any{"a": uint64(2), "b": uint64(3)},
					"d": map[string]any{"a": uint64(1), "b": uint64(3)},
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				src := "%YAML 1.1\n---\n" + tc.src

				var walked any
				require.NoError(t, codec.Unmarshal([]byte(src), &walked))
				assert.Equal(t, tc.want, walked, "the walk")

				var typed map[string]any
				require.NoError(t, codec.Unmarshal([]byte(src), &typed))
				assert.Equal(t, tc.want, typed, "the tree")
			})
		}
	})

	t.Run("WithYAMLVersion(YAML11) merges a document that declares nothing", func(t *testing.T) {
		var got any
		require.NoError(t, codec.UnmarshalWithOptions([]byte("d:\n  <<: {a: 1}\n  c: 2\n"), &got,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, map[string]any{"d": map[string]any{"a": uint64(1), "c": uint64(2)}}, got)
	})

	t.Run("a written !!merge tag merges under any version", func(t *testing.T) {
		// The tag names tag:yaml.org,2002:merge itself, and a written tag is
		// not tied to a spec version -- ast.TagNode.IsMergeKey reads the URI.
		for _, src := range []string{
			"!!merge <<: {a: 1}\nc: 2\n",
			"%YAML 1.2\n---\n!!merge <<: {a: 1}\nc: 2\n",
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"a": uint64(1), "c": uint64(2)}, got, "%q", src)
		}
	})

	t.Run("!!merge on anything but a merge key is refused", func(t *testing.T) {
		for _, src := range []string{
			"!!merge k: {a: 1}\n",
			"!!merge \"<<\": {a: 1}\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "could not find merge key", "%q", src)
		}
	})

	t.Run("merging something that is not a mapping is refused", func(t *testing.T) {
		// The message names the type that stood where a mapping was wanted, and
		// the walk and the tree point at different columns of the same
		// document, so only the sentence is asserted.
		//
		// "<<:" with no value is missing from this list on purpose: the walk
		// reads it and the tree refuses it, which is
		// TestDefectMergingNullIsReadByTheWalkAndRefusedByTheTree.
		for _, src := range []string{
			"<<: 1\n",
			"<<: x\n",
			"<<: [x]\n",
			"<<: [[x]]\n",
			"<<: [{a: 1}, [x]]\n",
		} {
			full := "%YAML 1.1\n---\n" + src

			var walked any
			werr := codec.Unmarshal([]byte(full), &walked)
			require.Errorf(t, werr, "the walk reads %q", src)
			assert.Containsf(t, werr.Error(), "where mapping is expected", "%q", src)

			var typed map[string]any
			terr := codec.Unmarshal([]byte(full), &typed)
			require.Errorf(t, terr, "the tree reads %q", src)
			assert.Containsf(t, terr.Error(), "where mapping is expected", "%q", src)

			// And the same bytes under the core schema are an ordinary key
			// holding an ordinary value.
			var under12 any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &under12), "%q", src)
		}
	})
}

// TestFixedATabSeparatesAsASpaceDoes: a tab between a node's properties and the
// node ends the property, exactly as a space does.
//
// 6.1 puts a tab in s-white and s-separate-in-line is s-white+, so a tab ends an
// anchor, an alias and a tag shorthand. Only the space did until `a0182a6`:
// `a: !!str<TAB>x` was refused as `found invalid tag character`, and
// `a: &n<TAB>x` cut the anchor as "nx" on the empty node, so the value was gone
// and a later `*n` named nothing.
//
// Reached by Style.TabSeparation, which was built to close the census gap that
// 56 YAML Test Suite documents hold a tab and no generated document did. Neither
// corpus writes a tab after a property, so nothing else could have found it.
func TestFixedATabSeparatesAsASpaceDoes(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want map[string]any
	}{
		{"a: !!str\tx\n", map[string]any{"a": "x"}},
		{"a: &n\tx\n", map[string]any{"a": "x"}},
		{"a: &n\tx\nb: *n\n", map[string]any{"a": "x", "b": "x"}},
		{"a: !!str x\n", map[string]any{"a": "x"}},
		{"a:\tx\n", map[string]any{"a": "x"}},
	} {
		var got map[string]any
		require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
		assert.Equalf(t, tc.want, got, "%q", tc.src)
	}
}

// TestFixedATabOpensABlockScalarAfterAnAnchor: an anchor separated from a block
// scalar by a tab takes the header, so the content is block content.
//
// `&a1<TAB>>-` over ` , a` was refused as `a plain scalar cannot begin with ","`
// until `a4b40c1` -- the header joined the anchor's name, so the token stream
// held Anchor and String "a1>-" where a space gives Anchor, String "a1", Folded
// ">-". A block scalar's content is not a plain scalar and may begin with
// anything.
//
// Older than the commit that made it visible: `a0182a6` made a tab end a
// property and fixed the tag spelling, and this one waited on the anchor path,
// which asks the question inside the second of the tab branch's two indentation
// tests. At the root the first test applies, so it was never asked.
//
// The three neighbors are the controls: change the tab for a space, the anchor
// for a tag, or the content for something a plain scalar could begin, and each
// read before the fix.
func TestFixedATabOpensABlockScalarAfterAnAnchor(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"&a1\t>-\n , a\n", ", a"},
		{"&a1\t>-\n  , a\n", ", a"},
		{"&a1 >-\n , a\n", ", a"},
		{"!!str\t>-\n , a\n", ", a"},
		{"&a1\t>-\n x\n", "x"},
	} {
		var got any
		require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
		assert.Equalf(t, tc.want, got, "%q", tc.src)
	}
}

// TestFixedTwoCollectionKeysAreTwoKeys: two collections used as keys in one
// mapping are two keys.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node, and two
// different mappings do not. Until `913fb19` the duplicate check named a
// collection key by its opening character, so every collection key in a mapping
// was the same key as every other and the second was refused as a duplicate.
//
// The empty key is the neighbor worth keeping: a key written empty is a real
// key and a repeat of it is still a duplicate, so the fix had to skip the
// collection rather than skip a missing name.
func TestFixedTwoCollectionKeysAreTwoKeys(t *testing.T) {
	t.Run("two collection keys read as two", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(`{{"": 0}: a, {"": 1}: b}`+"\n"), &got))
		assert.Equal(t, map[string]any{"map[:0]": "a", "map[:1]": "b"}, got)
	})

	t.Run("a repeated empty key is still a duplicate", func(t *testing.T) {
		var got any
		err := codec.Unmarshal([]byte(": a\n: b\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already defined")
	})
}

// TestFixedARepeatedCollectionKeyIsRefused: a collection key written twice in
// one mapping is a duplicate.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node, and two
// mappings spelled alike do. `913fb19` stopped naming every collection key by
// its opening character -- which had made every one collide with every other --
// and stopped checking any of them: `{{a: 0}: 1, {a: 0}: 2}` read as
// {map[a:0]: 2}, one entry short and nothing reported. `e842b79` names a
// collection key by the source text between its first and last token, which
// restores the check.
//
// Both spellings and both containers are asserted, because the two arrived by
// different routes: parseMapKeyValueNode's explicit branch returned before the
// duplicate check for anything that is not a scalar, so `? [a]` over `: 1`
// twice read as two entries while `{[a]: 1, [a]: 2}` was already refused.
//
// Naming by the source spelling then missed a repeat written two ways, which
// cost an entry rather than a refusal; [ast.KeyIdentity] replaced it -- see
// TestFixedACollectionKeyIsNamedByWhatItResolvesTo.
func TestFixedARepeatedCollectionKeyIsRefused(t *testing.T) {
	t.Run("eight spellings of the repeat are refused", func(t *testing.T) {
		for _, src := range []string{
			`{{a: 0}: 1, {a: 0}: 2}` + "\n",
			`{[""]: 1, [""]: 2}` + "\n",
			"{[a]: 1, [a]: 2}\n",
			"{[a, b]: 1, [a, b]: 2}\n",
			"? [a]\n: 1\n? [a]\n: 2\n",
			"? {a: 0}\n: 1\n? {a: 0}\n: 2\n",
			"[a]: 1\n[a]: 2\n",
			// Mixed: the explicit spelling over the implicit one. This is the
			// pair the explicit branch's early return let through.
			"? [a]\n: 1\n[a]: 2\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "already defined", "%q", src)
		}
	})

	// The other direction, and the reason the fix is not simply "refuse two
	// collection keys": collections that differ are different keys, and a
	// mapping keyed by several of them is an ordinary document. The last is
	// spec example 2.11.
	t.Run("collections that differ are still two keys", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{`{{a: 0}: 1, {a: 1}: 2}` + "\n", map[string]any{"map[a:0]": uint64(1), "map[a:1]": uint64(2)}},
			{"{[a]: 1, [b]: 2}\n", map[string]any{"[a]": uint64(1), "[b]": uint64(2)}},
			{"{[a]: 1, [a, b]: 2}\n", map[string]any{"[a]": uint64(1), "[a b]": uint64(2)}},
			{`{{"": 0}: a, {"": 1}: b}` + "\n", map[string]any{"map[:0]": "a", "map[:1]": "b"}},
			{
				"? - Detroit Tigers\n  - Chicago cubs\n: - 2001-07-23\n" +
					"? [ New York Yankees,\n    Atlanta Braves ]\n" +
					": [ 2001-07-02, 2001-08-12,\n    2001-08-14 ]\n",
				map[string]any{
					"[Detroit Tigers Chicago cubs]":     []any{"2001-07-23"},
					"[New York Yankees Atlanta Braves]": []any{"2001-07-02", "2001-08-12", "2001-08-14"},
				},
			},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestFixedACollectionKeyIsNamedByWhatItResolvesTo: a key too big for one token
// is told apart by the node it builds, not by the characters it was written
// with.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node. `e842b79`
// named a collection key by the source text between its first and last token,
// which reads `[a]` and `[ a ]` as two keys where the document holds one --
// `{[a]: 1, [ a ]: 2}` read as {"[a]": 2}, one entry short and nothing
// reported. The same span reaches only the first indicator of a block
// collection, so `? - a` over `: 1` over `? - b` over `: 2` was refused with
// `mapping key "-" already defined`, making every block collection key in a
// mapping the same key as every other.
//
// [ast.KeyIdentity] replaces the spelling: a scalar is its type and that type's
// canonical spelling, a collection its kind and the identities of its members
// in order. The parser records it when the entry is built, so `[a]`, `[ a ]`,
// `[a,]` and `["a"]` come out one key and `- a` and `- b` come out two.
//
// Both decode paths are asserted at every shape. The walk hands a collection's
// members over instead of keeping them, which left a key node empty and
// unnameable; inside a key the members are kept -- see Parser.readingKey -- so
// the walk names the key by the same tree the load does, down to the position
// in the message.
//
// ⚠️ No oracle can confirm any of this. libfyaml 1.0.0b1 cannot hash a
// collection key and dies with a Python traceback; go.yaml.in/yaml/v3 v3.0.5
// refuses one as "invalid map key"; perlref answers syntax and not meaning. So
// 3.2.1.1 is the only judge here, and yamlcorpus lists "two collection keys in
// one mapping" among its uncorroborated meanings for the same reason. These
// expectations are a reading of one sentence, not a measurement of the field --
// which is what to weigh against if one of them ever has to move.
func TestFixedACollectionKeyIsNamedByWhatItResolvesTo(t *testing.T) {
	t.Run("two spellings of one key are one key", func(t *testing.T) {
		for _, tc := range []struct{ src, says string }{
			// Whitespace alone, in flow and in block.
			{"{[a]: 1, [ a ]: 2}\n", `[1:10] mapping key "[a]" already defined at [1:2]`},
			{"[a]: 1\n[ a ]: 2\n", `[2:1] mapping key "[a]" already defined at [1:1]`},
			{"? [a]\n: 1\n? [ a ]\n: 2\n", `[3:1] mapping key "[a]" already defined at [1:1]`},
			// A trailing comma, which 7.4 allows and which changes nothing.
			{"{[a]: 1, [a,]: 2}\n", `[1:10] mapping key "[a]" already defined at [1:2]`},
			// Quoting, which settles presentation and not the node.
			{`{[a]: 1, ["a"]: 2}` + "\n", `[1:10] mapping key "[\"a\"]" already defined at [1:2]`},
			{`{['a']: 1, ["a"]: 2}` + "\n", `[1:12] mapping key "[\"a\"]" already defined at [1:2]`},
			// A tag that agrees with what the scalar already resolves to.
			{"{[1]: x, [!!int 1]: y}\n", `[1:10] mapping key "[!!int 1]" already defined at [1:2]`},
			// A mapping key, where the space is inside the entry.
			{"{{a: 0}: 1, {a: 0 }: 2}\n", `[1:13] mapping key "{a: 0}" already defined at [1:2]`},
		} {
			assertBothPathsSay(t, tc.src, tc.says)
		}
	})

	t.Run("two block collection keys are two keys", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{"? - a\n: 1\n? - b\n: 2\n", map[string]any{"[a]": uint64(1), "[b]": uint64(2)}},
			{"?\n  a: 0\n: 1\n?\n  b: 0\n: 2\n", map[string]any{"map[a:0]": uint64(1), "map[b:0]": uint64(2)}},
			// A block key beside a flow key, which the two namings used to tell
			// apart by accident.
			{"? - a\n: 1\n? [b]\n: 2\n", map[string]any{"[a]": uint64(1), "[b]": uint64(2)}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})

	// A block scalar is a string whatever it spells, and its own token is the
	// header. Named by that header every one of them was "|-", so two collided;
	// named by the string it folds to, "? |-" over "  a" is the plain key "a"
	// and meets it in the same store.
	t.Run("a block scalar key is the string it folds to", func(t *testing.T) {
		for _, tc := range []struct{ src, says string }{
			{"? |-\n  a\n: 1\n? |-\n  a\n: 2\n", `[4:1] mapping key "a" already defined at [1:1]`},
			{"? |-\n  a\n: 1\n? >-\n  a\n: 2\n", `[4:1] mapping key "a" already defined at [1:1]`},
			{"? |-\n  a\n: 1\na: 2\n", `[4:1] mapping key "a" already defined at [1:1]`},
			{"a: 1\n? |-\n  a\n: 2\n", `[2:1] mapping key "a" already defined at [1:1]`},
		} {
			assertBothPathsSay(t, tc.src, tc.says)
		}

		// Two block scalars that fold differently are two keys, and the
		// chomping indicator is enough: "|" keeps the trailing break that "|-"
		// strips, so "a\n" and "a" are two strings.
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{"? |-\n  a\n: 1\n? |-\n  b\n: 2\n", map[string]any{"a": uint64(1), "b": uint64(2)}},
			{"? |\n  a\n: 1\n? |-\n  a\n: 2\n", map[string]any{"a\n": uint64(1), "a": uint64(2)}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// assertBothPathsSay checks that decoding src is refused with says, by the walk
// and by the tree alike.
//
// codec.Unmarshal walks the document and codec.UseOrderedMap loads the tree.
// The two used to name a built key differently, so a document one refused the
// other read one entry short; asserting the whole message, positions included,
// is what holds them together.
func assertBothPathsSay(t *testing.T, src, says string) {
	t.Helper()

	var walked any
	err := codec.Unmarshal([]byte(src), &walked)
	require.Errorf(t, err, "the walk read %q as %v", src, walked)
	assert.Containsf(t, err.Error(), says, "the walk on %q", src)

	var loaded any
	err = codec.UnmarshalWithOptions([]byte(src), &loaded, codec.UseOrderedMap())
	require.Errorf(t, err, "the tree read %q as %v", src, loaded)
	assert.Containsf(t, err.Error(), says, "the tree on %q", src)
}

// TestFixedAnAliasKeyIsTheNodeItsAnchorNames: an alias standing as a mapping
// key repeats the key its anchor named.
//
// §3.2.2.2 makes an alias node the anchored node rather than a copy of it, so
// `&a x` and a later `*a` are one node used twice and §3.2.1.1 refuses the
// second. `{&a x: 1, *a : 2}` read `{x: 2}` before this, dropping the `1`
// silently -- the check skipped every alias key, because [ast.KeyIdentity]
// follows [ast.AliasNode.Target] and a walk scrubs the anchored node once the
// entry holding it closes: `&a [a, b]` read back as `seq()`.
//
// Parser.keepAnchorIdentity takes the identity where the node is still whole,
// which Parser.keepsNothing arranges by holding a walk's cells while an anchor
// is being read. One string per anchor outlives the node.
//
// The YAML Test Suite's aliases-in-flow-objects is this shape and is now
// refused at the load. The suite scores the event stream, where the document is
// legal; key uniqueness is a rule of the load step, so the parse still reads it
// and records the repeat on the mapping.
func TestFixedAnAliasKeyIsTheNodeItsAnchorNames(t *testing.T) {
	t.Run("an alias repeats the key its anchor named", func(t *testing.T) {
		for _, tc := range []struct{ src, says string }{
			// A scalar anchor is recorded among the scalar keys, which name a
			// key by what it resolves to -- as they do for "!!str a" -- so the
			// message reads "x" rather than "*a". A collection anchor is named
			// by the alias the document wrote: its node is gone by then, and
			// the alias is what a reader can find in the source.
			{"{&a x: 1, *a : 2}\n", `[1:11] mapping key "x" already defined at [1:2]`},
			{"a: &x s\n*x : p\n*x : q\n", `[3:1] mapping key "s" already defined at [2:1]`},
			{"a: &x [1,2]\n*x : p\n*x : q\n", `[3:1] mapping key "*x" already defined at [2:1]`},
			// The alias beside the collection written out: one node, two
			// spellings, and the anchor sits on the streaming path where the
			// walk keeps no children of its own.
			{"a: &x [1,2]\n[1, 2]: p\n*x : q\n", `[3:1] mapping key "*x" already defined at [2:1]`},
			{"{ &a [a, &b b]: *b, *a : [c, *b, d]}\n", `[1:21] mapping key "*a" already defined at [1:3]`},
			// The alias repeating a key written out further down, rather than
			// the anchor it names.
			{"k: &a1 n\n*a1 : 1\nn: 2\n", `[3:1] mapping key "n" already defined at [2:1]`},
			{"{&a1 x: 1, *a1 : 2}\n", `[1:12] mapping key "x" already defined at [1:2]`},
			// Two faults met here, and the second hid the first. A float key
			// behind a property is named by its canonical YAML spelling on the
			// tree and by Go's %v on the walk, so the tree held ".inf" and
			// "+Inf" as two keys and never saw a repeat to refuse; the walk
			// named both "+Inf" and kept the last. The identity is taken from
			// the anchored node, which spells it ".inf" on either path.
			{"&a1 .inf: 1\n*a1 : 2\n", `[2:1] mapping key ".inf" already defined at [1:1]`},
			// The control that puts it on the alias: an anchor beside a
			// written-out key was always refused.
			{"&a1 .inf: 1\n.inf: 2\n", `[2:1] mapping key ".inf" already defined at [1:1]`},
			{".inf: 1\n.inf: 2\n", `[2:1] mapping key ".inf" already defined at [1:1]`},
		} {
			assertBothPathsSay(t, tc.src, tc.says)
		}
	})

	// The other direction, and the one the generated corpus never drew: two
	// anchors naming different collections. Naming an alias key from a node the
	// walk had already scrubbed made both of them "seq()", so a valid document
	// was refused as a repeat -- inventing a duplicate rather than missing one.
	t.Run("aliases to different anchors are different keys", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{"a: &x [1,2]\nb: &y [3,4]\n*x : p\n*y : q\n",
				map[string]any{"a": []any{uint64(1), uint64(2)}, "b": []any{uint64(3), uint64(4)}, "[1 2]": "p", "[3 4]": "q"}},
			{"a: &x {k: 1}\nb: &y {k: 2}\n*x : p\n*y : q\n",
				map[string]any{"a": map[string]any{"k": uint64(1)}, "b": map[string]any{"k": uint64(2)}, "map[k:1]": "p", "map[k:2]": "q"}},
			{"a: &x s\nb: &y t\n*x : p\n*y : q\n",
				map[string]any{"a": "s", "b": "t", "s": "p", "t": "q"}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestFixedAnAnchoredFloatKeyKeepsItsSpelling: an anchor in front of a float
// key no longer changes the name the key gets.
//
// A float key is named by its canonical spelling, and the ".0" keeps it out of
// the integers' namespace: "1.0: x" comes back keyed "1.0". An anchor on the
// same key gave "1", and the two decode paths disagreed about it -- reading
// into an `any` gave "1" and into a map[string]any "1.0", so the destination
// decided the key.
//
// parseAnchor attached the node to ast.AnchorNode.Value after readAnchorValue
// returned, and readAnchorValue fires a walk's Leave on its way out, so a
// walking reader was handed the anchor with Value still nil and the name fell
// through to fmt.Sprint of the float. readAnchorValue attaches it first now.
//
// Reached on 2026-09-08 by TestRenderPreservesValue, on the first run after the
// aliaser began anchoring keys.
//
// ⚠️ A tag is a separate fault and is open: yamlcorpus.Departures records it as
// "a key tagged !!float". unwrapKeyNode does not unwrap an ast.TagNode, so
// "!!float 1.0: x" is named "1" on both paths -- and "!!float 1: a" over
// "1: b" reads {"1": "b"}, two keys the parser tells apart collapsed into one
// Go map entry with nothing reported. Naming a tagged key wants the tag
// resolved rather than stripped, since "!!float 1" is the float 1.0.
func TestFixedAnAnchoredFloatKeyKeepsItsSpelling(t *testing.T) {
	t.Run("a bare float key keeps its spelling", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{src: "1.0: x\n", key: "1.0"},
			{src: "1e3: x\n", key: "1000.0"},
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "x"}, got, "%q", tc.src)
		}
	})

	t.Run("an anchored float key keeps it, on both decode paths", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{src: "&a1 1.0: x\n", key: "1.0"},
			{src: "{&a1 1.0: x}\n", key: "1.0"},
			{src: "&a1 1e3: x\n", key: "1000.0"},
		} {
			var walked any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &walked), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "x"}, walked, "the walk: %q", tc.src)

			var tree map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &tree), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "x"}, tree, "the tree: %q", tc.src)
		}
	})

	t.Run("and every adjacent key is named correctly", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{src: "&a1 7: x\n", key: "7"},
			{src: "&a1 true: x\n", key: "true"},
			{src: "&a1 1.5: x\n", key: "1.5"},
			// The long form keeps the spelling, which places the fault on the
			// implicit key.
			{src: "? &a1 1.0\n: x\n", key: "1.0"},
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "x"}, got, "%q", tc.src)
		}
	})
}

// TestFixedATagOnAKeyReachesAStructFieldAndKeepsItsType: one node-to-name walk,
// read by every consumer.
//
// codec walked from a node down to the scalar a key is named after in three
// places -- unwrapKeyNode for the decoder, another for the reflection path, and
// tojson naming the node with no unwrapping at all -- and only one of them
// looked through an anchor. None looked through a tag. So `!!str x: 1` left a
// struct field tagged `x` at its zero value with nothing reported, and
// `!!float 226.0: x` came back keyed "226", a float in the integers' namespace.
//
// [ast.KeyName] owns that walk now and all of them call it. It resolves a tag
// rather than stepping over it, which is the part that matters: `!!float 1` is
// the float 1.0 and is named "1.0", where stripping the tag reads the token "1"
// and names it after an integer.
//
// Retiring three yamlcorpus departures at once -- "a key tagged !!float",
// "!!timestamp" and "!!binary" -- is the argument for owning the rule in one
// place. Each had been found separately, by a different consumer.
func TestFixedATagOnAKeyReachesAStructFieldAndKeepsItsType(t *testing.T) {
	t.Run("a tagged key fills the field it names", func(t *testing.T) {
		type target struct {
			X any `yaml:"x"`
		}

		for _, src := range []string{
			"!!str x: 1\n",
			"!!str \"x\": 1\n",
			"? !!str x\n: 1\n",
			"&a1 !!str x: 1\n",
			"!foo x: 1\n",
			"! x: 1\n",
		} {
			var got target
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, uint64(1), got.X, "%q", src)
		}
	})

	t.Run("a tag is resolved and not stripped", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			// Stripping would read the token and name these after an integer.
			{"!!float 1: x\n", "1.0"},
			{"!!float 226: x\n", "226.0"},
			// Writing the tag changes nothing where it agrees with the scalar.
			{"!!float 1.0: x\n", "1.0"},
			{"!!float 226.0: x\n", "226.0"},
			{"!!int 226: x\n", "226"},
			{"!!str 226.0: x\n", "226.0"},
			{"!!bool True: x\n", "true"},
			{"!!null ~: x\n", "null"},
			// A tag naming a kind leaves the node to speak for itself, so the
			// key is the text the document wrote.
			{"!!timestamp 2001-12-14: x\n", "2001-12-14"},
			{"!!binary aGVsbG8=: x\n", "aGVsbG8="},
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "x"}, got, "%q", tc.src)
		}

		// The two destinations name a byte string alike. They did not: a
		// map[string]any was keyed on the resolved value, and a []byte is not
		// comparable, so the document was refused where the same bytes read
		// into an any gave {"aGVsbG8=": "x"}. Neither destination has to hold
		// the []byte -- both are keyed by a string.
		var walked any
		require.NoError(t, codec.Unmarshal([]byte("!!binary aGVsbG8=: x\n"), &walked))
		assert.Equal(t, map[string]any{"aGVsbG8=": "x"}, walked)

		// A collection key is still refused into a string-keyed map, and
		// should be: its only name is the spelling Go prints.
		for _, src := range []string{"? [a, b]\n: v\n", "? {x: 1}\n: v\n"} {
			var typed map[string]any
			err := codec.Unmarshal([]byte(src), &typed)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "as a map key", "%q", src)
		}
	})

	// The three consumers agree on one document, which is what the shared walk
	// buys. ToJSON still writes a binary key as the decoded bytes, which is
	// defect 30 and is about what a byte string means in JSON, not about the
	// naming.
	t.Run("the decoder and ToJSON name a tagged float alike", func(t *testing.T) {
		const src = "!!float 226: x\n"

		var got map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"226.0": "x"}, got)

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.JSONEq(t, `{"226.0":"x"}`, string(out))
	})
}

// TestFixedAnAliasKeyIsNamedAsTheNodeItsAnchorNamed closes the last of the
// property-in-front-of-a-key naming.
//
// A float key is named by its canonical YAML spelling, and the ".0" keeps it
// out of the integers' namespace. An alias in front of the same key made the
// name Go's %v instead -- "*a1" naming an anchor on ".inf" came back "+Inf" --
// and the alias never reached a struct field, since no field is tagged with
// what Go's %v writes.
//
// 3.2.2.2 makes an alias node the node its anchor named, so it is named as that
// node. [ast.KeyName] reads [ast.AliasNode.Target] to do it, and every consumer
// of the name gets it at once: the decoder, the reflection path and ToJSON all
// read that one walk.
//
// ⚠️ Target became safe to read only on 2026-09-08. A walk hands an anchored
// node's cells out again once its entry closes, so Target held another part of
// the document -- an anchor on "1" read "499" with 500 entries in between, and
// the corruption was per-arena-block, so a string anchor could survive where an
// integer one did not. ast.Arena.Commit holds those cells for the document now.
// Before it, this arm handed back nothing rather than a wrong answer, which is
// why the fix waited.
//
// [ast.KeyIdentity] still does not read Target and must not: an identity writes
// the whole structure out, so following an alias expands the alias graph, which
// is exponential in a document's width. A name is one scalar, so this cannot.
func TestFixedAnAliasKeyIsNamedAsTheNodeItsAnchorNamed(t *testing.T) {
	t.Run("an alias key keeps the spelling its anchor had", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{"a: &a1 .inf\n*a1 : v\n", ".inf"},
			{"a: &a1 1.0\n*a1 : v\n", "1.0"},
			{"a: &a1 1e3\n*a1 : v\n", "1000.0"},
			// The long form too, which used not to save it.
			{"a: &a1 .inf\n? *a1\n: v\n", ".inf"},
		} {
			var walked any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &walked), "%q", tc.src)
			assert.Containsf(t, walked, tc.key, "the walk: %q", tc.src)

			var tree map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &tree), "%q", tc.src)
			assert.Containsf(t, tree, tc.key, "the tree: %q", tc.src)
		}
	})

	t.Run("an alias key reaches the struct field it names", func(t *testing.T) {
		type named struct {
			N any `yaml:"n"`
		}

		const src = "k: &a1 n\n*a1 : 1\n"

		var got named
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, uint64(1), got.N, "the alias resolves to the key \"n\"")

		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, map[string]any{"k": "n", "n": uint64(1)}, walked)
	})
}

// TestFixedAnExplicitKeyInsideAnExplicitKeyReads holds the nesting open.
//
// 8.2.2 puts an explicit entry's key at s-l+block-indented(n, block-out), which
// is any block node -- a mapping written the long way included. Nesting the two
// "?" was refused with "unexpected scalar value type", and in flow with "could
// not find flow map content".
//
// groupExplicitKeyBody ran the mapping passes over the body and not the
// explicit-key one, so a "?" inside the body stayed a bare indicator and the
// parser met it where a node belongs. groupExplicitKeysIn is that pass over a
// slice, and it recurses: "? ? ? a" nests three deep.
//
// Reached on 2026-09-07, when Keys began drawing a collection. No generated
// document held a collection key before that, and the census reports the YAML
// Test Suite holds no nested explicit key either, so nothing on either side had
// provoked it.
func TestFixedAnExplicitKeyInsideAnExplicitKeyReads(t *testing.T) {
	t.Run("the nesting reads, in block and in flow", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"?\n  ? a\n  : 0\n: v\n", map[string]any{"map[a:0]": "v"}},
			{"? ? a\n  : 1\n: 2\n", map[string]any{"map[a:1]": uint64(2)}},
			{"? {? a: 1}\n: v\n", map[string]any{"map[a:1]": "v"}},
			{"{? {? a: 1}: v}\n", map[string]any{"map[a:1]": "v"}},
			{"? ? ? a\n", map[string]any{"map[map[a:<nil>]:<nil>]": nil}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})

	t.Run("the same key written any other way reads", func(t *testing.T) {
		for _, tc := range []struct{ src, key string }{
			{"?\n  a: 0\n: v\n", "map[a:0]"},
			{"? {a: 0}\n: v\n", "map[a:0]"},
			{"?\n  - a\n  - b\n: v\n", "[a b]"},
		} {
			// Into an `any`, which names the key by walking it. A typed map
			// cannot hold one: `cannot use map[string]interface {} as a map
			// key: it is not comparable`.
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, map[string]any{tc.key: "v"}, got, "%q", tc.src)
		}
	})
}

// TestFixedASecondCommentOnAnExplicitKeysColonLineIsKept: a comment on the ":"
// line of the long form and a head comment under it, and both survive.
//
// The ":" line comment went on the value, and the value begins on a later line,
// so setLineComment degraded it to a head comment and it took the slot the head
// comment written under it needed: "? a" over ": # c4" over "  # c5" over
// "  - 1" kept c4 and lost c5, with nothing reported. On the entry it stays a
// line comment and both slots are free.
//
// The evidence is that the two spellings of one document now give the same
// comment map. They did not: the long form addressed the ":" line comment at
// $.a[0] as a head comment where the short form gives $.a a line comment.
func TestFixedASecondCommentOnAnExplicitKeysColonLineIsKept(t *testing.T) {
	for _, tc := range []struct{ long, short string }{
		{"? a\n: # c4\n  # c5\n  - 1\n", "a: # c4\n  # c5\n  - 1\n"},
		{"? a\n: # c4\n  # c5\n  b: 1\n", "a: # c4\n  # c5\n  b: 1\n"},
	} {
		assert.Equalf(t, commentsOf(t, tc.short), commentsOf(t, tc.long),
			"%q and %q are one document written two ways", tc.short, tc.long)
	}

	t.Run("every comment reaches the rendered text, and it settles", func(t *testing.T) {
		for src, renders := range map[string]string{
			"? a\n: # c4\n  # c5\n  - 1\n":  "? a\n: # c4\n# c5\n- 1\n",
			"? a\n: # c4\n  # c5\n  v\n":    "? a\n: # c4\n  v # c5\n",
			"? a\n: # c4\n  # c5\n  b: 1\n": "? a\n: # c4\n  # c5\n  b: 1\n",
		} {
			wellFormed(t, src)
			once := renderOnce(t, src)
			assert.Equal(t, renders, once, "%q", src)
			assert.Equal(t, once, renderOnce(t, once), "%q: the rendering settles", src)
		}
	})
}

// commentsOf reads src's comments as a map of path to "text/position", so two
// spellings of one document can be compared without depending on the order the
// map is built in.
func commentsOf(t *testing.T, src string) map[string][]string {
	t.Helper()

	cm := codec.CommentMap{}
	var v any
	require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.CommentToMap(cm)), "%q", src)

	out := make(map[string][]string, len(cm))
	for path, comments := range cm {
		for _, c := range comments {
			out[path] = append(out[path], fmt.Sprintf("%v/%v", c.Texts, c.Position))
		}
		sort.Strings(out[path])
	}

	return out
}

// TestFixedAKeyBelowItsIndicatorKeepsItsIndentation: a blank line under a "?"
// no longer takes the key out of the key.
//
// Renderer.entry wrote what follows a marker with a hanging indent, which
// leaves its first line where the marker left off. That is right while the
// marker holds the line, and wrong once a blank line has ended it: the key's
// content landed in column 1, where it reads as an entry of the document rather
// than as the key. `?` over a blank line over ` "": 0` over `: v` came back as
// `? ` over `"": 0` over `: v`, which parses, and then refuses to decode --
// "mapping key null already defined" -- so a document that read became a
// document that does not.
//
// A blank line ends the marker's line, so what follows takes the indentation
// every other line under the marker takes.
func TestFixedAKeyBelowItsIndicatorKeepsItsIndentation(t *testing.T) {
	t.Run("a blank line above the key keeps its indentation", func(t *testing.T) {
		const src = "?\n\n \"\": 0\n: v\n"

		f, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		assert.Equal(t, "? \n\n  \"\": 0\n: v\n", f.String(),
			"the key stands under its own indicator, and the blank line the author left stays")
	})

	t.Run("and the rendering reads back as the document that went in", func(t *testing.T) {
		for _, src := range []string{
			"?\n\n \"\": 0\n: v\n",
			"?\n\n  a: 0\n: v\n",
			"?\n\n  - a\n: v\n",
			"? # c\n\n \"\": 0\n: v\n",
		} {
			var want any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &want), "%q", src)

			f, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, err, "%q", src)

			var got any
			once := f.String()
			require.NoErrorf(t, codec.Unmarshal([]byte(once), &got), "%q rendered to %q", src, once)
			assert.Equalf(t, want, got, "%q rendered to %q, which reads as something else", src, once)
		}
	})

	t.Run("without the blank line the render settles", func(t *testing.T) {
		for _, src := range []string{"?\n a: 0\n: v\n", "?\n - a\n: v\n"} {
			f, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, err, "%q", src)

			once := f.String()
			g, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoErrorf(t, err, "%q", once)
			assert.Equalf(t, once, g.String(), "%q renders to %q and then moves", src, once)
		}
	})
}

// TestFixedADocumentSuffixReadsLikeAMarker: a "..." suffix followed by a bare
// document whose root is a block scalar carrying an anchor or a tag reads the
// same as the "---" spelling of the same stream.
//
// Two symptoms, one cause. scanDocumentStart cleared the scanner's indentation
// state for a "---" and scanDocumentEnd did not for a "...", so
// Scanner.lastDelimColumn crossed the marker: the next document's block scalar
// measured its content against whatever enclosed the node before it. With an
// indentation indicator a column of the content went missing; without one a
// valid stream was refused outright.
//
// A property in front of the header is what shows it, because a header at
// column 1 zeroes lastDelimColumn on its own -- "..." over "|2-" was right all
// along and "..." over "&a1 |2-" was not.
//
// The reference parser settles the content question and it agrees with the
// fix: "=VAL &a1 |  x" for both spellings. Do not reach for libfyaml or
// go.yaml.in/yaml/v3 here -- both strip a column from every root block scalar
// with an indicator, so they agree with each other and with neither the
// grammar nor the specification.
func TestFixedADocumentSuffixReadsLikeAMarker(t *testing.T) {
	t.Run("the two spellings read alike", func(t *testing.T) {
		for _, tc := range []struct{ suffix, marker, reads string }{
			{"a: 1\n...\n&a1 |2-\n  \n", "a: 1\n---\n&a1 |2-\n  \n", " "},
			{"a: 1\n...\n!!str |2-\n  x\n", "a: 1\n---\n!!str |2-\n  x\n", " x"},
			{"a: 1\n...\n&a1 |2-\n   x\n", "a: 1\n---\n&a1 |2-\n   x\n", "  x"},
			{"&a3 a: 1\n...\n&a1 >-\n -\n", "&a3 a: 1\n---\n&a1 >-\n -\n", "-"},
			// A header at column 1 was right before the fix and still is.
			{"a: 1\n...\n|2-\n  \n", "a: 1\n---\n|2-\n  \n", " "},
			// The enclosing indentation of the document before the marker is
			// what used to cross it, so a nested one is worth a case.
			{"a:\n  b: 1\n...\n&a1 |2-\n   x\n", "a:\n  b: 1\n---\n&a1 |2-\n   x\n", "  x"},
		} {
			wellFormed(t, tc.suffix)

			assert.Equalf(t, tc.reads, secondDocument(t, tc.suffix), "%q", tc.suffix)
			assert.Equalf(t, tc.reads, secondDocument(t, tc.marker), "%q", tc.marker)
		}
	})

	// The shapes that read before, kept so a fix here cannot quietly move them.
	t.Run("one anchor, or a plain scalar, read as they did", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\n...\n&a1 >-\n -\n",
			"&a3 a: 1\n...\n>-\n -\n",
			"&a3 a: 1\n...\n&a1 x\n",
		} {
			_, err := parser.ParseBytes([]byte(src), parser.WithComments())
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}

// TestFixedAMergeKeyAloneInFlowIsRefused holds both halves of Fred's ruling of
// 2026-09-08, closed on 2026-09-10.
//
// 3.2.1.1 makes two keys that resolve alike one key, and a flow entry written
// as a key alone is an entry like any other: `{a: 1, a}` is refused, and so is
// `{<<: {x: 1}, <<: {y: 2}}`. `{<<: {x: 1}, <<}` was read.
//
// Two faults, one per version, and the ruling settles them separately.
//
// Under the core schema nothing merges, so the two entries are one key spelled
// "<<" twice and the duplicate check answers. It did not, because the check
// read the key's token where ast.KeyName reads the node: the "<<" carrying a
// value reaches Parser.mapKeyIdentity as a StringNode over a token still typed
// MergeKeyType, and token.KeyName has no case for that type, so it was recorded
// "other/<<" where the bare one is "string/<<". A key's identity is its kind
// and its name, so the two never met -- the document read as {"<<": null} with
// the first entry's mapping gone and nothing reported. The StringNode arm of
// mapKeyIdentity makes both "string/<<".
//
// Under "%YAML 1.1" the merge key requires its ':', so a "<<" written as a flow
// entry's key alone is invalid merge syntax and the document is refused before
// any duplicate question arises. Read as an ordinary key it put a "<<" named
// nothing beside the entries a real merge had brought in, so the same two
// characters resolved to the merge type in one entry and to a string in
// another. parser.refuseMergeKeyAlone refuses it.
//
// The quoted spelling is the control and is untouched: `{"<<": {x: 1}, "<<"}`
// is an ordinary repeated key at both versions and is refused as one. So is the
// colon: write the second entry `<<: ` and every path refuses it, as it always
// did.
//
// The ruling is the strictest of four answers. go.yaml.in/yaml/v3 v3.0.5
// refuses both documents, libfyaml 1.0.0b1 reads them and drops an entry with
// nothing reported, and this library used to read the 1.1 one and keep "<<" as
// an ordinary key beside the merged entries -- which nobody else does.
//
// Found on 2026-09-07 when yamlcorpus's duplicateAKey landed on a merge key --
// the merge axis made that reachable for the first time.
func TestFixedAMergeKeyAloneInFlowIsRefused(t *testing.T) {
	t.Run("an ordinary key alone is refused, and so are two merge keys with values", func(t *testing.T) {
		for _, src := range []string{
			"{a: 1, a}\n",
			"{\"<<\": {x: 1}, \"<<\"}\n",
			"{<<: {x: 1}, <<: {y: 2}}\n",
			"b: &r {x: 1}\nd:\n  <<: *r\n  <<: *r\n",
		} {
			for _, full := range []string{src, "%YAML 1.1\n---\n" + src} {
				var got any
				assert.Errorf(t, codec.Unmarshal([]byte(full), &got), "%q", full)
			}
		}
	})

	t.Run("the core schema refuses the repeat", func(t *testing.T) {
		for _, src := range []string{"{<<: {x: 1}, <<}\n", "{<<, <<: {x: 1}}\n"} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), `mapping key "<<" already defined`,
				"and with the message an ordinary repeat gets: %q", src)
		}

		// The walk and the tree part on nothing now.
		var typed map[string]any
		assert.Error(t, codec.Unmarshal([]byte("{<<: {x: 1}, <<}\n"), &typed))
	})

	t.Run("and 1.1 refuses the syntax before the repeat", func(t *testing.T) {
		for _, src := range []string{
			"%YAML 1.1\n---\n{a: 1, <<}\n",
			"%YAML 1.1\n---\n{<<}\n",
			"%YAML 1.1\n---\n{<<: {x: 1}, <<}\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "merge key requires a ':' and a value to merge", "%q", src)
		}
	})

	t.Run("a bare merge key stays an ordinary key under the core schema", func(t *testing.T) {
		// Nothing merges there, so "<<" is two characters like any other.
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{src: "{a: 1, <<}\n", want: map[string]any{"a": uint64(1), "<<": nil}},
			{src: "{<<}\n", want: map[string]any{"<<": nil}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})

	t.Run("with the colon it is refused, which is the whole difference", func(t *testing.T) {
		for _, src := range []string{
			"{<<: {x: 1}, <<: }\n",
			"%YAML 1.1\n---\n{<<: {x: 1}, <<: }\n",
		} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), `mapping key "<<" already defined`, "%q", src)
		}
	})

	t.Run("and AllowDuplicateMapKey reads it with the last entry winning", func(t *testing.T) {
		var got any
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("{<<: {x: 1}, <<}\n"), &got, codec.AllowDuplicateMapKey()))
		assert.Equal(t, map[string]any{"<<": nil}, got,
			"the empty entry stands rather than being dropped")
	})
}
