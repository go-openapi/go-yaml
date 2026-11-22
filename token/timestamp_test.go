// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestParseTimestamp checks ParseTimestamp against the expression at https://yaml.org/type/timestamp.html.
func TestParseTimestamp(t *testing.T) {
	for tc := range parseTimestampCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := token.ParseTimestamp(tc.text)
			if tc.want == "" {
				assert.Falsef(t, ok, "%q read as %v", tc.text, got)

				return
			}
			require.Truef(t, ok, "%q is not read as a timestamp", tc.text)
			assert.Equal(t, tc.want, got.Format(time.RFC3339Nano))
		})
	}
}

// TestATimestampIsQuoted checks the encoder quotes every string ParseTimestamp reads, and leaves the rest plain.
//
// A string written plain that a YAML 1.1 reader resolves to a timestamp reads back as a time.Time.
func TestATimestampIsQuoted(t *testing.T) {
	for tc := range parseTimestampCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.text == "" || !startsPlain(tc.text) {
				return
			}
			_, isTimestamp := token.ParseTimestamp(tc.text)
			assert.Equal(t, isTimestamp, token.IsNeedQuoted(tc.text))
		})
	}
}

// TestAClockTimeIsQuoted checks the encoder quotes "12:30", which YAML 1.1 reads as a base 60 integer.
func TestAClockTimeIsQuoted(t *testing.T) {
	for _, text := range []string{"12:30", "1:2", "23:59"} {
		assert.Truef(t, token.IsNeedQuoted(text), "%q", text)
	}
}

// TestParseTimestampAllocatesNothingOnAMiss checks a text that is no timestamp costs no allocation.
func TestParseTimestampAllocatesNothingOnAMiss(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = token.ParseTimestamp("2001-12-14 21:59:43.10 -5x")
		_, _ = token.ParseTimestamp("1234")
	})
	assert.Zero(t, allocs)
}

// startsPlain reports whether text opens with a character that leaves IsNeedQuoted to its timestamp check.
func startsPlain(text string) bool {
	return text[0] >= '0' && text[0] <= '9' && text[len(text)-1] != ' ' && text[len(text)-1] != '\t'
}

// ***************** timestamp test cases **************************

// parseTimestampCase holds a text and the instant ParseTimestamp reads, in RFC 3339, or "" when it reads none.
type parseTimestampCase struct {
	name string
	text string
	want string
}

func parseTimestampCases() iter.Seq[parseTimestampCase] {
	return slices.Values([]parseTimestampCase{
		// The five examples of yaml.org/type/timestamp.html.
		{"canonical", "2001-12-15T02:59:43.1Z", "2001-12-15T02:59:43.1Z"},
		{"iso8601", "2001-12-14t21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"spaced", "2001-12-14 21:59:43.10 -5", "2001-12-14T21:59:43.1-05:00"},
		{"no time zone", "2001-12-15 2:59:43.10", "2001-12-15T02:59:43.1Z"},
		{"date", "2002-12-14", "2002-12-14T00:00:00Z"},

		{"upper-case T", "2001-12-14T21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"zone of two digits", "2001-12-14 21:59:43.10 -05", "2001-12-14T21:59:43.1-05:00"},
		{"zone against the time", "2001-12-14 21:59:43.10+05:30", "2001-12-14T21:59:43.1+05:30"},
		{"Z apart from the time", "2001-12-14 21:59:43.1 Z", "2001-12-14T21:59:43.1Z"},
		{"run of blanks", "2001-12-14 \t 21:59:43\t\t-5", "2001-12-14T21:59:43-05:00"},
		{"short date with a time", "2001-1-2 3:04:05", "2001-01-02T03:04:05Z"},
		{"no fraction", "2001-12-14T21:59:43Z", "2001-12-14T21:59:43Z"},
		{"a dot and no digit", "2001-12-14T21:59:43.Z", "2001-12-14T21:59:43Z"},
		{"fraction past nanoseconds", "2001-12-14T21:59:43.1234567891Z", "2001-12-14T21:59:43.123456789Z"},
		{"leap day", "2000-02-29", "2000-02-29T00:00:00Z"},

		{"padded date", "2001-01-02", "2001-01-02T00:00:00Z"},
		{"padded fields with a time", "2015-02-03T03:04:05Z", "2015-02-03T03:04:05Z"},
		{"one-digit month, day and hour", "2015-2-3T3:04:05Z", "2015-02-03T03:04:05Z"},
		{"short date alone", "2001-1-2", ""},
		{"one-digit minute and second", "2015-2-3T3:4:5Z", ""},
		{"short year", "201-12-14", ""},
		{"long year", "20011-12-14", ""},
		{"month 13", "2001-13-01", ""},
		{"February 29 in a common year", "2001-02-29", ""},
		{"1900 is not a leap year", "1900-02-29", ""},
		{"April 31", "2001-04-31", ""},
		{"hour 24", "2001-12-14T24:00:00Z", ""},
		{"minute of one digit", "2001-12-14T21:5:43Z", ""},
		{"second 60", "2001-12-14T21:59:60Z", ""},
		{"no second", "2001-12-14T21:59Z", ""},
		{"no separator", "2001-12-1421:59:43", ""},
		{"blanks and no zone", "2001-12-14 21:59:43 ", ""},
		{"zone hour 24", "2001-12-14 21:59:43 +24", ""},
		{"zone minute of one digit", "2001-12-14 21:59:43 +5:3", ""},
		{"lower-case z", "2001-12-14T21:59:43z", ""},
		{"text after the zone", "2001-12-14T21:59:43Zx", ""},
		{"trailing dash", "2001-12-14-", ""},
		{"a word", "not a date", ""},
		{"empty", "", ""},
	})
}
