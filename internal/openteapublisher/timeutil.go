// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"database/sql"
	"time"
)

// timeLayout matches internal/repo's own convention -- UTC, no fractional
// seconds -- for the same reason: a stable, unambiguous stored format.
const timeLayout = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

// parseNullTime is parseTime for an optional column (e.g.
// cicd_credential.revoked_at) -- mirrors internal/repo's own
// parseNullTime exactly (nil for SQL NULL, never the zero time.Time,
// which would be indistinguishable from "revoked at the Unix epoch").
func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
