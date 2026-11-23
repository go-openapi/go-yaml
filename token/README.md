# token

Types to support YAML tokens.

## A token passes in registers

`Scanner.NextToken` returns `(Token, bool)` for every token of a document, and a document holds hundreds of
thousands of them. On amd64 a call passes up to nine registers of arguments and results. Go counts a struct's
fields, not its words: each leaf field takes one register whatever its width, a string takes two.

A token that takes more than eight registers pushes the `bool` onto the stack, and past nine the whole token goes
to the stack and back on every call. At eleven registers, `NextToken` wrote 64 bytes to the stack per call.

`TestTokenFitsTheRegisterABI` (`abi_test.go`) holds the token at eight:

| field | registers |
|---|---|
| `Value string` | 2 |
| `Position` (`Line`, `Column`, and `where` packing `Offset` and `IndentNum`) | 3 |
| `end int32` | 1 |
| `spans uint64` | 1 |
| `Type` | 1 |
| total | 8 |

A field added to `Token` or `Position` for a new fact would push the count to nine. Pack it into `spans` instead.

## `Token.spans`

From the low bit:

| bits | fact | read with | limit |
|---|---|---|---|
| 0-31 | the line the token's text ends on | `Token.EndLine` | |
| 32-47 | the line breaks taken by the comments above the token | `Token.CommentBreaksAbove` | 65,535, clamped |
| 48-61 | the line breaks taken by the whitespace after the token | `Token.TrailingBreaks` | 16,383, clamped |
| 62 | a blank line stands above the token | `Token.BlankLineAbove` | |
| 63 | a scanner cut the token from a document | `Token.FromSource` | |

`MakeX`, `Assemble` and `MeasureOrigin` set the first three, the scanner's `token.Lookback` sets the next two, and
the scanner calls `MarkFromSource` on every token it reads.

## How exact a position is

Over the YAML Test Suite, `Position.Offset` addresses its own token's text for 3,414 of 3,430 tokens (99.5%).
The 16 misses are 13 `Invalid` tokens, which an error carries and which the scanner builds from its whole origin
buffer, and 3 block scalar contents, whose offset it counts back from its cursor. `offsetMissLedger` in
`internal/ledgers/scanner/offset_test.go` records them by token type.

`positionLedger` in `internal/ledgers/scanner/position_test.go` holds two invariants over the same suite, and is
empty: no token has a column below 1, and no token is positioned before the token in front of it.

## `ParseWholeNumber` beside `ParseInteger`

`ParseInteger` returns an `int64` or a `uint64` in an `any`, which boxes every integer it reads. Converting
citm_catalog to JSON through it, the boxing took an eighth of all allocations. `ParseWholeNumber` returns the
magnitude and the sign, so a caller that writes the digits back out allocates nothing.

## `MeasureOrigin` reads an origin in three walks

A token's origin is the text the scanner read since the previous token: leading blanks, the token's text,
trailing blanks. `MeasureOrigin` walks it forward over the leading blanks, backward over the trailing ones
counting their breaks, and across the text between. Every `Make` call pays for those walks, so the count matters:
the three separate helpers it replaced walked the origin six times, three of them over the same leading blanks.
