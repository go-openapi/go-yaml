# Unfinished: the typed walk in codec

The decoder's typed walk is still marked "SPIKE" and needs finalizing.

- `Decoder.canWalkTyped` and `Decoder.walkInto` (`codec/walkstruct.go`) fill a concrete Go type while the parser
  reads, and fall back to a tree on `errNeedsTheTree`. `decodeInit` tries `canWalk`, then `canWalkTyped`, then the
  tree.
- It landed on 2026-09-06 (9035a26 "perf(codec): read a struct by walking the document, without a tree") and was
  extended until 2026-09-08.
- Why it exists: after disappointing benchmarks, to test whether holding the destination in memory (filled as the
  parser hands over nodes) beats holding the source and its tree.

## To resume

1. Compare the typed walk against the tree with `internal/benchmarks`.
2. Then either drop the SPIKE label and settle the cost of the fallback (a refused walk parses the source a second
   time), or remove the path.

## Related, already settled

- `walkValues` (the walk into a bare `*any`) is production code, chosen by `canWalk`. The exported wrappers
  `WalkValue` and `WalkValues` were removed in 97b6bbe.
- The `Decoder` still reads the whole input (`readAll`) before either walk, so neither is streaming.
