---
title: "The YAML bestiary"
weight: 50
description: "YAML for conformant parsers only: legal documents that few humans read right."
---

Every entry below is legal YAML. Read it, decide what it holds, then open the answer: the JSON go-yaml reads from it.

Quizz: test your YAML knowledge (impossible level).

## Words and numbers

### Case counts, partly

{{< code file="tutorial/bestiary/testdata/spellings.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_spellings" nolink="true" >}}

The core schema knows three spellings of each word: all lower case, a capital first letter, all capitals. `tRUE` and
`nULL` are none of these, so they are strings.
{{% /expand %}}

### File modes

{{< code file="tutorial/bestiary/testdata/octal.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_octal" nolink="true" >}}

YAML 1.2 writes an octal number with `0o`. A leading zero alone means nothing, so `0755` is the decimal 755.

YAML 1.1 reads a leading zero as octal, and has no `0o`:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_octal11" nolink="true" >}}

A file mode written for one version reads as another number under the other.
{{% /expand %}}

### Base 60

{{< code file="tutorial/bestiary/testdata/sexagesimal.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_sexagesimal" nolink="true" >}}

Two strings, under YAML 1.2. YAML 1.1 reads digits split by colons as a number in base 60, for times and angles:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_sexagesimal11" nolink="true" >}}

`22:22` is 22 × 60 + 22, so a port mapping reads as 1342.
{{% /expand %}}

### Digit groups

{{< code file="tutorial/bestiary/testdata/underscore.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_underscore" nolink="true" >}}

YAML 1.2 has no digit separator, so `1_000` is a string. YAML 1.1 skips the `_` in a number:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_underscore11" nolink="true" >}}
{{% /expand %}}

### Numbers JSON would not write

{{< code file="tutorial/bestiary/testdata/numbers.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_numbers" nolink="true" >}}

All six are numbers under YAML 1.2: a hexadecimal integer, a signed one, a float with an exponent and no dot, and floats
with nothing before or after the dot. go-yaml writes `5.` as `5.0`, because JSON wants a digit after the dot.

YAML 1.1 wants a dot in every float, and a sign on its exponent, so `1e3` is a string there:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_numbers11" nolink="true" >}}
{{% /expand %}}

## Keys

### A key left out

{{< code file="tutorial/bestiary/testdata/empty-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_emptyKey" nolink="true" >}}

A key may be empty, and an empty node is null. JSON keys are strings, so the key reads as `"null"`.
{{% /expand %}}

### Keys that start with an indicator

{{< code file="tutorial/bestiary/testdata/indicators.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_indicators" nolink="true" >}}

`?` opens an explicit key and `:` a value only when a space follows them. Followed by a letter, each starts a plain
scalar.
{{% /expand %}}

### A key written as JSON

{{< code file="tutorial/bestiary/testdata/json-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_jsonKey" nolink="true" >}}

A `:` needs a space after it, except after a quoted key inside a flow collection. YAML 1.2 allows `"a":b` there, so
that JSON written without spaces still reads. A space before the `:` is allowed everywhere. In a block mapping,
`"a":b` is an error.
{{% /expand %}}

### A literal key

{{< code file="tutorial/bestiary/testdata/long-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_longKey" nolink="true" >}}

An explicit key may be any node, a block scalar included. It keeps its final line break, as a block scalar does by
default.
{{% /expand %}}

## Anchors and aliases

### An alias as a key

{{< code file="tutorial/bestiary/testdata/alias-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_aliasKey" nolink="true" >}}

`*k` stands for `name`, and the mapping uses it as a key. The space before `:` is needed: without it, the `:` belongs
to the alias name, as the next entry shows.
{{% /expand %}}

### A colon in an anchor

{{< code file="tutorial/bestiary/testdata/anchor-colon.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_anchorColon" nolink="true" >}}

An anchor name runs to the next space. It may hold any printable character except `,`, `[`, `]`, `{` and `}`, so
`an:chor` is one name.
{{% /expand %}}

### An anchor named !

{{< code file="tutorial/bestiary/testdata/anchor-bang.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_anchorBang" nolink="true" >}}

`&!` is not an anchor followed by a tag: it is the anchor named `!`, and `*!` is its alias.
{{% /expand %}}

## Flow collections

### A pair in a list

{{< code file="tutorial/bestiary/testdata/flow-pair.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_flowPair" nolink="true" >}}

Inside a flow sequence, `a: 1` is a mapping of one pair, written without its braces. The sequence holds that mapping
and the string `b`.
{{% /expand %}}

### Keys without values

{{< code file="tutorial/bestiary/testdata/flow-null.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_flowNull" nolink="true" >}}

In a flow mapping, an entry may be a key alone, or a key and a `:` with nothing after it. Both values are null.
{{% /expand %}}

### Explicit keys in flow

{{< code file="tutorial/bestiary/testdata/explicit-flow.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_explicitFlow" nolink="true" >}}

`?` works in flow too. `? c` is a key with no value, so `c` holds null.
{{% /expand %}}

### A trailing comma

{{< code file="tutorial/bestiary/testdata/trailing-comma.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_trailingComma" nolink="true" >}}

Two entries. A flow collection may end on a comma, which JSON forbids.
{{% /expand %}}

## Dashes

### Compact nesting

{{< code file="tutorial/bestiary/testdata/compact.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_compact" nolink="true" >}}

A sequence entry may open a collection on the line of its dash: `- - a` starts a sequence inside a sequence, and `- c: 1` a
mapping. The lines below line up with the inner collection's first entry.
{{% /expand %}}

### A dash and a space

{{< code file="tutorial/bestiary/testdata/dash-start.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_dashStart" nolink="true" >}}

A `-` opens an entry only when a space follows it. `-a` is a string, `-1` a negative number, and `- - 1` an entry
holding the sequence `[1]`.
{{% /expand %}}

### A dash on a continued line

{{< code file="tutorial/bestiary/testdata/dash-continued.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_dashContinued" nolink="true" >}}

One entry. `-b` has no space after its dash, so it opens no entry: the line continues the plain scalar `a`.
{{% /expand %}}

## Plain scalars and comments

### One scalar, three lines

{{< code file="tutorial/bestiary/testdata/plain-lines.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_plainLines" nolink="true" >}}

A plain scalar may run over several lines, as long as each continues further right than its key. YAML folds the line
breaks into spaces.
{{% /expand %}}

### A hash in the text

{{< code file="tutorial/bestiary/testdata/hash-inside.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_hashInside" nolink="true" >}}

A comment starts at a `#` after a space. The first `#` has none, so it belongs to the text, and the comment starts at
the second.
{{% /expand %}}

### A comment before the value

{{< code file="tutorial/bestiary/testdata/comment-value.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_commentValue" nolink="true" >}}

The comment ends the line, not the entry. The value on the next line, indented under the key, still belongs to `key`.
{{% /expand %}}

## Quoted scalars

### A quoted scalar over several lines

{{< code file="tutorial/bestiary/testdata/double-fold.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_doubleFold" nolink="true" >}}

A quoted scalar folds its lines as a plain scalar does: a line break becomes a space, and a blank line becomes one
line break. The indentation of each line is dropped.
{{% /expand %}}

### An escaped line break

{{< code file="tutorial/bestiary/testdata/escaped-break.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_escapedBreak" nolink="true" >}}

A `\` at the end of a line in a double-quoted scalar escapes the line break. The two lines join with no space, and the
next line's indentation is dropped.
{{% /expand %}}

### Escapes

{{< code file="tutorial/bestiary/testdata/escapes.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_escapes" nolink="true" >}}

A double-quoted scalar takes `\x` with two hex digits and `\u` with four. YAML 1.2 added `\/` for JSON's sake: it is a
`/`. A single-quoted scalar has no escapes: `''` is one quote, so `''''` holds a single `'`.
{{% /expand %}}

## Documents and layout

### Nothing, twice

{{< code file="tutorial/bestiary/testdata/empty-docs.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_emptyDocs" nolink="true" >}}

Two documents, each null. Each `---` opens a document, and a comment is not content.
{{% /expand %}}

### A scalar on the marker line

{{< code file="tutorial/bestiary/testdata/scalar-docs.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_scalarDocs" nolink="true" >}}

Three documents, each one scalar. A document may start its content on the `---` line, block scalar headers included.
{{% /expand %}}

### An indented document

{{< code file="tutorial/bestiary/testdata/indented-doc.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_indentedDoc" nolink="true" >}}

The root collection need not start in the first column. Its entries only have to line up with each other.
{{% /expand %}}

## From the generated corpus

go-yaml tests itself against a corpus of generated documents, in
[`internal/testintegration/yamlcorpus`](https://github.com/go-openapi/go-yaml/tree/master/internal/testintegration/yamlcorpus).
The generator writes valid documents, then changes each one a little: it swaps two neighbours, substitutes or inserts
an indicator, duplicates or deletes a byte, or cuts the document short. In the compact corpus, 16,239 of the 25,084
changed documents are still legal YAML. The entries below are some of them: legal documents that no person would
write.

### A directive that lost its %

`substitute an indicator` turned `%YAML 1.2` into this:

{{< code file="tutorial/bestiary/testdata/not-a-directive.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_notADirective" nolink="true" >}}

Two documents. `&YAML` is an anchor named `YAML` on the number `1.2`, and that is the first document. A directive must
come before a `---`, so the `---` here opens a second document, holding `-1`.
{{% /expand %}}

### A directive nobody knows

{{< code file="tutorial/bestiary/testdata/reserved-directive.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_reservedDirective" nolink="true" >}}

`%YA]L` is a directive with a name YAML does not define. The specification reserves such names, and a parser ignores
them, so the document reads as if the line were not there.
{{% /expand %}}

### Explicit keys under a swapped directive

{{< code file="tutorial/bestiary/testdata/swapped-directive.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_swappedDirective" nolink="true" >}}

`swap neighbors` turned `%YAML` into `%AYML`, an unknown directive, so it is ignored. The first key is the empty string
`''`, anchored, and its value `Null` is tagged `!!null`. The second key is the quoted `'? a'` under the non-specific
tag `!`, with no value.
{{% /expand %}}

### Four dashes

{{< code file="tutorial/bestiary/testdata/four-dashes.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_fourDashes" nolink="true" >}}

`duplicate a byte` added a fourth dash. `---` marks a document only when a space or a line break follows it, so
`----` is text. The next line continues that plain scalar, and the anchor, the tag and the number join the text.
{{% /expand %}}

### A marker glued to a mapping

{{< code file="tutorial/bestiary/testdata/dash-star.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_dashStar" nolink="true" >}}

`substitute a byte` replaced the line break after `---` with `*`. With no space after it, `---*{D` is a plain scalar,
and the first `: ` makes it a key. Inside a plain scalar of a block mapping, `{` and `}` are ordinary characters, so
the value is the string `False}`.
{{% /expand %}}

### An anchor that ate a tag

{{< code file="tutorial/bestiary/testdata/anchor-swallows.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_anchorSwallows" nolink="true" >}}

`swap neighbors` moved the `!` before the space. An anchor name runs to the next space, so the anchor is `a1!`, and
`<tag:yaml.org,2002:int> -23` is no longer a tag and a number: it is a string.
{{% /expand %}}

### An anchor and nothing

{{< code file="tutorial/bestiary/testdata/anchor-tilde.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_anchorTilde" nolink="true" >}}

The `~` would be null, but it belongs to the anchor name: `a1-~`. The anchor stands on an empty node, which is null
anyway.
{{% /expand %}}

### A tag that was a word

{{< code file="tutorial/bestiary/testdata/local-true.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_localTrue" nolink="true" >}}

`insert an indicator` put a `!` before `true`. `!true` is now a local tag, and the node it stands on is empty. go-yaml
has no type for the tag, so it keeps the node's text: the empty string, not a boolean and not null.
{{% /expand %}}

### Cut short

{{< code file="tutorial/bestiary/testdata/truncated.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_truncated" nolink="true" >}}

`truncate` cut the document after the anchor. `!!str` tags the key `base`, and the value is an anchor on an empty
node, which is null.
{{% /expand %}}

### A lost comma

{{< code file="tutorial/bestiary/testdata/lost-comma.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_lostComma" nolink="true" >}}

`delete a byte` removed the comma. A plain scalar may hold spaces, so `FALSE ~` is one entry, and it reads as neither a
boolean nor a null. With the comma, the same sequence holds two:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_commaKept" nolink="true" >}}
{{% /expand %}}

### An entry over three lines

{{< code file="tutorial/bestiary/testdata/entry-continued.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_entryContinued" nolink="true" >}}

The `-` has nothing after it on its line, so its value starts at `c2`, after a blank line. The next line is indented
further than the dash, so it continues the plain scalar: its `- ` opens no entry, and `&a1 {}` is text.
{{% /expand %}}

### A bracket left open

{{< code file="tutorial/bestiary/testdata/open-bracket.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_openBracket" nolink="true" >}}

The entry's anchor stands on the mapping below it, past a comment. The mapping holds one explicit key, the quoted
`"+0b11"`. `[` opens a flow sequence only at the start of a node, so `NULL[` is a plain scalar.
{{% /expand %}}

### A signed key

{{< code file="tutorial/bestiary/testdata/plus-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_plusKey" nolink="true" >}}

Two documents: `...` ends the first. Its key is a float, and go-yaml names a number key in JSON by its value, so the
`+` is gone. The second document is the string `#`: a quoted `#` starts no comment.
{{% /expand %}}

### An indentation indicator at the root

{{< code file="tutorial/bestiary/testdata/root-indent.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_rootIndent" nolink="true" >}}

`|3` sets the content's indentation to the block scalar's own level plus 3. Under a key or a `-`, the level is 0 and the
content starts in column 3. A root node stands at level -1, so here the content starts in column 2, and `|a` keeps no
space.

`indent a line further` moved the line four columns right. The four extra spaces are content:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_rootIndentFurther" nolink="true" >}}

Readers disagree on this one. go-yaml and the YAML reference parser follow the grammar's level -1. libyaml, and PyYAML
with it, count a root block scalar from level 0: it rejects the first document and reads three spaces in the second.
{{% /expand %}}
