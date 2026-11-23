---
title: Examples
weight: 40
description: |
  Recipes: one task per page, walked through a snippet at a time, with the
  output each step prints.
---

Each page takes one task and works it through. The code comes from `Example`
functions in [`docs/examples`](https://github.com/go-openapi/go-yaml/tree/master/docs/examples),
a module of its own, and `go test` checks every output printed here. To run a
recipe:

```cmd
cd docs/examples
go test -run Example -v ./decoding/unknownfields/
```

The [Decoding into Go values](../values/) and [Working with documents](../documents/)
sections describe each option and type in full. These pages show them at work.

{{< children type="card" description="true" depth=1 >}}
