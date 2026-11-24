---
title: Examples
weight: 40
description: |
  Code snippets that illustrate all features, with the output on display.
---

Each example page illustrates one task and works it through.

The code comes from `Example` functions in [`docs/examples`][examples].

> This is a module of its own, with actual tests run in CI that check every output printed here.

To run a recipe:

```cmd
cd docs/examples
go test -run Example -v ./decoding/unknownfields/
```

> Examples for [Decoding into Go values](../values/) and [Working with documents](../documents/)
> each describe how to use options and type in full. The dedicated examples below show them at work.

{{< children type="card" description="true" depth=1 >}}

[examples]: https://github.com/go-openapi/go-yaml/tree/master/docs/examples
