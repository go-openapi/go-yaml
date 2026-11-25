// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package intermediate_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
	"github.com/go-openapi/go-yaml/parser"
)

// Example_anchors reads an anchor, "&defaults", and an alias, "*defaults", that writes the anchored mapping again.
func Example_anchors() {
	answer.Print("testdata/anchors.yaml")

	// Output:
	// {
	//   "defaults": {
	//     "retries": 3,
	//     "timeout": "10s"
	//   },
	//   "service": {
	//     "settings": {
	//       "retries": 3,
	//       "timeout": "10s"
	//     }
	//   }
	// }
}

// Example_literal reads a literal block scalar: every line break is kept.
func Example_literal() {
	answer.Print("testdata/literal.yaml")

	// Output:
	// {
	//   "script": "echo one\necho two\n"
	// }
}

// Example_folded reads a folded block scalar: line breaks fold into spaces, and a blank line keeps one break.
func Example_folded() {
	answer.Print("testdata/folded.yaml")

	// Output:
	// {
	//   "summary": "This sentence is folded into one line.\nA blank line keeps a break.\n"
	// }
}

// Example_chomping reads the three chomping indicators: "-" strips the final break, none keeps one, "+" keeps them all.
func Example_chomping() {
	answer.Print("testdata/chomping.yaml")

	// Output:
	// {
	//   "strip": "text",
	//   "clip": "text\n",
	//   "keep": "text\n\n",
	//   "end": 1
	// }
}

// Example_documents reads the first document of a stream, as ToJSON does.
func Example_documents() {
	answer.Print("testdata/documents.yaml")

	// Output:
	// {
	//   "name": "first"
	// }
}

// Example_tags reads explicit tags: "!!str" makes 42 a string and "!!int" makes "7" a number.
func Example_tags() {
	answer.Print("testdata/tags.yaml")

	// Output:
	// {
	//   "number": "42",
	//   "quoted": 7,
	//   "date": "2026-11-25"
	// }
}

// Example_explicitKey reads an explicit key, written after "? " with its value after ": ".
func Example_explicitKey() {
	answer.Print("testdata/explicit-key.yaml")

	// Output:
	// {
	//   "a key written explicitly": "its value",
	//   "simple": "value"
	// }
}

// Example_complexKey reads a sequence used as a key, which YAML allows and JSON cannot write.
func Example_complexKey() {
	answer.Print("testdata/complex-key.yaml")

	// Output:
	// [1:3] a sequence cannot be a JSON key
	// >  1 | ? [a, b]
	//          ^
	//    2 | : a sequence as a key
}

// Example_anchorAgain reads an anchor name defined twice: an alias names the latest anchor of its name.
func Example_anchorAgain() {
	answer.Print("testdata/anchor-again.yaml")

	// Output:
	// {
	//   "a": 1,
	//   "b": 1,
	//   "c": 2,
	//   "d": 2
	// }
}

// Example_emptyEntries reads sequence entries with no value, which are nulls.
func Example_emptyEntries() {
	answer.Print("testdata/empty-entries.yaml")

	// Output:
	// [
	//   null,
	//   "a",
	//   null
	// ]
}

// Example_foldedIndent reads a folded scalar with a more-indented line, whose line breaks are kept.
func Example_foldedIndent() {
	answer.Print("testdata/folded-indent.yaml")

	// Output:
	// {
	//   "text": "folded line\n  kept as it is\nfolded again\n"
	// }
}

// Example_documentsAll reads every document of the stream in turn.
func Example_documentsAll() {
	answer.PrintDocuments("testdata/documents.yaml")

	// Output:
	// # document 1
	// {
	//   "name": "first"
	// }
	// # document 2
	// {
	//   "name": "second"
	// }
	// # document 3
	// [
	//   "a",
	//   "list"
	// ]
}

// Example_coreTags reads core tags that force a type against the text: a string from true, a float from 1, a boolean
// from a quoted "false", a null from an empty string.
func Example_coreTags() {
	answer.Print("testdata/core-tags.yaml")

	// Output:
	// {
	//   "a": "true",
	//   "b": 1.0,
	//   "c": false,
	//   "d": null
	// }
}

// Example_sequenceKey reads a flow sequence used as an implicit key, which YAML allows and JSON cannot write.
func Example_sequenceKey() {
	answer.Print("testdata/sequence-key.yaml")

	// Output:
	// [1:1] a sequence cannot be a JSON key
	// >  1 | [a, b]: pair
	//        ^
}

// Example_nonSpecificList reads the non-specific tag "!" on plain scalars, which makes each a string, and on a flow
// sequence, whose entries resolve as usual.
func Example_nonSpecificList() {
	answer.Print("testdata/non-specific-list.yaml")

	// Output:
	// [
	//   "42",
	//   "true",
	//   "null",
	//   [
	//     1,
	//     2
	//   ]
	// ]
}

// Example_norway reads the Norway problem under YAML 1.2: "NO" and "yes" are strings, and 1.10 keeps its text.
func Example_norway() {
	answer.Print("testdata/norway.yaml")

	// Output:
	// {
	//   "country": "NO",
	//   "answer": "yes",
	//   "version": 1.10
	// }
}

// Example_norway11 reads the same entry under YAML 1.1, where "NO" and "yes" are booleans.
func Example_norway11() {
	answer.Print("testdata/norway.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// {
	//   "country": false,
	//   "answer": true,
	//   "version": 1.10
	// }
}
