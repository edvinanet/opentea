// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"bytes"
	"context"
	"fmt"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/testdataset"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// collectionKey identifies one authored collection entry in a Manifest's
// Collections slice by its owner UUID and version -- used below to look
// up the exact authored Artifacts a given collection carries (names,
// markers, counts) without needing to re-derive anything testdataset's
// unexported helpers already computed once.
type collectionKey struct {
	uuid    string
	version int
}

func indexCollections(cols []tea.Collection) map[collectionKey]tea.Collection {
	out := make(map[collectionKey]tea.Collection, len(cols))
	for _, c := range cols {
		out[collectionKey{c.UUID, c.Version}] = c
	}
	return out
}

// markerUUIDOf extracts the authored UUID embedded in a's Name -- a test
// helper's own fatal-on-missing variant would be wrong here (this runs
// against suite-authored fixtures, never live server data, so a missing
// marker is this package's own bug, not something to tolerate silently).
func markerUUIDOf(a tea.Artifact) string {
	uuid, ok := testdataset.ExtractMarkerUUID(a.Name)
	if !ok {
		panic(fmt.Sprintf("conformance: artifact %q has no embedded marker -- a bug in internal/testdataset, not in a check", a.Name))
	}
	return uuid
}

// CheckComplex runs every data-correctness check the Complex reference
// dataset (internal/testdataset.Complex, see
// docs/consumer-api-conformance-test-rig.md's "Complex" row) supports
// against a live server reachable through client: pagination-scale
// identifier resolution (100 component releases), shared-artifact
// reachability across many collections, a component release pinned
// unchanged by more than one product release, a multi-version
// product-release collection history, and a multi-format artifact with a
// real downloadable, checksum-verified format.
func CheckComplex(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	m, _ := testdataset.Complex()
	byKey := indexCollections(m.Collections)
	resolver := NewResolver(client)

	productPURL, _ := purlOf(m.Product.Identifiers)
	product, err := resolver.ResolveProductByPURL(ctx, productPURL)
	cor.Check(err == nil, "Complex: product resolves by PURL")
	if err != nil {
		return
	}
	checkMarkerUUID(cor, "Complex: product name marker", product.Name, m.Product.UUID)

	releases, err := client.ListReleasesByProduct(ctx, product.UUID, teaclient.ListParams{PageSize: 50})
	cor.Check(err == nil && len(releases.Results) == 3, "Complex: product has exactly 3 releases")

	// Expected component-ref counts and collection shapes per product
	// release, mirroring complex.go's own pin schedule and
	// productReleaseHistory/case-switch exactly (see that file's comments
	// for why these specific numbers).
	wantComponentRefs := map[string]int{"1.0.0": 6, "2.0.0": 8, "3.0.0": 10}
	wantLatestVersion := map[string]int{"1.0.0": 4, "2.0.0": 1, "3.0.0": 1}
	wantLatestArtifactCount := map[string]int{"1.0.0": 3, "2.0.0": 2, "3.0.0": 3}

	componentsByVersion := map[string][]tea.ComponentRef{}
	for _, entry := range m.ProductReleases {
		components, ok := checkComplexProductRelease(ctx, cor, client, resolver, byKey, entry,
			wantComponentRefs[entry.Version], wantLatestVersion[entry.Version], wantLatestArtifactCount[entry.Version])
		if ok {
			componentsByVersion[entry.Version] = components
		}
	}

	// Component 3 is pinned unchanged across all three product releases
	// (complex.go's pin schedule: {0,0,0}) -- the same real server
	// componentRelease UUID must appear in every one of pr1/pr2/pr3's
	// Components for component 3 specifically, not just some component
	// that happened to match.
	comp3PURL, _ := purlOf(m.Components[2].Identifiers)
	comp3, err := resolver.ResolveComponentByPURL(ctx, comp3PURL)
	cor.Check(err == nil, "Complex: component 3 resolves by PURL (for the cross-release pin check)")
	if err == nil {
		checkPinnedAcrossReleases(cor, "Complex: component 3's release is pinned unchanged by all 3 product releases",
			comp3.UUID, componentsByVersion["1.0.0"], componentsByVersion["2.0.0"], componentsByVersion["3.0.0"])
	}

	for i := 1; i <= 10; i++ {
		checkComplexComponent(ctx, cor, client, resolver, byKey, m, i)
	}
}

// checkComplexProductRelease checks one product release entry (resolve by
// PURL, fetch, component-ref count, latest-collection shape, the shared
// license artifact, and -- for 1.0.0 only -- its 4-version collection
// history; for 3.0.0, the multi-format certificate). Returns the live
// Components list (for the cross-release pin check) and whether the
// checks ran far enough to produce one.
func checkComplexProductRelease(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, resolver *Resolver,
	byKey map[collectionKey]tea.Collection, entry bundle.ProductReleaseEntry,
	wantComponentRefs, wantLatestVersion, wantLatestArtifactCount int,
) ([]tea.ComponentRef, bool) {
	version := entry.Version
	prPURL, _ := purlOf(entry.Identifiers)
	pr, err := resolver.ResolveProductReleaseByPURL(ctx, prPURL)
	cor.Check(err == nil, "Complex: product release "+version+" resolves by PURL")
	if err != nil {
		return nil, false
	}

	withCollection, err := client.GetProductReleaseWithCollection(ctx, pr.UUID)
	cor.Check(err == nil, "Complex: product release "+version+" fetch succeeds")
	if err != nil {
		return nil, false
	}
	cor.Check(len(withCollection.Components) == wantComponentRefs,
		fmt.Sprintf("Complex: product release %s has %d component refs", version, wantComponentRefs))
	cor.Check(withCollection.LatestCollection.Version == wantLatestVersion,
		fmt.Sprintf("Complex: product release %s latest collection is version %d", version, wantLatestVersion))
	cor.Check(len(withCollection.LatestCollection.Artifacts) == wantLatestArtifactCount,
		fmt.Sprintf("Complex: product release %s latest collection has %d artifacts", version, wantLatestArtifactCount))

	expectedLicenseUUID := markerUUIDOf(byKey[collectionKey{entry.UUID, 1}].Artifacts[1])
	cor.Check(hasArtifactMarker(withCollection.LatestCollection.Artifacts, expectedLicenseUUID),
		"Complex: product release "+version+" latest collection carries the shared license artifact")

	switch version {
	case "1.0.0":
		all, err := client.ListCollectionsForProductRelease(ctx, pr.UUID, teaclient.ListParams{PageSize: 10})
		cor.Check(err == nil && len(all.Results) == 4, "Complex: product release 1.0.0 has exactly 4 collection versions")
		wantArtifactCounts := []int{2, 3, 4, 3}
		for v := 1; v <= 4; v++ {
			col, err := client.GetCollectionForProductRelease(ctx, pr.UUID, v)
			cor.Check(err == nil && len(col.Artifacts) == wantArtifactCounts[v-1],
				fmt.Sprintf("Complex: product release 1.0.0 collection v%d has %d artifacts", v, wantArtifactCounts[v-1]))
		}
	case "3.0.0":
		checkCertificate(ctx, cor, client, byKey, entry.UUID, withCollection.LatestCollection.Artifacts)
	}
	return withCollection.Components, true
}

// checkComplexComponent checks component index i (1-based): resolve by
// PURL, marker, release count, then every one of its 10 releases via
// checkComplexComponentRelease.
func checkComplexComponent(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, resolver *Resolver,
	byKey map[collectionKey]tea.Collection, m bundle.Manifest, i int,
) {
	entry := m.Components[i-1]
	compPURL, _ := purlOf(entry.Identifiers)
	comp, err := resolver.ResolveComponentByPURL(ctx, compPURL)
	cor.Check(err == nil, fmt.Sprintf("Complex: component %d resolves by PURL", i))
	if err != nil {
		return
	}
	checkMarkerUUID(cor, fmt.Sprintf("Complex: component %d name marker", i), comp.Name, entry.UUID)

	compReleases, err := client.ListReleasesByComponent(ctx, comp.UUID, teaclient.ListParams{PageSize: 20})
	cor.Check(err == nil && len(compReleases.Results) == 10, fmt.Sprintf("Complex: component %d has exactly 10 releases", i))

	wantArtifactCount := 2
	if i <= 5 {
		wantArtifactCount = 3
	}
	for v := 1; v <= 10; v++ {
		crEntry := m.ComponentReleases[(i-1)*10+(v-1)]
		checkComplexComponentRelease(ctx, cor, client, resolver, byKey, crEntry, i, v, wantArtifactCount)
	}
}

// checkComplexComponentRelease checks one (component index, release index)
// pair: resolve by PURL, fetch its collection, shared-artifact
// reachability (license always, base-image iff i<=5), its own SBOM
// round-tripping, the collection's own marker, and -- for component 1
// release 1 only -- the one real embedded-content download in this
// dataset.
func checkComplexComponentRelease(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, resolver *Resolver,
	byKey map[collectionKey]tea.Collection, crEntry bundle.ComponentReleaseEntry, i, v, wantArtifactCount int,
) {
	crPURL, _ := purlOf(crEntry.Identifiers)
	cr, err := resolver.ResolveComponentReleaseByPURL(ctx, crPURL)
	cor.Check(err == nil, fmt.Sprintf("Complex: component %d release %d resolves by PURL", i, v))
	if err != nil {
		return
	}
	withCollection, err := client.GetComponentReleaseWithCollection(ctx, cr.UUID)
	cor.Check(err == nil, fmt.Sprintf("Complex: component %d release %d fetch succeeds", i, v))
	if err != nil {
		return
	}
	cor.Check(len(withCollection.LatestCollection.Artifacts) == wantArtifactCount,
		fmt.Sprintf("Complex: component %d release %d collection has %d artifacts", i, v, wantArtifactCount))

	expectedCollection := byKey[collectionKey{crEntry.UUID, 1}]
	expectedLicenseUUID := markerUUIDOf(expectedCollection.Artifacts[1])
	cor.Check(hasArtifactMarker(withCollection.LatestCollection.Artifacts, expectedLicenseUUID),
		fmt.Sprintf("Complex: component %d release %d collection carries the shared license artifact", i, v))
	if i <= 5 {
		expectedBaseImageUUID := markerUUIDOf(expectedCollection.Artifacts[2])
		cor.Check(hasArtifactMarker(withCollection.LatestCollection.Artifacts, expectedBaseImageUUID),
			fmt.Sprintf("Complex: component %d release %d collection carries the shared base-image artifact", i, v))
	}
	expectedSBOMUUID := markerUUIDOf(expectedCollection.Artifacts[0])
	cor.Check(hasArtifactMarker(withCollection.LatestCollection.Artifacts, expectedSBOMUUID),
		fmt.Sprintf("Complex: component %d release %d collection's own SBOM round-trips", i, v))
	checkCollectionMarker(cor, fmt.Sprintf("Complex: component %d release %d collection marker matches its owner", i, v),
		withCollection.LatestCollection, crEntry.UUID, 1)

	if i == 1 && v == 1 {
		checkEmbeddedSBOMDownload(ctx, cor, client, withCollection.LatestCollection.Artifacts, expectedSBOMUUID)
	}
}

// hasArtifactMarker reports whether any artifact in artifacts carries
// wantUUID's embedded marker -- the shared-artifact-reachability check:
// not "is the UUID literally equal" (it won't be, post-import), but "is
// this recognizably the same authored artifact."
func hasArtifactMarker(artifacts []tea.Artifact, wantUUID string) bool {
	for _, a := range artifacts {
		if got, ok := testdataset.ExtractMarkerUUID(a.Name); ok && got == wantUUID {
			return true
		}
	}
	return false
}

// checkPinnedAcrossReleases asserts that compUUID's pinned componentRelease
// (ComponentRef.Release) is the exact same real server UUID in every one
// of pr1/pr2/pr3's Components lists -- proof that one real component
// release is reachable from more than one product release, not just that
// three authored UUIDs happened to match before import.
func checkPinnedAcrossReleases(cor *CorrectnessTracker, label, compUUID string, pr1, pr2, pr3 []tea.ComponentRef) {
	find := func(refs []tea.ComponentRef) (releaseUUID string, ok bool) {
		for _, ref := range refs {
			if ref.UUID == compUUID && ref.Release != nil {
				return *ref.Release, true
			}
		}
		return "", false
	}
	rel1, ok1 := find(pr1)
	rel2, ok2 := find(pr2)
	rel3, ok3 := find(pr3)
	cor.Check(ok1 && ok2 && ok3 && rel1 == rel2 && rel2 == rel3, label)
}

// checkCertificate validates the one multi-format artifact in the Complex
// dataset: two formats on the same artifact, found in product release
// 3.0.0's collection, one with real downloadable, checksum-verified
// content.
func checkCertificate(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, byKey map[collectionKey]tea.Collection, authoredReleaseUUID string, liveArtifacts []tea.Artifact) {
	expectedCert := byKey[collectionKey{authoredReleaseUUID, 1}].Artifacts[2]
	expectedCertUUID := markerUUIDOf(expectedCert)

	var cert tea.Artifact
	found := false
	for _, a := range liveArtifacts {
		if got, ok := testdataset.ExtractMarkerUUID(a.Name); ok && got == expectedCertUUID {
			cert = a
			found = true
			break
		}
	}
	cor.Check(found, "Complex: compliance certificate artifact round-trips into product release 3.0.0's collection")
	if !found {
		return
	}
	cor.Check(len(cert.Formats) == 2, "Complex: compliance certificate has exactly 2 formats")

	latest, err := client.GetLatestArtifact(ctx, cert.UUID)
	cor.Check(err == nil && latest.UUID == cert.UUID, "Complex: GET /artifact/{uuid}/latest agrees with the collection's own copy")
	byVersion, err := client.GetArtifactByVersion(ctx, cert.UUID, cert.Version)
	cor.Check(err == nil && byVersion.UUID == cert.UUID, "Complex: GET /artifact/{uuid}/{version} agrees with the collection's own copy")

	for _, f := range cert.Formats {
		if f.MediaType != "application/pdf" {
			continue
		}
		_, err := client.DownloadAndVerify(ctx, cert.UUID, cert.Version, f)
		cor.Check(err == nil, "Complex: compliance certificate PDF format downloads and verifies against its checksum")

		var buf bytes.Buffer
		err = client.DownloadLatestAndVerifyTo(ctx, cert.UUID, f, &buf)
		cor.Check(err == nil, "Complex: compliance certificate PDF format downloads and verifies via the latest-revision endpoint")
	}
}

// checkEmbeddedSBOMDownload exercises DownloadAndVerify against the one
// component-release SBOM in the whole Complex dataset that carries real
// embedded content (component 1, release 1 -- see buildComponentLadder's
// own comment for why only this one does).
func checkEmbeddedSBOMDownload(ctx context.Context, cor *CorrectnessTracker, client *teaclient.Client, liveArtifacts []tea.Artifact, expectedSBOMUUID string) {
	for _, a := range liveArtifacts {
		got, ok := testdataset.ExtractMarkerUUID(a.Name)
		if !ok || got != expectedSBOMUUID {
			continue
		}
		for _, f := range a.Formats {
			_, err := client.DownloadAndVerify(ctx, a.UUID, a.Version, f)
			cor.Check(err == nil, "Complex: component 1 release 1's embedded SBOM downloads and verifies against its checksum")
		}
	}
}
