// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stream_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/codec"
	yerrors "github.com/go-openapi/go-yaml/errors"
)

// snippet:types
type Job struct {
	Name  string `yaml:"name"`
	Every string `yaml:"every"`
}

// endsnippet:types

// snippet:source
// The second document writes a sequence where Job.Every is a string.
const jobs = `name: backup
every: 1h
---
name: report
every: [1d]
---
name: purge
every: 7d
`

// endsnippet:source

// snippet:broken
// The second document opens a flow sequence and never closes it.
const broken = `name: backup
every: 1h
---
name: [report
---
name: purge
every: 7d
`

// endsnippet:broken

// snippet:loop
// stops reports an error after which the decoder reads nothing more of the stream.
func stops(err error) bool {
	return errors.Is(err, yerrors.ErrSyntax) || errors.Is(err, yerrors.ErrExceededMaxDepth)
}

// readJobs decodes one Job per document, and goes on past a document that fails.
func readJobs(w io.Writer, r io.Reader) {
	dec := codec.NewDecoder(r)
	for i := 0; ; i++ {
		var j Job
		err := dec.Decode(&j)
		switch {
		case errors.Is(err, io.EOF):
			return
		case err != nil && stops(err):
			fmt.Fprintf(w, "document %d: stream stops: %v\n", i, firstLine(err))

			return
		case err != nil:
			fmt.Fprintf(w, "document %d: %v, kept %+v\n", i, firstLine(err), j)
		default:
			fmt.Fprintf(w, "document %d: %+v\n", i, j)
		}
	}
}

// endsnippet:loop

// firstLine drops the source excerpt the error draws under its message.
func firstLine(err error) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")

	return msg
}

// Example reads a stream whose second document holds a value that does not fit its field.
func Example() {
	// snippet:run
	readJobs(os.Stdout, strings.NewReader(jobs))
	// endsnippet:run

	// Output:
	// document 0: {Name:backup Every:1h}
	// document 1: [5:8] cannot unmarshal []interface {} into Go struct field Job.Every of type string, kept {Name:report Every:}
	// document 2: {Name:purge Every:7d}
}

// Example_syntaxError reads a stream whose second document cannot be parsed.
func Example_syntaxError() {
	// snippet:runbroken
	readJobs(os.Stdout, strings.NewReader(broken))
	// endsnippet:runbroken

	// Output:
	// document 0: {Name:backup Every:1h}
	// document 1: stream stops: [5:1] a flow collection continues on a line that is not indented past the one it started on
}
