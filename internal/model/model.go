// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package model holds admin-domain types (users, roles, dashboard stats) --
// these are server-only and not part of the TEA spec. Spec-derived wire
// types (Product, Collection, Artifact, CLE, etc.) live in pkg/tea, which is
// importable from outside this module; these stay under internal/ since
// nothing outside this server needs them.
package model

import "time"

// The two admin-domain roles: RoleAdmin satisfies both RoleAdmin- and
// RoleConsumer-gated routes; RoleConsumer only satisfies RoleConsumer ones.
const (
	RoleAdmin    = "admin"
	RoleConsumer = "consumer"
)

// User is JSON-facing and must never carry a password hash or any other secret.
type User struct {
	UUID      string    `json:"uuid"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Stats is the dashboard/monitoring response for GET /admin/v1/stats.
// StartedAt and OrgName are set by the handler (from the server process's
// own start time and configuration), not by the repo layer -- repo.GetStats
// only knows about DB counts.
type Stats struct {
	StartedAt         time.Time `json:"startedAt"`
	OrgName           string    `json:"orgName,omitempty"`
	Products          int       `json:"products"`
	ProductReleases   int       `json:"productReleases"`
	Components        int       `json:"components"`
	ComponentReleases int       `json:"componentReleases"`
	Collections       int       `json:"collections"`
	Artifacts         int       `json:"artifacts"`
}
