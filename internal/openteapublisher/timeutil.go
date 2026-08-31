// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import "time"

// timeLayout matches internal/repo's own convention -- UTC, no fractional
// seconds -- for the same reason: a stable, unambiguous stored format.
const timeLayout = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}
