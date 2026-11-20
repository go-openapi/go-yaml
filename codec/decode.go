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
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/format"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Decoder reads and decodes YAML values from an input stream.
type Decoder struct {
	// source is the document being decoded, so an error found in it can draw
	// the lines around itself.
	source yamlerrors.Source
	// entry is the node that writes the one being decoded: the "key:" of a
	// mapping entry, or the "-" of a sequence one. Decoding is depth first, so
	// each step saves it and puts it back, and the field behaves as a stack.
	//
	// It says where to report a validation error about a field the document
	// left out, there being nothing inside the node to point at, and what
	// indentation to strip when a node is handed to a custom unmarshaler.
	entry            ast.Node
	reader           io.Reader
	referenceReaders []io.Reader
	anchorNodeMap    map[string]ast.Node
	anchorNodeMaps   []map[string]ast.Node
	// referenceAnchorNodeMap holds the anchors published by ReferenceFiles and
	// ReferenceDirs. Every document of the input starts from these: naming an
	// anchor across files is what the option is for, where naming one across the
	// documents of a stream is not.
	referenceAnchorNodeMap map[string]ast.Node
	anchorValueMap         map[string]reflect.Value
	// openAnchors counts, by anchor name, the anchored nodes being decoded right
	// now. An alias to one of them stands inside its own anchor, which a Go
	// value has no way to hold. A count rather than a set: a document may
	// define the same anchor again inside the node it names.
	openAnchors map[string]int
	// merging is set while the decoder reads the mapping a "<<" brings in, and
	// stays set for everything decoded beneath it.
	merging              bool
	customUnmarshalerMap map[reflect.Type]func(any, []byte) error
	commentMaps          []CommentMap
	toCommentMap         CommentMap
	opts                 []DecodeOption
	referenceFiles       []string
	referenceDirs        []string
	isRecursiveDir       bool
	isResolvedReference  bool
	validator            StructValidator
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
	// without building a tree. src is kept so that a later Decode wanting
	// something the walk cannot serve can still build the tree. See canWalk.
	// walkedFirst records that Unmarshal filled its destination that way, and
	// firstOnly marks a Decoder that Unmarshal made, which reads one document.
	stream      *walkStream
	walkedFirst bool
	firstOnly   bool
	// streamErr is the error that stopped the stream: reading the source,
	// resolving its references, or parsing it into a tree. Decode returns it
	// from then on, as encoding/json.Decoder does, and as the walk does for a
	// parse it stopped itself. Before it was kept, the next Decode read the
	// drained reader as an empty source and returned io.EOF, which a Decode
	// loop takes for the stream's clean end.
	streamErr error
	// strs holds the text of every string a decode hands back, copied out of
	// the document so that the values outlive it.
	strs arena
	// built counts what this decode has made, and budget bounds it. See
	// aliasBudget. expanding is the outermost alias being read through, and
	// expanded the last one, which a refusal names.
	built       int
	budget      int
	expanding   *ast.AliasNode
	expanded    *ast.AliasNode
	src         []byte
	parsedFile  *ast.File
	streamIndex int
	decodeDepth int
	// docSchema is the resolution the document being decoded was read under.
	// See unmarshalableDocument.
	docSchema token.Schema
}

// NewDecoder returns a new decoder that reads from r.
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
	maxDecodeDepth = 10000

	// aliasBudgetFloor is the budget of an empty document, which aliasBudget adds 32 a byte to.
	aliasBudgetFloor = 1024
)

// aliasBudget bounds what a decode may build from a document of n bytes.
//
// An alias names a node and the value it stands for is built again wherever the
// alias appears, so a chain of them multiplies rather than adds: 259 bytes of
// YAML name 100,000 values and 423 bytes name 387 million. maxDecodeDepth does
// not see it -- nine levels of aliases is nine deep and as wide as it likes.
//
// Reading a document that holds no alias costs well under one step a byte, and
// the widest of the measured workloads reaches 0.19, so 32 a byte leaves a
// document written by a person alone and closes early on one written to exhaust
// memory.
//
// It counts what a decode builds and not what an alias builds, because an alias
// is the only thing that can build more than the document holds. Reading into an
// interface hands the same value to every alias of one anchor and so builds
// nothing extra, which is why that path does not reach this and why it reads a
// 423-byte document naming 387 million values in no time at all.
func aliasBudget(n int) int {
	return aliasBudgetFloor + 32*n
}

// nodeBudget bounds what a decode may build from node, as aliasBudget does
// from a document's bytes.
//
// A node handed to [Decoder.DecodeFromNode] has no source to measure, so this
// counts the nodes the tree holds written out, comment groups included.
// [ast.Walk] visits an alias once and does not follow its target, so an alias
// adds one to the count however much it names.
func nodeBudget(node ast.Node) int {
	var n nodeCount
	ast.Walk(&n, node)

	return aliasBudget(int(n))
}

// nodeCount counts the nodes [ast.Walk] visits.
type nodeCount int

func (c *nodeCount) Visit(node ast.Node) ast.Visitor {
	if node != nil {
		*c++
	}

	return c
}

// Decode reads the next YAML-encoded value from its input
// and stores it in the value pointed to by v.
//
// See the documentation for Unmarshal for details about the
// conversion of YAML into a Go value.
func (d *Decoder) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if !isPointer(rv) {
		return ErrDecodeRequiredPointerType
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
	return nil
}

// DecodeFromNode decodes node into the value pointed to by v.
func (d *Decoder) DecodeFromNode(node ast.Node, v any) error {
	rv := reflect.ValueOf(v)
	if !isPointer(rv) {
		return ErrDecodeRequiredPointerType
	}
	if !d.isInitialized() {
		// A node in hand is a tree, so this never takes the walking path.
		if err := d.decodeInit(reflect.Value{}); err != nil {
			return err
		}
	}
	// A node has no source of its own, and decodeInit budgeted the reader's,
	// so each call budgets the node it is given and starts its count afresh.
	d.built, d.budget = 0, nodeBudget(node)
	d.expanding, d.expanded = nil, nil
	// resolve references to the anchor on the same file
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

// enterMerge sets merging for the mapping a "<<" brings in, and returns the
// call that puts back what it was.
func (d *Decoder) enterMerge() (leave func()) {
	was := d.merging
	d.merging = true

	return func() { d.merging = was }
}

// castToFloatValue is castToFloat without a decoder, for the walking builder.
func castToFloatValue(v any) any {
	switch vv := v.(type) {
	case int:
		return float64(vv)
	case int8:
		return float64(vv)
	case int16:
		return float64(vv)
	case int32:
		return float64(vv)
	case int64:
		return float64(vv)
	case uint:
		return float64(vv)
	case uint8:
		return float64(vv)
	case uint16:
		return float64(vv)
	case uint32:
		return float64(vv)
	case uint64:
		return float64(vv)
	case float32:
		return float64(vv)
	case float64:
		return vv
	case *big.Int:
		f, _ := new(big.Float).SetInt(vv).Float64()

		return f
	case *big.Float:
		if f, exact := floatOf(vv); exact {
			return f
		}

		// The number outgrew float64 in one direction or the other, so it keeps
		// the width it was read at, as castToInteger keeps a big.Int and as the
		// same number untagged already does. Narrowed, "!!float 1e+310" came
		// back as +Inf and "!!float 1e-400" as zero.
		return vv
	case string:
		// The text came from a node "!!float" stands over, which the resolver
		// may have left as a string: under a "%YAML 1.1" directive "1e-330" is
		// a string, because 1.1 spells the exponent form with a "." in it.
		f, err := strconv.ParseFloat(vv, 64)
		if err == nil && (f != 0 || spellsZero(vv)) {
			return f
		}
		// Outside float64's range in one direction or the other. strconv
		// reports ErrRange and an infinity for an overflow, and a plain zero
		// with no error at all for an underflow, so the two are told apart by
		// the digits.
		if b, ok := token.ParseBigFloat(vv, token.ScalarType(vv, token.Schema12)); ok {
			return b
		}

		return f
	}
	return 0
}

// spellsZero reports whether text writes the number zero, which is what tells
// nought from a number too small for a float64: strconv.ParseFloat returns zero
// and no error for both.
func spellsZero(text string) bool {
	for _, r := range text {
		if r >= '1' && r <= '9' {
			return false
		}
	}

	return true
}

// floatOf narrows a big.Float, and reports false where float64 has no room for
// it: an overflow gives an infinity and an underflow a zero, and neither is the
// number.
func floatOf(v *big.Float) (float64, bool) {
	f, _ := v.Float64()
	if math.IsInf(f, 0) && !v.IsInf() {
		return 0, false
	}
	if f == 0 && v.Sign() != 0 {
		return 0, false
	}

	return f, true
}

// isPointer reports whether rv holds a pointer a decode can write through.
// A nil target holds nothing, and reflect panics on asking its type.
func isPointer(rv reflect.Value) bool {
	return rv.IsValid() && rv.Kind() == reflect.Pointer
}

func (d *Decoder) isExceededMaxDepth() bool {
	return d.decodeDepth > maxDecodeDepth
}

// isOverBudget reports a decode building more than the document can account
// for, which only an alias can do.
func (d *Decoder) isOverBudget() bool {
	return d.budget > 0 && d.built > d.budget
}

// hashableKey reports whether a decoded key can stand as a Go map key.
//
// reflect.Type.Comparable and not a switch on the kind: a slice, a map and a
// func are the kinds Go cannot hash, but a struct or an array holding one of
// them cannot be hashed either, and the kind alone reads "struct". A MapSlice
// is such a struct, so a mapping keyed on a mapping read under UseOrderedMap
// passed the switch and panicked comparing two of them.
func hashableKey(key any) bool {
	if key == nil {
		return true
	}

	return reflect.TypeOf(key).Comparable()
}

// mapKeyString is the text a decoded mapping key is addressed by.
//
// Taken from the node where the node is a plain scalar, so that the decoder,
// UseStringKeys and [ToJSON] name an entry the same way: keyName writes the
// type's own canonical spelling, where fmt.Sprint on the resolved value wrote a
// float and an integer alike and put "1: a" beside "1.0: b" under one name.
//
// A null gives "null" rather than the empty string, which is what the document
// wrote and what keeps it apart from the empty key: "null: a" and "\"\": b" are
// two entries.
//
// A collection has no spelling of its own and is refused. Go's own printing
// stood in before -- "map[k:v]" into an any, "[{k v}]" into a MapSlice -- which
// named the key after the value the decoder built rather than after the
// document. It gave three destinations three answers for one key, and let
// "? {k: v}" collide with a literal "map[k:v]" key and lose an entry with
// nothing reported. go.yaml.in/yaml/v3 refuses one on every destination.
func mapKeyString(node ast.Node, key any) (string, error) {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name, nil
	}
	if key == nil {
		return "null", nil
	}
	if k, ok := key.(string); ok {
		return k, nil
	}

	return "", yamlerrors.NewUnnamedKey(kindOfKey(node), node.GetToken())
}

// kindOfKey names the key the document wrote, for the message that refuses it.
//
// From the node and not from the value it decoded to. The value a mapping key
// decodes to is a Go map under one option and a MapSlice under another, so a
// mapping key was refused as "a sequence" where the reading was taken off the
// value.
func kindOfKey(node ast.Node) string {
	switch n := node.(type) {
	case *ast.MappingNode, *ast.MappingValueNode:
		return "mapping"
	case *ast.SequenceNode:
		return "sequence"
	case *ast.MappingKeyNode:
		return kindOfKey(n.Value)
	case *ast.AnchorNode:
		return kindOfKey(n.Value)
	case *ast.TagNode:
		return kindOfKey(n.Value)
	case *ast.AliasNode:
		if n.Target != nil {
			return kindOfKey(n.Target)
		}

		return "collection"
	default:
		return "collection"
	}
}

// keyedMap is a mapping being built for an `any` destination.
//
// It holds map[string]any while every key is a string, which is nearly every
// mapping, and widens to map[any]any on the first key that resolves to anything
// else -- an integer, a float, a null, a Base64, a time.Time. So a key keeps
// what it resolves to rather than the text that spells it, and "1: a" beside
// "\"1\": b" stays two entries where naming both "1" left one.
//
// valueBuilder.buildFrame does the same for the walk, and the two have to
// agree: a document read through either comes back the same way.
type keyedMap struct {
	byName map[string]any
	byKey  map[any]any
}

func newKeyedMap(size int) *keyedMap {
	return &keyedMap{byName: make(map[string]any, size)}
}

// holds reports whether the mapping already writes key.
//
// A mapping's own entries are written before the ones it merges, so this is
// where an own key beats a merged one. A timestamp key stands for its instant
// in any zone, and a wide number for its value, neither of which a Go map
// lookup finds.
func (k *keyedMap) holds(key any) bool {
	if k.byKey != nil {
		if _, stands := k.byKey[key]; stands {
			return true
		}
		_, stands := sameKeyIn(k.byKey, key)

		return stands
	}
	name, isString := key.(string)
	if !isString {
		return false
	}
	_, stands := k.byName[name]

	return stands
}

// put writes value under key, widening the mapping where the key is not a
// string.
func (k *keyedMap) put(key any, value any) {
	if name, isString := key.(string); isString && k.byKey == nil {
		k.byName[name] = value

		return
	}
	if k.byKey == nil {
		k.widen()
	}
	k.byKey[key] = value
}

// widen moves the mapping to map[any]any. The keys already written are strings
// and go over as they stand.
func (k *keyedMap) widen() {
	k.byKey = make(map[any]any, len(k.byName)+1)
	for name, value := range k.byName {
		k.byKey[name] = value
	}
	k.byName = nil
}

// value is the mapping, under whichever key type it settled on.
func (k *keyedMap) value() any {
	if k.byKey != nil {
		return k.byKey
	}

	return k.byName
}

// refuseOverBudget returns the error to stop a decode that has run away.
//
// It names an alias, as the walk does: only an alias builds more than the
// document holds, and the node in hand when the count runs out sits somewhere
// in an anchored tree, which may belong to another document. That alias is the
// outermost one being read through, or else the last one read.
//
// The walk counts only what it copies for an alias, so it always runs out
// inside one. This counts every node, so it can run out on the first node after
// an alias, and names that alias. Counting only under an alias would miss the
// aliases getMapNode and getArrayNode follow, which do not set expanding.
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
	return castToFloatValue(v)
}

// eachEntryOwnFirst calls fn for the mapping's own entries in document order,
// then for the ones a "<<" brings in, passing merged to say which.
//
// A mapping's own keys win over the ones it merges in, whichever side of the
// "<<" they were written, so reading them in this order settles precedence by
// position rather than by a lookup at each key. It also keeps the MapSlice scan
// off every mapping that holds no "<<".
func eachEntryOwnFirst(n *ast.MappingNode, fn func(value ast.Node, merged bool) error) error {
	for _, pass := range [2]bool{false, true} {
		for _, value := range n.Values {
			if mergeEntry(value) != pass {
				continue
			}
			if err := fn(value, pass); err != nil {
				return err
			}
		}
	}

	return nil
}

// mapEntry is one entry of a mapping: its key, its value, the node the walk
// hands over for the pair, and whether the key is a "<<".
type mapEntry struct {
	key, value, keyValue ast.Node
	merged               bool
}

// mapEntriesOwnFirst is the entries of a mapping, the ones it writes itself
// before the ones a "<<" brings in, each group in document order.
//
// It is [eachEntryOwnFirst]'s rule for the reflection path, which reads an
// [ast.MapNode] through its iterator where the other reads an *ast.MappingNode
// through its Values. Ordering the entries is what makes precedence fall out of
// writing only where nothing stands.
func mapEntriesOwnFirst(n ast.MapNode) []mapEntry {
	var own, merged []mapEntry

	iter := n.MapRange()
	for iter.Next() {
		entry := mapEntry{key: iter.Key(), value: iter.Value(), keyValue: iter.KeyValue()}
		if key, isKey := entry.key.(ast.MapKeyNode); isKey && key.IsMergeKey() {
			entry.merged = true
			merged = append(merged, entry)

			continue
		}
		own = append(own, entry)
	}

	return append(own, merged...)
}

// eachMergedEntry calls fn for the entries a "<<" brings in.
//
// A mapping contributes its keys in the order it would read in on its own --
// its own before the ones it merges itself -- so "<<: *b" where b is
// "<<: *a" over "y: 2" brings y before a's x, which is the order ToJSON writes
// and the order b reads as. A "<<" naming a sequence is folded into one MapNode
// before it gets here and is read through in the order the fold made.
func eachMergedEntry(value ast.MapNode, fn func(ast.Node) error) error {
	if n, ok := value.(*ast.MappingNode); ok {
		return eachEntryOwnFirst(n, func(v ast.Node, _ bool) error { return fn(v) })
	}

	iter := value.MapRange()
	for iter.Next() {
		if err := fn(iter.KeyValue()); err != nil {
			return err
		}
	}

	return nil
}

// mergeEntry reports whether a mapping's entry is a "<<".
func mergeEntry(node ast.Node) bool {
	n, ok := node.(*ast.MappingValueNode)

	return ok && n.Key != nil && n.Key.IsMergeKey()
}

// sameMapKey reports whether two decoded keys are one key.
//
// It compares the resolved values, so the string "1.0" and the float 1.0 are
// two keys and a merge cannot override one with the other. Comparing the text
// made them one, which is the half of defect 69 a MapSlice can represent; a
// map[string]any has lost one of them before this runs.
//
// Only a key mapKeyNodeToValue judged hashable reaches this as a value, so ==
// on the interfaces cannot panic -- the guard is there for a MapSlice a caller
// built by hand.
//
// Two Go values that == tells apart can still be one YAML key, and keyIDOf
// says which: YAML has one integer type, one float type and one timestamp
// type, so int(1) and uint64(1) are one key, float32(0.5) and 0.5 are one,
// and so are one instant in two zones. The parser refuses a mapping that
// writes one such key twice, and a merge or MapSlice.Set has to agree with it.
func sameMapKey(a, b any) bool {
	if !hashableKey(a) || !hashableKey(b) {
		return reflect.DeepEqual(a, b)
	}
	if a == b {
		return true
	}
	aKind, aName, aNamed := keyIDOf(a)
	if !aNamed {
		return false
	}
	bKind, bName, bNamed := keyIDOf(b)

	return bNamed && aKind == bKind && aName == bName
}

// keyIDOf returns the kind and the name a Go key is compared by in YAML, and
// whether it is a scalar YAML has a kind for.
//
// It is the value side of the identity [ast.ComparedKeyName] gives a key node,
// and gives the same answer for the value a key node decodes to:
// TestAKeyNodeAndItsValueHaveOneIdentity holds the two together over the
// corpus. A string is its text, a Base64 its canonical base64 text, a
// time.Time its instant in UTC, and a number [numberKey]'s name.
func keyIDOf(v any) (token.KeyKind, string, bool) {
	switch k := v.(type) {
	case nil:
		return token.KeyNull, "null", true
	case string:
		return token.KeyString, k, true
	case bool:
		return token.KeyBool, strconv.FormatBool(k), true
	case Base64:
		return token.KeyBinary, k.Canonical(), true
	case time.Time:
		return token.KeyTimestamp, k.UTC().Format(time.RFC3339Nano), true
	}

	return numberKey(v)
}

// numberKey returns the kind and the canonical name a Go number is keyed by in
// YAML, and whether v is a number at all. The names are the ones the parser
// gives the same number written in a document: [token.KeyNameOfFloat] and
// [token.KeyNameOfBigFloat] for a float, the decimal digits for an integer.
func numberKey(v any) (token.KeyKind, string, bool) {
	switch n := v.(type) {
	case *big.Int:
		if n == nil {
			return token.KeyOther, "", false
		}

		return token.KeyInt, n.String(), true
	case *big.Float:
		if n == nil {
			return token.KeyOther, "", false
		}

		return token.KeyFloat, token.KeyNameOfBigFloat(n), true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return token.KeyInt, strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return token.KeyInt, strconv.FormatUint(rv.Uint(), 10), true
	case reflect.Float32:
		return token.KeyFloat, token.KeyNameOfFloat(rv.Float(), 32), true
	case reflect.Float64:
		return token.KeyFloat, token.KeyNameOfFloat(rv.Float(), 64), true
	default:
		return token.KeyOther, "", false
	}
}

// comparesByValue reports whether a decoded key can be one YAML key with a key
// a Go map lookup does not find it under: a time.Time in another zone, a
// *big.Int or *big.Float, which == compares by address, or a Base64 written
// across lines.
func comparesByValue(key any) bool {
	switch key.(type) {
	case time.Time, *big.Int, *big.Float, Base64:
		return true
	default:
		return false
	}
}

// sameKeyIn returns the key of m that is one YAML key with key where a Go map
// lookup cannot find it, and whether m holds one.
//
// A scan, and only on a merge: the decoder builds uint64, int64 and float64
// keys that == compares by value, and scans only for the keys comparesByValue
// names.
func sameKeyIn(m map[any]any, key any) (any, bool) {
	if !comparesByValue(key) {
		return nil, false
	}
	for held := range m {
		if sameMapKey(held, key) {
			return held, true
		}
	}

	return nil, false
}

func (d *Decoder) mapKeyNodeToString(node ast.MapKeyNode) (string, error) {
	key, err := d.nodeToValue(node)
	if err != nil {
		return "", err
	}

	name, err := mapKeyString(node, key)
	if err != nil {
		return "", err
	}

	return d.strs.clone(name), nil
}

// mapKeyNodeToValue is the key a [MapSlice] entry is addressed by.
//
// A MapItem.Key is an interface{}, so it holds what the key resolves to, as a
// map[any]any key does: "1:" gives an integer, "1.0:" a float, "null:" nil.
// Naming it by text instead put `"1.0": a` and `1.0: b` in one MapSlice under
// one key, where 3.2.1.1 makes a string and a float two keys -- and a merge
// then let one override the other, which is what defect 69 was.
//
// [UseStringKeys] asks for the text instead and gets it for every scalar. A
// collection reaches mapKeyString either way, which refuses it: a MapSlice key
// must be comparable, and Go hashes no slice or map.
func (d *Decoder) mapKeyNodeToValue(node ast.MapKeyNode) (any, error) {
	key, err := d.nodeToValue(node)
	if err != nil {
		return nil, err
	}
	if d.useStringKeys || !hashableKey(key) {
		name, err := mapKeyString(node, key)
		if err != nil {
			return nil, err
		}

		return d.strs.clone(name), nil
	}

	return key, nil
}

// setToMapValue fills m from node.
//
// merged says the entries come from a "<<" rather than from the mapping
// holding it. A mapping's own keys win over the ones it merges in, whichever
// side of the "<<" they were written, and an earlier "<<" wins over a later one
// -- so a merged key is written only where none stands already. That is the
// rule keyToNodeMap applies for a struct, and this path did not: "x: 9" over
// "<<: *a" took the merged x, and "<<: [*a, *b]" took b's where the merge spec
// gives it to a.
func (d *Decoder) setToMapValue(node ast.Node, m *keyedMap, merged bool) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.MappingValueNode:
		if n.Key.IsMergeKey() {
			return d.eachMergeSource(n.Value, func(value ast.MapNode) error {
				return eachMergedEntry(value, func(v ast.Node) error {
					return d.setToMapValue(v, m, true)
				})
			})
		}
		key, err := d.mapKeyNodeToValue(n.Key)
		if err != nil {
			return err
		}
		if merged && m.holds(key) {
			return nil
		}

		v, err := d.nodeToValue(n.Value)
		if err != nil {
			return err
		}
		m.put(key, v)
	case *ast.MappingNode:
		return eachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
			return d.setToMapValue(value, m, merged || isMerge)
		})
	case *ast.AnchorNode:
		anchorName := n.Name.GetToken().Value
		d.anchorNodeMap[anchorName] = n.Value
	}
	return nil
}

// headCommentPath is the path a head comment is keyed by: a collection's own
// head comment belongs to its first key, as the walk below already records it.
func headCommentPath(node ast.Node) string {
	switch n := node.(type) {
	case *ast.MappingNode:
		if len(n.Values) != 0 {
			return n.Values[0].Key.GetPath()
		}
	case *ast.MappingValueNode:
		return n.Key.GetPath()
	}

	return node.GetPath()
}

// setToOrderedMapValue fills m from node, reading merged as [Decoder.setToMapValue] does.
//
// A MapSlice keeps what a map cannot: the order the document wrote. So a key
// already standing is overwritten where it stands rather than appended again --
// "<<: *a" over "x: 9" is one entry holding 9, at the place the merge brought x
// in. Appending gave a MapSlice with x twice, which no reader of one expects
// and which json.Marshal writes as a repeated member.
func (d *Decoder) setToOrderedMapValue(node ast.Node, m *MapSlice, merged bool) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.MappingValueNode:
		if n.Key.IsMergeKey() {
			return d.eachMergeSource(n.Value, func(value ast.MapNode) error {
				return eachMergedEntry(value, func(v ast.Node) error {
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
		return eachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
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
	d.addHeadOrLineCommentToMap(node)
	d.addFootCommentToMap(node)
}

func (d *Decoder) addHeadOrLineCommentToMap(node ast.Node) {
	d.addOwnHeadCommentToMap(node)

	sequence, ok := node.(*ast.SequenceNode)
	if ok {
		d.addSequenceNodeCommentToMap(sequence)
		return
	}
	d.addEntryLineCommentToMap(node)

	commentGroup := node.GetComment()
	if commentGroup == nil {
		return
	}
	texts := []string{}
	targetLine := int(node.GetToken().Position.Line)
	minCommentLine := math.MaxInt
	for _, comment := range commentGroup.Comments {
		if line := int(comment.Token.Position.Line); minCommentLine > line {
			minCommentLine = line
		}
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) == 0 {
		return
	}
	commentPath := node.GetPath()
	if minCommentLine < targetLine {
		switch n := node.(type) {
		case *ast.MappingNode:
			if len(n.Values) != 0 {
				commentPath = n.Values[0].Key.GetPath()
			}
		case *ast.MappingValueNode:
			commentPath = n.Key.GetPath()
		}
		d.addCommentToMap(commentPath, HeadComment(texts...))
	} else {
		d.addCommentToMap(commentPath, LineComment(texts[0]))
	}
}

// addOwnHeadCommentToMap records what [ast.BaseNode.HeadComment] holds.
//
// The walk below reads GetComment, which does not mean the same thing for every
// node -- head on a collection, beside the property on an anchor or a tag --
// and tells the two apart by comparing line numbers. HeadComment means "above
// this node" whatever the node is, so it needs no such test, and a comment that
// moved there would otherwise vanish from the map: a sequence's head comment
// did.
func (d *Decoder) addOwnHeadCommentToMap(node ast.Node) {
	if _, sequence := node.(*ast.SequenceNode); sequence {
		// addSequenceNodeCommentToMap reads it, and against the first element
		// rather than against the sequence.
		return
	}
	carrier, ok := node.(interface {
		GetHeadComment() *ast.CommentGroupNode
	})
	if !ok {
		return
	}
	group := carrier.GetHeadComment()
	if group == nil {
		return
	}

	texts := make([]string, 0, len(group.Comments))
	for _, comment := range group.Comments {
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) == 0 {
		return
	}

	d.addCommentToMap(headCommentPath(node), HeadComment(texts...))
}

// addEntryLineCommentToMap records the comment written on a mapping entry's own
// ":" line.
//
// An entry written the long way keeps it in a slot of its own, since neither
// the key nor the value can hold it: [ast.MappingValueNode.LineComment]. An
// entry written the short way puts it on the value, where the walk below finds
// it.
func (d *Decoder) addEntryLineCommentToMap(node ast.Node) {
	entry, ok := node.(*ast.MappingValueNode)
	if !ok || entry.LineComment == nil || len(entry.LineComment.Comments) == 0 {
		return
	}

	d.addCommentToMap(entry.GetPath(), LineComment(entry.LineComment.Comments[0].Token.Value))
}

func (d *Decoder) addSequenceNodeCommentToMap(node *ast.SequenceNode) {
	if len(node.ValueHeadComments) != 0 {
		for idx, headComment := range node.ValueHeadComments {
			if headComment == nil {
				continue
			}
			texts := make([]string, 0, len(headComment.Comments))
			for _, comment := range headComment.Comments {
				texts = append(texts, comment.Token.Value)
			}
			if len(texts) != 0 {
				d.addCommentToMap(node.Values[idx].GetPath(), HeadComment(texts...))
			}
		}
	}
	// A sequence's own comment is the first element's head comment, recorded
	// against that element. It reaches HeadComment now, Comment having meant
	// different things on different nodes, and the older field is still read
	// for a tree built by hand.
	firstElemHeadComment := node.GetComment()
	if firstElemHeadComment == nil {
		firstElemHeadComment = node.GetHeadComment()
	}
	if firstElemHeadComment != nil {
		texts := make([]string, 0, len(firstElemHeadComment.Comments))
		for _, comment := range firstElemHeadComment.Comments {
			texts = append(texts, comment.Token.Value)
		}
		if len(texts) != 0 {
			if len(node.Values) != 0 {
				d.addCommentToMap(node.Values[0].GetPath(), HeadComment(texts...))
			}
		}
	}
}

func (d *Decoder) addFootCommentToMap(node ast.Node) {
	var (
		footComment     *ast.CommentGroupNode
		footCommentPath = node.GetPath()
	)
	switch n := node.(type) {
	case *ast.SequenceNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	case *ast.MappingNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	case *ast.MappingValueNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	}
	if footComment == nil {
		return
	}
	var texts []string
	for _, comment := range footComment.Comments {
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) != 0 {
		d.addCommentToMap(footCommentPath, FootComment(texts...))
	}
}

func (d *Decoder) addCommentToMap(path string, comment *Comment) {
	for _, c := range d.toCommentMap[path] {
		if c.Position == comment.Position {
			// already added same comment
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
		return nil, ErrExceededMaxDepth
	}
	if d.isOverBudget() {
		return nil, d.refuseOverBudget(node)
	}

	d.setPathToCommentMap(node)
	switch n := node.(type) {
	case *ast.NullNode:
		return nil, nil
	case *ast.StringNode:
		return d.strs.value(n.Value), nil
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
		// What the tag made of the node is ast.TagNode.Resolve's answer and not
		// this switch's. The converter in codec/tojson.go asks the same
		// question; before it did, the two disagreed about "!!timestamp
		// not-a-date" and about "!!binary" on an empty node.
		res := n.Resolve()

		switch res.Verdict {
		case ast.TagUnresolved:
			// A local tag such as "!Ref", a foreign one, or a name YAML's type
			// repository does not define. 6.9.1 hands it to the application, so
			// the node is read by its kind.
			return d.nodeToValue(n.Value)
		case ast.TagKindMismatch:
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("%s does not support this kind of node", res.Tag), n.GetToken())
		case ast.TagValueMismatch:
			if res.Lax {
				// parser.WithLaxTags: the characters the scalar was written
				// with stand in for the value the tag could not make of them.
				return d.strs.value(res.Text), nil
			}

			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), n.Value.GetToken())
		default:
			// nothing to do
		}

		v, err := d.taggedValue(n, res)
		if err != nil {
			return nil, err
		}

		// The tag and the anchor under it belong to one node, whichever order
		// the document wrote them in (6.9), so the name stands for the tagged
		// value. Recorded here rather than by the AnchorNode case below, which
		// runs first and only ever sees what the tag was put in front of:
		// "a: !!int &a1 \"5\"" read 5 at a and "5" at "b: *a1", one node read
		// as a number where it stands and as a string through an alias to it.
		if anchor, anchored := n.Value.(*ast.AnchorNode); anchored && v != nil {
			d.anchorValueMap[anchor.Name.GetToken().Value] = reflect.ValueOf(v)
		}

		return v, nil
	case *ast.AnchorNode:
		anchorName := n.Name.GetToken().Value

		// To handle the case where alias is processed recursively, the result of alias can be set to nil in advance.
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
			// The alias stands inside the node its own anchor names, directly
			// or through another anchor. The parser reads that document and
			// the tree it builds holds a cycle -- YAML's representation is a
			// graph -- but a Go value is built by walking and has nowhere to
			// put one. Returning nil instead reported a mapping with a null in
			// it and no error at all.
			return nil, yamlerrors.NewRecursiveAlias(text, n.Value.GetToken())
		}
		if v, exists := d.anchorValueMap[text]; exists && d.shareAliases {
			// ShareAliases: one Go value under every name that points at it.
			if !v.IsValid() {
				return nil, nil
			}

			return v.Interface(), nil
		}
		if target, _ := d.aliasTarget(n); target != nil {
			// Read the node again rather than hand back what it decoded to
			// before. Two aliases of one anchor named one Go value, so writing
			// through either changed the other and a caller who never wrote a
			// document twice had two names for one map. Whether they shared
			// even turned on whether the destination happened to declare a
			// field for the anchor itself: with one, anchorValueMap held the
			// value and every alias got it; without one, nothing recorded it
			// and each alias decoded the node afresh.
			//
			// aliasBudget bounds what this costs.
			if d.expanding == nil {
				d.expanding, d.expanded = n, n
				defer func() { d.expanding = nil }()
			}

			return d.nodeToValue(target)
		}
		if v, exists := d.anchorValueMap[text]; exists {
			// No node to read: an anchor published by ReferenceFiles, which
			// carries a value and not a tree of this document.
			if !v.IsValid() {
				return nil, nil
			}
			return v.Interface(), nil
		}
		return nil, yamlerrors.NewUnknownAnchor(text, n.Value.GetToken())
	case *ast.LiteralNode:
		return d.strs.value(n.Value.Value), nil
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
		if err := refuseRepeats(n, d.useStringKeys); err != nil {
			return nil, err
		}
		if d.useOrderedMap {
			var m MapSlice
			if len(n.Values) > 0 {
				m.items = make([]MapItem, 0, len(n.Values))
			}
			if err := eachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
				return d.setToOrderedMapValue(value, &m, isMerge)
			}); err != nil {
				return nil, err
			}
			return m, nil
		}
		m := newKeyedMap(len(n.Values))
		if err := eachEntryOwnFirst(n, func(value ast.Node, isMerge bool) error {
			return d.setToMapValue(value, m, isMerge)
		}); err != nil {
			return nil, err
		}
		return m.value(), nil
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

// byteArray reports whether t is an array of bytes, which a "!!binary" fills as
// it fills a byte slice.
func byteArray(t reflect.Type) bool {
	return t.Kind() == reflect.Array && t.Elem().Kind() == reflect.Uint8
}

// copyIntoByteArray fills a byte array from b, and reports a length the array
// cannot hold rather than writing part of it.
func copyIntoByteArray(dst reflect.Value, b []byte, src ast.Node) error {
	if len(b) != dst.Len() {
		return yamlerrors.NewSyntax(
			fmt.Sprintf("a %d byte array cannot hold the %d bytes this !!binary carries", dst.Len(), len(b)),
			src.GetToken(),
		)
	}
	reflect.Copy(dst, reflect.ValueOf(b))

	return nil
}

// byteSlice reports whether t is a slice of bytes a []byte converts to. A
// []MyByte is not one: Go converts between slice types only where the element
// types are identical.
func byteSlice(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && reflect.TypeFor[[]byte]().ConvertibleTo(t)
}

// isBigNumber reports whether v holds a number wider than a native type.
func isBigNumber(v reflect.Value) bool {
	switch v.Interface().(type) {
	case *big.Int, *big.Float:
		return true
	default:
		return false
	}
}

func isFloatKind(k reflect.Kind) bool { return k == reflect.Float32 || k == reflect.Float64 }

// convertBigNumber converts a number the AST read into a [big.Int] or a
// [big.Float], which it does for one no native type holds.
//
// A float destination takes the nearest float64, and is refused where the
// number reaches past what one holds. strconv.ParseFloat answers the same
// values and returns ErrRange with them, and dropping that gave a document
// holding 1e400 a field reading +Inf with nothing said. A caller who wants the
// number whole decodes into a *big.Int or a *big.Float, which take it exactly.
//
// ±Inf is a float64 like any other and a document naming one gets it: ".inf"
// resolves to an ast.InfinityNode, which holds a float64 already and never
// reaches here. Only a finite big.Int or big.Float does, and a float64 of ±Inf
// from finite digits has lost the number rather than named it.
//
// A string destination takes the digits. Anything else is left to convertValue's
// own rules, which end in a type mismatch.
func convertBigNumber(v reflect.Value, typ reflect.Type) (reflect.Value, bool) {
	var (
		f    float64
		text string
	)
	switch n := v.Interface().(type) {
	case *big.Int:
		f, _ = new(big.Float).SetInt(n).Float64()
		text = n.String()
	case *big.Float:
		f, _ = n.Float64()
		text = n.Text('g', -1)
	default:
		return reflect.Value{}, false
	}

	switch typ.Kind() {
	case reflect.Float32, reflect.Float64:
		if outOfFloatRange(f, text) {
			return reflect.Value{}, false
		}

		return reflect.ValueOf(f).Convert(typ), true
	case reflect.String:
		return reflect.ValueOf(text).Convert(typ), true
	default:
		return reflect.Value{}, false
	}
}

// outOfFloatRange reports whether the float64 nearest to a finite number has
// lost it rather than rounded it: infinite, or zero from digits that are not.
func outOfFloatRange(f float64, text string) bool {
	if math.IsInf(f, 0) {
		return true
	}

	return f == 0 && strings.ContainsFunc(text, func(r rune) bool {
		return r >= '1' && r <= '9'
	})
}

// eachMergeSource hands fn each mapping a "<<" names, one at a time and in the
// order written: the mapping itself, or each mapping of a sequence of them.
//
// getMapNode folds a sequence into one MapNode, which loses where one of its
// mappings ends. Each mapping has to be read whole -- its own keys, then what
// its own "<<" brings -- before the next, since an earlier mapping wins over a
// later one: "<<: [*a, *b]" takes a key a merges in over the same key b writes.
func (d *Decoder) eachMergeSource(node ast.Node, fn func(ast.MapNode) error) error {
	return d.eachMergeNode(node, func(one ast.Node, inSequence bool) error {
		value, err := d.getMapNode(one, !inSequence)
		if err != nil {
			return err
		}

		return fn(value)
	})
}

// eachMergeNode hands fn the node of each mapping a "<<" names, as
// eachMergeSource reads them. inSequence says the node is an element of a
// sequence, which has to be a mapping: a sequence inside it is refused.
func (d *Decoder) eachMergeNode(node ast.Node, fn func(one ast.Node, inSequence bool) error) error {
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

// mergeSequence returns the sequence a "<<" names, through an anchor, an alias
// or a tag, and nil where it names anything else. It records an anchor on the
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
				// getMapNode reports the unknown anchor.
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

func (d *Decoder) getMapNode(node ast.Node, isMerge bool) (ast.MapNode, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, ErrExceededMaxDepth
	}

	if err := refuseDuplicateKeys(node); err != nil {
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

// aliasTarget returns the node an alias names, and the name it was written
// with.
//
// The parser fills [ast.AliasNode.Target] as it reads, so a node handed over on
// its own -- what DecodeFromNode and expressions.Path.Read are given -- already
// carries what its aliases name. Before this the decoder looked the name up in
// anchorNodeMap, which is built from a whole file, and an alias whose anchor
// stood outside the node found nothing.
//
// The map is still the answer for a tree built by hand or written by the
// encoder, neither of which went through a parse.
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
		return nil, ErrExceededMaxDepth
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

// binaryBytes returns the bytes a "!!binary" node holds, and reports false for
// every other node.
//
// The tag names a sequence of bytes and the decoder builds a []byte for an
// `any`, so a []byte destination is the one Go type the tag is for.
// decodeSlice read the node as a sequence and reported `string was used where
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
			// "k: !!binary" with nothing after it takes the tag's own default,
			// which tagZero writes as an empty []byte.
			return []byte{}, true
		}
		b, err := base64.StdEncoding.DecodeString(res.Text)

		return b, err == nil
	default:
		return nil, false
	}
}

// mergeKeyValue reads the mapping a "<<" names as a value of its own, an
// ordered one under UseOrderedMap.
func (d *Decoder) mergeKeyValue(node ast.Node) (any, error) {
	if d.useOrderedMap {
		m := MapSlice{}
		err := d.eachMergeSource(node, func(value ast.MapNode) error {
			return eachMergedEntry(value, func(v ast.Node) error {
				return d.setToOrderedMapValue(v, &m, true)
			})
		})

		return m, err
	}
	m := newKeyedMap(0)
	err := d.eachMergeSource(node, func(value ast.MapNode) error {
		return eachMergedEntry(value, func(v ast.Node) error {
			return d.setToMapValue(v, m, true)
		})
	})
	if err != nil {
		return nil, err
	}

	return m.value(), nil
}

func (d *Decoder) convertValue(v reflect.Value, typ reflect.Type, src ast.Node) (reflect.Value, error) {
	if converted, ok := convertBigNumber(v, typ); ok {
		return converted, nil
	}
	if isBigNumber(v) && isFloatKind(typ.Kind()) {
		return reflect.Value{}, yamlerrors.NewOverflow(typ, fmt.Sprint(v.Interface()), src.GetToken())
	}
	if typ.Kind() != reflect.String {
		return convertToKind(v, typ, src)
	}
	// cast value to string
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
		// A timestamp is written as its instant in RFC 3339, in its own zone,
		// as ast.KeyName names a timestamp key and ToJSON writes one.
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
		// Handle named types, e.g., `type MyString string`
		val = val.Convert(typ)
	}
	return val, nil
}

// convertToKind converts v to typ, a type that is not a string kind.
//
// A string converts to a float kind when strconv.ParseFloat reads it, which
// is how a quoted "1e3" fills a float field. The result has typ itself, so a
// named float type such as `type Ratio float64` is filled as well.
func convertToKind(v reflect.Value, typ reflect.Type, src ast.Node) (reflect.Value, error) {
	if v.Type().ConvertibleTo(typ) {
		return v.Convert(typ), nil
	}
	if isFloatKind(typ.Kind()) && v.Kind() == reflect.String {
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return reflect.ValueOf(f).Convert(typ), nil
		}
	}

	return reflect.Zero(typ), yamlerrors.NewTypeMismatch(typ, v.Type(), src.GetToken())
}

type jsonUnmarshaler interface {
	UnmarshalJSON(data []byte) error
}

// tagMode says which of encoding/json's rules this decoder adds to the `yaml`
// tag. Neither option set is go.yaml.in/yaml/v3 exactly.
func (d *Decoder) tagMode() tagMode {
	mode := yamlTags
	if d.useJSONTags {
		mode |= jsonTags
	}
	if d.useInferredNames {
		mode |= inferredNames
	}

	return mode
}

func (d *Decoder) deleteStructKeys(structType reflect.Type, unknownFields map[string]ast.Node) error {
	for structType.Kind() == reflect.Pointer {
		structType = structType.Elem()
	}
	if structType.Kind() == reflect.Map {
		// A `,inline` map takes every entry no field claims, so a mapping read
		// into this struct writes nothing unknown.
		clear(unknownFields)

		return nil
	}
	if structType.Kind() != reflect.Struct {
		return nil
	}

	structFieldMap, err := structFieldMap(structType, d.tagMode())
	if err != nil {
		return err
	}

	for field := range structType.Fields() {
		if isIgnoredStructField(field, d.tagMode()) {
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

// unmarshalableDocument renders node as the document a bytes unmarshaler reads.
//
// Where the document was read under YAML 1.1, the text opens with "%YAML 1.1",
// so a nested Unmarshal reads the fragment as the document read it: "<<" merges
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
// It used to render the node back to text and decode that as a document of
// its own, under YAML 1.2 and from line 1: under 1.1, "yes" reached
// UnmarshalText as "yes" where a string field holds "true", and a
// "!!timestamp" the string field accepts was refused.
func (d *Decoder) unmarshalableText(node ast.Node) ([]byte, error) {
	var v string
	if err := d.decodeValue(reflect.ValueOf(&v).Elem(), node); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

var astNodeType = reflect.TypeFor[ast.Node]()

func (d *Decoder) existsTypeInCustomUnmarshalerMap(t reflect.Type) bool {
	if _, exists := d.customUnmarshalerMap[t]; exists {
		return true
	}

	globalCustomUnmarshalerMu.Lock()
	defer globalCustomUnmarshalerMu.Unlock()
	if _, exists := globalCustomUnmarshalerMap[t]; exists {
		return true
	}
	return false
}

func (d *Decoder) unmarshalerFromCustomUnmarshalerMap(t reflect.Type) (func(any, []byte) error, bool) {
	if unmarshaler, exists := d.customUnmarshalerMap[t]; exists {
		return unmarshaler, exists
	}

	globalCustomUnmarshalerMu.Lock()
	defer globalCustomUnmarshalerMu.Unlock()
	if unmarshaler, exists := globalCustomUnmarshalerMap[t]; exists {
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

// decodesItself reports whether the value ptr points to reads itself from the
// document, through an interface it implements or a type the decoder reads
// specially. A json.Unmarshaler counts only with useJSONUnmarshaler.
func decodesItself(ptr reflect.Value, useJSONUnmarshaler bool) bool {
	switch ptr.Interface().(type) {
	case *MapSlice, *MapSliceSeq, *MapItem, *Base64:
		// Read by the decoder itself, ahead of any interface they satisfy.
		// [UseJSONUnmarshaler] is the counterpart of [UseJSONMarshaler] and a
		// last resort the same way: a document reaching a MapSlice through
		// ToJSON would arrive with JSON's keys, which are strings, and lose
		// every key this type exists to keep apart.
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

func (d *Decoder) decodeByUnmarshaler(dst reflect.Value, src ast.Node) error {
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
			if !isPointer(rv) {
				return ErrDecodeRequiredPointerType
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

	// textErr is why the node gave no text, returned when no other interface
	// reads it: "!!timestamp nope" into a TextUnmarshaler is refused for the
	// timestamp it fails to be.
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

// unmarshalJSON hands src to unmarshaler as the JSON document it converts to.
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
		return ErrExceededMaxDepth
	}
	if d.isOverBudget() {
		return d.refuseOverBudget(src)
	}
	if !dst.IsValid() {
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
		if dst.IsNil() {
			return nil
		}
		if src.Type() == ast.NullType {
			// set nil value to pointer
			dst.Set(reflect.Zero(valueType))
			return nil
		}
		v := d.createDecodableValue(dst.Type())
		decodeErr := d.decodeValue(v, src)
		castedValue, err := castToAssignableValue(v, dst.Type(), src)
		if err != nil {
			return errors.Join(decodeErr, err)
		}
		// What decoded stands, error or not: see createDecodedNewValue.
		dst.Set(castedValue)
		if decodeErr != nil {
			return decodeErr
		}
	case reflect.Interface:
		if dst.Type() == astNodeType {
			dst.Set(reflect.ValueOf(src))
			return nil
		}
		srcVal, err := d.nodeToValue(src)
		if err != nil {
			return err
		}
		v := reflect.ValueOf(srcVal)
		if v.IsValid() {
			dst.Set(v)
		} else {
			dst.Set(reflect.Zero(valueType))
		}
	case reflect.Map:
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
		case int:
			// What "!!int" reads a number as: castToInteger hands back an int
			// where the number fits one, which is what strconv.Atoi gave. The
			// untagged number arrives as a uint64 or an int64, so without this
			// case "n: !!int 5" was refused where "n: 5" read.
			if !dst.OverflowInt(int64(vv)) {
				dst.SetInt(int64(vv))
				return nil
			}
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
			// The AST hands one of these over only for a number that outgrew
			// int64, uint64 or float64, so nothing here has room for it. Fall
			// through to the overflow error, which names the number.
		case string: // handle scientific notation
			if i, err := strconv.ParseFloat(vv, 64); err == nil {
				if 0 <= i && i <= math.MaxUint64 && !dst.OverflowInt(int64(i)) {
					dst.SetInt(int64(i))
					return nil
				}
			} else { // couldn't be parsed as float
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
		case int:
			// See the signed case above: "!!int" reads a number that fits as an
			// int.
			if 0 <= vv && !dst.OverflowUint(uint64(vv)) {
				dst.SetUint(uint64(vv))
				return nil
			}
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
		case string: // handle scientific notation
			if i, err := strconv.ParseFloat(vv, 64); err == nil {
				if 0 <= i && i <= math.MaxUint64 && !dst.OverflowUint(uint64(i)) {
					dst.SetUint(uint64(i))
					return nil
				}
			} else { // couldn't be parsed as float
				return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
			}

		default:
			return yamlerrors.NewTypeMismatch(valueType, reflect.TypeOf(v), src.GetToken())
		}
		return yamlerrors.NewOverflow(valueType, fmt.Sprint(v), src.GetToken())
	default:
		// nothing to do
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

// castToAssignableValue returns value in a form a target of type target can hold:
// value itself where it is assignable, or, for a pointer target, its address,
// taken up to five times. It reports a type mismatch at src otherwise. This is
// how an alias reuses its anchor's value under ShareAliases.
func castToAssignableValue(value reflect.Value, target reflect.Type, src ast.Node) (reflect.Value, error) {
	if target.Kind() != reflect.Pointer {
		if !value.Type().AssignableTo(target) {
			return reflect.Value{}, yamlerrors.NewTypeMismatch(target, value.Type(), src.GetToken())
		}
		return value, nil
	}

	const maxAddrCount = 5

	for range maxAddrCount {
		if value.Type().AssignableTo(target) {
			break
		}
		if !value.CanAddr() {
			break
		}
		value = value.Addr()
	}
	if !value.Type().AssignableTo(target) {
		return reflect.Value{}, yamlerrors.NewTypeMismatch(target, value.Type(), src.GetToken())
	}
	return value, nil
}

// aliasedValue returns the value the anchor of alias decoded to, cast to typ,
// when it can be reused; otherwise it returns the node the anchor names, for
// the caller to read again, which is nil for an anchor without one.
//
// The decoded value is reused when aliases share it, and when there is no
// node to read again: an anchor ReferenceFiles published.
func (d *Decoder) aliasedValue(alias *ast.AliasNode, typ reflect.Type) (reflect.Value, ast.Node, bool) {
	target, name := d.aliasTarget(alias)
	if value := d.anchorValueMap[name]; value.IsValid() && (d.shareAliases || target == nil) {
		if v, err := castToAssignableValue(value, typ, alias); err == nil {
			return v, nil, true
		}
	}

	return reflect.Value{}, target, false
}

// isNullNode reports whether n is the empty node, written as one, under an
// anchor or under a "!!null" tag: "&a ~", "&a" alone, "!!null ~" and "!!null"
// alone are null as "~" is. An anchor names a node and does not change it, and
// the tag names the type the node already has.
//
// Without the tag case, "!!null ~" into a struct or a pointer read as a
// mapping and was refused with "null was used where mapping is expected".
func isNullNode(n ast.Node) bool {
	for {
		switch node := n.(type) {
		case *ast.AnchorNode:
			n = node.Value

			continue
		case *ast.TagNode:
			res := node.Resolve()

			return res.Verdict == ast.TagResolved && res.Tag == token.NullTag
		}

		return n == nil || n.Type() == ast.NullType
	}
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
	if isNullNode(node) {
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
	if !isNullNode(node) {
		if err := d.decodeValue(newValue, node); err != nil {
			// The value is decoded in place, as encoding/json decodes it: what
			// decoded before the error stands, beside the error, and the caller
			// writes it. Handing back nothing threw away a nested struct's other
			// fields, and a map's caller lost the entry, which SetMapIndex
			// deletes for an invalid value.
			if v, castErr := castToAssignableValue(newValue, typ, node); castErr == nil {
				return v, err
			}

			return reflect.Value{}, err
		}
	}
	return castToAssignableValue(newValue, typ, node)
}

func (d *Decoder) keyToNodeMap(node ast.Node, ignoreMergeKey bool, getKeyOrValueNode func(*ast.MapNodeIter) ast.Node) (map[string]ast.Node, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(node, false)
	if err != nil {
		return nil, err
	}
	// The map is keyed by name.
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping {
		if err := refuseRepeats(m, true); err != nil {
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

		// Named as decodeStruct names an entry, so "1: x" reaches an inline
		// map as "1", as it reaches a map[string]any. Keeping only the keys
		// that decoded to a Go string returned an empty map, and no error, on
		// the first that did not.
		key, named, err := d.entryName(keyNode)
		if err != nil {
			return nil, err
		}
		if !named {
			// A collection key names no field and no string-keyed entry.
			continue
		}
		// refuseRepeats refused a repeat the parse did not allow; one it
		// allowed leaves the last value standing.
		keyToNodeMap[key] = getKeyOrValueNode(mapIter)
	}

	// A mapping's own keys win over the ones it merges in, whichever side of the
	// "<<" they were written, and an earlier "<<" wins over a later one. Merged
	// keys were validated against the mapping's own until now, so "<<: *base"
	// beside a key the base also writes -- the whole point of a merge -- was
	// refused as a duplicate. The key is not written twice: it is written once
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

// orderedMapShape reports the sequence of one-entry mappings an "!!omap" names,
// where the node the tag stands on is one.
//
// A tag naming a kind its node is not is refused, as every other malformed tag
// is: the document parses -- the shape is a question about what it means, not
// about how it is written -- and the loader declines to build a value. So
// "!!omap [1, 2]" is a document the parser reports and nodeToValue refuses.
//
// No implementation here votes on the shape. Measured 2026-09-10:
// go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1 both pass "!!omap" through
// whatever stands under it -- a conforming sequence included -- so neither
// builds an ordered map at all and neither reports a key written twice. What
// they read says nothing about the shape a reader that builds the type should
// require.
func orderedMapShape(node ast.Node) (*ast.SequenceNode, bool) {
	// The anchor may follow the tag, as in "!!omap &o [{x: 1}]".
	seq := orderedMapSequence(node)
	if seq == nil {
		return nil, false
	}
	for _, entry := range seq.Values {
		if entryMapping(entry) == nil {
			return nil, false
		}
	}

	return seq, true
}

// notAnOrderedMap reports a node "!!omap" stands on that is not the shape the
// tag names.
func notAnOrderedMap(node ast.Node) error {
	return yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", node.GetToken())
}

// entryMapping is the one entry an "!!omap" element holds, or nil where the
// element is not a mapping holding exactly one key.
//
// A mapping that writes its one key twice, "{a: 1, a: 2}", comes back whole:
// it is one key and a repeat, which orderedMapOf refuses as a mapping's repeat
// is refused and reads under AllowDuplicateMapKey with the last value standing,
// as the walk does. Counting the entries instead refused it as the wrong shape,
// where every other reader reported the repeat.
//
// A mapping holding a "<<" comes back whole too, and orderedMapOf counts its
// keys once the merge is read: "{<<: {a: 0}, a: 1}" is one entry, and
// "{<<: {a: 0, b: 1}}" is two. The walk and ToJSON count them the same way.
func entryMapping(entry ast.Node) ast.Node {
	switch n := entry.(type) {
	case *ast.MappingNode:
		if slices.ContainsFunc(n.Values, func(v *ast.MappingValueNode) bool { return mergeEntry(v) }) {
			return n
		}
		if distinctKeys(n) != 1 {
			return nil
		}
		if len(n.Values) != 1 {
			return n
		}

		return n.Values[0]
	case *ast.MappingValueNode:
		return n
	case *ast.AnchorNode:
		return entryMapping(n.Value)
	case *ast.TagNode:
		return entryMapping(n.Value)
	case *ast.AliasNode:
		// "!!omap [{x: 1}, *m]" is an ordered map where m names a one-entry
		// mapping. The tree keeps the node an alias names, so it is read
		// through; without this the element was refused as no entry at all.
		if n.Target == nil {
			return nil
		}

		return entryMapping(n.Target)
	default:
		return nil
	}
}

// tagZero is what a tag standing on no value denotes: the value its type starts
// at. "k: !!int" is 0 and "k: !!bool" is false, exactly as
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

// mergedKeyToNodeMap reads what a "<<" names, which is a mapping or a sequence
// of them: getMapNode folds a sequence into one MapNode.
//
// The keys are not validated against one another and the first written stands.
// Two mappings of a sequence may write the same key -- "<<: [*one, *two]" where
// both write "b" -- and YAML 1.1's merge says the mapping named first wins
// rather than that the document is wrong.
func (d *Decoder) mergedKeyToNodeMap(
	node ast.Node, ignoreMergeKey bool, getKeyOrValueNode func(*ast.MapNodeIter) ast.Node,
) (map[string]ast.Node, error) {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return nil, ErrExceededMaxDepth
	}

	keyToNodeMap := map[string]ast.Node{}
	hold := func(k string, v ast.Node) {
		if _, held := keyToNodeMap[k]; !held {
			keyToNodeMap[k] = v
		}
	}
	err := d.eachMergeSource(node, func(mapNode ast.MapNode) error {
		// The mapping's own keys first, then what its own "<<" bring in.
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

// taggedValue is what a tagged node denotes, once ast.TagNode.Resolve has said
// the tag applies.
func (d *Decoder) taggedValue(n *ast.TagNode, res ast.Resolution) (any, error) {
	if res.Empty {
		// A tag with nothing after it takes the tag's own default, which
		// parser's newTagDefaultScalarValueNode builds and nothing read:
		// "k: !!int" was 0 while "k: !!bool" and "k: !!binary" were errors.
		return tagZero(res.Tag)
	}

	switch res.Tag {
	case token.TimestampTag:
		return d.castToTime(n.Value)
	case token.IntegerTag:
		// Read from the text the tag stands over, in the base the document's
		// own schema gives those digits. Resolving the node instead took a
		// quoted scalar as the string it is and sent it through
		// castToInteger's fixed 1.2 reading, so `!!int "017"` was 17 in a
		// document where `!!int 017` was 15 -- and `!!int "0b101"` under 1.1
		// was 0 where the plain spelling was 5. 3.3.2 gives the "!"
		// non-specific tag only to a node lacking an explicit tag, so the two
		// spellings are one node.
		//
		// And one Go value: uint64, int64 or *big.Int, as the untagged number
		// decodes. Narrowed to int, "!!int 1" was int(1) where "1" was
		// uint64(1), two keys to a Go map, so an own "!!int 1" did not beat the
		// "1" a "<<" brought in and the mapping held both.
		return taggedInteger(res.Text, res.Schema), nil
	case token.FloatTag:
		return d.castToFloat(taggedFloat(res.Text, res.Schema)), nil
	case token.NullTag:
		return nil, nil
	case token.BinaryTag:
		// A Base64 and not a []byte: it is comparable, so "? !!binary" keys a
		// mapping where a byte slice cannot, and it tells an encoder the value
		// is binary, so a round trip writes "!!binary" again. Resolve has read
		// the text as base64 already. See [Base64].
		return Base64(res.Text), nil
	case token.BooleanTag:
		// The tag says boolean whatever the text is, so a spelling neither
		// schema resolves is read in lower case: "!!bool Yes" and
		// "!!bool YES" are the same request. Resolve has agreed there is
		// one to read.
		b, _ := token.ParseBool(strings.ToLower(res.Text))

		return b, nil
	case token.StringTag:
		// The tag names the type, so the scalar keeps the text it was
		// written with rather than what the core schema resolved it to:
		// "!!str 0x10" is "0x10" and not "16", and "!!str False" keeps its
		// capital F. An anchor over that scalar names the same string, so
		// its recorded value is replaced too.
		text := d.strs.clone(res.Text)
		if anchor, anchored := n.Value.(*ast.AnchorNode); anchored {
			d.anchorValueMap[anchor.Name.GetToken().Value] = reflect.ValueOf(text)
		}

		return text, nil
	case token.OrderedMapTag:
		seq, ordered := orderedMapShape(n.Value)
		if !ordered {
			return nil, notAnOrderedMap(n.Value)
		}

		return d.orderedMapOf(seq)
	default:
		// A tag naming a kind -- !!seq, !!map, !!set, !!merge. The node is read
		// as it stands.
		return d.nodeToValue(n.Value)
	}
}

// orderedMapOf reads a sequence an "!!omap" names into a [MapSliceSeq].
//
// The shape is orderedMapShape's, so every element holds one entry and the
// ordered map is the sequence's order. The parser records a key repeated across
// entries on the sequence, and refuseOrderedMapDuplicates reads that record as
// every other load does. Where the parse allowed the repeat, the later entry's
// value replaces the earlier one's, as it does in a mapping.
func (d *Decoder) orderedMapOf(seq *ast.SequenceNode) (MapSliceSeq, error) {
	// An entry's own repeat first, then a repeat across entries: the walk and
	// the converters meet the first as the entry's mapping closes, and the
	// second only as the "!!omap" closes.
	for _, entry := range seq.Values {
		if err := refuseDuplicateKeys(entryMapping(entry)); err != nil {
			return MapSliceSeq{}, err
		}
	}
	if err := refuseOrderedMapDuplicates(seq); err != nil {
		return MapSliceSeq{}, err
	}

	var m MapSlice
	for _, entry := range seq.Values {
		var one MapSlice
		if err := d.setToOrderedMapValue(entryMapping(entry), &one, false); err != nil {
			return MapSliceSeq{}, err
		}
		if len(one.items) != 1 {
			// A "<<" left the count to now. See entryMapping.
			return MapSliceSeq{}, notAnOrderedMap(seq)
		}
		for _, item := range one.items {
			if err := m.Set(item.Key, item.Value); err != nil {
				return MapSliceSeq{}, err
			}
		}
	}

	return MapSliceSeq(m), nil
}

// getMergeAliasName support single alias only
// mapEntryFunc is handed one entry of a mapping. merged says the entry comes
// from a "<<" rather than from the mapping itself.
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
		// A "!!timestamp" on an empty node, or on a null. It takes the tag's
		// own default -- the zero time. newTagDefaultScalarValueNode builds the
		// empty string for the node a tag stands on with nothing after it, and
		// handNull builds a null where the document ends there.
		return time.Time{}, nil
	}
	s, ok := v.(string)
	if !ok {
		return time.Time{}, yamlerrors.NewTypeMismatch(reflect.TypeFor[time.Time](), reflect.TypeOf(v), src.GetToken())
	}
	if t, ok := ast.ParseTimestamp(s); ok {
		return t, nil
	}

	// Refused rather than answered with the zero time. A text no format reads
	// used to come back as 0001-01-01 with no error, so a document holding
	// "2001-12-14 21:59:43.10 -5" -- which the timestamp expression allows and
	// the layouts above do not spell -- decoded to a date nobody wrote.
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

// writtenFields records which fields a mapping has set, so that what a "<<"
// merges in does not overwrite them. A struct of 64 fields or fewer costs no
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

// located is the field a mapping key names, found in a destination.
//
// sf is nil where no field of the type answers to the name. ord is the bit the
// merge bookkeeping marks, and at is the index path, empty where the field
// stands in the type itself.
type located struct {
	value reflect.Value
	sf    *StructField
	ord   int
	at    []int
}

// name reports the field as a Go path -- "U.T.A" for a field A promoted from an
// embedded T of U -- which is how encoding/json names one in a type error.
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

// fieldFor finds the field a mapping key names.
//
// A type that embeds another is read through readFields.flat, which names every
// field the embedding promotes and holds the index path to reach it; fieldAt
// fills the pointers along the way. A type that embeds nothing takes the direct
// lookup and no index path at all, which is the common case and the fast one.
func fieldFor(dst reflect.Value, fields *readFields, name string) (located, error) {
	ff, known := fields.lookup(name)
	if !known {
		return located{}, nil
	}
	if ff.at == nil {
		return located{value: dst.Field(ff.sf.Index), sf: ff.sf, ord: ff.ord}, nil
	}

	field, err := fieldAt(dst, ff.at)
	if err != nil {
		// fieldAt could not make a pointer standing between, which happens
		// where an embedded pointer is unexported. encoding/json refuses the
		// same shape.
		return located{}, fmt.Errorf(
			"cannot set embedded pointer to unexported struct on the way to %q in %s",
			name, dst.Type(),
		)
	}

	return located{value: field, sf: ff.sf, ord: ff.ord, at: ff.at}, nil
}

// rangeMapEntries reads the mapping src names and hands its entries over: the
// ones it writes itself first, in document order, then the ones each "<<"
// merges in, the earliest "<<" first.
//
// Order carries the precedence, so nothing has to be collected to get it right.
// A field the mapping writes itself is set before any merged entry reaches it,
// and an earlier "<<" is read before a later one, which is what YAML 1.1's
// merge asks for. The caller skips a merged entry whose field is already set.
func (d *Decoder) rangeMapEntries(src ast.Node, ignoreMergeKey bool, visit mapEntryFunc) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(src, false)
	if err != nil {
		return err
	}
	// A struct names its fields, so two keys of one name are one field.
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping {
		if err := refuseRepeats(m, true); err != nil {
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

// rangeMergedEntries hands over the entries of what a "<<" names, which is a
// mapping or a sequence of them, each mapping whole before the next: see
// eachMergeSource.
func (d *Decoder) rangeMergedEntries(src ast.Node, ignoreMergeKey bool, visit mapEntryFunc) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
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

// entryName reads the text a mapping key addresses its entry by. named is false
// for a key no field can be named after, such as "[a]:".
//
// keyName writes the type's own canonical spelling, so "true: x" addresses a
// field tagged "true" and "1.0: x" one tagged "1.0", which is how the same
// document reads into a map[string]any. Requiring the key to be an ast string
// dropped every one of those instead.
func (d *Decoder) entryName(keyNode ast.Node) (string, bool, error) { //nolint:unparam // signature kept stable: callers already handle the error
	name, kind := ast.KeyName(keyNode)

	return name, kind != token.KeyOther, nil
}

// getParentMapTokenIfExists if the NodeType is a container type such as MappingType or SequenceType,
// it is necessary to return the parent MapNode's colon token to represent the entire container.
// validationErrorToken returns where to report a field that failed validation.
//
// A scalar is reported at itself: the message names the field, and what a
// reader wants pointed at is the value that failed. A mapping or a sequence has
// no one token to point at -- the first of them stands inside the value rather
// than naming it -- so those are reported at the ':' of the entry that writes
// them, which is what entry carries.
func validationErrorToken(value, entry ast.Node) *token.Token {
	if value.Type() == ast.MappingType || value.Type() == ast.SequenceType {
		if entry != nil {
			return entry.GetToken()
		}
	}

	return value.GetToken()
}

// decodeInlineFields reads the three kinds of `,inline` field an index path
// cannot place, each from the whole mapping rather than from an entry of it.
//
// An ordinary embedded struct is not one of them: readFields.flat names every
// field it promotes and setField writes each entry straight into the field it
// reaches, so the two decode paths place an entry the same way. What is left
// here is a map, which takes the entries no field claims and so cannot be known
// until the mapping ends; a field carrying `,alias`, which takes the value the
// document's "<<" names; and a field whose type reads its own node, which
// go.yaml.in/yaml/v3 also hands the whole mapping.
//
// A `,inline` map is handed the entries readFields.flat does not name -- the
// struct's own fields and everything its embedded structs promote. With nothing
// left over the map stays nil, as v3 leaves it.
func (d *Decoder) decodeInlineFields(
	dst reflect.Value, src ast.Node, fields *readFields,
	ignoreMergeKey bool, unknownFields map[string]ast.Node, foundErr *error,
) error {
	structType := dst.Type()
	aliasName := d.getMergeAliasName(src)

	var entries map[string]ast.Node

	for _, sf := range fields.inline {
		fieldValue := dst.Field(sf.Index)
		if sf.IsAutoAlias {
			if err := d.setAliasedField(fieldValue, aliasName); err != nil {
				return err
			}

			continue
		}

		fieldType := fieldValue.Type()
		takesUnclaimed := inlineTakesUnclaimedEntries(fieldType)
		if !takesUnclaimed && !inlineReadsItsOwnNode(fieldType) {
			// An embedded struct, whose fields setField has already filled.
			continue
		}
		if !fieldValue.CanSet() {
			field := structType.Field(sf.Index)

			return fmt.Errorf("cannot set embedded type as unexported field %s.%s", field.PkgPath, field.Name)
		}
		if fieldValue.Type().Kind() == reflect.Pointer && isNullNode(src) {
			// set nil value to pointer
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
			// Every entry went to a field, so the map stays nil rather than
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
		setDecoded(fieldValue, newFieldValue)
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

// setAliasedField sets a field carrying `,alias` to the value of the anchor the mapping's "<<" names.
// It leaves the field as it is where the mapping merges no alias, or the anchor holds no value.
func (d *Decoder) setAliasedField(fieldValue reflect.Value, aliasName string) error {
	if aliasName == "" {
		return nil
	}
	anchored := d.anchorValueMap[aliasName]
	if !anchored.IsValid() {
		return nil
	}
	value, err := castToAssignableValue(anchored, fieldValue.Type(), d.anchorNodeMap[aliasName])
	if err != nil {
		return err
	}
	fieldValue.Set(value)

	return nil
}

// inlineMapNode builds the mapping an inline field decodes: every entry, or with takesUnclaimed only the entries
// no field of fields claims.
func inlineMapNode(entries map[string]ast.Node, fields *readFields, takesUnclaimed bool) *ast.MappingNode {
	mapNode := ast.Mapping(nil, false)
	for k, v := range entries {
		if takesUnclaimed && fields.claims(k) {
			continue
		}
		key := &ast.StringNode{Value: k}
		mapNode.Values = append(mapNode.Values, ast.MappingValue(nil, key, v))
	}

	return mapNode
}

// setDecoded writes v, what createDecodedNewValue decoded, into dst. v is
// invalid only where nothing decoded at all, and dst then keeps what it held.
func setDecoded(dst, v reflect.Value) {
	if v.IsValid() {
		dst.Set(v)
	}
}

// stopsTheDocument reports whether err ends the decoding of its document, at any depth: a repeated key, which
// makes the document invalid YAML, the limits, ErrExceededMaxDepth and the alias budget, and an unknown field,
// which the caller asked to stop on with DisallowUnknownField. Any other error is a field error: the first one is
// kept and the fields after it decode.
func stopsTheDocument(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, yamlerrors.ErrDuplicateKey) ||
		errors.Is(err, ErrExceededMaxDepth) ||
		errors.Is(err, yamlerrors.ErrExcessiveAliasing) ||
		errors.Is(err, yamlerrors.ErrRecursiveAlias) ||
		errors.Is(err, yamlerrors.ErrUnknownAnchor) ||
		errors.Is(err, yamlerrors.ErrUnknownField)
}

// recordFieldError keeps err in foundErr unless an earlier field error is there already. A type mismatch is
// tagged with the struct field fieldName returns for it.
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

// sequenceEntryToken returns the '-' that writes entry idx of node, where the
// sequence kept it.
func sequenceEntryNode(node ast.ArrayNode, idx int) ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok || idx < 0 || idx >= len(seq.Entries) {
		return nil
	}

	return seq.Entries[idx]
}

// findEntry returns the value a mapping writes under name, and the entry that
// writes it. Both are nil where the mapping does not write it.
//
// This walks the mapping rather than reading a map built up front: only a
// document that fails validation asks, and only for the field that failed.
func (d *Decoder) findEntry(src ast.Node, ignoreMergeKey bool, name string) (value, entry ast.Node) {
	_ = d.rangeMapEntries(src, ignoreMergeKey,
		func(entryName string, _, valueNode, entryNode ast.Node, _ bool) error {
			if value == nil && entryName == name {
				value, entry = valueNode, entryNode
			}

			return nil
		})

	return value, entry
}

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
	if src == nil || isNullNode(src) {
		// A null writes nothing, so the struct keeps what it holds. A field's
		// null never gets here: createDecodedNewValue reads it first. A
		// document that is only "~" does.
		return nil
	}
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	structType := dst.Type()
	srcValue := reflect.ValueOf(src)
	srcType := srcValue.Type()
	if srcType.Kind() == reflect.Pointer {
		srcType = srcType.Elem()
		srcValue = srcValue.Elem()
	}
	if structType == srcType {
		// dst value implements ast.Node
		dst.Set(srcValue)
		return nil
	}
	fields, err := structFields(structType, d.tagMode())
	if err != nil {
		return err
	}
	ignoreMergeKey := fields.fields.hasMergeProperty()

	var unknownFields map[string]ast.Node
	if d.disallowUnknownField {
		unknownFields = map[string]ast.Node{}
	}

	var (
		written  writtenFields
		foundErr error
	)

	// Walk the document and look each entry's field up, rather than walking the
	// fields and looking each entry up. What has to be held is then the struct's
	// fields -- read once per type and shared -- and not a map of every entry
	// the mapping writes.
	setField := func(name string, keyNode, valueNode, entryNode ast.Node, merged bool) error {
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
			// The mapping writes this itself, or an earlier "<<" does.
			return nil
		}
		written.mark(found.ord)

		fieldValue := found.value
		if fieldValue.Type().Kind() == reflect.Pointer && isNullNode(src) {
			// set nil value to pointer
			fieldValue.Set(reflect.Zero(fieldValue.Type()))

			return nil
		}

		prevEntry := d.enterEntry(entryNode)
		newFieldValue, err := d.createDecodedNewValue(fieldValue.Type(), fieldValue, valueNode)
		d.entry = prevEntry
		setDecoded(fieldValue, newFieldValue)
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

	if len(fields.inline) > 0 {
		if err := d.decodeInlineFields(dst, src, fields, ignoreMergeKey, unknownFields, &foundErr); err != nil {
			return err
		}
	}

	if foundErr != nil {
		return foundErr
	}

	// Ignore unknown fields when parsing an inline struct (recognized by a nil token).
	// Unknown fields are expected (they could be fields from the parent struct).
	if d.disallowUnknownField && src.GetToken() != nil {
		if err := d.refuseUnknownFields(unknownFields); err != nil {
			return err
		}
	}

	return d.validateStruct(dst, src, fields, ignoreMergeKey)
}

// refuseUnknownFields returns an error for the first of unknownFields whose
// name carries none of the allowed field prefixes.
func (d *Decoder) refuseUnknownFields(unknownFields map[string]ast.Node) error {
	for key, node := range unknownFields {
		allowed := slices.ContainsFunc(d.allowedFieldPrefixes, func(prefix string) bool {
			return strings.HasPrefix(key, prefix)
		})
		if !allowed {
			return yamlerrors.NewUnknownField(fmt.Sprintf(`unknown field "%s"`, key), node.GetToken())
		}
	}

	return nil
}

// validateStruct runs the decoder's validator on dst, once src is decoded into it.
//
// A validation error is reported at the entry that writes the field, which is
// the ":" rather than anything inside the value. The entries are read here
// rather than during the decode: a document that validates never needs them.
func (d *Decoder) validateStruct(dst reflect.Value, src ast.Node, fields *readFields, ignoreMergeKey bool) error {
	if d.validator == nil {
		return nil
	}
	err := d.validator.Struct(dst.Interface())
	if err == nil {
		return nil
	}
	ev := reflect.ValueOf(err)
	if ev.Kind() != reflect.Slice {
		return err
	}
	for i := range ev.Len() {
		fieldErr, ok := reflect.TypeAssert[FieldError](ev.Index(i))
		if !ok {
			continue
		}
		renderName, exists := fields.renderNameOf(fieldErr.StructField())
		if !exists {
			continue
		}
		value, entry := d.findEntry(src, ignoreMergeKey, renderName)
		if value != nil {
			// TODO: to make FieldError message cutomizable
			return yamlerrors.NewSyntax(err.Error(), validationErrorToken(value, entry))
		}
		if t := d.missingFieldToken(src); t != nil {
			// A missing field has no entry of its own, so the error goes to
			// the mapping that should have held it.
			return yamlerrors.NewSyntax(err.Error(), t)
		}
	}

	return err
}

// missingFieldToken returns where to report a field the document left out.
//
// There is no entry to point at, so the error goes to the first key of the
// mapping that should have held one.
func (d *Decoder) missingFieldToken(src ast.Node) *token.Token {
	if d.entry != nil {
		return d.entry.GetToken()
	}
	if m, ok := src.(*ast.MappingNode); ok && len(m.Values) > 0 {
		return m.Values[0].Key.GetToken()
	}

	return src.GetToken()
}

// enterEntry records the node that writes the one about to be decoded, and
// returns the one it replaces for the caller to put back.
func (d *Decoder) enterEntry(entry ast.Node) ast.Node {
	prev := d.entry
	d.entry = entry

	return prev
}

func (d *Decoder) decodeArray(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	if b, ok := d.binaryBytes(src); ok && byteArray(dst.Type()) {
		// "!!binary" into a byte array, as decodeSlice reads one into a byte
		// slice. The text is a scalar, so the array reader below refused it
		// with "string was used where sequence is expected".
		return copyIntoByteArray(dst, b, src)
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
		if elemType.Kind() == reflect.Pointer && isNullNode(v) {
			// set nil value to pointer
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
		return ErrExceededMaxDepth
	}

	if b, ok := d.binaryBytes(src); ok && byteSlice(dst.Type()) {
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

	var (
		foundErr error
		idx      int
	)
	for iter.Next() {
		v := iter.Value()
		entryIdx := idx
		idx++
		if elemType.Kind() == reflect.Pointer && isNullNode(v) {
			// set nil value to pointer
			sliceValue = reflect.Append(sliceValue, reflect.Zero(elemType))
			continue
		}
		prevEntry := d.enterEntry(sequenceEntryNode(arrayNode, entryIdx))
		dstValue, err := d.createDecodedNewValue(elemType, reflect.Value{}, v)
		d.entry = prevEntry
		if stopsTheDocument(err) {
			return err
		}
		if err != nil && foundErr == nil {
			foundErr = err
		}
		if !dstValue.IsValid() {
			// The element decoded to nothing at all: keep its place.
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
		return ErrExceededMaxDepth
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

// taggedOrderedMap reports the sequence an "!!omap" standing on node names,
// through an anchor.
func taggedOrderedMap(node ast.Node) (*ast.SequenceNode, bool) {
	switch n := node.(type) {
	case *ast.TagNode:
		if res := n.Resolve(); res.Verdict == ast.TagResolved && res.Tag == token.OrderedMapTag && !res.Empty {
			return orderedMapShape(n.Value)
		}
	case *ast.AnchorNode:
		return taggedOrderedMap(n.Value)
	}

	return nil, false
}

// validateDuplicateKey refuses two entries that land on one key of the Go
// destination, which the parse does not see: "1: a" and "\"1\": b" are two YAML
// keys and one key of a map[string]T, and "1: a" and "1.0: b" are two YAML keys
// and one key of a map[float64]T.
//
// Every comparable key counts, not only a string: a float-keyed destination
// kept the last of two keys that resolve to one float and dropped the other
// with nothing reported. Under AllowDuplicateMapKey the last one stands, as for
// a repeat the parse records.
// admitMapKey checks k, the decoded key of an entry about to go into mapValue,
// and reports whether the map already holds it under a folded key, in which
// case the entry is skipped.
//
// An invalid k, a null key, passes unchecked. Otherwise k must be comparable:
// a sequence or a mapping used as a key, into a map whose key type admits one,
// made SetMapIndex panic with "hash of unhashable type" rather than reporting
// the document.
func (d *Decoder) admitMapKey(mapValue reflect.Value, keyMap map[any]struct{}, k reflect.Value, key ast.Node, folded bool) (bool, error) {
	if !k.IsValid() {
		return false, nil
	}
	if !k.Comparable() {
		return false, yamlerrors.NewUnhashableKey(dynamicTypeOf(k), key.GetToken())
	}
	if folded {
		return mapValue.MapIndex(k).IsValid(), nil
	}

	return false, d.validateDuplicateKey(keyMap, k.Interface(), key)
}

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
// It reads it the way nodeToValue reads one under [UseOrderedMap], through
// setToOrderedMapValue, so the two destinations agree about a merge. They did
// not: this loop walked the entries in document order, appended each one, and
// asked validateDuplicateKey whether the key had been seen -- which is true of
// every key a "<<" overrides. So "x: 9" under a "<<: *a" that also writes x was
// refused as a duplicate key here and read as 9 into an any, and a merge
// sequence whose mappings share a key was refused on the name of the anchored
// mapping. That is the fault defect 40 fixed for a Go map, on the fifth place a
// merge is resolved.
func (d *Decoder) decodeMapSlice(dst *MapSlice, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	if seq, ordered := taggedOrderedMap(src); ordered {
		one, err := d.orderedMapOf(seq)
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
// The destination names the spelling, so a plain mapping read into a
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
	if seq, ordered := taggedOrderedMap(src); ordered {
		one, err := d.orderedMapOf(seq)

		return MapSlice(one), err
	}

	var m MapSlice
	err := d.decodeMapSlice(&m, src)

	return m, err
}

// decodeMap reads the mapping src names into dst.
//
// A map dst already holds keeps its entries, and the mapping's entries are
// written over them, as encoding/json and go.yaml.in/yaml/v3 do and as the
// walk does. A merged entry is written over a caller's entry too, as it is
// over a struct field's default; only a key the mapping or an earlier "<<"
// wrote stops it.
func (d *Decoder) decodeMap(dst reflect.Value, src ast.Node) error {
	d.stepIn()
	defer d.stepOut()
	if d.isExceededMaxDepth() {
		return ErrExceededMaxDepth
	}

	mapNode, err := d.getMapNode(src, d.merging)
	if err != nil {
		return err
	}
	mapType := dst.Type()
	if m, isMapping := mapNode.(*ast.MappingNode); isMapping && !d.merging && mapType.Key().Kind() == reflect.String {
		// A string-keyed map holds one entry for two keys of one name. A key
		// of another type answers through admitMapKey, on the value it
		// decodes to: 1 and 1.0 are two names and one float64.
		if err := refuseRepeats(m, true); err != nil {
			return err
		}
	}
	mapValue := dst
	if mapValue.IsNil() {
		mapValue = reflect.MakeMap(mapType)
	}
	keyType := mapValue.Type().Key()
	valueType := mapValue.Type().Elem()
	// keyMap records the keys this mapping writes, its own and the merged
	// ones, which is what a later merge has to step around. The keys a
	// caller's map held before are not in it.
	keyMap := map[any]struct{}{}
	// A mapping read under a merge is the fold getMapNode makes of "<<: [a, b]",
	// so two entries alike in it are the sequence doing its job -- the earlier
	// mapping wins -- and not the repeated key 3.2.1.1 refuses.
	folded := d.merging
	var foundErr error
	for _, entry := range mapEntriesOwnFirst(mapNode) {
		key := entry.key
		value := entry.value
		if entry.merged {
			// The merge type gives a mapping's own keys precedence over the
			// ones a "<<" brings in, and an earlier "<<" precedence over a
			// later one. Both fall out of writing only where nothing stands:
			// the own entries went in above, and the merges are read in
			// document order.
			//
			// Nothing here asks validateDuplicateKey. Two mappings of a merge
			// sequence that define one key is what the sequence is for, and a
			// merged key equal to an own key is the override the type
			// describes -- neither is the repeated key 3.2.1.1 refuses.
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
		if valueType.Kind() == reflect.Pointer && isNullNode(value) {
			// set nil value to pointer
			mapValue.SetMapIndex(k, reflect.Zero(valueType))
			continue
		}
		prevEntry := d.enterEntry(entry.keyValue)
		dstValue, err := d.createDecodedNewValue(valueType, reflect.Value{}, value)
		d.entry = prevEntry
		if stopsTheDocument(err) {
			return err
		}
		if err != nil && foundErr == nil {
			foundErr = err
		}
		if !dstValue.IsValid() {
			// SetMapIndex deletes the key for an invalid value, which took the
			// caller's own entry away with it.
			continue
		}
		if !k.IsValid() {
			// expect nil key
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

// mergeIntoMap decodes the mapping a "<<" brings in, value, into a map of its own, and copies into mapValue each of
// its entries whose key the mapping has not written, recording the key in written.
//
// The merged mapping is decoded apart because mapValue may be the caller's map, which decodeMap writes into: decoding
// the merge straight into it would write over the mapping's own entries.
//
// Each mapping of a merge sequence is decoded whole before the next, so an
// earlier one wins with what its own "<<" brought: see eachMergeSource.
func (d *Decoder) mergeIntoMap(mapValue reflect.Value, written map[any]struct{}, value ast.Node) error {
	return d.eachMergeNode(value, func(source ast.Node, inSequence bool) error {
		merged := reflect.New(mapValue.Type()).Elem()
		leave := func() {}
		if !inSequence {
			// A sequence here is the merge's own, and decodeMap reads it
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
		// through the same path a map[string]any takes, so the two agree
		// on the spelling.
		decodeKeyAs = stringType
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

// stringMapKey returns the name a map keyed by a string kind stores key under.
//
// It reads the key the way nodeToValue reads it for the map[string]any
// it builds, so a document decoded into either gives the same keys.
// Decoding a null through createDecodedNewValue gave "", the Go
// zero, which collides with the empty key: "null: a" and "\"\": b"
// are two entries and were read as one, so the document was refused
// as holding a duplicate key.
func (d *Decoder) stringMapKey(key ast.Node) (string, error) {
	keyValue, err := d.nodeToValue(key)
	if err != nil {
		return "", err
	}
	if _, named := ast.KeyName(key); named == token.KeyOther &&
		keyValue != nil && !reflect.TypeOf(keyValue).Comparable() {
		// A sequence or a mapping used as a key. mapKeyString would
		// give it the spelling Go prints, "[a b]", which no reader
		// takes apart again.
		//
		// The destination is keyed by a string, so what matters is
		// whether the key can be named, not whether the value it
		// resolves to is comparable. ast.KeyName names every scalar,
		// and returns token.KeyOther for a collection alone -- which
		// is where mapKeyString falls back to Go's printing. Asking
		// comparability instead refused "!!binary aGVsbG8=: x", whose
		// name is the base64 the document wrote: the same document
		// read into an any gives {"aGVsbG8=": "x"}, and the two
		// destinations disagreed over a []byte neither had to hash.
		return "", yamlerrors.NewUnhashableKey(reflect.TypeOf(keyValue), key.GetToken())
	}
	name, err := mapKeyString(key, keyValue)
	if err != nil {
		return "", err
	}
	return d.strs.clone(name), nil
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

// holdsAValue reports whether the document carries something to decode.
//
// A document of directives and nothing else opens their scope and denotes no
// value, and a source holding only comments is not a document at all. Everything
// else is one, including a tag or an anchor standing on nothing: "!!null" and
// "&a" denote null exactly as "~" does.
//
// This replaced folding the whole document into a Go value and asking whether it
// came out nil. That read "!!null" and "&a" as no document at all and dropped
// them from the stream -- "a: 1" over "---" over "!!null" handed back one
// document where go.yaml.in/yaml/v3 hands back two -- and it cost a second
// reading of every document to answer a question about its shape.
func holdsAValue(doc *ast.DocumentNode) bool {
	if isEmptyDocument(doc) {
		return true
	}
	if doc.Body == nil {
		return false
	}

	switch doc.Body.(type) {
	case *ast.DirectiveNode, *ast.CommentGroupNode:
		return false
	default:
		return true
	}
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

// readAll returns everything r holds, without copying it where r already has
// it in one piece.
//
// Unmarshal wraps the caller's slice in a bytes.Buffer, so copying it into
// another buffer and then calling String on that made three copies of the
// document before the parse began. The parser keeps windows into what it is
// given, so handing it the caller's own bytes costs nothing and holds nothing
// twice.
//
// A reader that is not backed by a slice still has to be read into one. That
// buffer grows by doubling, so what it hands back may have spare capacity, and
// the tree pins the whole block; the alternative is a copy the size of the
// document, which is what this exists to avoid.
func readAll(r io.Reader) ([]byte, error) {
	switch b := r.(type) {
	case *bytes.Buffer:
		return b.Bytes(), nil
	case *bytes.Reader:
		src := make([]byte, b.Len())
		if _, err := io.ReadFull(b, src); err != nil {
			return nil, err
		}

		return src, nil
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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

		// assign new anchor definition to anchorMap
		if _, err := d.parse(bytes); err != nil {
			return err
		}

		// A reference file exists to publish its anchors, so they become the
		// base every later file and every document of the input starts from.
		// Its own documents are not the input's, so the bookkeeping the parse
		// left behind is folded in here rather than indexed by stream position
		// later.
		for _, m := range d.anchorNodeMaps {
			maps.Copy(d.referenceAnchorNodeMap, m)
		}
		d.anchorNodeMaps = nil
		d.commentMaps = nil
	}
	d.isResolvedReference = true
	return nil
}

// parserOptions are the options this decoder's own options ask the parse for.
func (d *Decoder) parserOptions() []parser.Option {
	var opts []parser.Option
	if d.toCommentMap != nil {
		opts = append(opts, parser.WithComments())
	}
	if d.allowDuplicateMapKey {
		opts = append(opts, parser.WithAllowDuplicateMapKey())
	}
	if len(d.referenceAnchorNodeMap) > 0 {
		// The parser refuses an alias naming no anchor of its own document, and
		// a reference file exists so that another document may name its
		// anchors. Publishing them is what lets that alias through.
		opts = append(opts, parser.WithAnchors(d.referenceAnchorNodeMap))
	}

	// Last, so that a caller who asks for a parser option directly overrides
	// what a decode option asked for on its behalf.
	opts = append(opts, d.extraParserOptions...)

	return opts
}

func (d *Decoder) parse(bytes []byte) (*ast.File, error) {
	f, err := parser.ParseBytes(bytes, d.parserOptions()...)
	if err != nil {
		return nil, err
	}
	normalizedFile := &ast.File{}
	for _, doc := range f.Docs {
		if !holdsAValue(doc) {
			continue
		}
		normalizedFile.Docs = append(normalizedFile.Docs, doc)

		// An anchor belongs to the document it was written in: a stream is a run
		// of documents, each independent of the rest, so each gets its own books
		// and an alias naming an anchor from an earlier one has nothing to name.
		// Kept alongside the documents, as the comment maps are, because this
		// runs over the whole stream and decode reads one document at a time
		// afterwards.
		//
		// The parser fills ast.DocumentNode.Anchors as it reads, so the books
		// are taken from it rather than collected again here.
		anchors := make(map[string]ast.Node, len(doc.Anchors)+len(d.referenceAnchorNodeMap))
		maps.Copy(anchors, d.referenceAnchorNodeMap)
		maps.Copy(anchors, doc.Anchors)
		d.anchorNodeMaps = append(d.anchorNodeMaps, anchors)

		if d.toCommentMap == nil {
			continue
		}

		// Only a caller that asked for the comments pays for reading them.
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

	return normalizedFile, nil
}

// collectComments fills the map a caller asked for with CommentToMap.
//
// It reads the document the way the decode does, since a comment is recorded
// against the path of the node carrying it and the paths are what the map is
// keyed by.
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
// A bare "any" asks for the value the document denotes, which valueBuilder
// builds as the parse goes, anchors and aliases included. A struct is filled by
// typedBuilder, which gives up on what it cannot serve; see errNeedsTheTree.
// Everything else needs the tree: a CommentMap is keyed by the paths of the
// nodes carrying the comments, MapSlice keeps the order the tree holds, and
// ReferenceFiles publishes anchors from documents this one never sees.
func (d *Decoder) canWalk(v reflect.Value) bool {
	if d.toCommentMap != nil || d.useOrderedMap {
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
	if holdsAnything(dst) {
		// UseStringKeys names every key by its text, which mapKeyNodeToValue
		// does on the tree. valueBuilder keys a mapping by what each key
		// resolves to, so it would give "1: a" the key uint64(1) whatever the
		// option says.
		return !d.useStringKeys
	}

	// SPIKE: reading into a Go struct by walking.
	if !typedWalkEnabled || d.disallowUnknownField || d.validator != nil || d.allowDuplicateMapKey || d.useJSONUnmarshaler {
		return false
	}
	if d.useStringKeys {
		// An "any" field is built by valueBuilder, which keys a mapping by
		// value whatever the option says, as for a bare "any" above.
		return false
	}
	if dst.Kind() != reflect.Struct || !dst.CanSet() {
		return false
	}

	return walkableType(dst.Type(), d.tagMode())
}

// isEmptyDocument reports whether doc holds no value and is a document all the
// same.
//
// "---" opens a document and "..." closes one, so a document written with
// either marker exists whatever stands between them: "---\n...\n" holds the
// empty node and decodes to null. A run of comments carries no marker and is
// no document at all -- l-yaml-stream puts those in a document prefix -- so
// "# c\n" decodes to nothing.
func isEmptyDocument(doc *ast.DocumentNode) bool {
	if doc.Start == nil && doc.End == nil {
		return false
	}
	if doc.Body == nil {
		return true
	}

	_, comments := doc.Body.(*ast.CommentGroupNode)

	return comments
}

func (d *Decoder) decodeInit(v reflect.Value) error {
	if !d.isResolvedReference {
		if err := d.resolveReference(); err != nil {
			return err
		}
	}
	src, err := readAll(d.reader)
	if err != nil {
		return err
	}
	// Keep the document: an error found while decoding draws the lines around
	// itself, and only the text can say what those are. It shares the bytes
	// the tree was built from rather than copying them again.
	d.source = yamlerrors.Source{Text: nocopy.String(src), FirstLine: 1}
	d.src = src
	d.built, d.budget = 0, aliasBudget(len(src))
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
	if err != nil {
		return err
	}
	d.parsedFile = file

	return nil
}

// decodeWalking reads the next document of the stream into v by walking it, and
// returns true when it has.
//
// It returns false where the document needs a tree, or where v is a destination
// the walk cannot fill. The walk then stops for good, the source is read into a
// tree, and the decode carries on from the document in hand. A document the
// walk had started is decoded again from the start, over what the walk wrote.
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
// One Decoder may be handed different destinations for the documents of one
// stream, and only the first of them decided how the source was read.
func (d *Decoder) buildTree() error {
	file, err := d.parse(d.src)
	if err != nil {
		d.streamErr = err

		return err
	}
	d.parsedFile = file

	return nil
}

func (d *Decoder) decode(v reflect.Value) error {
	if d.walkedFirst {
		// Unmarshal's destination was filled as the parse read the source.
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
	if len(d.parsedFile.Docs) == 0 {
		// empty document.
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
	if isEmptyDocument(doc) {
		// A document written with "---" or "..." and holding nothing holds the
		// empty node, which decodes to null. Leaving the stream index where it
		// was returned the document before it again and never reached the ones
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
	// Left where it was, the index handed the same document and its error to
	// every Decode after it, and a Decode loop never reached the end.
	err := d.decodeValue(v.Elem(), body)
	d.streamIndex++

	return err
}

// stringType is the string type, for reading a mapping key as text.
var stringType = reflect.TypeFor[string]()

// dynamicTypeOf returns the type inside an interface value, or v's own type
// where v is not one. It names what a key actually holds in an error about a
// map[any]any.
func dynamicTypeOf(v reflect.Value) reflect.Type {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		return v.Elem().Type()
	}

	return v.Type()
}
