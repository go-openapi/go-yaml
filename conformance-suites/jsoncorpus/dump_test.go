// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/jsoncorpus"
	"github.com/go-openapi/conformance-suites/stance"
)

// TestFreezeCorpus writes a generated corpus out for replay by a parser this
// module cannot import.
//
// The lexer under test lives in another repository, so the corpus crosses the
// boundary as a file: name, mutation, the expectation a stance derives, and the
// bytes. It writes only what a replay is entitled to know -- there is no
// verdict about any parser in it, and an entry no stance can decide is left
// out rather than guessed at.
//
// Skipped unless CORPUSDUMP names a destination, since freezing a corpus is a
// deliberate act and not something an ordinary test run should do.
func TestFreezeCorpus(t *testing.T) {
	out := os.Getenv("CORPUSDUMP")
	if out == "" {
		t.Skip("set CORPUSDUMP")
	}

	docs, mut := 500, 8
	if v := os.Getenv("CORPUSDOCS"); v != "" {
		docs, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("CORPUSMUT"); v != "" {
		mut, _ = strconv.Atoi(v)
	}
	entries := jsoncorpus.Generate(1, docs, mut)

	var b strings.Builder
	for _, e := range entries {
		want, _ := jsoncorpus.DefaultLexer.Expect(e.Doc)
		if want == stance.Undecided {
			continue
		}
		b.WriteString(strings.Join([]string{
			e.Name, e.Mutation, want.String(),
			base64.StdEncoding.EncodeToString(e.Doc.Src),
		}, "\t") + "\n")
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil { //nolint:gosec // the dump goes where the person running the test asked for it
		t.Fatal(err)
	}
	t.Logf("%d entries -> %s", len(entries), out)
}
