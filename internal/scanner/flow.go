// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import "github.com/go-openapi/go-yaml/token"

// scanFlowDash refuses a '-' that is neither a sequence entry nor the start of a scalar.
//
// A plain scalar may begin with '-' only when the character behind it can continue the scalar, and neither the
// characters structuring a flow collection nor a blank or a line break can. A flow collection has no block sequence
// entries either, so such a '-' opens nothing at all.
//
// Example:
//
//	[-]        # refused: the ']' cannot continue a scalar
//	[-, -]     # refused, twice
//	[- a]      # refused: the space cannot continue a scalar
//	[-1, -x]   # both read as scalars, '1' and 'x' continuing the '-'
//
// Outside a flow collection the '-' is a sequence entry, so this returns nil there and the entry scan claims it.
// Inside one, a '-' before a blank was left to that scan: "[- a]" read as [[a]] and "{a: -" over "}" as {a: [null]}.
func (s *Scanner) scanFlowDash(ctx *Context) error {
	if ctx.existsBuffer() || !s.isFlowMode() {
		return nil
	}

	switch nc := ctx.nextChar(); {
	case nc == ',', nc == '[', nc == ']', nc == '{', nc == '}':
	case nc == ' ', nc == '\t', nc == 0, isNewLineChar(nc):
	default:
		return nil
	}

	return s.refuse(ctx, '-', "'-' is not a scalar, and a flow collection has no sequence entries")
}

// enterFlow records what a flow collection's continuation lines must clear.
//
// Only the outermost one matters: a collection nested inside another is already past the indentation its parent required.
func (s *Scanner) enterFlow(ctx *Context) {
	if s.isFlowMode() {
		return
	}
	s.flowIndent = s.contentIndent()
	s.flowOpenOffset, s.flowOpenLine = ctx.idx, s.line
}

// isFlowMode reports whether the scan stands inside a flow collection, of either kind.
func (s *Scanner) isFlowMode() bool {
	return s.startedFlowSequenceNum > 0 || s.startedFlowMapNum > 0
}

func (s *Scanner) scanFlowMapStart(ctx *Context) bool {
	if ctx.existsBuffer() && !s.isFlowMode() {
		return false
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf('{')
	ctx.addTokenValue(token.MakeMappingStart(ctx.origin(), s.pos()))
	s.enterFlow(ctx)
	s.startedFlowMapNum++
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

// scanFlowMapEnd hands a '}' over as a token inside any flow collection, and leaves it to a plain scalar outside one.
//
// A plain scalar inside a flow collection holds no flow indicator (ns-plain-safe(c) in YAML 1.2), so a '}' ends it
// even where no flow mapping is open. Kept as text, "[b}]" read as the sequence ["b}"] where yaml.v3 and the spec
// refuse it. The parser rejects the '}' that closes no mapping.
func (s *Scanner) scanFlowMapEnd(ctx *Context) bool {
	if !s.isFlowMode() {
		return false
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf('}')
	ctx.addTokenValue(token.MakeMappingEnd(ctx.origin(), s.pos()))
	if s.startedFlowMapNum > 0 {
		s.startedFlowMapNum--
	}
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

func (s *Scanner) scanFlowArrayStart(ctx *Context) bool {
	if ctx.existsBuffer() && !s.isFlowMode() {
		return false
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf('[')
	ctx.addTokenValue(token.MakeSequenceStart(ctx.origin(), s.pos()))
	s.enterFlow(ctx)
	s.startedFlowSequenceNum++
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

// scanFlowArrayEnd hands a ']' over as a token, as [Scanner.scanFlowMapEnd] does a '}': "{a: b]}" read the value
// "b]". Outside a flow collection a ']' continues a plain scalar under way, and opens nothing otherwise.
func (s *Scanner) scanFlowArrayEnd(ctx *Context) bool {
	if ctx.existsBuffer() && !s.isFlowMode() {
		return false
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf(']')
	ctx.addTokenValue(token.MakeSequenceEnd(ctx.origin(), s.pos()))
	if s.startedFlowSequenceNum > 0 {
		s.startedFlowSequenceNum--
	}
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

func (s *Scanner) scanFlowEntry(ctx *Context, c rune) bool {
	if s.startedFlowSequenceNum <= 0 && s.startedFlowMapNum <= 0 {
		return false
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf(c)
	ctx.addTokenValue(token.MakeCollectEntry(ctx.origin(), s.pos()))
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}
