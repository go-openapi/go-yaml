// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build !yamlprobe

package group

// poisonLeaves and poisonGroups record the cells a release took back, and
// reviveLeaf and reviveGroup that one is in use again. They do nothing here;
// see poison_on.go.
func poisonLeaves(_ []TapeToken) {}

func poisonGroups(_ []TokenGroup) {}

func reviveLeaf(_ *TapeToken) {}

func reviveGroup(_ *TokenGroup) {}

// checkLive records that a cell was read after the grouper handed it back. at
// names the accessor that read it.
func (t *TapeToken) checkLive(_ string) {}

func (g *TokenGroup) checkLive(_ string) {}
