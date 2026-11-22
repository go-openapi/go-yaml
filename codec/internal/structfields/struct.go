// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package structfields

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

const (
	// structTagName is the struct tag key Marshal and Unmarshal read: `yaml:"name,flags"`.
	structTagName = "yaml"
	// jsonTagName is the tag encoding/json reads, which [github.com/go-openapi/go-yaml/codec.UseJSONTags] adds where
	// a field carries no `yaml` tag.
	jsonTagName = "json"
)

// Mode says which of encoding/json's rules a decode adds to the `yaml` tag,
// as a pair of independent bits.
//
// The `yaml` tag is read in every mode and always wins. What the two bits add
// is everything a `yaml` tag has no spelling for. encoding/json promotes the
// fields of an untagged embedded struct into the outer one, where YAML needs an
// explicit `yaml:",inline"`; and it names an untagged field after its Go name,
// where go.yaml.in/yaml/v3 lowercases it. A type tagged for JSON alone --
// go-openapi/spec is one, and every field of it is `json:` -- carries no inline
// marker anywhere, so reading its tags without its embedding rule fills nothing.
type Mode uint8

const (
	// YAMLTags reads `yaml` alone and inlines an embedded struct only where a
	// tag says to, as go.yaml.in/yaml/v3 does. The default, and the mode with
	// neither bit set.
	YAMLTags Mode = 0
	// JSONTags reads `json` where a field carries no `yaml` tag, and promotes an
	// untagged embedded struct, as encoding/json does. [github.com/go-openapi/go-yaml/codec.UseJSONTags] sets it.
	JSONTags Mode = 1 << 0
	// InferredNames names a field that no tag names after its Go name,
	// verbatim, where every other mode lowercases it. [github.com/go-openapi/go-yaml/codec.UseInferredNames] sets
	// it.
	InferredNames Mode = 1 << 1
	// ModeCount sizes the per-mode caches, one for each pair of bits.
	ModeCount = (JSONTags | InferredNames) + 1
)

// readsJSONTags reports whether mode reads the `json` tag and applies
// encoding/json's field selection to what it names.
func (m Mode) readsJSONTags() bool { return m&JSONTags != 0 }

// infersNames reports whether mode names a field no tag names after its Go
// name rather than after the lowercased one.
func (m Mode) infersNames() bool { return m&InferredNames != 0 }

// tagSource says which tag gave a field the name it writes.
//
// encoding/json's conflict rule needs it twice over: a candidate a tag names
// beats one named after its Go name at the same depth, and a name some `yaml`
// tag wrote takes the whole type out of encoding/json's reach.
type tagSource uint8

const (
	// nameFromGo is a field no tag names, which takes its Go name.
	nameFromGo tagSource = iota
	// nameFromYAML is a field the `yaml` tag names.
	nameFromYAML
	// nameFromJSON is a field the `json` tag names, under [github.com/go-openapi/go-yaml/codec.UseJSONTags].
	nameFromJSON
)

// Info holds what the struct tags say about one field of a struct: the name it is written under, and its flags.
type Info struct {
	// Index is where the field stands in the struct, for reflect.Value.Field.
	// FieldByName walks the type's fields and compares names on every call.
	Index        int
	FieldName    string
	RenderName   string
	AnchorName   string
	AliasName    string
	IsAutoAnchor bool
	IsAutoAlias  bool
	IsOmitEmpty  bool
	IsOmitZero   bool
	IsFlow       bool
	IsInline     bool
	// namedBy says which tag wrote RenderName, which the conflict rule reads.
	namedBy tagSource
}

// getTag returns the tag naming field under mode, and whether it came from the
// `yaml` tag.
//
// The `yaml` tag always wins. The default mode reads it and nothing else, as
// go.yaml.in/yaml/v3 does, so a type tagged for encoding/json alone takes the
// lowercased Go name for every field. [github.com/go-openapi/go-yaml/codec.UseJSONTags] adds `json` where a field
// carries no `yaml` tag, and takes nothing away from one that does.
//
// Callers need to know which tag they got, because the two vocabularies part
// company on a flag neither library defines: v3 refuses the type, encoding/json
// ignores the flag.
func getTag(field reflect.StructField, mode Mode) (string, bool) {
	if tag := field.Tag.Get(structTagName); tag != "" {
		return tag, true
	}
	if mode.readsJSONTags() {
		return field.Tag.Get(jsonTagName), false
	}

	return "", true
}

// promotesUntaggedEmbedded reports whether field is an embedded struct that
// mode reads as inline without being told to.
//
// encoding/json promotes an anonymous field of struct type that its tag does
// not name. An anonymous field of any other kind is an ordinary field there,
// and so here.
func promotesUntaggedEmbedded(field reflect.StructField, name string, mode Mode) bool {
	if !mode.readsJSONTags() || !field.Anonymous || name != "" {
		return false
	}
	t := field.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Kind() == reflect.Struct
}

// inlineTarget says what a `,inline` field holds.
type inlineTarget uint8

const (
	// notInlinable is anything `,inline` cannot be written on.
	notInlinable inlineTarget = iota
	// inlinedStruct has its fields promoted into the outer struct.
	inlinedStruct
	// inlinedMap takes the entries no field of the outer struct claims.
	inlinedMap
)

// inlineTargetOf reports what go.yaml.in/yaml/v3 accepts after `,inline`: a
// struct, or a chain of pointers ending in one; or a map, which must be the
// field's own type. v3 follows a pointer to a struct and not one to a map, so
// `*map[string]any` is refused there and here.
func inlineTargetOf(t reflect.Type) inlineTarget {
	if t.Kind() == reflect.Map {
		return inlinedMap
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		return inlinedStruct
	}

	return notInlinable
}

// InlineTakesUnclaimedEntries reports whether a `,inline` field of type t
// collects the entries no other field of the struct claims. Only a map does.
//
// go.yaml.in/yaml/v3 fills such a map with the keys its FieldsMap does not
// name, and leaves it nil where every key is named. Fields.flat holds the
// same set here: the struct's own fields, and everything its embedded structs
// promote.
func InlineTakesUnclaimedEntries(t reflect.Type) bool {
	return inlineTargetOf(t) == inlinedMap
}

// selfDecoding holds the interfaces through which a type decodes the node it is handed. See [SelfDecoding].
var selfDecoding []reflect.Type

// SelfDecoding sets the interfaces through which a type decodes the node it is handed, the decoder's unmarshalers.
// A `,inline` field whose type implements one reads the mapping itself, and its fields are not promoted.
//
// The decoder calls it once, at init, before any type is read: the fields read for a type are cached, so a later
// call would not change them.
func SelfDecoding(interfaces ...reflect.Type) { selfDecoding = interfaces }

// InlineReadsItsOwnNode reports whether a `,inline` field of type t decodes the
// mapping itself rather than having its fields filled one at a time.
//
// go.yaml.in/yaml/v3 keeps such a field out of the flattened field list and
// hands it the whole node, filling the rest of the struct besides. Any type
// implementing one of the unmarshalers this decoder honors is that case, so
// its fields stay out of Fields.flat and decodeInlineFields gives it the
// mapping.
func InlineReadsItsOwnNode(t reflect.Type) bool {
	pointer := reflect.PointerTo(t)
	for _, iface := range selfDecoding {
		if t.Implements(iface) || pointer.Implements(iface) {
			return true
		}
	}

	return false
}

const (
	anchorFlag = "anchor="
	aliasFlag  = "alias="
)

// structField reads one field's tag: the name it writes, and the flags after
// it.
//
// Each tag keeps its own vocabulary. A `yaml` tag takes v3's flags and the five
// this library adds, and refuses anything else; a `json` tag takes the two
// encoding/json defines, and ignores anything else. So `json:",inline"` names
// no field inline, because encoding/json has no such option -- an anonymous
// struct is promoted there by [github.com/go-openapi/go-yaml/codec.UseJSONTags] and the rule in
// promotesUntaggedEmbedded, not by a flag.
func structField(field reflect.StructField, index int, mode Mode) (*Info, error) {
	tag, fromYAML := getTag(field, mode)
	fieldName := strings.ToLower(field.Name)
	if mode.infersNames() {
		fieldName = field.Name
	}
	options := strings.Split(tag, ",")
	if options[0] != "" {
		fieldName = options[0]
	}
	sf := &Info{
		Index:      index,
		FieldName:  field.Name,
		RenderName: fieldName,
		namedBy:    nameSource(options[0], fromYAML),
	}
	if promotesUntaggedEmbedded(field, options[0], mode) {
		sf.IsInline = true
	}

	if !fromYAML {
		readJSONFlags(sf, options[1:])

		return sf, nil
	}

	return sf, readYAMLFlags(sf, tag, options[1:])
}

// nameSource says which tag wrote a field's name: none where the tag gave no
// name and the Go name stood in, and otherwise the tag getTag read it from.
func nameSource(name string, fromYAML bool) tagSource {
	switch {
	case name == "":
		return nameFromGo
	case fromYAML:
		return nameFromYAML
	default:
		return nameFromJSON
	}
}

// readYAMLFlags applies the flags a `yaml` tag may carry and refuses the rest.
//
// `omitempty`, `flow` and `inline` are go.yaml.in/yaml/v3's. `omitzero` is
// encoding/json's own since Go 1.24. `anchor`, `anchor=name`, `alias` and
// `alias=name` are this library's, for a YAML feature no v3 tag reaches.
// Anything else refuses the type, as v3 refuses it -- an empty flag included,
// so "a," and "a,,flow" are both refused.
func readYAMLFlags(sf *Info, tag string, flags []string) error {
	for _, opt := range flags {
		switch {
		case opt == "omitempty":
			sf.IsOmitEmpty = true
		case opt == "omitzero":
			sf.IsOmitZero = true
		case opt == "flow":
			sf.IsFlow = true
		case opt == "inline":
			sf.IsInline = true
		case opt == "anchor":
			sf.IsAutoAnchor = true
		case opt == "alias":
			sf.IsAutoAlias = true
		case len(opt) > len(anchorFlag) && strings.HasPrefix(opt, anchorFlag):
			sf.AnchorName = opt[len(anchorFlag):]
		case len(opt) > len(aliasFlag) && strings.HasPrefix(opt, aliasFlag):
			sf.AliasName = opt[len(aliasFlag):]
		default:
			// "anchor=" and "alias=" land here too, since neither carries the
			// name the two cases above cut off.
			return fmt.Errorf("unsupported flag %q in tag %q", opt, tag)
		}
	}

	return nil
}

// readJSONFlags applies the flags a `json` tag may carry, and ignores the rest
// as encoding/json ignores one it does not define.
//
// encoding/json has three: `omitempty`, `omitzero` and `string`. The first two
// mean here what they mean there. `string` is not read yet, so a number written
// as a string reads either way rather than only the one.
func readJSONFlags(sf *Info, flags []string) {
	for _, opt := range flags {
		switch opt {
		case "omitempty":
			sf.IsOmitEmpty = true
		case "omitzero":
			sf.IsOmitZero = true
		}
	}
}

func Ignored(field reflect.StructField, mode Mode) bool {
	if field.PkgPath != "" && !field.Anonymous {
		// private field
		return true
	}
	tag, _ := getTag(field, mode)

	return tag == "-"
}

// InfoMap holds the Info of each field of a struct, keyed by Go field name.
type InfoMap map[string]*Info

func (m InfoMap) HasMergeProperty() bool {
	for _, v := range m {
		if v.IsOmitEmpty && v.IsInline && v.IsAutoAlias {
			return true
		}
	}
	return false
}

// structFieldMaps holds the fields of each struct type read so far, one map per
// Mode and each keyed by reflect.Type.
//
// A type read for its `yaml` tags and for its `json` tags gives two different
// field maps, and a decoder using one must not be handed the other's. One map
// per mode keeps reflect.Type as the key: it is already an interface, where a
// struct holding the type and the mode would be boxed at every lookup.
var structFieldMaps [ModeCount]Cache[*Fields]

// Fields holds one type's fields, or the error its tags raised. Both are
// kept, so a type with a duplicated field name is refused as fast as one that
// reads.
type Fields struct {
	ByGoName InfoMap
	// byRenderName finds the field a mapping key names, which is the lookup a
	// decode makes. InfoMap is keyed by Go field name, which is the one
	// an encode makes. An embedded field is left out: it takes the whole
	// mapping rather than the entry its own name would match.
	byRenderName map[string]*Info
	// Inline lists the embedded fields, which the tree decoder reads after the
	// rest.
	Inline []*Info
	// flat finds the field a mapping key names through the embedded structs
	// standing between. Both decode paths read it, so both place an entry in
	// the same field. Nil where the type embeds nothing.
	flat map[string]Flat
	// folded finds the field a key names but for case, which encoding/json
	// accepts once an exact match has failed. Nil outside [github.com/go-openapi/go-yaml/codec.UseJSONTags], where
	// a key names a field exactly or names none.
	folded map[string]Flat
	// dropped holds the names the conflict rule left reaching no field at all,
	// which flat cannot record by leaving out: a name it does not hold may also
	// be one no field of this type declares, such as a key an embedded struct's
	// own `,inline` map carries at run time.
	dropped map[string]struct{}
	err     error
}

// MapOf returns the fields of structType, keyed by Go field name.
//
// A type's fields never change, so each is read once and handed out again
// afterwards. Reading them for every value decoded was the largest single cost
// of decoding into Go types: citm_catalog read into the structs it describes
// allocated 73,708 times in here per document -- a InfoMap, a
// renderNameMap and a Info per value, and 21,627 calls to strings.Split
// on tags already split thousands of times -- against 243,679 allocations for
// the whole decode.
//
// The map and the StructFields in it are shared between every decode and
// encode of the type, so no caller may write to them.
func MapOf(structType reflect.Type, mode Mode) (InfoMap, error) {
	r, err := Of(structType, mode)

	return r.ByGoName, err
}

// Of returns everything read from structType: the fields by Go name,
// by the name a mapping key writes, and the embedded ones on their own.
func Of(structType reflect.Type, mode Mode) (*Fields, error) {
	cache := &structFieldMaps[mode]
	if r, read := cache.Load(structType); read {
		return r, r.err
	}

	r := cache.LoadOrStore(structType, readType(structType, mode))

	return r, r.err
}

// Lookup finds the field a mapping key names.
//
// An exact match first, and under [github.com/go-openapi/go-yaml/codec.UseJSONTags] one differing only in case
// after that, which is the order encoding/json tries them in. The result's at
// is nil where the field stands in the type itself and there is no index path
// to walk.
func (r *Fields) Lookup(name string) (Flat, bool) {
	if r.flat == nil {
		if own, named := r.byRenderName[name]; named {
			return Flat{SF: own, Ord: own.Index}, true
		}
	} else if ff, named := r.flat[name]; named {
		return ff, true
	}

	if r.folded == nil {
		return Flat{}, false
	}
	ff, named := r.folded[foldName(name)]

	return ff, named
}

// Claims reports whether some field of the type answers to name.
func (r *Fields) Claims(name string) bool {
	_, ok := r.Lookup(name)

	return ok
}

// WritesPromoted reports whether an entry the embedded struct at index encoded
// belongs in the mapping around it.
//
// flat holds the one field each name reaches, so a name it places under another
// index is one a shallower field writes already, and a name in dropped is one
// the conflict rule left reaching nothing. Writing either would put a key in
// the document that reading it back would ignore or refuse.
//
// A name in neither is a key the type system never saw -- an embedded struct's
// own `,inline` map carries those -- and it is written.
func (r *Fields) WritesPromoted(name string, index int) bool {
	if ff, known := r.flat[name]; known {
		return len(ff.At) > 0 && ff.At[0] == index
	}
	_, gone := r.dropped[name]

	return !gone
}

// foldedNames keys every name a type answers to by its folded form.
//
// Two names folding alike keep the first in sorted order, so a type reads the
// same way twice. An exact match is tried before this in every case, so it only
// ever decides between names no document spelled exactly.
func foldedNames(r *Fields) map[string]Flat {
	names := make([]string, 0, max(len(r.byRenderName), len(r.flat)))
	if r.flat == nil {
		for name := range r.byRenderName {
			names = append(names, name)
		}
	} else {
		for name := range r.flat {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	out := make(map[string]Flat, len(names))
	for _, name := range names {
		folded := foldName(name)
		if _, taken := out[folded]; taken {
			continue
		}
		if r.flat == nil {
			sf := r.byRenderName[name]
			out[folded] = Flat{SF: sf, Ord: sf.Index}

			continue
		}
		out[folded] = r.flat[name]
	}

	return out
}

// foldName returns the key a case-insensitive lookup compares on: every rune
// folded to the smallest of those unicode.SimpleFold walks it through.
//
// For a letter that is its upper case, and it also puts the Kelvin sign on K
// and the long s on S, which is where encoding/json puts them. strings.ToUpper
// would leave both standing on their own, so a document writing "K" would miss
// a field called "k".
func foldName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		b.WriteRune(foldRune(r))
	}

	return b.String()
}

// foldRune returns the smallest rune in r's simple-fold cycle. A rune that
// folds to nothing else is its own cycle and comes back unchanged.
func foldRune(r rune) rune {
	lowest := r
	for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
		lowest = min(lowest, folded)
	}

	return lowest
}

// RenderNameOf returns the name a mapping key writes for the Go field called
// fieldName, following the embedded structs.
//
// A validator names the field it rejected by its Go name, and the entry that
// wrote it is found by the name the document used. A promoted field is not in
// fields, which holds the type's own, so flat is read as well.
func (r *Fields) RenderNameOf(fieldName string) (string, bool) {
	if sf, own := r.ByGoName[fieldName]; own && !sf.IsInline {
		return sf.RenderName, true
	}
	for _, ff := range r.flat {
		if ff.SF.FieldName == fieldName {
			return ff.SF.RenderName, true
		}
	}

	return "", false
}

func readType(structType reflect.Type, mode Mode) *Fields {
	fields, err := readStructFields(structType, mode)
	if err != nil {
		return &Fields{err: err}
	}

	r := &Fields{ByGoName: fields, byRenderName: make(map[string]*Info, len(fields))}
	for _, sf := range fields {
		if sf.IsInline {
			r.Inline = append(r.Inline, sf)

			continue
		}
		r.byRenderName[sf.RenderName] = sf
	}
	slices.SortFunc(r.Inline, func(a, b *Info) int { return a.Index - b.Index })
	if len(r.Inline) > 0 {
		f := &flattener{
			root:       structType,
			candidates: map[string][]flatCandidate{},
			onPath:     map[reflect.Type]bool{structType: true},
			mode:       mode,
		}
		if err := f.walk(structType, fields, r.Inline, nil); err != nil {
			return &Fields{err: err}
		}
		flat, dropped, err := f.resolve()
		if err != nil {
			return &Fields{err: err}
		}
		r.flat, r.dropped = flat, dropped
	}
	if mode.readsJSONTags() {
		r.folded = foldedNames(r)
	}

	return r
}

// Flat is the field one mapping key reaches: where it stands, what its
// tag said, and a number of its own.
//
// ord numbers the names a type answers to, from zero. The tree decoder marks
// what a mapping has written so it can tell an entry from a later "<<" merge,
// and an index path is no use as a bit position; a leaf's own Index is no use
// either, since two embedded structs both have a field at zero.
type Flat struct {
	At  []int
	SF  *Info
	Ord int
}

// flatCandidate is one field a mapping key could reach, and how it got its
// name. Its depth is the length of at.
type flatCandidate struct {
	at      []int
	sf      *Info
	namedBy tagSource
}

// flattener collects every field each name reaches, following the embedded
// structs. It carries what does not change as the walk descends, and resolve
// settles the names more than one field answers to.
type flattener struct {
	root       reflect.Type
	candidates map[string][]flatCandidate
	onPath     map[reflect.Type]bool
	mode       Mode
}

// walk records the names t reaches at the index path at, then descends into the
// structs t inlines.
//
// Every path is followed, so a type embedded down two branches contributes its
// fields twice and resolve sees the collision. encoding/json reaches the same
// verdict by walking such a type once and recording its fields twice over,
// which is the same arithmetic written differently.
//
// It reads each embedded type with readStructFields and not with Of,
// because Of calls this and the type being read is not in the cache
// yet. Two types embedding one another would otherwise never finish. onPath
// guards that cycle: it holds the types between root and here, and clears them
// on the way back out.
func (f *flattener) walk(t reflect.Type, fields InfoMap, inline []*Info, at []int) error {
	for _, sf := range fields {
		if sf.IsInline {
			continue
		}
		f.candidates[sf.RenderName] = append(f.candidates[sf.RenderName], flatCandidate{
			at:      append(append([]int{}, at...), sf.Index),
			sf:      sf,
			namedBy: sf.namedBy,
		})
	}

	for _, sf := range inline {
		embedded := t.Field(sf.Index).Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if embedded.Kind() != reflect.Struct || f.onPath[embedded] {
			continue
		}
		if InlineReadsItsOwnNode(t.Field(sf.Index).Type) {
			// Its fields are its own to fill, so no name here reaches them.
			continue
		}
		f.onPath[embedded] = true

		under, err := readStructFields(embedded, f.mode)
		if err != nil {
			return err
		}
		var deeper []*Info
		for _, sfe := range under {
			if sfe.IsInline {
				deeper = append(deeper, sfe)
			}
		}
		slices.SortFunc(deeper, func(a, b *Info) int { return a.Index - b.Index })
		if err := f.walk(embedded, under, deeper, append(at, sf.Index)); err != nil {
			return err
		}
		delete(f.onPath, embedded)
	}

	return nil
}

// resolve returns the field each name reaches, once every candidate is in.
//
// A name only one field answers to is that field's. The rest go through
// dominant, and the contested names are settled in order so that a type with
// two of them always reports the same one.
func (f *flattener) resolve() (map[string]Flat, map[string]struct{}, error) {
	out := make(map[string]Flat, len(f.candidates))
	contested := make([]string, 0, len(f.candidates))
	for name, cs := range f.candidates {
		if len(cs) == 1 {
			out[name] = Flat{At: cs[0].at, SF: cs[0].sf}

			continue
		}
		contested = append(contested, name)
	}
	slices.Sort(contested)

	var dropped map[string]struct{}
	for _, name := range contested {
		won, err := f.dominant(name, f.candidates[name])
		if err != nil {
			return nil, nil, err
		}
		if won == nil {
			if dropped == nil {
				dropped = map[string]struct{}{}
			}
			dropped[name] = struct{}{}

			continue
		}
		out[name] = Flat{At: won.at, SF: won.sf}
	}

	// The names are numbered in order, so a type reads the same way twice.
	numbered := make([]string, 0, len(out))
	for name := range out {
		numbered = append(numbered, name)
	}
	slices.Sort(numbered)
	for i, name := range numbered {
		ff := out[name]
		ff.Ord = i
		out[name] = ff
	}

	return out, dropped, nil
}

// dominant picks between the fields one name reaches, or refuses the type.
//
// Two rules run before encoding/json's. go.yaml.in/yaml/v3 refuses a type where
// one name reaches two fields, so the default mode refuses it here. And a name
// some `yaml` tag wrote is refused whatever the mode: a type mixing the two
// vocabularies is a case neither reference speaks to, and refusing it is
// narrower than inventing an answer.
//
// What is left is encoding/json's rule, in three clauses: the shallowest
// candidate wins; at equal depth the one a tag names wins, if exactly one does;
// and otherwise the name reaches no field at all, which a nil return says.
func (f *flattener) dominant(name string, cs []flatCandidate) (*flatCandidate, error) {
	refused := fmt.Errorf("duplicated key %q in struct %s", name, f.root)
	if !f.mode.readsJSONTags() {
		return nil, refused
	}
	for _, c := range cs {
		if c.namedBy == nameFromYAML {
			return nil, refused
		}
	}

	shallowest := len(cs[0].at)
	for _, c := range cs[1:] {
		shallowest = min(shallowest, len(c.at))
	}

	var top, tagged []flatCandidate
	for _, c := range cs {
		if len(c.at) == shallowest {
			top = append(top, c)
			if c.namedBy != nameFromGo {
				tagged = append(tagged, c)
			}
		}
	}
	switch {
	case len(top) == 1:
		return &top[0], nil
	case len(tagged) == 1:
		return &tagged[0], nil
	default:
		return nil, nil
	}
}

// checkInlineTarget refuses the three shapes go.yaml.in/yaml/v3 refuses after
// `,inline`: a field that is neither a struct nor a map, a map whose keys are
// not exactly string, and a second map in the same struct.
//
// inlineMaps counts the maps seen so far in structType, and this raises it.
// Only one map can take the entries no field claims, so a second one has no
// rule to fill it by.
func checkInlineTarget(structType reflect.Type, field reflect.StructField, inlineMaps *int) error {
	switch inlineTargetOf(field.Type) {
	case inlinedStruct:
		return nil
	case inlinedMap:
		if field.Type.Key() != reflect.TypeFor[string]() {
			return fmt.Errorf("option ,inline needs a map with string keys in struct %s", structType)
		}
		*inlineMaps++
		if *inlineMaps > 1 {
			return fmt.Errorf("multiple ,inline maps in struct %s", structType)
		}

		return nil
	default:
		return fmt.Errorf(
			"option ,inline may only be used on a struct or map field: %s.%s is %s",
			structType, field.Name, field.Type,
		)
	}
}

func readStructFields(structType reflect.Type, mode Mode) (InfoMap, error) {
	fieldMap := InfoMap{}
	renderNameMap := map[string]struct{}{}
	inlineMaps := 0
	for i := range structType.NumField() {
		field := structType.Field(i)
		if Ignored(field, mode) {
			continue
		}
		sf, err := structField(field, i, mode)
		if err != nil {
			return nil, fmt.Errorf("%w of type %s", err, structType)
		}
		if sf.IsInline {
			if err := checkInlineTarget(structType, field, &inlineMaps); err != nil {
				return nil, err
			}
		}
		if _, exists := renderNameMap[sf.RenderName]; exists {
			return nil, fmt.Errorf("duplicated key %q in struct %s", sf.RenderName, structType)
		}
		fieldMap[sf.FieldName] = sf
		renderNameMap[sf.RenderName] = struct{}{}
	}
	return fieldMap, nil
}
