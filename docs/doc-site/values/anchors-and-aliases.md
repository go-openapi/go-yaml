---
title: Anchors, aliases and merge keys
weight: 50
description: |
  Declaring an anchor, referring to it, merging one mapping into another —
  and the two places where the default does not do what the document says.
---

## Writing an anchor

Tag the field that holds the shared value with `,anchor`, and the field that
refers to it with `,alias`:

```go
type Person struct {
	*Person `yaml:",omitempty,inline,alias"`
	Name    string `yaml:",omitempty"`
	Age     int    `yaml:",omitempty"`
}

defaultPerson := &Person{Name: "John Smith", Age: 20}

var doc struct {
	Default *Person   `yaml:"default,anchor"`
	People  []*Person `yaml:"people"`
}
doc.Default = defaultPerson
doc.People = []*Person{
	{Person: defaultPerson, Name: "Ken", Age: 10},
	{Person: defaultPerson},
}

out, _ := yaml.Marshal(doc)
```

```yaml
default: &default
  name: John Smith
  age: 20
people:
- <<: *default
  name: Ken
  age: 10
- <<: *default
```

`codec.MarshalAnchor` gives you a callback to name each anchor yourself, and
`codec.WithSmartAnchor` names them from the field.

## Reading a merge key

{{% notice style="warning" title="A plain `<<` does nothing under YAML 1.2" %}}
`<<` is a plain scalar that a 1.1 processor resolves to the merge type. YAML 1.2
dropped resolution by scalar shape, and this library follows the schema — so
under the 1.2 default `<<` is an ordinary key, the merge never happens, and
**there is no error**.

Writing the tag out says the same thing without relying on resolution, and that
is honoured at every version:

| written | 1.2 | 1.1 |
|---|---|---|
| `<<: *d` | not merged | merged |
| `!!merge <<: *d` | merged | merged |

`!!merge` on any key other than `<<` is refused: `could not find merge key`.

Decode the document above, exactly as `Marshal` wrote it:

```
1.2 default    people[0] = {Name:"Ken" Age:10 Person:nil}
               people[1] = {Name:""    Age:0  Person:nil}

%YAML 1.1      people[0] = {Name:"Ken" Age:10 Person:&{John Smith 20}}
               people[1] = {Name:""    Age:0  Person:&{John Smith 20}}
```

`people[1]` is meant to be the default person entire. Under 1.2 it comes back
empty.

**Use `parser.WithMergeKeys()` when you have to read documents written this
way.** It resolves a bare `<<` at any version and changes nothing else:

```go
codec.Unmarshal(src, &v,
	codec.WithParserOptions(parser.WithMergeKeys()))
```

| | merge | `a: yes` | `b: 0100` |
|---|---|---|---|
| default | no | `"yes"` | `100` |
| `WithMergeKeys()` | **yes** | `"yes"` | `100` |
| `WithYAMLVersion(YAML11)` | yes | `true` | `64` |

Reading a whole document under 1.1 to get one merge key changes far more than the
merge: `0100` becomes 64, `1_000` becomes 1000, `1:30` becomes 90 and `yes`
becomes `true`. Reach for `WithMergeKeys` instead, or write `!!merge <<:` in the
document if you control it.
{{% /notice %}}

This follows from one rule the library holds everywhere: **a written tag is
honoured at any version, and only resolution by scalar shape is version-gated.**
It has a consequence worth knowing before you rely on a round trip — the encoder
writes the plain `<<`, so `Marshal` produces a document that `Unmarshal` will not
merge unless the reading side passes `WithMergeKeys`. Other Go libraries resolve
`<<` at any version, which is what `WithMergeKeys` exists to match.

When the merge does fire, the mapping's own key beats the merged one, and an
earlier merge beats a later one.

## Reading an alias

By default an alias decodes to **an independent value**. Given

```yaml
a: &x {n: 1}
b: *x
```

`a` and `b` are two Go values, and changing one does not change the other.
`codec.ShareAliases` makes them the same value.

{{% notice style="note" title="Sharing and round-tripping go together" %}}
The encoder finds anchors by pointer address. Two independent values are two
anchors, so a decode without `ShareAliases` followed by an encode writes the
shared subtree out twice instead of anchoring it once. If you are reading a
document in order to write it back, pass `ShareAliases`.
{{% /notice %}}

## Anchors declared in another file

`codec.ReferenceDirs`, `ReferenceFiles` and `ReferenceReaders` publish anchors
from elsewhere. Given `testdata/anchor.yml`:

```yaml
a: &a
  b: 1
  c: hello
```

```go
dec := codec.NewDecoder(
	bytes.NewBufferString("a: *a\n"),
	codec.ReferenceDirs("testdata"),
)
var v struct {
	A struct {
		B int
		C string
	}
}
if err := dec.Decode(&v); err != nil {
	// ...
}
fmt.Printf("%+v\n", v) // {A:{B:1 C:hello}}
```

`RecursiveDir(true)` walks subdirectories; on its own it means nothing.

## Anchors do not cross documents

Documents in one stream are independent, so an alias must name an anchor declared
in the same document. To reach an anchor from another parse, hand it over with
`parser.WithAnchors` — see [the parser](../../documents/parser/).

## The alias budget

A document can name one anchor from many aliases, and an anchor can hold a large
subtree, so an adversarial document expands enormously. The decoder caps the work
and returns `errors.NewExcessiveAliasing` when a document exceeds it.
`NewRecursiveAlias` and `NewUnknownAnchor` cover the other two failures.
