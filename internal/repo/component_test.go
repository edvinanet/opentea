// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/oej/opentea/pkg/tea"
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

// TestCreateComponentIdentifierConflict covers security-review fix 12
// (docs/security-review-publisher-design-260828.md): identifier uniqueness
// is enforced inside CreateComponent's own transaction, not left to a
// client's find-before-create convention.
func TestCreateComponentIdentifierConflict(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	ids := []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/acme-widget-core"}}
	if _, err := r.CreateComponent(ctx, "acme-widget-core", ids); err != nil {
		t.Fatalf("CreateComponent (first): %v", err)
	}
	if _, err := r.CreateComponent(ctx, "acme-widget-core-fork", ids); !errors.Is(err, ErrComponentIdentifierConflict) {
		t.Fatalf("err = %v, want ErrComponentIdentifierConflict", err)
	}

	// No identifiers -- nothing to conflict on, even with the same name.
	if _, err := r.CreateComponent(ctx, "no-identifiers", nil); err != nil {
		t.Fatalf("CreateComponent (no identifiers, 1st): %v", err)
	}
	if _, err := r.CreateComponent(ctx, "no-identifiers", nil); err != nil {
		t.Fatalf("CreateComponent (no identifiers, 2nd): %v", err)
	}
}
