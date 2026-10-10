// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/testdataset"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// CheckLifecycle runs every data-correctness check the Lifecycle &
// compliance reference dataset (internal/testdataset.Lifecycle, see
// docs/consumer-api-conformance-test-rig.md's "Lifecycle & compliance"
// row) supports against a live server reachable through client: every
// CLEEventType value at every entity level the spec defines cle on,
// COMPLIANCE_DOCUMENT identifiers on the two entity kinds the spec
// restricts them to, and a real compliance artifact with embedded,
// checksum-verified content.
//
// Not checked here (a real, documented gap -- see
// docs/bundle-import-export-test-rig.md): the dataset's one
// artifactFormat.signatureUrl is an external placeholder, not a real
// address, so it is never fetched (that would make a live outbound
// request to a domain this suite doesn't control). The bundle format has
// no way to carry a *self-hosted* signature (no files/-style embedding
// for one), so no reference dataset can exercise
// .../signature/download's self-hosted path today -- it is expected to
// show up as never-called in the coverage report, honestly, rather than
// be worked around.
func CheckLifecycle(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	m, _ := testdataset.Lifecycle()
	byKey := indexCollections(m.Collections)
	resolver := NewResolver(client)

	productPURL, _ := purlOf(m.Product.Identifiers)
	product, err := resolver.ResolveProductByPURL(ctx, productPURL)
	cor.Check(err == nil, "Lifecycle: product resolves by PURL")
	if err != nil {
		return
	}
	checkMarkerUUID(cor, "Lifecycle: product name marker", product.Name, m.Product.UUID)

	productCLE, err := client.GetCLEByProduct(ctx, product.UUID)
	cor.Check(err == nil && len(productCLE.Events) == 2, "Lifecycle: product CLE has exactly 2 events")
	cor.Check(hasEventType(productCLE.Events, tea.CLEEventTypeReleased), "Lifecycle: product CLE includes a 'released' event")
	cor.Check(hasEventType(productCLE.Events, tea.CLEEventTypeEndOfMarketing), "Lifecycle: product CLE includes an 'endOfMarketing' event")

	for _, entry := range m.ProductReleases {
		checkLifecycleProductRelease(ctx, cor, client, resolver, entry)
	}

	comp1PURL, _ := purlOf(m.Components[0].Identifiers)
	comp1, err := resolver.ResolveComponentByPURL(ctx, comp1PURL)
	cor.Check(err == nil, "Lifecycle: component 1 resolves by PURL")
	if err == nil {
		checkMarkerUUID(cor, "Lifecycle: component 1 name marker", comp1.Name, m.Components[0].UUID)
		cor.Check(hasComplianceDocument(comp1.Identifiers, tea.ComplianceDocumentTypeISO27001),
			"Lifecycle: component 1 carries the ISO_27001 COMPLIANCE_DOCUMENT identifier")

		comp1CLE, err := client.GetCLEByComponent(ctx, comp1.UUID)
		cor.Check(err == nil && len(comp1CLE.Events) == 2, "Lifecycle: component 1 CLE has exactly 2 events")
		cor.Check(hasEventType(comp1CLE.Events, tea.CLEEventTypeEndOfSupport), "Lifecycle: component 1 CLE includes 'endOfSupport'")
		cor.Check(hasEventType(comp1CLE.Events, tea.CLEEventTypeComponentRenamed), "Lifecycle: component 1 CLE includes 'componentRenamed'")
	}

	comp2PURL, _ := purlOf(m.Components[1].Identifiers)
	comp2, err := resolver.ResolveComponentByPURL(ctx, comp2PURL)
	cor.Check(err == nil, "Lifecycle: component 2 resolves by PURL")
	if err == nil {
		comp2CLE, err := client.GetCLEByComponent(ctx, comp2.UUID)
		cor.Check(err == nil && len(comp2CLE.Events) == 0, "Lifecycle: component 2 CLE has no events")
	}

	// Component 1 release 1.0.0: the deepest check in this dataset -- 3
	// CLE event types (endOfLife, supersededBy, withdrawn), the
	// SOC_2_TYPE_II compliance identifier (the one TEA 1.0 allows at the
	// componentRelease level), and the one real embedded compliance
	// artifact, alongside an ordinary SBOM.
	checkLifecycleComp1Release1(ctx, cor, client, resolver, byKey, m.ComponentReleases[0])
}

// checkLifecycleProductRelease checks one product release entry: PURL
// resolution, CLE (present and shaped correctly for 1.0.0, absent for
// 2.0.0 -- entry.CLE's own nilness tells us which to expect), and its
// collection's single artifact.
func checkLifecycleProductRelease(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, resolver *Resolver, entry bundle.ProductReleaseEntry) {
	prPURL, _ := purlOf(entry.Identifiers)
	pr, err := resolver.ResolveProductReleaseByPURL(ctx, prPURL)
	cor.Check(err == nil, "Lifecycle: product release "+entry.Version+" resolves by PURL")
	if err != nil {
		return
	}
	prCLE, err := client.GetCLEByProductRelease(ctx, pr.UUID)
	cor.Check(err == nil, "Lifecycle: product release "+entry.Version+" CLE fetch succeeds")
	if err != nil {
		return
	}
	if entry.CLE != nil {
		cor.Check(len(prCLE.Events) == 2, "Lifecycle: product release "+entry.Version+" CLE has exactly 2 events")
		cor.Check(hasEventType(prCLE.Events, tea.CLEEventTypeEndOfDevelopment), "Lifecycle: product release "+entry.Version+" CLE includes 'endOfDevelopment'")
		cor.Check(hasEventType(prCLE.Events, tea.CLEEventTypeEndOfDistribution), "Lifecycle: product release "+entry.Version+" CLE includes 'endOfDistribution'")
		cor.Check(prCLE.Definitions != nil && len(prCLE.Definitions.Support) >= 1,
			"Lifecycle: product release "+entry.Version+" CLE has a support definition")
	} else {
		cor.Check(len(prCLE.Events) == 0, "Lifecycle: product release "+entry.Version+" CLE has no events")
	}

	withCollection, err := client.GetProductReleaseWithCollection(ctx, pr.UUID)
	cor.Check(err == nil && len(withCollection.LatestCollection.Artifacts) == 1,
		"Lifecycle: product release "+entry.Version+" collection has exactly 1 artifact")
}

// checkLifecycleComp1Release1 is the dataset's deepest check: 3 CLE event
// types (endOfLife, supersededBy, withdrawn), the SOC_2_TYPE_II compliance
// identifier (the one TEA 1.0 allows at the componentRelease level), and
// the one real embedded compliance artifact, alongside an ordinary SBOM.
func checkLifecycleComp1Release1(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, resolver *Resolver,
	byKey map[collectionKey]tea.Collection, comp1Rel1Entry bundle.ComponentReleaseEntry,
) {
	comp1Rel1PURL, _ := purlOf(comp1Rel1Entry.Identifiers)
	comp1Rel1, err := resolver.ResolveComponentReleaseByPURL(ctx, comp1Rel1PURL)
	cor.Check(err == nil, "Lifecycle: component 1 release 1.0.0 resolves by PURL")
	if err != nil {
		return
	}
	cor.Check(hasComplianceDocument(comp1Rel1.Identifiers, tea.ComplianceDocumentTypeSOC2TypeII),
		"Lifecycle: component 1 release 1.0.0 carries the SOC_2_TYPE_II COMPLIANCE_DOCUMENT identifier")

	crCLE, err := client.GetCLEByComponentRelease(ctx, comp1Rel1.UUID)
	cor.Check(err == nil && len(crCLE.Events) == 3, "Lifecycle: component 1 release 1.0.0 CLE has exactly 3 events")
	cor.Check(hasEventType(crCLE.Events, tea.CLEEventTypeEndOfLife), "Lifecycle: component 1 release 1.0.0 CLE includes 'endOfLife'")
	cor.Check(hasEventType(crCLE.Events, tea.CLEEventTypeSupersededBy), "Lifecycle: component 1 release 1.0.0 CLE includes 'supersededBy'")
	cor.Check(hasEventType(crCLE.Events, tea.CLEEventTypeWithdrawn), "Lifecycle: component 1 release 1.0.0 CLE includes 'withdrawn'")
	for _, e := range crCLE.Events {
		if e.Type == tea.CLEEventTypeSupersededBy {
			cor.Check(e.SupersededByVersion == "2.0.0", "Lifecycle: component 1 release 1.0.0's 'supersededBy' event names version 2.0.0")
		}
		if e.Type == tea.CLEEventTypeWithdrawn {
			cor.Check(e.EventID != nil, "Lifecycle: component 1 release 1.0.0's 'withdrawn' event references another event by id")
		}
	}

	withCollection, err := client.GetComponentReleaseWithCollection(ctx, comp1Rel1.UUID)
	cor.Check(err == nil && len(withCollection.LatestCollection.Artifacts) == 2,
		"Lifecycle: component 1 release 1.0.0 collection has exactly 2 artifacts")
	if err != nil {
		return
	}
	expectedCollection := byKey[collectionKey{comp1Rel1Entry.UUID, 1}]
	checkCollectionMarker(cor, "Lifecycle: component 1 release 1.0.0 collection marker matches its owner",
		withCollection.LatestCollection, comp1Rel1Entry.UUID, 1)

	expectedSBOMUUID := markerUUIDOf(expectedCollection.Artifacts[0])
	cor.Check(hasArtifactMarker(withCollection.LatestCollection.Artifacts, expectedSBOMUUID),
		"Lifecycle: component 1 release 1.0.0's SBOM round-trips")

	expectedCertUUID := markerUUIDOf(expectedCollection.Artifacts[1])
	cert, found := findArtifactByMarker(withCollection.LatestCollection.Artifacts, expectedCertUUID)
	cor.Check(found, "Lifecycle: component 1 release 1.0.0's compliance certificate round-trips")
	if found {
		cor.Check(cert.Type == tea.ArtifactTypeCertification, "Lifecycle: the compliance certificate's type is CERTIFICATION")
		checkCertDownload(ctx, cor, client, cert)
	}

	sbom, found := findArtifactByMarker(withCollection.LatestCollection.Artifacts, expectedSBOMUUID)
	if found && len(sbom.Formats) > 0 {
		cor.Check(sbom.Formats[0].SignatureURL != "", "Lifecycle: component 1 release 1.0.0's SBOM format carries a signatureUrl")
	}
}

func hasEventType(events []tea.CLEEvent, want string) bool {
	for _, e := range events {
		if e.Type == want {
			return true
		}
	}
	return false
}

func hasComplianceDocument(ids []tea.Identifier, want string) bool {
	for _, id := range ids {
		if id.IDType == tea.IdentifierTypeComplianceDocument && id.IDValue == want {
			return true
		}
	}
	return false
}

func findArtifactByMarker(artifacts []tea.Artifact, wantUUID string) (tea.Artifact, bool) {
	for _, a := range artifacts {
		if got, ok := testdataset.ExtractMarkerUUID(a.Name); ok && got == wantUUID {
			return a, true
		}
	}
	return tea.Artifact{}, false
}

// checkCertDownload exercises DownloadAndVerify against the compliance
// certificate's embedded PDF content -- the dataset's one real compliance
// artifact, see testdataset.Lifecycle's own doc comment.
func checkCertDownload(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, cert tea.Artifact) {
	for _, f := range cert.Formats {
		if len(f.Checksums) == 0 {
			continue
		}
		_, err := client.DownloadAndVerify(ctx, cert.UUID, cert.Version, f)
		cor.Check(err == nil, "Lifecycle: compliance certificate downloads and verifies against its checksum")
	}
}
