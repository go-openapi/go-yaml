# Linting style

`.golangci.yml` enables every linter (`default: all`) and disables a few by name.
This file records why, for the choices that are not obvious from the configuration.

A finding that stays is marked in place with `//nolint:<linter> // <reason>`.
Prefer disabling a linter here over scattering the same directive across the code.

## Disabled linters

### nilnil

A function here returns `nil, nil` in two cases, and both are results, not omissions:

- the value is nil: a YAML null decodes to a Go nil, as in `nodeToValue`, `scalarValue` or a `!!null` tag;
- nothing is there: an empty stream (`walkFirst` in the codec tests), a null where an array is read (`getArrayNode`), a drained
  reader (`reader.peek`), or no entry node when comments are off (`sequenceEntry`).

A sentinel error would cost an allocation at the return and an `errors.Is` at every caller, and in the first case
it would be wrong: the document wrote a null, and nil is its value.

## Settings

### Complexity: cyclop, gocyclo, gocognit

The three thresholds are 35. cyclop and gocyclo both count cyclomatic complexity, and both are kept while we
learn where they differ.

A function over the limit is split where a step stands on its own, as `parseFlowMap` and `decodeMap` were.
A function holding one case per node kind, `reflect.Kind`, token type or rune carries a nolint naming that:
`scan`, `scanDoubleQuote`, `nodeToValue`, `decodeValue`, `encodeStruct`, `Type.String`.

### mnd

The powers of two used for shifts and masks, and 10, are exempt: 2, 4, 8, 10, 16, 32, 63 and 64.
The exclusion matches `Magic number: <n>,`, comma included, so that 24 or 1024 still report.
Any other number gets a named constant.

### gosec G115

Integer conversions that may overflow are excluded by rule. Positions, offsets and indents are int32 throughout
the scanner, the parser and `token.Position`: a document that overflows one does not fit in memory, and a token
holds its position by value, so the width is the token's width. The exclusion goes with a move to int64, which
is not studied yet.
