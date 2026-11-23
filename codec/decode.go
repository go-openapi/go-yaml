// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/astutils"
	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
	"github.com/go-openapi/go-yaml/codec/internal/structfields"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/format"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Decoder reads the documents of a YAML stream into Go values, one document for each call to [Decoder.Decode].
type Decoder struct {
	// source holds the document being decoded. An error found in it uses source to print the lines around itself.
	source yamlerrors.Source
	// entryKey holds the key token of the mapping entry that writes the node being decoded. It is nil for a
	// sequence element. Decoding is depth first. Each step saves entryKey and puts it back, so the field behaves
	// as a stack.
	//
	// missingFieldToken reads entryKey to place a validation error about a field missing from the document,
	// because the node holds nothing to point at.
	// A sequence element has no entry here. The parser keeps the "- " entry node only under WithComments,
	// and the error must not move with that option. missingFieldToken then points at the element's first key.
	// The walk passes no ':' token over, so the error points at the key and not at the ':'.
	entryKey         *token.Token
	reader           io.Reader
	referenceReaders []io.Reader
	anchorNodeMap    map[string]ast.Node
	anchorNodeMaps   []map[string]ast.Node
	// referenceAnchorNodeMap holds the anchors published by ReferenceFiles and
	// ReferenceDirs. Every document of the input starts from these.
	// The options exist to share an anchor across files.
	// An anchor is not shared across the documents of one stream.
	referenceAnchorNodeMap map[string]ast.Node
	anchorValueMap         map[string]reflect.Value
	// openAnchors counts, by anchor name, the anchored nodes being decoded right
	// now. An alias to one of them stands inside its own anchor, and a Go
	// value has no way to hold that. openAnchors is a count instead of a set,
	// because a document may define the same anchor again inside the node it names.
	openAnchors map[string]int
	// merging is set while the decoder reads the mapping brought in by a "<<".
	// It stays set for everything decoded beneath that mapping.
	merging              bool
	customUnmarshalerMap map[reflect.Type]func(any, []byte) error
	commentMaps          []CommentMap
	toCommentMap         CommentMap
	opts                 []DecodeOption
	referenceFiles       []string
	referenceDirs        []string
	isRecursiveDir       bool
	isResolvedReference  bool
	validator            func(any) error
	disallowUnknownField bool
	allowedFieldPrefixes []string
	allowDuplicateMapKey bool
	extraParserOptions   []parser.Option
	shareAliases         bool
	useOrderedMap        bool
	useStringKeys        bool
	useJSONUnmarshaler   bool
	useJSONTags          bool
	useInferredNames     bool

	// stream walks the source into Go values, one document for each Decode,
	// without building a tree. The decoder keeps src, so a later Decode that needs
	// something the walk cannot serve can still build the tree. See canWalk.
	// walkedFirst records that Unmarshal filled its destination by the walk.
	// firstOnly marks a Decoder made by Unmarshal, which reads one document.
	stream      *walkStream
	walkedFirst bool
	firstOnly   bool
	// streamErr holds the error that stopped the stream: reading the source,
	// resolving its references, or parsing it into a tree. Decode returns it
	// from then on, as encoding/json.Decoder does. The walk does the same for a
	// parse error it found itself. Before streamErr was kept, the next Decode read the
	// drained reader as an empty source and returned io.EOF.
	// A Decode loop takes io.EOF for the clean end of the stream.
	streamErr error
	// pendingErr holds the parse error of the document after the last one in the tree.
	// The parse keeps the documents before a broken one, so those decode.
	// The Decode that reaches the broken document returns pendingErr and makes it the streamErr.
	// Unmarshal reads one document and returns pendingErr after that document.
	pendingErr error
	// strs holds the text of every string returned by a decode. The text is copied out of
	// the document, so the values outlive it.
	strs codecutils.Arena
	// built counts the nodes this decode has made, and budget bounds it. See
	// codecutils.AliasBudget. expanding holds the outermost alias being read through,
	// and expanded the last one. The over-budget error names one of them.
	built       int
	budget      int
	expanding   *ast.AliasNode
	expanded    *ast.AliasNode
	src         []byte
	parsedFile  *ast.File
	streamIndex int
	decodeDepth int
	// docSchema records the resolution schema the document being decoded was read under.
	// See unmarshalableDocument.
	docSchema token.Schema
}

// NewDecoder returns a decoder that reads the stream from r, with the given options.
func NewDecoder(r io.Reader, opts ...DecodeOption) *Decoder {
	return &Decoder{
		reader:                 r,
		anchorNodeMap:          map[string]ast.Node{},
		referenceAnchorNodeMap: map[string]ast.Node{},
		anchorValueMap:         map[string]reflect.Value{},
		openAnchors:            map[string]int{},
		customUnmarshalerMap:   map[reflect.Type]func(any, []byte) error{},
		opts:                   opts,
		referenceReaders:       []io.Reader{},
		referenceFiles:         []string{},
		referenceDirs:          []string{},
		isRecursiveDir:         false,
		isResolvedReference:    false,
		disallowUnknownField:   false,
		allowDuplicateMapKey:   false,
		useOrderedMap:          false,
		useStringKeys:          false,
	}
}

const (
	// maxDecodeDepth bounds the nested calls made by the tree decoder.
	// The parser rejects a document nested past its own 10,000
	// levels with ErrExceededMaxDepth. A level costs the decoder up to
	// three calls for a struct behind a pointer, so this guard does not fire on
	// a document read by the parser. It guards a tree built by hand and passed
	// to NodeToValue, which no parse has bounded.
	maxDecodeDepth = 50000
)

// Decode reads the next document of the stream into the value v points to, and returns [io.EOF] once the stream holds
// no more document.
//
// A value that does not fit its field leaves that field alone, and Decode reads the rest of the document, then returns
// the first such error. An error that stops one document, such as a repeated key, fails that call only: the next call
// reads the next document. A syntax error, or a document nested past the depth limit, stops the stream, and every later
// call returns it.
//
// See [Unmarshal] for how a document maps to a Go value.
func (d *Decoder) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if !codecutils.IsPointer(rv) {
		return yamlerrors.ErrDecodeRequiredPointerType
	}
	if d.streamErr != nil {
		return yamlerrors.WithSource(d.streamErr, d.source)
	}
	if d.isInitialized() {
		if err := d.decode(rv); err != nil {
			return yamlerrors.WithSource(err, d.source)
		}

		return nil
	}
	if err := d.decodeInit(rv); err != nil {
		d.streamErr = err

		return yamlerrors.WithSource(err, d.source)
	}
	if err := d.decode(rv); err != nil {
		return yamlerrors.WithSource(err, d.source)
	}
	if d.firstOnly && d.pendingErr != nil {
		// Unmarshal rejects a stream whose later documents do not parse, as the walk does.
		// It has decoded the first document into v by then, as the walk has.
		return yamlerrors.WithSource(d.pendingErr, d.source)
	}

	return nil
}

// DecodeFromNode decodes node into the value v points to, with the decoder's options. It reads nothing from the stream.
func (d *Decoder) DecodeFromNode(node ast.Node, v any) error {
	rv := reflect.ValueOf(v)
	if !codecutils.IsPointer(rv) {
		return yamlerrors.ErrDecodeRequiredPointerType
	}
	if !d.isInitialized() {
		// The caller passes a tree, so this never takes the walking path.
		if err := d.decodeInit(reflect.Value{}); err != nil {
			return err
		}
	}
	// A node has no source of its own, and decodeInit budgeted the source of the reader.
	// Each call budgets its node and starts its count afresh.
	d.built, d.budget = 0, codecutils.AliasBudget(astutils.CountNodes(node))
	d.expanding, d.expanded = nil, nil
	// Resolve references to the anchors of the same file.
	if _, err := d.nodeToValue(node); err != nil {
		return err
	}
	if err := d.decodeValue(rv.Elem(), node); err != nil {
		return err
	}
	return nil
}

func (d *Decoder) stepIn() {
	d.decodeDepth++
	d.built++
}

func (d *Decoder) stepOut() {
	d.decodeDepth--
}

// enterAnchor records that the node anchored as name is being decoded, and
// returns the call that undoes it.
func (d *Decoder) enterAnchor(name string) (leave func()) {
	d.openAnchors[name]++

	return func() {
		if d.openAnchors[name]--; d.openAnchors[name] == 0 {
			delete(d.openAnchors, name)
		}
	}
}

// enterMerge sets merging for the mapping brought in by a "<<". It returns the
// call that restores the previous value.
func (d *Decoder) enterMerge() (leave func()) {
	was := d.merging
	d.merging = true

	return func() { d.merging = was }
}

func (d *Decoder) isExceededMaxDepth() bool {
	return d.decodeDepth > maxDecodeDepth
}

// isOverBudget reports whether the decode has built more than the document can account
// for. Only an alias can do that.
func (d *Decoder) isOverBudget() bool {
	return d.budget > 0 && d.built > d.budget
}

// refuseOverBudget returns the error that stops a decode past its budget.
//
// The error names an alias, as the walk does. Only an alias builds more than the
// document holds. The current node when the count runs out sits somewhere
// in an anchored tree, which may belong to another document. The named alias is the
// outermost one being read through, or else the last one read.
//
// The walk counts only what it copies for an alias, so it always runs out
// inside one. refuseOverBudget counts every node, so it can run out on the first node after
// an alias, and then names that alias. Counting only under an alias would miss the
// aliases followed by getMapNode and getArrayNode, which do not set expanding.
func (d *Decoder) refuseOverBudget(at ast.Node) error {
	switch {
	case d.expanding != nil:
		at = d.expanding
	case d.expanded != nil:
		at = d.expanded
	}
	var tk *token.Token
	if at != nil {
		tk = at.GetToken()
	}

	return yamlerrors.NewExcessiveAliasing(d.built, d.budget, tk)
}

func (d *Decoder) castToFloat(v any) any {
	return codecutils.CastToFloatValue(v)
}

func (d *Decoder) mapKeyNodeToString(node ast.MapKeyNode) (string, error) {
	key, err := d.nodeToValue(node)
	if err != nil {
		return "", err
	}

	name, err := astutils.MapKeyString(node, key)
	if err != nil {
		return "", err
	}

	return d.strs.Clone(name), nil
}

// mapKeyNodeToValue returns the key that addresses a [MapSlice] entry.
//
// A MapItem.Key is an interface{}, so it holds the value the key resolves to, as a
// map[any]any key does: "1:" gives an integer, "1.0:" a float, "null:" nil.
// Keying by text put `"1.0": a` and `1.0: b` in one MapSlice under
// one key, while 3.2.1.1 makes a string and a float two keys.
// A merge then let one override the other. That was defect 69.
//
// [UseStringKeys] selects the text instead, for every scalar. A
// collection goes to astutils.MapKeyString either way, which rejects it. A MapSlice key
// must be comparable, and Go hashes no slice or map.
func (d *Decoder) mapKeyNodeToValue(node ast.MapKeyNode) (any, error) {
	key, err := d.nodeToValue(node)
	if err != nil {
		return nil, err
	}
	if d.useStringKeys || !codecutils.HashableKey(key) {
		name, err := astutils.MapKeyString(node, key)
		if err != nil {
			return nil, err
		}

		return d.strs.Clone(name), nil
	}

	return key, nil
}

// setToMapValue fills m from node.
//
// merged marks entries that come from a "<<" instead of from the mapping
// holding it. The own keys of a mapping win over the merged ones, on whichever
// side of the "<<" they were written. An earlier "<<" wins over a later one.
// So setToMapValue writes a merged key only where no key stands already.
// keyToNodeMap applies the same rule for a struct. Before, this path did not:
// "x: 9" over "<<: *a" took the merged x, and "<<: [*a, *b]" took the value of b
// where the merge spec gives it to a.
func (d *Decoder) setToMapValue(node ast.Node, m *codecutils.KeyedMap, merged bool) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.MappingValueNode:
		if n.Key.IsMergeKey() {
			return d.eachMergeSource(n.Value, func(value ast.MapNode) error {
				return astutils.EachMergedEntry(value, func(v ast.Node) error {
					return d.setToMapValue(v, m, true)
				})
			})
		}
		key, err := d.mapKeyNodeToValue(n.Key)
		if err != nil {
			return err
		}
		if merged && m.Holds(key) {
			return nil
		}

		v, err := d.nodeToValue(n.Value)
		if err != nil {
			return err
		}
		m.Put(key, v)
	case *ast.MappingNode:
		return astutils.EachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
			return d.setToMapValue(value, m, merged || isMerge)
		})
	case *ast.AnchorNode:
		anchorName := n.Name.GetToken().Value
		d.anchorNodeMap[anchorName] = n.Value
	}
	return nil
}

// setToOrderedMapValue fills m from node, reading merged as [Decoder.setToMapValue] does.
//
// A MapSlice keeps the order of the document, which a map cannot. So setToOrderedMapValue
// overwrites a key already present in place instead of appending it again.
// "<<: *a" over "x: 9" gives one entry holding 9, at the place where the merge brought x
// in. Appending gave a MapSlice with x twice. No reader of a MapSlice expects that,
// and json.Marshal writes it as a repeated member.
func (d *Decoder) setToOrderedMapValue(node ast.Node, m *MapSlice, merged bool) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.MappingValueNode:
		if n.Key.IsMergeKey() {
			return d.eachMergeSource(n.Value, func(value ast.MapNode) error {
				return astutils.EachMergedEntry(value, func(v ast.Node) error {
					return d.setToOrderedMapValue(v, m, true)
				})
			})
		}
		key, err := d.mapKeyNodeToValue(n.Key)
		if err != nil {
			return err
		}
		if merged && m.index(key) >= 0 {
			return nil
		}
		value, err := d.nodeToValue(n.Value)
		if err != nil {
			return err
		}

		return m.Set(key, value)
	case *ast.MappingNode:
		return astutils.EachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
			return d.setToOrderedMapValue(value, m, merged || isMerge)
		})
	}
	return nil
}

func (d *Decoder) setPathToCommentMap(node ast.Node) {
	if node == nil {
		return
	}
	if d.toCommentMap == nil {
		return
	}
	astutils.EachComment(node, func(path string, place astutils.CommentPlace, texts []string) {
		d.addCommentToMap(path, &Comment{Texts: texts, Position: commentPositionOf(place)})
	})
}

// commentPositionOf returns the CommentMap position of a comment found by astutils.EachComment at place.
func commentPositionOf(place astutils.CommentPlace) CommentPosition {
	switch place {
	case astutils.LinePlace:
		return CommentLinePosition
	case astutils.FootPlace:
		return CommentFootPosition
	default:
		return CommentHeadPosition
	}
}

func (d *Decoder) addCommentToMap(path string, comment *Comment) {
	for _, c := range d.toCommentMap[path] {
		if c.Position == comment.Position {
			// The same comment is already added.
			return
		}
	}
	d.toCommentMap[path] = append(d.toCommentMap[path], comment)
	sort.Slice(d.toCommentMap[path], func(i, j int) bool {
		return d.toCommentMap[path][i].Position < d.toCommentMap[path][j].Position
	})
}

//nolint:gocyclo,cyclop,maintidx,gocognit // one case per node kind, and the tag case one per ast.TagNode.Resolve verdict
func (d *Decoder) nodeToValue(node ast.Node) (any, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, yamlerrors.ErrExceededMaxDepth
	}
	if d.isOverBudget() {
		return nil, d.refuseOverBudget(node)
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.NullNode:
		return nil, nil
	case *ast.StringNode:
		return d.strs.Value(n.Value), nil
	case *ast.IntegerNode:
		return n.GetValue(), nil
	case *ast.FloatNode:
		return n.GetValue(), nil
	case *ast.BoolNode:
		return n.GetValue(), nil
	case *ast.InfinityNode:
		return n.GetValue(), nil
	case *ast.NanNode:
		return n.GetValue(), nil
	case *ast.TagNode:
		// ast.TagNode.Resolve decides what the tag makes of the node, not
		// this switch. The converter in codec/tojson.go calls Resolve too.
		// Before it did, the two disagreed about "!!timestamp
		// not-a-date" and about "!!binary" on an empty node.
		res := n.Resolve()

		switch res.Verdict {
		case ast.TagUnresolved:
			// A local tag such as "!Ref", a foreign one, or a name YAML's type
			// repository does not define. 6.9.1 leaves it to the application, so
			// the decoder reads the node by its kind.
			return d.nodeToValue(n.Value)
		case ast.TagKindMismatch:
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("%s does not support this kind of node", res.Tag), n.GetToken())
		case ast.TagValueMismatch:
			if res.Lax {
				// parser.WithLaxTags: the source text of the scalar
				// replaces the value the tag could not make of it.
				return d.strs.Value(res.Text), nil
			}

			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), n.Value.GetToken())
		default:
			// Nothing to do.
		}

		v, err := d.taggedValue(n, res)
		if err != nil {
			return nil, err
		}

		// The tag and the anchor under it belong to one node, in whichever order
		// the document wrote them (6.9), so the anchor names the tagged
		// value. This case records it instead of the AnchorNode case below.
		// That case runs first and only sees the node under the tag.
		// Before, "a: !!int &a1 \"5\"" read 5 at a and "5" at "b: *a1": one node read
		// as a number in place and as a string through an alias to it.
		if anchor, anchored := n.Value.(*ast.AnchorNode); anchored && v != nil {
			d.anchorValueMap[anchor.Name.GetToken().Value] = reflect.ValueOf(v)
		}

		return v, nil
	case *ast.AnchorNode:
		anchorName := n.Name.GetToken().Value

		// An alias may be processed recursively, so the result of the alias is set to nil in advance.
		d.anchorNodeMap[anchorName] = nil
		leave := d.enterAnchor(anchorName)
		anchorValue, err := d.nodeToValue(n.Value)
		leave()
		if err != nil {
			delete(d.anchorNodeMap, anchorName)
			return nil, err
		}
		d.anchorNodeMap[anchorName] = n.Value
		d.anchorValueMap[anchorName] = reflect.ValueOf(anchorValue)
		return anchorValue, nil
	case *ast.AliasNode:
		text := n.Value.String()
		if d.openAnchors[text] > 0 {
			// The alias stands inside the node named by its own anchor, directly
			// or through another anchor. The parser reads that document, and
			// the tree it builds holds a cycle, because the YAML representation is a
			// graph. A Go value is built by walking and has nowhere to
			// put a cycle. Returning nil instead gave a mapping with a null in
			// it and no error at all.
			return nil, yamlerrors.NewRecursiveAlias(text, n.Value.GetToken())
		}
		if v, exists := d.anchorValueMap[text]; exists && d.shareAliases {
			// ShareAliases: every alias of the anchor gets the same Go value.
			if !v.IsValid() {
				return nil, nil
			}

			return v.Interface(), nil
		}
		if target, _ := d.aliasTarget(n); target != nil {
			// Read the node again instead of returning its earlier value.
			// Before, two aliases of one anchor named one Go value. Writing
			// through either changed the other, and a caller who never wrote a
			// document twice had two names for one map. Sharing
			// even depended on whether the destination declared a
			// field for the anchor itself. With one, anchorValueMap held the
			// value and every alias got it. Without one, nothing recorded it,
			// and each alias decoded the node afresh.
			//
			// codecutils.AliasBudget bounds what this costs.
			if d.expanding == nil {
				d.expanding, d.expanded = n, n
				defer func() { d.expanding = nil }()
			}

			return d.nodeToValue(target)
		}
		if v, exists := d.anchorValueMap[text]; exists {
			// No node to read: the anchor was published by ReferenceFiles.
			// It carries a value, not a tree of this document.
			if !v.IsValid() {
				return nil, nil
			}
			return v.Interface(), nil
		}
		return nil, yamlerrors.NewUnknownAnchor(text, n.Value.GetToken())
	case *ast.LiteralNode:
		return d.strs.Value(n.Value.Value), nil
	case *ast.MappingKeyNode:
		return d.nodeToValue(n.Value)
	case *ast.MappingValueNode:
		if n.Key.IsMergeKey() {
			return d.mergeKeyValue(n.Value)
		}
		if d.useOrderedMap {
			key, err := d.mapKeyNodeToValue(n.Key)
			if err != nil {
				return nil, err
			}
			v, err := d.nodeToValue(n.Value)
			if err != nil {
				return nil, err
			}
			var m MapSlice
			if err := m.Set(key, v); err != nil {
				return nil, err
			}

			return m, nil
		}
		key, err := d.mapKeyNodeToString(n.Key)
		if err != nil {
			return nil, err
		}
		v, err := d.nodeToValue(n.Value)
		if err != nil {
			return nil, err
		}
		return map[string]any{key: v}, nil
	case *ast.MappingNode:
		// Under UseStringKeys an any keys its entries by name.
		if err := astutils.RefuseRepeats(n, d.useStringKeys); err != nil {
			return nil, err
		}
		if d.useOrderedMap {
			var m MapSlice
			if len(n.Values) > 0 {
				m.items = make([]MapItem, 0, len(n.Values))
			}
			if err := astutils.EachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
				return d.setToOrderedMapValue(value, &m, isMerge)
			}); err != nil {
				return nil, err
			}
			return m, nil
		}
		m := codecutils.NewKeyedMap(len(n.Values))
		if err := astutils.EachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
			return d.setToMapValue(value, m, isMerge)
		}); err != nil {
			return nil, err
		}
		return m.Value(), nil
	case *ast.SequenceNode:
		v := make([]any, 0, len(n.Values))
		for _, value := range n.Values {
			vv, err := d.nodeToValue(value)
			if err != nil {
				return nil, err
			}
			v = append(v, vv)
		}
		return v, nil
	}
	return nil, nil
}

// eachMergeSource calls fn on each mapping named by a "<<", one at a time and in the
// order written: the mapping itself, or each mapping of a sequence of them.
//
// getMapNode folds a sequence into one MapNode, which loses the end of each of its
// mappings. Each mapping must be read whole, its own keys and then the entries
// of its own "<<", before the next, because an earlier mapping wins over a
// later one. "<<: [*a, *b]" takes a key merged in by a over the same key written by b.
func (d *Decoder) eachMergeSource(node ast.Node, fn func(ast.MapNode) error) error {
	return d.eachMergeNode(node, func(one ast.Node, inSequence bool) error {
		value, err := d.getMapNode(one, !inSequence)
		if err != nil {
			return err
		}

		return fn(value)
	})
}

// eachMergeNode calls fn on the node of each mapping named by a "<<", in the
// order eachMergeSource reads them. inSequence marks a node that is an element of a
// sequence. Such an element must be a mapping, and a sequence there is rejected.
func (d *Decoder) eachMergeNode(node ast.Node, fn func(one ast.Node, inSequence bool) error) error {
	if err := d.refuseMergeTags(node); err != nil {
		return err
	}
	seq := d.mergeSequence(node)
	if seq == nil {
		return fn(node, false)
	}
	for _, one := range seq.Values {
		if err := fn(one, true); err != nil {
			return err
		}
	}

	return nil
}

// mergeSequence returns the sequence named by a "<<", through an anchor, an alias
// or a tag. It returns nil when the "<<" names anything else. It records an anchor on the
// way, as getMapNode does.
func (d *Decoder) mergeSequence(node ast.Node) *ast.SequenceNode {
	for {
		switch n := node.(type) {
		case *ast.SequenceNode:
			return n
		case *ast.AnchorNode:
			d.anchorNodeMap[n.Name.GetToken().Value] = n.Value
			node = n.Value
		case *ast.AliasNode:
			target, _ := d.aliasTarget(n)
			if target == nil {
				// getMapNode returns the unknown anchor error.
				return nil
			}
			node = target
		case *ast.TagNode:
			node = n.Value
		default:
			return nil
		}
	}
}

// refuseMergeTags returns an error for a tag on a node named by a "<<". See mergeTagError.
func (d *Decoder) refuseMergeTags(node ast.Node) error {
	return mergeTagError(node, func(n *ast.AliasNode) ast.Node {
		target, _ := d.aliasTarget(n)

		return target
	})
}

// mergeTagError returns an error for a tag on value, the node named by a "<<", or on an element of the sequence it
// names, when the tag does not fit the node, as taggedValue returns for the node decoded as a value. It reads the
// tags through anchors, and through aliases by target, which returns the node an alias names or nil.
//
// A tag naming another kind than its node, such as "!!seq" on a mapping, is an error. So is an "!!omap" that is
// not a sequence of one-entry mappings with no key repeated across them. A valid "!!omap" merges as the sequence of
// mappings it is written as.
//
// The decoder and the JSON tokens both call it, so the readers agree on which tags a merge accepts.
func mergeTagError(value ast.Node, target func(*ast.AliasNode) ast.Node) error {
	core, err := underMergeTags(value, target)
	if err != nil {
		return err
	}
	if seq, isSeq := core.(*ast.SequenceNode); isSeq {
		for _, one := range seq.Values {
			if _, err := underMergeTags(one, target); err != nil {
				return err
			}
		}
	}

	return nil
}

// underMergeTags returns the node under the anchors, aliases and tags written on node, or an error for the first
// tag that does not fit. See mergeTagError. It returns nil for an alias that names nothing.
func underMergeTags(node ast.Node, target func(*ast.AliasNode) ast.Node) (ast.Node, error) {
	for range maxMergeWrappers {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
		case *ast.AliasNode:
			node = target(n)
		case *ast.TagNode:
			res := n.Resolve()
			if res.Verdict == ast.TagKindMismatch {
				return nil, yamlerrors.NewSyntax(fmt.Sprintf("%s does not support this kind of node", res.Tag), n.GetToken())
			}
			if res.Tag == token.OrderedMapTag {
				if err := refuseBadOrderedMap(n); err != nil {
					return nil, err
				}
			}
			node = n.Value
		default:
			return node, nil
		}
	}

	return node, nil
}

// maxMergeWrappers bounds the aliases underMergeTags follows. An alias naming itself, through an anchor around it,
// would loop otherwise. The parser rejects such a document, so the bound only guards the loop.
const maxMergeWrappers = 64

// refuseBadOrderedMap returns an error for n, tagged "!!omap", when it is not an ordered map.
func refuseBadOrderedMap(n *ast.TagNode) error {
	seq, entries, ordered := ast.OrderedMapOf(n)
	if !ordered {
		return astutils.NotAnOrderedMap(n.Value)
	}
	for _, entry := range entries {
		if err := astutils.RefuseDuplicateKeys(entry); err != nil {
			return err
		}
	}

	return astutils.RefuseOrderedMapDuplicates(seq)
}

func (d *Decoder) getMapNode(node ast.Node, isMerge bool) (ast.MapNode, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, yamlerrors.ErrExceededMaxDepth
	}

	if node == nil {
		// A node the parse left unfinished, such as the value of a tag when the document ends inside a flow
		// collection. The walk hands such a node over as the parse fails, and the parse error is what the decode
		// returns.
		return nil, yamlerrors.NewSyntax("a mapping holds no node", nil)
	}
	if err := astutils.RefuseDuplicateKeys(node); err != nil {
		return nil, err
	}

	switch n := node.(type) {
	case ast.MapNode:
		return n, nil
	case *ast.AnchorNode:
		anchorName := n.Name.GetToken().Value
		d.anchorNodeMap[anchorName] = n.Value
		return d.getMapNode(n.Value, isMerge)
	case *ast.AliasNode:
		target, name := d.aliasTarget(n)
		if target == nil {
			return nil, yamlerrors.NewUnknownAnchor(name, n.GetToken())
		}
		return d.getMapNode(target, isMerge)
	case *ast.TagNode:
		return d.getMapNode(n.Value, isMerge)
	case *ast.SequenceNode:
		if !isMerge {
			return nil, yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken())
		}
		var mapNodes []ast.MapNode
		for _, value := range n.Values {
			mapNode, err := d.getMapNode(value, false)
			if err != nil {
				return nil, err
			}
			mapNodes = append(mapNodes, mapNode)
		}
		return ast.SequenceMergeValue(mapNodes...), nil
	}
	return nil, yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken())
}

// aliasTarget returns the node named by an alias, and the name written in the
// alias.
//
// The parser fills [ast.AliasNode.Target] as it reads. So a node passed on
// its own, as DecodeFromNode and expressions.Path.Read receive one, already
// carries the targets of its aliases. Before this, the decoder looked the name up in
// anchorNodeMap, which is built from a whole file. An alias whose anchor
// stood outside the node found nothing.
//
// aliasTarget still reads anchorNodeMap for a tree built by hand or written by the
// encoder. Neither went through a parse.
func (d *Decoder) aliasTarget(n *ast.AliasNode) (ast.Node, string) {
	name := n.Value.GetToken().Value
	if n.Target != nil {
		return n.Target, name
	}

	return d.anchorNodeMap[name], name
}

func (d *Decoder) getArrayNode(node ast.Node) (ast.ArrayNode, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, yamlerrors.ErrExceededMaxDepth
	}

	if node == nil {
		// A node the parse left unfinished. See getMapNode.
		return nil, nil
	}
	if _, ok := node.(*ast.NullNode); ok {
		return nil, nil
	}
	if anchor, ok := node.(*ast.AnchorNode); ok {
		return d.getArrayNode(anchor.Value)
	}
	if alias, ok := node.(*ast.AliasNode); ok {
		target, name := d.aliasTarget(alias)
		if target == nil {
			return nil, yamlerrors.NewUnknownAnchor(name, alias.GetToken())
		}
		return d.getArrayNode(target)
	}
	if tag, ok := node.(*ast.TagNode); ok {
		return d.getArrayNode(tag.Value)
	}
	arrayNode, ok := node.(ast.ArrayNode)
	if !ok {
		return nil, yamlerrors.NewUnexpectedNodeType(node.Type(), ast.SequenceType, node.GetToken())
	}
	return arrayNode, nil
}

// binaryBytes returns the bytes held by a "!!binary" node. It returns false for
// every other node.
//
// The tag names a sequence of bytes, and the decoder builds a []byte for an
// `any`. So a []byte destination is the Go type that matches the tag.
// Before, decodeSlice read the node as a sequence and returned `string was used where
// sequence is expected`, while the same document into a string field gave the
// decoded bytes.
func (d *Decoder) binaryBytes(node ast.Node) ([]byte, bool) {
	switch n := node.(type) {
	case *ast.AnchorNode:
		return d.binaryBytes(n.Value)
	case *ast.AliasNode:
		target, _ := d.aliasTarget(n)
		if target == nil {
			return nil, false
		}

		return d.binaryBytes(target)
	case *ast.TagNode:
		res := n.Resolve()
		if res.Tag != token.BinaryTag || res.Verdict != ast.TagResolved {
			return nil, false
		}
		if res.Empty {
			// "k: !!binary" with nothing after it takes the default of the tag.
			// tagZero writes that default as an empty []byte.
			return []byte{}, true
		}
		b, err := base64.StdEncoding.DecodeString(res.Text)

		return b, err == nil
	default:
		return nil, false
	}
}

// mergeKeyValue reads the mapping named by a "<<" as a value of its own.
// Under UseOrderedMap the value is a MapSlice.
func (d *Decoder) mergeKeyValue(node ast.Node) (any, error) {
	if d.useOrderedMap {
		m := MapSlice{}
		err := d.eachMergeSource(node, func(value ast.MapNode) error {
			return astutils.EachMergedEntry(value, func(v ast.Node) error {
				return d.setToOrderedMapValue(v, &m, true)
			})
		})

		return m, err
	}
	m := codecutils.NewKeyedMap(0)
	err := d.eachMergeSource(node, func(value ast.MapNode) error {
		return astutils.EachMergedEntry(value, func(v ast.Node) error {
			return d.setToMapValue(v, m, true)
		})
	})
	if err != nil {
		return nil, err
	}

	return m.Value(), nil
}

func (d *Decoder) convertValue(v reflect.Value, typ reflect.Type, src ast.Node) (reflect.Value, error) {
	if converted, ok := codecutils.ConvertBigNumber(v, typ); ok {
		return converted, nil
	}
	if codecutils.IsBigNumber(v) && codecutils.IsFloatKind(typ.Kind()) {
		return reflect.Value{}, yamlerrors.NewOverflow(typ, fmt.Sprint(v.Interface()), src.GetToken())
	}
	if typ.Kind() != reflect.String {
		return codecutils.ConvertToKind(v, typ, src)
	}
	// Cast the value to a string.
	var strVal string
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		strVal = strconv.FormatInt(v.Int(), 10)
	case reflect.Float32, reflect.Float64:
		strVal = fmt.Sprint(v.Float())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		strVal = strconv.FormatUint(v.Uint(), 10)
	case reflect.Bool:
		strVal = strconv.FormatBool(v.Bool())
	case reflect.Struct:
		// A timestamp is written as its instant in RFC 3339, in its own zone.
		// ast.KeyName names a timestamp key the same way, and ToJSON writes one the same way.
		// "2001-12-14" under YAML 1.1 is "2001-12-14T00:00:00Z".
		t, isTime := reflect.TypeAssert[time.Time](v)
		if !isTime {
			return reflect.Zero(typ), yamlerrors.NewTypeMismatch(typ, v.Type(), src.GetToken())
		}
		strVal = t.Format(time.RFC3339Nano)
	default:
		if !v.Type().ConvertibleTo(typ) {
			return reflect.Zero(typ), yamlerrors.NewTypeMismatch(typ, v.Type(), src.GetToken())
		}
		return v.Convert(typ), nil
	}

	val := reflect.ValueOf(strVal)
	if val.Type() != typ {
		// Handle named types, such as `type MyString string`.
		val = val.Convert(typ)
	}
	return val, nil
}

// tagMode returns the encoding/json rules this decoder adds to the `yaml`
// tag. With neither option set, the mode matches go.yaml.in/yaml/v3 exactly.
func (d *Decoder) tagMode() structfields.Mode {
	mode := structfields.YAMLTags
	if d.useJSONTags {
		mode |= structfields.JSONTags
	}
	if d.useInferredNames {
		mode |= structfields.InferredNames
	}

	return mode
}

func (d *Decoder) deleteStructKeys(structType reflect.Type, unknownFields map[string]ast.Node) error {
	for structType.Kind() == reflect.Pointer {
		structType = structType.Elem()
	}
	if structType.Kind() == reflect.Map {
		// A `,inline` map takes every entry not claimed by a field, so a mapping read
		// into this struct leaves no unknown field.
		clear(unknownFields)

		return nil
	}
	if structType.Kind() != reflect.Struct {
		return nil
	}

	structFieldMap, err := structfields.MapOf(structType, d.tagMode())
	if err != nil {
		return err
	}

	for field := range structType.Fields() {
		if structfields.Ignored(field, d.tagMode()) {
			continue
		}

		structField, exists := structFieldMap[field.Name]
		if !exists {
			continue
		}

		if structField.IsInline {
			_ = d.deleteStructKeys(field.Type, unknownFields)
		} else {
			delete(unknownFields, structField.RenderName)
		}
	}
	return nil
}

// unmarshalableDocument renders node as the document read by a bytes unmarshaler.
//
// When the document was read under YAML 1.1, the text opens with "%YAML 1.1".
// A nested Unmarshal then reads the fragment under the same version: "<<" merges
// and "yes" is true. Without the line the fragment parsed as 1.2, and a merge
// inside it came back as an ordinary key named "<<".
func (d *Decoder) unmarshalableDocument(node ast.Node) ([]byte, error) { //nolint:unparam // signature kept stable: callers already handle the error
	doc := format.NodeWithResolvedAlias(node, d.anchorNodeMap)
	if d.docSchema == token.Schema11 {
		return []byte("%YAML 1.1\n---\n" + doc), nil
	}
	return []byte(doc), nil
}

// unmarshalableText decodes node as a string field would, for an
// encoding.TextUnmarshaler: under the document's YAML version, through its
// anchors, and with an error at the node's own position.
//
// Before, it rendered the node back to text and decoded that as a document of
// its own, under YAML 1.2 and from line 1. Under 1.1, "yes" reached
// UnmarshalText as "yes" where a string field holds "true". A
// "!!timestamp" accepted by the string field was rejected.
func (d *Decoder) unmarshalableText(node ast.Node) ([]byte, error) {
	var v string
	if err := d.decodeValue(reflect.ValueOf(&v).Elem(), node); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

func (d *Decoder) existsTypeInCustomUnmarshalerMap(t reflect.Type) bool {
	if _, exists := d.customUnmarshalerMap[t]; exists {
		return true
	}

	customUnmarshalerMx.RLock()
	_, exists := customUnmarshalerMap[t]
	customUnmarshalerMx.RUnlock()

	return exists
}

func (d *Decoder) unmarshalerFromCustomUnmarshalerMap(t reflect.Type) (func(any, []byte) error, bool) {
	if unmarshaler, exists := d.customUnmarshalerMap[t]; exists {
		return unmarshaler, exists
	}

	customUnmarshalerMx.RLock()
	defer customUnmarshalerMx.RUnlock()
	if unmarshaler, exists := customUnmarshalerMap[t]; exists {
		return unmarshaler, exists
	}
	return nil, false
}

func (d *Decoder) canDecodeByUnmarshaler(dst reflect.Value) bool {
	ptrValue := dst.Addr()
	if d.existsTypeInCustomUnmarshalerMap(ptrValue.Type()) {
		return true
	}

	return decodesItself(ptrValue, d.useJSONUnmarshaler)
}

// decodesItself reports whether the value pointed to by ptr reads itself from the
// document. It does through an interface it implements, or when the decoder reads
// its type specially. A json.Unmarshaler counts only with useJSONUnmarshaler.
func decodesItself(ptr reflect.Value, useJSONUnmarshaler bool) bool {
	if v3NodeHook(ptr.Type()) != nil {
		// UnmarshalYAML(*yaml.Node), go.yaml.in/yaml/v3's own hook.
		return true
	}
	switch ptr.Interface().(type) {
	case *MapSlice, *MapSliceSeq, *MapItem, *Base64:
		// The decoder reads these itself, ahead of any interface they satisfy.
		// [UseJSONUnmarshaler] is the counterpart of [UseJSONMarshaler], and a
		// last resort the same way. A document passed to a MapSlice through
		// ToJSON would arrive with JSON keys, which are strings. It would lose
		// every key that MapSlice keeps apart.
		return false
	case Unmarshaler,
		GoYAMLUnmarshaler,
		NodeUnmarshaler,
		*time.Time,
		*time.Duration,
		encoding.TextUnmarshaler:
		return true
	case jsonUnmarshaler:
		return useJSONUnmarshaler
	}
	return false
}

// decodeByUnmarshaler decodes src into dst through the interface or the registered function dst decodes itself with.
//
// An error the type returns comes back as ErrUnmarshaler at src, so it carries a position.
// An error that already is one of this package's, such as a type mismatch from the unmarshal
// callback of a GoYAMLUnmarshaler, comes back unchanged with its own position.
func (d *Decoder) decodeByUnmarshaler(dst reflect.Value, src ast.Node) error {
	err := d.callUnmarshaler(dst, src)
	if err == nil {
		return nil
	}
	if _, ours := errors.AsType[*yamlerrors.Error](err); ours {
		return err
	}

	return yamlerrors.NewUnmarshaler(err, src.GetToken())
}

func (d *Decoder) callUnmarshaler(dst reflect.Value, src ast.Node) error {
	ptrValue := dst.Addr()
	if unmarshaler, exists := d.unmarshalerFromCustomUnmarshalerMap(ptrValue.Type()); exists {
		b, err := d.unmarshalableDocument(src)
		if err != nil {
			return err
		}
		if err := unmarshaler(ptrValue.Interface(), b); err != nil {
			return err
		}
		return nil
	}
	if layout := v3NodeHook(ptrValue.Type()); layout != nil {
		return d.callV3NodeHook(ptrValue, layout, src)
	}
	iface := ptrValue.Interface()

	if unmarshaler, ok := iface.(Unmarshaler); ok {
		b, err := d.unmarshalableDocument(src)
		if err != nil {
			return err
		}
		if err := unmarshaler.UnmarshalYAML(b); err != nil {
			return err
		}
		return nil
	}

	if unmarshaler, ok := iface.(GoYAMLUnmarshaler); ok {
		if err := unmarshaler.UnmarshalYAML(func(v any) error {
			rv := reflect.ValueOf(v)
			if !codecutils.IsPointer(rv) {
				return yamlerrors.ErrDecodeRequiredPointerType
			}
			if err := d.decodeValue(rv.Elem(), src); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	if unmarshaler, ok := iface.(NodeUnmarshaler); ok {
		if err := unmarshaler.UnmarshalYAML(src); err != nil {
			return err
		}

		return nil
	}

	if _, ok := iface.(*time.Time); ok {
		return d.decodeTime(dst, src)
	}

	if _, ok := iface.(*time.Duration); ok {
		return d.decodeDuration(dst, src)
	}

	// textErr holds the error that kept the node from giving text. It is returned
	// when no other interface reads the node. "!!timestamp nope" into a TextUnmarshaler
	// is rejected because it is not a valid timestamp.
	var textErr error
	if unmarshaler, isText := iface.(encoding.TextUnmarshaler); isText {
		b, err := d.unmarshalableText(src)
		if err == nil {
			return unmarshaler.UnmarshalText(b)
		}
		textErr = err
	}

	if d.useJSONUnmarshaler {
		if unmarshaler, ok := iface.(jsonUnmarshaler); ok {
			return d.unmarshalJSON(unmarshaler, src)
		}
	}
	if textErr != nil {
		return textErr
	}

	return errors.New("does not implemented GoYAMLUnmarshaler")
}

// unmarshalJSON converts src to a JSON document and passes it to unmarshaler.
func (d *Decoder) unmarshalJSON(unmarshaler jsonUnmarshaler, src ast.Node) error {
	b, err := d.unmarshalableDocument(src)
	if err != nil {
		return err
	}
	jsonBytes, err := ToJSON(b)
	if err != nil {
		return err
	}

	return unmarshaler.UnmarshalJSON(bytes.TrimRight(jsonBytes, "\n"))
}

//nolint:gocyclo,cyclop,maintidx,gocognit // one case per reflect.Kind of the destination
func (d *Decoder) decodeValue(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}
	if d.isOverBudget() {
		return d.refuseOverBudget(src)
	}
	if !dst.IsValid() {
		return nil
	}
	if layout := v3NodeLayoutOf(dst.Type()); layout != nil {
		// A yaml.Node of go.yaml.in/yaml/v3 gets the node itself, its anchor or alias included, as in v3.
		dst.Set(d.newV3NodeBuilder(layout).build(src).Elem())

		return nil
	}

	if src.Type() == ast.AnchorType {
		anchor, _ := src.(*ast.AnchorNode)
		anchorName := anchor.Name.GetToken().Value
		leave := d.enterAnchor(anchorName)
		err := d.decodeValue(dst, anchor.Value)
		leave()
		if err != nil {
			return err
		}
		d.anchorValueMap[anchorName] = dst
		return nil
	}
	if d.canDecodeByUnmarshaler(dst) {
		if err := d.decodeByUnmarshaler(dst, src); err != nil {
			return err
		}
		return nil
	}
	valueType := dst.Type()
	switch valueType.Kind() {
	case reflect.Pointer:
		// A nil pointer is filled with a new value, as at the root of "var p *int; Unmarshal(src, &p)". A field never
		// reaches here nil: createDecodedNewValue allocates it first. ast.IsNull also reads a null under a tag, such
		// as "!!null null", which src.Type() does not.
		if ast.IsNull(src) {
			// Set the pointer to nil.
			dst.Set(reflect.Zero(valueType))
			return nil
		}
		v := d.createDecodableValue(dst.Type())
		decodeErr := d.decodeValue(v, src)
		castedValue, err := codecutils.CastToAssignableValue(v, dst.Type(), src)
		if err != nil {
			return errors.Join(decodeErr, err)
		}
		// Keep the decoded value, error or not. See createDecodedNewValue.
		dst.Set(castedValue)

		// The value is written. Falling out of the switch decoded the node again, into the pointer type itself, and
		// returned a type mismatch beside the value.
		return decodeErr
	case reflect.Interface:
		if dst.Type() == reflect.TypeFor[ast.Node]() {
			dst.Set(reflect.ValueOf(src))
			return nil
		}
		srcVal, err := d.nodeToValue(src)
		if err != nil {
			return err
		}
		v := reflect.ValueOf(srcVal)
		if v.IsValid() && !v.Type().AssignableTo(valueType) {
			// An interface with methods holds only a type that implements them. Setting a "string" into a
			// fmt.Stringer panicked.
			return yamlerrors.NewTypeMismatch(valueType, v.Type(), src.GetToken())
		}
		if v.IsValid() {
			dst.Set(v)
		} else {
			dst.Set(reflect.Zero(valueType))
		}

		// The value is written. Falling out of the switch decoded the node again, and set it again.
		return nil
	case reflect.Map:
		if ast.IsNull(src) {
			// A null leaves the map as it stands, as for a field. Only a root map reaches here with one:
			// createDecodedNewValue reads a field's null first.
			return nil
		}

		return d.decodeMap(dst, src)
	case reflect.Array:
		return d.decodeArray(dst, src)
	case reflect.Slice:
		return d.decodeSlice(dst, src)
	case reflect.Struct:
		if mapSlice, ok := reflect.TypeAssert[*MapSlice](dst.Addr()); ok {
			return d.decodeMapSlice(mapSlice, src)
		}
		if seq, ok := reflect.TypeAssert[*MapSliceSeq](dst.Addr()); ok {
			return d.decodeOrderedMapSeq(seq, src)
		}
		if mapItem, ok := reflect.TypeAssert[*MapItem](dst.Addr()); ok {
			return d.decodeMapItem(mapItem, src)
		}
		return d.decodeStruct(dst, src)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := d.nodeToValue(src)
		if err != nil {
			return err
		}
		switch vv := v.(type) {
		case nil:
			// A null writes nothing, as for a field, and the value keeps what it holds.
			return nil
		case int64:
			if !dst.OverflowInt(vv) {
				dst.SetInt(vv)
				return nil
			}
		case uint64:
			if vv <= math.MaxInt64 && !dst.OverflowInt(int64(vv)) {
				dst.SetInt(int64(vv))
				return nil
			}
		case float64:
			if vv <= math.MaxInt64 && !dst.OverflowInt(int64(vv)) {
				dst.SetInt(int64(vv))
				return nil
			}
		case *big.Int, *big.Float:
			// The AST returns one of these only for a number too large for
			// int64, uint64 or float64, so no integer kind here can hold it. Fall
			// through to the overflow error, which names the number.
		case string: // Handle scientific notation.
			if i, err := strconv.ParseFloat(vv, 64); err == nil {
				if 0 <= i && i <= math.MaxUint64 && !dst.OverflowInt(int64(i)) {
					dst.SetInt(int64(i))
					return nil
				}
			} else { // It could not be parsed as a float.
				return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
			}
		default:
			return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
		}
		return yamlerrors.NewOverflow(valueType, fmt.Sprint(v), src.GetToken())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := d.nodeToValue(src)
		if err != nil {
			return err
		}
		switch vv := v.(type) {
		case nil:
			// A null writes nothing, as for a field, and the value keeps what it holds.
			return nil
		case int64:
			if 0 <= vv && !dst.OverflowUint(uint64(vv)) {
				dst.SetUint(uint64(vv))
				return nil
			}
		case uint64:
			if !dst.OverflowUint(vv) {
				dst.SetUint(vv)
				return nil
			}
		case float64:
			if 0 <= vv && vv <= math.MaxUint64 && !dst.OverflowUint(uint64(vv)) {
				dst.SetUint(uint64(vv))
				return nil
			}
		case *big.Int, *big.Float:
			// See the signed case above.
		case string: // Handle scientific notation.
			if i, err := strconv.ParseFloat(vv, 64); err == nil {
				if 0 <= i && i <= math.MaxUint64 && !dst.OverflowUint(uint64(i)) {
					dst.SetUint(uint64(i))
					return nil
				}
			} else { // It could not be parsed as a float.
				return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
			}

		default:
			return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
		}
		return yamlerrors.NewOverflow(valueType, fmt.Sprint(v), src.GetToken())
	default:
		// Nothing to do.
	}
	srcVal, err := d.nodeToValue(src)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(srcVal)
	if v.IsValid() {
		convertedValue, err := d.convertValue(v, dst.Type(), src)
		if err != nil {
			return err
		}
		dst.Set(convertedValue)
	}
	return nil
}

func (d *Decoder) createDecodableValue(typ reflect.Type) reflect.Value {
	for {
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
			continue
		}
		break
	}
	return reflect.New(typ).Elem()
}

/*
func (d *Decoder) keyToKeyNodeMap(node ast.Node, ignoreMergeKey bool) (map[string]ast.Node, error) {
	m, err := d.keyToNodeMap(node, ignoreMergeKey, func(nodeMap *ast.MapNodeIter) ast.Node { return nodeMap.Key() })
	if err != nil {
		return nil, err
	}
	return m, nil
}
*/

// aliasedValue returns the decoded value of the anchor of alias, cast to typ,
// when the value can be reused. Otherwise it returns the node named by the anchor,
// for the caller to read again. That node is nil for an anchor without one.
//
// aliasedValue reuses the decoded value when aliases share it, and when there is no
// node to read again, as for an anchor published by ReferenceFiles.
func (d *Decoder) aliasedValue(alias *ast.AliasNode, typ reflect.Type) (reflect.Value, ast.Node, bool) {
	target, name := d.aliasTarget(alias)
	if value := d.anchorValueMap[name]; value.IsValid() && (d.shareAliases || target == nil) {
		if v, err := codecutils.CastToAssignableValue(value, typ, alias); err == nil {
			return v, nil, true
		}
	}

	return reflect.Value{}, target, false
}

func (d *Decoder) createDecodedNewValue(
	typ reflect.Type, defaultVal reflect.Value, node ast.Node,
) (reflect.Value, error) {
	if alias, aliased := node.(*ast.AliasNode); aliased {
		if d.expanding == nil {
			d.expanding, d.expanded = alias, alias
			defer func() { d.expanding = nil }()
		}
		v, target, reused := d.aliasedValue(alias, typ)
		if reused {
			return v, nil
		}
		if target != nil {
			// Read the node again, so that two aliases of one anchor give two
			// values. See the AliasNode case of nodeToValue.
			node = target
		}
	}
	var newValue reflect.Value
	if ast.IsNull(node) {
		newValue = reflect.New(typ).Elem()
	} else {
		newValue = d.createDecodableValue(typ)
	}
	for defaultVal.Kind() == reflect.Pointer {
		defaultVal = defaultVal.Elem()
	}
	if defaultVal.IsValid() && defaultVal.Type().AssignableTo(newValue.Type()) {
		newValue.Set(defaultVal)
	}
	if !ast.IsNull(node) {
		if err := d.decodeValue(newValue, node); err != nil {
			// The value is decoded in place, as encoding/json decodes it. The part
			// decoded before the error is returned beside the error, and the caller
			// writes it. Returning nothing threw away the other fields of a nested struct.
			// The caller of a map lost the entry, because SetMapIndex
			// deletes it for an invalid value.
			if v, castErr := codecutils.CastToAssignableValue(newValue, typ, node); castErr == nil {
				return v, err
			}

			return reflect.Value{}, err
		}
	}
	return codecutils.CastToAssignableValue(newValue, typ, node)
}

func (d *Decoder) keyToNodeMap(node ast.Node, ignoreMergeKey bool, getKeyOrValueNode func(*ast.MapNodeIter) ast.Node) (map[string]ast.Node, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, yamlerrors.ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(node, false)
	if err != nil {
		return nil, err
	}
	// The map is keyed by name.
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping {
		if err := astutils.RefuseRepeats(m, true); err != nil {
			return nil, err
		}
	}
	keyToNodeMap := map[string]ast.Node{}
	var merged []map[string]ast.Node
	mapIter := mapNode.MapRange()
	for mapIter.Next() {
		keyNode := mapIter.Key()
		if keyNode.IsMergeKey() {
			if ignoreMergeKey {
				continue
			}
			mergeMap, err := d.mergedKeyToNodeMap(mapIter.Value(), ignoreMergeKey, getKeyOrValueNode)
			if err != nil {
				return nil, err
			}
			merged = append(merged, mergeMap)

			continue
		}

		// Name the entry as decodeStruct names it, so "1: x" reaches an inline
		// map as "1", as it reaches a map[string]any. Keeping only the keys
		// that decoded to a Go string returned an empty map, and no error, on
		// the first key that did not.
		key, named, err := d.entryName(keyNode)
		if err != nil {
			return nil, err
		}
		if !named {
			// A collection key names no field and no string-keyed entry.
			continue
		}
		// astutils.RefuseRepeats rejected a repeat not allowed by the parse.
		// An allowed repeat keeps the last value.
		keyToNodeMap[key] = getKeyOrValueNode(mapIter)
	}

	// The own keys of a mapping win over the merged ones, on whichever side of the
	// "<<" they were written. An earlier "<<" wins over a later one. Before, merged
	// keys were validated against the own keys of the mapping. So "<<: *base"
	// beside a key also written by the base was rejected as a duplicate, though
	// overriding is the purpose of a merge. The key is not written twice: it is written once
	// here and once in another mapping.
	for _, m := range merged {
		for k, v := range m {
			if _, own := keyToNodeMap[k]; own {
				continue
			}
			keyToNodeMap[k] = v
		}
	}

	return keyToNodeMap, nil
}

// tagZero returns the value of a tag standing on no value: the zero value of its
// type. "k: !!int" is 0 and "k: !!bool" is false, exactly as
// parser.newTagDefaultScalarValueNode writes them into the tree.
func tagZero(tag token.ReservedTagKeyword) (any, error) {
	switch tag {
	case token.IntegerTag:
		return uint64(0), nil
	case token.FloatTag:
		return float64(0), nil
	case token.BooleanTag:
		return false, nil
	case token.StringTag:
		return "", nil
	case token.BinaryTag:
		return Base64(""), nil
	case token.TimestampTag:
		return time.Time{}, nil
	default:
		return nil, nil
	}
}

// mergedKeyToNodeMap reads the node named by a "<<", which is a mapping or a sequence
// of them. getMapNode folds a sequence into one MapNode.
//
// The keys are not validated against one another, and the first one written wins.
// Two mappings of a sequence may write the same key, as in "<<: [*one, *two]" where
// both write "b". The YAML 1.1 merge spec gives the key to the mapping named first,
// and does not reject the document.
func (d *Decoder) mergedKeyToNodeMap(
	node ast.Node, ignoreMergeKey bool, getKeyOrValueNode func(*ast.MapNodeIter) ast.Node,
) (map[string]ast.Node, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, yamlerrors.ErrExceededMaxDepth
	}

	keyToNodeMap := map[string]ast.Node{}
	hold := func(k string, v ast.Node) {
		if _, held := keyToNodeMap[k]; !held {
			keyToNodeMap[k] = v
		}
	}
	err := d.eachMergeSource(node, func(mapNode ast.MapNode) error {
		// Read the own keys of the mapping first, then the entries brought in by its own "<<".
		var merges []ast.Node
		mapIter := mapNode.MapRange()
		for mapIter.Next() {
			keyNode := mapIter.Key()
			if keyNode.IsMergeKey() {
				if !ignoreMergeKey {
					merges = append(merges, mapIter.Value())
				}

				continue
			}

			key, named, err := d.entryName(keyNode)
			if err != nil {
				return err
			}
			if named {
				hold(key, getKeyOrValueNode(mapIter))
			}
		}
		for _, from := range merges {
			nested, err := d.mergedKeyToNodeMap(from, ignoreMergeKey, getKeyOrValueNode)
			if err != nil {
				return err
			}
			for k, v := range nested {
				hold(k, v)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return keyToNodeMap, nil
}

func (d *Decoder) keyToValueNodeMap(node ast.Node, ignoreMergeKey bool) (map[string]ast.Node, error) {
	m, err := d.keyToNodeMap(node, ignoreMergeKey, func(nodeMap *ast.MapNodeIter) ast.Node { return nodeMap.Value() })
	if err != nil {
		return nil, err
	}
	return m, nil
}

// taggedValue returns the value of a tagged node, once ast.TagNode.Resolve has found
// that the tag applies.
func (d *Decoder) taggedValue(n *ast.TagNode, res ast.Resolution) (any, error) {
	if res.Empty {
		// A tag with nothing after it takes the default of the tag.
		// The parser function newTagDefaultScalarValueNode builds it, and before, nothing read it:
		// "k: !!int" was 0 while "k: !!bool" and "k: !!binary" were errors.
		return tagZero(res.Tag)
	}

	switch res.Tag {
	case token.TimestampTag:
		return d.castToTime(n.Value)
	case token.IntegerTag:
		// Read the text under the tag, in the base that the schema of the document
		// gives those digits. Resolving the node instead took a
		// quoted scalar as a string and sent it through
		// the fixed 1.2 reading of the former castToInteger. So `!!int "017"` was 17 in a
		// document where `!!int 017` was 15, and `!!int "0b101"` under 1.1
		// was 0 where the plain spelling was 5. 3.3.2 gives the "!"
		// non-specific tag only to a node lacking an explicit tag, so the two
		// spellings are one node.
		//
		// The two spellings also give one Go value: uint64, int64 or *big.Int, as the
		// untagged number decodes. Narrowed to int, "!!int 1" was int(1) where "1" was
		// uint64(1). Those are two keys in a Go map, so an own "!!int 1" did not beat the
		// "1" brought in by a "<<", and the mapping held both.
		return taggedInteger(res.Text, res.Schema), nil
	case token.FloatTag:
		return d.castToFloat(taggedFloat(res.Text, res.Schema)), nil
	case token.NullTag:
		return nil, nil
	case token.BinaryTag:
		// Return a Base64, not a []byte. A Base64 is comparable, so "? !!binary" can key a
		// mapping, which a byte slice cannot. It also marks the value as binary for an
		// encoder, so a round trip writes "!!binary" again. Resolve has read
		// the text as base64 already. See [Base64].
		return Base64(res.Text), nil
	case token.BooleanTag:
		// The tag sets the type to boolean whatever the text is. So a spelling
		// resolved by neither schema is read in lower case: "!!bool Yes" and
		// "!!bool YES" give the same value. Resolve has already checked that
		// the text holds a boolean.
		b, _ := token.ParseBool(strings.ToLower(res.Text))

		return b, nil
	case token.StringTag:
		// The tag names the type, so the scalar keeps its source text
		// instead of the value resolved by the core schema:
		// "!!str 0x10" is "0x10" and not "16", and "!!str False" keeps its
		// capital F. An anchor over that scalar names the same string, so
		// taggedValue replaces its recorded value too.
		text := d.strs.Clone(res.Text)
		if anchor, anchored := n.Value.(*ast.AnchorNode); anchored {
			d.anchorValueMap[anchor.Name.GetToken().Value] = reflect.ValueOf(text)
		}

		return text, nil
	case token.OrderedMapTag:
		seq, entries, ordered := ast.OrderedMapOf(n)
		if !ordered {
			return nil, astutils.NotAnOrderedMap(n.Value)
		}

		return d.orderedMapOf(seq, entries)
	default:
		// A tag naming a kind: !!seq, !!map, !!set, !!merge. Read the node
		// as it is.
		return d.nodeToValue(n.Value)
	}
}

// orderedMapOf reads a sequence named by an "!!omap" into a [MapSliceSeq].
//
// [ast.OrderedMapOf] checks the shape, so every element holds one entry, and the
// ordered map keeps the order of the sequence. The parser records on the sequence a key
// repeated across entries. astutils.RefuseOrderedMapDuplicates reads that record, as
// every other load does. When the parse allowed the repeat, the value of the later entry
// replaces the earlier one, as it does in a mapping.
func (d *Decoder) orderedMapOf(seq *ast.SequenceNode, entries []ast.Node) (MapSliceSeq, error) {
	// Check a repeat inside an entry first, then a repeat across entries. The walk and
	// the converters find the first when the mapping of the entry closes, and the
	// second only when the "!!omap" closes.
	for _, entry := range entries {
		if err := astutils.RefuseDuplicateKeys(entry); err != nil {
			return MapSliceSeq{}, err
		}
	}
	if err := astutils.RefuseOrderedMapDuplicates(seq); err != nil {
		return MapSliceSeq{}, err
	}

	var m MapSlice
	for _, entry := range entries {
		var one MapSlice
		if err := d.setToOrderedMapValue(entry, &one, false); err != nil {
			return MapSliceSeq{}, err
		}
		if len(one.items) != 1 {
			// [ast.OrderedMapOf] could not count the entries behind a "<<", so check here.
			return MapSliceSeq{}, astutils.NotAnOrderedMap(seq)
		}
		for _, item := range one.items {
			if err := m.Set(item.Key, item.Value); err != nil {
				return MapSliceSeq{}, err
			}
		}
	}

	return MapSliceSeq(m), nil
}

// mapEntryFunc receives one entry of a mapping. merged marks an entry that comes
// from a "<<" instead of from the mapping itself.
type mapEntryFunc func(name string, keyNode, valueNode, entryNode ast.Node, merged bool) error

func (d *Decoder) castToTime(src ast.Node) (time.Time, error) {
	if src == nil {
		return time.Time{}, nil
	}
	v, err := d.nodeToValue(src)
	if err != nil {
		return time.Time{}, err
	}
	if t, ok := v.(time.Time); ok {
		return t, nil
	}
	if v == nil || v == "" {
		// A "!!timestamp" on an empty node, or on a null. It takes the default
		// of the tag, the zero time. newTagDefaultScalarValueNode builds the
		// empty string for a tag with nothing after it.
		// handNull builds a null when the document ends there.
		return time.Time{}, nil
	}
	s, ok := v.(string)
	if !ok {
		return time.Time{}, yamlerrors.NewTypeMismatch(reflect.TypeFor[time.Time](), reflect.TypeOf(v), src.GetToken())
	}
	if t, ok := token.ParseTimestamp(s); ok {
		return t, nil
	}

	// Return an error instead of the zero time, so a text that is no timestamp does not decode to 0001-01-01.
	return time.Time{}, yamlerrors.NewSyntax(
		fmt.Sprintf("cannot read %q as a timestamp", s), src.GetToken())
}

func (d *Decoder) decodeTime(dst reflect.Value, src ast.Node) error {
	t, err := d.castToTime(src)
	if err != nil {
		return err
	}
	dst.Set(reflect.ValueOf(t))
	return nil
}

func (d *Decoder) castToDuration(src ast.Node) (time.Duration, error) {
	if src == nil {
		return 0, nil
	}
	v, err := d.nodeToValue(src)
	if err != nil {
		return 0, err
	}
	if t, ok := v.(time.Duration); ok {
		return t, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, yamlerrors.NewTypeMismatch(reflect.TypeFor[time.Duration](), reflect.TypeOf(v), src.GetToken())
	}
	t, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return t, nil
}

// writtenFields records which fields a mapping has set, so that the entries merged by a "<<"
// do not overwrite them. A struct of 64 fields or fewer costs no
// allocation.
type writtenFields struct {
	first uint64
	rest  []uint64
}

func (w *writtenFields) mark(i int) {
	if i < 64 {
		w.first |= 1 << uint(i%64)

		return
	}
	word := i/64 - 1
	for len(w.rest) <= word {
		w.rest = append(w.rest, 0)
	}
	w.rest[word] |= 1 << uint(i%64)
}

func (w *writtenFields) has(i int) bool {
	if i < 64 {
		return w.first&(1<<uint(i%64)) != 0
	}
	word := i/64 - 1

	return word < len(w.rest) && w.rest[word]&(1<<uint(i%64)) != 0
}

func (d *Decoder) decodeDuration(dst reflect.Value, src ast.Node) error {
	t, err := d.castToDuration(src)
	if err != nil {
		return err
	}
	dst.Set(reflect.ValueOf(t))
	return nil
}

// located describes the field named by a mapping key, found in a destination.
//
// sf is nil when no field of the type has the name. ord gives the bit marked by
// writtenFields for merges. at holds the index path, which is empty when the field
// stands in the type itself.
type located struct {
	value reflect.Value
	sf    *structfields.Info
	ord   int
	at    []int
}

// name returns the field as a Go path, such as "U.T.A" for a field A promoted from an
// embedded T of U. encoding/json names a field the same way in a type error.
func (l located) name(structType reflect.Type) string {
	if len(l.at) == 0 {
		return structType.Name() + "." + l.sf.FieldName
	}

	var b strings.Builder
	b.WriteString(structType.Name())
	t := structType
	for _, step := range l.at {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		field := t.Field(step)
		b.WriteByte('.')
		b.WriteString(field.Name)
		t = field.Type
	}

	return b.String()
}

// fieldFor finds the field named by a mapping key.
//
// fieldFor reads a type that embeds another through structfields.Fields.flat. It names every
// field promoted by the embedding and holds the index path to reach it.
// codecutils.FieldAt fills the pointers along the way. A type that embeds nothing takes the direct
// lookup with no index path. That is the common case and the fast one.
func fieldFor(dst reflect.Value, fields *structfields.Fields, name string) (located, error) {
	ff, known := fields.Lookup(name)
	if !known {
		return located{}, nil
	}
	if ff.At == nil {
		return located{value: dst.Field(ff.SF.Index), sf: ff.SF, ord: ff.Ord}, nil
	}

	field, ok := codecutils.FieldAt(dst, ff.At)
	if !ok {
		// codecutils.FieldAt could not make a pointer on the path. That happens
		// when an embedded pointer is unexported. encoding/json rejects the
		// same shape.
		return located{}, fmt.Errorf(
			"cannot set embedded pointer to unexported struct on the way to %q in %s",
			name, dst.Type(),
		)
	}

	return located{value: field, sf: ff.SF, ord: ff.Ord, at: ff.At}, nil
}

// rangeMapEntries reads the mapping named by src and calls visit on its entries.
// It visits the own entries first, in document order, then the entries merged by each "<<",
// the earliest "<<" first.
//
// The order gives the precedence, so rangeMapEntries collects nothing.
// A field written by the mapping itself is set before any merged entry reaches it,
// and an earlier "<<" is read before a later one, as the YAML 1.1
// merge spec requires. The caller skips a merged entry whose field is already set.
func (d *Decoder) rangeMapEntries(src ast.Node, ignoreMergeKey bool, visit mapEntryFunc) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(src, false)
	if err != nil {
		return err
	}
	// A struct names its fields, so two keys of one name are one field.
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping {
		if err := astutils.RefuseRepeats(m, true); err != nil {
			return err
		}
	}

	var merges []ast.Node

	mapIter := mapNode.MapRange()
	for mapIter.Next() {
		keyNode := mapIter.Key()
		if keyNode.IsMergeKey() {
			if !ignoreMergeKey {
				merges = append(merges, mapIter.Value())
			}

			continue
		}

		name, named, err := d.entryName(keyNode)
		if err != nil {
			return err
		}
		if !named {
			continue
		}
		if err := visit(name, keyNode, mapIter.Value(), mapIter.KeyValue(), false); err != nil {
			return err
		}
	}

	for _, from := range merges {
		if err := d.rangeMergedEntries(from, ignoreMergeKey, visit); err != nil {
			return err
		}
	}

	return nil
}

// rangeMergedEntries calls visit on the entries of the node named by a "<<". That node is a
// mapping or a sequence of them, and each mapping is read whole before the next. See
// eachMergeSource.
func (d *Decoder) rangeMergedEntries(src ast.Node, ignoreMergeKey bool, visit mapEntryFunc) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	return d.eachMergeSource(src, func(mapNode ast.MapNode) error {
		var merges []ast.Node

		mapIter := mapNode.MapRange()
		for mapIter.Next() {
			keyNode := mapIter.Key()
			if keyNode.IsMergeKey() {
				if !ignoreMergeKey {
					merges = append(merges, mapIter.Value())
				}

				continue
			}

			name, named, err := d.entryName(keyNode)
			if err != nil {
				return err
			}
			if !named {
				continue
			}
			if err := visit(name, keyNode, mapIter.Value(), mapIter.KeyValue(), true); err != nil {
				return err
			}
		}

		for _, from := range merges {
			if err := d.rangeMergedEntries(from, ignoreMergeKey, visit); err != nil {
				return err
			}
		}

		return nil
	})
}

// entryName returns the name of the entry for a mapping key. named is false
// for a key that cannot name a field, such as "[a]:".
//
// [ast.KeyName] writes the canonical spelling of the type, so "true: x" addresses a
// field tagged "true" and "1.0: x" one tagged "1.0". The same
// document reads that way into a map[string]any. Requiring the key to be an ast string
// dropped every one of those instead.
func (d *Decoder) entryName(keyNode ast.Node) (string, bool, error) {
	if tag := keyTag(keyNode); tag != nil && !tag.IsMergeKey() {
		// ast.KeyName names a key by its text, which leaves the tag unread: "!!float x: 1" named the field "x",
		// where a map refuses the key with `cannot read "x" as !!float`. The key is read as a map's key is.
		if _, err := d.nodeToValue(tag); err != nil {
			return "", false, err
		}
	}
	name, kind := ast.KeyName(keyNode)

	return name, kind != token.KeyOther, nil
}

// keyTag returns the tag written on a mapping key, under the "?" and the anchor that may stand around it, or nil.
// A "!!merge" is one too, and the callers leave it to the merge.
func keyTag(key ast.Node) *ast.TagNode {
	for {
		switch n := key.(type) {
		case *ast.MappingKeyNode:
			key = n.Value
		case *ast.AnchorNode:
			key = n.Value
		case *ast.TagNode:
			return n
		default:
			return nil
		}
	}
}

// decodeInlineFields reads the three kinds of `,inline` field that an index path
// cannot place. It reads each from the whole mapping instead of from one entry.
//
// An ordinary embedded struct is not one of them. structfields.Fields.flat names every
// field it promotes, and setField writes each entry straight into its field,
// so the two decode paths place an entry the same way. The three kinds are:
//   - a map, which takes the entries claimed by no field, and so is only known
//     when the mapping ends;
//   - a field carrying `,alias`, which takes the value named by the "<<" of the document;
//   - a field whose type reads its own node. go.yaml.in/yaml/v3 also passes such a
//     field the whole mapping.
//
// A `,inline` map receives the entries not named by structfields.Fields.flat, which names the
// own fields of the struct and every field promoted by its embedded structs. With nothing
// left over, the map stays nil, as v3 leaves it.
func (d *Decoder) decodeInlineFields(
	dst reflect.Value, src ast.Node, fields *structfields.Fields,
	ignoreMergeKey bool, unknownFields map[string]ast.Node, foundErr *error,
) error {
	structType := dst.Type()
	aliasName := d.getMergeAliasName(src)

	var entries map[string]ast.Node

	for _, sf := range fields.Inline {
		fieldValue := dst.Field(sf.Index)
		if sf.IsAutoAlias {
			if err := d.setAliasedField(fieldValue, aliasName); err != nil {
				return err
			}

			continue
		}

		fieldType := fieldValue.Type()
		takesUnclaimed := structfields.InlineTakesUnclaimedEntries(fieldType)
		if !takesUnclaimed && !structfields.InlineReadsItsOwnNode(fieldType) {
			// An embedded struct. setField has already filled its fields.
			continue
		}
		if !fieldValue.CanSet() {
			field := structType.Field(sf.Index)

			return fmt.Errorf("cannot set embedded type as unexported field %s.%s", field.PkgPath, field.Name)
		}
		if fieldValue.Type().Kind() == reflect.Pointer && ast.IsNull(src) {
			// Set the pointer to nil.
			fieldValue.Set(reflect.Zero(fieldValue.Type()))

			continue
		}

		if entries == nil {
			var err error
			if entries, err = d.keyToValueNodeMap(src, ignoreMergeKey); err != nil {
				return err
			}
		}
		mapNode := inlineMapNode(entries, fields, takesUnclaimed)
		if takesUnclaimed && len(mapNode.Values) == 0 {
			// Every entry went to a field, so the map stays nil instead of
			// becoming an empty one. A caller can then tell a mapping that
			// wrote nothing else from one that wrote nothing at all.
			continue
		}

		newFieldValue, err := d.createDecodedNewValue(fieldValue.Type(), fieldValue, mapNode)
		if unknownFields != nil {
			if err := d.deleteStructKeys(fieldValue.Type(), unknownFields); err != nil {
				return err
			}
		}
		codecutils.SetDecoded(fieldValue, newFieldValue)
		if stopsTheDocument(err) {
			return err
		}
		if err != nil {
			recordFieldError(foundErr, err, func(te *yamlerrors.Error) string {
				leaf := te.StructField()
				if leaf == "" {
					leaf = sf.FieldName
				}

				return structType.Name() + "." + leaf
			})
		}
	}

	return nil
}

// setAliasedField sets a field carrying `,alias` to the value of the anchor named by the "<<" of the mapping.
// It leaves the field as it is when the mapping merges no alias, or the anchor holds no value.
func (d *Decoder) setAliasedField(fieldValue reflect.Value, aliasName string) error {
	if aliasName == "" {
		return nil
	}
	anchored := d.anchorValueMap[aliasName]
	if !anchored.IsValid() {
		return nil
	}
	value, err := codecutils.CastToAssignableValue(anchored, fieldValue.Type(), d.anchorNodeMap[aliasName])
	if err != nil {
		return err
	}
	fieldValue.Set(value)

	return nil
}

// inlineMapNode builds the mapping decoded by an inline field. It holds every entry, or with takesUnclaimed only
// the entries claimed by no field of fields.
func inlineMapNode(entries map[string]ast.Node, fields *structfields.Fields, takesUnclaimed bool) *ast.MappingNode {
	mapNode := ast.Mapping(nil, false)
	for k, v := range entries {
		if takesUnclaimed && fields.Claims(k) {
			continue
		}
		key := &ast.StringNode{Value: k}
		mapNode.Values = append(mapNode.Values, ast.MappingValue(nil, key, v))
	}

	return mapNode
}

// stopsTheDocument reports whether err ends the decoding of its document, at any depth. These errors stop it:
//   - a repeated key, which makes the document invalid YAML. The walk finds it when the mapping closes,
//     so the mapping's own entries are decoded and the stop applies to what comes after it.
//     The tree checks before it decodes the mapping;
//   - the limits, ErrExceededMaxDepth and the alias budget;
//   - an unknown field, when the caller set DisallowUnknownField.
//
// Any other error is a field error. The decoder keeps the first one and decodes the fields after it.
//
// The decoder returns ErrExceededMaxDepth only for a tree built by hand.
// The parser returns it for a document nested past its cap, and like any parse error it stops the stream.
func stopsTheDocument(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, yamlerrors.ErrDuplicateKey) ||
		errors.Is(err, yamlerrors.ErrExceededMaxDepth) ||
		errors.Is(err, yamlerrors.ErrExcessiveAliasing) ||
		errors.Is(err, yamlerrors.ErrRecursiveAlias) ||
		errors.Is(err, yamlerrors.ErrUnknownAnchor) ||
		errors.Is(err, yamlerrors.ErrUnknownField)
}

// recordFieldError keeps err in foundErr unless an earlier field error is there already. It tags a type mismatch
// with the struct field returned by fieldName.
func recordFieldError(foundErr *error, err error, fieldName func(*yamlerrors.Error) string) {
	if *foundErr != nil {
		return
	}
	var te *yamlerrors.Error
	if errors.As(err, &te) && errors.Is(te, yamlerrors.ErrTypeMismatch) {
		te.SetStructField(fieldName(te))
		*foundErr = te

		return
	}
	*foundErr = err
}

// findEntry returns the value written by a mapping under name, and the key of the entry that
// writes it. Both are nil when the mapping does not write name.
//
// findEntry walks the mapping instead of reading a map built up front. Only a
// document that fails validation calls it, and only for the field that failed.
func (d *Decoder) findEntry(src ast.Node, ignoreMergeKey bool, name string) (value, key ast.Node) {
	_ = d.rangeMapEntries(src, ignoreMergeKey,
		func(entryName string, keyNode, valueNode, _ ast.Node, _ bool) error {
			if value == nil && entryName == name {
				value, key = valueNode, keyNode
			}

			return nil
		})

	return value, key
}

// getMergeAliasName returns the name of the alias under a "<<" of src.
// It supports a single alias only.
func (d *Decoder) getMergeAliasName(src ast.Node) string {
	mapNode, err := d.getMapNode(src, true)
	if err != nil {
		return ""
	}
	mapIter := mapNode.MapRange()
	for mapIter.Next() {
		key := mapIter.Key()
		value := mapIter.Value()
		if alias, isAlias := value.(*ast.AliasNode); isAlias && key.IsMergeKey() {
			return alias.Value.GetToken().Value
		}
	}
	return ""
}

func (d *Decoder) decodeStruct(dst reflect.Value, src ast.Node) error {
	if src == nil || ast.IsNull(src) {
		// A null writes nothing, so the struct keeps its values. The null of a
		// field never gets here, because createDecodedNewValue reads it first. A
		// document that is only "~" does get here.
		return nil
	}
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	structType := dst.Type()
	srcValue := reflect.ValueOf(src)
	srcType := srcValue.Type()
	if srcType.Kind() == reflect.Pointer {
		srcType = srcType.Elem()
		srcValue = srcValue.Elem()
	}
	if structType == srcType {
		// The dst value implements ast.Node.
		dst.Set(srcValue)
		return nil
	}
	fields, err := structfields.Of(structType, d.tagMode())
	if err != nil {
		return err
	}
	ignoreMergeKey := fields.ByGoName.HasMergeProperty()

	var unknownFields map[string]ast.Node
	if d.disallowUnknownField {
		unknownFields = map[string]ast.Node{}
	}

	var (
		written  writtenFields
		foundErr error
	)

	// Walk the document and look up the field of each entry, instead of walking the
	// fields and looking up each entry. The decoder then holds only the fields of the struct,
	// read once per type and shared, and no map of every entry
	// written by the mapping.
	setField := func(name string, keyNode, valueNode, _ ast.Node, merged bool) error {
		found, err := fieldFor(dst, fields, name)
		if err != nil {
			return err
		}
		if found.sf == nil {
			if unknownFields != nil {
				if _, seen := unknownFields[name]; !seen {
					unknownFields[name] = keyNode
				}
			}

			return nil
		}
		if merged && written.has(found.ord) {
			// The mapping writes this field itself, or an earlier "<<" does.
			return nil
		}
		written.mark(found.ord)

		fieldValue := found.value
		if fieldValue.Type().Kind() == reflect.Pointer && ast.IsNull(src) {
			// Set the pointer to nil.
			fieldValue.Set(reflect.Zero(fieldValue.Type()))

			return nil
		}

		prevEntry := d.enterEntry(keyNode.GetToken())
		newFieldValue, err := d.createDecodedNewValue(fieldValue.Type(), fieldValue, valueNode)
		d.entryKey = prevEntry
		codecutils.SetDecoded(fieldValue, newFieldValue)
		if stopsTheDocument(err) {
			return err
		}
		if err != nil {
			// Only the first field error is kept, and decoding goes on, as encoding/json does.
			recordFieldError(&foundErr, err, func(*yamlerrors.Error) string { return found.name(structType) })
		}

		return nil
	}

	if err := d.rangeMapEntries(src, ignoreMergeKey, setField); err != nil {
		return err
	}

	return d.finishStruct(dst, src, fields, ignoreMergeKey, unknownFields, foundErr)
}

// finishStruct completes a struct once its own entries are read. In this order, it decodes the `,inline` fields,
// returns the first field error, rejects an unknown field under DisallowUnknownField, and runs the validator.
func (d *Decoder) finishStruct(
	dst reflect.Value, src ast.Node, fields *structfields.Fields, ignoreMergeKey bool,
	unknownFields map[string]ast.Node, foundErr error,
) error {
	if len(fields.Inline) > 0 {
		if err := d.decodeInlineFields(dst, src, fields, ignoreMergeKey, unknownFields, &foundErr); err != nil {
			return err
		}
	}

	if foundErr != nil {
		return foundErr
	}

	// Ignore unknown fields when parsing an inline struct, recognized by a nil token.
	// Unknown fields are expected there, because they may be fields of the parent struct.
	if d.disallowUnknownField && src.GetToken() != nil {
		if err := d.refuseUnknownFields(unknownFields); err != nil {
			return err
		}
	}

	return d.validateStruct(dst, src, fields, ignoreMergeKey)
}

// refuseUnknownFields returns an error for the unknown field written first in the document, among those whose name
// carries none of the allowed field prefixes. The typed walk reports the same field.
func (d *Decoder) refuseUnknownFields(unknownFields map[string]ast.Node) error {
	var (
		first     string
		firstNode ast.Node
	)
	for key, node := range unknownFields {
		allowed := slices.ContainsFunc(d.allowedFieldPrefixes, func(prefix string) bool {
			return strings.HasPrefix(key, prefix)
		})
		if allowed {
			continue
		}
		if firstNode == nil || node.GetToken().Position.Offset() < firstNode.GetToken().Position.Offset() {
			first, firstNode = key, node
		}
	}
	if firstNode == nil {
		return nil
	}

	return yamlerrors.NewUnknownField(fmt.Sprintf(`unknown field "%s"`, first), firstNode.GetToken())
}

// validateStruct runs the validator of the decoder on dst, once src is decoded into it.
//
// validateStruct places a validation error at the field's value, or at the key that writes it for a mapping
// or a sequence. It reads the entries here instead of during the decode, because a document that validates
// never needs them.
func (d *Decoder) validateStruct(dst reflect.Value, src ast.Node, fields *structfields.Fields, ignoreMergeKey bool) error {
	if d.validator == nil {
		return nil
	}
	err := d.validator(dst.Interface())
	if err == nil {
		return nil
	}

	return validationError(err, fields, treePlaces{d: d, src: src, ignoreMergeKey: ignoreMergeKey})
}

// validationPlaces returns the tokens at which to place the error of a validator on one struct.
type validationPlaces interface {
	// field returns where to place an error about the field rendered as name, or nil when the mapping
	// does not write it.
	field(name string) *token.Token
	// missing returns where to place an error about a field the mapping leaves out.
	missing() *token.Token
	// whole returns the token of the struct's mapping.
	whole() *token.Token
}

// validationError returns the error of a validator, placed by places.
//
// An error that is a slice of fieldError values names the struct fields that failed, as go-playground/validator
// does. The first element that names a field of the struct places the error: at the field's entry when the
// mapping writes it, and at places.missing otherwise. Any other error goes to the struct's mapping.
func validationError(err error, fields *structfields.Fields, places validationPlaces) error {
	ev := reflect.ValueOf(err)
	if ev.Kind() != reflect.Slice {
		// The error names no field, so it goes to the mapping of the struct.
		return yamlerrors.NewValidation(err, places.whole())
	}
	for i := range ev.Len() {
		fieldErr, ok := reflect.TypeAssert[fieldError](ev.Index(i))
		if !ok {
			continue
		}
		renderName, exists := fields.RenderNameOf(fieldErr.StructField())
		if !exists {
			continue
		}
		if t := places.field(renderName); t != nil {
			return yamlerrors.NewValidation(err, t)
		}
		if t := places.missing(); t != nil {
			// A missing field has no entry of its own, so the error goes to
			// the mapping that should have held it.
			return yamlerrors.NewValidation(err, t)
		}
	}

	// No element names a field written by the document or held by the struct.
	return yamlerrors.NewValidation(err, places.whole())
}

// treePlaces places a validation error on the tree, by reading the struct's mapping again.
type treePlaces struct {
	d              *Decoder
	src            ast.Node
	ignoreMergeKey bool
}

func (p treePlaces) field(name string) *token.Token {
	value, key := p.d.findEntry(p.src, p.ignoreMergeKey, name)
	if value == nil {
		return nil
	}

	return astutils.ValidationErrorToken(value, key.GetToken())
}

func (p treePlaces) missing() *token.Token { return p.d.missingFieldToken(p.src) }

func (p treePlaces) whole() *token.Token { return p.src.GetToken() }

// missingFieldToken returns the token at which to place the error for a field missing from the document.
//
// The field has no entry to point at. missingFieldToken returns d.entryKey when set: the key of the entry
// that writes the struct. Otherwise it returns the first key of the mapping that should have held the field.
func (d *Decoder) missingFieldToken(src ast.Node) *token.Token {
	if d.entryKey != nil {
		return d.entryKey
	}
	if m, ok := src.(*ast.MappingNode); ok && len(m.Values) > 0 {
		return m.Values[0].Key.GetToken()
	}

	return src.GetToken()
}

// enterEntry records the key token of the entry that writes the node about to be decoded. It returns the token it
// replaces, for the caller to put back.
func (d *Decoder) enterEntry(key *token.Token) *token.Token {
	prev := d.entryKey
	d.entryKey = key

	return prev
}

func (d *Decoder) decodeArray(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	if b, ok := d.binaryBytes(src); ok && codecutils.ByteArray(dst.Type()) {
		// Read "!!binary" into a byte array, as decodeSlice reads one into a byte
		// slice. The text is a scalar, so the array reader below rejected it
		// with "string was used where sequence is expected".
		return codecutils.CopyIntoByteArray(dst, b, src)
	}

	arrayNode, err := d.getArrayNode(src)
	if err != nil {
		return err
	}
	if arrayNode == nil {
		return nil
	}
	iter := arrayNode.ArrayRange()
	arrayValue := reflect.New(dst.Type()).Elem()
	arrayType := dst.Type()
	elemType := arrayType.Elem()
	idx := 0

	var foundErr error
	for iter.Next() {
		v := iter.Value()
		if idx == arrayType.Len() {
			// The array holds no more. The entries it holds are written, and the rest are dropped, as
			// encoding/json drops them. Indexing past the end panicked.
			if foundErr == nil {
				foundErr = yamlerrors.NewOverflow(arrayType, fmt.Sprintf("a sequence of %d entries", iter.Len()), v.GetToken())
			}

			break
		}
		if elemType.Kind() == reflect.Pointer && ast.IsNull(v) {
			// Set the pointer to nil.
			arrayValue.Index(idx).Set(reflect.Zero(elemType))
		} else {
			dstValue, err := d.createDecodedNewValue(elemType, reflect.Value{}, v)
			if stopsTheDocument(err) {
				return err
			}
			if err != nil && foundErr == nil {
				foundErr = err
			}
			if dstValue.IsValid() {
				arrayValue.Index(idx).Set(dstValue)
			}
		}
		idx++
	}
	dst.Set(arrayValue)
	if foundErr != nil {
		return foundErr
	}
	return nil
}

func (d *Decoder) decodeSlice(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	if b, ok := d.binaryBytes(src); ok && codecutils.ByteSlice(dst.Type()) {
		dst.Set(reflect.ValueOf(b).Convert(dst.Type()))

		return nil
	}

	arrayNode, err := d.getArrayNode(src)
	if err != nil {
		return err
	}
	if arrayNode == nil {
		return nil
	}
	iter := arrayNode.ArrayRange()
	sliceType := dst.Type()
	sliceValue := reflect.MakeSlice(sliceType, 0, iter.Len())
	elemType := sliceType.Elem()

	var foundErr error
	for iter.Next() {
		v := iter.Value()
		if elemType.Kind() == reflect.Pointer && ast.IsNull(v) {
			// Set the pointer to nil.
			sliceValue = reflect.Append(sliceValue, reflect.Zero(elemType))
			continue
		}
		prevEntry := d.enterEntry(nil)
		dstValue, err := d.createDecodedNewValue(elemType, reflect.Value{}, v)
		d.entryKey = prevEntry
		if stopsTheDocument(err) {
			return err
		}
		if err != nil && foundErr == nil {
			foundErr = err
		}
		if !dstValue.IsValid() {
			// The element decoded to nothing at all. Keep its place.
			dstValue = reflect.Zero(elemType)
		}
		sliceValue = reflect.Append(sliceValue, dstValue)
	}
	dst.Set(sliceValue)
	if foundErr != nil {
		return foundErr
	}
	return nil
}

func (d *Decoder) decodeMapItem(dst *MapItem, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(src, d.merging)
	if err != nil {
		return err
	}
	mapIter := mapNode.MapRange()
	if !mapIter.Next() {
		return nil
	}
	key := mapIter.Key()
	value := mapIter.Value()
	if key.IsMergeKey() {
		leave := d.enterMerge()
		err := d.decodeMapItem(dst, value)
		leave()
		if err != nil {
			return err
		}
		return nil
	}
	k, err := d.nodeToValue(key)
	if err != nil {
		return err
	}
	v, err := d.nodeToValue(value)
	if err != nil {
		return err
	}
	*dst = MapItem{Key: k, Value: v}
	return nil
}

// admitMapKey checks k, the decoded key of an entry about to go into mapValue.
// It reports whether the map already holds k under a folded key. The caller then
// skips the entry.
//
// An invalid k, a null key, passes unchecked. Otherwise k must be comparable.
// Before, a sequence or a mapping used as a key, into a map whose key type admits one,
// made SetMapIndex panic with "hash of unhashable type" instead of returning an error
// for the document.
func (d *Decoder) admitMapKey(mapValue reflect.Value, keyMap map[any]struct{}, k reflect.Value, key ast.Node, folded bool) (bool, error) {
	if !k.IsValid() {
		return false, nil
	}
	if !k.Comparable() {
		return false, yamlerrors.NewUnhashableKey(codecutils.DynamicTypeOf(k), key.GetToken())
	}
	if folded {
		return mapValue.MapIndex(k).IsValid(), nil
	}

	return false, d.validateDuplicateKey(keyMap, k.Interface(), key)
}

// validateDuplicateKey rejects two entries that land on one key of the Go
// destination. The parse does not see them: "1: a" and "\"1\": b" are two YAML
// keys and one key of a map[string]T, and "1: a" and "1.0: b" are two YAML keys
// and one key of a map[float64]T.
//
// Every comparable key counts, not only a string. Before, a float-keyed destination
// kept the last of two keys that resolve to one float, and dropped the other
// with no error. Under AllowDuplicateMapKey the last one wins, as for
// a repeat recorded by the parse.
func (d *Decoder) validateDuplicateKey(keyMap map[any]struct{}, key any, keyNode ast.Node) error {
	if key != nil && !reflect.TypeOf(key).Comparable() {
		return nil
	}
	if !d.allowDuplicateMapKey {
		if _, exists := keyMap[key]; exists {
			return yamlerrors.NewDuplicateKey(fmt.Sprintf(`duplicate key "%v"`, key), keyNode.GetToken())
		}
	}
	keyMap[key] = struct{}{}
	return nil
}

// decodeMapSlice reads a mapping into dst.
//
// It reads the mapping as nodeToValue reads one under [UseOrderedMap], through
// setToOrderedMapValue, so the two destinations agree about a merge. Before, they did
// not. This loop walked the entries in document order, appended each one, and
// asked validateDuplicateKey whether the key had been seen. That is true of
// every key overridden by a "<<". So "x: 9" under a "<<: *a" that also writes x was
// rejected here as a duplicate key, and read as 9 into an any. A merge
// sequence whose mappings share a key was rejected on the name of the anchored
// mapping. Defect 40 fixed the same fault for a Go map. decodeMapSlice is the fifth place
// that resolves a merge.
func (d *Decoder) decodeMapSlice(dst *MapSlice, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	if seq, entries, ordered := ast.OrderedMapOf(src); ordered {
		one, err := d.orderedMapOf(seq, entries)
		if err != nil {
			return err
		}
		*dst = MapSlice(one)

		return nil
	}

	mapNode, err := d.getMapNode(src, d.merging)
	if err != nil {
		return err
	}

	var m MapSlice
	switch n := mapNode.(type) {
	case *ast.MappingNode:
		if len(n.Values) > 0 {
			m.items = make([]MapItem, 0, len(n.Values))
		}
		if err := d.setToOrderedMapValue(n, &m, d.merging); err != nil {
			return err
		}
	case *ast.MappingValueNode:
		if err := d.setToOrderedMapValue(n, &m, d.merging); err != nil {
			return err
		}
	}
	*dst = m

	return nil
}

// decodeOrderedMapSeq reads a mapping or an "!!omap" into dst.
//
// The destination sets the spelling, so a plain mapping read into a
// MapSliceSeq writes back as an "!!omap" and an "!!omap" read into a MapSlice
// writes back as a plain mapping.
func (d *Decoder) decodeOrderedMapSeq(dst *MapSliceSeq, src ast.Node) error {
	m, err := d.orderedEntriesOf(src)
	if err != nil {
		return err
	}
	*dst = MapSliceSeq(m)

	return nil
}

// orderedEntriesOf reads the entries of a mapping or of an "!!omap".
func (d *Decoder) orderedEntriesOf(src ast.Node) (MapSlice, error) {
	if seq, entries, ordered := ast.OrderedMapOf(src); ordered {
		one, err := d.orderedMapOf(seq, entries)

		return MapSlice(one), err
	}

	var m MapSlice
	err := d.decodeMapSlice(&m, src)

	return m, err
}

// decodeMap reads the mapping named by src into dst.
//
// A map already held by dst keeps its entries, and decodeMap writes the entries of the mapping
// over them, as encoding/json, go.yaml.in/yaml/v3 and the
// walk do. A merged entry is written over an entry of the caller too, as it is
// over the default of a struct field. Only a key written by the mapping or by an earlier "<<"
// stops it.
func (d *Decoder) decodeMap(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return yamlerrors.ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(src, d.merging)
	if err != nil {
		return err
	}
	mapType := dst.Type()
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping && !d.merging && mapType.Key().Kind() == reflect.String {
		// A string-keyed map holds one entry for two keys of one name. admitMapKey
		// checks a key of another type, on its decoded value:
		// 1 and 1.0 are two names and one float64.
		if err := astutils.RefuseRepeats(m, true); err != nil {
			return err
		}
	}
	mapValue := dst
	if mapValue.IsNil() {
		mapValue = reflect.MakeMap(mapType)
	}
	keyType := mapValue.Type().Key()
	valueType := mapValue.Type().Elem()
	// keyMap records the keys written by this mapping, its own and the merged
	// ones. A later merge must skip them. The keys held before in the map of the
	// caller are not in keyMap.
	keyMap := map[any]struct{}{}
	// A mapping read under a merge is the fold made by getMapNode of "<<: [a, b]".
	// Two equal keys in it come from the sequence, where the earlier
	// mapping wins. They are not the repeated key rejected by 3.2.1.1.
	folded := d.merging
	var foundErr error
	for _, entry := range astutils.MapEntriesOwnFirst(mapNode) {
		key := entry.Key
		value := entry.Value
		if entry.Merged {
			// The merge type gives the own keys of a mapping precedence over the
			// keys brought in by a "<<", and an earlier "<<" precedence over a
			// later one. mergeIntoMap gets both by writing only keys not yet written.
			// The own entries went in first, and the merges are read in
			// document order.
			//
			// This branch does not call validateDuplicateKey. A merge sequence
			// lets two of its mappings define one key, and a
			// merged key equal to an own key is the override defined by the type.
			// Neither is the repeated key rejected by 3.2.1.1.
			if err := d.mergeIntoMap(mapValue, keyMap, value); err != nil {
				return err
			}

			continue
		}

		k, decodeKeyAs, err := d.decodeMapKey(keyType, key)
		if err != nil {
			return err
		}

		if seen, err := d.admitMapKey(mapValue, keyMap, k, key, folded); err != nil {
			return err
		} else if seen {
			continue
		}
		if valueType.Kind() == reflect.Pointer && ast.IsNull(value) {
			// Set the pointer to nil.
			mapValue.SetMapIndex(k, reflect.Zero(valueType))
			continue
		}
		prevEntry := d.enterEntry(key.GetToken())
		dstValue, err := d.createDecodedNewValue(valueType, reflect.Value{}, value)
		d.entryKey = prevEntry
		if stopsTheDocument(err) {
			return err
		}
		if err != nil && foundErr == nil {
			foundErr = err
		}
		if !dstValue.IsValid() {
			// SetMapIndex deletes the key for an invalid value, and that removed the
			// entry of the caller.
			continue
		}
		if !k.IsValid() {
			// Expect a nil key.
			nilKey := d.createDecodableValue(decodeKeyAs)
			keyMap[nilKey.Interface()] = struct{}{}
			mapValue.SetMapIndex(nilKey, dstValue)
			continue
		}
		if decodeKeyAs.Kind() != k.Kind() {
			return yamlerrors.NewSyntax(
				fmt.Sprintf("cannot convert %q type to %q type", k.Kind(), keyType.Kind()),
				key.GetToken(),
			)
		}
		mapValue.SetMapIndex(k, dstValue)
	}
	dst.Set(mapValue)

	return foundErr
}

// mergeIntoMap decodes value, the mapping brought in by a "<<", into a map of its own. It copies into mapValue
// each entry whose key is not yet in written, and records the key in written.
//
// The merged mapping is decoded apart because mapValue may be the map of the caller, which decodeMap writes into.
// Decoding the merge straight into it would write over the own entries of the mapping.
//
// Each mapping of a merge sequence is decoded whole before the next, so an
// earlier one wins with the entries of its own "<<". See eachMergeSource.
func (d *Decoder) mergeIntoMap(mapValue reflect.Value, written map[any]struct{}, value ast.Node) error {
	return d.eachMergeNode(value, func(source ast.Node, inSequence bool) error {
		merged := reflect.New(mapValue.Type()).Elem()
		leave := func() {}
		if !inSequence {
			// A sequence here belongs to the merge, and decodeMap reads it
			// only under a merge.
			leave = d.enterMerge()
		}
		err := d.decodeMap(merged, source)
		leave()
		if err != nil {
			return err
		}
		iter := merged.MapRange()
		for iter.Next() {
			key := iter.Key().Interface()
			if _, wrote := written[key]; wrote {
				continue
			}
			written[key] = struct{}{}
			mapValue.SetMapIndex(iter.Key(), iter.Value())
		}

		return nil
	})
}

// decodeMapKey decodes key into a map whose keys are of keyType, and returns the key with the type it was decoded as.
// Under UseStringKeys a map that takes any key reads its keys as strings.
func (d *Decoder) decodeMapKey(keyType reflect.Type, key ast.Node) (reflect.Value, reflect.Type, error) {
	decodeKeyAs := keyType
	if d.useStringKeys && keyType.Kind() == reflect.Interface {
		// The map takes any key, so a scalar would otherwise arrive with
		// the Go type its text resolves to. Read it as a string instead,
		// through the same path as a map[string]any, so the two agree
		// on the spelling.
		decodeKeyAs = reflect.TypeFor[string]()
	}
	k := d.createDecodableValue(decodeKeyAs)
	switch {
	case d.canDecodeByUnmarshaler(k):
		if err := d.decodeByUnmarshaler(k, key); err != nil {
			return reflect.Value{}, nil, err
		}
	case decodeKeyAs.Kind() == reflect.String:
		name, err := d.stringMapKey(key)
		if err != nil {
			return reflect.Value{}, nil, err
		}
		k = reflect.ValueOf(name).Convert(decodeKeyAs)
	default:
		keyVal, err := d.createDecodedNewValue(decodeKeyAs, reflect.Value{}, key)
		if err != nil {
			return reflect.Value{}, nil, err
		}
		k = keyVal
	}

	return k, decodeKeyAs, nil
}

// stringMapKey returns the name under which a map keyed by a string kind stores key.
//
// It reads the key as nodeToValue reads it for the map[string]any
// it builds, so a document decoded into either gives the same keys.
// Decoding a null through createDecodedNewValue gave "", the Go
// zero value, which collides with the empty key. "null: a" and "\"\": b"
// are two entries and were read as one, so the document was rejected
// as holding a duplicate key.
func (d *Decoder) stringMapKey(key ast.Node) (string, error) {
	keyValue, err := d.nodeToValue(key)
	if err != nil {
		return "", err
	}
	if _, named := ast.KeyName(key); named == token.KeyOther &&
		keyValue != nil && !reflect.TypeOf(keyValue).Comparable() {
		// A sequence or a mapping used as a key. astutils.MapKeyString would
		// give it the spelling printed by Go, "[a b]", which no reader
		// takes apart again.
		//
		// The destination is keyed by a string, so the check asks
		// whether the key can be named, not whether its resolved value
		// is comparable. ast.KeyName names every scalar,
		// and returns token.KeyOther for a collection alone.
		// For a collection, astutils.MapKeyString falls back to Go printing. Checking
		// comparability instead rejected "!!binary aGVsbG8=: x", whose
		// name is the base64 written in the document. The same document
		// read into an any gives {"aGVsbG8=": "x"}, so the two
		// destinations disagreed over a []byte that neither had to hash.
		return "", yamlerrors.NewUnhashableKey(reflect.TypeOf(keyValue), key.GetToken())
	}
	name, err := astutils.MapKeyString(key, keyValue)
	if err != nil {
		return "", err
	}
	return d.strs.Clone(name), nil
}

func (d *Decoder) fileToReader(file string) (io.Reader, error) {
	reader, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

func (d *Decoder) isYAMLFile(file string) bool {
	ext := filepath.Ext(file)
	if ext == ".yml" {
		return true
	}
	if ext == ".yaml" {
		return true
	}
	return false
}

func (d *Decoder) readersUnderDir(dir string) ([]io.Reader, error) {
	pattern := dir + "/*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	readers := []io.Reader{}
	for _, match := range matches {
		if !d.isYAMLFile(match) {
			continue
		}
		reader, err := d.fileToReader(match)
		if err != nil {
			return nil, err
		}
		readers = append(readers, reader)
	}
	return readers, nil
}

func (d *Decoder) readersUnderDirRecursive(dir string) ([]io.Reader, error) {
	readers := []io.Reader{}
	if err := filepath.Walk(dir, func(path string, _ os.FileInfo, _ error) error {
		if !d.isYAMLFile(path) {
			return nil
		}
		reader, readerErr := d.fileToReader(path)
		if readerErr != nil {
			return readerErr
		}
		readers = append(readers, reader)
		return nil
	}); err != nil {
		return nil, err
	}
	return readers, nil
}

func (d *Decoder) resolveReference() error {
	for _, opt := range d.opts {
		if err := opt(d); err != nil {
			return err
		}
	}
	for _, file := range d.referenceFiles {
		reader, err := d.fileToReader(file)
		if err != nil {
			return err
		}
		d.referenceReaders = append(d.referenceReaders, reader)
	}
	for _, dir := range d.referenceDirs {
		if !d.isRecursiveDir {
			readers, err := d.readersUnderDir(dir)
			if err != nil {
				return err
			}
			d.referenceReaders = append(d.referenceReaders, readers...)
		} else {
			readers, err := d.readersUnderDirRecursive(dir)
			if err != nil {
				return err
			}
			d.referenceReaders = append(d.referenceReaders, readers...)
		}
	}
	for _, reader := range d.referenceReaders {
		bytes, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		// Assign the new anchor definitions to anchorMap.
		if _, err := d.parse(bytes); err != nil {
			return err
		}

		// A reference file exists to publish its anchors. They become the
		// base for every later file and every document of the input.
		// The documents of a reference file are not part of the input, so resolveReference
		// merges the anchor maps left by the parse here, instead of indexing them by stream
		// position later.
		for _, m := range d.anchorNodeMaps {
			maps.Copy(d.referenceAnchorNodeMap, m)
		}
		d.anchorNodeMaps = nil
		d.commentMaps = nil
	}
	d.isResolvedReference = true
	return nil
}

// parserOptions returns the parser options set by the options of this decoder.
func (d *Decoder) parserOptions() []parser.Option {
	var opts []parser.Option
	if d.toCommentMap != nil {
		opts = append(opts, parser.WithComments())
	}
	if d.allowDuplicateMapKey {
		opts = append(opts, parser.WithAllowDuplicateMapKey())
	}
	if len(d.referenceAnchorNodeMap) > 0 {
		// The parser rejects an alias that names no anchor of its own document.
		// A reference file exists so that another document may name its
		// anchors. parser.WithAnchors publishes them, so the parser accepts that alias.
		opts = append(opts, parser.WithAnchors(d.referenceAnchorNodeMap))
	}

	// Append these last, so that a parser option set directly by the caller overrides
	// one set on its behalf by a decode option.
	opts = append(opts, d.extraParserOptions...)

	return opts
}

// parse reads bytes into a tree of the documents that hold a value.
//
// On a parse error it returns the error with the documents read before the broken one. It returns a nil file only
// when the parser read none. Each document stands on its own, so a Decoder still decodes those.
func (d *Decoder) parse(bytes []byte) (*ast.File, error) {
	f, parseErr := parser.ParseBytes(bytes, d.parserOptions()...)
	if f == nil {
		return nil, parseErr
	}
	normalizedFile := &ast.File{}
	for _, doc := range f.Docs {
		if !doc.HoldsValue() {
			continue
		}
		normalizedFile.Docs = append(normalizedFile.Docs, doc)

		// An anchor belongs to the document it was written in. A stream is a run
		// of documents, each independent of the rest, so each gets its own anchor map.
		// An alias naming an anchor of an earlier document finds nothing.
		// The maps are kept beside the documents, as the comment maps are, because parse
		// runs over the whole stream and decode reads one document at a time
		// afterwards.
		//
		// The parser fills ast.DocumentNode.Anchors as it reads, so parse copies
		// the anchors from it instead of collecting them again.
		anchors := make(map[string]ast.Node, len(doc.Anchors)+len(d.referenceAnchorNodeMap))
		maps.Copy(anchors, d.referenceAnchorNodeMap)
		maps.Copy(anchors, doc.Anchors)
		d.anchorNodeMaps = append(d.anchorNodeMaps, anchors)

		if d.toCommentMap == nil {
			continue
		}

		// Read the comments only for a caller who asked for them.
		d.anchorNodeMap = anchors
		d.anchorValueMap = make(map[string]reflect.Value)
		if err := d.collectComments(doc.Body); err != nil {
			return nil, err
		}

		cm := CommentMap{}
		maps.Copy(cm, d.toCommentMap)
		d.commentMaps = append(d.commentMaps, cm)
		for k := range d.toCommentMap {
			delete(d.toCommentMap, k)
		}
	}

	return normalizedFile, parseErr
}

// collectComments fills the map requested by the caller with CommentToMap.
//
// It reads the document as the decode does, because a comment is recorded
// against the path of the node carrying it, and the map is keyed by those paths.
func (d *Decoder) collectComments(body ast.Node) error {
	_, err := d.nodeToValue(body)

	return err
}

func (d *Decoder) isInitialized() bool {
	return d.parsedFile != nil || d.stream != nil
}

// canWalk reports whether this decode can read the source into v by walking it,
// without building a tree.
//
// A bare "any" takes the value of the document. valueBuilder
// builds it as the parse goes, anchors and aliases included. typedBuilder fills a struct,
// and gives up on what it cannot serve. See errNeedsTheTree.
// Everything else needs the tree:
//   - a CommentMap is keyed by the paths of the nodes carrying the comments;
//   - MapSlice keeps the order held by the tree;
//   - ReferenceFiles publishes anchors from documents this one never sees.
func (d *Decoder) canWalk(v reflect.Value) bool {
	if d.toCommentMap != nil {
		return false
	}
	if len(d.referenceFiles) > 0 || len(d.referenceDirs) > 0 || len(d.referenceReaders) > 0 {
		return false
	}
	if len(d.customUnmarshalerMap) > 0 {
		return false
	}
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}

	dst := v.Elem()
	if v3NodeLayoutOf(dst.Type()) != nil {
		// A yaml.Node at the root holds the document node, which the decode builds from the tree.
		return false
	}
	if codecutils.HoldsAnything(dst) {
		return true
	}

	// A struct walks unless an option needs the tree.
	if d.useJSONUnmarshaler {
		return false
	}
	if !dst.CanSet() {
		return false
	}

	return walkableType(dst.Type(), d.tagMode())
}

func (d *Decoder) decodeInit(v reflect.Value) error {
	if !d.isResolvedReference {
		if err := d.resolveReference(); err != nil {
			return err
		}
	}
	src, err := codecutils.ReadAll(d.reader)
	if err != nil {
		return err
	}
	// Keep the document. An error found while decoding prints the lines around
	// itself, and only the text holds those lines. d.source shares the bytes
	// the tree was built from instead of copying them again.
	d.source = yamlerrors.Source{Text: nocopy.String(src), FirstLine: 1}
	d.src = src
	d.built, d.budget = 0, codecutils.AliasBudget(len(src))
	d.expanding, d.expanded = nil, nil

	if d.canWalk(v) {
		if !d.firstOnly {
			d.stream = newWalkStream(d, src)

			return nil
		}
		switch _, err := unmarshalWalking(d, src, v); {
		case err == nil:
			d.walkedFirst = true

			return nil
		case !errors.Is(err, errNeedsTheTree):
			return err
		}
		// Fall through and read it again into a tree.
	}

	file, err := d.parse(src)
	if file == nil {
		return err
	}
	d.parsedFile, d.pendingErr = file, err

	return nil
}

// decodeWalking reads the next document of the stream into v by walking it, and
// returns true when it has.
//
// It returns false when the document needs a tree, or when the walk cannot fill v.
// The walk then stops for good, decodeWalking reads the source into a
// tree, and the decode carries on from the current document. A document started by the
// walk is decoded again from the start, over the values written by the walk.
func (d *Decoder) decodeWalking(v reflect.Value) (bool, error) {
	w := d.stream
	if d.canWalk(v) {
		err := w.decode(v)
		d.streamIndex = w.st.delivered
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, errEndOfStream):
			return true, io.EOF
		case !errors.Is(err, errNeedsTheTree):
			return true, err
		}
	}

	w.stop()
	d.stream = nil
	d.streamIndex = w.st.delivered

	return false, d.buildTree()
}

// buildTree reads the source into a tree, for a decode the walk cannot serve.
//
// One Decoder may receive different destinations for the documents of one
// stream, and only the first of them decided how the source was read.
func (d *Decoder) buildTree() error {
	file, err := d.parse(d.src)
	if file == nil {
		d.streamErr = err

		return err
	}
	d.parsedFile, d.pendingErr = file, err

	return nil
}

func (d *Decoder) decode(v reflect.Value) error {
	if d.walkedFirst {
		// The parse filled the destination of Unmarshal as it read the source.
		d.walkedFirst = false
		d.streamIndex++

		return nil
	}
	if d.stream != nil {
		if done, err := d.decodeWalking(v); done || err != nil {
			return err
		}
	}

	d.decodeDepth = 0
	d.anchorValueMap = make(map[string]reflect.Value)
	if d.streamIndex >= len(d.parsedFile.Docs) && d.pendingErr != nil {
		// The documents before the broken one have been read. The stream stops here, for this Decode and every
		// one after it.
		d.streamErr = d.pendingErr

		return d.pendingErr
	}
	if len(d.parsedFile.Docs) == 0 {
		// An empty document.
		dst := v.Elem()
		if dst.IsValid() {
			dst.Set(reflect.Zero(dst.Type()))
		}
	}
	if len(d.parsedFile.Docs) <= d.streamIndex {
		return io.EOF
	}
	doc := d.parsedFile.Docs[d.streamIndex]
	d.docSchema = doc.Schema
	body := doc.Body
	if doc.IsEmpty() {
		// A document written with "---" or "..." and holding nothing holds the
		// empty node, which decodes to null. Before, the stream index stayed where it
		// was, so Decode returned the previous document again and never reached the ones
		// after it.
		if dst := v.Elem(); dst.IsValid() {
			dst.Set(reflect.Zero(dst.Type()))
		}
		d.streamIndex++

		return nil
	}
	if body == nil {
		return nil
	}
	if len(d.commentMaps) > d.streamIndex {
		maps.Copy(d.toCommentMap, d.commentMaps[d.streamIndex])
	}
	d.anchorNodeMap = d.anchorNodeMaps[d.streamIndex]
	// The document is read, whatever it held, so the next Decode reads the
	// next one, as encoding/json.Decoder moves past a value that does not fit.
	// Before, the index stayed where it was. Every later Decode returned the same document
	// and its error, and a Decode loop never reached the end.
	if root := v.Elem(); root.IsValid() {
		if layout := v3NodeLayoutOf(root.Type()); layout != nil {
			// A yaml.Node at the root gets the document node, as in v3.
			root.Set(d.newV3NodeBuilder(layout).document(doc, body).Elem())
			d.streamIndex++

			return nil
		}
	}
	err := d.decodeValue(v.Elem(), body)
	d.streamIndex++

	return err
}
