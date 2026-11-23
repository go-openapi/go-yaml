// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"fmt"
	"strconv"

	"github.com/go-openapi/go-yaml/ast"
)

// pathNode is one step of a path. filter returns the node the step and the steps after it address, and
// replace puts target there.
type pathNode interface {
	fmt.Stringer
	chain(node pathNode) pathNode
	filter(node ast.Node) (ast.Node, error)
	replace(node ast.Node, target ast.Node) error
}

// basePathNode holds the step after this one, nil for the last.
type basePathNode struct {
	child pathNode
}

// chain sets node as the step after this one, and returns it.
func (n *basePathNode) chain(node pathNode) pathNode {
	n.child = node
	return node
}

// rootNode is "$", the first step of every path.
type rootNode struct {
	*basePathNode
}

func newRootNode() *rootNode {
	return &rootNode{basePathNode: &basePathNode{}}
}

func (n *rootNode) String() string {
	s := "$"
	if n.child != nil {
		s += n.child.String()
	}
	return s
}

func (n *rootNode) filter(node ast.Node) (ast.Node, error) {
	if n.child == nil {
		return node, nil
	}
	return n.child.filter(node)
}

func (n *rootNode) replace(node ast.Node, target ast.Node) error {
	if n.child == nil {
		return nil
	}
	return n.child.replace(node, target)
}

// selectorNode is ".name": the value of the entry called name, in a mapping.
type selectorNode struct {
	*basePathNode

	selector string
}

func newSelectorNode(selector string) *selectorNode {
	return &selectorNode{
		basePathNode: &basePathNode{},
		selector:     selector,
	}
}

func (n *selectorNode) String() string {
	var builder PathBuilder
	selector := builder.normalizeSelectorName(n.selector)
	s := "." + selector
	if n.child != nil {
		s += n.child.String()
	}
	return s
}

func (n *selectorNode) filter(node ast.Node) (ast.Node, error) {
	selector := n.name()
	switch typed := node.(type) {
	case *ast.MappingNode:
		for _, value := range typed.Values {
			key, err := selectorKey(value.Key.GetToken().Value)
			if err != nil {
				return nil, err
			}
			if key == selector {
				if n.child == nil {
					return value.Value, nil
				}
				return n.child.filter(value.Value)
			}
		}
	case *ast.MappingValueNode:
		key := typed.Key.GetToken().Value
		if key == selector {
			if n.child == nil {
				return typed.Value, nil
			}
			return n.child.filter(typed.Value)
		}
	default:
		return nil, errNotAMapping(node.Type())
	}
	return nil, nil
}

// selectorKey returns the text of a key's token without its quotes, which the selector is compared to.
func selectorKey(key string) (string, error) {
	if len(key) == 0 {
		return key, nil
	}
	switch key[0] {
	case '"':
		return strconv.Unquote(key)
	case '\'':
		if len(key) > 1 && key[len(key)-1] == '\'' {
			return key[1 : len(key)-1], nil
		}
	}

	return key, nil
}

// errNotAMapping, errNotASequence and errOutOfRange return the [ErrInvalidQuery] a step gives a node it cannot
// read. The tree filter, the replace methods and the walk share them.
func errNotAMapping(got ast.NodeType) error {
	return fmt.Errorf("expected node type is map or map value. but got %s: %w", got, ErrInvalidQuery)
}

func errNotASequence(got ast.NodeType) error {
	return fmt.Errorf("expected sequence type node. but got %s: %w", got, ErrInvalidQuery)
}

func errOutOfRange(index uint, length int) error {
	return fmt.Errorf("expected index is %d. but got sequences has %d items: %w", index, length, ErrInvalidQuery)
}

// name returns the key the selector matches, without the quotes a reserved character needs.
func (n *selectorNode) name() string {
	selector := n.selector
	if len(selector) > 1 && selector[0] == '\'' && selector[len(selector)-1] == '\'' {
		selector = selector[1 : len(selector)-1]
	}

	return selector
}

func (n *selectorNode) replaceMapValue(value *ast.MappingValueNode, target ast.Node) error {
	key := value.Key.GetToken().Value
	if key != n.selector {
		return nil
	}
	if n.child == nil {
		if err := value.Replace(target); err != nil {
			return err
		}
	} else {
		if err := n.child.replace(value.Value, target); err != nil {
			return err
		}
	}
	return nil
}

func (n *selectorNode) replace(node ast.Node, target ast.Node) error {
	switch typed := node.(type) {
	case *ast.MappingNode:
		for _, value := range typed.Values {
			if err := n.replaceMapValue(value, target); err != nil {
				return err
			}
		}
	case *ast.MappingValueNode:
		if err := n.replaceMapValue(typed, target); err != nil {
			return err
		}
	default:
		return errNotAMapping(node.Type())
	}
	return nil
}

// indexNode is "[n]": the n'th entry of a sequence, counting from 0.
type indexNode struct {
	*basePathNode

	selector uint
}

func newIndexNode(selector uint) *indexNode {
	return &indexNode{
		basePathNode: &basePathNode{},
		selector:     selector,
	}
}

func (n *indexNode) String() string {
	s := fmt.Sprintf("[%d]", n.selector)
	if n.child != nil {
		s += n.child.String()
	}
	return s
}

func (n *indexNode) filter(node ast.Node) (ast.Node, error) {
	if node.Type() != ast.SequenceType {
		return nil, errNotASequence(node.Type())
	}
	sequence, _ := node.(*ast.SequenceNode)
	if n.selector >= uint(len(sequence.Values)) {
		return nil, errOutOfRange(n.selector, len(sequence.Values))
	}
	value := sequence.Values[n.selector]
	if n.child == nil {
		return value, nil
	}
	return n.child.filter(value)
}

func (n *indexNode) replace(node ast.Node, target ast.Node) error {
	if node.Type() != ast.SequenceType {
		return errNotASequence(node.Type())
	}
	sequence, _ := node.(*ast.SequenceNode)
	if n.selector >= uint(len(sequence.Values)) {
		return errOutOfRange(n.selector, len(sequence.Values))
	}
	if n.child == nil {
		if err := sequence.Replace(int(n.selector), target); err != nil {
			return err
		}
		return nil
	}
	return n.child.replace(sequence.Values[n.selector], target)
}

// indexAllNode is "[*]": every entry of a sequence.
type indexAllNode struct {
	*basePathNode
}

func newIndexAllNode() *indexAllNode {
	return &indexAllNode{
		basePathNode: &basePathNode{},
	}
}

func (n *indexAllNode) String() string {
	s := "[*]"
	if n.child != nil {
		s += n.child.String()
	}
	return s
}

func (n *indexAllNode) filter(node ast.Node) (ast.Node, error) {
	if node.Type() != ast.SequenceType {
		return nil, errNotASequence(node.Type())
	}
	sequence, _ := node.(*ast.SequenceNode)
	if n.child == nil {
		return sequence, nil
	}
	out := *sequence
	out.Values = []ast.Node{}
	for _, value := range sequence.Values {
		filtered, err := n.child.filter(value)
		if err != nil {
			return nil, err
		}
		if filtered == nil {
			// The element does not match. It contributes nothing: keeping a hole
			// for it would put a null in the result where the query found
			// nothing at all.
			continue
		}
		out.Values = append(out.Values, filtered)
	}
	return &out, nil
}

func (n *indexAllNode) replace(node ast.Node, target ast.Node) error {
	if node.Type() != ast.SequenceType {
		return errNotASequence(node.Type())
	}
	sequence, _ := node.(*ast.SequenceNode)
	if n.child == nil {
		for idx := range sequence.Values {
			if err := sequence.Replace(idx, target); err != nil {
				return err
			}
		}
		return nil
	}
	for _, value := range sequence.Values {
		if err := n.child.replace(value, target); err != nil {
			return err
		}
	}
	return nil
}

// recursiveNode is "..name": the value of every entry called name, at any depth, gathered in a sequence.
type recursiveNode struct {
	*basePathNode

	selector string
}

func newRecursiveNode(selector string) *recursiveNode {
	return &recursiveNode{
		basePathNode: &basePathNode{},
		selector:     selector,
	}
}

func (n *recursiveNode) String() string {
	s := ".." + n.selector
	if n.child != nil {
		s += n.child.String()
	}
	return s
}

func (n *recursiveNode) filterNode(node ast.Node) (*ast.SequenceNode, error) {
	sequence := &ast.SequenceNode{BaseNode: ast.BaseNode{}}
	switch typedNode := node.(type) {
	case *ast.MappingNode:
		for _, value := range typedNode.Values {
			seq, err := n.filterNode(value)
			if err != nil {
				return nil, err
			}
			sequence.Values = append(sequence.Values, seq.Values...)
		}
	case *ast.MappingValueNode:
		key := typedNode.Key.GetToken().Value
		if n.selector == key {
			sequence.Values = append(sequence.Values, typedNode.Value)
		}
		seq, err := n.filterNode(typedNode.Value)
		if err != nil {
			return nil, err
		}
		sequence.Values = append(sequence.Values, seq.Values...)
	case *ast.SequenceNode:
		for _, value := range typedNode.Values {
			seq, err := n.filterNode(value)
			if err != nil {
				return nil, err
			}
			sequence.Values = append(sequence.Values, seq.Values...)
		}
	}
	return sequence, nil
}

func (n *recursiveNode) filter(node ast.Node) (ast.Node, error) {
	sequence, err := n.filterNode(node)
	if err != nil {
		return nil, err
	}
	sequence.Start = node.GetToken()
	return sequence, nil
}

func (n *recursiveNode) replaceNode(node ast.Node, target ast.Node) error {
	switch typedNode := node.(type) {
	case *ast.MappingNode:
		for _, value := range typedNode.Values {
			if err := n.replaceNode(value, target); err != nil {
				return err
			}
		}
	case *ast.MappingValueNode:
		key := typedNode.Key.GetToken().Value
		if n.selector == key {
			if err := typedNode.Replace(target); err != nil {
				return err
			}
		}
		if err := n.replaceNode(typedNode.Value, target); err != nil {
			return err
		}
	case *ast.SequenceNode:
		for _, value := range typedNode.Values {
			if err := n.replaceNode(value, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *recursiveNode) replace(node ast.Node, target ast.Node) error {
	return n.replaceNode(node, target)
}
