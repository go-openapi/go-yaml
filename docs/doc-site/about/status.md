---
title: Status
weight: 40
description: |
  The API is still in flux. The list of what is not built yet.
---

{{% notice style="warning" title="Experimental" %}}
There is no tagged release yet. `go get github.com/go-openapi/go-yaml` resolves a
pseudo-version on `master`, and the API changes without notice.
{{% /notice %}}

## Can I depend on this?

Not blindly yet :) But we are steadily improving the library.

The parts that remain very much in flux are the `ast` and the `transform` package.
`codec` and `parser` should be steadier now.

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

Releases will appear on the [GitHub releases page][releases]

[releases]: https://github.com/go-openapi/go-yaml/releases
