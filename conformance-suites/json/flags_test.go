// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import "flag"

// writeCorpus gates rewriting the stored artifact, so that regenerating it is
// always something somebody asked for.
var writeCorpus = flag.Bool("jsoncorpus.write", false,
	"rewrite the stored corpus from the current grammar and generator")
