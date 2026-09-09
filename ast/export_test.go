// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

// RenderedFacts returns what render builds for n: the text it flattens to, and
// the three facts a parent reads off a child without flattening it.
//
// It is exported to the external test package because only that one may import
// the parser, and a parsed document is what these have to be checked against.
func (r *Renderer) RenderedFacts(n Node) (text string, spans, leads, empty bool) {
	p := r.render(n)

	return p.string(), p.spans, p.leads, p.empty()
}
