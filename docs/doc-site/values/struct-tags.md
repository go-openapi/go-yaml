---
title: Struct tags
weight: 20
description: |
  What `yaml:"..."` means, what `json:"..."` means, and the two options that
  decide whether the second one is read at all.
---

## The default is `go.yaml.in/yaml/v3`, exactly

A field with no `yaml` tag is named after its Go field, lowercased. A `json` tag
is **not** read.

{{% notice style="warning" title="A json tag alone does nothing" %}}
```go
type J struct {
	A int `json:"foo"`
}

var j J
codec.Unmarshal([]byte("foo: 1\n"), &j)  // err == nil, j.A == 0
```
The document says `foo`, the field wants `a`, nothing matches, and the decode
succeeds. There is no error and no warning. The same struct marshals to
`a: 7`, not `foo: 7`.
{{% /notice %}}

That is v3's model, and a 35-row parity table in the test suite holds it against
v3 itself. If you want `encoding/json`'s model, ask for it.

## Three naming layers, behind two booleans

| option | what it adds |
|---|---|
| *(default)* | the `yaml` tag, else the lowercased Go name — v3's model |
| `UseJSONTags(true)` | the `json` tag is read, the way `encoding/json` reads it |
| `UseInferredNames(true)` | an untagged field takes its Go name verbatim, the way `encoding/json` does |

When a field carries both tags, `yaml` wins:

```go
type Both struct {
	A int `yaml:"ya" json:"ja"`
}
// "ya: 2\nja: 3\n" gives A == 2, with or without UseJSONTags.
```

The encoder has the mirror pair, `WriteJSONTags` and `WriteInferredNames`.

{{% notice style="note" %}}
The two directions are separate options. Setting `UseJSONTags` on a decode
without setting `WriteJSONTags` on the matching encode gives you an asymmetric
round trip, and nothing reports it.
{{% /notice %}}

## The tag itself

```go
type T struct {
	Name    string            `yaml:"name"`
	Skipped string            `yaml:"-"`
	Inner   Embedded          `yaml:",inline"`
	Extra   map[string]string `yaml:",inline"`
	Flow    []int             `yaml:"flow,flow"`
	Maybe   string            `yaml:"maybe,omitempty"`
	Zero    time.Time         `yaml:"zero,omitzero"`
	Def     *Person           `yaml:"default,anchor"`
	Use     *Person           `yaml:",inline,alias"`
}
```

| option | effect |
|---|---|
| `-` | the field is never read or written |
| `,inline` | an embedded struct's fields, or a map's entries, sit in the parent mapping |
| `,flow` | written in flow style: `[1, 2, 3]` |
| `,omitempty` | not written when empty, or when `IsZero()` says so |
| `,omitzero` | not written when the Go zero value |
| `,anchor` | written with an anchor — see [Anchors and aliases](../anchors-and-aliases/) |
| `,alias` | written as an alias to a matching anchor |

The tag name is `yaml`.

## Related options

`AllowFieldPrefixes` accepts a key that begins with one of the prefixes you give
it. There is no counterpart on the encoder, so a document read that way does not
round-trip.
