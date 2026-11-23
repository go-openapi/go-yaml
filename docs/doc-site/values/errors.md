---
title: Errors
weight: 70
description: |
  What the library returns when a document is wrong, and how to print it with
  the offending line underneath.
---

## The pretty rendering

Every error carries the token it was raised at, so it can be printed with the
source around it. `errors.FormatError` does that:

```go
fmt.Println(errors.FormatError(err, false, true))
```

```
[6:1] cannot use []interface {} as a map key: it is not comparable
   3 | version: 1.2.3
   4 | limits:
   5 |   max: 18446744073709551615
>  6 | ? [a, b]
       ^
   7 | : a sequence used as a key
```

The two booleans are `colored` and `inclSource`. `FormatErrorAtToken` does the
same for a message and a token of your own, which is what you want when your
program rejects a document the parser accepted.

## Telling one failure from another

Match on the sentinels with `errors.Is`:

```go
if errors.Is(err, yamlerrors.ErrSyntax) {
	// the document is not YAML
}
```

Or take the error apart with `errors.As` and an `*errors.Error`, which carries
the token and therefore the position.

The conditions the library distinguishes: syntax, type mismatch, overflow,
duplicate key, unknown field, unknown anchor, recursive alias, excessive
aliasing, exceeded depth, unhashable key, not-JSON, not-JSON syntax, unexpected
node type, validation, and an error from a type that decodes itself.

## A value that does not fit its field

A value of the wrong type for its field does not stop the decode. The decoder
skips it, keeps the first such error, and decodes the rest, as `encoding/json`
does:

```go
var v struct {
	Port int
	Name string
}
err := yaml.Unmarshal([]byte("port: eighty\nname: web\n"), &v)
// errors.Is(err, yamlerrors.ErrTypeMismatch) is true, and v.Name is "web".
```

The destination is decoded in place. A nested struct keeps the fields it read
before the error and the defaults you set, a slice keeps a zero value where
the element failed, and a map keeps your own entries.

## A repeated key

A key written twice in one mapping makes the document invalid, and the decode
returns `errors.ErrDuplicateKey`. The decoder finds the repeat when the mapping
closes, so the whole mapping is decoded first, and the later value wins:

```go
var m map[string]int
err := yaml.Unmarshal([]byte("a: 1\na: 2\nb: 3\n"), &m)
// errors.Is(err, yamlerrors.ErrDuplicateKey) is true, and m is map[a:2 b:3].
```

The decode of the document stops there. Everything decoded before the stop
stands, into a struct, a map or an `any` alike, and a field or an entry after
that mapping is not decoded. A `<<` in the mapping merges nothing. On a stream,
the next `Decode` reads the next document.

## A struct your validator refuses

`codec.Validator` runs your function on each struct the decoder fills. Its error
comes back as `ErrValidation`, placed at the field it names, and wraps your
validator's own error:

```go
validate := validator.New()
err := codec.Unmarshal(src, &v, codec.Validator(validate.Struct))
// errors.Is(err, yamlerrors.ErrValidation) is true, and
// errors.As(err, &validator.ValidationErrors{}) reaches go-playground's errors.
```

## A type that decodes itself

An error returned by `UnmarshalYAML`, `UnmarshalText`, or a function passed to
`codec.RegisterCustomUnmarshaler` comes back as `ErrUnmarshaler`, at the value
the type was decoding, and wraps the type's own error. An error the decoder
raised for the type, such as a type mismatch inside the `unmarshal` callback,
comes back as it is, with its own position. Like a value that does not fit its
field, it does not stop the decode.

## Pointing at a value your own code rejected

The parse succeeded, your validation did not, and you want the same rendering.
Address the value by path and annotate the source:

```go
var v struct {
	A int
	B string
}
if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
	// ...
}
if v.A != 2 {
	path, err := expressions.PathString("$.a")
	if err != nil {
		// ...
	}
	source, err := path.AnnotateSource([]byte(yml), true)
	if err != nil {
		// ...
	}
	fmt.Printf("a value expected 2 but actual %d:\n%s\n", v.A, source)
}
```

See [YAMLPath](../../documents/yamlpath/) for the path syntax.
