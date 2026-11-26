# conformance ledgers

Five harnesses measure this library against the YAML Test Suite (`internal/yamltestsuite`), and each holds a ledger of
the cases it expects to diverge, with the reason.

| ledger | file | test | measures |
|---|---|---|---|
| `acceptanceLedger` | `acceptance_test.go` | `TestSuiteAcceptance` | the parser accepts each valid case and refuses each invalid one |
| `decodeLedger` | `decode_test.go` | `TestYAMLTestSuite` | `Unmarshal` decodes the value the case's `in.json` states |
| `jsonLedger` | `json_test.go` | `TestSuiteToJSON` | `codec.ToJSON` writes the JSON the case states |
| `jsonTokenLedger` | `jsontokens_test.go` | `TestSuiteToJSONTokens` | `codec.ToJSONTokens` writes the same JSON |
| `roundTripLedger` | `roundtrip_test.go` | `TestSuiteRoundTrip` | a rendered document reads back and renders the same |

`noexpectation_test.go` is not a ledger. `noExpectationPins` says what each fixture `decodeLedger` files under
`reasonNoExpectation` decodes to, since those fixtures carry no `in.json`, and
`TestEveryFixtureStatingNoExpectationHasAPin` keeps the two lists in step.

The Test Suite is a fixed set of documents, so a count here moves only when the library moves. A case that starts
diverging fails as a regression, and a recorded case that stops diverging fails too: record a fix by deleting its
entry.

```sh
go test -v ./internal/ledgers/conformance/
```

Each test logs its counts.
