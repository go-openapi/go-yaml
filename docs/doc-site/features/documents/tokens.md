---
title: Tokens
weight: 30
description: |
  What a token holds, how it addresses the source it came from, and the
  functions that read a YAML scalar spelling on their own.
---

## Where a token comes from

There is no exported tokenizer. A token reaches you from a node:

```go
tk := node.GetToken()
```

The scanner lives at `internal/scanner`, under the parser, so `token` is a type
you read rather than a stream you drive.

## What a token holds

| | |
|---|---|
| `Value` | the meaningful text, with the syntax taken off |
| `Type` | which lexical kind — `StringType`, `DoubleQuoteType`, `LiteralType`, `IntegerType`, and so on |
| `Position` | `Line`, `Column`, and `Offset()` |

`Value` is processed and the source is not. For `name: "pet store"` the value
token has `Value == "pet store"`; the quotes are gone.

## Addressing the source

`Position.Offset()` counts bytes from 0 and `Token.EndOffset()` ends the token,
so `src[off:end]` is the token **as written**:

```go
p := tk.Position
fmt.Printf("%q\n", src[p.Offset():tk.EndOffset()])
```

| token | `Value` | `src[Offset():EndOffset()]` |
|---|---|---|
| `name` | `"name"` | `"name"` |
| `"pet store"` | `"pet store"` | `"\"pet store\""` |
| a literal block's body | `"two\nlines\n"` | `"two\n  lines\n"` |

That third row is the useful one: `Value` has the block folded and re-indented,
and the offsets still address the bytes the document actually carried, indentation
included.

`Line` and `Column` count from 1 and count characters, because characters are
what YAML measures indentation in. `Position.IndentNum()` gives the indentation
the token sits at.

{{% notice style="note" %}}
Offsets address the token for 97.1% of the YAML Test Suite's tokens, and line and
column for 94.2%. Two kinds of token are still reported early. Reconstructing a
document byte for byte is a [target, not a feature](../../about/status/), and
those percentages are the distance to it.
{{% /notice %}}

## A block scalar splits over two nodes

`ast.LiteralNode.Start` is the header token — `|` or `>`, with any chomping
indicator. The content is a separate node, and its token carries the folded text
and the offsets of the raw lines.

## Reading a scalar spelling without a document

The functions that turn YAML's scalar text into Go values are exported, so you
can use them on a string you got some other way:

```go
token.ParseInteger("0x1F", token.HexIntegerType)  // 31, true
token.ParseInteger("1_000", token.IntegerType)    // 1000, true
token.ToNumber("1.5e3")                            // {Type:float Value:1500 Text:1.5e3}
```

`ParseInteger` returns an `int64` where the text carries a sign and a `uint64`
where it does not. `ParseBool`, `ParseFloat`, `ParseWholeNumber` and
`ParseTimestamp` cover the rest, and `ParseBigInteger` and `ParseBigFloat` handle
what does not fit a machine word. `ToNumber` reports the base it read as well as
the value.

## Deciding whether a scalar needs quoting

Two functions, and the names do not tell them apart:

| | asks |
|---|---|
| `IsNeedQuoted(v)` | would a plain scalar spelling `v` be read back as something else? `"yes"`, `"1.0"` and `"a: b"` all say true. |
| `NeedsQuotedSpelling(v)` | does `v` hold a character **only** a double-quoted scalar can carry? Exactly two: a carriage return, and U+FEFF. |

So `IsNeedQuoted("yes")` is true and `NeedsQuotedSpelling("yes")` is false. Use
the first to decide about quoting, the second to decide that nothing *but*
double quotes will do.

`LiteralBlockHeader(v)` gives the header a literal block needs for `v` —
the chomping and indentation indicators.

## Tags

`token.YAMLTagPrefix` is `tag:yaml.org,2002:`. `ReservedTagOf` maps a full tag
URI to a `ReservedTagKeyword` such as `!!int`, and `ReservedTagKeywordMap` holds
the set. `token.Schema` names the resolution schema — see
[YAML versions and tags](../../values/yaml-versions/).

## Two constructors for every token

`token.Alias(org string, pos)` returns a `*Token`; `token.MakeAlias[T Text](org T, pos)`
returns a `Token` by value, over a `string` or a `[]byte`. There is such a pair
for about twenty token kinds. They differ in where the token is allocated, not in
what it means. If you are building tokens — which mostly means building a tree by
hand — either family works; the value form lets the parser fill an arena instead
of allocating one token at a time.
