// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"slices"
	"time"
)

// decodeTimeCases covers timestamps decoded into time.Time and durations into time.Duration.
func decodeTimeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name: "timestamp/date",
			// Date only.
			source: "v: 2015-01-01\n",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "timestamp/fraction",
			// RFC3339
			source: "v: 2015-02-24T18:19:39.12Z\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, .12e9, time.UTC)},
		},
		{
			name: "timestamp/single-digit-fields",
			// With a time, the month, day and hour may take one digit. The minute and second take two.
			source: "v: 2015-2-3T3:04:05Z",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 3, 3, 4, 5, 0, time.UTC)},
		},
		{
			name: "timestamp/lowercase-t",
			// ISO8601 lower case t
			source: "v: 2015-02-24t18:19:39Z\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, 0, time.UTC)},
		},
		{
			name: "timestamp/space-separated",
			// space separate, no time zone
			source: "v: 2015-02-24 18:19:39\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, 0, time.UTC)},
		},
		{
			name:   "duration/seconds",
			source: "v: 60s\n",
			value:  map[string]time.Duration{"v": time.Minute},
		},
		{
			name:   "duration/negative-fractional-hours",
			source: "v: -0.5h\n",
			value:  map[string]time.Duration{"v": -30 * time.Minute},
		},
	})
}
