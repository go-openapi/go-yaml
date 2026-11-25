---
title: "More"
weight: 60
description: YAML's relatives and successors.
---

## KYAML: simplified YAML

KYAML is a subset of YAML that Kubernetes defined to avoid YAML's traps: whitespace that changes the meaning, and plain
text that turns into another type. It writes every collection in flow style, with braces and brackets, and every string
value double-quoted.

{{< code file="tutorial/more/testdata/kyaml.yaml" lang="yaml" nolink="true" >}}

Because KYAML is YAML, any YAML parser reads it. This is the example of the
[KYAML reference](https://kubernetes.io/docs/reference/encodings/kyaml/), as go-yaml reads it:

{{< code file="tutorial/more/example_test.go" lang="json" output="Example_kyaml" nolink="true" >}}

`kubectl get -o kyaml` writes it. The format came with
[KEP-5295](https://www.kubernetes.dev/resources/keps/5295/), as alpha in Kubernetes 1.34.

## YAMLStar, YAMLScript

Ingy döt Net, one of YAML's authors, has two projects built on the YAML 1.2 specification.

[YAMLStar](https://github.com/yaml/yamlstar) is a YAML loader and dumper for many languages, built on the YAML
Reference Parser. It aims at full YAML 1.2 compliance, and it evaluates nothing: it reads data.

[YAMLScript](https://github.com/yaml/yamlscript), or YS, is a YAML loader that is also a functional programming
language. A plain YAML file loads as data. A file tagged `!ys-0` may hold code as well, which YS compiles to Clojure and
runs. YAMLStar derives from YAMLScript, without the code.

Both are MIT-licensed and offer bindings for some thirty languages, Go among them.
