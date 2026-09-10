// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package testscanner provides testing utilities to the scanner package.
//
// It holds [RunCases], which compares a scan against a table of expected tokens, and [WorkloadDocs], which the
// benchmarks and the property tests read.
//
// Nothing here imports the scanner. Both test packages reach it that way: plain_test.go and printable_test.go are
// package scanner files, and a scanner import here would close a cycle for them. [RunCases] takes the scan as a
// parameter for the same reason.
//
// The scanner's defect ledgers moved to github.com/go-openapi/go-yaml/internal/ledgers/scanner, which cannot import
// this package.
package testscanner
