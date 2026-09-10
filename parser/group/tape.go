package group

import (
	"github.com/go-openapi/go-yaml/token"
)

// TapeToken is one token as the grouping sees it: the token the scanner read, and
// the group it was joined into where a pass joined it.
//
// The scanner's token is held here by value rather than by pointer, so a token
// of a document is one thing on the tape and not two. The tree points into it
// with RawToken, which stays good for as long as the chunk it sits in does.
//
// A pass turns a token into a group in place, by hanging the group on it. Group
// therefore answers before raw does: raw is what this token was read as, and
// Group what it became.
type TapeToken struct {
	raw   token.Token
	Group *TokenGroup
	// seq is where this token stands on the tape, counted from the first the
	// scanner handed over. A walk tells the arena how far the descent has read
	// with it, and the arena reclaims what is behind that.
	seq int32
}

// NewSynthetic returns a token holding tk, for a token the parse makes rather
// than reads: an implicit null, or one standing in for a tag's absent value.
//
// It is off the tape, so it outlives whatever the tail does. There are few of
// them and each is one small allocation.
func NewSynthetic(tk *token.Token) *TapeToken {
	if tk == nil {
		return nil
	}

	return &TapeToken{raw: *tk}
}

// Raw fills this token in from what the scanner read.
func (t *TapeToken) Raw(tk token.Token, seq int) {
	t.raw, t.Group, t.seq = tk, nil, int32(seq)
}

// Seq returns where this token stands on the tape.
//
// A grouping pass turns a token into a group by hanging the group on it and
// clearing the raw token, so a group token keeps the place the token it was
// made from held -- which is where the group begins. Its own seq is therefore
// the answer, and the group is only read where a token was made by the grouping
// rather than drawn from the stream, which leaves seq at zero.
func (t *TapeToken) Seq() int32 {
	if t == nil {
		return 0
	}
	if t.seq > 0 {
		return t.seq
	}
	if t.Group != nil {
		return t.Group.First().Seq()
	}

	return 0
}

func (t *TapeToken) RawToken() *token.Token {
	if t == nil {
		return nil
	}
	t.checkLive("TapeToken.RawToken")
	if t.Group != nil {
		return t.Group.RawToken()
	}

	return &t.raw
}

func (t *TapeToken) Type() token.Type {
	if t == nil {
		return 0
	}
	t.checkLive("TapeToken.Type")
	if t.Group != nil {
		return t.Group.TokenType()
	}

	return t.raw.Type
}

func (t *TapeToken) GroupType() TokenGroupType {
	if t == nil {
		return TokenGroupNone
	}
	t.checkLive("TapeToken.GroupType")
	if t.Group == nil {
		return TokenGroupNone
	}

	return t.Group.Type
}

func (t *TapeToken) Line() int {
	if t == nil {
		return 0
	}
	if t.Group != nil {
		return t.Group.Line()
	}

	return int(t.raw.Position.Line)
}

func (t *TapeToken) Column() int {
	if t == nil {
		return 0
	}
	if t.Group != nil {
		return t.Group.Column()
	}

	return int(t.raw.Position.Column)
}

func (t *TapeToken) SetGroupType(typ TokenGroupType) {
	if t.Group == nil {
		return
	}
	t.Group.Type = typ
}

/*
func (t *TapeToken) Dump() {
	ctx := new(groupTokenRenderContext)
	if t.Group == nil {
		fmt.Fprint(os.Stdout, t.raw.Value)

		return
	}
	t.Group.dump(ctx)
	fmt.Fprintf(os.Stdout, "\n")
}

func (t *TapeToken) dump(ctx *groupTokenRenderContext) {
	if t.Group == nil {
		fmt.Fprint(os.Stdout, t.raw.Value)

		return
	}
	t.Group.dump(ctx)
}
*/
