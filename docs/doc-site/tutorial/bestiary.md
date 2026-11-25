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

