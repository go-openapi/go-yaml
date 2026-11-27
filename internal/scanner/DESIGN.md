# Design notes - the YAML scanner

Companion to the package documentation in [`doc.go`](doc.go).

The godoc says what [`Scanner`](scanner.go) offers a caller. This document says how it is built and why:
the layering, the two buffers the scan keeps, how a token's position and extent are worked out, the memory
layout, and the measurements that settled each of them.

Audience: whoever changes this package next.

The scanner is one stage of a pipeline. It reads bytes and returns tokens; the parser above it builds nodes,
resolves anchors and owns the `%YAML` directive. Nothing here knows what a document means.

---

## 1. Guiding principles

Four constraints, and they pull against each other.

1. **Point into the source, do not copy it.** `Init` takes the caller's bytes without copying, and a token's
   `Value` is normally a slice of them. Only a scalar the scan has to rewrite - an escape, a folded break, a
   chomped tail - gets a buffer of its own.
2. **One token or two in flight.** The scan hands each token over as it cuts it. `Context.pending` holds two
   at its fullest, whatever the document's length; `TestBufferHoldsTwoTokens` checks that bound.
3. **Refuse early and name the character.** A source the YAML 1.2 grammar does not admit stops the scan at the
   byte that broke it, with a position and the text around it. See section 8.
4. **Nothing per byte that can be done per token, and nothing per token that can be done once.** The scan loop
   runs for every character of the document, so what lives in it is what the design is about.

(1) and (4) fight: keeping a token's text as a window into the source means the scan must track where that
window starts and ends, which is per-byte work. Section 4 is how that is paid for.

---

## 2. Layering

```
                  parser  (package parser)
                     |  pulls one token at a time
                     v
   +--------------------------------------------------+
   |  Scanner            scanner.go                    |
   |    Init, SetSchema, Tokens, NextToken, Err        |
   |    scan()  - the per-character loop and its switch|
   |    the scanXxx functions, one per indicator       |
   +---------------------+----------------------------+
                         |
   +---------------------v----------------------------+
   |  Context            context.go                   |
   |    tokens read and not yet handed over            |
   |    what the tokens already emitted say            |
   |    the open block scalar                          |
   +---------------------+----------------------------+
                         |  embeds, at offset 0
   +---------------------v----------------------------+
   |  cursor             cursor.go                     |
   |    src, idx, size   - where the scan stands       |
   |    buf              - the value being built       |
   |    originStart/End  - the text being read         |
   +--------------------------------------------------+

   swar/               eight bytes at a time, one mask per word, never a loop
   internal/probe      counts and invariants, compiled out without -tags yamlprobe
   internal/testscanner  workload loading and the position ledger, for tests
```

`token` is the leaf everything returns: `token.Token` is a 56-byte value, and `token.Position` stores lines,
columns, offsets and indentation as int. That int is why the scanner counts in int too (section 6).

---

## 3. The scan loop

`Scanner.scan` reads one character, dispatches on it, and returns as soon as a token has been buffered. The
source stays where it is, so the next call continues from there. Two entry points drive it:

- `NextToken` pulls one token at a time. The parser uses this.
- `Tokens` returns an `iter.Seq` and pushes each token into the loop as it is cut, buffering none of them.

`TestPushAndPullAgree` holds the two to the same token stream.

`Tokens` is the faster of the two by **15 to 20 ns a token**, measured 2026-09-06 interleaved over the twenty-six
benchmark shapes, n=10: -9.98% geomean, every shape faster, and 144 fewer bytes allocated because `pending` never
grows. The saving is flat per token, so the percentage tracks how cheap the token is - 20.8% on `flow` at 87 ns a
token, 4.4% on `blockscalars` at 471 ns. The pull path spends what this saves on one `NextToken` call, one re-entry
into `scan`, one append into `pending` and one copy back out.

Weigh that against the whole parse before moving the parser to the push side: `golang_source` costs 382 ns a token to
parse, 112 ms over 293,142 tokens, so 16 ns is about 4% of it. `internal/analysis/pull_bench_test.go` carries an older
+18.4% figure that is sometimes read as this comparison. It prices `iter.Pull`'s coroutine bridge at ~62 ns a token,
which is a different thing, and its conclusion - pull natively, do not bridge - is what `NextToken` already does.

The dispatch is a switch on the character. Most arms call a `scanXxx` that either claims the character and
returns true, or declines and lets the switch fall through to the plain-scalar path at the bottom. An arm that
refuses returns an error carrying the token it refused.

Before the switch, `updateIndent` runs for every character. It tracks the indentation of the current line and
sets `indentState` to Up, Down or Keep, which is what tells the loop where a block construct ends.

---

## 4. The origin window, and the one case that breaks it

Every token carries the text the document wrote it as, indentation and line breaks included. The scan does not
copy that text. `cursor.originStart` and `cursor.originEnd` bracket it in `src`, and `addOriginBuf` moves
`originEnd` for each character read. `cursor.origin` then slices the source.

That holds for 107,805 of 107,811 reads over the fuzz corpus.

The exception is a line whose trailing spaces are cut. `removeRightSpaceFromBuf` takes a suffix off the text,
the scan reads on, and the result has a gap in its middle that no single window can express. The first cut
copies what the window held into `originCopy`, sets `originCut`, and everything after it appends to that copy.
Every reader of the text goes through `cursor.origin`, so the two cases are invisible above it.

`addOriginBuf` is worth reading before changing it. Appending each character to a buffer was 8% of the scanner:
a call the inliner refused at cost 106 against a budget of 80, wrapped around an append that copied a byte
already present in the source. The wide and cut cases live in `addOriginWide`, marked `//go:noinline`, to keep
`addOriginBuf` inside the budget. It runs for about one byte in a thousand.

---

## 5. The value buffer and its mark

`cursor.buf` holds the value of the token being read, and only for a token whose value the scan must rewrite.
`cursor.notSpaceCharPos` marks how much of it belongs to the value, leaving out the whitespace it ends with.

The mark means two different things, and this is the part that surprises:

- **Outside a block scalar it is derivable.** It equals the buffer's length less the whitespace and the fold
  break that `scanNewLine` appends. 135,723 reads over the fuzz corpus, no disagreement. Working it out at the
  read instead of marking per character is an open change worth about 1%.
- **Inside a block scalar it is not, and will not be.** `updateNewLineInFolded` keeps the space that folds a
  line, and a content line's trailing tab is dropped. Both sites set the mark outright, and no scan of the
  bytes tells a break that folds from one the block keeps. The mark records the choice the scanner made, not a
  property of the text.

`bufferedSrc` reads the mark and then applies the block scalar's chomping indicator. `bufferedToken` clears the
buffer and the mark together: leaving the mark behind let it outrun the buffer, and `buf[:mark]` on an emptied
buffer with room still in it is a byte the previous token wrote. Three reads in 137,129 hit that, all after a
block scalar whose content was whitespace.

---

## 6. Positions, extents, and int

`token.Position` counts lines and columns from 1, and offsets from 0. The scanner holds all of them in int, the
type `len` and `utf8.RuneCount` return, so counting converts nothing.

`Position` stores `Line` and `Column` as int32, and packs `Offset` in 40 bits and `IndentNum` in 24. Lines and
columns narrow in one place each: `posInt`, and `token.At`. `Init` refuses a source longer than `maxSourceLen`
(1 TiB less one byte, so the stream end's offset fits 40 bits). `lineLimits` bounds the rest in a second check,
which returns at once for a source under 16 MiB: a line of 2 GiB or more, a line opening with 2^24 blanks or more,
and more than `math.MaxInt32 - 1` line breaks. A column stands one past the last byte of its line, so each limit
leaves one for it. `.golangci.yml` excludes gosec's G115 on that argument. See [`source.go`](source.go).

Two things about a token's position are not obvious:

- **The cursor is not the offset.** The scan cuts a plain scalar only once it has established the scalar did
  not run on to the next line, and by then the cursor stands well past it. `cursor.textAt` finds the value in
  the source and returns the offset it was found at. For a value folding has rewritten there is nothing to
  find, and the offset comes from the origin's start plus its leading whitespace.
- **The extent is assembled, not measured.** The scan has already read every byte of the origin, so
  `Context.bufferedToken` closes the token from where the origin began plus its length, and takes the end line
  from the caller. Only a block scalar, which carries its own breaks, and a cut origin, which no longer stands
  in the source as one run, go back through `token.MeasureOrigin`. The `token.extentMatchesTheOrigin` probe
  checks the assembled extent against the measured one.

---

## 7. Memory layout

Both `Scanner` and `Context` live for as long as a document is read, one of each, so their layout is worth
choosing. Three rounds, each measured interleaved against `BenchmarkScannerNextToken` over its twenty-six
shapes, `sec/token`:

| change | size | per token |
|---|---|---|
| group the one-byte fields | `Scanner` 784 -> 720, `Context` 464 -> 424 | **-2.82%** geomean, twelve shapes faster, none slower |
| hold the counters in int32 | `Scanner` 720 -> 640, `Context` 424 -> 376 | +0.23%, a wash |
| `cursor` first, ordered by read frequency | unchanged | **-1.11%** geomean, thirteen faster, two slower |

`cursor` is 112 bytes and stands at offset 0 of `Context`. Its first 61 bytes are the seven fields every
character touches - `src`, `buf`, `idx`, `size`, `originEnd`, `notSpaceCharPos`, `originCut` - so one cache
line holds all of them. `raw`, which `indentRun` reads once a line, and `originCopy`, which only a cut fills,
sit past that line. **This one orders its fields by how often the scan reads each; everywhere else the package
orders fields by width.**

The same move on `Scanner` - grouping its twelve per-character fields and putting `ctx` behind them - measured
+0.49% and was not kept. It bundled two changes, the flags moving up and `ctx` moving from offset 240 to 40,
and only the pair was measured. Worth retrying as two.

**A 1-4% shift on one shape is not evidence here.** Adding a file to the package moves the `commented`
workload by about 3% with byte-identical per-token instructions, confirmed with `go tool objdump`. Read the
geomean over all twenty-six shapes, and for a claim that a change is neutral, compare the instruction streams.

---

## 8. What the scanner refuses

`Init` runs `validateSource` before a token is read, and the first scan returns what it found. Two checks:
the length bound of section 6, and `validateStream`, which holds every character to YAML 1.2's `c-printable` and
rejects a byte that belongs to no character. A byte that is not text would otherwise decode to U+FFFD and pass
unreported, which is why that check reads the string and not the runes the rest of the scanner works on.

Everything else is refused where it is read, by the `scanXxx` that met it, through `ErrInvalidToken`. The
refusal carries the token, so the parser can report where it stood. A stopped scanner serves the tokens it had
already read, then the token the refusal names, then nothing; `Err` returns the refusal.

Refusals worth knowing about, because each is a rule the grammar states and a reader may not expect:

- A byte order mark stands only in a document prefix. `nb-char` excludes it, so no node may hold one.
- A tab is never indentation. `s-indent(n)` is spaces and nothing else.
- A flow collection's later lines must be indented past the line that opened it.
- An escape whose digits name no code point, including a lone surrogate half. See `escapedRune` in
  [`string.go`](string.go).
- A tag the grammar has no production for, including `!<>`. See [`tagtext.go`](tagtext.go).

---

## 9. The probe ledger

`internal/probe` compiles out unless the build carries `-tags yamlprobe`: `probe.Enabled` is a constant false,
so `if probe.Enabled { ... }` and everything it would have evaluated disappear.

`Check` records how often an invariant held, `Count` how much work was done. A count is the honest instrument
for asking whether a parse stayed linear: it does not vary with the machine, where a stopwatch does.

Five checks stand in this package. `probe_test.go` holds each to a ledger of known divergences, so a new one
fails the build:

| probe | asks |
|---|---|
| `buf.notSpaceCharPos<=len(buf)` | the mark stays inside its buffer |
| `buf.notSpaceCharPos==trimmed/plain` | outside a block scalar the mark is derivable (section 5) |
| `buf.notSpaceCharPos==trimmed/block` | inside one it is not, and the ledger says how often |
| `token.extentMatchesTheOrigin` | the assembled extent equals the measured one (section 6) |
| `indent.indentNum==column-1/spaces` and `/tab` | the indentation count tracks the column |

Run them:

    go test -tags yamlprobe -run TestStateLedger ./internal/scanner/

**Placement is as delicate as the invariant.** An early `offset == ctx.idx` probe reported 394,369 failures and
was simply wrong: it sat where the assignment had not happened yet. A misplaced probe gives a confident wrong
answer.

---

## 10. Fast paths, and three that failed

`swar/` reads eight bytes in one word and returns a per-lane mask; the caller keeps its own loop. Every
function there is small enough to inline, and `TestInlinable` fails when one stops being. A helper that owned
the loop measured cost 98 against the budget of 80 in the JSON lexer this technique came from, and lost the
win it was written for. **Extract the per-word arithmetic, never the loop.**

Two loops use it. `firstUnprintable` tests a word for control characters, stepping through a word carrying a
byte over 0x7f one character at a time. `indentRun` finds the end of a run of leading spaces, after a
four-space probe that a document pays only until it shows one deeply indented line.

Three attempts to skip a run of characters in one go each broke something different:

1. **Quoted scalars** lost four origin recordings, so a token's extent came up short.
2. **Indentation, first try**: `swar.SpaceMask` borrowed across lanes and reported the first non-space two
   bytes late, so `  !!null` counted the `!!` as indentation. Fixed in the mask, not a scanner fault.
3. **Indentation, second try**: a token boundary moved on a blank line.

They are one problem. `idx`, `originStart`/`originEnd`, `indentNum`, `isFirstCharAtLine`, `notSpaceCharPos`
and the token-cut points are all advanced one character at a time, which keeps them consistent by accident. A
bulk advance has to make that consistency explicit. In particular it must not fire while `isFirstCharAtLine`
is true and the column has already advanced, which is where the second attempt went wrong.

`swar.DoubleQuoteStopMask` and `SingleQuoteStopMask` are written, tested and unused: three attempts to wire
them into the quote scanners were reverted for the reasons above.

One fast path was tried and refused on its own merits. The seventeen escapes in `scanDoubleQuote` that stand
for a single character repeat the same three statements, which invites a table. Two rewrites were measured and
both cost: a lookup table behind a five-case switch, +3.1% per token on `escaped-dense-1000` (p=0.047, n=12);
and one switch whose arms set only the character, +1.2% geomean (p=0.003, n=10). Go compiles the switch as
written into a jump table whose constants sit in the instruction stream, and both alternatives add either a
data load or a branch to every escape.

---

## 11. Maintenance playbook

### 11.1 Changing the scan

1. `go test ./...` from the repository root, not just this package: the parser, the AST and the conformance
   suites all read what the scanner emits.
2. Run the probes: `go test -tags yamlprobe -run TestStateLedger ./internal/scanner/`. A new divergence means
   either a defect or a ledger entry, and the ledger entry needs a reason.
3. `golangci-lint run` from this directory - it carries its own `.golangci.yml`, stricter than the root's.

### 11.2 Claiming a change is neutral or faster

Interleave. Run one round of each variant alternately in a single window, never against a stored baseline: a
single-shot comparison drifted 13% on a benchmark the change could not touch.

    go test -run=XXX -bench=BenchmarkScannerNextToken -benchmem -count=1 ./internal/scanner/

Read `sec/token` before `sec/op`: a change that emits fewer tokens moves `sec/op` without making the scan
faster. Read the geomean over all twenty-six shapes before any single one (section 7). For a claim of neutrality,
compare the instruction streams instead:

    go tool objdump -s 'scanner.\(\*Scanner\).scan$' ./scanner.test

### 11.3 Adding a refusal

Refuse at the `scanXxx` that reads the character, through `ErrInvalidToken`, and give it the token so the
position survives. Spell the message as the scanner spells the same complaint elsewhere, so the refusal
vocabulary stays one list. Then check `internal/testintegration/yamlcorpus`: its `Valid` flags and tag prose carry rulings no test
asserts, and moving the accept/refuse line without reading them moves the conformance score silently.

### 11.4 Adding state

Ask where it is read. Per character, and it belongs in [`cursor`](cursor.go), ahead of the gap, and the
measurement in section 7 applies. Per token, and it belongs on [`Context`](context.go). Once a document, and it
belongs on [`Scanner`](scanner.go) among the fields after `ctx`.

A field only ever written is a field to delete. `MultiLineState.isFolded` survived that way for a long time,
set by `setFolded` and read nowhere, because `!isLiteral` already answered the question.

---

## 12. Map of the source

| File | Role |
|------|------|
| `scanner.go` | `Scanner`, the exported surface, `scan` and its switch, positions |
| `context.go` | `Context`: the token buffer, what earlier tokens say, the open block scalar |
| `cursor.go` | `cursor`: the source, the cursor, the two buffers, and the per-character methods |
| `source.go` | `maxSourceLen`, `validateSource`, the int32 narrowing |
| `printable.go` | `c-printable`, the byte order mark, `validateStream` and its word loop |
| `string.go` | single and double quoted scalars, the escapes |
| `multiline.go` | block scalars: the header, the indicators, `MultiLineState` |
| `indent.go` | `IndentState`, the indentation of a line, the flow and continuation checks |
| `blanks.go` | whitespace, line breaks, tabs, and the indentation run |
| `tag.go`, `tagtext.go` | tags, and the grammar they are held to |
| `anchor.go`, `comment.go`, `directive.go`, `document.go`, `flow.go`, `map.go` | one construct each |
| `invalid.go` | the indicators no other scan function claimed |
| `error.go` | `InvalidTokenError` |
| `swar/` | eight bytes at a time, one mask per word |
| `internal/testscanner/` | workload loading and the position ledger |
