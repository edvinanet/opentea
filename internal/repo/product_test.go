package repo

import (
	"context"
	"testing"

	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

func TestCreateAndGetProduct(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	created, err := r.CreateProduct(ctx, "Apache Log4j 2", []tea.Identifier{
		{IDType: "PURL", IDValue: "pkg:maven/org.apache.logging.log4j/log4j-api"},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if created.UUID == "" {
		t.Fatal("expected generated uuid")
	}

	got, err := r.GetProduct(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.Name != "Apache Log4j 2" {
		t.Fatalf("Name = %q, want %q", got.Name, "Apache Log4j 2")
	}
	if len(got.Identifiers) != 1 || got.Identifiers[0].IDValue != "pkg:maven/org.apache.logging.log4j/log4j-api" {
		t.Fatalf("Identifiers = %+v", got.Identifiers)
	}
}

func TestGetProductNotFound(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.GetProduct(context.Background(), "00000000-0000-4000-8000-000000000000")
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestQueryProductsPagination verifies keyset pagination pages through a
// full result set without skipping or duplicating rows, including the exact
// page-boundary case.
func TestQueryProductsPagination(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	names := []string{"p1", "p2", "p3", "p4", "p5"}
	uuids := make(map[string]string, len(names))
	for _, n := range names {
		p, err := r.CreateProduct(ctx, n, nil)
		if err != nil {
			t.Fatalf("CreateProduct(%s): %v", n, err)
		}
		uuids[n] = p.UUID
	}

	var cursor *pagination.Cursor
	var pages [][]string
	for i := 0; i < 10; i++ { // safety bound against infinite loop on a bug
		got, err := r.QueryProducts(ctx, "", "", "name", "asc", cursor, 2)
		if err != nil {
			t.Fatalf("QueryProducts: %v", err)
		}
		if len(got) == 0 {
			break
		}
		var page []string
		for _, p := range got {
			page = append(page, p.Name)
		}
		pages = append(pages, page)

		last := got[len(got)-1]
		cursor = &pagination.Cursor{SortField: "name", SortOrder: "asc", LastValue: last.Name, LastUUID: last.UUID}
	}

	want := [][]string{{"p1", "p2"}, {"p3", "p4"}, {"p5"}}
	if len(pages) != len(want) {
		t.Fatalf("got %d pages %v, want %d pages %v", len(pages), pages, len(want), want)
	}
	for i := range want {
		if len(pages[i]) != len(want[i]) {
			t.Fatalf("page %d = %v, want %v", i, pages[i], want[i])
		}
		for j := range want[i] {
			if pages[i][j] != want[i][j] {
				t.Fatalf("page %d = %v, want %v", i, pages[i], want[i])
			}
		}
	}
}

func TestQueryProductsIdentifierFilter(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateProduct(ctx, "match", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/match"}}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := r.CreateProduct(ctx, "nomatch", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/other"}}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	got, err := r.QueryProducts(ctx, "PURL", "pkg:generic/match", "name", "asc", nil, 25)
	if err != nil {
		t.Fatalf("QueryProducts: %v", err)
	}
	if len(got) != 1 || got[0].Name != "match" {
		t.Fatalf("got %+v, want single product named 'match'", got)
	}
}

func TestDeleteProduct(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	p, err := r.CreateProduct(ctx, "to-delete", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if err := r.DeleteProduct(ctx, p.UUID); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if _, err := r.GetProduct(ctx, p.UUID); err != ErrNotFound {
		t.Fatalf("GetProduct after delete: err = %v, want ErrNotFound", err)
	}
	if err := r.DeleteProduct(ctx, p.UUID); err != ErrNotFound {
		t.Fatalf("DeleteProduct (already gone): err = %v, want ErrNotFound", err)
	}
}

// TestExistsProduct is the regression test for the design-review-caught
// "skip the DB entirely for immutable resources" bug: product has no
// revision column (it's provably immutable -- no update path exists), but
// DeleteProduct is a real write path, so an ETag built from the UUID alone
// still needs an existence check first -- otherwise a client polling a
// deleted product's cached ETag would get 304 with stale data forever.
func TestExistsProduct(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	p, err := r.CreateProduct(ctx, "exists-check", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if err := r.ExistsProduct(ctx, p.UUID); err != nil {
		t.Fatalf("ExistsProduct: %v", err)
	}

	if err := r.DeleteProduct(ctx, p.UUID); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if err := r.ExistsProduct(ctx, p.UUID); err != ErrNotFound {
		t.Fatalf("ExistsProduct after delete: err = %v, want ErrNotFound", err)
	}
}

func TestExistsProductNotFound(t *testing.T) {
	r := newTestRepo(t)
	if err := r.ExistsProduct(context.Background(), "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
