// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"net/url"
	"strconv"
)

// ListParams holds the pagination/sort/filter query parameters shared by
// every list endpoint in the spec.
type ListParams struct {
	PageSize  int
	PageToken string
	SortField string
	SortOrder string
	IDType    string
	IDValue   string
}

func (p ListParams) values() url.Values {
	v := url.Values{}
	if p.PageSize > 0 {
		v.Set("pageSize", strconv.Itoa(p.PageSize))
	}
	if p.PageToken != "" {
		v.Set("pageToken", p.PageToken)
	}
	if p.SortField != "" {
		v.Set("sortField", p.SortField)
	}
	if p.SortOrder != "" {
		v.Set("sortOrder", p.SortOrder)
	}
	if p.IDType != "" {
		v.Set("idType", p.IDType)
	}
	if p.IDValue != "" {
		v.Set("idValue", p.IDValue)
	}
	return v
}
