# ledgers

One place for the defect ledgers.

A ledger records what a package is known to get wrong, keyed by the case or the invariant that shows it. It is a
measurement with a name attached, not an excuse. It fails in both directions: a defect that is not recorded fails,
and a recorded defect that stops showing fails too. Record a fix by deleting the entry.

An empty ledger keeps its map and its comparison. The empty map is the claim that nothing diverges, re-checked on
every run, and the next defect found has a place to go.

## Every ledger

| ledger | file | measures | over | ratchet | runs |
|---|---|---|---|---|---|
| `positionLedger` | `scanner/position_test.go` | tokens whose position is malformed | YAML Test Suite | `Compare` | always |
| `offsetMissLedger` | `scanner/offset_test.go` | tokens whose `Offset` misses their own text | YAML Test Suite | `Compare` | always |
| `extentLedger` | `scanner/extent_test.go` | accepted documents whose token extents do not tile the source | YAML Test Suite and fuzz corpus | `Compare` | always |
| `stateLedger` | `scanner/state_test.go` | pairs of scanner state that should agree and do not | fuzz corpus | `Compare` | `-tags yamlprobe` |
| `keyLedger` | `parser/key_test.go` | the parser's mapping-key invariants that fail | fuzz corpus | `Compare` | `-tags yamlprobe` |
| `acceptanceLedger` | `conformance/acceptance_test.go` | cases the parser accepts or refuses against the suite | YAML Test Suite | `Compare` on the verdict | always |
| `decodeLedger` | `conformance/decode_test.go` | cases `Unmarshal` does not decode to `in.json` | YAML Test Suite | `CompareNames` | not on Windows |
| `jsonLedger` | `conformance/json_test.go` | cases `codec.ToJSON` does not write as `in.json` | YAML Test Suite | `CompareNames` | always |
| `jsonTokenLedger` | `conformance/jsontokens_test.go` | the same, through `codec.ToJSONTokens` | YAML Test Suite | `CompareNames` | always |
| `roundTripLedger` | `conformance/roundtrip_test.go` | accepted cases that do not survive render and reparse | YAML Test Suite | `Compare` on the outcome | always |
| `walkStreamHoldOuts` | `codec/walkvalue_internal_test.go` | documents the walk and the tree decode differently | fuzz corpus | `CompareNames` | always |

Five of the eleven hold entries today: `stateLedger` (6, all at 0), `keyLedger` (1, at 0), `decodeLedger` (31),
`jsonLedger` and `jsonTokenLedger` (2 each). The other six are empty.

Two live outside this tree:

- `walkStreamHoldOuts` stays in `codec`. `TestWalkMatchesTheStream` is `package codec` and decodes with the unexported
  `treeOnly()`, which a package here cannot call.
- The `yamlgen` registers (`ledger`, `lax`, `strict` in `internal/testintegration/yamlgen/ledger_test.go`) stay in the
  `internal/testintegration` module, which requires conformance-suites. They are lists of shape predicates that rapid
  property tests consult through `yamlgen.Known` on every draw, so there is no fixed set of names for `Compare` to
  measure. `TestEveryLedgerEntryNamesItsPin` and the `TestFixed*` pins in `fixed_test.go` keep them honest.

`conformance/noexpectation_test.go` is a pin table, not a ledger: it states what each fixture `decodeLedger` files under
`reasonNoExpectation` decodes to.

## The two ratchets

`Compare` checks the value of each entry against the measurement: a count, a verdict, an outcome, a defect's
description. Use it when the entry records what was measured.

`CompareNames` checks the names only. Use it when the entry records why a case diverges: the measurement and the
reason are two different strings and would never match. Its failure message carries the measured difference for a new
case and the recorded reason for a stale one.

Both want a map holding only the names that show a defect. Filter out the clean ones first, as `state_test.go` does
with `probe.Checks`. A ledger whose entry pairs a measurement with a reason (`acceptanceLedger`, `roundTripLedger`)
compares the measurement half with `Compare` and keeps the reason for the reader.

## What may fail a ledger

The defect count, and nothing else. `stateLedger` and `keyLedger` log the number of checks next to each count and do
not compare it: it moves with the corpus and with any scanner fix that changes what gets scanned. A check the ledger
names that ran zero times fails, because the probe stopped being reached.

## Re-baselining

Over the YAML Test Suite (`internal/yamltestsuite`, a fixed set of documents), a count moves only when the library
moves. A change is a claim about the library and wants explaining in the entry.

Over the fuzz corpus (`internal/fuzzseeds`, which reads
`internal/testintegration/yamlcorpus/testdata/yaml-compact.jsonl.gz`), a count can also move because the corpus did.
That file copies the corpus of `github.com/go-openapi/conformance-suites/yaml/corpus`, and is rewritten whenever that
requirement moves. `scanner/README.md` says how to tell the two apart before changing a number.

Key a document by its test suite case name, or by a hash of its text for a fuzz seed (`extentLedger`,
`walkStreamHoldOuts`), never by a seed's index or a commit SHA: a regenerated corpus renumbers every seed, and a rebase
rewrites every SHA.

## Running them

A directory here holds test files only, and nothing in the library imports it.
The tree runs under `go test ./...` like everything else, and a change that breaks a ledger breaks it locally.

```sh
go test -count=1 ./internal/ledgers/...                    # all but the two probe ledgers
go test -count=1 -tags yamlprobe ./internal/ledgers/...    # all of them
```

CI runs both: the shared test job, and the `ledgers` job in `.github/workflows/go-test.yml`, which runs
`go test -tags yamlprobe ./...` over the root module.

## Adding a ledger here

1. Make a directory named after the package under measurement, holding test files only, in `package <name>_test`.
2. Copy the ledger variable and the test that measures it, with the comment that says what each entry is for. Leave
   behind the tests that are not ledgers.
3. Replace the comparison loop with `Compare` or `CompareNames`.
4. Copy whatever helpers the test needs. A package keeps its test helpers unexported, and an `internal/.../internal/test*`
   package cannot be imported from here, so a short helper is copied. `originsOf` in `scanner/offset_test.go` is one.
