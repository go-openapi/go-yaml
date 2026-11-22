---
title: Status
weight: 40
description: |
  No release yet, an API that still moves, and the list of what is not built.
---

{{% notice style="warning" title="Experimental" %}}
There is no tagged release. `go get github.com/go-openapi/go-yaml` resolves a
pseudo-version on `master`, and the API changes without notice.
{{% /notice %}}

## Can I depend on this?

Not with any promise. Any exported name may still be renamed or removed.
`parser.ParseFile`, `codec.MarshalWithOptions` and the `context.Context`
variants of the codec functions were removed in 2026.

Pin a pseudo-version if you depend on it now. If you need a stable library
today, use [`goccy/go-yaml`](https://github.com/goccy/go-yaml), the project this
one forked from, or [`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml) to
decode YAML into Go values.

## Not built

- **Streaming.** Every entry point holds the whole document. `codec.NewDecoder`
  takes an `io.Reader`, but reads all of it on the first call to `Decode`, before
  it parses the first document.
- **Documents of 2 GiB or more.** The scanner rejects a source of
  2,147,483,647 bytes or more, because a `token.Position` stores its offsets as
  `int32`.

## Not planned

- **Encodings other than UTF-8.** Convert a UTF-16 or UTF-32 document to UTF-8
  before you parse it.

## Where to watch

Releases will appear on the
[GitHub releases page](https://github.com/go-openapi/go-yaml/releases).

{{% notice style="info" %}}
This page is the one place on the site allowed to describe what does not exist.
Everywhere else, a page describes committed code or says nothing.
{{% /notice %}}
