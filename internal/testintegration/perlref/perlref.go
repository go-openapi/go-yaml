// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package perlref runs the YAML 1.2 reference parser and returns its event
// stream.
//
// # A different question from the other two
//
// [libfyaml] and [goyaml] say what a document *denotes*, and the corpus asks
// them where a meaning is in doubt. This one says what a document *is*, and it
// resolves nothing at all: a plain "1.0" comes back as "=VAL :1.0", the text
// that was written, with no schema applied and no type decided.
//
// That is the question underneath the other two. When libfyaml reads "1.0: a"
// as the key "1.0" and this library reads it as "1", both are naming a node
// that neither of them disagrees about -- and the reference parser is what
// shows the node. It settles structure and identity, which is exactly what JSON
// cannot express and where the other two sources are weakest.
//
// yaml/yaml-reference-parser generates a parser into four languages from the
// specification's own grammar. This is the Perl one, chosen because it needs
// nothing but perl.
//
// # It is not a value oracle
//
// It has no schema, builds no values and answers no question about what a
// scalar means. Asking it whether "1.0" is a float is asking the wrong source.
//
// [libfyaml]: https://pkg.go.dev/github.com/go-openapi/go-yaml/internal/testintegration/libfyaml
// [goyaml]: https://pkg.go.dev/github.com/go-openapi/go-yaml/internal/testintegration/goyaml
package perlref

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInstalled says the reference parser could not be found.
var ErrNotInstalled = errors.New(
	"perlref: not installed, run hack/conformance/install-reference-parser.sh")

// Home is where the install script puts it.
//
// REFPARSER_HOME overrides it, and the script reads the same variable, so the
// two cannot drift apart.
func Home() string {
	if set := os.Getenv("REFPARSER_HOME"); set != "" {
		return set
	}

	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}

		cache = filepath.Join(home, ".cache")
	}

	return filepath.Join(cache, "go-openapi", "yaml-reference-parser")
}

// Available reports whether the reference parser can be run.
func Available() bool {
	_, err := os.Stat(filepath.Join(Home(), "bin", "yaml-parser"))

	return err == nil
}

// Commit is the pinned revision, so a corpus entry that rests on this reading
// records which one gave it.
func Commit() string {
	out, err := os.ReadFile(filepath.Join(Home(), "COMMIT"))
	if err != nil {
		return ""
	}

	first, _, _ := strings.Cut(string(out), "\n")

	return strings.TrimSpace(first)
}

// Events runs src through the reference parser and returns the event stream,
// one line per event, and whether the parse succeeded.
//
// A failed parse still returns the events it managed, which is the useful part:
// where a document stops being one is more informative than the fact that it
// did.
func Events(src []byte) (events []string, ok bool, err error) {
	if !Available() {
		return nil, false, ErrNotInstalled
	}

	cmd := exec.CommandContext(context.Background(), "perl", filepath.Join(Home(), "bin", "yaml-parser")) //nolint:gosec // the reference parser the install script put under Home
	cmd.Stdin = bytes.NewReader(src)

	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs

	// A refused document exits non-zero on some builds and zero on others, and
	// the PASS/FAIL line is the answer either way, so the exit status is not
	// consulted. Only a failure to run at all is an error.
	if runErr := cmd.Run(); runErr != nil {
		if _, ok := errors.AsType[*exec.ExitError](runErr); !ok {
			return nil, false, fmt.Errorf("perlref: %w: %s", runErr, strings.TrimSpace(errs.String()))
		}
	}

	return parse(out.String())
}

// parse reads the parser's report: a PASS or FAIL line naming the document,
// then the events, then a timing line this package has no use for.
func parse(out string) ([]string, bool, error) {
	var (
		events []string
		seen   bool
		ok     bool
	)

	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "PASS"):
			seen, ok = true, true
		case strings.HasPrefix(line, "FAIL"):
			seen, ok = true, false
		case strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") ||
			strings.HasPrefix(line, "="):
			events = append(events, line)
		}
	}

	if !seen {
		return nil, false, fmt.Errorf("perlref: no PASS or FAIL in its report: %q", out)
	}

	return events, ok, nil
}
