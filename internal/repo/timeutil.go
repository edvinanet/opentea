package repo

import (
	"database/sql"
	"time"
)

// timeLayout matches the spec's date-time schema pattern
// (^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$) -- UTC, no fractional seconds.
const timeLayout = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
