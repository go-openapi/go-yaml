// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAParserReadsOneStreamUntilReset checks that Parse and Walk return parser.ErrParserReused
// on a Parser that has already read a stream, in every order, and read again after Reset.
func TestAParserReadsOneStreamUntilReset(t *testing.T) {
	t.Parallel()

	const src = "a: 1\n"
	uses := map[string]func(p *parser.Parser) (*ast.File, error){
		"Parse": func(p *parser.Parser) (*ast.File, error) { return p.Parse([]byte(src)) },
		"Walk":  func(p *parser.Parser) (*ast.File, error) { return p.Walk([]byte(src), discardVisitor{}) },
	}

	for firstName, first := range uses {
		for secondName, second := range uses {
			t.Run(firstName+" then "+secondName, func(t *testing.T) {
				t.Parallel()

				p := parser.New()
				_, err := first(p)
				require.NoError(t, err)

				f, err := second(p)
				require.ErrorIs(t, err, parser.ErrParserReused)
				assert.Nil(t, f)

				p.Reset()
				_, err = second(p)
				require.NoError(t, err)
			})
		}
	}

	t.Run("a parse that fails still uses the parser", func(t *testing.T) {
		t.Parallel()

		p := parser.New()
		_, err := p.Parse([]byte("a: [\n"))
		require.Error(t, err)
		require.NotErrorIs(t, err, parser.ErrParserReused)

		_, err = p.Parse([]byte(src))
		require.ErrorIs(t, err, parser.ErrParserReused)

		p.Reset()
		f, err := p.Parse([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, src, f.String())
	})
}

// TestResetStartsFromTheDefaults checks that Reset drops the options given to New and applies its own.
func TestResetStartsFromTheDefaults(t *testing.T) {
	t.Parallel()

	const src = "a: 1 # note\n"
	p := parser.New(parser.WithComments())

	f, err := p.Parse([]byte(src))
	require.NoError(t, err)
	assert.Equal(t, src, f.String())

	p.Reset()
	f, err = p.Parse([]byte(src))
	require.NoError(t, err)
	assert.Equal(t, "a: 1\n", f.String(), "Reset without options keeps no comment")

	p.Reset(parser.WithComments())
	f, err = p.Parse([]byte(src))
	require.NoError(t, err)
	assert.Equal(t, src, f.String())
}

// TestAResetParserReadsWhatANewOneReads reads every walk source with one Parser, Reset between two sources,
// and compares each result with a new Parser's: the rendered text and the error of Parse, and the digest of Walk.
//
// The sources include documents the parser rejects, so Reset must also clear a parse that stopped part way.
func TestAResetParserReadsWhatANewOneReads(t *testing.T) {
	t.Parallel()

	sources := walkSources(t)
	modes := map[string][]parser.Option{
		"without comments": nil,
		"with comments":    {parser.WithComments()},
	}

	for mode, opts := range modes {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			// mixed alternates the two, since a walk leaves the arenas in another state than a parse does.
			parses := parser.New(opts...)
			walks := parser.New(opts...)
			mixed := parser.New(opts...)
			for i, source := range sources {
				src := []byte(source.text)
				wantParse := renderParse(parser.New(opts...).Parse(src))
				wantWalk := digestWalk(parser.New(opts...), src)

				parses.Reset(opts...)
				assert.Equalf(t, wantParse, renderParse(parses.Parse(src)), "%s: Parse after Reset", source.name)

				walks.Reset(opts...)
				assert.Equalf(t, wantWalk, digestWalk(walks, src), "%s: Walk after Reset", source.name)

				mixed.Reset(opts...)
				if i%2 == 0 {
					assert.Equalf(t, wantParse, renderParse(mixed.Parse(src)), "%s: Parse after a Walk", source.name)
				} else {
					assert.Equalf(t, wantWalk, digestWalk(mixed, src), "%s: Walk after a Parse", source.name)
				}
			}
		})
	}
}

// renderParse returns the text a parse renders to, or its error message.
func renderParse(f *ast.File, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}

	return f.String()
}

// digestWalk returns the digest of what a walk of src hands over, or its error message.
func digestWalk(p *parser.Parser, src []byte) string {
	sum := sha256.New()
	if _, err := p.Walk(src, &digestVisitor{out: sum}); err != nil {
		return "error: " + err.Error()
	}

	return hex.EncodeToString(sum.Sum(nil))
}

// discardVisitor visits every node and keeps nothing.
type discardVisitor struct{}

func (discardVisitor) Enter(ast.Node, parser.Cursor) error  { return nil }
func (discardVisitor) Leave(ast.Node, parser.Closing) error { return nil }
