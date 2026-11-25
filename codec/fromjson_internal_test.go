// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// TestTheLexerAndTheParserAgreeOnTheJSONTestSuite reads every file of the JSONTestSuite that
// github.com/go-openapi/core/json ships twice: with FromJSON, which reads it with core's lexer, and with the YAML
// parser's walk under WithJSONSyntax, which FromJSON used before. Both must accept the same files and write the same
// YAML for them, and FromJSON must reject the others with ErrNotJSONSyntax or ErrDuplicateKey.
func TestTheLexerAndTheParserAgreeOnTheJSONTestSuite(t *testing.T) {
	t.Parallel()

	dir, err := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", "github.com/go-openapi/core/json").Output()
	require.NoError(t, err)
	root := filepath.Join(strings.TrimSpace(string(dir)), "lexers", "testdata", "JSONTestSuite", "test_parsing")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Greater(t, len(entries), 300)

	var accepted, rejected int
	for _, e := range entries {
		src, err := os.ReadFile(filepath.Join(root, e.Name()))
		require.NoError(t, err)

		fromLexer, lexerErr := FromJSON(src)
		fromParser, parserErr := fromJSON(src, true)
		require.Equalf(t, parserErr == nil, lexerErr == nil, "%s: lexer %v, parser %v", e.Name(), lexerErr, parserErr)
		if lexerErr != nil {
			rejected++
			assert.Truef(t,
				errors.Is(lexerErr, yamlerrors.ErrNotJSONSyntax) || errors.Is(lexerErr, yamlerrors.ErrDuplicateKey),
				"%s: %v", e.Name(), lexerErr)

			continue
		}
		accepted++
		assert.Equalf(t, string(fromParser), string(fromLexer), "%s", e.Name())
	}
	t.Logf("%d files: %d accepted and %d rejected by both readers", len(entries), accepted, rejected)
}
