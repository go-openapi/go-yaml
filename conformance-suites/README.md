# conformance-suites

Conformance corpora and the tools that build them, for JSON and YAML.

This module is a tenant. It lives here while it is being separated from the parser it grew up beside, and
it moves to `github.com/go-openapi/conformance-suites` once the separation is complete. Its module path is
already the destination, so the move is a directory rename and the deletion of a `replace` line.

## The one rule

**Nothing in this module imports `github.com/go-openapi/go-yaml`, in test files included.**

A corpus that depends on one parser cannot score another, and a corpus that stores one parser's verdict is
storing that parser's opinion as though it were the language. Consensus against a real parser is a
different job, and it gets a module of its own.

## What is here

| package | holds |
|---|---|
| `suite` | the on-disk form of a corpus: `Reader`, `Writer`, `Header`, `Case`, `Meaning` |
| `stance` | the vocabulary a corpus is labelled with: `Tag`, `Stage`, `Stand`, `Table`, `Rules`, `Shape` |
| `grammar` | the recognizer: a compiler from a published grammar, and the YAML 1.2 productions |
| `internal/yamltestsuite` | the vendored YAML Test Suite, read by the recognizer's calibration tests |

`suite` and `stance` import nothing outside the standard library. `grammar` adds the test framework and
nothing else.

The vendored fixtures come from `yaml/yaml-test-suite` and `go-yaml` vendors them too, so a version bump
upstream touches both copies.

## Reading an artifact

An artifact is gzipped JSONL: a header object on the first line, then one case per line, with the document
base64-encoded in `src`. Anyone with a JSON reader and base64 can read it -- no Go, no toolkit, no oracle.
From Go:

	header, cases, err := suite.ReadAll(f)

A case carries the grammar's verdict in `WellFormed` and the implementation-defined properties it exhibits
in `Tags`. Turn those into an expectation with a `stance.Table` holding your own positions: the corpus
states what the document is and what questions it raises, and your table states how you answer them.

## What comes next

The generators, the recognizer and the corpora themselves, each once it no longer depends on the parser.
The adapters that run other implementations for consensus go into a module of their own, which is the one
place a real parser is imported.
