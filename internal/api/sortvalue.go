// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"strconv"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// sortTimeLayout must match internal/repo's timeLayout exactly: cursor
// values are compared against the DB's TEXT columns as plain strings, so
// the formatting has to be byte-identical.
const sortTimeLayout = "2006-01-02T15:04:05Z"

func formatSortTime(t time.Time) string {
	return t.UTC().Format(sortTimeLayout)
}

// productReleaseSortValue returns the string form of pr's value for the
// given (already-validated) sortField, for building an opaque page cursor.
func productReleaseSortValue(pr tea.ProductRelease, sortField string) string {
	switch sortField {
	case "releaseDate":
		if pr.ReleaseDate != nil {
			return formatSortTime(*pr.ReleaseDate)
		}
		return ""
	case "version":
		return pr.Version
	default: // "createdDate"
		return formatSortTime(pr.CreatedDate)
	}
}

func componentReleaseSortValue(cr tea.ComponentRelease, sortField string) string {
	switch sortField {
	case "releaseDate":
		if cr.ReleaseDate != nil {
			return formatSortTime(*cr.ReleaseDate)
		}
		return ""
	case "version":
		return cr.Version
	default: // "createdDate"
		return formatSortTime(cr.CreatedDate)
	}
}

func collectionSortValue(c tea.Collection) string {
	return strconv.Itoa(c.Version)
}
