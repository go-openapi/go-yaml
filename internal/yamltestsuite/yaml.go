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
	"strings"
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

func TestSuites() ([]*TestSuite, error) {
	dir := fixturesDir()
	testMap := make(map[string]*TestSuite)
	if err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, dir+"/")
		name = strings.TrimSuffix(name, "/"+filepath.Base(name))
		if _, exists := testMap[name]; !exists {
			testMap[name] = &TestSuite{}
		}
		f, err := os.ReadFile(path)
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
