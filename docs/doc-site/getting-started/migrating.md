---
title: Migrating
weight: 20
description: |
  Coming from go.yaml.in/yaml/v3, goccy/go-yaml or gopkg.in/yaml.v2.

  What is the same, what is renamed, and what decodes differently.
---

`yaml.Marshal(v)` and `yaml.Unmarshal(data, &v)` have the same signatures as in
the three libraries below, so a call that passes no option compiles after you
change the import path. The options live in [`codec`](../../values/):
`codec.Marshal(v, opts...)` and `codec.Unmarshal(data, &v, opts...)`.

A program that compiles can still decode differently. Every output on this page
was copied from a run on 2026-11-22.

## YAML 1.2 by default

This library reads a document as YAML 1.2 unless the document starts with a
`%YAML 1.1` directive. The three others read some of YAML 1.1's types. Into an
`any`:

| document | go-openapi | v3 | goccy | v2 |
|---|---|---|---|---|
| `a: yes` | `"yes"` | `"yes"` | `"yes"` | `true` |
| `a: 0777` | `777` | `511` | `511` | `511` |
| `a: 1_000` | `"1_000"` | `1000` | `1000` | `1000` |
| `a: 2001-12-14` | `"2001-12-14"` | a `time.Time` | `"2001-12-14"` | `"2001-12-14"` |
| `<<: *b` in a mapping | a key `"<<"` | merged | merged | merged |

To read a document as YAML 1.1 without a directive, pass
`codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))`. To merge `<<`
keys and keep the rest of YAML 1.2, pass
`codec.WithParserOptions(parser.WithMergeKeys())`. See
[YAML versions](../../values/yaml-versions/).

## Integers in an `any`

A positive integer decodes into an `any` as a `uint64`, and a negative one as an
`int64`. v3 and v2 give an `int` for both. goccy gives the same types as this
library. A type switch that tests for `int` misses every integer.

## From `go.yaml.in/yaml/v3`

The default struct tags are v3's: a field is named after its `yaml` tag, or
after its Go name lowercased, and a `json` tag is not read. `omitempty` follows
v3's rule. See [Struct tags](../../values/struct-tags/).

A repeated key is an error in both: `mapping key "a" already defined`.

**There is no `yaml.Node`.** Parse with `parser.ParseBytes` to get an
`*ast.File`, and see [Documents](../../documents/).

{{% notice style="warning" title="A v3 node hook is never called" %}}
A type whose method is `UnmarshalYAML(*yaml.Node) error` matches none of this
library's interfaces. The decoder ignores the method, returns no error, and
leaves the field at its zero value.

Rewrite the method to one of these:

| interface | method |
|---|---|
| `codec.NodeUnmarshaler` | `UnmarshalYAML(ast.Node) error` |
| `codec.Unmarshaler` | `UnmarshalYAML([]byte) error`, which receives the value's YAML text |
| `codec.GoYAMLUnmarshaler` | `UnmarshalYAML(func(any) error) error`, v2's shape |
{{% /notice %}}

## From `goccy/go-yaml`

This library forked from goccy/go-yaml. The packages moved, and
[The fork](../../about/fork/) lists where. Most calls change as follows:

| goccy/go-yaml | go-openapi/go-yaml |
|---|---|
| `yaml.MarshalWithOptions(v, opts...)` | `codec.Marshal(v, opts...)` |
| `yaml.UnmarshalWithOptions(data, &v, opts...)` | `codec.Unmarshal(data, &v, opts...)` |
| `yaml.NewEncoder`, `yaml.NewDecoder` | `codec.NewEncoder`, `codec.NewDecoder` |
| `yaml.PathString` | `expressions.PathString` |
| `parser.ParseBytes(src, parser.ParseComments)` | `parser.ParseBytes(src, parser.WithComments())` |
| `parser.ParseFile(path, mode)` | `os.ReadFile`, then `parser.ParseBytes` |
| `printer.Printer` | `errors.FormatError` for an error with its source lines |
| the `context.Context` variants | removed |

**A `json` tag is not read by default.** goccy names a field after its `json`
tag when it has no `yaml` tag. This library follows v3 instead. For a field
``Name string `json:"full_name"` ``, goccy reads `full_name: x` into the field and
this library leaves it empty. Pass `codec.UseJSONTags(true)` to read the `json`
tag.

The `Unmarshaler`, `GoYAMLUnmarshaler` and `NodeUnmarshaler` interfaces have
goccy's method shapes. `NodeUnmarshaler` takes this library's `ast.Node`, so
change the import of `ast` too.

goccy's variants that take a `context.Context` first, such as
`UnmarshalYAML(context.Context, []byte) error`, are gone. As with v3's node hook,
the decoder ignores such a method and returns no error. Drop the context
argument.

## From `gopkg.in/yaml.v2`

v2 reads YAML 1.1, so the table above matters most: `yes`, `0777` and `<<` all
decode differently. v2 also accepts a repeated key and keeps the last value,
where this library returns an error. Pass `codec.AllowDuplicateMapKey()` to
keep the last value.

v2 decodes a mapping into an `any` as a `map[interface{}]interface{}`. This
library gives a `map[string]any` when every key is a string, and a
`map[any]any` otherwise.

`UnmarshalYAML(func(interface{}) error) error`, v2's method, is
`codec.GoYAMLUnmarshaler`, and is called as it was.

## Open questions for the API

- There is no `yaml.Node` equivalent with a stable, small surface. `ast.Node` is
  an interface over ~30 concrete types. A migrating caller has nothing to put in
  a struct field. Is that a gap, or the point of the fork?
- The decoder ignores a method named `UnmarshalYAML` whose signature matches
  none of its interfaces, such as v3's `UnmarshalYAML(*yaml.Node)` or goccy's
  context variants. Should it return an error instead?
