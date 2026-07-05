package api

import (
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/pagination"
)

// pageParams holds the parsed & validated pagination/sort query parameters
// shared by every list endpoint.
type pageParams struct {
	PageSize  int
	SortField string
	SortOrder string
	Cursor    *pagination.Cursor
}

// parsePageParams parses pageSize/sortField/sortOrder/pageToken, validating
// sortField against allowedSortFields and (if a pageToken is present)
// requiring it to match the current sortField/sortOrder. Writes a 400 and
// returns ok=false on any validation failure.
func parsePageParams(w http.ResponseWriter, r *http.Request, allowedSortFields []string) (pageParams, bool) {
	var p pageParams

	pageSize, err := httpx.PageSize(r)
	if err != nil {
		httpx.BadRequest(w, "invalid pageSize: must be an integer between 1 and 100")
		return p, false
	}
	p.PageSize = pageSize

	sortField, err := httpx.SortField(r, allowedSortFields)
	if err != nil {
		httpx.BadRequest(w, "invalid sortField")
		return p, false
	}
	p.SortField = sortField

	sortOrder, err := httpx.SortOrder(r)
	if err != nil {
		httpx.BadRequest(w, "invalid sortOrder: must be \"asc\" or \"desc\"")
		return p, false
	}
	p.SortOrder = sortOrder

	if token := httpx.PageToken(r); token != "" {
		cursor, err := pagination.Decode(token)
		if err != nil {
			httpx.BadRequest(w, "invalid pageToken")
			return p, false
		}
		if cursor.SortField != sortField || cursor.SortOrder != sortOrder {
			httpx.BadRequest(w, "pageToken does not match the current sortField/sortOrder")
			return p, false
		}
		p.Cursor = &cursor
	}

	return p, true
}

// splitPage trims a limit=pageSize+1 fetch down to pageSize rows and reports
// whether there's a following page.
func splitPage[T any](rows []T, pageSize int) (page []T, hasNext bool) {
	if len(rows) > pageSize {
		return rows[:pageSize], true
	}
	return rows, false
}

func nextPageToken(hasNext bool, sortField, sortOrder, lastValue, lastUUID string) string {
	if !hasNext {
		return ""
	}
	return pagination.Encode(pagination.Cursor{
		SortField: sortField,
		SortOrder: sortOrder,
		LastValue: lastValue,
		LastUUID:  lastUUID,
	})
}
