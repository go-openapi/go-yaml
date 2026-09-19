// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
)

// TestRecognizerAgreesWithJSONTestSuite scores the recognizer against every
// document in Nicolas Seriot's JSONTestSuite.
//
// This runs before anything is generated, and it is the one step of the spike
// that must come out clean. A corpus is a cache of the oracle's verdicts, so an
// oracle that is wrong anywhere produces fixtures that accuse a parser of
// defects it does not have -- and unlike a live oracle, a frozen one cannot
// retroactively correct itself.
//
// The naming convention is the suite's own:
//
//   - y_*  must be accepted
//   - n_*  must be rejected
//   - i_*  implementation-defined, so recorded and never asserted
func TestRecognizerAgreesWithJSONTestSuite(t *testing.T) {
	dir := suiteDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	var (
		yes, no, impl int
		disagreements []string
		implAccepted  []string
	)

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		src, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			t.Fatalf("cannot read %s: %v", name, rerr)
		}

		got := jsoncorpus.Text(src).OK

		switch {
		case strings.HasPrefix(name, "y_"):
			yes++
			if !got {
				disagreements = append(disagreements, "refused a valid document: "+name+" -- "+excerpt(src))
			}
		case strings.HasPrefix(name, "n_"):
			no++
			if got {
				disagreements = append(disagreements, "accepted an invalid document: "+name+" -- "+excerpt(src))
			}
		default:
			impl++
			if got {
				implAccepted = append(implAccepted, name)
			}
		}
	}

	t.Logf("scored %d must-accept, %d must-reject, %d implementation-defined", yes, no, impl)
	t.Logf("of the implementation-defined cases, %d of %d are accepted", len(implAccepted), impl)

	if len(disagreements) == 0 {
		t.Logf("no disagreements")

		return
	}

	sort.Strings(disagreements)
	for _, d := range disagreements {
		t.Errorf("%s", d)
	}
	t.Errorf("%d disagreements over %d asserted documents", len(disagreements), yes+no)
}

// excerpt renders the start of a fixture so a disagreement is readable without
// opening the file. The suite carries deliberately hostile bytes, so it is
// quoted rather than printed.
func excerpt(src []byte) string {
	const most = 48

	s := string(src)
	if len(s) > most {
		return strings.TrimSuffix(strconv.Quote(s[:most]), `"`) + `..."`
	}

	return strconv.Quote(s)
}

// suiteDir finds the vendored JSONTestSuite.
//
// The suite lives in a sibling repository rather than here: it is 1.6MB, and a
// spike should not vendor it a second time. JSONTESTSUITE names it outright;
// otherwise the go-openapi checkout root is found by walking up from this file
// and the sibling is taken from there.
func suiteDir(t *testing.T) string {
	t.Helper()

	const under = "core/json/lexers/testdata/JSONTestSuite/test_parsing"

	if named := os.Getenv("JSONTESTSUITE"); named != "" {
		return named
	}

	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot locate this file, so cannot find the suite; set JSONTESTSUITE")
	}

	for at := filepath.Dir(self); at != "/" && at != "."; at = filepath.Dir(at) {
		candidate := filepath.Join(at, under)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	t.Skipf("no %s above %s; set JSONTESTSUITE to score the recognizer", under, self)

	return ""
}
