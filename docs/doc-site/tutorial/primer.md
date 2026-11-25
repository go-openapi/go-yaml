---
title: "A primer on the YAML language"
weight: 10
description: Where YAML comes from, and its versions.
---

## Where it comes from

YAML started in 2001, from two efforts that met. Clark Evans and Oren Ben-Kiki were working on a simpler XML in the
sml-dev group. Ingy döt Net, then known as Brian Ingerson, needed a serialization format for Perl's Inline module, and
had written Data::Denter for it. The three wrote the first drafts together: the one of 1 August 2001 is titled *Yet
Another Markup Language*. By 2002 the name read *YAML Ain't Markup Language*, to say what the language was for: data,
not documents.

Its roots are in the Perl world. Data::Denter, the format Ingy wrote before YAML, was a Perl module, and so was
[YAML.pm](https://metacpan.org/pod/YAML), released in December 2001. Its documentation calls it the first YAML
implementation.

## The versions

| Version | Published | What it brought |
|---|---|---|
| 1.0 | 2004 | The first specification, after three years of design on the yaml-core mailing list. |
| 1.1 | 2005 | The version libyaml implements, and the tools built on it. Its type repository adds booleans such as `yes` and `off`, timestamps, merge keys and more. |
| 1.2 | 2009 | A rework: JSON becomes a subset of YAML, and the types a plain scalar can take are cut down to a small core. |
| 1.2.2 | 2021 | A revision that changes no rule. It rewrites the text to read clearly and remove ambiguity. |

Work on a version 1.3 has gone on since 2017, with drafts published at [spec.yaml.io](https://spec.yaml.io/).

## YAML 1.2, the big undertaking

YAML 1.2 set out to make YAML a strict superset of JSON, and to pin the language down with a formal grammar. Two
changes follow, and both reach every user.

First, YAML 1.2 reads a plain scalar through a *schema*. The *core* schema, the default, knows only nulls, booleans,
integers and floats, written as JSON writes them, plus a few YAML spellings: `~` for null, `0o17` for octal, `.inf`
and `.nan`. Everything else is a string. So `yes` and `on` are strings in 1.2, booleans in 1.1, and `0100` is the
integer 100 in 1.2, octal 64 in 1.1.

Second, the types of the 1.1 repository leave the core: timestamps, binary data, sets, ordered maps, merge keys. A
document still gets them by writing their tag, such as `!!timestamp` or `!!merge`, and a parser may still support them.

Many tools never moved to 1.2. libyaml, a C library that PyYAML and many other bindings use, implements YAML 1.1. A
file that reads one way in one tool can read another way in the next, and the [Norway problem](../intermediate/#quizz-test-your-yaml-knowledge-expert-level) is the
best-known case.

go-yaml reads YAML 1.2 by default, and YAML 1.1 on request or where a document opens with `%YAML 1.1`. See
[YAML versions](../../features/values/yaml-versions/) for every scalar the two versions read differently.

## Sources

- [About YAML](https://yaml.org/about.html), on yaml.org.
- The [YAML 1.2.2 specification](https://yaml.org/spec/1.2.2/), and its
  [list of changes](https://yaml.org/spec/1.2.2/ext/changes/).
- [YAML.pm](https://metacpan.org/pod/YAML) on MetaCPAN, and its [release history](https://metacpan.org/dist/YAML).
- The [2001 working draft](https://yaml.org/spec/history/2001-08-01.html) and the
  [YAML 1.1 specification](https://yaml.org/spec/1.1/).
