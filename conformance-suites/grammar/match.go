// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import "unicode/utf8"

// invoke runs one rule through the memo table.
//
// Memoizing at the rule boundary is what keeps a PEG over YAML from going
// exponential: the same rule is asked about the same position from many
// different branches. The key has to carry the env, because a parameterized PEG
// genuinely answers differently under a different indentation or context, and
// that costs hit rate, which the throughput test reports rather than hides.
//
// The env it returns is the callee's. Whether any of it reaches the caller is
// the call site's business, not the rule's: see compiler.call.
func invoke(s *slot, st *state, e env) (env, bool) {
	st.steps++

	// The one funnel every production entry passes through, which is why the
	// coverage hook is here and nowhere else. A nil vector is how it is turned
	// off, so the cost when nothing is measuring is one comparison.
	if st.cover != nil {
		st.cover.attempt(s.id, e)
	}

	// A nil table is how memoization is turned off, to measure what it is
	// worth rather than assume it.
	memoizing := st.memo != nil
	start := st.pos

	if memoizing {
		for _, got := range st.memo[start] {
			if !got.matches(s.id, e, st.limit) {
				continue
			}
			st.hits++
			if got.ok {
				st.pos = int(got.end)
				e.m = int(got.outM)
				e.t = got.outT
			}

			if st.cover != nil && got.ok {
				st.cover.succeed(s.id, e)
			}

			return e, got.ok
		}
	}

	out, ok := s.fn(st, e)
	if !ok {
		st.pos = start
		out = e
	}

	if st.cover != nil && ok {
		st.cover.succeed(s.id, e)
	}

	if memoizing {
		st.memo[start] = append(st.memo[start], memoEntry{
			rule:  s.id,
			n:     int32(e.n),
			m:     int32(e.m),
			limit: int32(st.limit),
			c:     e.c,
			t:     e.t,
			end:   int32(st.pos),
			ok:    ok,
			outM:  int32(out.m),
			outT:  out.t,
		})
	}

	return out, ok
}

func choice(alts []expr) expr {
	return func(s *state, e env) (env, bool) {
		start := s.pos
		for _, alt := range alts {
			if out, ok := alt(s, e); ok {
				return out, true
			}
			s.pos = start
		}

		return e, false
	}
}

// sequence runs its steps left to right, threading the env through them.
//
// The threading is what makes (set) mean anything: c-b-block-header reports the
// header's m and t, and the step after it is what reads them.
func sequence(steps []expr) expr {
	return func(s *state, e env) (env, bool) {
		start := s.pos
		// (max) and (exclude) set a limit that holds for the rest of the
		// enclosing sequence and no further, which is where the spec's "in the
		// next 1024 characters" and "excluding c-forbidden content" both land.
		limit := s.limit

		out := e
		for _, step := range steps {
			next, ok := step(s, out)
			if !ok {
				s.pos = start
				s.limit = limit

				return e, false
			}
			out = next
		}
		s.limit = limit

		return out, true
	}
}

// repeat runs inner between min and max times, where max < 0 means unbounded.
//
// The guard on a non-advancing match matters more than it looks: several of the
// grammar's rules can match empty, and (***) over one of them would otherwise
// never terminate.
func repeat(inner expr, minCount, maxCount int) expr {
	return func(s *state, e env) (env, bool) {
		start := s.pos
		count := 0
		out := e

		for maxCount < 0 || count < maxCount {
			before := s.pos
			next, ok := inner(s, out)
			if !ok {
				s.pos = before

				break
			}
			out = next
			count++
			if s.pos == before {
				break
			}
		}

		if count < minCount {
			s.pos = start

			return e, false
		}

		return out, true
	}
}

// lookahead reports on inner without keeping anything it did, which includes
// anything it set: an assertion about what comes next says nothing about the
// header we are under.
func lookahead(inner expr, want bool) expr {
	return func(s *state, e env) (env, bool) {
		start := s.pos
		_, got := inner(s, e)
		s.pos = start

		return e, got == want
	}
}

// lookbehind runs inner against the character before the current position.
//
// Only ns-plain-char uses it, to say that a '#' is part of a plain scalar when
// something non-space precedes it.
func lookbehind(inner expr) expr {
	return func(s *state, e env) (env, bool) {
		if s.pos == 0 {
			return e, false
		}

		r, size := utf8.DecodeLastRune(s.src[:s.pos])
		if r == utf8.RuneError && size <= 1 {
			return e, false
		}

		end := s.pos
		s.pos -= size
		_, ok := inner(s, e)
		ok = ok && s.pos == end
		s.pos = end

		return e, ok
	}
}

// capture marks where a match began so that (match) can report the text it
// covered. The two indentation comparison rules are its only users.
func capture(inner expr) expr {
	return func(s *state, e env) (env, bool) {
		mark := s.mark
		s.mark = s.pos
		out, ok := inner(s, e)
		s.mark = mark

		return out, ok
	}
}

// limitTo implements (max): the enclosing sequence must complete within n
// characters of here. It matches nothing itself.
func limitTo(n int) expr {
	return func(s *state, e env) (env, bool) {
		s.limit = tightenLimit(s.limit, s.pos+n)

		return e, true
	}
}

// tightenLimit narrows a limit rather than replacing it. A limit inside a limit
// is the tighter of the two: an implicit key nested in a bare document does not
// get to read past the document's end because its own budget is larger.
func tightenLimit(current, want int) int {
	if current >= 0 && current < want {
		return current
	}

	return want
}

func literalRune(want rune) expr {
	return func(s *state, e env) (env, bool) {
		r, size := s.rune()
		if size == 0 || r != want {
			return e, false
		}
		s.pos += size

		return e, true
	}
}

func literalString(want string) expr {
	if utf8.RuneCountInString(want) == 1 {
		r, _ := utf8.DecodeRuneInString(want)

		return literalRune(r)
	}

	n := len(want)

	return func(s *state, e env) (env, bool) {
		if s.limit >= 0 && s.pos+n > s.limit {
			return e, false
		}
		if s.pos+n > len(s.src) || string(s.src[s.pos:s.pos+n]) != want {
			return e, false
		}
		s.pos += n

		return e, true
	}
}

func (c *compiler) charRange(pair []any) expr {
	if len(pair) != 2 {
		panic("grammar: a code point range takes exactly two bounds")
	}

	lo := decodeHex(asType[string](pair[0]))
	hi := decodeHex(asType[string](pair[1]))

	return func(s *state, e env) (env, bool) {
		r, size := s.rune()
		if size == 0 || r < lo || r > hi {
			return e, false
		}
		s.pos += size

		return e, true
	}
}

// rune decodes the character at the current position, reporting a zero size at
// end of input or past the (max) limit.
func (s *state) rune() (rune, int) {
	if s.pos >= len(s.src) {
		return 0, 0
	}
	if s.limit >= 0 && s.pos >= s.limit {
		return 0, 0
	}

	r, size := utf8.DecodeRune(s.src[s.pos:])
	if r == utf8.RuneError && size <= 1 {
		return 0, 0
	}

	return r, size
}

// byteOrderMark is c-byte-order-mark encoded, which is how it appears in the
// source rather than as the code point the grammar names.
const byteOrderMark = "\xef\xbb\xbf"

// startOfLine reports whether the position begins a line.
//
// A byte order mark does not put anything on the line it precedes.
// l-document-prefix admits one in front of every document, and nb-char excludes
// it from content, so the only place it can appear is exactly the place where
// treating it as content is wrong.
//
// Counting it made the position after it look mid-line, which was enough to
// refuse a marked "a: 1" and a marked "# c". Both s-l-comments and
// s-separate-in-line ask this question, and every block collection and every
// comment goes through one of them. A marked "---" survived only because
// c-directives-end is a literal that asks nobody.
func startOfLine(s *state, e env) (env, bool) {
	pos := s.pos

	// A prefix per document, and l-yaml-stream repeats the prefix, so more than
	// one mark can stand here.
	for pos >= len(byteOrderMark) && string(s.src[pos-len(byteOrderMark):pos]) == byteOrderMark {
		pos -= len(byteOrderMark)
	}

	if pos == 0 {
		return e, true
	}

	prev := s.src[pos-1]

	return e, prev == '\n' || prev == '\r'
}

func endOfStream(s *state, e env) (env, bool) { return e, s.pos >= len(s.src) }

func matchEmpty(_ *state, e env) (env, bool) { return e, true }
