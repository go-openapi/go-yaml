// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package advanced_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
	"github.com/go-openapi/go-yaml/parser"
)

// Example_yamlDirective reads a document whose "%YAML 1.1" directive sets the version: "yes" is a boolean, 0100 an octal 64.
func Example_yamlDirective() {
	answer.Print("testdata/yaml-directive.yaml")

	// Output:
	// {
	//   "answer": true,
	//   "octal": 64
	// }
}

// Example_tagDirective reads a "%TAG" directive that names a tag prefix. go-yaml does not know the tag, and keeps the text.
func Example_tagDirective() {
	answer.Print("testdata/tag-directive.yaml")

	// Output:
	// {
	//   "price": "12.50"
	// }
}

// Example_merge reads "<<" under YAML 1.2, where it is an ordinary key.
func Example_merge() {
	answer.Print("testdata/merge.yaml")

	// Output:
	// {
	//   "base": {
	//     "retries": 3,
	//     "timeout": "10s"
	//   },
	//   "service": {
	//     "<<": {
	//       "retries": 3,
	//       "timeout": "10s"
	//     },
	//     "timeout": "30s"
	//   }
	// }
}

// Example_indentIndicator reads a block scalar whose indentation indicator keeps the spaces of its first line.
func Example_indentIndicator() {
	answer.Print("testdata/indent-indicator.yaml")

	// Output:
	// {
	//   "code": "  indented by two more\nthan the block\n"
	// }
}

// Example_nonSpecific reads the non-specific tag "!", which makes a plain scalar a string.
func Example_nonSpecific() {
	answer.Print("testdata/non-specific.yaml")

	// Output:
	// {
	//   "plain": 12,
	//   "nonspecific": "12"
	// }
}

// Example_localTag reads a local tag on a mapping. JSON has no tags, and the mapping reads as it is.
func Example_localTag() {
	answer.Print("testdata/local-tag.yaml")

	// Output:
	// {
	//   "point": {
	//     "x": 1,
	//     "y": 2
	//   }
	// }
}

// Example_anchoredKey reads a key that is an alias to an earlier key: the two keys are one, and go-yaml rejects the repeat.
func Example_anchoredKey() {
	answer.Print("testdata/anchored-key.yaml")

	// Output:
	// [2:1] mapping key "key" already defined at [1:1]
	//    1 | &k key: value
	// >  2 | *k : again
	//        ^
}

// Example_mergeList reads "<<" naming a list of mappings, under YAML 1.2.
func Example_mergeList() {
	answer.Print("testdata/merge-list.yaml")

	// Output:
	// {
	//   "a": {
	//     "x": 1,
	//     "y": 1
	//   },
	//   "b": {
	//     "y": 2,
	//     "z": 2
	//   },
	//   "c": {
	//     "<<": [
	//       {
	//         "x": 1,
	//         "y": 1
	//       },
	//       {
	//         "y": 2,
	//         "z": 2
	//       }
	//     ]
	//   }
	// }
}

// Example_keepTrailing reads a kept block scalar that starts and ends on blank lines.
func Example_keepTrailing() {
	answer.Print("testdata/keep-trailing.yaml")

	// Output:
	// {
	//   "text": "\nstarts on a blank line\n\n"
	// }
}

// Example_mergeKeys reads "<<" with parser.WithMergeKeys: the mapping it names merges in, and a key written in place wins.
func Example_mergeKeys() {
	answer.Print("testdata/merge.yaml", parser.WithMergeKeys())

	// Output:
	// {
	//   "base": {
	//     "retries": 3,
	//     "timeout": "10s"
	//   },
	//   "service": {
	//     "timeout": "30s",
	//     "retries": 3
	//   }
	// }
}

// Example_mergeListKeys reads "<<" naming a list with parser.WithMergeKeys: the first mapping to hold a key wins.
func Example_mergeListKeys() {
	answer.Print("testdata/merge-list.yaml", parser.WithMergeKeys())

	// Output:
	// {
	//   "a": {
	//     "x": 1,
	//     "y": 1
	//   },
	//   "b": {
	//     "y": 2,
	//     "z": 2
	//   },
	//   "c": {
	//     "x": 1,
	//     "y": 1,
	//     "z": 2
	//   }
	// }
}

// Example_mergeTag reads "<<" with the explicit tag "!!merge": go-yaml honors a written tag under every version, so the
// mapping merges without parser.WithMergeKeys.
func Example_mergeTag() {
	answer.Print("testdata/merge-tag.yaml")

	// Output:
	// {
	//   "base": {
	//     "retries": 3,
	//     "timeout": "10s"
	//   },
	//   "service": {
	//     "timeout": "30s",
	//     "retries": 3
	//   }
	// }
}

// Example_set reads a "!!set" that names a member twice. A set holds each member once, so go-yaml rejects it.
func Example_set() {
	answer.Print("testdata/set.yaml")

	// Output:
	// [4:3] mapping key "red" already defined at [2:3]
	//    1 | colors: !!set
	//    2 |   ? red
	//    3 |   ? green
	// >  4 |   ? red
	//          ^
}

// Example_pairs reads "!!pairs", a sequence of single-pair mappings that may repeat a key and keeps their order.
func Example_pairs() {
	answer.Print("testdata/pairs.yaml")

	// Output:
	// {
	//   "moves": [
	//     {
	//       "e2": "e4"
	//     },
	//     {
	//       "e7": "e5"
	//     },
	//     {
	//       "e2": "e3"
	//     }
	//   ]
	// }
}

// Example_timestamp reads a "!!timestamp" written with a space and an offset, which go-yaml writes in RFC 3339.
func Example_timestamp() {
	answer.Print("testdata/timestamp.yaml")

	// Output:
	// {
	//   "when": "2026-11-25T10:30:00+01:00"
	// }
}
