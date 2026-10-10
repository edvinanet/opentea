// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"

	"github.com/oej/opentea/internal/testdataset"
	"github.com/oej/opentea/pkg/teaclient"
)

// CheckBadData verifies a conformant client correctly detects the two
// kinds of corruption dataset 4 half B force-imports onto a disposable
// test server (docs/bundle-import-export-test-rig.md's "Force-import"
// section) rather than silently accepting bad data as if it were fine.
// This is the payoff docs/consumer-api-conformance-test-rig.md's
// "Deliberately invalid data" section describes.
//
// Unlike CheckSimple/CheckComplex/CheckLifecycle, a passing run here
// means detecting an error, not getting a clean result -- call this only
// against a server that has had testdataset.BadChecksumMismatch and
// testdataset.BadMissingFile force-imported onto it. Never run it against
// an ordinary conformant deployment, which should never have this data at
// all.
func CheckBadData(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	checkBadChecksumMismatchDetected(ctx, client, cor)
	checkBadMissingFileDetected(ctx, client, cor)
}

// checkBadChecksumMismatchDetected resolves BadChecksumMismatch's one
// artifact and confirms a conformant client (DownloadAndVerify) detects
// that its downloaded content doesn't match its declared checksum, rather
// than returning the mismatched bytes as if they were verified.
func checkBadChecksumMismatchDetected(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	m, _ := testdataset.BadChecksumMismatch()
	resolver := NewResolver(client)

	crPURL, _ := purlOf(m.ComponentReleases[0].Identifiers)
	cr, err := resolver.ResolveComponentReleaseByPURL(ctx, crPURL)
	cor.Check(err == nil, "Bad data (checksum mismatch): component release resolves by PURL")
	if err != nil {
		return
	}
	withCollection, err := client.GetComponentReleaseWithCollection(ctx, cr.UUID)
	cor.Check(err == nil && len(withCollection.LatestCollection.Artifacts) == 1,
		"Bad data (checksum mismatch): collection has exactly 1 artifact")
	if err != nil || len(withCollection.LatestCollection.Artifacts) != 1 {
		return
	}
	artifact := withCollection.LatestCollection.Artifacts[0]
	cor.Check(len(artifact.Formats) == 1, "Bad data (checksum mismatch): artifact has exactly 1 format")
	if len(artifact.Formats) != 1 {
		return
	}

	_, downloadErr := client.DownloadAndVerify(ctx, artifact.UUID, artifact.Version, artifact.Formats[0])
	cor.Check(downloadErr != nil, "Bad data (checksum mismatch): a conformant client detects the corrupted content via DownloadAndVerify")
}

// checkBadMissingFileDetected resolves BadMissingFile's one artifact and
// confirms a conformant client's download attempt fails cleanly (the
// server has no content for the declared checksum at all) rather than
// hanging, crashing, or reporting success with no content.
func checkBadMissingFileDetected(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	m, _ := testdataset.BadMissingFile()
	resolver := NewResolver(client)

	crPURL, _ := purlOf(m.ComponentReleases[0].Identifiers)
	cr, err := resolver.ResolveComponentReleaseByPURL(ctx, crPURL)
	cor.Check(err == nil, "Bad data (missing file): component release resolves by PURL")
	if err != nil {
		return
	}
	withCollection, err := client.GetComponentReleaseWithCollection(ctx, cr.UUID)
	cor.Check(err == nil && len(withCollection.LatestCollection.Artifacts) == 1,
		"Bad data (missing file): collection has exactly 1 artifact")
	if err != nil || len(withCollection.LatestCollection.Artifacts) != 1 {
		return
	}
	artifact := withCollection.LatestCollection.Artifacts[0]
	cor.Check(len(artifact.Formats) == 1, "Bad data (missing file): artifact has exactly 1 format")
	if len(artifact.Formats) != 1 {
		return
	}

	_, downloadErr := client.DownloadAndVerify(ctx, artifact.UUID, artifact.Version, artifact.Formats[0])
	cor.Check(downloadErr != nil, "Bad data (missing file): a conformant client detects the unavailable content via DownloadAndVerify")
}
