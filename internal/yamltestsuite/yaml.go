// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamltestsuite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

type TestSuite struct {
	Name    string
	InYAML  []byte
	InJSON  []any
	OutYAML []byte
	Error   bool

	// hasJSON records that an in.json file was present, which is not the same
	// as InJSON being non-empty: an empty in.json is a real expectation -- the
	// stream yields nothing -- and empty-stream relies on exactly that.
	hasJSON bool
}

// HasExpectation reports whether the fixture says anything about what should
// happen to its document.
//
// Nine of the vendored cases carry only in.yaml -- no in.json, no out.yaml, no
// error marker -- and every one of them is an empty-key or explicit-key case.
// Reading "no error marker" as "must be accepted" would invent an expectation
// the fixture does not state, and score us against it either way.
func (t *TestSuite) HasExpectation() bool {
	return t.Error || t.hasJSON || t.OutYAML != nil
}

// fixturesDir is the vendored copy of the YAML Test Suite, held in this
// package's testdata directory so that the fixtures sit beside the loader that
// reads them.
func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0) //nolint:dogsled

	return filepath.Join(filepath.Dir(file), "testdata")
}

// TestSuites reads the vendored YAML Test Suite.
func TestSuites() ([]*TestSuite, error) {
	return suitesIn(fixturesDir())
}

// suiteName names the suite a fixture file belongs to: the directory holding it, relative to dir, with forward
// slashes on every OS, as in "trailing-whitespace-in-streams/02".
//
// The name was cut out of the path with "/", and filepath.Walk hands a Windows path over with "\\", so nothing was
// cut and every suite there was named after the whole path of its in.yaml.
func suiteName(dir, path string) string {
	rel, err := filepath.Rel(dir, filepath.Dir(path))
	if err != nil {
		return filepath.ToSlash(path)
	}

	return filepath.ToSlash(rel)
}

// suitesIn reads the test suites under dir, one per directory holding an in.yaml.
func suitesIn(dir string) ([]*TestSuite, error) {
	testMap := make(map[string]*TestSuite)
	if err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			// Walk hands a nil info with the error, so it is checked first.
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := suiteName(dir, path)
		if _, exists := testMap[name]; !exists {
			testMap[name] = &TestSuite{}
		}
		f, err := os.ReadFile(path) //nolint:gosec // walks the YAML test suite checked out with the repository
		if err != nil {
			return err
		}
		fileName := filepath.Base(path)
		switch fileName {
		case "in.yaml":
			testMap[name].InYAML = f
		case "in.json":
			dec := json.NewDecoder(bytes.NewReader(f))
			var inJSON []any
			for {
				var v any
				if err := dec.Decode(&v); err != nil {
					if err == io.EOF {
						break
					}
					return fmt.Errorf("failed to decode json: %s: %s: %w", name, string(f), err)
				}
				inJSON = append(inJSON, v)
			}
			testMap[name].InJSON = inJSON
			testMap[name].hasJSON = true
		case "out.yaml":
			testMap[name].OutYAML = f
		case "error":
			testMap[name].Error = true
		}
		testMap[name].Name = name
		return nil
	}); err != nil {
		return nil, err
	}

	tests := make([]*TestSuite, 0, len(testMap))
	for _, test := range testMap {
		if test.InYAML == nil {
			continue
		}
		tests = append(tests, test)
	}
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})
	return tests, nil
}
