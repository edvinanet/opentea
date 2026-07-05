package repo

import "fmt"

// pageQuery composes the WHERE/ORDER BY SQL fragments for keyset pagination
// over (sortColumn, uuid).
//
// SortColumn must come from a compile-time whitelist switch in the caller
// (validated sortField -> real column name) -- it is interpolated directly
// into the query text, so it must never be a raw request value.
type pageQuery struct {
	SortColumn string
	SortOrder  string // "asc" or "desc"
	Cursor     *cursorValue
}

type cursorValue struct {
	LastValue string
	LastUUID  string
}

// whereClause returns an SQL fragment (starting with "AND ...", or "" if
// there's no cursor) plus its bind args.
func (p pageQuery) whereClause() (string, []any) {
	if p.Cursor == nil {
		return "", nil
	}
	op := ">"
	if p.SortOrder == "desc" {
		op = "<"
	}
	frag := fmt.Sprintf(" AND (%s %s ? OR (%s = ? AND uuid %s ?))", p.SortColumn, op, p.SortColumn, op)
	return frag, []any{p.Cursor.LastValue, p.Cursor.LastValue, p.Cursor.LastUUID}
}

func (p pageQuery) orderByClause() string {
	dir := "ASC"
	if p.SortOrder == "desc" {
		dir = "DESC"
	}
	return fmt.Sprintf(" ORDER BY %s %s, uuid %s", p.SortColumn, dir, dir)
}
