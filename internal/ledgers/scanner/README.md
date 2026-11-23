# scanner ledgers

Three ledgers, measuring `internal/scanner` from three angles. Read this before changing a number in any of them.

| ledger | file | measures | over | runs |
|---|---|---|---|---|
| `positionLedger` | `position_test.go` | tokens whose position is malformed | the YAML Test Suite | always |
| `offsetMissLedger` | `offset_test.go` | tokens whose `Offset` misses their own text | the YAML Test Suite | always |
| `stateLedger` | `state_test.go` | pairs of scanner state that should agree and do not | the fuzz corpus | `-tags yamlprobe` |

All three are held by `ledgers.Compare`, which fails in both directions. The entries carry the reasoning; this file
carries what is true of the three together.

## The two that measure over the Test Suite

`positionLedger` and `offsetMissLedger` read `internal/yamltestsuite`, which is a fixed set of documents. Their counts
move only when the scanner moves, so a change to either number is a claim about the scanner and wants explaining in
the entry.

`positionLedger` is empty, and that is the interesting part: an empty ledger is a claim re-earned on every run. Keep
it empty. `offsetMissLedger` is not, and its entry says what the remaining 18 misses are.

## The one that measures over the fuzz corpus

`stateLedger` is different in a way that has misled before, so read this before touching a number in it.

It runs over `internal/fuzzseeds`, which reads the corpus at
`internal/testintegration/yamlcorpus/testdata/yaml-smoke.jsonl.gz`. That file copies the corpus of
`github.com/go-openapi/conformance-suites/yaml/corpus`, rewritten whenever that requirement moves, and it grows. **A count that moved does not mean the scanner moved.** On 2026-09-08 the
corpus went from 621 KB to 992 KB and all three live entries drifted, with the scanner standing still.

So before re-baselining:

1. **Read the ratio, not the count.** `lastIndentLevel==indentLevel`, retired 2026-11-20, went 2,729 -> 4,213 while its ratio went 3.26%
   -> 3.25%. The count followed the corpus; nothing about the scanner changed. The test logs `n of N (x%)` for every
   measured invariant, so the ratio is in the output of the failure you are looking at.
2. **Separate the corpus from the scanner.** Put the corpus back as it was when the number was last written down and
   run again:

   ```sh
   git show <commit>:internal/testintegration/yamlcorpus/testdata/yaml-smoke.jsonl.gz > /tmp/old.gz
   cp /tmp/old.gz internal/testintegration/yamlcorpus/testdata/yaml-smoke.jsonl.gz
   go test -count=1 -tags yamlprobe -run TestStateLedger ./internal/ledgers/scanner/
   git checkout internal/testintegration/yamlcorpus/testdata/yaml-smoke.jsonl.gz
   ```

   If it passes, the corpus grew and the new count is the baseline. If it fails, the scanner moved, and that is a
   finding rather than a number to update.
3. **Write down which it was**, in the entry, with both counts and both ratios.

An entry of `0` means the pair must never disagree. Those are load-bearing: `state_test.go` measures a clean
invariant the ledger names, at 0, precisely so a `0` entry keeps asserting something.

`/tab` reads 100% by construction and always will -- `updateIndent` takes a tab in leading whitespace without counting
it, so every space after a tab trips the probe. Reading that bucket as a count of anything else is a mistake made
twice already.

## Nothing here runs in CI

`stateLedger` needs `-tags yamlprobe`, and no CI job passes it. It went red for a day before anyone noticed. Run it
by hand after touching indentation, buffering or token extents:

```sh
go test -count=1 -tags yamlprobe ./internal/ledgers/scanner/
```
