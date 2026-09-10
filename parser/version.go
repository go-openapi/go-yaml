// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// YAMLVersion is a version of the YAML specification, as a "%YAML" directive
// names one and as [WithYAMLVersion] asks for one.
//
// It decides how a plain scalar resolves. 1.1 reads "0100" as 64, "1_000" as
// 1000, "1:30" as 90 and "yes" as true, where 1.2 reads 100 and the three
// strings. 1.0 and 1.3 are accepted where a document names them -- the parser
// reads a 1.x document -- and resolve as 1.1 and 1.2 respectively.
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

// schemaFor is the scalar schema a version resolves against. 1.0 predates the
// core schema and is read as 1.1, which is the closest thing it has.
// schemaInForce is the schema the document being read is resolved under: the
// version it declared with a "%YAML" line, and the option where it declared
// none.
//
// The directive wins over the option, and its scope is one document --
// endVersionScope clears yamlVersion at each document's end, which is defect
// 43's fix. Reading schemaFor(p.opts.version) instead asks for the option alone and
// misses every directive, which is what a first cut of the merge rule did.
func (p *Parser) schemaInForce() token.Schema {
	if p.yamlVersion != "" {
		return schemaFor(p.yamlVersion)
	}

	return schemaFor(p.opts.version)
}

func schemaFor(v YAMLVersion) token.Schema {
	switch v {
	case YAML10, YAML11:
		return token.Schema11
	default:
		return token.Schema12
	}
}

// endVersionScope takes the version the document just read out of scope, so the
// next one resolves against whatever the caller asked for.
//
// Setting the schema back is not enough on its own. The scanner runs ahead of
// the descent, and how far ahead depends on the marker: after "---" the next
// document's scalars are usually still uncut, while after "..." the grouping
// has read the whole of the next document to know the "..." closed anything.
// So "%YAML 1.1" over "---" over "a: yes" over "..." over "b: yes" had "yes"
// cut as a Bool before the scope ended, and the schema went back with nothing
// to apply it to.
//
// [Parser.retypeAhead] reads those tokens again, which is what it already does
// for the tokens cut before a directive is parsed. It starts one past the
// sequence it is given, so the first token the descent has not taken --
// reader.out[reader.at], the first of the next document -- is passed one lower.
func (p *Parser) endVersionScope() {
	if p.yamlVersion == "" {
		return
	}
	p.yamlVersion = ""
	schema := schemaFor(p.opts.version)
	p.scan.SetSchema(schema)

	from := int32(p.reader.seq) - 1
	if p.reader.at < len(p.reader.out) && p.reader.out[p.reader.at] != nil {
		from = p.reader.out[p.reader.at].Seq() - 1
	}
	p.retypeAhead(schema, from)
}

// retypeAhead reads the plain scalars the scan has already cut past seq again,
// against schema.
//
// A schema reaches only what the scanner cuts after it is set, and the grouping
// reads one token past the directive to know the directive's own document has
// ended. For a document whose body is a bare scalar that one token is the body:
// "%YAML 1.1" over "---" over "N" had N typed by 1.2 before the directive was
// parsed, and read "N" where the same document read false at every other
// position -- inside a collection a "-", a key or a "[" stands between the two,
// so the schema was in place by the time the scalar was cut.
//
// token.ScalarType is a pure function of the text and the schema, so what is
// already cut is read again rather than scanned again. There is one token at
// stake in practice; the grouping holds what it cannot settle yet and no more.
//
// Only a plain scalar is read this way. A quoted or folded one is a string
// whatever it spells, and the scanner gives it a type of its own, so it is not
// among the types below and keeps what it was cut as.
func (p *Parser) retypeAhead(schema token.Schema, from int32) {
	if p.tokens == nil {
		return
	}

	// content says the token about to be read is a block scalar's, because the
	// one before it was a "|" or a ">" header.
	var content bool

	for seq := int(from) + 1; seq < p.tokens.Len(); seq++ {
		tk := p.tokens.At(seq)
		if tk == nil {
			continue
		}
		raw := tk.RawToken()
		if raw == nil {
			continue
		}
		if content {
			// A block scalar is a string whatever it spells, so its content is
			// not the schema's to read. The scan cuts that content as a plain
			// String and it is the one string here a schema must not touch:
			// "%YAML 1.1" over "---" over ">-" over " null" had the content
			// retyped as a null, and parseLiteral then refused the document
			// with "unexpected token. required string token".
			content = false

			continue
		}
		if raw.Type == token.LiteralType || raw.Type == token.FoldedType {
			// Whatever follows the header is its content, which is how
			// stageBlockScalars reads it. The tape is not grouped yet here, so
			// the two are still separate tokens.
			content = true

			continue
		}
		if !resolvedByAnySchema(raw.Type) {
			continue
		}
		raw.Type = token.ScalarType(raw.Value, schema)
	}
}

// resolvedByAnySchema reports whether the scanner gave the token its type by
// reading a plain scalar against a schema, which is what makes reading it again
// against another one meaningful.
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

// resolvedBySchema reports whether the scanner typed a plain scalar by the core
// schema. A tag that resolves to nothing overrides that typing, and the scalar
// keeps the text it was written with.
func resolvedBySchema(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.BoolType, token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType,
		token.HexIntegerType, token.FloatType, token.InfinityType, token.NanType, token.NullType:
		return true
	default:
		return false
	}
}
