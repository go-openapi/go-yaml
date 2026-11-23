---
title: A type that decodes itself
weight: 70
description: |
  Decode a value written in its own notation with UnmarshalYAML, and a type you
  do not own with CustomUnmarshaler or RegisterCustomUnmarshaler.
---

The task: a configuration writes sizes as `10MiB` and webhooks as URLs. Decode
both into Go types that hold them properly.

## A type of your own

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="size" >}}

A type with an `UnmarshalYAML([]byte) error` method implements
`codec.Unmarshaler`, and the decoder calls it with the value's YAML text, as
the document wrote it. A quoted scalar arrives with its quotes, and a sequence
arrives as `[1, 2]`. Decoding the text into a `string` with `codec.Unmarshal`
strips the quotes, and returns an error for a mapping or a sequence.

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="limits" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="decode" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="text" output="Example" >}}

## When it fails

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="error" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="text" output="Example_error" >}}

The decoder wraps the method's error in `errors.ErrUnmarshaler` at the value the
method was decoding, so it prints the line and column and the source around
them. `errors.Is` still reaches `errSize`. The decode goes on, so `Cache` is
filled.

## A type you do not own

`url.URL` has no YAML method, and the decoder reads it as a struct:

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="hook" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="reflect" >}}

Give the decode a function for the type with `codec.CustomUnmarshaler`. It
receives the same YAML text a method would:

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="parseURL" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="custom" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="text" output="Example_notOwned" >}}

## For every decode

`codec.RegisterCustomUnmarshaler` installs the function for every decoder in
the process:

{{< code file="decoding/selfdecode/example_test.go" lang="go" region="register" >}}

{{< code file="decoding/selfdecode/example_test.go" lang="text" output="Example_register" >}}

A registration is global. Two packages that register a function for one type
replace each other's, and neither sees it. Prefer `CustomUnmarshaler`, which
applies to one call and takes precedence over a registered function.

## Variations

- **The node, not the text.** `codec.NodeUnmarshaler` receives the
  `ast.Node` the value was read from, with its position and comments.
- **The go-yaml v2 shape.** `codec.GoYAMLUnmarshaler`,
  `UnmarshalYAML(func(any) error) error`, decodes the value into a destination
  of your choice.

The interfaces in full: [Marshal and Unmarshal](../../../values/marshal-unmarshal/#letting-a-type-encode-itself).
