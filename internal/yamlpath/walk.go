// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"reflect"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// readByWalking reads the first node of src that p addresses, without building the document's tree.
//
// It answers as FilterFile does on the parsed file, refusals included, and holds only the nodes it returns:
// every branch off the path is skipped, and a match is kept whole with [parser.KeepNode] and cloned.
func (p *Path) readByWalking(src []byte) (ast.Node, error) {
	w := &pathWalker{steps: p.steps()}
	if _, err := parser.New().Walk(src, w); err != nil {
		return nil, err
	}
	if w.found == nil {
		return nil, p.errNotFound()
	}

	return w.found, nil
}

// steps lists the path's steps after the root, in order.
func (p *Path) steps() []pathNode {
	var steps []pathNode
	root, _ := p.node.(*rootNode)
	if root == nil {
		return nil
	}
	for step := root.child; step != nil; step = childOf(step) {
		steps = append(steps, step)
	}

	return steps
}

func childOf(step pathNode) pathNode {
	switch s := step.(type) {
	case *selectorNode:
		return s.child
	case *indexNode:
		return s.child
	case *indexAllNode:
		return s.child
	default:
		// The tree filter reads no step after "..name", so neither does the walk.
		return nil
	}
}

type walkFrameKind uint8

const (
	// frameKeep is a node kept whole, which is the path's answer or, under a recursive step, one of its matches.
	frameKeep walkFrameKind = iota
	// frameSelect is a mapping a ".name" step reads.
	frameSelect
	// frameIndex is a sequence a "[k]" step reads.
	frameIndex
	// frameAll is a sequence a "[*]" step reads, with a step after it.
	frameAll
	// frameRecursive is a mapping or a sequence a "..name" step reads.
	frameRecursive
)

// walkFrame is a node the walker has open: every node it did not skip has one, and its Leave closes it.
type walkFrame struct {
	kind walkFrameKind
	// step indexes the step this node is read by.
	step int

	// result is the answer the node's matched child gave, for a select or an index frame, and delivered
	// records that it has gone up already: the rest of the node cannot change it.
	result    ast.Node
	delivered bool

	// pending records that the key just handed over matched, so the next value is the one to read.
	// done records that a ".name" step matched once: the tree filter reads the first match only.
	pending bool
	done    bool

	// count counts a sequence's entries, and found records that "[k]" reached entry k.
	count int
	found bool

	// results holds what each entry of a "[*]" sequence answered, the nil answers left out.
	results []ast.Node

	// rec is the recursive step's collection, shared by every frame below the node it started on.
	// root marks that node, whose Leave hands the collection over.
	rec     *recursiveMatch
	root    bool
	mapping bool
}

// recursiveMatch collects what a "..name" step finds, as recursiveNode.filterNode does on a tree.
//
// start is a copy of the token the step started on, since the walk reuses the token once its node closes.
// For a mapping or a sequence it is taken at the node's Leave.
type recursiveMatch struct {
	step   *recursiveNode
	start  *token.Token
	values []ast.Node
}

// pathWalker is the [parser.Visitor] that reads a path.
type pathWalker struct {
	steps  []pathNode
	frames []walkFrame
	found  ast.Node
}

func (w *pathWalker) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsRoot() {
		// A document starts, and the last one answered nil: the walk stops at the first answer.
		w.frames = w.frames[:0]
		if _, isDirective := node.(*ast.DirectiveNode); isDirective {
			return parser.SkipNode
		}

		return w.read(node, 0)
	}

	top := &w.frames[len(w.frames)-1]
	switch top.kind {
	case frameSelect:
		return w.enterSelected(top, node, at)
	case frameIndex:
		entry := top.count
		top.count++
		if uint(entry) != w.steps[top.step].(*indexNode).selector {
			return parser.SkipNode
		}
		top.found = true

		return w.read(node, top.step+1)
	case frameAll:
		return w.read(node, top.step+1)
	case frameRecursive:
		return w.enterRecursive(top, node, at)
	default:
		// A kept node hands nothing over.
		return nil
	}
}

// enterSelected reads an entry of a mapping a ".name" step stands on.
func (w *pathWalker) enterSelected(top *walkFrame, node ast.Node, at parser.Cursor) error {
	if top.done {
		return parser.SkipNode
	}
	if at.IsKey() {
		key, err := selectorKey(node.GetToken().Value)
		if err != nil {
			return err
		}
		top.pending = key == w.steps[top.step].(*selectorNode).name()

		return parser.SkipNode
	}
	if !top.pending {
		return parser.SkipNode
	}
	top.pending, top.done = false, true

	return w.read(node, top.step+1)
}

// enterRecursive reads a node below the one a "..name" step started on.
//
// The tree filter looks through mappings and sequences only, and a value under a matching key is taken whole:
// the walk keeps it, and its own matches are read from the copy when it closes.
func (w *pathWalker) enterRecursive(top *walkFrame, node ast.Node, at parser.Cursor) error {
	if top.mapping && at.IsKey() {
		// recursiveNode compares the key's token as it is, quotes and all.
		top.pending = node.GetToken().Value == top.rec.step.selector

		return parser.SkipNode
	}
	if top.pending {
		top.pending = false
		w.frames = append(w.frames, walkFrame{kind: frameKeep, rec: top.rec})

		return parser.KeepNode
	}

	return w.descend(node, top.rec, false)
}

// descend opens a mapping or a sequence for a recursive step, and skips anything else.
func (w *pathWalker) descend(node ast.Node, rec *recursiveMatch, root bool) error {
	switch node.(type) {
	case *ast.MappingNode:
		w.frames = append(w.frames, walkFrame{kind: frameRecursive, rec: rec, root: root, mapping: true})
	case *ast.SequenceNode:
		w.frames = append(w.frames, walkFrame{kind: frameRecursive, rec: rec, root: root})
	default:
		if root {
			// A recursive step on anything else finds nothing, and answers an empty sequence all the same.
			if err := w.deliver(rec.answer()); err != nil {
				return err
			}
		}

		return parser.SkipNode
	}

	return nil
}

// read opens node as what step reads, or keeps it whole where no step is left.
func (w *pathWalker) read(node ast.Node, step int) error {
	if step == len(w.steps) {
		w.frames = append(w.frames, walkFrame{kind: frameKeep})

		return parser.KeepNode
	}

	switch s := w.steps[step].(type) {
	case *selectorNode:
		if node.Type() != ast.MappingType {
			return errNotAMapping(node.Type())
		}
		w.frames = append(w.frames, walkFrame{kind: frameSelect, step: step})
	case *indexNode:
		if node.Type() != ast.SequenceType {
			return errNotASequence(node.Type())
		}
		w.frames = append(w.frames, walkFrame{kind: frameIndex, step: step})
	case *indexAllNode:
		if node.Type() != ast.SequenceType {
			return errNotASequence(node.Type())
		}
		if step+1 == len(w.steps) {
			// "[*]" last answers the sequence itself.
			w.frames = append(w.frames, walkFrame{kind: frameKeep})

			return parser.KeepNode
		}
		w.frames = append(w.frames, walkFrame{kind: frameAll, step: step, results: []ast.Node{}})
	case *recursiveNode:
		return w.descend(node, &recursiveMatch{step: s, start: node.GetToken().Clone()}, true)
	}

	return nil
}

func (w *pathWalker) Leave(node ast.Node, _ parser.Closing) error {
	top := w.frames[len(w.frames)-1]
	w.frames = w.frames[:len(w.frames)-1]

	switch top.kind {
	case frameKeep:
		kept := cloneWithPaths(node)
		if top.rec != nil {
			top.rec.values = append(top.rec.values, kept)
			below, err := top.rec.step.filterNode(kept)
			if err != nil {
				return err
			}
			top.rec.values = append(top.rec.values, below.Values...)

			return nil
		}

		return w.deliver(kept)
	case frameSelect:
		if top.delivered {
			return nil
		}

		return w.deliver(nil)
	case frameIndex:
		if !top.found {
			return errOutOfRange(w.steps[top.step].(*indexNode).selector, top.count)
		}

		return nil
	case frameAll:
		out, _ := cloneWithPaths(node).(*ast.SequenceNode)
		out.Values = top.results

		return w.deliver(out)
	default:
		if top.root {
			// A block collection's token moves between Enter and Leave, and the tree filter reads the last one.
			top.rec.start = node.GetToken().Clone()

			return w.deliver(top.rec.answer())
		}

		return nil
	}
}

// deliver hands what a node answered to the node that read it, or ends the walk with the first answer.
func (w *pathWalker) deliver(answer ast.Node) error {
	return w.deliverTo(len(w.frames)-1, answer)
}

// deliverTo hands answer to the frame at index at, and on up where that settles the frame's own answer.
//
// A ".name" answers with its first match and a "[k]" with entry k, so neither waits for the rest of its node:
// the answer goes up at once, and a walk whose first document answers stops there.
func (w *pathWalker) deliverTo(at int, answer ast.Node) error {
	if at < 0 {
		if answer == nil {
			return nil
		}
		w.found = answer

		return parser.StopWalk
	}

	parent := &w.frames[at]
	if parent.kind == frameAll {
		if answer != nil {
			parent.results = append(parent.results, answer)
		}

		return nil
	}
	parent.result, parent.delivered = answer, true

	return w.deliverTo(at-1, answer)
}

// answer is the sequence a recursive step returns, as recursiveNode.filter builds it.
func (r *recursiveMatch) answer() ast.Node {
	return &ast.SequenceNode{
		BaseNode: ast.BaseNode{},
		Start:    r.start,
		Values:   r.values,
	}
}

// cloneWithPaths copies node with [ast.Clone], and gives every node of the copy the path its original had.
//
// Clone drops the path, since a copy is usually put somewhere else. The tree filter returns the parsed node
// itself, path and all, so the walk's copy carries the paths too. The copy has the original's shape, so
// [ast.Walk] reaches its nodes in the same order.
func cloneWithPaths(node ast.Node) ast.Node {
	cloned := ast.Clone(node)

	var paths []*ast.PathNode
	ast.Walk(nodeLister(func(n ast.Node) {
		paths = append(paths, n.GetPathNode())
	}), node)

	next := 0
	ast.Walk(nodeLister(func(n ast.Node) {
		if next < len(paths) {
			n.SetPathNode(paths[next])
		}
		next++
	}), cloned)

	return cloned
}

// nodeLister calls itself on every node [ast.Walk] reaches, and skips the typed nil nodes a tree holds.
type nodeLister func(ast.Node)

func (f nodeLister) Visit(n ast.Node) ast.Visitor {
	if n != nil && !reflect.ValueOf(n).IsNil() {
		f(n)
	}

	return f
}
