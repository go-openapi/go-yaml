// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package libfyaml reads a document with libfyaml, so the corpus can settle a
// question against a second implementation instead of against itself.
//
// # Why a second implementation at all
//
// The recognizer answers whether a run of bytes is a document and nothing
// else. Alias resolution, key identity and what a tag means are outside the
// grammar, so for those the corpus has only what somebody wrote down by hand --
// and a hand-written answer is one library's opinion until something
// independent agrees with it. Fifty-seven of its documents are well formed and
// state no meaning at all for exactly that reason.
//
// libfyaml is a separate implementation of YAML 1.2, in C, by the author of the
// specification's own test suite. Where it and this library agree, a stated
// meaning rests on two readings rather than one. Where they disagree, one of
// them is wrong and the corpus says which rather than guessing.
//
// # It is a yardstick and not an oracle
//
// Two limits, both met in practice and both worth knowing before a
// disagreement is read as a verdict.
//
// [Load] goes through json_dumps, which is a lossy view: a mapping with two
// keys that render alike comes back as JSON with a repeated key, so the
// rendering is not always the key identity. Where the question is identity
// rather than value, compare event streams instead.
//
// And loads builds a value, so a refusal may be the binding declining to hold
// something rather than the parser refusing the document. Cycles are where that
// matters.
//
// # A merge needs [LoadMode]
//
// The merge key is a YAML 1.1 type (yaml.org/type/merge) and not part of 1.2,
// so libfyaml resolves "<<" only when it is asked for 1.1. [Load] asks for 1.2,
// where "a: &a {x: 1}" over "m:" over "  <<: *a" comes back as
// {"m": {"<<": {"x": 1}}} -- the handle written out as a member name. Read that
// as the mode and not as libfyaml lacking the feature.
//
// Ask [LoadMode] with [YAML11] for any question about what a merge means. It
// is still not an oracle for two of them: it resolves a merge by document order,
// so the mapping's own key loses to a "<<" written after it, where the merge
// type says "unless the key already exists in it" and go.yaml.in/yaml/v3 agrees
// with the type. And it refuses no duplicate key at all -- "{a: 1, a}" comes
// back as {"a": null} -- so it cannot corroborate a duplicate-key refusal.
//
// # Read the error, not the failure
//
// That second limit cuts both ways, and both directions have cost a register
// entry, so the discriminator is worth stating: **a position and a parser
// message is a syntax verdict; a Python traceback is not.**
//
//	{[a\nb]: 1}    missing comma in flow mapping at 2:3   -- the C parser
//	{[a, b]: 1}    TypeError: unhashable type: 'sequence' -- the binding, after a clean parse
//
// Taking the first for the binding put a wrong ruling in yamlgen.Strict; taking
// the second for the parser would have put a wrong defect in the ledger. When a
// refusal decides a claim, quote the message into the entry so the next reader
// can tell which one it was.
//
// The other tell is a tag whose type cannot be built from its content:
// "{a: !!bool &x}" is refused here because no boolean comes from an empty node,
// and "{a: !!str &x}" reads. Ask with `!!str` when the question is syntax, or
// ask [github.com/go-openapi/go-yaml/internal/testintegration/perlref], which
// builds nothing and so cannot make either mistake.
package libfyaml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInstalled says libfyaml could not be found.
//
// A test that wants it skips on this rather than failing: the harness has to
// run on a machine that has never installed it, and a conformance suite that
// cannot be run without a C library is one nobody runs.
var ErrNotInstalled = errors.New("libfyaml: not installed, run hack/conformance/install-libfyaml.sh")

// Home is where the install script puts the bindings.
//
// LIBFYAML_HOME overrides it, and the script reads the same variable, so the
// two cannot drift apart.
func Home() string {
	if set := os.Getenv("LIBFYAML_HOME"); set != "" {
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

	return filepath.Join(cache, "go-openapi", "libfyaml")
}

// Available reports whether libfyaml can be reached.
func Available() bool { return Version() != "" }

// Version is the installed version, empty when there is none.
func Version() string {
	out, err := run(`import libfyaml; print(getattr(libfyaml, "__version__", "unknown"))`, "")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}

// Mode is libfyaml's own "mode" argument: the YAML version it reads a document
// under. It selects the resolution table and, with it, whether "<<" is a merge
// key.
type Mode string

const (
	// YAML12 is libfyaml's default: the 1.2 core schema, where "<<" is an
	// ordinary key.
	YAML12 Mode = "1.2"
	// YAML11 reads the 1.1 types, which is the only mode that resolves a
	// merge. Use it for a question about what "<<" means and for nothing else.
	YAML11 Mode = "1.1"
)

// Load reads src under YAML 1.2 and returns what libfyaml makes of each
// document in it, rendered as JSON.
//
// A document libfyaml refuses returns an error naming what it said. Use
// [LoadMode] with [YAML11] when the question is what a merge means.
func Load(src []byte) ([]string, error) { return LoadMode(src, YAML12) }

// LoadMode is [Load] under a named YAML version.
func LoadMode(src []byte, mode Mode) ([]string, error) {
	const script = `
import json, sys
import libfyaml

mode = sys.argv[1] if len(sys.argv) > 1 else "1.2"
src = sys.stdin.buffer.read().decode("utf-8", "surrogateescape")
out = [libfyaml.json_dumps(doc) for doc in libfyaml.loads_all(src, mode=mode)]
print(json.dumps(out))
`

	out, err := run(script, string(src), string(mode))
	if err != nil {
		return nil, err
	}

	var docs []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &docs); err != nil {
		return nil, fmt.Errorf("libfyaml: reading back its answer: %w", err)
	}

	return docs, nil
}

func run(script, stdin string, args ...string) (string, error) {
	home := Home()
	if home == "" {
		return "", ErrNotInstalled
	}

	if _, err := os.Stat(filepath.Join(home, "libfyaml")); err != nil {
		return "", ErrNotInstalled
	}

	argv := append([]string{"-c", script}, args...)
	cmd := exec.CommandContext(context.Background(), "python3", argv...)
	cmd.Env = append(os.Environ(), "PYTHONPATH="+home)
	cmd.Stdin = strings.NewReader(stdin)

	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("libfyaml: %w: %s", err, strings.TrimSpace(errs.String()))
	}

	return out.String(), nil
}
