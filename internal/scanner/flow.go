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
	if ctx.existsBuffer() || !s.isFlowMode() || s.inAnchorName('-') {
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

// scanFlowQuestion refuses a '?' inside a flow collection that a flow indicator follows.
//
// ns-plain-first [126] opens a plain scalar with a '?' only before an ns-plain-safe character, which leaves out the
// flow indicators [129]. An explicit key's '?' needs a separation after it ([142], [150]). So nothing reads the '?' of
// "[?]", "{?}", "[?, a]" or "{a: ?}", and each was read as the scalar "?". "[? ]", "[?" over "]" and "[?x]" stay as
// they were: an empty pair, an empty pair, and the scalar "?x".
//
// A '?' inside an anchor or alias name is the name's: "[&? x, *?]" reads ["x", "x"].
func (s *Scanner) scanFlowQuestion(ctx *Context) error {
	if ctx.existsBuffer() || !s.isFlowMode() || s.inAnchorName('?') {
		return nil
	}

	switch ctx.nextChar() {
	case ',', '[', ']', '{', '}':
	default:
		return nil
	}

	return s.refuse(ctx, '?', "'?' is not a scalar, and an explicit key needs a space after it")
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
	s.flowOpen = append(s.flowOpen, '{')
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

// scanFlowMapEnd hands a '}' over as a token inside any flow collection, and leaves it to a plain scalar outside one.
//
// A plain scalar inside a flow collection holds no flow indicator (ns-plain-safe(c) in YAML 1.2), so a '}' ends it
// even where no flow mapping is open. Kept as text, "[b}]" read as the sequence ["b}"] where yaml.v3 and the spec
// refuse it. The parser rejects the '}' that closes no mapping.
//
// A '}' while a flow sequence is the innermost collection open is refused here: "[{a]}" and "[}]" close the wrong
// collection.
func (s *Scanner) scanFlowMapEnd(ctx *Context) (bool, error) {
	if !s.isFlowMode() {
		return false, nil
	}
	if !s.closesInnermostFlow('{') {
		// The closer ends a plain scalar under way, as it does where it may stand, so the scalar goes over first.
		s.addBufferedTokenIfExists(ctx)

		return false, s.refuse(ctx, '}', "found '}' where ']' closes a flow sequence")
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf('}')
	ctx.addTokenValue(token.MakeMappingEnd(ctx.origin(), s.pos()))
	if s.startedFlowMapNum > 0 {
		s.startedFlowMapNum--
	}
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true, nil
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
	s.flowOpen = append(s.flowOpen, '[')
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true
}

// scanFlowArrayEnd hands a ']' over as a token, as [Scanner.scanFlowMapEnd] does a '}': "{a: b]}" read the value
// "b]". Outside a flow collection a ']' continues a plain scalar under way, and opens nothing otherwise.
//
// A ']' while a flow mapping is the innermost collection open is refused here: "[{]}" closes the wrong collection.
func (s *Scanner) scanFlowArrayEnd(ctx *Context) (bool, error) {
	if ctx.existsBuffer() && !s.isFlowMode() {
		return false, nil
	}
	if !s.closesInnermostFlow('[') {
		// The closer ends a plain scalar under way, as it does where it may stand, so the scalar goes over first.
		s.addBufferedTokenIfExists(ctx)

		return false, s.refuse(ctx, ']', "found ']' where '}' closes a flow mapping")
	}

	s.endsProperty(ctx)
	ctx.addOriginBuf(']')
	ctx.addTokenValue(token.MakeSequenceEnd(ctx.origin(), s.pos()))
	if s.startedFlowSequenceNum > 0 {
		s.startedFlowSequenceNum--
	}
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true, nil
}

// closesInnermostFlow reports whether a closer for the collection opened by opener may stand here, and pops that
// collection when it may.
//
// With no flow collection open the closer stands outside every one, and the parser refuses the closer that closes
// nothing. Otherwise it must close the innermost collection.
func (s *Scanner) closesInnermostFlow(opener byte) bool {
	n := len(s.flowOpen)
	if n == 0 {
		return true
	}
	if s.flowOpen[n-1] != opener {
		return false
	}
	s.flowOpen = s.flowOpen[:n-1]

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
