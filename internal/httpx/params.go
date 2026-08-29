// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import (
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/oej/opentea/internal/idgen"
)

// ErrInvalidParam is returned by the Path*/Page*/SortField/IDFilter helpers
// below when a request parameter fails validation.
var ErrInvalidParam = errors.New("httpx: invalid parameter")

// PathUUID reads and validates a UUID path parameter (registered via
// net/http.ServeMux's {name} pattern syntax).
func PathUUID(r *http.Request, name string) (string, error) {
	v := r.PathValue(name)
	if !idgen.Valid(v) {
		return "", ErrInvalidParam
	}
	return v, nil
}

// PathPositiveInt reads and validates an integer path parameter that must be >= 1
// (used for collectionVersion / artifactVersion).
func PathPositiveInt(r *http.Request, name string) (int, error) {
	v, err := strconv.Atoi(r.PathValue(name))
	if err != nil || v < 1 {
		return 0, ErrInvalidParam
	}
	return v, nil
}

const (
	defaultPageSize = 25
	minPageSize     = 1
	maxPageSize     = 100
)

// PageSize parses the pageSize query parameter (default 25, range 1-100).
func PageSize(r *http.Request) (int, error) {
	v := r.URL.Query().Get("pageSize")
	if v == "" {
		return defaultPageSize, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minPageSize || n > maxPageSize {
		return 0, ErrInvalidParam
	}
	return n, nil
}

// SortOrder parses the sortOrder query parameter ("asc" or "desc", default "asc").
func SortOrder(r *http.Request) (string, error) {
	v := r.URL.Query().Get("sortOrder")
	if v == "" {
		return "asc", nil
	}
	if v != "asc" && v != "desc" {
		return "", ErrInvalidParam
	}
	return v, nil
}

// SortField parses the sortField query parameter, validating it against the
// endpoint's allowed set and defaulting to allowed[0] when absent.
func SortField(r *http.Request, allowed []string) (string, error) {
	v := r.URL.Query().Get("sortField")
	if v == "" {
		return allowed[0], nil
	}
	if !slices.Contains(allowed, v) {
		return "", ErrInvalidParam
	}
	return v, nil
}

// PageToken returns the raw pageToken query parameter, or "" if absent.
func PageToken(r *http.Request) string {
	return r.URL.Query().Get("pageToken")
}

var validIDTypes = []string{"CPE", "TEI", "PURL", "COMPLIANCE_DOCUMENT"}

// IDFilter parses the idType/idValue query parameters used to filter list
// endpoints by identifier.
func IDFilter(r *http.Request) (idType, idValue string, err error) {
	idType = r.URL.Query().Get("idType")
	idValue = r.URL.Query().Get("idValue")
	if idType != "" && !slices.Contains(validIDTypes, idType) {
		return "", "", ErrInvalidParam
	}
	return idType, idValue, nil
}
