// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	v3 "go.yaml.in/yaml/v3"
)

// commentedFrom is the workload the commented one is written over, and
// commentedName is what the result is stored as.
const (
	commentedFrom = "azure_swagger"
	commentedName = "commented_swagger"
)

// Comments are placed every so many candidate lines. The three strides are
// coprime, so a heading, a note and a closing remark rarely land together.
const (
	headingStride = 9
	noteStride    = 13
	closingStride = 17
	entryStride   = 11
)

// annotate writes comments into one stored workload and saves the result beside
// it.
func annotate(dir string) error {
	src, err := readGzip(filepath.Join(dir, commentedFrom+".yaml.gz"))
	if err != nil {
		return err
	}

	out := injectComments(src)
	if err := sameValue(src, out); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%-18s %8d bytes -> %8d with comments (%d lines, %d of them comment)\n",
		commentedFrom, len(src), len(out), lineCount(out), commentLines(out))

	return writeGzip(filepath.Join(dir, commentedName+".yaml.gz"), out)
}

// injectComments returns src with comments written into it, the way someone
// annotating a specification writes them: a block at the top, a heading before
// a section, a note at the end of a value's line, and a remark closing a block.
//
// Which lines get one is decided by counting candidates, so the file comes out
// the same every time this runs.
func injectComments(src []byte) []byte {
	lines := strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")

	out := make([]string, 0, len(lines)+len(lines)/8)
	out = append(out, header...)

	var headings, notes, closings, entries int
	for i, line := range lines {
		indent := indentOf(line)
		key := keyOf(line)

		// A sequence entry takes a heading of its own. It is the one comment
		// the parse files under SequenceNode.ValueHeadComments, so a corpus
		// without one leaves that slice empty everywhere.
		if isEntry(line) {
			entries++
			if entries%entryStride == 0 {
				out = append(out, strings.Repeat(" ", indent)+"# "+phrase(entryPhrases, "", entries))
			}
		}

		if key != "" && indent <= 6 {
			headings++
			if headings%headingStride == 0 {
				pad := strings.Repeat(" ", indent)
				out = append(out,
					pad+"# "+phrase(headingPhrases, key, headings),
					pad+"#",
					pad+"# "+phrase(detailPhrases, key, headings),
				)
			}
		}

		if hasValue(line) {
			notes++
			if notes%noteStride == 0 {
				line += " # " + phrase(notePhrases, key, notes)
			}
		}
		out = append(out, line)

		// A remark closing the block that ends here is written at the block's
		// own indent, deeper than the line that follows it.
		if i+1 < len(lines) && indentOf(lines[i+1]) < indent {
			closings++
			if closings%closingStride == 0 {
				out = append(out, strings.Repeat(" ", indent)+"# "+phrase(closingPhrases, key, closings))
			}
		}
	}

	return []byte(strings.Join(out, "\n") + "\n")
}

// header is the block every annotated specification seems to carry: what the
// file is and where it came from.
var header = []string{
	"# Azure Network management API, annotated.",
	"#",
	"# This is " + commentedFrom + ".yaml with comments written over it, and nothing",
	"# else changed: the two decode to the same value, which is what gen checks",
	"# before storing this file.",
	"#",
	"# It is here because the rest of the corpus has no comments at all, and a",
	"# comment travels through the parser on a path of its own: the scanner may",
	"# drop it, the grouping may lift it out of the token stream, and the parse",
	"# attaches it to a node afterwards. Measurements taken without one see none",
	"# of that.",
	"#",
	"# See SOURCE.md for how the comments below are placed.",
}

var (
	headingPhrases = []string{
		"%s -- the section below describes it in full",
		"%s: everything the service accepts here",
		"the %s block",
		"%s, as the API version above defines it",
		"what follows configures %s",
	}
	detailPhrases = []string{
		"Kept in step with the service; edit the specification, not the client.",
		"Order matters to the generator, so leave the keys where they are.",
		"Generated from the service contract. Changes here are overwritten.",
		"Refer to the Azure REST guidelines before adding a field.",
		"The response shapes below are shared with the other API versions.",
	}
	notePhrases = []string{
		"required",
		"defaults to the resource group's location",
		"read-only",
		"see the guidelines",
		"deprecated, kept for the older clients",
		"filled in by the service",
	}
	entryPhrases = []string{
		"one of the values the service accepts",
		"listed in the order the API returns them",
		"kept for the older API versions",
		"added in a later revision of the specification",
	}
	closingPhrases = []string{
		"end of %s",
		"nothing else belongs under %s",
		"%s ends here",
	}
)

// phrase picks one of a set and fills the key into it where it takes one.
func phrase(set []string, key string, n int) string {
	out := set[n/len(set)%len(set)]
	if strings.Contains(out, "%s") {
		if key == "" {
			key = "this block"
		}

		return fmt.Sprintf(out, key)
	}

	return out
}

// indentOf returns how far a line is indented. A blank line has no indent of
// its own and takes the deepest, so it never reads as the end of a block.
func indentOf(line string) int {
	if strings.TrimSpace(line) == "" {
		const deepestIndent = 1 << 30

		return deepestIndent
	}

	return len(line) - len(strings.TrimLeft(line, " "))
}

// keyOf returns the key a line opens, or "" where the line is a sequence entry
// or carries no key at all.
func keyOf(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if isEntry(line) || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	colon := strings.Index(trimmed, ":")
	if colon <= 0 {
		return ""
	}

	key := trimmed[:colon]
	if strings.ContainsAny(key, " \"'") {
		return ""
	}

	return key
}

// isEntry reports whether a line opens a sequence entry.
func isEntry(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " "), "- ")
}

// hasValue reports whether a line ends in a value a note may be written after.
func hasValue(line string) bool {
	trimmed := strings.TrimRight(line, " ")
	if trimmed == "" || strings.HasSuffix(trimmed, ":") {
		return false
	}

	return strings.Contains(trimmed, ": ") || strings.HasPrefix(strings.TrimLeft(trimmed, " "), "- ")
}

func lineCount(data []byte) int { return bytes.Count(data, []byte("\n")) }

func commentLines(data []byte) int {
	var n int
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimLeft(line, " "), []byte("#")) {
			n++
		}
	}

	return n
}

// sameValue checks the comments changed nothing but the comments.
//
// Read back with go.yaml.in/yaml/v3 rather than with this library, so the check
// does not rest on the parser the corpus exists to measure.
func sameValue(before, after []byte) error {
	var a, b any
	if err := v3.Unmarshal(before, &a); err != nil {
		return fmt.Errorf("reading the source back: %w", err)
	}
	if err := v3.Unmarshal(after, &b); err != nil {
		return fmt.Errorf("reading the annotated document back: %w", err)
	}
	if !reflect.DeepEqual(a, b) {
		return errors.New("the comments changed the document")
	}

	return nil
}
