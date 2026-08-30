// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package model

import "time"

// Publisher credential scopes (publisher_credential.scope CHECK values,
// internal/db/migrations/0007_publisher.sql). See internal/publisher's
// package doc comment for what each scope may call.
const (
	PublisherScopeFull = "full"
	PublisherScopeCICD = "cicd"
)

// PublisherCredential is a Layer B/D bearer credential for /publisher/v1
// (design/publisher-service.md §10.2/§10.4) -- opentea-internal
// bookkeeping, not part of the standard wire protocol (same category as
// Entitlement, not pkg/tea/pkg/teapublisher). Admin-issued and
// admin-managed via /admin/v1/publisherCredentials; the raw token is
// returned only once, at creation.
type PublisherCredential struct {
	UUID      string     `json:"uuid"`
	Label     string     `json:"label"`
	Scope     string     `json:"scope"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}
