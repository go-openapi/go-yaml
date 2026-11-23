---
title: Getting started
weight: 10
description: |
  Install go-yaml, decode a document, encode one back.

  And decide which of the two APIs you need: the codec, or the parser and the AST.
---

## Install

```cmd
go get github.com/go-openapi/go-yaml
```

Requires Go {{% siteparam "goyaml.goVersion" %}} or newer.

{{% notice style="warning" title="No release yet" %}}
The module has no tagged version. `go get` resolves a pseudo-version on `master`,
and the API still moves. See [Status](../about/status/).
{{% /notice %}}

## Decode

Into a struct:

```go
import yaml "github.com/go-openapi/go-yaml"

type Service struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Ports   []int  `yaml:"ports"`
}

src := []byte(`name: pet-store
version: 1.2.3
ports:
  - 80
  - 443
`)

var s Service
if err := yaml.Unmarshal(src, &s); err != nil {
	// ...
}
// {Name:pet-store Version:1.2.3 Ports:[80 443]}
```

Into `any`, a mapping becomes a `map[string]any` and a sequence a `[]any`:

```go
var v any
if err := yaml.Unmarshal(src, &v); err != nil {
	// ...
}
// map[string]any{"name":"pet-store", "ports":[]any{uint64(80), uint64(443)}, "version":"1.2.3"}
```

A positive integer comes back as a `uint64` and a negative one as an `int64`.
v3 gives an `int` for both — see [Migrating](migrating/).

## Encode

```go
out, err := yaml.Marshal(s)
```

```yaml
name: pet-store
version: 1.2.3
ports:
- 80
- 443
```

A struct is written in field order, a map in sorted key order. Sequence entries
stay at their key's column; `codec.IndentSequence(true)` indents them. A field
with no `yaml` tag is named by its Go name in lower case, so `MaxConns` is
written `maxconns` — see [Struct tags](../values/struct-tags/).

The root package holds `Marshal`, `Unmarshal`, `ToJSON` and `FromJSON` with no
options. The same functions with options are in `codec`.

## Then what

Read [Which layer you need](three-levels/). Most callers stay in
[Decoding into Go values](../values/). Programs that edit, convert or report on
a document read [Working with documents](../documents/).
