// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// leaveRecorder records the token each flow collection and each tag carries when the walk enters it, and compares it
// when the walk leaves it.
type leaveRecorder struct {
	entered []string
	changed []string
}

func (r *leaveRecorder) Enter(node ast.Node, _ parser.Cursor) error {
	if isTracked(node) {
		r.entered = append(r.entered, tokenAt(node))
	}

	return nil
}

func (r *leaveRecorder) Leave(node ast.Node, _ parser.Closing) error {
	if !isTracked(node) {
		return nil
	}
	want := r.entered[len(r.entered)-1]
	r.entered = r.entered[:len(r.entered)-1]
	if got := tokenAt(node); got != want {
		r.changed = append(r.changed, fmt.Sprintf("entered on %s, left on %s", want, got))
	}

	return nil
}

// isTracked reports whether node is a flow mapping, a flow sequence or a tag.
//
// A block mapping is left out: its token is its first key when the walk enters it and that key's ':' when the walk
// leaves it, whatever the document's length.
func isTracked(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.MappingNode:
		return n.IsFlowStyle
	case *ast.SequenceNode:
		return n.IsFlowStyle
	case *ast.TagNode:
		return true
	default:
		return false
	}
}

func tokenAt(node ast.Node) string {
	tk := node.GetToken()
	if tk == nil {
		return "no token"
	}

	return fmt.Sprintf("%q at %d:%d", tk.Value, tk.Position.Line, tk.Position.Column)
}

// TestAWalkLeavesAFlowCollectionOnItsOwnToken holds the token a flow collection or a tag hands to the visitor's Leave:
// the '{' or '[' it opened on, or the tag, as at Enter.
//
// A walk moves the tape's tail past the tokens of each entry it hands over, and the chunks behind the tail are
// refilled. A block mapping and a block sequence keep the chunk their run opened in, through holdRun, and a flow
// collection and a tag kept nothing: on a node long enough to refill the chunk its first token stood in, Leave read
// whatever token had taken its place. "{k0: 0, ..., k199: 199}" was left on a ',' at column 1122, and a fresh parser
// and a reset one, whose chunks were already grown, read two different tokens there.
func TestAWalkLeavesAFlowCollectionOnItsOwnToken(t *testing.T) {
	flowMap := func(n int) string {
		entries := make([]string, n)
		for i := range entries {
			entries[i] = fmt.Sprintf("k%d: %d", i, i)
		}

		return "{" + strings.Join(entries, ", ") + "}"
	}
	blockMap := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "# c%d\nk%d: %d # e%d\n", i, i, i, i)
		}

		return b.String()
	}
	flowSeq := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = strconv.Itoa(i)
		}

		return "[" + strings.Join(items, ", ") + "]"
	}

	for name, source := range map[string]string{
		"a flow mapping":                    flowMap(2000) + "\n",
		"a flow sequence":                   flowSeq(2000) + "\n",
		"a flow mapping under a block key":  "a: " + flowMap(2000) + "\nb: 1\n",
		"a flow sequence in a flow mapping": "{a: " + flowSeq(2000) + ", b: 1}\n",
		"a flow mapping in a block entry":   "- " + flowMap(2000) + "\n- 1\n",
		"a tag over a block mapping":        "a: 1\n---\n!!map\n" + blockMap(500),
		"a tag over a flow sequence":        "!!seq " + flowSeq(2000) + "\n",
		"a local tag over a flow mapping":   "k: !t " + flowMap(2000) + "\n",
		"a tag over a commented block mapping after a merge": "%YAML 1.2\n---\n&a1\n# c1\n<<: # c2\n  # c3\n" +
			"  \"\\\"a\": -6 # c4\n# c5\n\"\\\"a\": -1.0334519141370140801 # c6\n# c7\n" +
			"\"0b1010\": -5.3220642200655310211196996175146706005915890 # c8\n# c9\n\"4\": .nan # c10\n---\n" +
			"!!map\n# c11\n\"base\": &m1 # c12\n  # c13\n  \"b0\": 0x6 # c14\n  # c15\n  \"b1\": -3 # c16\n" +
			"  # c17\n  \"b2\": -2 # c18\n# c19\n\"item\": # c20\n  # c21\n" +
			"  !!merge <<: !!seq [{\"c0\": -6, \"c1\": -2}, *m1] # c22\n  # c23\n  \"b0\": 0x63 # c24\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, opts := range [][]parser.Option{nil, {parser.WithComments()}} {
				r := &leaveRecorder{}
				_, err := parser.New(opts...).Walk([]byte(source), r)
				require.NoError(t, err)
				assert.Empty(t, r.entered, "every collection entered is left")
				assert.Empty(t, r.changed)
			}
		})
	}
}
