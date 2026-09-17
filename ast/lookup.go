// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

// Lookup returns the entry of node whose key is name, and nil where node holds no such key.
//
// node is a mapping, or an anchor, a tag or an alias standing on one: a value read from a document carries
// whatever the document wrote in front of it, and an alias is followed through [AliasNode.Target].
//
// Keys are compared by [KeyName], so a quoted key matches its text and "1" and 1 are one name. A "<<" brings
// keys in from elsewhere and Lookup does not read them; use [LookupMerged] for that.
func Lookup(node Node, name string) *MappingValueNode {
	return LookupIn(mappingOf(node), name)
}

// LookupIn is [Lookup] over a mapping already in hand, such as one of [MergeEntry.Sources].
func LookupIn(mapping MapNode, name string) *MappingValueNode {
	if mapping == nil {
		return nil
	}

	for iter := mapping.MapRange(); iter.Next(); {
		if key, _ := KeyName(iter.Key()); key == name {
			return iter.KeyValue()
		}
	}

	return nil
}

// LookupMerged returns the entry of node whose key is name, reading the mappings a "<<" brings in where
// node itself does not write the key.
//
// The mapping's own keys win, and among the merged ones an earlier "<<" beats a later one, which is what
// the merge type requires. A merged mapping may merge in turn, and is read the same way.
func LookupMerged(node Node, name string) *MappingValueNode {
	mapping := mappingOf(node)
	if mapping == nil {
		return nil
	}
	if own := LookupIn(mapping, name); own != nil {
		return own
	}

	for iter := mapping.MapRange(); iter.Next(); {
		merge := MergeOf(iter.KeyValue())
		if merge.Verdict != Folds {
			continue
		}
		for _, source := range merge.Sources {
			if found := lookupMergedIn(source, name); found != nil {
				return found
			}
		}
	}

	return nil
}

// lookupMergedIn is [LookupMerged] over a mapping in hand, for a source that merges in its turn.
func lookupMergedIn(mapping MapNode, name string) *MappingValueNode {
	if own := LookupIn(mapping, name); own != nil {
		return own
	}
	if node, isNode := mapping.(Node); isNode {
		return LookupMerged(node, name)
	}

	return nil
}

// mappingOf is the mapping node stands on, through the properties a document may write in front of it, and
// nil where node is not a mapping.
func mappingOf(node Node) MapNode {
	switch n := node.(type) {
	case nil:
		return nil
	case *MappingNode:
		return n
	case *MappingValueNode:
		return n
	case *AnchorNode:
		return mappingOf(n.Value)
	case *TagNode:
		return mappingOf(n.Value)
	case *MappingKeyNode:
		return mappingOf(n.Value)
	case *AliasNode:
		return mappingOf(n.Target)
	default:
		return nil
	}
}
