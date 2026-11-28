# YAML benchmark workloads

Seven documents the parser is measured on. Four are the JSON corpus every JSON
parser is compared on; the fifth is an OpenAPI specification, which is the shape
this library actually meets; the sixth is that specification with comments
written over it; the seventh is `citm_catalog` written in flow style.

| file | shape | bytes |
|------|-------|------:|
| `canada_geometry.yaml.gz` | deep sequences of floats (geo coordinates) | 398,206 |
| `citm_catalog.yaml.gz` | wide mappings of short string values | 588,395 |
| `golang_source.yaml.gz` | deeply nested tree (a Go AST dump) | 3,790,249 |
| `twitter_status.yaml.gz` | unicode-rich strings, mixed shapes | 506,987 |
| `azure_swagger.yaml.gz` | an OpenAPI 2.0 specification | 544,150 |
| `commented_swagger.yaml.gz` | the same specification, annotated | 601,617 |
| `citm_catalog_flow.yaml.gz` | `citm_catalog` in flow style | 720,341 |

## Where they came from

The JSON originals live in `go-openapi/core` at
`json/testdata/workloads/{standard,additional}/`, where they benchmark that
repository's JSON lexer and parser. `standard/SOURCE.md` there records their own
provenance: the `encoding/json/v2` reference corpus, BSD-licensed, Copyright The
Go Authors.

Only the YAML is stored here. Regenerating needs a checkout of `go-openapi/core`
beside this one:

```sh
cd internal/analysis/workloads
go run ./gen -json ../../../../core/json/testdata/workloads -out testdata
```

`gen/main.go` is the rewriting described below, so the stored files and the tool
that made them do not drift apart.

`commented_swagger.yaml.gz` and `citm_catalog_flow.yaml.gz` need no such
checkout: they are written over the stored `azure_swagger.yaml.gz` and
`citm_catalog.yaml.gz`. Leave `-json` out to rebuild only those two.

```sh
go run ./gen -out testdata
```

## How they were rewritten

JSON is YAML, so these files would parse as they stand -- and would exercise flow
collections and double-quoted scalars and nothing else. Each was decoded with
`encoding/json` keeping object key order, then written with `yaml.Marshal` from a
`yaml.MapSlice`, which lays a document out the way a person writes YAML: block
mappings and sequences, plain scalars, quotes only where the scalar needs them.

Two adjustments, both recorded because they make the YAML something other than a
transcription of the JSON:

- **CRLF becomes LF inside a scalar.** YAML normalizes a line break, so a string
  holding `\r\n` cannot survive being written and read back. 7 strings in
  `twitter_status`.
- **A string that would resolve to something else is double-quoted.** `088253`
  written plain is a string to this library and the integer 88253 to
  `go.yaml.in/yaml/v3`. 4 strings in `twitter_status`, all colour codes.

Every file was then checked by decoding the YAML with `go.yaml.in/yaml/v3` -- not
with this library, so the check does not rest on the parser it exists to measure
-- and comparing against the JSON decoded with `encoding/json`, with every number
reduced to a `float64` and the two adjustments above applied to both sides. All
five compare equal.

## The commented workload

The five documents above hold no comment at all -- they came from JSON, which
has none. That leaves the comment code unmeasured, and it is not a small
corner: the scanner drops a comment where the mode does not ask for one,
`attachLineComments` lifts a line comment out of the token stream and records
it in a map that lives for the whole parse, and the descent turns the comments
that remain into head and foot comments. On a 5,000-entry mapping annotated on
every line, `ParseComments` raised what a parse retains by 74%.

`commented_swagger.yaml.gz` is `azure_swagger.yaml.gz` with comments written
over it by `gen/comments.go`. What it holds, of 37,035 tokens:

| | count |
|---|---:|
| comment tokens | 1,563 (4.2% of tokens) |
| line comments, lifted out of the stream by the grouping | 655 |
| standalone comments, read by the descent | 908 |
| foot comments, attached to a block after it closed | 193 |
| head comments filed under `SequenceNode.ValueHeadComments` | 70 |

Five kinds are placed, at strides chosen so they rarely coincide: a block at
the top of the file, a heading before a section, a heading before a sequence
entry, a note at the end of a value's line, and a remark closing a block.
Placement counts candidate lines rather than drawing at random, so rerunning
`gen` writes the same file.

The sequence-entry headings are there for one structure. A head comment on a
sequence entry is the only comment the parse files in
`SequenceNode.ValueHeadComments`, a slice indexed alongside `Values` and read
back by `ast/render.go` -- so the entry's comment is held by the sequence for
as long as the sequence lives, and a second time on the `SequenceEntryNode`.
Without one of these the slice is empty in every workload.

The foot comments are the ones to keep an eye on. A foot comment is read after
the block it belongs to has closed, and the parser writes it into an entry it
had already finished -- so a caller consuming entries as they complete cannot
be handed the last entry of a block until the following token settles whether a
foot comment attaches to it.

Comments were checked to change nothing else: `gen` decodes both documents with
`go.yaml.in/yaml/v3` -- not with this library, so the check does not rest on the
parser the corpus exists to measure -- and compares. It refuses to store a file
that fails.

## The flow-style workload

The other six hold no flow collection with entries in it. `citm_catalog`,
`golang_source` and `twitter_status` write thousands of `[]`, and a handful of
one-element collections, so the code that reads a flow collection's entries
runs almost never. A difference that shows only on flow-style documents, as
JSON read as YAML is, cannot show on them.

`citm_catalog_flow.yaml.gz` is `citm_catalog.yaml.gz` rewritten by
`gen/flow.go`, with every collection in flow style, the top-level mapping
included:

| | count |
|---|---:|
| flow mappings | 10,937 (25,869 entries) |
| flow sequences | 10,451 (11,908 elements; 8,695 empty) |
| lines | 22,694, none over 80 columns |

A collection that fits in 80 columns is written on one line,
`{areaId: 205705999, blockIds: []}`. A longer one opens a line for each entry,
indented two spaces past its bracket, with a `,` after every entry but the last.
yaml.v3 writes each scalar, so a scalar is quoted wherever a plain one would end
at a flow indicator or read as another type.

The top-level mapping is a flow mapping too, so the file opens with `{` and
closes with `}` at column 0. Under a block mapping, a flow value closed at
column 0 is not valid YAML: every line of a flow collection in a block value
must be indented past the block. yaml.v3 and goccy/go-yaml read that form
anyway; this library and the YAML reference parser reject it.

`gen` compares the value with the source's, through yaml.v3, and refuses to
store a file that differs.

## The stress documents

`testdata/stress/` holds eight documents built to be difficult rather than to resemble
anything anybody wrote. `workloads.Stress()` returns them; `workloads.All()` does not, on
purpose — a geomean over the corpus is meant to say what an ordinary document costs, and a
fifty-thousand-key mapping in that average would stop it saying so.

They exist because the ordinary corpus cannot size a token store that reclaims behind the
parse. Not one of the ordinary documents holds an anchor, only `citm_catalog_flow` holds a flow
collection with entries, and the widest level in any of them is 1,789 entries against 293,142
tokens. They would make any store look good.

| document | bytes | tokens | widest map | widest seq | depth | anchors | peak | × source |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `flow_wide` | 270,007 | 60,003 | 1 | **30,000** | 4 | 0 | 11.2 MB | 43× |
| `flow_nested` | 75,190 | 72,900 | 300 | 1 | **123** | 0 | 14.5 MB | **197×** |
| `flow_long_scalars` | 466,810 | 2,403 | 1 | 1,200 | 4 | 0 | 1.7 MB | 4× |
| `anchors_far` | 262,940 | 64,014 | 8,002 | 0 | 6 | 1 | 14.4 MB | 56× |
| `anchors_many` | 226,890 | 56,000 | 8,000 | 0 | 6 | **4,000** | 12.4 MB | 56× |
| `anchors_nested` | 74,182 | 23,200 | 1,600 | 2 | 11 | 2,400 | 5.0 MB | 69× |
| `map_wide` | 1,150,000 | 150,000 | **50,000** | 0 | 3 | 0 | 36.6 MB | 33× |
| `comments_dense` | 1,161,001 | 18,183 | 61 | 0 | 3 | 0 | 4.8 MB | 4× |

Read with `ParseComments` throughout, since `comments_dense` is about comments and a mode
that drops them at the door measures a different document.

What each one is for:

- **`flow_wide`** — one flow sequence of 30,000 members, 60,003 tokens in a single collection
  that does not close until the last of them. A store reclaiming behind the parse has nothing
  to reclaim until the document ends.
- **`flow_nested`** — flow collections 120 deep. Every open level holds a token for its column,
  and this is the deepest the descent's stack of token references is asked to grow. At 197× the
  source it is the worst ratio in either corpus.
- **`flow_long_scalars`** — 466 kB in 2,403 tokens, 194 bytes a token. A store sized in tokens is
  blind to this document; one sized in bytes is blind to `flow_nested`.
- **`anchors_far`** — the anchor is the first entry and the alias is the last, so what the anchor
  covers has to be held for the length of the document.
- **`anchors_many`** — 4,000 anchors spread evenly, each aliased straight after. The friendly
  case: nothing needs holding for long, and a store that holds it anyway has the wrong rule.
- **`anchors_nested`** — anchors inside anchored subtrees, four deep, referred to from inside and
  outside. A store marking blocks with a flag cannot read this; it needs a count of the anchors
  still open.
- **`map_wide`** — one mapping of 50,000 keys. The parse gathers a level's entries before it
  builds the node above them, so here the widest level is the whole document.
- **`comments_dense`** — runs of 300 comment lines between entries. The descent looks past a
  comment to the token after it and the look is not bounded.

Built by `gen/stress.go`, deterministically and from nothing, so `go run ./gen -out testdata`
rewrites them byte for byte. `TestEveryStressDocumentParsesAndSettles` holds them to the same
round trip the ordinary corpus is held to.

## The gzip container

Written at level 9 with no modification time and the operating system byte set to
255, so the stored bytes do not record the machine that produced them. Note the
compressed bytes still depend on the compressor: Go's `compress/flate` and zlib
encode the same stream differently. What is stable is the content, which is what
`TestEveryWorkloadParsesAndSettles` reads.
