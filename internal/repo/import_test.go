// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/pkg/tea"
)

func TestImportProductIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid := idgen.New()
	ids := []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/foo"}}

	created, err := r.ImportProduct(ctx, uuid, "Foo", ids)
	if err != nil || !created {
		t.Fatalf("first ImportProduct: created=%v err=%v", created, err)
	}
	created, err = r.ImportProduct(ctx, uuid, "Foo", ids)
	if err != nil || created {
		t.Fatalf("second ImportProduct: created=%v err=%v, want created=false", created, err)
	}

	got, err := r.GetProduct(ctx, uuid)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.Name != "Foo" {
		t.Fatalf("Name = %q, want original %q", got.Name, "Foo")
	}
	if len(got.Identifiers) != 1 {
		t.Fatalf("Identifiers = %+v, want the original single identifier (no duplication)", got.Identifiers)
	}
}

// TestImportProductConflict asserts the fix this session's redesign is
// about: a source server's UUID is only meaningful within that source
// server, so re-importing the same UUID with genuinely different content
// (as would happen if two unrelated source servers independently assigned
// the same UUID) must be rejected, not silently ignored -- see
// ErrImportIdentityConflict's doc comment.
func TestImportProductConflict(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid := idgen.New()

	created, err := r.ImportProduct(ctx, uuid, "Foo", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/foo"}})
	if err != nil || !created {
		t.Fatalf("first ImportProduct: created=%v err=%v", created, err)
	}
	created, err = r.ImportProduct(ctx, uuid, "Foo (renamed elsewhere)", nil)
	if !errors.Is(err, ErrImportIdentityConflict) {
		t.Fatalf("second ImportProduct: err=%v, want ErrImportIdentityConflict", err)
	}
	if created {
		t.Fatalf("second ImportProduct: created=%v, want false on conflict", created)
	}

	got, err := r.GetProduct(ctx, uuid)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.Name != "Foo" {
		t.Fatalf("Name = %q, want original %q (rejected import must not overwrite)", got.Name, "Foo")
	}
}

func TestImportComponentIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid := idgen.New()

	created, err := r.ImportComponent(ctx, uuid, "libfoo", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/libfoo"}})
	if err != nil || !created {
		t.Fatalf("first ImportComponent: created=%v err=%v", created, err)
	}
	created, err = r.ImportComponent(ctx, uuid, "libfoo", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/libfoo"}})
	if err != nil || created {
		t.Fatalf("second ImportComponent: created=%v err=%v, want created=false", created, err)
	}

	got, err := r.GetComponent(ctx, uuid)
	if err != nil {
		t.Fatalf("GetComponent: %v", err)
	}
	if len(got.Identifiers) != 1 {
		t.Fatalf("Identifiers = %+v, want no duplication across two imports", got.Identifiers)
	}
}

func TestImportProductReleaseIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	productUUID := idgen.New()
	if _, err := r.ImportProduct(ctx, productUUID, "Foo", nil); err != nil {
		t.Fatalf("ImportProduct: %v", err)
	}
	releaseUUID := idgen.New()
	in := ImportProductReleaseInput{
		UUID:        releaseUUID,
		ProductUUID: productUUID,
		ProductName: "Foo",
		Version:     "1.0.0",
		CreatedDate: time.Now().UTC().Truncate(time.Second),
	}

	created, err := r.ImportProductRelease(ctx, in)
	if err != nil || !created {
		t.Fatalf("first ImportProductRelease: created=%v err=%v", created, err)
	}
	created, err = r.ImportProductRelease(ctx, in)
	if err != nil || created {
		t.Fatalf("second ImportProductRelease: created=%v err=%v, want created=false", created, err)
	}

	got, err := r.GetProductRelease(ctx, releaseUUID)
	if err != nil {
		t.Fatalf("GetProductRelease: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Fatalf("Version = %q", got.Version)
	}
}

func TestImportComponentReleaseIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	componentUUID := idgen.New()
	if _, err := r.ImportComponent(ctx, componentUUID, "libfoo", nil); err != nil {
		t.Fatalf("ImportComponent: %v", err)
	}
	releaseUUID := idgen.New()
	in := ImportComponentReleaseInput{
		UUID:          releaseUUID,
		ComponentUUID: componentUUID,
		ComponentName: "libfoo",
		Version:       "2.0.0",
		CreatedDate:   time.Now().UTC().Truncate(time.Second),
	}

	created, err := r.ImportComponentRelease(ctx, in)
	if err != nil || !created {
		t.Fatalf("first ImportComponentRelease: created=%v err=%v", created, err)
	}
	created, err = r.ImportComponentRelease(ctx, in)
	if err != nil || created {
		t.Fatalf("second ImportComponentRelease: created=%v err=%v, want created=false", created, err)
	}
}

func TestImportDistributionIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	componentUUID := idgen.New()
	if _, err := r.ImportComponent(ctx, componentUUID, "libfoo", nil); err != nil {
		t.Fatalf("ImportComponent: %v", err)
	}
	releaseUUID := idgen.New()
	if _, err := r.ImportComponentRelease(ctx, ImportComponentReleaseInput{
		UUID: releaseUUID, ComponentUUID: componentUUID, ComponentName: "libfoo",
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("ImportComponentRelease: %v", err)
	}

	distID := idgen.New()
	in := ImportDistributionInput{
		DistributionID:       distID,
		ComponentReleaseUUID: releaseUUID,
		Description:          "binary tarball",
		URL:                  "http://example.com/files/abc",
		Checksums:            []tea.Checksum{{AlgType: "SHA-256", AlgValue: "abc123"}},
	}

	created, err := r.ImportDistribution(ctx, in)
	if err != nil || !created {
		t.Fatalf("first ImportDistribution: created=%v err=%v", created, err)
	}
	created, err = r.ImportDistribution(ctx, in)
	if err != nil || created {
		t.Fatalf("second ImportDistribution: created=%v err=%v, want created=false", created, err)
	}

	got, err := r.GetDistribution(ctx, distID)
	if err != nil {
		t.Fatalf("GetDistribution: %v", err)
	}
	if len(got.Checksums) != 1 {
		t.Fatalf("Checksums = %+v, want no duplication across two imports", got.Checksums)
	}
}

func TestImportArtifactIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid := idgen.New()
	in := ImportArtifactInput{
		UUID:    uuid,
		Version: 1,
		Type:    "BOM",
		Formats: []ImportArtifactFormatInput{
			{MediaType: "application/json", URL: "http://example.com/files/xyz", Checksums: []tea.Checksum{{AlgType: "SHA-256", AlgValue: "xyz789"}}},
		},
	}

	created, err := r.ImportArtifact(ctx, in)
	if err != nil || !created {
		t.Fatalf("first ImportArtifact: created=%v err=%v", created, err)
	}
	created, err = r.ImportArtifact(ctx, in)
	if err != nil || created {
		t.Fatalf("second ImportArtifact: created=%v err=%v, want created=false", created, err)
	}

	got, err := r.GetArtifactByVersion(ctx, uuid, 1)
	if err != nil {
		t.Fatalf("GetArtifactByVersion: %v", err)
	}
	if len(got.Formats) != 1 {
		t.Fatalf("Formats = %+v, want no duplication across two imports", got.Formats)
	}

	// A different version of the same artifact uuid is a distinct entity and
	// should be created even though version 1 already exists.
	in2 := in
	in2.Version = 2
	created, err = r.ImportArtifact(ctx, in2)
	if err != nil || !created {
		t.Fatalf("ImportArtifact new version: created=%v err=%v, want created=true", created, err)
	}
}

func TestImportCollectionIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	productUUID := idgen.New()
	if _, err := r.ImportProduct(ctx, productUUID, "Foo", nil); err != nil {
		t.Fatalf("ImportProduct: %v", err)
	}
	releaseUUID := idgen.New()
	if _, err := r.ImportProductRelease(ctx, ImportProductReleaseInput{
		UUID: releaseUUID, ProductUUID: productUUID, ProductName: "Foo",
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("ImportProductRelease: %v", err)
	}

	in := ImportCollectionInput{
		UUID:      releaseUUID,
		Version:   1,
		Date:      time.Now().UTC().Truncate(time.Second),
		BelongsTo: "PRODUCT_RELEASE",
	}
	created, err := r.ImportCollection(ctx, in)
	if err != nil || !created {
		t.Fatalf("first ImportCollection: created=%v err=%v", created, err)
	}
	created, err = r.ImportCollection(ctx, in)
	if err != nil || created {
		t.Fatalf("second ImportCollection: created=%v err=%v, want created=false", created, err)
	}
}

func TestImportCLEEventIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	productUUID := idgen.New()
	if _, err := r.ImportProduct(ctx, productUUID, "Foo", nil); err != nil {
		t.Fatalf("ImportProduct: %v", err)
	}

	event := tea.CLEEvent{
		ID:        5,
		Type:      "released",
		Effective: time.Now().UTC().Truncate(time.Second),
		Published: time.Now().UTC().Truncate(time.Second),
		Version:   "1.0.0",
	}
	created, err := r.ImportCLEEvent(ctx, OwnerProduct, productUUID, event)
	if err != nil || !created {
		t.Fatalf("first ImportCLEEvent: created=%v err=%v", created, err)
	}
	created, err = r.ImportCLEEvent(ctx, OwnerProduct, productUUID, event)
	if err != nil || created {
		t.Fatalf("second ImportCLEEvent: created=%v err=%v, want created=false", created, err)
	}

	cle, err := r.GetCLE(ctx, OwnerProduct, productUUID)
	if err != nil {
		t.Fatalf("GetCLE: %v", err)
	}
	if len(cle.Events) != 1 {
		t.Fatalf("Events = %+v, want exactly the one imported event, preserving its original id", cle.Events)
	}
	if cle.Events[0].ID != 5 {
		t.Fatalf("Events[0].ID = %d, want the preserved source id 5", cle.Events[0].ID)
	}
}

// TestImportArtifactConflictFailsFast is the regression test for this
// session's r.conn()/dbtx-twin deadlock fix: internal/db/db.go pins the
// connection pool to a single connection, so a conflict check that wrongly
// called back through an r.conn()-routed Get* method (rather than a dbtx-
// parameterized twin) from inside runInTx's own standalone transaction
// would try to check out a second connection while the first is still held
// -- a self-deadlock, not a stale read. This only reproduces when
// ImportArtifact is called directly on a fresh *Repo (r.tx == nil), which
// makes runInTx open its own standalone transaction; called from inside an
// outer Repo.WithTx (as internal/bundle.Import always does), r.tx is
// already set and runInTx reuses it, masking the bug -- see
// internal/bundle/import_test.go's TestImportConflictDetection/artifact,
// which exercises the same conflict logic but would NOT catch this
// particular regression. A short context.WithTimeout is used because
// context.Background() would just hang forever, not fail, if this
// regressed.
func TestImportArtifactConflictFailsFast(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid := idgen.New()
	in := ImportArtifactInput{
		UUID:    uuid,
		Version: 1,
		Type:    "BOM",
		Formats: []ImportArtifactFormatInput{
			{MediaType: "application/json", Checksums: []tea.Checksum{{AlgType: "SHA-256", AlgValue: "xyz789"}}},
		},
	}
	if _, err := r.ImportArtifact(ctx, in); err != nil {
		t.Fatalf("first ImportArtifact: %v", err)
	}

	conflicting := in
	conflicting.Type = "VULNERABILITIES"

	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.ImportArtifact(shortCtx, conflicting)
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("ImportArtifact hung until context deadline instead of failing fast -- likely a regression of the r.conn()/dbtx-twin deadlock fix")
	}
	if !errors.Is(err, ErrImportIdentityConflict) {
		t.Fatalf("second ImportArtifact: err = %v, want ErrImportIdentityConflict", err)
	}
}

func TestImportCLESupportDefinitionIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	productUUID := idgen.New()
	if _, err := r.ImportProduct(ctx, productUUID, "Foo", nil); err != nil {
		t.Fatalf("ImportProduct: %v", err)
	}

	def := tea.CLESupportDefinition{ID: "ltsm", Description: "Long-term support"}
	created, err := r.ImportCLESupportDefinition(ctx, OwnerProduct, productUUID, def)
	if err != nil || !created {
		t.Fatalf("first ImportCLESupportDefinition: created=%v err=%v", created, err)
	}
	created, err = r.ImportCLESupportDefinition(ctx, OwnerProduct, productUUID, def)
	if err != nil || created {
		t.Fatalf("second ImportCLESupportDefinition: created=%v err=%v, want created=false", created, err)
	}
}
