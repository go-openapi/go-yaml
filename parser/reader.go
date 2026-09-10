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

// reader turns a document into the tokens the descent walks, as the descent
// asks for them.
//
// It scans a run into the arena, groups that run, and hands the result on a
// token at a time. Nothing reads further than the descent has asked, so the
// scanner, the grouping and the parse all run at once and the tape may be
// filled again behind them.
//
// A document opens at "---" or at the first token of the stream, and closes at
// "...", at the "---" opening the next one, or at the end. The parse asks for
// those three in turn: [reader.openDocument], then [reader.bodyToken] until it
// says the document has run out, then [reader.closeDocument].
type reader struct {
	scan  *scanner.Scanner
	arena *tokenarena.TokenArena[group.TapeToken]
	g     group.Grouper

	// out holds what the grouping has handed out and the descent has not yet
	// taken, from at onward.
	out []*group.TapeToken
	at  int

	// seq is the place on the tape of the next token read.
	seq int
	// drained says the scanner has no more to give, and finished that the
	// grouping has been emptied of what it was still holding.
	drained, finished bool
	// keepComments says the parse was asked for them. The rest are dropped as they
	// arrive and never reach the grouping.
	keepComments bool

	// afterHeader and afterEnd hold the marker just read, so the token after it
	// can be held against the line the marker stands on.
	afterHeader, afterEnd *group.TapeToken
	// taken says a token was read at all, since an empty stream is one empty
	// document and a "..." closing nothing is none. ended says the document
	// being read has run out. tail says the empty document at the end of a
	// stream has been handed over.
	taken, ended, tail bool
	// tookDirective says the body just handed a directive over. A directive is
	// a document of its own, so the next one closes this document rather than
	// standing beside it.
	tookDirective bool

	// err is a refusal bodyToken could not report, since the descent's pull
	// answers with a token or nothing.
	err error
}

// newReader returns a reader over what scan hands out.
//
// estimate is how many tokens the document is guessed to hold; it sizes the
// grouping's own buffers and nothing else.
func newReader(scan *scanner.Scanner, arena *tokenarena.TokenArena[group.TapeToken], estimate int, keepComments bool) *reader {
	r := &reader{
		scan:         scan,
		arena:        arena,
		g:            group.NewGrouper(estimate),
		out:          make([]*group.TapeToken, 0, runarena.MinGroupBlock),
		keepComments: keepComments,
	}
	if keepComments {
		// Taken here rather than where the first comment arrives, so that the
		// parser may hold the same map from the start. A parse dropping
		// comments takes none.
		r.g.LineComments = make(map[*group.TapeToken]*token.Token)
	}

	return r
}

// peek returns the next grouped token without taking it, grouping another run
// where it has none in hand.
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

// openDocument begins the next document and returns the "---" that opened it,
// where there is one. ok is false at the end of the stream.
func (r *reader) openDocument() (*group.TapeToken, bool, error) {
	// A "..." standing where nothing is open closes nothing: l-yaml-stream
	// admits a run of suffixes and only the first closes anything.
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
		// The stream ends here. It holds one last document -- the empty one --
		// unless something was read from it.
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

	// The tape token and not its raw one: the "---" carries the comment closing
	// its line, staged against this token, and judgeNext below clears
	// afterHeader before the caller could look it up again.
	return head, true, r.judgeNext()
}

// bodyToken draws the next token of the document being read, and reports false
// where the document ends: at "...", at the "---" opening the next one, or at
// the end of the stream.
//
// It is what the descent's run pulls from, so it answers with a token or
// nothing and keeps a refusal in err for the parse to find.
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
		// A directive stands on its own: the body of a document is one node,
		// and 6.8 lets several directives stand before one "---". Handing them
		// over together made the second one a second value in one body, which
		// parseDocumentBody refuses as "value is not allowed in this context",
		// so "%YAML 1.2" over "%TAG !e! ..." -- the ordinary prelude -- could
		// not be read at all.
		//
		// The next directive is what ends this one, not the next token of any
		// kind: a comment written under a directive belongs to it, which is
		// what the Test Suite's spec-example-6-13-reserved-directives is.
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

// isDirectiveToken reports whether tk is a directive, grouped with its values
// or with its name alone.
func isDirectiveToken(tk *group.TapeToken) bool {
	switch tk.GroupType() {
	case group.TokenGroupDirective, group.TokenGroupDirectiveName:
		return true
	default:
		return false
	}
}

// closeDocument finishes the document being read and returns the "..." that
// ended it, where there is one.
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

// judgeNext holds the token after a marker against the line the marker stands
// on. It reads one token ahead and takes none.
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
		// "..." ends the document and takes the rest of its line: only a
		// comment may follow it there. On the next line a new document begins,
		// and it may be a bare one.
		return yamlerrors.NewSyntax("unexpected end content", tk.RawToken())
	}
	r.afterHeader, r.afterEnd = nil, nil

	return nil
}

// fill reads tokens until the grouping hands one out, or the scanner runs dry.
//
// One token at a time: the grouping is a state machine now, so a token can be
// fed the moment it is read and there is nothing to gain by reading a run of
// them first. What that used to cost was a slice of tokens waiting to be
// grouped, a second slice holding what the grouping made of the last run, and a
// copy joining the two on every fill.
//
// It also means the scanner is never read further than the descent has asked
// for, which is what lets a stage ask the scanner about the token in hand.
func (r *reader) fill() error {
	if r.at == len(r.out) {
		// Everything handed out has been taken, so the buffer starts again
		// rather than growing for the length of the document.
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
			// A token the scanner refused carries no reason of its own:
			// Scanner.Err has it, and it names the character or the header
			// option that was wrong. Reporting "found an invalid token"
			// instead loses that.
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
