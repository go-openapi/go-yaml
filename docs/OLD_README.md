
## Install

```sh
go get github.com/go-openapi/go-yaml
```

Requires Go 1.26 or later. We support the two most recent stable Go minor versions.

## What it does

go-yaml decodes and encodes Go values, like `go.yaml.in/yaml/v3`. It also exposes the layer
underneath: a parser, a syntax tree and a token stream, so a program can read and transform a
document without turning it into Go values first.

### Speed and memory

`BenchmarkWorkloads`, in `internal/benchmarks`, decodes six real documents into `any` and has
`go.yaml.in/yaml/v3` do the same:

| | against yaml/v3 |
|---|---|
| time | 1.29x faster (geomean -22.3%) |
| bytes allocated | 4.5x fewer |
| allocations | 12.9x fewer |

`ToJSON` converts a document without building a Go value for it, and allocates 1.49x the size of
the source where it once allocated 89.2x.

### Conformance

Conformance is a primary concern. Every number below comes from a suite in this repository.

- **The [YAML Test Suite](https://github.com/yaml/yaml-test-suite)**, 402 cases. The parser scores
  393 of them — the 9 that state no expectation are excluded — and agrees with all 393. The decoder
  scores 370 and decodes all 370 as expected; `ToJSON` and `ToJSONTokens` score 274 and agree on 271.
  The three they differ on are answers this library gives on purpose, listed in
  `conformance/json_test.go`.
- **A grammar-generated corpus**, much wider than the suite: 21,843 cases over 605 buckets, checked
  against a reference parser rather than against ourselves.

What it supports:

- the YAML 1.2 core schema, and the YAML 1.1 schema and tags on request
- constructs JSON has no equivalent for, such as a mapping or a sequence used as a key
- arbitrarily large and small numbers
- ordered mappings
- comments, on the tree and through a path-keyed map

What it will not support:

- **encodings other than UTF-8.** UTF-16 and UTF-32 are rejected. This is a deliberate departure from
  [§5.1](https://yaml.org/spec/1.2.2/#51-character-set), which asks a processor to accept UTF-16 as well,
  and it is one of very few. Invalid UTF-8 is refused rather than repaired.
- **the YAML 1.1 tags `!!pair` and `!!value`** (2005).
- **documents larger than 2^31-1 bytes**, that is 2.1 GB.

## Where this is going

The numbers above are where the library stands today. These are the marks it is being built to,
and none of them is reached yet. Read the section as a statement of intent, not of behaviour.

| target | today |
|---|---|
| **2x faster than yaml/v3**, and better where the document favours us | 1.29x |
| **5 to 10x less memory** | 4.5x fewer bytes, 12.9x fewer allocations |
| **Every path at 100% of the YAML Test Suite**, not the decoder alone | every path agrees on every case it can score; 3 of `ToJSON`'s 274 are declared departures |
| **Streaming, with a bounded memory footprint** — parse a document without holding it, so a caller can transform nodes as they arrive and find positions without building a tree | a parse reads the whole document |
| **Verbatim reconstruction** — render a document back byte for byte. A prospect rather than scheduled work; it is why the tree keeps each token's source text and position | nothing renders a document back |

The conformance target is not negotiable down to "good enough for our documents": go-openapi exists
to implement standards, and this library is held to the specification rather than to our own use of
it.

## Where we stand on the ambiguous parts of the spec

What follows is undefined, ambiguous, or has no consensus between implementations. Each of these
is a choice, not a reading.

- **Anchors do not cross documents.** Documents in one stream are independent, so an alias must
  name an anchor declared in the same document. libfyaml keeps a stream-scoped table and resolves
  across the boundary; we refuse, because an alias may name any earlier anchor of its name, and
  carrying the table on would pin every anchored subtree until the stream ends. To reach an anchor
  from elsewhere, hand it in with `parser.WithAnchors`.
- **Directives and tags do not cross documents either**, for the same reason: a `%YAML` or `%TAG`
  directive in one document does not reach the next, and neither does a resolved tag.
- **`yes` and `no` are strings by default.** They become booleans when the document carries a
  `%YAML 1.1` directive, or under `parser.WithYAMLVersion(parser.YAML11)`.
- **`!!timestamp` is honoured when written, and never inferred.** A date-shaped scalar with no tag
  stays a string.
- **Duplicate mapping keys are rejected by default.** `codec.AllowDuplicateMapKey` accepts them and
  keeps the last.
- **`ToJSON` refuses what JSON cannot hold**, and stringifies a non-string scalar key. That can
  produce a duplicate: `'1.0': 1` and `!!float 1: 2` are two YAML keys and one JSON key.
- **The decoder accepts more than JSON can express** — `map[float64]any` works. It still rejects a
  key Go cannot hash into a map, such as `.nan` or `~`.

## Why fork?

`go-openapi` needs a library that works on YAML *documents*, not only a decoder, and it needs three
things that no Go YAML library offers together:

- **Low-level access** — a token and AST surface with accurate positions, not a `Marshal`/`Unmarshal`
  facade. We drive editor and TUI tooling (syntax colouring, diagnostics, JSON-pointer navigation)
  over OpenAPI documents, so we need to know *where* every construct is, not just what it means.
- **A bounded memory footprint.** OpenAPI documents get large. At the fork point the whole input was
  materialised as `[]rune`, every token was retained, and nothing could be emitted before the entire
  document had been parsed — a tree cost roughly 32x the source. The `[]rune` is gone and a parse now
  makes 84% fewer allocations, but the parser still reads a whole document.
- **Conformance** good enough to project YAML onto JSON semantics faithfully.

`goccy/go-yaml` is the only Go YAML library whose architecture exposes the machinery to build that
on, which is why we started from it rather than from anything else. See
[`ANALYSIS-go-openapi.md`](./ANALYSIS-go-openapi.md) for the measurements behind all of the above.

## Relationship to `go-yaml/yaml`

None, and that is inherited from upstream. This library was written from scratch by
[@goccy](https://github.com/goccy) rather than ported from libyaml, which is what makes its internals
approachable enough to fork. Coming from `gopkg.in/yaml.v3` or `go.yaml.in/yaml/v3`, what you gain is:

- source written in Go rather than transliterated from C
- higher coverage of the YAML Test Suite
- errors that carry a position in the source, which is what makes a diagnostic possible
- comments and anchors that survive a round trip
- a parser and a tree, not only `Encoder` and `Decoder`

## Packages

The root package holds `Marshal`, `Unmarshal`, `ToJSON` and `FromJSON` — the four calls that take no
option. Everything else lives a layer down. The imports run one way: no package in this table imports
one listed below it.

| package | what it holds | imports |
|---|---|---|
| `token` | a token, its position and its source text | — |
| `ast` | the document as a tree | `token` |
| `printer` | draws a document, or a line of it under an error | `ast` |
| `errors` | `Error`, the kind it carries, and `FormatError` | `printer` |
| `parser` | builds a tree from a source | `errors` |
| `codec` | `Encoder`, `Decoder`, their 36 options, `MapSlice`, `RawMessage`, the comment types, the marshaler interfaces | `parser` |
| `expressions` | `Path` and `PathString`, to navigate a document by path | `codec` |
| `github.com/go-openapi/go-yaml` | `Marshal`, `Unmarshal`, `ToJSON`, `FromJSON` | `codec` |

The scanner is not exported. It lives at `internal/scanner`, under the parser.

## Usage

### 1. Simple Encode/Decode

An interface like `go-yaml/yaml`, using `reflect`:

```go
var v struct {
	A int
	B string
}
v.A = 1
v.B = "hello"
bytes, err := yaml.Marshal(v)
if err != nil {
	//...
}
fmt.Println(string(bytes)) // "a: 1\nb: hello\n"
```

```go
	yml := `
%YAML 1.2
---
a: 1
b: c
`
var v struct {
	A int
	B string
}
if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
	//...
}
```

To control marshal/unmarshal behavior, you can use the `yaml` tag:

```go
	yml := `---
foo: 1
bar: c
`
var v struct {
	A int    `yaml:"foo"`
	B string `yaml:"bar"`
}
if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
	//...
}
```

For convenience, the `json` tag is also accepted. Note that not all options from the `json` tag have significance
when parsing YAML documents. If both tags exist, the `yaml` tag takes precedence.

For custom marshal/unmarshaling, implement one of the two variants declared in
[`codec`](https://pkg.go.dev/github.com/go-openapi/go-yaml/codec). `codec.Marshaler`/`codec.Unmarshaler` return
and take the YAML text as `[]byte`, like [`encoding/json`](https://pkg.go.dev/encoding/json);
`codec.GoYAMLMarshaler`/`codec.GoYAMLUnmarshaler` return and take another Go value, like
[`gopkg.in/yaml.v2`](https://pkg.go.dev/gopkg.in/yaml.v2).

Semantically both are the same, but they differ in performance. Because indentation matters in YAML, a valid YAML
fragment returned by a marshaler cannot simply be spliced into the parent container's serialized form — so when
we receive `[]byte` from a `codec.Marshaler`, we must decode it once to work out how to place it in context. With
a `codec.GoYAMLMarshaler`, that decode is skipped. If you repeatedly marshal complex objects, the latter is
always better; for a config file read once, the former is easier to write.

### 2. Reference elements declared in another file

Given a directory -- `testdata` here -- holding an `anchor.yml` file:

```yaml
a: &a
  b: 1
  c: hello
```

If the `codec.ReferenceDirs("testdata")` option is passed to `codec.Decoder`, the decoder looks for anchor
definitions in the YAML files under that directory:

```go
buf := bytes.NewBufferString("a: *a\n")
dec := codec.NewDecoder(buf, codec.ReferenceDirs("testdata"))
var v struct {
	A struct {
		B int
		C string
	}
}
if err := dec.Decode(&v); err != nil {
	//...
}
fmt.Printf("%+v\n", v) // {A:{B:1 C:hello}}
```

### 3. Encode with `Anchor` and `Alias`

#### 3.1. Explicitly declared anchor and alias names

Declare them as a struct tag. If the value specified for an anchor is a pointer and the same address is found
again, the value is automatically emitted as an alias. If an explicit alias name is specified, an error is raised
when its value differs from the value specified in the anchor.

```go
type T struct {
  A int
  B string
}
var v struct {
  C *T `yaml:"c,anchor=x"`
  D *T `yaml:"d,alias=x"`
}
v.C = &T{A: 1, B: "hello"}
v.D = v.C
bytes, err := yaml.Marshal(v)
if err != nil {
  panic(err)
}
fmt.Println(string(bytes))
/*
c: &x
  a: 1
  b: hello
d: *x
*/
```

#### 3.2. Implicitly declared anchor and alias names

Without an explicit anchor name, the default is `strings.ToLower($FieldName)`.

```go
type T struct {
	I int
	S string
}
var v struct {
	A *T `yaml:"a,anchor"`
	B *T `yaml:"b,anchor"`
	C *T `yaml:"c"`
	D *T `yaml:"d"`
}
v.A = &T{I: 1, S: "hello"}
v.B = &T{I: 2, S: "world"}
v.C = v.A // C has the same pointer address as A
v.D = v.B // D has the same pointer address as B
bytes, err := yaml.Marshal(v)
if err != nil {
	//...
}
fmt.Println(string(bytes))
/*
a: &a
  i: 1
  s: hello
b: &b
  i: 2
  s: world
c: *a
d: *b
*/
```

#### 3.3 Merge key and alias

A merge key with an alias (`<<: *alias`) can be used by embedding a structure with the `inline,alias` tag.

```go
type Person struct {
	*Person `yaml:",omitempty,inline,alias"` // embed Person type for default value
	Name    string `yaml:",omitempty"`
	Age     int    `yaml:",omitempty"`
}
defaultPerson := &Person{
	Name: "John Smith",
	Age:  20,
}
people := []*Person{
	{
		Person: defaultPerson, // assign default value
		Name:   "Ken",         // override Name property
		Age:    10,            // override Age property
	},
	{
		Person: defaultPerson, // assign default value only
	},
}
var doc struct {
	Default *Person   `yaml:"default,anchor"`
	People  []*Person `yaml:"people"`
}
doc.Default = defaultPerson
doc.People = people
bytes, err := yaml.Marshal(doc)
if err != nil {
	//...
}
fmt.Println(string(bytes))
/*
default: &default
  name: John Smith
  age: 20
people:
- <<: *default
  name: Ken
  age: 10
- <<: *default
*/
```

### 4. Pretty formatted errors

Errors produced during parsing carry the location of the problem in the source document, and can optionally be
colorized. Use `errors.FormatError` from `github.com/go-openapi/go-yaml/errors` to control both, which
accepts two boolean values.

### 5. Use YAMLPath

```go
yml := `
store:
  book:
    - author: john
      price: 10
    - author: ken
      price: 12
  bicycle:
    color: red
    price: 19.95
`
path, err := expressions.PathString("$.store.book[*].author")
if err != nil {
  //...
}
var authors []string
if err := path.Read(strings.NewReader(yml), &authors); err != nil {
  //...
}
fmt.Println(authors)
// [john ken]
```

#### 5.1 Print a customized error with the YAML source

```go
package main

import (
  "fmt"

  "github.com/go-openapi/go-yaml"
  "github.com/go-openapi/go-yaml/expressions"
)

func main() {
  yml := `
a: 1
b: "hello"
`
  var v struct {
    A int
    B string
  }
  if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
    panic(err)
  }
  if v.A != 2 {
    // output error with YAML source
    path, err := expressions.PathString("$.a")
    if err != nil {
      panic(err)
    }
    source, err := path.AnnotateSource([]byte(yml), true)
    if err != nil {
      panic(err)
    }
    fmt.Printf("a value expected 2 but actual %d:\n%s\n", v.A, string(source))
  }
}
```

## For developers

See [`.github/CONTRIBUTING.md`](./.github/CONTRIBUTING.md).

The library has **no runtime dependencies**, and that is a property worth keeping: `go-openapi/core`
depends on this module, so anything we add here propagates.

Tests use [`go-openapi/testify/v2`](https://github.com/go-openapi/testify), which is itself
dependency-free — so the only entry in `go.mod` is a test dependency that never reaches your binary.
Everything that needs more than that lives under `internal/`, in modules of its own listed in
`go.work`:

| | |
|---|---|
| `internal/analysis` | the reproducible measurements behind `ANALYSIS-go-openapi.md` |
| `internal/benchmarks` | comparisons against other YAML libraries |
| `internal/testintegration` | tests needing third-party libraries |

```sh
go test ./...          # the library
go test work ./...     # the library and every module in the workspace
```

