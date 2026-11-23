---
title: YAML to JSON and back
weight: 10
description: |
  Convert YAML to JSON with ToJSON and back with FromJSON, read input that must
  be JSON, and handle a value JSON cannot hold.
---

The task: serve a YAML configuration as JSON, accept JSON back, and reject
whatever does not fit.

## The document

{{< code file="conversion/tojson/example_test.go" lang="go" region="source" >}}

## Convert to JSON

{{< code file="conversion/tojson/example_test.go" lang="go" region="tojson" >}}

{{< code file="conversion/tojson/example_test.go" lang="json" output="Example" >}}

`ToJSON` builds no Go value. It walks the document and writes JSON as it reads:

- `1.10` keeps its text. A decode into `float64` would give `1.1`.
- `empty:` with no value is `null`.
- The comment is dropped: JSON has none.
- The alias `*base` writes what its anchor wrote.
- `<<` is an ordinary key, as YAML 1.2 reads it.

## Merge keys

{{< code file="conversion/tojson/example_test.go" lang="go" region="mergeKeys" >}}

{{< code file="conversion/tojson/example_test.go" lang="json" output="Example_mergeKeys" >}}

`parser.WithMergeKeys` makes `<<` merge the mapping it names, and changes
nothing else. `ToJSON` takes any [parser option](../../../documents/parser/).

## Back to YAML

{{< code file="conversion/tojson/example_test.go" lang="go" region="fromjson" >}}

{{< code file="conversion/tojson/example_test.go" lang="yaml" output="Example_fromJSON" >}}

`FromJSON` keeps the member order and the number's text. It quotes `"yes"`,
which would read back as a boolean under YAML 1.1, and writes a string over
several lines as a `|` block.

## Accept JSON only

{{< code file="conversion/tojson/example_test.go" lang="go" region="jsonSyntax" >}}

{{< code file="conversion/tojson/example_test.go" lang="text" output="Example_jsonSyntax" >}}

YAML reads all of these, since JSON is almost all YAML. `parser.WithJSONSyntax`
checks each token against JSON's grammar and rejects the document with
`errors.ErrNotJSONSyntax` at the first one that is not JSON: a bare key, a
trailing comma, a comment. `FromJSON` reads its input this way.

## A value JSON cannot hold

{{< code file="conversion/tojson/example_test.go" lang="go" region="notJSON" >}}

{{< code file="conversion/tojson/example_test.go" lang="text" output="Example_notJSON" >}}

JSON has no infinity, NaN or collection key. `ToJSON` returns an error that
matches `errors.ErrNotJSON`, with the position and the source around it.

Two keys YAML distinguishes can write one JSON member name:

{{< code file="conversion/tojson/example_test.go" lang="go" region="jsonName" >}}

{{< code file="conversion/tojson/example_test.go" lang="text" output="Example_jsonName" >}}

The integer `1` and the string `"1"` are two keys in YAML and one member `"1"`
in JSON. `ToJSON` rejects the pair, at the second key.

## Variations

- **Several documents.** `ToJSON` converts the first document of a stream, and
  still returns an error if a later one cannot be converted.
- **Tokens instead of one buffer.** See [JSON tokens as they arrive](../json-tokens/).
- **The full list of what converts.** See [The JSON bridge](../../../documents/json/).
