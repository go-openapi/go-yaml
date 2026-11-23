---
title: A stream of documents
weight: 40
description: |
  Read a multi-document stream one document at a time with NewDecoder, go on
  past a document that fails, and stop where the stream cannot be read.
---

The task: read a file of jobs separated by `---`, one `Job` per document, and
keep the good ones when one document is wrong.

## The type and the stream

{{< code file="decoding/stream/example_test.go" lang="go" region="types" >}}

{{< code file="decoding/stream/example_test.go" lang="go" region="source" >}}

## The loop

{{< code file="decoding/stream/example_test.go" lang="go" region="loop" >}}

`Decoder.Decode` reads the next document into the value it is given, and
returns `io.EOF` once the stream holds no more. Decode each document into a
fresh value: a value reused across calls keeps the fields the next document
leaves out.

## A document that fails

{{< code file="decoding/stream/example_test.go" lang="go" region="run" >}}

{{< code file="decoding/stream/example_test.go" lang="text" output="Example" >}}

A value that does not fit its field leaves that field alone. `Decode` reads the
rest of the document, fills `Name`, and returns the error. The next call reads
the next document.

## A document that cannot be parsed

{{< code file="decoding/stream/example_test.go" lang="go" region="broken" >}}

{{< code file="decoding/stream/example_test.go" lang="go" region="runbroken" >}}

{{< code file="decoding/stream/example_test.go" lang="text" output="Example_syntaxError" >}}

A syntax error stops the stream: the parser cannot tell where the broken
document ends, so `purge` is never read. Every later call returns the same
error, never `io.EOF`, which is why the loop tests `errors.ErrSyntax` and
`errors.ErrExceededMaxDepth` and returns.

## Variations

- **A repeated key** fails its own `Decode` with `errors.ErrDuplicateKey`, and
  the next call reads the next document.
- **Anchors** do not cross a `---`: an alias in one document cannot name an
  anchor declared in another.
- **One document only.** `codec.Unmarshal` decodes the first document of a
  stream and still reads the others, returning an error if one cannot be
  parsed.

The decoder's entry points: [Marshal and Unmarshal](../../../values/marshal-unmarshal/#reading-a-stream-of-documents).
