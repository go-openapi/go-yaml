// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"errors"
	"fmt"
	"strconv"
)

// PathString create Path from string
//
// YAMLPath rule
// $     : the root object/element
// .     : child operator
// ..    : recursive descent
// [num] : object/element of array by number
// [*]   : all objects/elements for array.
//
// If you want to use reserved characters such as `.` and `*` as a key name,
// enclose them in single quotation as follows ( $.foo.'bar.baz-*'.hoge ).
// If you want to use a single quote with reserved characters, escape it with `\` ( $.foo.'bar.baz\'s value'.hoge ).
func PathString(s string) (*Path, error) {
	buf := s
	length := len(buf)
	cursor := 0
	builder := &PathBuilder{}
	for cursor < length {
		c := buf[cursor]
		switch c {
		case '$':
			builder = builder.Root()
			cursor++
		case '.':
			b, buf, c, err := parsePathDot(builder, buf, cursor)
			if err != nil {
				return nil, err
			}
			length = len(buf)
			builder = b
			cursor = c
		case '[':
			b, buf, c, err := parsePathIndex(builder, buf, cursor)
			if err != nil {
				return nil, err
			}
			length = len(buf)
			builder = b
			cursor = c
		default:
			return nil, fmt.Errorf("invalid path at %d: %w", cursor, ErrInvalidPathString)
		}
	}
	return builder.Build(), nil
}

func parsePathRecursive(b *PathBuilder, buf string, cursor int) (*PathBuilder, string, int, error) {
	length := len(buf)
	cursor += 2 // skip .. characters
	start := cursor
	for ; cursor < length; cursor++ {
		c := buf[cursor]
		switch c {
		case '$':
			return nil, "", 0, fmt.Errorf("specified '$' after '..' character: %w", ErrInvalidPathString)
		case '*':
			return nil, "", 0, fmt.Errorf("specified '*' after '..' character: %w", ErrInvalidPathString)
		case '.', '[':
			goto end
		case ']':
			return nil, "", 0, fmt.Errorf("specified ']' after '..' character: %w", ErrInvalidPathString)
		}
	}
end:
	if start == cursor {
		return nil, "", 0, fmt.Errorf("not found recursive selector: %w", ErrInvalidPathString)
	}
	return b.Recursive(buf[start:cursor]), buf, cursor, nil
}

func parsePathDot(b *PathBuilder, buf string, cursor int) (*PathBuilder, string, int, error) {
	if b.root == nil || b.node == nil {
		return nil, "", 0, fmt.Errorf("required '$' character at first: %w", ErrInvalidPathString)
	}
	length := len(buf)
	if cursor+1 < length && buf[cursor+1] == '.' {
		b, buf, c, err := parsePathRecursive(b, buf, cursor)
		if err != nil {
			return nil, "", 0, err
		}
		return b, buf, c, nil
	}
	cursor++ // skip . character
	start := cursor

	// if started single quote, looking for end single quote char
	if cursor < length && buf[cursor] == '\'' {
		return parseQuotedKey(b, buf, cursor)
	}
	for ; cursor < length; cursor++ {
		c := buf[cursor]
		switch c {
		case '$':
			return nil, "", 0, fmt.Errorf("specified '$' after '.' character: %w", ErrInvalidPathString)
		case '*':
			return nil, "", 0, fmt.Errorf("specified '*' after '.' character: %w", ErrInvalidPathString)
		case '.', '[':
			goto end
		case ']':
			return nil, "", 0, fmt.Errorf("specified ']' after '.' character: %w", ErrInvalidPathString)
		}
	}
end:
	if start == cursor {
		return nil, "", 0, fmt.Errorf("could not find by empty key: %w", ErrInvalidPathString)
	}
	return b.child(buf[start:cursor]), buf, cursor, nil
}

func parseQuotedKey(b *PathBuilder, buf string, cursor int) (*PathBuilder, string, int, error) {
	if b.root == nil || b.node == nil {
		return nil, "", 0, fmt.Errorf("required '$' character at first: %w", ErrInvalidPathString)
	}

	cursor++ // skip single quote
	start := cursor
	length := len(buf)
	var foundEndDelim bool
	for ; cursor < length; cursor++ {
		switch buf[cursor] {
		case '\\':
			buf = buf[:cursor] + buf[cursor+1:]
			length = len(buf)
		case '\'':
			foundEndDelim = true
			goto end
		}
	}
end:
	if !foundEndDelim {
		return nil, "", 0, fmt.Errorf("could not find end delimiter for key: %w", ErrInvalidPathString)
	}
	if start == cursor {
		return nil, "", 0, fmt.Errorf("could not find by empty key: %w", ErrInvalidPathString)
	}
	selector := buf[start:cursor]
	cursor++
	if cursor < length {
		switch buf[cursor] {
		case '$':
			return nil, "", 0, fmt.Errorf("specified '$' after '.' character: %w", ErrInvalidPathString)
		case '*':
			return nil, "", 0, fmt.Errorf("specified '*' after '.' character: %w", ErrInvalidPathString)
		case ']':
			return nil, "", 0, fmt.Errorf("specified ']' after '.' character: %w", ErrInvalidPathString)
		}
	}
	return b.child(selector), buf, cursor, nil
}

func parsePathIndex(b *PathBuilder, buf string, cursor int) (*PathBuilder, string, int, error) {
	if b.root == nil || b.node == nil {
		return nil, "", 0, fmt.Errorf("required '$' character at first: %w", ErrInvalidPathString)
	}

	length := len(buf)
	cursor++ // skip '[' character
	if length <= cursor {
		return nil, "", 0, fmt.Errorf("unexpected end of YAML Path: %w", ErrInvalidPathString)
	}
	c := buf[cursor]
	switch c {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '*':
		start := cursor
		cursor++
		for ; cursor < length; cursor++ {
			c := buf[cursor]
			switch c {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				continue
			}
			break
		}
		if cursor >= length {
			return nil, "", 0, fmt.Errorf("unexpected end of YAML Path: %w", ErrInvalidPathString)
		}
		if buf[cursor] != ']' {
			return nil, "", 0, fmt.Errorf("invalid character %q at %d: %w", buf[cursor:cursor+1], cursor, ErrInvalidPathString)
		}
		numOrAll := buf[start:cursor]
		if numOrAll == "*" {
			return b.IndexAll(), buf, cursor + 1, nil
		}
		num, err := strconv.ParseUint(numOrAll, 10, 0)
		if err != nil {
			// The scan admits only digits here, so the one error left is an index too large for a uint.
			return nil, "", 0, fmt.Errorf("index %s at %d: %w", numOrAll, start, errors.Join(ErrInvalidPathString, err))
		}
		return b.Index(uint(num)), buf, cursor + 1, nil
	}
	return nil, "", 0, fmt.Errorf("invalid character %q at %d: %w", c, cursor, ErrInvalidPathString)
}
