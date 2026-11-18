// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"fmt"

	"github.com/go-openapi/go-yaml"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// Example_errorSource prints an error with the lines of the document around it: the line in error is marked with ">"
// and a caret points at the column.
//
// Pass colored as true to [yamlerrors.FormatError] to print the message and the margin with ANSI colors for a
// terminal, and inclSource as false to print the position and the message alone.
func Example_errorSource() {
	src := []byte("server:\n  host: example.com\n  port: 443\n  host: example.org\n  debug: true\n")

	var v map[string]any
	if err := yaml.Unmarshal(src, &v); err != nil {
		fmt.Println(yamlerrors.FormatError(err, false, true))
	}

	// Output:
	// [4:3] mapping key "host" already defined at [2:3]
	//    1 | server:
	//    2 |   host: example.com
	//    3 |   port: 443
	// >  4 |   host: example.org
	//          ^
	//    5 |   debug: true
}
