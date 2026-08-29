// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
)

// TestExistsComponent mirrors product_test.go's TestExistsProduct -- see
// there for why an existence check is required (not skippable) even though
// component, like product, has no revision column.
func TestExistsComponent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	c, err := r.CreateComponent(ctx, "exists-check", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if err := r.ExistsComponent(ctx, c.UUID); err != nil {
		t.Fatalf("ExistsComponent: %v", err)
	}

	if err := r.DeleteComponent(ctx, c.UUID); err != nil {
		t.Fatalf("DeleteComponent: %v", err)
	}
	if err := r.ExistsComponent(ctx, c.UUID); err != ErrNotFound {
		t.Fatalf("ExistsComponent after delete: err = %v, want ErrNotFound", err)
	}
}

func TestExistsComponentNotFound(t *testing.T) {
	r := newTestRepo(t)
	if err := r.ExistsComponent(context.Background(), "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
