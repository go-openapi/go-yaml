// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import "github.com/go-openapi/conformance-suites/stance"

// SeparationShapes records where a tab may stand and where it may not.
//
// # One rule, and everything here follows from it
//
// 6.1 puts a tab in s-white and keeps it out of s-indent. So a tab separates
// exactly as a space does -- between an indicator and what follows it, between
// a node's properties and the node, before a comment -- and indents nowhere at
// all. A document that indents with one is not YAML; a document that separates
// with one is the same document as the one written with a space.
//
// # Why the family exists
//
// The generated corpus reached none of this until 2026-09-07. yamlgen wrote
// spaces everywhere, and grammar coverage could not report the gap: a tab and a
// space enter the same productions, so 605 of 605 buckets were entered with the
// whole distinction untouched. The census that compares the corpus against the
// YAML Test Suite found it -- 56 suite documents hold a tab and no generated
// document did.
//
// Style.TabSeparation draws them now. These are the two the drawing found, kept
// as documents so they survive a regeneration, and they carry a meaning rather
// than a stance because 6.1 settles them and every other implementation agrees.
//
// Both were defects when they were written down and both were fixed the next
// day, in `a0182a6`. The shapes stay: what they record is what 6.1 requires, and
// that does not change when a library starts agreeing with it.
// TestFixedATabSeparatesAsASpaceDoes is what the fix left behind.
func SeparationShapes() []stance.Shape {
	return []stance.Shape{
		{
			// The tag ends at the tab rather than swallowing it. Refused here
			// as `found invalid tag character` until a0182a6.
			Name:   "a tab between a tag and its node",
			Src:    []byte("a: !!str\tx\n"),
			Intent: []stance.Tag{TagSeparatedByTab},
			Means:  map[string]any{"a": "x"},
		},
		{
			// The worse of the two while it stood: this one answered. The
			// anchor was cut as "nx" on the empty node, so the value was gone
			// with no error reported, and `b: *n` named nothing and took the
			// whole document down.
			Name:   "a tab between an anchor and its node",
			Src:    []byte("a: &n\tx\nb: *n\n"),
			Intent: []stance.Tag{TagSeparatedByTab},
			Means:  map[string]any{"a": "x", "b": "x"},
		},
		{
			// The control, and it earns its place: a tab where no property
			// stands is read correctly, so the fault is the properties and not
			// the separation. Without this the family would look like "tabs are
			// broken", which is not what was measured.
			Name:   "a tab where no property stands",
			Src:    []byte("a:\tx\n"),
			Intent: []stance.Tag{TagSeparatedByTab},
			Means:  map[string]any{"a": "x"},
		},
	}
}

// SeparationVocabulary says at which stage a tab question is answered.
func SeparationVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		// A tab is separation or it is not, and the scanner decides before
		// anything is built.
		TagSeparatedByTab: stance.Parse,
	}
}

// TagSeparatedByTab is a document separating with a tab where a space would do.
const TagSeparatedByTab stance.Tag = "separation/tab"
