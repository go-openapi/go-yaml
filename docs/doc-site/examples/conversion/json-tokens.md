---
title: JSON tokens as they arrive
weight: 20
description: |
  Read a YAML document as a stream of JSON tokens with ToJSONTokens, with a
  JSON pointer and a source position for each, and stop as soon as you have
  what you need.
---

The task: find one value in a large YAML file, as a JSON pointer, with its line
and column, without converting the rest.

## The document

{{< code file="conversion/jsontokens/example_test.go" lang="go" region="source" >}}

## Range over the tokens

{{< code file="conversion/jsontokens/example_test.go" lang="go" region="range" >}}

{{< code file="conversion/jsontokens/example_test.go" lang="text" output="Example" >}}

`ToJSONTokens` reads nothing until the range starts. Each token holds:

- `Kind`, one of nine: object and array start and end, key, string, number,
  bool and null. No token stands for a `,` or a `:`.
- `Value`, the text of a key, a string or a number. A number keeps its digits.
- `At`, the position of its first byte in the YAML source.

`ts.Path()` returns the RFC 6901 pointer of the token the loop is on. A key and
its value share the pointer of their member. The YAML wrote no brace to
close the document, so its `}` takes the position just past the last byte the
mapping holds: 3:22, after the `}` of `db`.

## Stop when you have it

{{< code file="conversion/jsontokens/example_test.go" lang="go" region="find" >}}

{{< code file="conversion/jsontokens/example_test.go" lang="text" output="Example_find" >}}

Breaking out of the range stops the parse. `Err` returns nil: a break is not
an error, and nothing after `db.host` is read.

## Check Err after the loop

{{< code file="conversion/jsontokens/example_test.go" lang="go" region="err" >}}

{{< code file="conversion/jsontokens/example_test.go" lang="text" output="Example_err" >}}

The loop body ran for every token before `timeout: .inf`, then the range ended,
with nothing in the loop to show why. `Err` returns the error that stopped it.
Call it after every range.

## Cap the tokens

{{< code file="conversion/jsontokens/example_test.go" lang="go" region="budget" >}}

{{< code file="conversion/jsontokens/example_test.go" lang="text" output="Example_budget" >}}

Each alias hands over everything its anchor names again, so a few hundred bytes
can name more values than memory holds. `Budget` caps the tokens one document
may hand over. The default, 0, sets no cap. Set one for input you did not write.
The token past the cap stops the range, and `Err` returns an error that matches
`errors.ErrNotJSON`, at that token's position.

## Variations

- **One buffer.** `ToJSON` writes the same tokens, with the commas and colons,
  into one `[]byte`. See [YAML to JSON and back](../yaml-json/).
- **Reuse a buffer for the pointer.** `ts.AppendPath(dst)` appends the pointer
  to `dst`.
