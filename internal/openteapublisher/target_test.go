// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"testing"
)

func TestCreateListGetDeleteTarget(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	created, err := r.CreateTarget(ctx, "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t-token")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if created.Label != "Acme production" || created.BearerToken != "s3cr3t-token" {
		t.Fatalf("created = %+v", created)
	}

	got, err := r.GetTarget(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if got.BaseURL != "https://tea.example.com/publisher/v1" {
		t.Fatalf("got = %+v", got)
	}

	list, err := r.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(list) != 1 || list[0].UUID != created.UUID {
		t.Fatalf("list = %+v", list)
	}

	if err := r.DeleteTarget(ctx, created.UUID); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
	if _, err := r.GetTarget(ctx, created.UUID); err != ErrNotFound {
		t.Fatalf("GetTarget after delete: err = %v, want ErrNotFound", err)
	}
	if err := r.DeleteTarget(ctx, created.UUID); err != ErrNotFound {
		t.Fatalf("DeleteTarget (already deleted): err = %v, want ErrNotFound", err)
	}
}
