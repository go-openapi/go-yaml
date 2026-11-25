---
title: "The YAML bestiary"
weight: 50
description: "YAML for conformant parsers only: legal documents that few humans read right."
---

Every entry below is legal YAML. Read it, decide what it holds, then open the answer: the JSON go-yaml reads from it.

Quizz: test your YAML knowledge (impossible level).

## The Norway problem

{{< code file="tutorial/bestiary/testdata/norway.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_norway" nolink="true" >}}

Under YAML 1.2 the only booleans are `true` and `false`, so `NO` and `yes` are strings. `1.10` is a number, and go-yaml
keeps its text instead of rounding it to `1.1`.

The same document read as YAML 1.1, where `NO`, `yes`, `on` and their kin are booleans:

{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_norway11" nolink="true" >}}

The country code for Norway reads as `false`.
{{% /expand %}}

## One scalar, three lines

{{< code file="tutorial/bestiary/testdata/plain-lines.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_plainLines" nolink="true" >}}

A plain scalar may run over several lines, as long as each continues further right than its key. YAML folds the line
breaks into spaces.
{{% /expand %}}

## A pair in a list

{{< code file="tutorial/bestiary/testdata/flow-pair.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/bestiary/example_test.go" lang="json" output="Example_flowPair" nolink="true" >}}

Inside a flow sequence, `a: 1` is a mapping of one pair, written without its braces. The sequence holds that mapping
and the string `b`.
{{% /expand %}}
