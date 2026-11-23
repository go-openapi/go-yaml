// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package selfdecode_test

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-openapi/go-yaml/codec"
)

// snippet:size
// Size is a byte count written with a unit: "512KiB", "10MiB".
type Size int64

var errSize = errors.New("invalid size")

// UnmarshalYAML receives the YAML text of the value, as the document wrote it.
func (s *Size) UnmarshalYAML(text []byte) error {
	// Decode the text as a string first: this strips the quotes of "10MiB"
	// and rejects a mapping or a sequence.
	var str string
	if err := codec.Unmarshal(text, &str); err != nil {
		return err
	}

	units := []struct {
		suffix string
		scale  int64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}}
	for _, u := range units {
		if digits, ok := strings.CutSuffix(str, u.suffix); ok {
			n, err := strconv.ParseInt(digits, 10, 64)
			if err != nil {
				return fmt.Errorf("%w: %q", errSize, str)
			}
			*s = Size(n * u.scale)

			return nil
		}
	}

	return fmt.Errorf("%w: %q has no unit", errSize, str)
}

// endsnippet:size

// snippet:limits
type Limits struct {
	Upload Size `yaml:"upload"`
	Cache  Size `yaml:"cache"`
}

// endsnippet:limits

// Example decodes two sizes, one of them quoted, through Size.UnmarshalYAML.
func Example() {
	// snippet:decode
	var l Limits
	err := codec.Unmarshal([]byte("upload: 10MiB\ncache: \"512KiB\"\n"), &l)
	fmt.Printf("%+v %v\n", l, err)
	// endsnippet:decode

	// Output:
	// {Upload:10485760 Cache:524288} <nil>
}

// Example_error decodes a size with no unit, and shows the error UnmarshalYAML returns.
func Example_error() {
	// snippet:error
	var l Limits
	err := codec.Unmarshal([]byte("upload: 10\ncache: 1KiB\n"), &l)
	fmt.Println(err)
	fmt.Println(errors.Is(err, errSize))
	fmt.Printf("%+v\n", l)
	// endsnippet:error

	// Output:
	// invalid size: "10" has no unit
	// true
	// {Upload:0 Cache:1024}
}

// snippet:hook
type Webhook struct {
	Target url.URL `yaml:"target"`
}

// endsnippet:hook

// snippet:parseURL
// parseURL decodes a url.URL from its YAML text.
func parseURL(u *url.URL, text []byte) error {
	var str string
	if err := codec.Unmarshal(text, &str); err != nil {
		return err
	}
	parsed, err := url.Parse(str)
	if err != nil {
		return err
	}
	*u = *parsed

	return nil
}

// endsnippet:parseURL

// Example_notOwned decodes a url.URL, which has no YAML method, first by reflection and then with CustomUnmarshaler.
func Example_notOwned() {
	const src = "target: https://hooks.example.com/build?token=abc\n"

	// snippet:reflect
	var w Webhook
	err := codec.Unmarshal([]byte(src), &w)
	fmt.Println(err)
	// endsnippet:reflect

	// snippet:custom
	var c Webhook
	err = codec.Unmarshal([]byte(src), &c, codec.CustomUnmarshaler(parseURL))
	fmt.Println(c.Target.Host, c.Target.Query().Get("token"), err)
	// endsnippet:custom

	// Output:
	// [1:9] string was used where mapping is expected
	// >  1 | target: https://hooks.example.com/build?token=abc
	//                ^
	//
	// hooks.example.com abc <nil>
}

// Example_register registers a decoder for mail.Address for every decode in the process.
func Example_register() {
	// snippet:register
	codec.RegisterCustomUnmarshaler(func(a *mail.Address, text []byte) error {
		var str string
		if err := codec.Unmarshal(text, &str); err != nil {
			return err
		}
		parsed, err := mail.ParseAddress(str)
		if err != nil {
			return err
		}
		*a = *parsed

		return nil
	})

	var team struct {
		Owner mail.Address `yaml:"owner"`
	}
	err := codec.Unmarshal([]byte("owner: Ada Lovelace <ada@example.com>\n"), &team)
	fmt.Printf("%q %q %v\n", team.Owner.Name, team.Owner.Address, err)
	// endsnippet:register

	// Output:
	// "Ada Lovelace" "ada@example.com" <nil>
}
