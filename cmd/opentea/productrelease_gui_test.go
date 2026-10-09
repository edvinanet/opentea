// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/openteapublisher"
	"github.com/oej/opentea/pkg/teaclient"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// newGUITestTarget stands up a real internal/publisher target and a real
// opentea-publisher server pointed at it via a stored Target holding a
// genuine full-scoped credential -- the GUI-side counterpart of
// TestCICDAPIProxiesToRealTarget's own setup.
func newGUITestTarget(t *testing.T) (target *testServer, fullToken string, r *openteapublisher.Repo, openteaTarget openteapublisher.Target, opSrv *httptest.Server) {
	t.Helper()
	target = newTestServer(t)
	fullToken = createPublisherCredential(t, target, "full-cred", "full")

	sqlDB, err := openteapublisher.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if err != nil {
		t.Fatalf("openteapublisher.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r = openteapublisher.New(sqlDB)

	openteaTarget, err = r.CreateTarget(context.Background(), "test target", target.URL+"/publisher/v1", fullToken)
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	opSrv = httptest.NewServer(nil)
	t.Cleanup(opSrv.Close)
	opSrv.Config.Handler = openteapublisher.NewRouter(r, openteapublisher.Config{RootURL: opSrv.URL})
	return target, fullToken, r, openteaTarget, opSrv
}

// createGUIProductRelease drives create-product + create-release through
// the GUI's own routes (products.go) and returns the resulting release's
// UUID and its /targets/.../productReleases/{uuid} path.
func createGUIProductRelease(t *testing.T, opSrv *httptest.Server, client *http.Client, targetUUID string) (releaseUUID, releasePath string) {
	t.Helper()
	createProductResp := guiPostForm(t, opSrv, client, "/targets/"+targetUUID+"/products", url.Values{"name": {"Acme Widget"}})
	if createProductResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(createProductResp.Body)
		t.Fatalf("create product: status=%d body=%s", createProductResp.StatusCode, body)
	}
	productUUID := guiLastPathSegment(t, createProductResp)
	_ = createProductResp.Body.Close()

	createReleaseResp := guiPostForm(t, opSrv, client, "/targets/"+targetUUID+"/products/"+productUUID+"/releases", url.Values{"version": {"1.0.0"}})
	if createReleaseResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(createReleaseResp.Body)
		t.Fatalf("create release: status=%d body=%s", createReleaseResp.StatusCode, body)
	}
	releaseUUID = guiLastPathSegment(t, createReleaseResp)
	_ = createReleaseResp.Body.Close()
	return releaseUUID, "/targets/" + targetUUID + "/productReleases/" + releaseUUID
}

// createRealArtifact creates and uploads an artifact directly against the
// target (simulating CI/CD calling /publisher/v1 or /cicdapi/v1 directly)
// -- the GUI's draft panel only ever references an artifact by
// UUID+version, never creates one itself (Artifacts review, §18.5, is
// out of scope for this pass).
func createRealArtifact(t *testing.T, target *testServer, token string) teapublisher.ArtifactCreated {
	t.Helper()
	client := teapublisherclient.NewClient(target.URL+"/publisher/v1", token)
	artifact, err := client.CreateArtifact(context.Background(), teapublisher.ArtifactCreate{
		Type:    "BOM",
		Formats: []teapublisher.ArtifactFormatCreate{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := client.UploadArtifactFile(context.Background(), artifact.UUID, artifact.Version, "application/vnd.cyclonedx+json", "sbom.json", strings.NewReader(`{"ok":true}`)); err != nil {
		t.Fatalf("UploadArtifactFile: %v", err)
	}
	return artifact
}

// TestPublisherGUICollectionDraftAddRemove proves design/publisher-
// service.md §18.7's add/remove-artifact-by-reference mechanics work
// against a real target, driven entirely through the GUI's own
// collectionDraft route (GET-current-draft, mutate, PUT-with-
// ExpectedRevision). Approval/signing aren't exercised here -- see
// TestPublisherGUIApproveAndSignPublish for those, kept separate because
// of a real, documented limitation (TODO.md): a draft PUT through
// opentea-publisher can never itself be approved through opentea-
// publisher, since every GUI action for one target shares the exact same
// stored credential, and the target's own maker-checker rejects a
// same-credential decision regardless of which staff member clicked.
func TestPublisherGUICollectionDraftAddRemove(t *testing.T) {
	target, fullToken, r, openteaTarget, opSrv := newGUITestTarget(t)
	if _, err := r.CreateStaff(context.Background(), "dave", "hunter777drafter", openteapublisher.StaffRoleMember, ""); err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	client := guiLoggedInClient(t, opSrv, "dave", "hunter777drafter")

	artifact1 := createRealArtifact(t, target, fullToken)
	artifact2 := createRealArtifact(t, target, fullToken)

	_, releasePath := createGUIProductRelease(t, opSrv, client, openteaTarget.UUID)

	// Add artifact1.
	addResp1 := guiPostForm(t, opSrv, client, releasePath+"/collectionDraft", url.Values{
		"action": {"add"}, "artifactUuid": {artifact1.UUID}, "artifactVersion": {"1"},
	})
	body1, _ := io.ReadAll(addResp1.Body)
	_ = addResp1.Body.Close()
	if addResp1.StatusCode != http.StatusOK || !strings.Contains(string(body1), artifact1.UUID) {
		t.Fatalf("add artifact1: status=%d body=%s", addResp1.StatusCode, body1)
	}

	// Add artifact2 (revision bumps again).
	addResp2 := guiPostForm(t, opSrv, client, releasePath+"/collectionDraft", url.Values{
		"action": {"add"}, "artifactUuid": {artifact2.UUID}, "artifactVersion": {"1"},
	})
	_ = addResp2.Body.Close()
	if addResp2.StatusCode != http.StatusOK {
		t.Fatalf("add artifact2: status=%d", addResp2.StatusCode)
	}

	// Remove artifact1 -- only artifact2 should remain.
	removeResp := guiPostForm(t, opSrv, client, releasePath+"/collectionDraft", url.Values{
		"action": {"remove"}, "artifactUuid": {artifact1.UUID}, "artifactVersion": {"1"},
	})
	_ = removeResp.Body.Close()
	if removeResp.StatusCode != http.StatusOK {
		t.Fatalf("remove artifact1: status=%d", removeResp.StatusCode)
	}

	// Confirm against the *target* directly, bypassing opentea-publisher,
	// that the draft really ended up with exactly artifact2.
	directClient := teapublisherclient.NewClient(target.URL+"/publisher/v1", fullToken)
	draft, err := directClient.GetProductReleaseCollectionDraft(context.Background(), strings.TrimPrefix(releasePath, "/targets/"+openteaTarget.UUID+"/productReleases/"))
	if err != nil {
		t.Fatalf("target GetProductReleaseCollectionDraft: %v", err)
	}
	if len(draft.Artifacts) != 1 || draft.Artifacts[0].UUID != artifact2.UUID {
		t.Fatalf("draft.Artifacts = %+v, want exactly artifact2 (%s)", draft.Artifacts, artifact2.UUID)
	}
	if draft.Revision != 3 {
		t.Fatalf("draft.Revision = %d, want 3 (add, add, remove)", draft.Revision)
	}
}

// TestPublisherGUIApproveAndSignPublish proves design/publisher-
// service.md §18.9's protocol-level approve/reject and §18.10's "Sign &
// Publish" action both work against a real target, driven through the
// GUI. The draft itself is assembled directly against the target with a
// *different* credential than the one opentea-publisher stores for this
// target -- not through the GUI -- deliberately avoiding the same-
// credential self-approval collision TestPublisherGUICollectionDraftAddRemove's
// own doc comment names (a draft the GUI itself assembled can never be
// approved through the GUI, since every GUI action for one target shares
// its one stored credential). This mirrors a real, legitimate deployment
// shape too: CI/CD assembling a draft with its own direct target
// credential, handing off only the human approval/signing phase to
// opentea-publisher.
func TestPublisherGUIApproveAndSignPublish(t *testing.T) {
	target, fullToken, r, openteaTarget, opSrv := newGUITestTarget(t)
	draftingToken := createPublisherCredential(t, target, "direct-draft-cred", "full")

	if _, err := r.CreateStaff(context.Background(), "dave", "hunter777drafter", openteapublisher.StaffRoleMember, ""); err != nil {
		t.Fatalf("CreateStaff (drafter): %v", err)
	}
	if _, err := r.CreateStaff(context.Background(), "carol", "hunter444approve", openteapublisher.StaffRoleMember, openteapublisher.StaffWorkflowRoleSecurityComplianceApprover); err != nil {
		t.Fatalf("CreateStaff (approver): %v", err)
	}
	drafter := guiLoggedInClient(t, opSrv, "dave", "hunter777drafter")
	approver := guiLoggedInClient(t, opSrv, "carol", "hunter444approve")

	artifact := createRealArtifact(t, target, fullToken)

	releaseUUID, releasePath := createGUIProductRelease(t, opSrv, drafter, openteaTarget.UUID)

	draftingClient := teapublisherclient.NewClient(target.URL+"/publisher/v1", draftingToken)
	if _, err := draftingClient.PutProductReleaseCollectionDraft(context.Background(), releaseUUID, teapublisher.CollectionDraftArtifactList{
		Actor:     "ci-pipeline",
		Artifacts: []teapublisher.ArtifactVersionRef{{UUID: artifact.UUID, Version: artifact.Version}},
	}); err != nil {
		t.Fatalf("direct PutProductReleaseCollectionDraft: %v", err)
	}

	// The approver decides, through the GUI.
	approveResp := guiPostForm(t, opSrv, approver, releasePath+"/collectionDraft/approve", url.Values{"comment": {"lgtm"}})
	approveBody, _ := io.ReadAll(approveResp.Body)
	_ = approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK || strings.Contains(string(approveBody), `class="error"`) {
		t.Fatalf("approve: status=%d body=%s", approveResp.StatusCode, approveBody)
	}

	// Sign & Publish, through the GUI.
	publishResp := guiPostForm(t, opSrv, drafter, releasePath+"/collectionDraft/signAndPublish", url.Values{})
	publishBody, _ := io.ReadAll(publishResp.Body)
	_ = publishResp.Body.Close()
	if publishResp.StatusCode != http.StatusOK || strings.Contains(string(publishBody), `class="error"`) {
		t.Fatalf("signAndPublish: status=%d body=%s", publishResp.StatusCode, publishBody)
	}

	// Confirm the *target* really has the new, real, evidence-bound
	// collection -- not just that opentea-publisher claimed success.
	// Fetched independently off the target's own consumer /tea/v1,
	// bypassing opentea-publisher entirely.
	readClient := teaclient.NewClient(target.URL + "/tea/v1")
	collection, err := readClient.GetLatestCollectionForProductRelease(context.Background(), releaseUUID)
	if err != nil {
		t.Fatalf("target GetLatestCollectionForProductRelease: %v", err)
	}
	if collection.Version != 2 {
		t.Fatalf("collection.Version = %d, want 2 (createProductRelease already creates an empty v1)", collection.Version)
	}
	if len(collection.Artifacts) != 1 || collection.Artifacts[0].UUID != artifact.UUID {
		t.Fatalf("collection.Artifacts = %+v, want exactly the one artifact approved and published through the GUI", collection.Artifacts)
	}
	// tea.Collection.EvidenceBundle isn't populated by the standard
	// consumer read API (deliberate Phase 1 scope boundary --
	// internal/webadmin's own evidence badges look it up separately
	// rather than wiring it into this response shape), so evidence
	// presence is confirmed directly against the target's own repo
	// instead -- the most direct, authoritative check available.
	if _, err := target.repo.GetEvidenceBundleForOwner(context.Background(), "COLLECTION", releaseUUID, collection.Version); err != nil {
		t.Fatalf("target.repo.GetEvidenceBundleForOwner: %v, want a real evidence bundle for the published collection", err)
	}

	// The draft is gone (commit deletes it) -- getCollectionDraft 404s.
	if _, err := draftingClient.GetProductReleaseCollectionDraft(context.Background(), releaseUUID); !teapublisherclient.IsNotFound(err) {
		t.Fatalf("GetProductReleaseCollectionDraft after publish: err=%v, want 404 (draft deleted on commit)", err)
	}
}

func guiLoggedInClient(t *testing.T, srv *httptest.Server, username, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp := guiPostFormWith(t, client, srv.URL+"/login", url.Values{"username": {username}, "password": {password}})
	_ = resp.Body.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if len(jar.Cookies(u)) == 0 {
		t.Fatalf("login as %s did not set a session cookie (status=%d)", username, resp.StatusCode)
	}
	return client
}

func guiPostForm(t *testing.T, srv *httptest.Server, client *http.Client, path string, form url.Values) *http.Response {
	t.Helper()
	return guiPostFormWith(t, client, srv.URL+path, form)
}

func guiPostFormWith(t *testing.T, client *http.Client, fullURL string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request POST %s: %v", fullURL, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	u, err := url.Parse(fullURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	req.Header.Set("Origin", u.Scheme+"://"+u.Host)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", fullURL, err)
	}
	return resp
}

// guiLastPathSegment extracts the trailing UUID segment from a redirect
// response's final URL -- every create handler in products.go/
// collectiondraft.go redirects to the newly created resource's own detail
// page, so this is how the test learns the server-assigned UUID without
// needing a separate read call.
func guiLastPathSegment(t *testing.T, resp *http.Response) string {
	t.Helper()
	if resp.Request == nil {
		t.Fatal("response has no final Request to read the redirected-to URL from")
	}
	path := strings.TrimRight(resp.Request.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		t.Fatalf("could not extract a path segment from %s", path)
	}
	return parts[len(parts)-1]
}
