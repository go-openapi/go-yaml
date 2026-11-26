// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

// addTag builds the token for a tag the scan has read through, and returns the error for one the YAML 1.2 grammar
// does not admit.
//
// It sits here and not in the parser because the scanner already refuses a tag holding a flow indicator.
// "!<>" raises the same complaint: the grammar has no production for it.
func (s *Scanner) addTag(ctx *Context, value string, tagPos token.Position) error {
	if msg := checkTagText(value); msg != "" {
		return ErrInvalidToken(msg, token.Invalid(ctx.origin(), s.pos()))
	}
	ctx.addTokenValue(token.MakeTag(value, ctx.origin(), tagPos))

	return nil
}

func (s *Scanner) scanTag(ctx *Context) (bool, error) {
	if ctx.existsBuffer() || s.inAnchorName('!') {
		return false, nil
	}

	ctx.addOriginBuf('!')
	// The offset counts the bytes the cursor has crossed and the column counts the characters, so both take the '!'.
	// The offset was left out once and stayed a byte behind for the rest of the document; the column was left out with
	// it and stayed a character behind, so "!!str k: v" reported k at offset 6 -- which addresses it -- and column 6,
	// where it is the seventh character. Every token after a tag on that line was one short.
	// tagPos is taken before the step, where the tag's own text begins.
	tagPos := s.pos()
	s.progressColumn(ctx, 1) // skip '!' character

	// A verbatim tag, "!<...>", holds a URI and takes it as written: the characters a shorthand may not contain are
	// ordinary inside the brackets.
	verbatim := ctx.currentChar() == '<'

	// idx counts bytes into the source; progress counts the characters the column has to advance by, which is not the same
	// thing.
	var progress int
	for idx, c := range ctx.src[ctx.idx:] {
		progress++
		if verbatim {
			ctx.addOriginBuf(c)
			if c == '>' {
				verbatim = false
			}

			continue
		}
		switch c {
		case ' ', '\t':
			// s-separate-in-line is s-white+, and s-white is a space or a tab,
			// so both end a tag shorthand. Only the space did: a tab fell to
			// the default arm and joined the name, and "a: !!str\tx" was
			// refused as "found invalid tag character". The grammar reads it,
			// and so do go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1.
			ctx.addOriginBuf(c)
			value := ctx.source(ctx.idx-1, ctx.idx+idx)
			if err := s.addTag(ctx, value, tagPos); err != nil {
				return false, err
			}
			s.progressColumn(ctx, utf8.RuneCountInString(value))
			ctx.clear()
			return true, nil
		case ',':
			if s.startedFlowSequenceNum > 0 || s.startedFlowMapNum > 0 {
				value := ctx.source(ctx.idx-1, ctx.idx+idx)
				if err := s.addTag(ctx, value, tagPos); err != nil {
					return false, err
				}
				s.progressColumn(ctx, utf8.RuneCountInString(value)-1) // progress column before collect-entry for scanning it at scanFlowEntry function.
				ctx.clear()
				return true, nil
			}
			// Outside a flow collection nothing ends the tag here, and a ',' is not a character a tag may contain: it has to be
			// percent-encoded.
			ctx.addOriginBuf(c)
			s.progressColumn(ctx, progress)

			return false, ErrInvalidToken(fmt.Sprintf("a tag cannot hold %q", c), token.Invalid(ctx.origin(), s.pos()))
		case '\n', '\r':
			ctx.addOriginBuf(c)
			value := ctx.source(ctx.idx-1, ctx.idx+idx)
			if err := s.addTag(ctx, value, tagPos); err != nil {
				return false, err
			}
			s.progressColumn(ctx, utf8.RuneCountInString(value)-1) // progress column before new-line-char for scanning new-line-char at scanNewLine function.
			ctx.clear()
			return true, nil
		case '}', ']':
			if s.startedFlowSequenceNum > 0 || s.startedFlowMapNum > 0 {
				// The closer ends the collection the tag stands in, so it ends the tag: "[!]" is the non-specific tag on the empty
				// node and not a tag whose name is "]".
				value := ctx.source(ctx.idx-1, ctx.idx+idx)
				if err := s.addTag(ctx, value, tagPos); err != nil {
					return false, err
				}
				s.progressColumn(ctx, utf8.RuneCountInString(value)-1) // progress column before the closer so it is scanned on its own

				ctx.clear()

				return true, nil
			}

			ctx.addOriginBuf(c)
			s.progressColumn(ctx, progress)
			invalidMsg := fmt.Sprintf("a tag cannot hold %q", c)
			invalidTk := token.Invalid(ctx.origin(), s.pos())

			return false, ErrInvalidToken(invalidMsg, invalidTk)
		case '{':
			ctx.addOriginBuf(c)
			s.progressColumn(ctx, progress)
			invalidMsg := fmt.Sprintf("a tag cannot hold %q", c)
			invalidTk := token.Invalid(ctx.origin(), s.pos())

			return false, ErrInvalidToken(invalidMsg, invalidTk)
		default:
			ctx.addOriginBuf(c)
		}
	}
	// The source ran out while the tag was being read. It is still a tag --
	// "k: !!str" with no closing break is a document, and the loop above only
	// ever emits on the character that ends the tag, so falling out of it here
	// dropped the token and the tag with it.
	value := ctx.source(ctx.idx-1, len(ctx.src))
	if err := s.addTag(ctx, value, tagPos); err != nil {
		return false, err
	}
	s.progressColumn(ctx, progress)
	ctx.clear()

	return true, nil
}
