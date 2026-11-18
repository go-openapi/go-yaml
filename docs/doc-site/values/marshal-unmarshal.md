---
title: Marshal and Unmarshal
weight: 10
description: |
  The three entry points, the two constructors, and the interfaces a type can
  implement to control its own encoding.
---

## Encode

```go
var v struct {
	A int
	B string
}
v.A = 1
v.B = "hello"

out, err := yaml.Marshal(v)
if err != nil {
	// ...
}
fmt.Println(string(out)) // "a: 1\nb: hello\n"
```

A field with no tag is written under its Go name, lowercased. That is
`go.yaml.in/yaml/v3`'s rule, and it is the default here — see
[Struct tags](../struct-tags/) for the two options that change it.

## Decode

```go
yml := `
%YAML 1.2
---
a: 1
b: c
`
var v struct {
	A int
	B string
}
if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
	// ...
}
```

## The three entry points, and when each one

| call | use it when |
|---|---|
| `yaml.Marshal` / `yaml.Unmarshal` | you need no options |
| `codec.MarshalWithOptions` / `codec.UnmarshalWithOptions` | you need options for one call |
| `codec.NewEncoder` / `codec.NewDecoder` | you are reading or writing a stream, or reusing the options |

The root package re-exports only `Marshal` and `Unmarshal`. Everything that takes
an option lives in
[`codec`](https://pkg.go.dev/github.com/go-openapi/go-yaml/codec).

## Reading a stream of documents

A YAML stream holds any number of documents, separated by `---`. `Decoder.Decode`
reads one per call and returns `io.EOF` when the stream ends:

```go
dec := codec.NewDecoder(r)
for {
	var v Config
	err := dec.Decode(&v)
	if errors.Is(err, io.EOF) {
		break
	}
	if err != nil {
		return err
	}
	// ...
}
```

Documents in one stream are independent: an anchor declared in one is not visible
from the next. See [Anchors and aliases](../anchors-and-aliases/).

## Letting a type encode itself

Five interfaces: two pairs, and `NodeUnmarshaler`. Implement one of the first two on your type and
the codec calls it instead of using reflection.

| interface | you return / receive | modelled on |
|---|---|---|
| `codec.Marshaler` / `codec.Unmarshaler` | YAML text, as `[]byte` | `encoding/json` |
| `codec.GoYAMLMarshaler` / `codec.GoYAMLUnmarshaler` | another Go value | `gopkg.in/yaml.v2` |

`codec.NodeUnmarshaler` receives an
[`ast.Node`](../../documents/ast/) rather than bytes, and a type with an
`IsZero() bool` method — `codec.IsZeroer`, which `time.Time` satisfies — decides
what `omitempty` skips.

**The two families differ in cost, not in meaning.** Indentation is significant
in YAML, so a fragment returned by a `codec.Marshaler` cannot be spliced into its
parent as text — the encoder has to parse it back to work out how to place it. A
`codec.GoYAMLMarshaler` returns a Go value and skips that parse. For a type
marshalled in a loop, prefer the second; for a config file written once, the
first is easier to write.

## Registering a marshaler for a type you do not own

`codec.CustomMarshaler[T]` and `codec.CustomUnmarshaler[T]` take a function for
one type, as an option on one call. `codec.RegisterCustomMarshaler[T]` does the
same globally, for every call in the process — which is why two packages
registering for the same type is a conflict neither can see. Prefer the option.

## Ordered mappings

`codec.MapSlice` is a `[]codec.MapItem`, and it decodes and encodes in document
order where a Go map cannot. Use it when order matters and the document is still
going through the codec; when order matters and the rest of the document does
too, work on [the document](../../documents/) instead.
