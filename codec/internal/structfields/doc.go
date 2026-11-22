// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package structfields reads the fields a Go struct type shows to the codec: the name each is written under, its
// `yaml` flags, and the fields an embedded or `,inline` struct promotes, under the tag [Mode] a decode or an encode
// asks for. [Of] reads a type once per mode and caches what it read.
package structfields
