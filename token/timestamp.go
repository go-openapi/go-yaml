// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "time"

// ParseTimestamp reads text as a timestamp of the YAML 1.1 type tag:yaml.org,2002:timestamp.
//
// It accepts what the regular expression at https://yaml.org/type/timestamp.html accepts, and nothing else:
//
//	[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]                    (a date)
//	[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?                  (a date and a time)
//	([Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](\.[0-9]*)?
//	([ \t]*(Z|[-+][0-9][0-9]?(:[0-9][0-9])?))?
//
// So a date alone takes two-digit month and day fields, and a date with a time may take one-digit ones.
// The zone may be written "-5", "-05" or "-05:00", and any run of spaces or tabs may stand before it.
//
// The fraction of a second may be a dot with no digit after it, as in "21:59:43.Z".
// ParseTimestamp keeps the first nine digits of a fraction and drops the rest.
//
// Each field must also be in range: a month from 1 to 12, a day the month has, an hour below 24,
// and a minute or second below 60.
// A timestamp with no zone, or with "Z", is in UTC. One with an offset is in a [time.FixedZone] of that offset,
// whatever the local zone.
//
// Under YAML 1.1, the parser resolves a plain scalar ParseTimestamp accepts to a timestamp.
// The decoder reads a "!!timestamp" scalar or a time.Time destination with ParseTimestamp.
// [IsNeedQuoted] quotes a string ParseTimestamp accepts, so the string reads back as a string.
func ParseTimestamp(text string) (time.Time, bool) {
	ts, ok := scanTimestamp(text)
	if !ok {
		return time.Time{}, false
	}

	loc := time.UTC
	if ts.hasOffset {
		loc = time.FixedZone("", ts.offset)
	}

	return time.Date(ts.year, time.Month(ts.month), ts.day, ts.hour, ts.minute, ts.second, ts.nanosecond, loc), true
}

// timestampFields holds the fields [scanTimestamp] reads.
type timestampFields struct {
	year, month, day     int
	hour, minute, second int
	nanosecond           int
	offset               int // seconds east of UTC
	hasOffset            bool
}

// scanTimestamp reads the fields of text, as [ParseTimestamp] describes. It allocates nothing.
func scanTimestamp(text string) (timestampFields, bool) {
	var ts timestampFields
	c := timestampCursor{text: text}

	year, yearDigits := c.number(4)
	if yearDigits != 4 || !c.skip('-') {
		return ts, false
	}
	month, monthDigits := c.number(2)
	if monthDigits == 0 || !c.skip('-') {
		return ts, false
	}
	day, dayDigits := c.number(2)
	if dayDigits == 0 {
		return ts, false
	}
	ts.year, ts.month, ts.day = year, month, day

	if c.done() {
		// A date alone spells its month and day with two digits each.
		return ts, monthDigits == 2 && dayDigits == 2 && ts.validDate()
	}

	if !c.skip('T') && !c.skip('t') && c.blanks() == 0 {
		return ts, false
	}

	hour, hourDigits := c.number(2)
	if hourDigits == 0 || !c.skip(':') {
		return ts, false
	}
	minute, minuteDigits := c.number(2)
	if minuteDigits != 2 || !c.skip(':') {
		return ts, false
	}
	second, secondDigits := c.number(2)
	if secondDigits != 2 {
		return ts, false
	}
	ts.hour, ts.minute, ts.second = hour, minute, second

	if c.skip('.') {
		ts.nanosecond = c.fraction()
	}

	if c.blanks() > 0 && c.done() {
		// The blanks belong to the zone, which must follow them.
		return ts, false
	}
	switch {
	case c.skip('Z'):
	case c.peek('+'), c.peek('-'):
		sign := 1
		if c.skip('-') {
			sign = -1
		} else {
			c.skip('+')
		}
		zoneHour, zoneHourDigits := c.number(2)
		if zoneHourDigits == 0 || zoneHour > 23 {
			return ts, false
		}
		zoneMinute := 0
		if c.skip(':') {
			var zoneMinuteDigits int
			zoneMinute, zoneMinuteDigits = c.number(2)
			if zoneMinuteDigits != 2 || zoneMinute > 59 {
				return ts, false
			}
		}
		ts.offset = sign * (zoneHour*3600 + zoneMinute*60)
		ts.hasOffset = true
	}

	return ts, c.done() && ts.validDate() && ts.validTime()
}

func (ts timestampFields) validDate() bool {
	return ts.month >= 1 && ts.month <= 12 && ts.day >= 1 && ts.day <= daysIn(ts.month, ts.year)
}

func (ts timestampFields) validTime() bool {
	return ts.hour < 24 && ts.minute < 60 && ts.second < 60
}

// daysIn returns the number of days of month in year. Day 0 of the next month normalizes to the last day of month.
func daysIn(month, year int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// timestampCursor reads text from left to right for [scanTimestamp].
type timestampCursor struct {
	text string
	at   int
}

func (c *timestampCursor) done() bool { return c.at == len(c.text) }

func (c *timestampCursor) peek(b byte) bool { return c.at < len(c.text) && c.text[c.at] == b }

// skip moves past b and returns true when b comes next.
func (c *timestampCursor) skip(b byte) bool {
	if !c.peek(b) {
		return false
	}
	c.at++

	return true
}

// blanks moves past a run of spaces and tabs and returns its length.
func (c *timestampCursor) blanks() int {
	start := c.at
	for c.peek(' ') || c.peek('\t') {
		c.at++
	}

	return c.at - start
}

// number reads up to limit decimal digits and returns their value and how many it read.
func (c *timestampCursor) number(limit int) (value, digits int) {
	for digits < limit && c.at < len(c.text) && isDecimalDigit(c.text[c.at]) {
		value = value*10 + int(c.text[c.at]-'0')
		c.at++
		digits++
	}

	return value, digits
}

// fraction reads a run of decimal digits as a fraction of a second, and returns it in nanoseconds.
// Digits past the ninth are read and dropped.
func (c *timestampCursor) fraction() int {
	const nanoDigits = 9

	nanos, digits := 0, 0
	for c.at < len(c.text) && isDecimalDigit(c.text[c.at]) {
		if digits < nanoDigits {
			nanos = nanos*10 + int(c.text[c.at]-'0')
			digits++
		}
		c.at++
	}
	for ; digits < nanoDigits; digits++ {
		nanos *= 10
	}

	return nanos
}

func isDecimalDigit(b byte) bool { return b >= '0' && b <= '9' }
