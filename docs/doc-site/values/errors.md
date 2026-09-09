---
title: Errors
weight: 70
description: |
  What the library returns when a document is wrong, and how to print it with
  the offending line underneath.
---

## The pretty rendering

Every error carries the token it was raised at, so it can be printed with the
source around it. `errors.FormatError` does that:

```go
fmt.Println(errors.FormatError(err, false, true))
```

```
[6:1] cannot use []interface {} as a map key: it is not comparable
   3 | version: 1.2.3
   4 | limits:
   5 |   max: 18446744073709551615
>  6 | ? [a, b]
       ^
   7 | : a sequence used as a key
```

The two booleans are `colored` and `inclSource`. `FormatErrorAtToken` does the
same for a message and a token of your own, which is what you want when your
program rejects a document the parser accepted.

## Telling one failure from another

Match on the sentinels with `errors.Is`:

```go
if errors.Is(err, yamlerrors.ErrSyntax) {
	// the document is not YAML
}
```

Or take the error apart with `errors.As` and an `*errors.Error`, which carries
the token and therefore the position.

The conditions the library distinguishes: syntax, type mismatch, overflow,
duplicate key, unknown field, unknown anchor, recursive alias, excessive
aliasing, unhashable key, not-JSON, and unexpected node type.

## Pointing at a value your own code rejected

The parse succeeded, your validation did not, and you want the same rendering.
Address the value by path and annotate the source:

```go
var v struct {
	A int
	B string
}
if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
	// ...
}
if v.A != 2 {
	path, err := expressions.PathString("$.a")
	if err != nil {
		// ...
	}
	source, err := path.AnnotateSource([]byte(yml), true)
	if err != nil {
		// ...
	}
	fmt.Printf("a value expected 2 but actual %d:\n%s\n", v.A, source)
}
```

See [YAMLPath](../../documents/yamlpath/) for the path syntax.
