# Conformance harness

For anyone fixing the parser, the scanner or the renderer.

This module checks the library against YAML documents drawn from the generator
in [conformance-suites](https://github.com/go-openapi/conformance-suites), and
checks both against a recognizer compiled from the YAML 1.2 spec grammar. It is
a measurement rather than a sample: the YAML Test Suite is four hundred
documents someone thought to write down, and this explores the space around
them.

## What is broken right now

```sh
go test -v -run 'Outstanding|WronglyAccepted|ValidDocuments' ./internal/testintegration/yamlgen/
```

That is the work list. It passes either way — it reports rather than asserts,
so a suite red for known reasons still gets read. The `-v` is not optional: `go
test` swallows the report without it.

Three kinds of entry come out. The first two fail in opposite directions; the
third differs by how it was found rather than by what it says:

| | The library is | Where it is recorded |
|---|---|---|
| `still failing` | too strict, or reads a valid document wrongly | `ledger` in `yamlgen/ledger_test.go` |
| `still read` | too lax — it reads a document that is not YAML | `lax` in `yamlgen/ledger_test.go` |
| `still refused` | too strict, on a document nothing here generates | `strict` in `yamlgen/ledger_test.go` |

`ledger` names a *shape*, because the generator draws a different document
every run and there is nothing to point at. `Lax` and `Strict` name
*documents*: one that survived the mutation hunt, and one somebody met by hand
while fixing something else.

That last list exists because the harness cannot find its own blind spots. Every
entry in it is a document the generator does not produce, so nothing here was
watching it — and four of the five are an empty node standing where the
generator only ever puts a full one.

## Fixing one

Every entry names a document. Start there, not with the generator.

1. Reproduce with the document alone. The pinned cases are in
   `defects_test.go`, `laxity.go` and `strictness.go`, each small enough to
   paste.
2. Fix it.
3. Run the work list again. The entry flips to `NOW HOLDS`, `NOW REFUSED` or
   `NOW READ`.
4. Delete it — from `Ledger`, `Lax` or `Strict`, from the outstanding list, and from
   `defects_test.go`. Move the case to `fixed_test.go`, which is where shapes
   the generator once found go to stay found.

Leaving an entry in place after fixing it is what the suite is built to catch:
the property tests count how often each entry is drawn and how often it still
diverges, so one that has stopped diverging shows up as such.

## Hunting for more

Both hunts are behind flags, because both currently find something and a suite
that is red by default stops being read.

```sh
# valid documents the library refuses, or reads as the wrong value
go test -run TestInvariant ./internal/testintegration/yamlgen/ \
    -args -yamlgen.invariants -rapid.checks=100000

# documents that are not YAML 1.2 and that the library reads anyway
go test -run TestEveryDocument ./internal/testintegration/yamlgen/ \
    -args -yamlgen.laxity -rapid.checks=100000
```

Custom flags go after `-args` because `go test` validates the ones it does not
recognize against the package in the current directory, which is not this one.

Failures arrive reduced to the smallest document that still shows the problem,
with a test case to paste. The rarer shapes turn up a few times in a hundred
thousand, so a short run reports a success it has not earned.

## Before you believe a finding

The recognizer is `internal/testintegration/grammar`, compiled from the 211
productions the spec publishes as a data structure. It is the only thing here
that is neither the library nor our own emitter, which is what lets it say
which side of a disagreement is wrong.

It is not infallible, and the two directions do not rest on it equally.

A finding in `Ledger` needs the grammar's **acceptance** to be right. A finding
in `Lax` needs its **refusal** to be right — a much stronger claim, and one the
recognizer has already got wrong once: it refused a compact collection written
under a wider parent, which this library's renderer emits and every other
parser reads. So each `Lax` entry names the production it breaks, and that
production is what to check against the spec text. The recognizer is not
evidence for its own verdict.

It also cannot see anything that is not syntax. Alias resolution, distinct
mapping keys and tag meaning are all outside a grammar, so it will happily
accept documents that break them.

## What is where

| | |
|---|---|
| `yamlgen/ledger_test.go` | `ledger`, `lax`, `strict` — this library's known defects |
| `yamlgen/defects_test.go`, `fixed_test.go` | one pin per open defect, one per fixed one |
| `yamlcorpus/stance.go` | `GoYAML`, `GoYAML11`, `GoYAMLParser`, `Departures` — this library's stance tables |
| `yamlcorpus/testdata/` | a copy of the stored corpus, read by `internal/fuzzseeds` |

The generator, the corpus builder and the recognizer live in
[conformance-suites](https://github.com/go-openapi/conformance-suites):

| | |
|---|---|
| `yaml/yamlgen` | what a document means, how to write it down, how to break it, how to shrink a failure |
| `yaml/yamlgen/yamlbuild` | the corpus builder and its tag vocabulary |
| `yaml/corpus` | the stored corpus, embedded (package `yamlembed`) |
| `yaml/consensus` | adapters for `go.yaml.in/yaml/v3`, libfyaml and the YAML 1.2 reference parser |
| `yaml` | the YAML 1.2 recognizer |

The property tests are in `yamlgen/invariance_test.go` and
`yamlgen/roundtrip_test.go`. `TestRenderWritesValidYAML` gates our own side
rather than the library's, and a failure there is ours, not yours. Its twin for
the generator, `TestEveryEmittedDocumentIsValidYAML`, runs in conformance-suites.
