---
title: Posture
weight: 30
description: |
  Our posture on YAML quirks.
---

## The grammar

The YAML grammar is a highly complex beast. The 1.1 spec is not even really a grammar, and rather emerged gradually from usage and
consensus. YAML 1.2 (and later [1.2.2][yamlgrammar]) has been a huge effort by the YAML authors to formalize YAML with a proper grammar.

Thanks to that effort, it has become possible to produce _generated_ parsers.

However, this grammar is not perfect, nor is it even close to fully describe the YAML language. Many aspects of it are
ruled by plain English statements in the spec alone, sometimes in a non-prescriptive way, and some of it may be interpreted in different ways.

[yamlgrammar]: https://github.com/yaml/yaml-grammar/blob/master/yaml-spec-1.2.yaml

## The consensus

## YAML 1.1 vs 1.2

## YAML 1.2 quirks and ambiguities

### Multiple documents in a stream
### Anchors and aliases
### Empty values, implied values, non-specific tag
### Explicit vs implicit keys
### Honoring tags
### Trailing CRLF
### Non-printable characters
### BOM and encoding

## Adding up go quirks

### Ordered maps
### Non-comparable keys
### Numbers
### Binary strings
### Time
