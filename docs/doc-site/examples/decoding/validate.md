---
title: Validate on decode
weight: 30
description: |
  Check each struct as the decoder fills it with Validator, and get the error
  back at the line that holds the bad value.
---

The task: reject a configuration whose values are out of range, and report the
line of the bad value, as a syntax error would be reported.

## The types and the file

{{< code file="decoding/validate/example_test.go" lang="go" region="types" >}}

{{< code file="decoding/validate/example_test.go" lang="go" region="source" >}}

## A validator

{{< code file="decoding/validate/example_test.go" lang="go" region="check" >}}

`codec.Validator` calls the function once for each struct the decoder fills,
after it is filled, with the struct by value: here once per `Listener`, then
once for the `Config`. A type switch or a type assertion picks the structs the
function checks.

## Decode with it

{{< code file="decoding/validate/example_test.go" lang="go" region="decode" >}}

{{< code file="decoding/validate/example_test.go" lang="text" output="Example" >}}

The decoder places the error at the mapping of the struct the validator
rejected, the second listener, at line 5, and draws the source around it.

## Match the error

{{< code file="decoding/validate/example_test.go" lang="go" region="kinds" >}}

{{< code file="decoding/validate/example_test.go" lang="text" output="Example_kinds" >}}

The error matches `errors.ErrValidation`, and it wraps the validator's own
error, so `errors.Is` also reaches `errPort`. The value is decoded in full:
the validator runs after the struct is filled and changes nothing in it.

## Point at the field

An error that is a slice of values with a `StructField() string` method points
at the value of the first field it names:

{{< code file="decoding/validate/example_test.go" lang="go" region="fielderrors" >}}

{{< code file="decoding/validate/example_test.go" lang="go" region="field" >}}

{{< code file="decoding/validate/example_test.go" lang="text" output="Example_field" >}}

`StructField` returns the Go field name, `Port`, and the decoder finds the key
that field reads, `port`. The error now stands on `70000` at line 6.

## Variations

- **go-playground/validator.** Its `ValidationErrors` already has this shape.
  Pass its `Struct` method: `codec.Validator(validator.New().Struct)`.
- **A stream.** The option works the same on `codec.NewDecoder`. See
  [A stream of documents](../stream/).

The error kinds in full: [Errors](../../../values/errors/).
