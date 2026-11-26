# ledgers

One place for the defect ledgers, which were scattered over four packages.

A ledger records what a package is known to get wrong, keyed by the case or the invariant that shows it, with the
extent written down. It is a measurement with a name attached, not an excuse: `Compare` fails when a defect appears
that is not recorded, and fails again when a recorded defect stops showing. A fix lands by deleting the entry.

## Layout

```
ledgers/
  ledger.go            Compare and CompareNames, the ratchets every ledger is held with
  scanner/             one directory per package under measurement
    scan_test.go       helpers shared by that package's ledgers
    position_test.go   positionLedger
    offset_test.go     offsetMissLedger
    extent_test.go     extentLedger
    state_test.go      stateLedger, behind -tags yamlprobe
  conformance/         the five YAML Test Suite harnesses and their ledgers; see its README
```

A directory here holds test files and nothing else, so nothing in the library imports it. That is deliberate: it
means this tree can be given its own `go.mod` the day a ledger needs a dependency the library should not carry.
Until then it runs under `go test ./...` like everything else, and a change that breaks a ledger breaks it locally.

## Moving a ledger here

The scanner's ledgers and the five YAML Test Suite harnesses are here. `parser.keyLedger` is still in `parser`.
`codec`'s `walkStreamHoldOuts` stays beside `codec`, because its test decodes with an unexported option, and the
`yamlgen` ledger stays in the `internal/testintegration` module, which requires conformance-suites.

1. Make a directory named after the package, with test files in `package <name>_test`.
2. Copy the ledger variable and the test that measures it, with the comment that says what each entry is for.
   Leave behind the tests that are not ledgers: `internal/scanner/position_test.go` and `probe_test.go` each held one
   of each, and only the ledger half moved.
3. Replace the comparison loop with `ledgers.Compare`, or `ledgers.CompareNames` when an entry records a reason and
   not a measurement. Both want a map holding only the names that show a defect, so
   filter out the clean ones first -- `state_test.go` does that with `probe.Checks`.
4. Copy whatever helpers the test needs. The packages under measurement keep their own test helpers unexported, and
   a `internal/.../internal/test*` package cannot be imported from here, so a short helper is copied rather than
   shared. `originsOf` in `scanner/offset_test.go` is the one that is.

## What runs, and when

`scanner/position_test.go` and `scanner/offset_test.go` run under `go test ./...`. `scanner/state_test.go` needs
`-tags yamlprobe`, which no CI job passes -- it went red for a day before anyone ran it. Run the tagged ones by hand
after touching the scanner:

```sh
go test -count=1 -tags yamlprobe ./internal/ledgers/...
```

A ledger measured over a generated corpus moves when the corpus grows, which is not the same as the code moving.
`scanner/README.md` says how to tell those apart before re-baselining anything.

`stateLedger` compares the number of disagreements only. It logs the number of checks and a fingerprint of the seeds
next to each count, so two runs show whether they read the same corpus, and it fails when a check it names ran zero
times.
