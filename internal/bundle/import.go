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
	manifestRaw, err := readZipFile(zr, "manifest.json")
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
	sha256ToURL, err := importBlobs(ctx, r, store, zr, rootURL, sha256ToMediaType)
	if err != nil {
		return nil, err
	}

	res := newImportResult()

	productCreated, err := r.ImportProduct(ctx, m.Product.UUID, m.Product.Name, m.Product.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("import product: %w", err)
	}
	res.ProductCreated = productCreated
	res.record("product", productCreated)
	if err := importCLE(ctx, r, repo.OwnerProduct, m.Product.UUID, m.Product.CLE, res); err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("import product release %s: %w", pr.UUID, err)
		}
		res.record("productRelease", created)
		if err := importCLE(ctx, r, repo.OwnerProductRelease, pr.UUID, pr.CLE, res); err != nil {
			return nil, err
		}
	}

	for _, c := range m.Components {
		created, err := r.ImportComponent(ctx, c.UUID, c.Name, c.Identifiers)
		if err != nil {
			return nil, fmt.Errorf("import component %s: %w", c.UUID, err)
		}
		res.record("component", created)
		if err := importCLE(ctx, r, repo.OwnerComponent, c.UUID, c.CLE, res); err != nil {
			return nil, err
		}
	}

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
			return nil, fmt.Errorf("import component release %s: %w", cr.UUID, err)
		}
		res.record("componentRelease", created)
		if err := importCLE(ctx, r, repo.OwnerComponentRelease, cr.UUID, cr.CLE, res); err != nil {
			return nil, err
		}
		for _, d := range cr.Distributions {
			if err := importDistribution(ctx, r, cr.UUID, d, sha256ToURL, res); err != nil {
				return nil, err
			}
		}
	}

	// Component links are made only now, after every component release they
	// might pin has been imported -- product_release_component's FK on
	// component_release_uuid would otherwise fail for a pinned ref.
	for _, pr := range m.ProductReleases {
		for _, ref := range pr.Components {
			if _, err := r.LinkComponent(ctx, pr.UUID, ref); err != nil {
				return nil, fmt.Errorf("link component %s to product release %s: %w", ref.UUID, pr.UUID, err)
			}
		}
	}

	for _, col := range m.Collections {
		for _, a := range col.Artifacts {
			if err := importArtifact(ctx, r, a, sha256ToURL, res); err != nil {
				return nil, err
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
			return nil, fmt.Errorf("import collection %s v%d: %w", col.UUID, col.Version, err)
		}
		res.record("collection", created)
	}

	return res, nil
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
		actualSHA256, size, err := store.Put(ctx, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("store bundle entry %s: %w", f.Name, err)
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

func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	f, err := zr.Open(name)
	if err != nil {
		return nil, fmt.Errorf("bundle is missing %s: %w", name, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return raw, nil
}
