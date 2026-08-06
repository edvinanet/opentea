package bundle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/pkg/tea"
)

// maxZipEntrySize bounds how much of any single zip entry (a files/<sha256>
// blob, checked here and in check.go) is read from a bundle -- guards
// against a maliciously crafted, highly-compressed entry expanding to
// unbounded size when decompressed (a "zip bomb"). Matches the whole-bundle
// upload cap already used at internal/admin/bundle.go's maxImportBundleSize.
// A var (not const) so tests can temporarily lower it rather than needing a
// multi-gigabyte fixture to exercise the oversized-entry path; this
// package's tests never run in parallel, so mutating it in one test and
// restoring via t.Cleanup is safe.
var maxZipEntrySize int64 = 1 << 30 // 1 GiB

// maxManifestSize bounds manifest.json specifically -- generous for a real
// bundle's metadata (a JSON document describing one product's entities),
// far below maxZipEntrySize since a manifest is never itself a large binary
// blob like a files/ entry can legitimately be.
var maxManifestSize int64 = 10 << 20 // 10 MiB

// maxZipEntries and maxZipTotalUncompressed bound a bundle zip's aggregate
// shape -- not any single entry's real bytes (maxZipEntrySize already
// bounds that via streaming, unchanged by these) -- rejecting an absurd
// entry count or declared total size outright, before opening or reading a
// single entry. Real bundles are per-product exports with a handful of
// files, so both are set with a wide margin for legitimate use while still
// bounding an archive engineered to be expensive to even enumerate.
//
// f.UncompressedSize64 comes from the zip's own central directory and is
// attacker-controlled, exactly like any other entry metadata -- this is an
// advisory, cheap pre-filter only, NOT a substitute for the streaming
// io.LimitReader checks in importBlobs/sha256OfZipEntry, which independently
// bound each entry's real decompressed byte count regardless of what this
// field claims. A crafted zip can understate UncompressedSize64 to sail
// through this check while a maliciously-crafted deflate stream still
// expands past maxZipEntrySize on actual read -- the streaming check is
// what catches that case, and removing it in favor of this one would
// reopen the zip-bomb hole this pair of checks is meant to close.
var (
	maxZipEntries                  = 1_000
	maxZipTotalUncompressed uint64 = 4 << 30 // 4 GiB
)

// checkZipResourceLimits rejects a bundle zip whose aggregate shape (entry
// count or declared total uncompressed size) is implausible for a real
// bundle, before any entry is opened or read. The entry-count check runs
// first and returns immediately if it fails: len(zr.File) is O(1) (the
// central directory is already fully parsed into that slice by the time a
// *zip.Reader exists), while summing UncompressedSize64 is O(n) -- bounding
// entry count first keeps that second pass itself cheap even against a zip
// crafted with an excessive number of entries.
func checkZipResourceLimits(zr *zip.Reader) error {
	if len(zr.File) > maxZipEntries {
		return fmt.Errorf("bundle: %d entries exceeds %d entry limit", len(zr.File), maxZipEntries)
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > maxZipTotalUncompressed {
			return fmt.Errorf("bundle: declared uncompressed size exceeds %d byte limit", maxZipTotalUncompressed)
		}
	}
	return nil
}

// ImportResult summarizes what an Import call actually did, broken down by
// entity kind, so callers (and the admin API's JSON response) can tell a
// fresh import from a no-op re-import of already-present data.
type ImportResult struct {
	ProductCreated bool
	Created        map[string]int
	AlreadyExisted map[string]int
}

func newImportResult() *ImportResult {
	return &ImportResult{Created: map[string]int{}, AlreadyExisted: map[string]int{}}
}

func (res *ImportResult) record(kind string, created bool) {
	if created {
		res.Created[kind]++
	} else {
		res.AlreadyExisted[kind]++
	}
}

// Import reads a bundle zip (as produced by Export) and applies it to r/store
// idempotently: every shared entity is identity-keyed, so re-importing (or
// importing a second bundle that overlaps this one) never duplicates data --
// see docs/bundle-format.md for the full dedup rule. rootURL is this
// (destination) server's own root URL, used to rebuild file URLs; the
// source server's original URLs in the manifest are never reused directly.
func Import(ctx context.Context, r *repo.Repo, store storage.Storage, rootURL string, zr *zip.Reader) (*ImportResult, error) {
	if err := checkZipResourceLimits(zr); err != nil {
		return nil, err
	}

	manifestRaw, err := readZipFile(zr, "manifest.json", maxManifestSize)
	if err != nil {
		return nil, err
	}

	// Schema validation happens on the raw bytes, before the manifest is
	// decoded into Go structs or anything is written to the repository --
	// a non-conformant bundle is rejected outright, not partially imported.
	if err := ValidateManifest(manifestRaw); err != nil {
		return nil, fmt.Errorf("bundle manifest rejected: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return nil, fmt.Errorf("decode manifest.json: %w", err)
	}

	sha256ToMediaType := collectMediaTypes(m)
	res := newImportResult()

	// Every DB write below runs inside one transaction spanning the whole
	// import: a failure partway through (bad FK, disk error, canceled
	// context) rolls back every entity written so far instead of leaving a
	// partially-imported product tree committed. Blob *files* are written to
	// storage.Put outside any rollback's reach (the filesystem isn't
	// transactional) -- see TODO.md for the follow-up orphan-blob GC that
	// covers that separate, lower-risk gap; a blob's own DB bookkeeping row
	// (UpsertBlob, inside importBlobs) does still participate in this
	// transaction, so it never survives with nothing referencing it.
	err = r.WithTx(ctx, func(txRepo *repo.Repo) error {
		sha256ToURL, err := importBlobs(ctx, txRepo, store, zr, rootURL, sha256ToMediaType)
		if err != nil {
			return err
		}

		productCreated, err := txRepo.ImportProduct(ctx, m.Product.UUID, m.Product.Name, m.Product.Identifiers)
		if err != nil {
			return fmt.Errorf("import product: %w", err)
		}
		res.ProductCreated = productCreated
		res.record("product", productCreated)
		if err := importCLE(ctx, txRepo, repo.OwnerProduct, m.Product.UUID, m.Product.CLE, res); err != nil {
			return err
		}

		if err := importProductReleases(ctx, txRepo, m, res); err != nil {
			return err
		}
		if err := importComponents(ctx, txRepo, m, res); err != nil {
			return err
		}
		if err := importComponentReleases(ctx, txRepo, m, sha256ToURL, res); err != nil {
			return err
		}
		// Component links are made only now, after every component release
		// they might pin has been imported -- product_release_component's FK
		// on component_release_uuid would otherwise fail for a pinned ref.
		if err := importComponentLinks(ctx, txRepo, m); err != nil {
			return err
		}
		if err := importCollections(ctx, txRepo, m, sha256ToURL, res); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func importProductReleases(ctx context.Context, r *repo.Repo, m Manifest, res *ImportResult) error {
	for _, pr := range m.ProductReleases {
		preRelease := pr.PreRelease != nil && *pr.PreRelease
		created, err := r.ImportProductRelease(ctx, repo.ImportProductReleaseInput{
			UUID:        pr.UUID,
			ProductUUID: m.Product.UUID,
			ProductName: pr.ProductName,
			Version:     pr.Version,
			CreatedDate: pr.CreatedDate,
			ReleaseDate: pr.ReleaseDate,
			PreRelease:  preRelease,
			Identifiers: pr.Identifiers,
		})
		if err != nil {
			return fmt.Errorf("import product release %s: %w", pr.UUID, err)
		}
		res.record("productRelease", created)
		if err := importCLE(ctx, r, repo.OwnerProductRelease, pr.UUID, pr.CLE, res); err != nil {
			return err
		}
	}
	return nil
}

func importComponents(ctx context.Context, r *repo.Repo, m Manifest, res *ImportResult) error {
	for _, c := range m.Components {
		created, err := r.ImportComponent(ctx, c.UUID, c.Name, c.Identifiers)
		if err != nil {
			return fmt.Errorf("import component %s: %w", c.UUID, err)
		}
		res.record("component", created)
		if err := importCLE(ctx, r, repo.OwnerComponent, c.UUID, c.CLE, res); err != nil {
			return err
		}
	}
	return nil
}

func importComponentReleases(ctx context.Context, r *repo.Repo, m Manifest, sha256ToURL map[string]string, res *ImportResult) error {
	for _, cr := range m.ComponentReleases {
		preRelease := cr.PreRelease != nil && *cr.PreRelease
		created, err := r.ImportComponentRelease(ctx, repo.ImportComponentReleaseInput{
			UUID:          cr.UUID,
			ComponentUUID: cr.Component,
			ComponentName: cr.ComponentName,
			Version:       cr.Version,
			CreatedDate:   cr.CreatedDate,
			ReleaseDate:   cr.ReleaseDate,
			PreRelease:    preRelease,
			Identifiers:   cr.Identifiers,
		})
		if err != nil {
			return fmt.Errorf("import component release %s: %w", cr.UUID, err)
		}
		res.record("componentRelease", created)
		if err := importCLE(ctx, r, repo.OwnerComponentRelease, cr.UUID, cr.CLE, res); err != nil {
			return err
		}
		for _, d := range cr.Distributions {
			if err := importDistribution(ctx, r, cr.UUID, d, sha256ToURL, res); err != nil {
				return err
			}
		}
	}
	return nil
}

func importComponentLinks(ctx context.Context, r *repo.Repo, m Manifest) error {
	for _, pr := range m.ProductReleases {
		for _, ref := range pr.Components {
			if err := r.ImportComponentLink(ctx, pr.UUID, ref); err != nil {
				return fmt.Errorf("link component %s to product release %s: %w", ref.UUID, pr.UUID, err)
			}
		}
	}
	return nil
}

func importCollections(ctx context.Context, r *repo.Repo, m Manifest, sha256ToURL map[string]string, res *ImportResult) error {
	for _, col := range m.Collections {
		for _, a := range col.Artifacts {
			if err := importArtifact(ctx, r, a, sha256ToURL, res); err != nil {
				return err
			}
		}
		artifactRefs := make([]repo.ArtifactRef, 0, len(col.Artifacts))
		for _, a := range col.Artifacts {
			artifactRefs = append(artifactRefs, repo.ArtifactRef{UUID: a.UUID, Version: a.Version})
		}
		created, err := r.ImportCollection(ctx, repo.ImportCollectionInput{
			UUID:         col.UUID,
			Version:      col.Version,
			Date:         col.Date,
			BelongsTo:    col.BelongsTo,
			UpdateReason: col.UpdateReason,
			Artifacts:    artifactRefs,
		})
		if err != nil {
			return fmt.Errorf("import collection %s v%d: %w", col.UUID, col.Version, err)
		}
		res.record("collection", created)
	}
	return nil
}

func importCLE(ctx context.Context, r *repo.Repo, ownerType, ownerUUID string, cle *tea.CLE, res *ImportResult) error {
	if cle == nil {
		return nil
	}
	for _, e := range cle.Events {
		created, err := r.ImportCLEEvent(ctx, ownerType, ownerUUID, e)
		if err != nil {
			return fmt.Errorf("import CLE event %d for %s %s: %w", e.ID, ownerType, ownerUUID, err)
		}
		res.record("cleEvent", created)
	}
	if cle.Definitions != nil {
		for _, def := range cle.Definitions.Support {
			created, err := r.ImportCLESupportDefinition(ctx, ownerType, ownerUUID, def)
			if err != nil {
				return fmt.Errorf("import CLE support definition %s for %s %s: %w", def.ID, ownerType, ownerUUID, err)
			}
			res.record("cleSupportDefinition", created)
		}
	}
	return nil
}

func importDistribution(ctx context.Context, r *repo.Repo, componentReleaseUUID string, d tea.ReleaseDistribution, sha256ToURL map[string]string, res *ImportResult) error {
	created, err := r.ImportDistribution(ctx, repo.ImportDistributionInput{
		DistributionID:       d.DistributionID,
		ComponentReleaseUUID: componentReleaseUUID,
		Description:          d.Description,
		Identifiers:          d.Identifiers,
		URL:                  rewriteURL(d.Checksums, sha256ToURL, d.URL),
		SignatureURL:         d.SignatureURL,
		Checksums:            d.Checksums,
	})
	if err != nil {
		return fmt.Errorf("import distribution %s: %w", d.DistributionID, err)
	}
	res.record("distribution", created)
	return nil
}

func importArtifact(ctx context.Context, r *repo.Repo, a tea.Artifact, sha256ToURL map[string]string, res *ImportResult) error {
	formats := make([]repo.ImportArtifactFormatInput, 0, len(a.Formats))
	for _, f := range a.Formats {
		formats = append(formats, repo.ImportArtifactFormatInput{
			MediaType:    f.MediaType,
			Description:  f.Description,
			URL:          rewriteURL(f.Checksums, sha256ToURL, f.URL),
			SignatureURL: f.SignatureURL,
			Checksums:    f.Checksums,
		})
	}
	created, err := r.ImportArtifact(ctx, repo.ImportArtifactInput{
		UUID:            a.UUID,
		Version:         a.Version,
		Name:            a.Name,
		Type:            a.Type,
		CreatedDate:     a.CreatedDate,
		DistributionIDs: a.DistributionIDs,
		Formats:         formats,
	})
	if err != nil {
		return fmt.Errorf("import artifact %s v%d: %w", a.UUID, a.Version, err)
	}
	res.record("artifact", created)
	return nil
}

// rewriteURL rebuilds a file's download URL against the destination
// server's own root URL rather than reusing the source server's original
// (which wouldn't resolve here). Falls back to the manifest's original URL
// if no SHA-256 checksum is present to key off of (e.g. a
// not-yet-uploaded/reference-only entry).
func rewriteURL(checksums []tea.Checksum, sha256ToURL map[string]string, fallback string) string {
	for _, c := range checksums {
		if c.AlgType == tea.ChecksumTypeSHA256 {
			if url, ok := sha256ToURL[c.AlgValue]; ok {
				return url
			}
		}
	}
	return fallback
}

// collectMediaTypes maps each SHA-256 hash referenced by an artifact
// format's checksums to that format's declared MediaType, so imported blobs
// get a sensible Content-Type recorded (distributions have no media-type
// field in the spec, so their blobs are recorded with an empty one, exactly
// like an upload with no Content-Type header).
func collectMediaTypes(m Manifest) map[string]string {
	out := map[string]string{}
	for _, col := range m.Collections {
		for _, a := range col.Artifacts {
			for _, f := range a.Formats {
				for _, c := range f.Checksums {
					if c.AlgType == tea.ChecksumTypeSHA256 {
						out[c.AlgValue] = f.MediaType
					}
				}
			}
		}
	}
	return out
}

// importBlobs stores every files/<sha256> zip entry via store.Put and
// records it in the repo's blob bookkeeping, verifying that the actual
// content hash matches the entry's claimed name (its SHA-256) before
// accepting it -- a bundle whose file bytes don't match its own checksum is
// rejected as corrupt, and this happens before any distribution or artifact
// that references it is imported. Returns a sha256 -> destination-server URL
// map for rewriting entity URLs.
func importBlobs(ctx context.Context, r *repo.Repo, store storage.Storage, zr *zip.Reader, rootURL string, mediaTypes map[string]string) (map[string]string, error) {
	sha256ToURL := map[string]string{}
	for _, f := range zr.File {
		const prefix = "files/"
		if len(f.Name) <= len(prefix) || f.Name[:len(prefix)] != prefix {
			continue
		}
		expectedSHA256 := f.Name[len(prefix):]

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open bundle entry %s: %w", f.Name, err)
		}
		actualSHA256, size, err := store.Put(ctx, io.LimitReader(rc, maxZipEntrySize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("store bundle entry %s: %w", f.Name, err)
		}
		if size > maxZipEntrySize {
			return nil, fmt.Errorf("bundle entry %s exceeds %d byte limit", f.Name, maxZipEntrySize)
		}
		if actualSHA256 != expectedSHA256 {
			return nil, fmt.Errorf("bundle entry %s is corrupt: actual content hash %s does not match", f.Name, actualSHA256)
		}

		if err := r.UpsertBlob(ctx, actualSHA256, size, mediaTypes[actualSHA256]); err != nil {
			return nil, fmt.Errorf("record blob %s: %w", actualSHA256, err)
		}
		sha256ToURL[actualSHA256] = rootURL + "/files/" + actualSHA256
	}
	return sha256ToURL, nil
}

// readZipFile reads the named zip entry in full, rejecting it if it exceeds
// maxSize -- the same io.LimitReader(+1)/length-check pattern used
// elsewhere in this package for maxZipEntrySize, applied here so callers
// can bound a specific entry (manifest.json, via maxManifestSize) without
// an unbounded io.ReadAll.
func readZipFile(zr *zip.Reader, name string, maxSize int64) ([]byte, error) {
	f, err := zr.Open(name)
	if err != nil {
		return nil, fmt.Errorf("bundle is missing %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(raw)) > maxSize {
		return nil, fmt.Errorf("%s exceeds %d byte limit", name, maxSize)
	}
	return raw, nil
}
