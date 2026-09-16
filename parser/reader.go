// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	runarena "github.com/go-openapi/go-yaml/parser/arena"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// reader turns a document into the tokens the descent reads, as the descent pulls them.
//
// It scans a token into the arena, feeds it to the grouping, and passes the grouped result on a token at a time.
// It never reads further than the descent has pulled,
// so the scanner, the grouping and the parse run together and the tape can be refilled behind them.
//
// A document opens at "---" or at the first token of the stream,
// and closes at "...", at the "---" opening the next one, or at the end of the stream.
// The parse calls [reader.openDocument], then [reader.bodyToken] until it returns false, then [reader.closeDocument].
type reader struct {
	scan  *scanner.Scanner
	arena *tokenarena.TokenArena[group.TapeToken]
	g     group.Grouper

	// out holds the tokens the grouping has emitted and the descent has not taken yet, from index at onward.
	out []*group.TapeToken
	at  int

	// seq is the tape position of the next token read.
	seq int
	// drained is set once the scanner has no more tokens,
	// and finished once the grouping has emitted the tokens it was still holding.
	drained, finished bool
	// keepComments is set under WithComments.
	// Without it, comments are dropped as they arrive and never reach the grouping.
	keepComments bool
	// onToken receives every token the scanner cuts, before the comment drop and before the grouping.
	// See [WithTokens].
	onToken func(token.Token)

	// afterHeader and afterEnd hold the marker just read,
	// so that the token after it can be checked against the marker's line.
	afterHeader, afterEnd *group.TapeToken
	// taken is set once any token has been read:
	// an empty stream is one empty document, and a "..." closing nothing is none.
	// ended is set when the current document has run out.
	// tail is set once the empty document at the end of a stream has been returned.
	taken, ended, tail bool
	// tookDirective is set when the body just returned a directive.
	// A directive is a document of its own, so the next directive closes this document.
	tookDirective bool

	// err holds an error bodyToken cannot return, since the descent's pull returns a token or nothing.
	err error
}

// newReader returns a reader over the tokens of scan.
//
// estimate is a guess at the document's token count. It sizes the grouping's buffers and nothing else.
func newReader(
	scan *scanner.Scanner,
	arena *tokenarena.TokenArena[group.TapeToken],
	estimate int,
	keepComments bool,
	onToken func(token.Token),
) *reader {
	r := &reader{
		scan:         scan,
		arena:        arena,
		g:            group.NewGrouper(estimate),
		out:          make([]*group.TapeToken, 0, runarena.MinGroupBlock),
		keepComments: keepComments,
		onToken:      onToken,
	}
	if keepComments {
		// The map is made here, before the first comment arrives, so the parser can hold the same map from the start.
		// A parse that drops comments makes none.
		r.g.LineComments = make(map[*group.TapeToken]*token.Token)
	}

	return r
}

// reset prepares r for another stream over arena, and keeps the grouper's cells and the room r's buffers have grown.
func (r *reader) reset(arena *tokenarena.TokenArena[group.TapeToken], keepComments bool, onToken func(token.Token)) {
	r.g.Reset()
	clear(r.out[:cap(r.out)])
	*r = reader{scan: r.scan, arena: arena, g: r.g, out: r.out[:0], keepComments: keepComments, onToken: onToken}

	switch {
	case !keepComments:
		r.g.LineComments = nil
	case r.g.LineComments == nil:
		r.g.LineComments = make(map[*group.TapeToken]*token.Token)
	}
}

// peek returns the next grouped token without taking it, and fills the buffer when it is empty.
func (r *reader) peek() (*group.TapeToken, error) {
	for r.at >= len(r.out) {
		if r.drained {
			return nil, nil
		}
		if err := r.fill(); err != nil {
			return nil, err
		}
	}

	return r.out[r.at], nil
}

// take returns the next grouped token and steps past it.
func (r *reader) take() (*group.TapeToken, error) {
	tk, err := r.peek()
	if err != nil || tk == nil {
		return nil, err
	}
	r.at++
	r.taken = true

	return tk, nil
}

// openDocument begins the next document and returns the "---" that opened it, if there is one.
// The bool is false at the end of the stream.
func (r *reader) openDocument() (*group.TapeToken, bool, error) {
	// A "..." where no document is open closes nothing:
	// l-yaml-stream allows a run of suffixes, and only the first closes a document.
	for {
		tk, err := r.peek()
		if err != nil {
			return nil, false, err
		}
		if tk == nil || tk.Type() != token.DocumentEndType {
			break
		}
		if _, err := r.take(); err != nil {
			return nil, false, err
		}
		r.afterEnd = tk
		if err := r.judgeNext(); err != nil {
			return nil, false, err
		}
	}

	tk, err := r.peek()
	if err != nil {
		return nil, false, err
	}
	r.ended = false
	r.tookDirective = false

	if tk == nil {
		// The stream ends here. It holds one last, empty document unless a token was read from it.
		if r.taken || r.tail {
			return nil, false, nil
		}
		r.tail = true

		return nil, true, nil
	}
	if tk.Type() != token.DocumentHeaderType {
		return nil, true, nil
	}

	head, err := r.take()
	if err != nil {
		return nil, false, err
	}
	r.afterHeader = head

	// Return the tape token, not its raw one: the comment closing the "---" line is staged against this token.
	// judgeNext clears afterHeader, so the caller cannot find the marker there.
	return head, true, r.judgeNext()
}

// bodyToken returns the next token of the current document,
// and false where the document ends: at "...", at the "---" opening the next one, or at the end of the stream.
//
// The descent's run pulls from it, so it returns a token or nothing and keeps an error in err for the parse to find.
func (r *reader) bodyToken() (*group.TapeToken, bool) {
	if r.ended || r.err != nil {
		return nil, false
	}

	tk, err := r.peek()
	if err != nil {
		r.err = err

		return nil, false
	}
	if tk == nil {
		r.ended = true

		return nil, false
	}

	switch tk.Type() {
	case token.DocumentHeaderType, token.DocumentEndType:
		r.ended = true

		return nil, false
	}

	if r.tookDirective && isDirectiveToken(tk) {
		// Section 6.8 allows several directives before one "---", but a document body is one node,
		// so the second directive would reach parseDocumentBody as a second value and be rejected.
		// The next directive ends this one, not the next token of any kind:
		// a comment written under a directive belongs to it.
		r.ended = true

		return nil, false
	}

	if _, err := r.take(); err != nil {
		r.err = err

		return nil, false
	}
	r.tookDirective = isDirectiveToken(tk)

	return tk, true
}

// isDirectiveToken reports whether tk is a directive, grouped with its values or with its name alone.
func isDirectiveToken(tk *group.TapeToken) bool {
	switch tk.GroupType() {
	case group.TokenGroupDirective, group.TokenGroupDirectiveName:
		return true
	default:
		return false
	}
}

// closeDocument finishes the current document and returns the "..." that ended it, if there is one.
func (r *reader) closeDocument() (*group.TapeToken, error) {
	if r.err != nil {
		return nil, r.err
	}

	tk, err := r.peek()
	if err != nil || tk == nil || tk.Type() != token.DocumentEndType {
		return nil, err
	}

	end, err := r.take()
	if err != nil {
		return nil, err
	}
	r.afterEnd = end

	// The tape token, for the reason openDocument gives.
	return end, r.judgeNext()
}

// judgeNext checks the token after a marker against the marker's line. It reads one token ahead and takes none.
func (r *reader) judgeNext() error {
	tk, err := r.peek()
	if err != nil || tk == nil {
		r.afterHeader, r.afterEnd = nil, nil

		return err
	}

	switch {
	case r.afterHeader != nil && r.afterHeader.Line() == tk.Line():
		switch tk.GroupType() {
		case group.TokenGroupMapKey, group.TokenGroupMapKeyValue:
			return yamlerrors.NewSyntax("value cannot be placed after document separator", tk.RawToken())
		}
		if tk.Type() == token.SequenceEntryType {
			return yamlerrors.NewSyntax("value cannot be placed after document separator", tk.RawToken())
		}
	case r.afterEnd != nil && r.afterEnd.Line() == tk.Line():
		// "..." ends the document and the rest of its line: only a comment may follow it there.
		// On the next line a new document begins, and it may be a bare one.
		return yamlerrors.NewSyntax("unexpected end content", tk.RawToken())
	}
	r.afterHeader, r.afterEnd = nil, nil

	return nil
}

// fill reads tokens until the grouping emits one, or the scanner runs dry.
//
// It feeds the grouping one token at a time,
// since the grouping is a state machine and takes a token as soon as it is read.
// The scanner is therefore never read further than the descent has pulled,
// which lets a grouping stage query the scanner about the token in hand.
func (r *reader) fill() error {
	if r.at == len(r.out) {
		// Every token emitted has been taken, so the buffer restarts instead of growing with the document.
		r.out, r.at = r.out[:0], 0
	}

	for r.at >= len(r.out) {
		tk, ok := r.scan.NextToken()
		if !ok {
			if r.finished {
				break
			}
			r.drained, r.finished, r.g.Ending = true, true, true
			r.out = r.g.Finish(r.out)

			break
		}
		if r.onToken != nil {
			// Before the comment drop, so a consumer tiling the source sees every token the document wrote,
			// and before the grouping, which is where a token stops standing for itself.
			r.onToken(tk)
		}
		if tk.Type == token.CommentType && probe.Enabled {
			probe.Count("comment.scanned", 1)
			if !r.keepComments {
				probe.Count("comment.droppedBeforeGrouping", 1)
			}
		}
		if !r.keepComments && tk.Type == token.CommentType {
			continue
		}

		held, _ := r.arena.Add(group.TapeToken{})
		held.Raw(tk, r.seq)
		r.seq++

		if tk.Type == token.InvalidType {
			// An invalid token carries no reason of its own.
			// Scanner.Err names the character or the header option at fault, so return it when there is one.
			if scanErr := r.scan.Err(); scanErr != nil {
				return scanErr
			}

			return yamlerrors.NewSyntax("found an invalid token", held.RawToken())
		}

		r.out = r.g.Feed(held, r.out)
	}

	if err := r.scan.Err(); err != nil {
		return err
	}

	return r.g.Err
}
