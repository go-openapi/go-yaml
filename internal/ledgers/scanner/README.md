# scanner ledgers

Four ledgers measure `internal/scanner`, each checked with `ledgers.Compare`. Read this before changing a number in
any of them. `../README.md` lists every ledger in the repository.

| ledger | file | measures | over | runs |
|---|---|---|---|---|
| `positionLedger` | `position_test.go` | tokens whose position is malformed | the YAML Test Suite | always |
| `offsetMissLedger` | `offset_test.go` | tokens whose `Offset` misses their own text | the YAML Test Suite | always |
| `extentLedger` | `extent_test.go` | accepted documents whose token extents do not tile the source | the Test Suite and the fuzz corpus | always |
| `stateLedger` | `state_test.go` | pairs of scanner state that should agree and do not | the fuzz corpus | `-tags yamlprobe` |

The entries carry the reasoning; this file carries what is true of the four together.

## Over the Test Suite

`positionLedger` and `offsetMissLedger` read `internal/yamltestsuite`, a fixed set of documents. Their counts move only
when the scanner moves, so a change to either is a claim about the scanner and wants explaining in the entry. Both
are empty.

## Over the fuzz corpus

`extentLedger` and `stateLedger` also read `internal/fuzzseeds`, which reads the corpus at
`internal/testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz`. That file copies the corpus of
`github.com/go-openapi/conformance-suites/yaml/corpus`, rewritten whenever that requirement moves. **A count that moved
does not mean the scanner moved.** On 2026-09-08 the corpus went from 621 KB to 992 KB and all three live entries of
`stateLedger` drifted, with the scanner standing still.

`stateLedger` compares the number of disagreements only. Its log line for each check reads `n of N (x%)`, and opens
with the seed corpus's fingerprint, so two runs show whether they read the same corpus.

Before changing a disagreement count:

1. **Read the ratio, not the count.** `lastIndentLevel==indentLevel`, retired 2026-11-20, went 2,729 -> 4,213 while
   its ratio went 3.26% -> 3.25%. The count followed the corpus; nothing about the scanner changed.
2. **Separate the corpus from the scanner.** Put the corpus back as it was when the number was last written down and
   run again:

   ```sh
   git show <commit>:internal/testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz > /tmp/old.gz
   cp /tmp/old.gz internal/testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz
   go test -count=1 -tags yamlprobe -run TestStateLedger ./internal/ledgers/scanner/
   git checkout internal/testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz
   ```

   Before the tier was renamed, the file was `yaml-smoke.jsonl.gz`: use that name in `git show` for older commits.

   If it passes, the corpus moved and the new count is the baseline. If it fails, the scanner moved, and that is a
   finding to explain, not a number to update.
3. **Write down which it was**, in the entry, with both counts and both ratios.

An entry of `0` means the pair must never disagree. `state_test.go` measures a clean check the ledger names, at 0, so
a `0` entry keeps asserting something, and it fails if that check ran zero times.
