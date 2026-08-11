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

// filterAuthorized repeatedly fetches pages from fetch (each call passing
// the appropriate next cursor) and drops rows authorized rejects, until it
// has accumulated pageSize+1 authorized rows or fetch returns fewer than
// requested (meaning there's no more underlying data). This keeps
// splitPage's pageSize+1 "is there a next page" trick correct even though
// filtering happens after the DB query: hasNext reflects "is there another
// AUTHORIZED row", not "did the underlying table have more rows" -- which
// is what spec Sec 18 requires (pagination must operate on the authorized
// set, no gaps or leaks via cursor or count).
//
// fetch(cursor, limit) must return rows in stable sort order and accept a
// nil cursor for the first call. toCursor derives the resumption cursor
// from a row's own sort field/UUID, so a short/filtered batch can resume
// correctly from the last underlying row seen (not the last authorized
// one).
//
// Known Phase 1 simplification: authorized calls authz.Decide once per
// row, and a caller whose authorized rows are sparse relative to the
// underlying table triggers repeated round trips (worst case O(table size
// / pageSize)). Simpler and more obviously correct than a join-based
// filter; batching CandidateRules to evaluate many rows in one query is a
// documented follow-up, not a blocker for Phase 1's expected data volumes.
func filterAuthorized[T any](
	fetch func(cursor *pagination.Cursor, limit int) ([]T, error),
	toCursor func(T) pagination.Cursor,
	authorized func(T) (bool, error),
	startCursor *pagination.Cursor,
	pageSize int,
) (page []T, hasNext bool, err error) {
	cursor := startCursor
	for {
		rows, err := fetch(cursor, pageSize+1)
		if err != nil {
			return nil, false, err
		}
		for _, row := range rows {
			ok, err := authorized(row)
			if err != nil {
				return nil, false, err
			}
			if ok {
				page = append(page, row)
			}
		}
		gotFullUnderlyingBatch := len(rows) > pageSize
		if len(page) > pageSize || !gotFullUnderlyingBatch {
			break
		}
		last := rows[len(rows)-1]
		c := toCursor(last)
		cursor = &c
	}
	hasNext = len(page) > pageSize
	if hasNext {
		page = page[:pageSize]
	}
	return page, hasNext, nil
}
