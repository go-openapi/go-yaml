// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package bestiary_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
	"github.com/go-openapi/go-yaml/parser"
)

// Example_plainLines reads a plain scalar over three lines, folded into one line with a space at each break.
func Example_plainLines() {
	answer.Print("testdata/plain-lines.yaml")

	// Output:
	// {
	//   "motto": "this is one plain scalar"
	// }
}

// Example_flowPair reads a flow sequence whose first entry is a single-pair mapping.
func Example_flowPair() {
	answer.Print("testdata/flow-pair.yaml")

	// Output:
	// [
	//   {
	//     "a": 1
	//   },
	//   "b"
	// ]
}

// Example_octal reads 0755 as the decimal 755 under YAML 1.2, and 0o755 as octal.
func Example_octal() {
	answer.Print("testdata/octal.yaml")

	// Output:
	// {
	//   "mode": 755,
	//   "flags": 493
	// }
}

// Example_octal11 reads 0755 as octal under YAML 1.1, and 0o755 as a string.
func Example_octal11() {
	answer.Print("testdata/octal.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// {
	//   "mode": 493,
	//   "flags": "0o755"
	// }
}

// Example_sexagesimal reads 22:22 and 1:30 as strings under YAML 1.2.
func Example_sexagesimal() {
	answer.Print("testdata/sexagesimal.yaml")

	// Output:
	// {
	//   "port": "22:22",
	//   "time": "1:30"
	// }
}

// Example_sexagesimal11 reads 22:22 and 1:30 as base 60 integers under YAML 1.1.
func Example_sexagesimal11() {
	answer.Print("testdata/sexagesimal.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// {
	//   "port": 1342,
	//   "time": 90
	// }
}

// Example_underscore reads 1_000 as a string under YAML 1.2.
func Example_underscore() {
	answer.Print("testdata/underscore.yaml")

	// Output:
	// {
	//   "big": "1_000"
	// }
}

// Example_underscore11 reads 1_000 as the integer 1000 under YAML 1.1.
func Example_underscore11() {
	answer.Print("testdata/underscore.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// {
	//   "big": 1000
	// }
}

// Example_numbers reads hex, signed, exponent and dot-only spellings as numbers under YAML 1.2.
func Example_numbers() {
	answer.Print("testdata/numbers.yaml")

	// Output:
	// [
	//   31,
	//   12,
	//   1e3,
	//   0.5,
	//   5.0,
	//   -0
	// ]
}

// Example_numbers11 reads 1e3 as a string under YAML 1.1, which wants a dot in a float.
func Example_numbers11() {
	answer.Print("testdata/numbers.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// [
	//   31,
	//   12,
	//   "1e3",
	//   0.5,
	//   5.0,
	//   -0
	// ]
}

// Example_spellings reads the three spellings of true and of null that the core schema knows.
func Example_spellings() {
	answer.Print("testdata/spellings.yaml")

	// Output:
	// [
	//   true,
	//   true,
	//   "tRUE",
	//   null,
	//   null,
	//   "nULL",
	//   null
	// ]
}

// Example_emptyKey reads a mapping whose one key is left empty, a null.
func Example_emptyKey() {
	answer.Print("testdata/empty-key.yaml")

	// Output:
	// {
	//   "null": "value"
	// }
}

// Example_aliasKey reads an alias standing as a key, with a space before its colon.
func Example_aliasKey() {
	answer.Print("testdata/alias-key.yaml")

	// Output:
	// {
	//   "base": "name",
	//   "name": "value"
	// }
}

// Example_anchorColon reads an anchor whose name holds a colon.
func Example_anchorColon() {
	answer.Print("testdata/anchor-colon.yaml")

	// Output:
	// {
	//   "key": "value",
	//   "again": "value"
	// }
}

// Example_anchorBang reads an anchor named "!".
func Example_anchorBang() {
	answer.Print("testdata/anchor-bang.yaml")

	// Output:
	// {
	//   "a": "x",
	//   "b": "x"
	// }
}

// Example_jsonKey reads a quoted flow key followed by a colon with no space.
func Example_jsonKey() {
	answer.Print("testdata/json-key.yaml")

	// Output:
	// {
	//   "a": "b",
	//   "c": "d"
	// }
}

// Example_flowNull reads flow mapping entries that have no value.
func Example_flowNull() {
	answer.Print("testdata/flow-null.yaml")

	// Output:
	// {
	//   "a": null,
	//   "b": "c",
	//   "d": null
	// }
}

// Example_trailingComma reads a flow sequence ending on a comma.
func Example_trailingComma() {
	answer.Print("testdata/trailing-comma.yaml")

	// Output:
	// [
	//   "a",
	//   "b"
	// ]
}

// Example_compact reads a sequence and a mapping opened on the line of their dash.
func Example_compact() {
	answer.Print("testdata/compact.yaml")

	// Output:
	// [
	//   [
	//     "a",
	//     "b"
	//   ],
	//   {
	//     "c": 1,
	//     "d": 2
	//   }
	// ]
}

// Example_dashStart reads entries starting with a dash, with and without a space after it.
func Example_dashStart() {
	answer.Print("testdata/dash-start.yaml")

	// Output:
	// [
	//   "-a",
	//   -1,
	//   [
	//     1
	//   ]
	// ]
}

// Example_dashContinued reads a plain scalar continued on a line starting with a dash.
func Example_dashContinued() {
	answer.Print("testdata/dash-continued.yaml")

	// Output:
	// [
	//   "a -b"
	// ]
}

// Example_indicators reads plain scalars starting with ? and :.
func Example_indicators() {
	answer.Print("testdata/indicators.yaml")

	// Output:
	// {
	//   "?foo": "bar",
	//   ":baz": ":qux"
	// }
}

// Example_hashInside reads a # with no space before it as text.
func Example_hashInside() {
	answer.Print("testdata/hash-inside.yaml")

	// Output:
	// {
	//   "key": "a#b"
	// }
}

// Example_commentValue reads a value written below a comment on its key's line.
func Example_commentValue() {
	answer.Print("testdata/comment-value.yaml")

	// Output:
	// {
	//   "key": "value"
	// }
}

// Example_doubleFold reads a double-quoted scalar over several lines.
func Example_doubleFold() {
	answer.Print("testdata/double-fold.yaml")

	// Output:
	// {
	//   "text": "one two\nthree"
	// }
}

// Example_escapedBreak reads a double-quoted scalar whose line break is escaped.
func Example_escapedBreak() {
	answer.Print("testdata/escaped-break.yaml")

	// Output:
	// {
	//   "text": "abcdef"
	// }
}

// Example_escapes reads two double-quoted escapes and a single-quoted quote.
func Example_escapes() {
	answer.Print("testdata/escapes.yaml")

	// Output:
	// [
	//   "Aé",
	//   "/",
	//   "'"
	// ]
}

// Example_emptyDocs reads two empty documents.
func Example_emptyDocs() {
	answer.PrintDocuments("testdata/empty-docs.yaml")

	// Output:
	// # document 1
	// null
	// # document 2
	// null
}

// Example_scalarDocs reads three documents, each a scalar opened on its --- line.
func Example_scalarDocs() {
	answer.PrintDocuments("testdata/scalar-docs.yaml")

	// Output:
	// # document 1
	// "a"
	// # document 2
	// "b\n"
	// # document 3
	// "c d\n"
}

// Example_explicitFlow reads explicit keys in a flow mapping, one without a value.
func Example_explicitFlow() {
	answer.Print("testdata/explicit-flow.yaml")

	// Output:
	// {
	//   "a": "b",
	//   "c": null
	// }
}

// Example_indentedDoc reads a root mapping indented by two spaces.
func Example_indentedDoc() {
	answer.Print("testdata/indented-doc.yaml")

	// Output:
	// {
	//   "a": 1,
	//   "b": 2
	// }
}

// Example_longKey reads a literal block scalar as a key.
func Example_longKey() {
	answer.Print("testdata/long-key.yaml")

	// Output:
	// {
	//   "multi\nline\n": "value"
	// }
}

// Example_notADirective reads "&YAML 1.2" as an anchored number, in a document of its own.
func Example_notADirective() {
	answer.PrintDocuments("testdata/not-a-directive.yaml")

	// Output:
	// # document 1
	// 1.2
	// # document 2
	// -1
}

// Example_reservedDirective ignores a directive whose name it does not know.
func Example_reservedDirective() {
	answer.Print("testdata/reserved-directive.yaml")

	// Output:
	// true
}

// Example_fourDashes reads "----" and the line below it as one plain scalar.
func Example_fourDashes() {
	answer.Print("testdata/four-dashes.yaml")

	// Output:
	// "---- &a1 !!float 55.12738563616954"
}

// Example_dashStar reads "---*" as the start of a key, not a document marker.
func Example_dashStar() {
	answer.Print("testdata/dash-star.yaml")

	// Output:
	// {
	//   "---*{D": "False}"
	// }
}

// Example_anchorSwallows reads an anchor named "a1!" and the text after it.
func Example_anchorSwallows() {
	answer.Print("testdata/anchor-swallows.yaml")

	// Output:
	// "<tag:yaml.org,2002:int> -23"
}

// Example_anchorTilde reads an anchor named "a1-~" on an empty node.
func Example_anchorTilde() {
	answer.Print("testdata/anchor-tilde.yaml")

	// Output:
	// null
}

// Example_localTrue reads "!true" as a local tag on an empty node.
func Example_localTrue() {
	answer.Print("testdata/local-true.yaml")

	// Output:
	// ""
}

// Example_truncated reads a tagged key whose value is an anchor and nothing else.
func Example_truncated() {
	answer.Print("testdata/truncated.yaml")

	// Output:
	// {
	//   "base": null
	// }
}

// Example_entryContinued reads a sequence entry whose plain scalar runs over a line starting with a dash.
func Example_entryContinued() {
	answer.Print("testdata/entry-continued.yaml")

	// Output:
	// [
	//   "c2 - &a1 {}"
	// ]
}

// Example_lostComma reads a flow sequence of one plain scalar holding a space.
func Example_lostComma() {
	answer.Print("testdata/lost-comma.yaml")

	// Output:
	// [
	//   "FALSE ~"
	// ]
}

// Example_commaKept reads the same flow sequence with its comma.
func Example_commaKept() {
	answer.Print("testdata/comma-kept.yaml")

	// Output:
	// [
	//   false,
	//   null
	// ]
}

// Example_swappedDirective reads explicit flow keys under an unknown directive.
func Example_swappedDirective() {
	answer.Print("testdata/swapped-directive.yaml")

	// Output:
	// {
	//   "": null,
	//   "? a": null
	// }
}

// Example_openBracket reads an anchored entry holding an explicit key, with a value ending on "[".
func Example_openBracket() {
	answer.Print("testdata/open-bracket.yaml")

	// Output:
	// [
	//   {
	//     "+0b11": "NULL["
	//   }
	// ]
}

// Example_plusKey reads a float key, then a second document holding "#".
func Example_plusKey() {
	answer.PrintDocuments("testdata/plus-key.yaml")

	// Output:
	// # document 1
	// {
	//   "212.83274740247398": null
	// }
	// # document 2
	// "#"
}
