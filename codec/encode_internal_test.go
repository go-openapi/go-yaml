// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// keyIdentityCases are keys of every kind a YAML key can be, for the kinds the
// corpus writes rarely or never as a key.
var keyIdentityCases = []string{
	"-0: a\n-0.0: b\n0x10: c\n0o17: d\n1e400: e\n-1e400: f\n.inf: g\n.NaN: h\n" +
		"123456789012345678901: i\n-9223372036854775808: j\n18446744073709551615: k\n" +
		"null: l\n~: m\ntrue: n\n\"q\\u0041\": o\n? |\n  lit\n: p\n<<: q\n",
	"!!float 1: a\n!!int 0x10: b\n!!str 1: c\n!!null : d\n!!bool true: e\n" +
		"!!binary AA==: f\n? !!binary |\n  AA\n  AA\n: g\n" +
		"!!timestamp 2001-12-14t21:59:43.10-05:00: h\n!!timestamp 2001-12-14: i\n" +
		"!!timestamp 2001-12-15T02:59:43.1+00:00: j\n",
	"%YAML 1.1\n---\n2001-12-14: a\n2001-12-15 2:59:43.10: b\n0b101: c\n017: d\n" +
		"1_000: e\n190:20:30: f\nyes: g\nOff: h\n1.0e+400: i\n",
}

// TestAKeyNodeAndItsValueHaveOneIdentity holds the two sides of a key's
// identity together.
//
// The parser compares a key by [ast.ComparedKeyName], read off the node. A
// merge, a MapSlice and the encoder compare a Go value by keyIDOf. Every key
// defect of this kind sat between two such rules, so for every scalar key the
// corpus writes, the value the key decodes to must have the identity the key's
// node has.
//
//nolint:gocognit // walks every scalar key of the corpus and compares its identity under each rule
func TestAKeyNodeAndItsValueHaveOneIdentity(t *testing.T) {
	srcs := corpusSources()
	for i, text := range keyIdentityCases {
		srcs = append(srcs, corpusSource{name: "cases/" + string(rune('0'+i)), text: text})
	}

	var compared int
	for _, src := range srcs {
		file, err := parser.ParseBytes([]byte(src.text))
		if err != nil {
			if strings.HasPrefix(src.name, "cases/") {
				t.Errorf("%s does not parse, so its keys are compared nowhere: %v", src.name, err)
			}

			continue
		}
		for _, doc := range file.Docs {
			if doc == nil || doc.Body == nil {
				continue
			}
			for _, n := range ast.Filter(ast.MappingValueType, doc.Body) {
				entry, isEntry := n.(*ast.MappingValueNode)
				if !isEntry || entry.Key == nil || entry.Key.IsMergeKey() {
					continue
				}
				name, kind, named := ast.ComparedKeyName(entry.Key, nil)
				if !named {
					continue
				}

				d := NewDecoder(bytes.NewReader(nil))
				value, err := d.mapKeyNodeToValue(entry.Key)
				if err != nil || !codecutils.HashableKey(value) {
					continue
				}
				gotKind, gotName, gotNamed := keyIDOf(value)
				if !gotNamed || gotKind != kind || gotName != name {
					t.Errorf("%s: key %q is %v %q as a node and %v %q (%v) as the value %#v",
						src.name, entry.Key.String(), kind, name, gotKind, gotName, gotNamed, value)
				}
				compared++
			}
		}
	}

	// 11,191 on 2026-09-11. A floor near it fails when the loop stops reaching
	// the corpus, which a floor of a few keys would not notice.
	t.Logf("%d keys compared", compared)
	if compared < 10000 {
		t.Errorf("only %d keys compared: the loop has stopped reaching the corpus", compared)
	}
}
