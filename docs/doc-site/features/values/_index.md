---
title: Decoding into Go values
weight: 10
description: |
  The codec: turning YAML into Go values and back.

  Marshal and Unmarshal, the options that steer them, struct tags, anchors,
  comments and errors.
---

Everything on these pages is reachable from
[`codec`](https://pkg.go.dev/github.com/go-openapi/go-yaml/codec). The root
package re-exports `Marshal`, `Unmarshal`, `ToJSON` and `FromJSON`; anything
with options lives in `codec`.

{{< children type="card" description="true" >}}
