// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/pkg/tea"
)

// exportFetchLimit is used in place of real pagination for every read during
// export: this is an internal bulk operation, not subject to the public
// API's 1-100 page-size cap, and no realistic deployment has anywhere near
// this many releases/collections for a single product or component.
const exportFetchLimit = 1_000_000

// Export streams a complete bundle zip for productUUID into w: manifest.json
// plus a files/<sha256> entry for every distinct blob referenced by any
// distribution or artifact-format checksum. See docs/bundle-format.md for
// the full format and the "full history" scoping rule.
func Export(ctx context.Context, r *repo.Repo, store storage.Storage, productUUID string, w io.Writer) error {
	manifest, fileHashes, err := buildManifest(ctx, r, productUUID)
	if err != nil {
		return err
	}

	zw := zip.NewWriter(w)

	mw, err := zw.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create manifest.json entry: %w", err)
	}
	enc := json.NewEncoder(mw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	for sha256Hex := range fileHashes {
		if err := copyBlobIntoZip(ctx, zw, store, sha256Hex); err != nil {
			return err
		}
	}

	return zw.Close()
}

func copyBlobIntoZip(ctx context.Context, zw *zip.Writer, store storage.Storage, sha256Hex string) error {
	rc, err := store.Open(ctx, sha256Hex)
	if err != nil {
		return fmt.Errorf("open blob %s: %w", sha256Hex, err)
	}
	defer func() { _ = rc.Close() }()

	fw, err := zw.Create("files/" + sha256Hex)
	if err != nil {
		return fmt.Errorf("create files/%s entry: %w", sha256Hex, err)
	}
	if _, err := io.Copy(fw, rc); err != nil {
		return fmt.Errorf("copy blob %s: %w", sha256Hex, err)
	}
	return nil
}

// buildManifest gathers the product's full data per the "full history"
// scoping rule and returns the set of distinct SHA-256 hashes referenced by
// any distribution/artifact-format checksum along the way.
func buildManifest(ctx context.Context, r *repo.Repo, productUUID string) (Manifest, map[string]struct{}, error) {
	fileHashes := map[string]struct{}{}

	product, err := r.GetProduct(ctx, productUUID)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("get product: %w", err)
	}
	productCLE, err := r.GetCLE(ctx, repo.OwnerProduct, productUUID)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("get product CLE: %w", err)
	}

	releases, err := r.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, exportFetchLimit)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("list product releases: %w", err)
	}

	releaseEntries := make([]ProductReleaseEntry, 0, len(releases))
	collections := []tea.Collection{}
	seenComponents := map[string]bool{}
	componentEntries := []ComponentEntry{}
	componentReleaseEntries := []ComponentReleaseEntry{}

	for _, release := range releases {
		releaseCLE, err := r.GetCLE(ctx, repo.OwnerProductRelease, release.UUID)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("get CLE for product release %s: %w", release.UUID, err)
		}
		releaseEntries = append(releaseEntries, ProductReleaseEntry{ProductRelease: release, CLE: &releaseCLE})

		ownCollections, err := r.ListCollections(ctx, release.UUID, "asc", nil, exportFetchLimit, repo.BelongsToProductRelease)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("list collections for product release %s: %w", release.UUID, err)
		}
		collections = append(collections, ownCollections...)
		collectFileHashes(fileHashes, ownCollections)

		for _, ref := range release.Components {
			if !seenComponents[ref.UUID] {
				seenComponents[ref.UUID] = true

				component, err := r.GetComponent(ctx, ref.UUID)
				if err != nil {
					return Manifest{}, nil, fmt.Errorf("get component %s: %w", ref.UUID, err)
				}
				componentCLE, err := r.GetCLE(ctx, repo.OwnerComponent, ref.UUID)
				if err != nil {
					return Manifest{}, nil, fmt.Errorf("get CLE for component %s: %w", ref.UUID, err)
				}
				componentEntries = append(componentEntries, ComponentEntry{Component: component, CLE: &componentCLE})
			}

			if ref.Release == nil {
				continue // unpinned: only the bare Component is part of this product's data
			}
			componentReleaseUUID := *ref.Release

			componentRelease, err := r.GetComponentRelease(ctx, componentReleaseUUID)
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("get component release %s: %w", componentReleaseUUID, err)
			}
			collectDistributionFileHashes(fileHashes, componentRelease.Distributions)

			componentReleaseCLE, err := r.GetCLE(ctx, repo.OwnerComponentRelease, componentReleaseUUID)
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("get CLE for component release %s: %w", componentReleaseUUID, err)
			}
			componentReleaseEntries = append(componentReleaseEntries, ComponentReleaseEntry{ComponentRelease: componentRelease, CLE: &componentReleaseCLE})

			releaseCollections, err := r.ListCollections(ctx, componentReleaseUUID, "asc", nil, exportFetchLimit, repo.BelongsToComponentRelease)
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("list collections for component release %s: %w", componentReleaseUUID, err)
			}
			collections = append(collections, releaseCollections...)
			collectFileHashes(fileHashes, releaseCollections)
		}
	}

	manifest := Manifest{
		FormatVersion:     FormatVersion,
		ExportedAt:        time.Now().UTC(),
		Product:           ProductEntry{Product: product, CLE: &productCLE},
		ProductReleases:   releaseEntries,
		Components:        componentEntries,
		ComponentReleases: componentReleaseEntries,
		Collections:       collections,
	}
	return manifest, fileHashes, nil
}

func collectFileHashes(hashes map[string]struct{}, collections []tea.Collection) {
	for _, c := range collections {
		for _, artifact := range c.Artifacts {
			for _, format := range artifact.Formats {
				collectSHA256(hashes, format.Checksums)
			}
		}
	}
}

func collectDistributionFileHashes(hashes map[string]struct{}, dists []tea.ReleaseDistribution) {
	for _, d := range dists {
		collectSHA256(hashes, d.Checksums)
	}
}

func collectSHA256(hashes map[string]struct{}, checksums []tea.Checksum) {
	for _, c := range checksums {
		if c.AlgType == tea.ChecksumTypeSHA256 {
			hashes[c.AlgValue] = struct{}{}
		}
	}
}
