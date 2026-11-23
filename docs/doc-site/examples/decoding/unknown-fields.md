---
title: Reject unknown fields
weight: 20
description: |
  Catch a typo in a configuration file with DisallowUnknownField, and read the
  line and column off the error.
---

The task: a configuration file with a typo in a key should fail to load, and
say where the typo is.

## The type and the file

{{< code file="decoding/unknownfields/example_test.go" lang="go" region="types" >}}

{{< code file="decoding/unknownfields/example_test.go" lang="go" region="source" >}}

By default, `Unmarshal` ignores a key no field takes: this file decodes with
no error and `Port` set to 9090. The typos go unnoticed.

## Decode with DisallowUnknownField

{{< code file="decoding/unknownfields/example_test.go" lang="go" region="decode" >}}

{{< code file="decoding/unknownfields/example_test.go" lang="text" output="Example" >}}

The error names the first unknown key and its position, `[line:column]`, and
draws the lines around it. `codec.Strict()` sets the same option and nothing
else.

## The decode goes on

{{< code file="decoding/unknownfields/example_test.go" lang="go" region="partial" >}}

{{< code file="decoding/unknownfields/example_test.go" lang="text" output="Example_partial" >}}

An unknown field is an error about one field. The decoder records the first
one and goes on filling the fields it knows, so `Port` still holds 9090. The
error stops nothing else in the document. See [Errors](../../../values/errors/).

## Read the position

A linter or an editor wants the position as data, not as text:

{{< code file="decoding/unknownfields/example_test.go" lang="go" region="position" >}}

{{< code file="decoding/unknownfields/example_test.go" lang="text" output="Example_position" >}}

`errors.Is` matches the kind against `errors.ErrUnknownField`. `GetToken`
returns the token the error stands on, and `GetMessage` the message without
the position or the source.

## Variations

- **Only the first typo is reported.** `hots` in the file above is not
  mentioned. Fix one, run again.
- **Keys from an embedding format.** `codec.AllowFieldPrefixes("x-")` lets
  keys with that prefix through while unknown fields are rejected.
