// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package structfields

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const (
	// structTagName names the struct tag read by Marshal and Unmarshal: `yaml:"name,flags"`.
	structTagName = "yaml"
	// jsonTagName names the tag read by encoding/json.
	// [github.com/go-openapi/go-yaml/codec.UseJSONTags] reads it where a field has no `yaml` tag.
	jsonTagName = "json"
)

// Mode selects which of encoding/json's rules a decode adds to the `yaml` tag,
// as a pair of independent bits.
//
// The `yaml` tag is read in every mode and always wins. The two bits add the rules a `yaml` tag has no spelling for:
//
//   - encoding/json promotes the fields of an untagged embedded struct into the outer one.
//     YAML needs an explicit `yaml:",inline"` for that.
//   - encoding/json names an untagged field after its Go name. go.yaml.in/yaml/v3 lowercases the name.
//
// A type tagged for JSON alone has no inline marker anywhere. go-openapi/spec is one: every field has a `json:` tag.
// Read without encoding/json's embedding rule, such a type leaves its embedded structs empty.
type Mode uint8

const (
	// YAMLTags reads `yaml` alone and inlines an embedded struct only where a
	// tag asks for it, as go.yaml.in/yaml/v3 does. The default, and the mode with
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

// infersNames reports whether mode names a field without a tag after its Go name,
// instead of the lowercased name.
func (m Mode) infersNames() bool { return m&InferredNames != 0 }

// tagSource records which tag gave a field its name.
//
// The conflict rule in dominant reads it twice:
//
//   - at the same depth, a field named by a tag beats a field named after its Go name;
//   - a conflict over a name that a `yaml` tag wrote is an error, and encoding/json's rule does not settle it.
type tagSource uint8

const (
	// nameFromGo marks a field named by no tag, which takes its Go name.
	nameFromGo tagSource = iota
	// nameFromYAML marks a field named by its `yaml` tag.
	nameFromYAML
	// nameFromJSON marks a field named by its `json` tag, under [github.com/go-openapi/go-yaml/codec.UseJSONTags].
	nameFromJSON
)

// Info holds what the struct tags set on one field of a struct: the name it is written under, and its flags.
//
// The flags follow the `yaml` tag: `omitempty`, `omitzero`, `flow` and `inline`, then `anchor`, `anchor=name`,
// `alias` and `alias=name`, which set IsAutoAnchor, AnchorName, IsAutoAlias and AliasName.
type Info struct {
	// Index gives the field's position in the struct, for reflect.Value.Field.
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
	// namedBy records which tag wrote RenderName, for the conflict rule.
	namedBy tagSource
}

// getTag returns the tag naming field under mode, and whether it came from the
// `yaml` tag.
//
// The `yaml` tag always wins. The default mode reads it and nothing else, as go.yaml.in/yaml/v3 does.
// A type tagged for encoding/json alone then takes the lowercased Go name for every field.
// [github.com/go-openapi/go-yaml/codec.UseJSONTags] adds the `json` tag where a field has no `yaml` tag, and changes nothing where it has one.
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

// inlineTarget classifies the type held by a `,inline` field.
type inlineTarget uint8

const (
	// notInlinable marks a type that `,inline` cannot apply to.
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

// InlineReadsItsOwnNode reports whether a `,inline` field of type t decodes the whole mapping itself.
//
// It does when t implements one of the unmarshalers passed to [SelfDecoding].
// Such a field gets no entry in Fields.flat, and the decoder's decodeInlineFields passes it the whole mapping.
// The decoder fills the other fields of the struct from the same mapping, as go.yaml.in/yaml/v3 does.
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
// Each tag keeps its own vocabulary.
// A `yaml` tag takes v3's flags and the five this library adds, and any other flag is an error.
// A `json` tag takes the two flags encoding/json defines, and ignores any other.
//
// `json:",inline"` therefore inlines nothing, because encoding/json has no such option.
// Under [github.com/go-openapi/go-yaml/codec.UseJSONTags], promotesUntaggedEmbedded promotes an anonymous struct without a flag.
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

// nameSource returns the source of a field's name.
// It returns nameFromGo where the tag gave no name and the Go name is used,
// and otherwise the tag that getTag read the name from.
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
// Any other flag is an error, as in v3. An empty flag is one too, so "a," and "a,,flow" are both rejected.
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
			// "anchor=" and "alias=" land here too: they carry no name for the two cases above to cut off.
			return fmt.Errorf("unsupported flag %q in tag %q", opt, tag)
		}
	}

	return nil
}

// readJSONFlags applies the flags a `json` tag may carry, and ignores the rest
// as encoding/json ignores one it does not define.
//
// encoding/json has three: `omitempty`, `omitzero` and `string`. The first two
// mean here what they mean there. `string` is not supported yet:
// a number field reads both a number and a quoted number, where encoding/json reads only the quoted one.
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

// Ignored reports whether the codec skips field: an unexported field that is not embedded,
// or a field whose tag under mode is "-".
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

// HasMergeProperty reports whether a field is tagged `,omitempty,inline,alias`.
// The tree decoder then skips the "<<" key of the mapping it reads into the struct.
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
// field maps, and a decoder using one must not get the other's.
// One map per mode keeps reflect.Type as the key. A reflect.Type is already an interface,
// but a struct holding the type and the mode would be boxed at every lookup.
var structFieldMaps [ModeCount]Cache[*Fields]

// Fields holds one type's fields, or the error raised by its tags. Both are
// kept, so a type with a duplicated field name is refused as fast as one that
// reads.
type Fields struct {
	// ByGoName holds every field, keyed by Go field name.
	ByGoName InfoMap
	// byRenderName maps a key name to its field, for a decode. ByGoName is keyed by Go field name, for an encode.
	// An embedded field is left out: it takes the whole mapping, not the entry that would match its own name.
	byRenderName map[string]*Info
	// Inline lists the embedded fields, which the tree decoder reads after the
	// rest.
	Inline []*Info
	// flat maps a key to its field, through any embedded structs in between.
	// Both decode paths read it, so both place an entry in the same field.
	// It is nil where the type embeds nothing.
	flat map[string]Flat
	// dropped holds the names that reach no field, because the conflict rule picked none.
	// flat cannot record them by leaving them out:
	// a name missing from flat may also be a name no field declares,
	// such as a key held at run time by an embedded struct's own `,inline` map.
	dropped map[string]struct{}
	err     error
}

// MapOf returns the fields of structType, keyed by Go field name.
//
// A type's fields never change, so MapOf reads them once and returns the same map afterwards.
// Reading them for every decoded value was the largest single cost of decoding into Go types.
// Decoding citm_catalog into its own structs allocated 73,708 times here per document,
// out of 243,679 allocations for the whole decode.
// Each value cost an InfoMap, a renderNameMap and an Info,
// and strings.Split ran 21,627 times on tags already split thousands of times.
//
// Every decode and encode of the type shares the map and the Info values in it, so no caller may write to them.
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
// A key matches a name exactly or matches nothing, in every mode, as in go.yaml.in/yaml/v3 and encoding/json/v2.
// The result's At is nil where the field is in the type itself and there is no index path to walk.
func (r *Fields) Lookup(name string) (Flat, bool) {
	if r.flat == nil {
		if own, named := r.byRenderName[name]; named {
			return Flat{SF: own, Ord: own.Index}, true
		}
	} else if ff, named := r.flat[name]; named {
		return ff, true
	}

	return Flat{}, false
}

// Claims reports whether some field of the type answers to name.
func (r *Fields) Claims(name string) bool {
	_, ok := r.Lookup(name)

	return ok
}

// WritesPromoted reports whether an entry encoded by the embedded struct at index
// belongs in the mapping around it.
//
// flat holds the one field each name reaches.
// A name that flat places under another index is already written by a shallower field.
// A name in dropped reaches no field, because the conflict rule picked none.
// Writing either name would put a key in the document that a decode then ignores or rejects.
//
// A name in neither map is a key no struct field declares, such as one from an embedded struct's own `,inline` map.
// WritesPromoted returns true for it.
func (r *Fields) WritesPromoted(name string, index int) bool {
	if ff, known := r.flat[name]; known {
		return len(ff.At) > 0 && ff.At[0] == index
	}
	_, gone := r.dropped[name]

	return !gone
}

// RenderNameOf returns the name a mapping key writes for the Go field called
// fieldName, following the embedded structs.
//
// A validator names a rejected field by its Go name.
// The entry that wrote the field is found by the name used in the document.
// A promoted field is missing from ByGoName, which holds only the type's own fields, so RenderNameOf reads flat too.
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
	return r
}

// Flat describes the field reached by a mapping key.
//
// It holds the field's location At, its tag information SF, and an ordinal index Ord.
// The ordinal index Ord starts from zero, and numbers the key names of the type.
//
// The tree decoder uses Ord as a bit position. It sets one bit for each field written by a mapping,
// to tell an own entry from a later "<<" merge.
// An index path cannot serve as a bit position. Neither can a leaf's own Index,
// because two embedded structs can both have a field at index zero.
type Flat struct {
	At  []int
	SF  *Info
	Ord int
}

// flatCandidate describes one field that a mapping key could reach, and the tag that gave the field its name.
// Its depth is the length of at.
type flatCandidate struct {
	at      []int
	sf      *Info
	namedBy tagSource
}

// flattener collects every field each name reaches, following the embedded structs.
// It holds the state that stays fixed as walk descends.
// resolve then settles each name that more than one field answers to.
type flattener struct {
	root       reflect.Type
	candidates map[string][]flatCandidate
	onPath     map[reflect.Type]bool
	mode       Mode
}

// walk records the names t reaches at the index path at, then descends into the
// structs t inlines.
//
// walk follows every path, so a type embedded down two branches adds its fields twice, and resolve sees the collision.
// encoding/json walks such a type once and records each of its fields twice, which gives the same result.
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
// A name that only one field answers to goes to that field.
// dominant picks the field for each of the other names.
// resolve calls dominant in sorted name order, so a type with two contested names always fails on the same one.
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

	// resolve numbers the names in sorted order, so every read of the type numbers them the same way.
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

// dominant picks one of the fields reached by name, or returns an error for the type.
//
// Two rules run before encoding/json's:
//
//   - go.yaml.in/yaml/v3 rejects a type where one name reaches two fields, so the default mode rejects it here.
//   - A contested name that a `yaml` tag wrote is an error in every mode.
//     Neither v3 nor encoding/json defines a type that mixes the two vocabularies,
//     and dominant rejects it instead of inventing an answer.
//
// encoding/json's rule then applies, in three clauses:
//
//   - the shallowest candidate wins;
//   - at equal depth, the candidate named by a tag wins, if exactly one is;
//   - otherwise the name reaches no field, and dominant returns nil.
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
// inlineMaps counts the maps seen so far in structType, and checkInlineTarget increments it.
// Only one map can take the entries that no field claims. A second map would have no rule to decide its entries.
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
