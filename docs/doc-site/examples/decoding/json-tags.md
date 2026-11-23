---
title: A config with json tags
weight: 10
description: |
  Decode into a type tagged for encoding/json with UseJSONTags. Without it,
  the json tags name nothing, and the decode returns no error.
---

The task: read a YAML file into a type that was written for `encoding/json`,
with `json` tags and no `yaml` tags.

## The type and the file

{{< code file="decoding/jsontags/example_test.go" lang="go" region="types" >}}

{{< code file="decoding/jsontags/example_test.go" lang="go" region="source" >}}

## The default reads the yaml tag alone

{{< code file="decoding/jsontags/example_test.go" lang="go" region="default" >}}

{{< code file="decoding/jsontags/example_test.go" lang="text" output="Example" >}}

The decode returns no error, and only `Timeout` is filled. The decoder follows
`go.yaml.in/yaml/v3`: it reads the `yaml` tag and nothing else, so
`listen_addr` and `max_conns` match no field and are skipped.

A field with no `yaml` tag takes its Go name in lower case:

{{< code file="decoding/jsontags/example_test.go" lang="go" region="lowercased" >}}

{{< code file="decoding/jsontags/example_test.go" lang="text" output="Example_lowercased" >}}

## Decode with UseJSONTags

{{< code file="decoding/jsontags/example_test.go" lang="go" region="jsontags" >}}

{{< code file="decoding/jsontags/example_test.go" lang="text" output="Example_useJSONTags" >}}

`UseJSONTags(true)` names a field by its `json` tag wherever the `yaml` tag
leaves off. The `yaml` tag still wins: `Timeout` reads `timeout_s`, not
`timeout`.

The option also promotes the fields of an untagged embedded struct, as
`encoding/json` does, and lets a key match a field name that differs only in
case.

## Variations

- **Catch the silent miss.** Add `codec.DisallowUnknownField()`: under the
  default, `listen_addr` then fails the decode. See
  [Reject unknown fields](../unknown-fields/).
- **No tags at all.** `codec.UseInferredNames(true)` names an untagged field by
  its Go name verbatim, `ListenAddr`, instead of `listenaddr`.
- **The type has an `UnmarshalJSON`.** `UseJSONTags` fills the fields directly
  and does not call it. `codec.UseJSONUnmarshaler()` converts the document to
  JSON and calls it.

The naming rules in full: [Struct tags](../../../values/struct-tags/).
