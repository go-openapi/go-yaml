// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

// YAMLVersion is a version of the YAML specification, as named by a "%YAML" directive or by [WithYAMLVersion].
//
// The version decides how a plain scalar resolves.
//
// 1.1 reads "0100" as 64, "1_000" as 1000, "1:30" as 90 and "yes" as true, where 1.2 reads 100 and three strings.
// A document may also name 1.0 or 1.3: 1.0 resolves as 1.1, and 1.3 as 1.2.
type YAMLVersion string

const (
	YAML10 YAMLVersion = "1.0"
	YAML11 YAMLVersion = "1.1"
	YAML12 YAMLVersion = "1.2"
	YAML13 YAMLVersion = "1.3"
)

var yamlVersionMap = map[string]YAMLVersion{
	"1.0": YAML10,
	"1.1": YAML11,
	"1.2": YAML12,
	"1.3": YAML13,
}

// Schema returns the YAML schema to resolve plain scalars.
//
// 1.0 and 1.1 resolve against [token.Schema11].
//
// 1.2, 1.3, the zero value and any other value resolve against [token.Schema12].
//
// Use it to scan a document alongside a parse with the same reading of plain scalars.
func (v YAMLVersion) Schema() token.Schema {
	switch v {
	case YAML10, YAML11:
		return token.Schema11
	default:
		return token.Schema12
	}
}

// schemaInForce returns the schema of the document being read:
// the version its "%YAML" line declares, or the option where it declares none.
//
// A directive applies to one document, and endVersionScope clears yamlVersion at each document's end.
// Call schemaInForce instead of opts.version.Schema(), which ignores the directive.
func (p *Parser) schemaInForce() token.Schema {
	if p.yamlVersion != "" {
		return p.yamlVersion.Schema()
	}

	return p.opts.version.Schema()
}

// endVersionScope takes the declared version out of scope, so the next document resolves under the option.
//
// Resetting the scanner's schema is not enough, because the scanner runs ahead of the descent.
// After a "..." marker the grouping has already read the next document, and its plain scalars are cut:
// in "%YAML 1.1" over "---" over "a: yes" over "..." over "b: yes", the second "yes" is cut as a bool.
// retypeAhead types them again under the option's schema.
//
// retypeAhead starts one past the sequence it is given,
// so the first token the descent has not taken, reader.out[reader.at], is passed one lower.
func (p *Parser) endVersionScope() {
	if p.yamlVersion == "" {
		return
	}
	p.yamlVersion = ""
	schema := p.opts.version.Schema()
	p.scan.SetSchema(schema)

	from := p.reader.seq - 1
	if p.reader.at < len(p.reader.out) && p.reader.out[p.reader.at] != nil {
		from = p.reader.out[p.reader.at].Seq() - 1
	}
	p.retypeAhead(schema, from)
}

// retypeAhead types again, under schema, the plain scalars the scanner has already cut after from.
//
// A new schema applies only to what the scanner cuts after it is set,
// and the grouping reads one token past a directive to find where the directive's document ends.
// When the document's body is a bare scalar, that token is the body:
// in "%YAML 1.1" over "---" over "N", N is cut under 1.2 before the directive is parsed.
//
// token.ScalarType depends only on the text and the schema, so the cut tokens are retyped and not scanned again.
//
// A quoted or folded scalar is a string whatever it spells.
// The scanner gives it a type of its own, which resolvedByAnySchema excludes, so it keeps its type.
func (p *Parser) retypeAhead(schema token.Schema, from int) {
	if p.tokens == nil {
		return
	}

	// content marks the next token as a block scalar's content, because the token before it was a "|" or ">" header.
	var content bool

	for seq := from + 1; seq < p.tokens.Len(); seq++ {
		tk := p.tokens.At(seq)
		if tk == nil {
			continue
		}
		raw := tk.RawToken()
		if raw == nil {
			continue
		}
		if content {
			// A block scalar's content is a string whatever it spells, but the scanner cuts it as a plain String.
			// Retyped, " null" under ">-" would become a null, and parseLiteral would reject the document.
			content = false

			continue
		}
		if raw.Type == token.LiteralType || raw.Type == token.FoldedType {
			// The token after the header is its content, as stageBlockScalars reads it.
			// The tape is not grouped yet, so the header and its content are still two tokens.
			content = true

			continue
		}
		if !resolvedByAnySchema(raw.Type) {
			continue
		}
		raw.Type = token.ScalarType(raw.Value, schema)
	}
}

// resolvedByAnySchema reports whether the scanner gives a plain scalar type t by reading it against a schema.
// Only such a token can change type when retyped under another schema.
func resolvedByAnySchema(t token.Type) bool {
	switch t {
	case token.StringType, token.BoolType, token.IntegerType, token.BinaryIntegerType,
		token.OctetIntegerType, token.HexIntegerType, token.FloatType,
		token.InfinityType, token.NanType, token.NullType:
		return true
	default:
		return false
	}
}

// resolvedBySchema reports whether the scanner typed tk as a bool, a number or a null by reading it against a schema.
// A tag that resolves to nothing overrides that type, and the scalar keeps the text it was written with.
func resolvedBySchema(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.BoolType, token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType,
		token.HexIntegerType, token.FloatType, token.InfinityType, token.NanType, token.NullType:
		return true
	default:
		return false
	}
}
