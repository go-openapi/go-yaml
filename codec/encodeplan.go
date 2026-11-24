// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"reflect"
	"strings"

	"github.com/go-openapi/go-yaml/codec/internal/structfields"
	"github.com/go-openapi/go-yaml/token"
)

// structPlan lists the fields encodeStruct writes for one struct type and one [structfields.Mode], in field order.
//
// encodeStruct used to work them out on every value: it read each field's tag again in [structfields.Ignored],
// found the field with reflect.Value.FieldByName, a search by name, and looked its Info up by Go name. The plan does
// all three once per type.
type structPlan struct {
	fields *structfields.Fields
	write  []fieldPlan
	// inlineAnchor records an inline field tagged as an anchor, which anchors the whole mapping. A streamed encode
	// builds such a struct whole.
	inlineAnchor bool
}

// fieldPlan is one field encodeStruct writes.
type fieldPlan struct {
	// index is the field's position in the struct, for reflect.Value.Field.
	index int
	info  *structfields.Info
	// plainKey records that the field's name needs no quotes under any option but [JSON]: it holds none of the
	// characters isNeedQuoted checks in a flow collection or for a literal block, and token.IsNeedQuoted passes it.
	plainKey bool
	// streamed records a field whose entry needs no look at its encoded value: no alias name, no anchor, not inline,
	// not flow. A streamed encode writes its key and streams its value; it runs structField for any other field.
	streamed bool
}

// structPlans caches structPlan by type, one cache per [structfields.Mode], as [structfields.Of] caches the fields.
var structPlans [structfields.ModeCount]structfields.Cache[*structPlan]

// planOf returns the plan of structType under mode, building it on first use. It returns the error of
// [structfields.Of] for a type whose tags do not read, and caches no plan for it.
func planOf(structType reflect.Type, mode structfields.Mode) (*structPlan, error) {
	if plan, ok := structPlans[mode].Load(structType); ok {
		return plan, nil
	}

	fields, err := structfields.Of(structType, mode)
	if err != nil {
		return nil, err
	}
	plan := &structPlan{fields: fields}
	for i := range structType.NumField() {
		field := structType.Field(i)
		if structfields.Ignored(field, mode) {
			continue
		}
		info := fields.ByGoName[field.Name]
		plan.write = append(plan.write, fieldPlan{
			index:    i,
			info:     info,
			plainKey: !token.IsNeedQuoted(info.RenderName) && !strings.ContainsAny(info.RenderName, "]},'\"\n:"),
			streamed: info.AliasName == "" && info.AnchorName == "" && !info.IsAutoAnchor && !info.IsInline && !info.IsFlow,
		})
		plan.inlineAnchor = plan.inlineAnchor || (info.IsInline && info.IsAutoAnchor)
	}

	return structPlans[mode].LoadOrStore(structType, plan), nil
}
