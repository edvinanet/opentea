// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// Every operation below exists in two owner-scoped forms -- ProductRelease
// and ComponentRelease -- matching internal/publisher's own route split
// (design/publisher-openapi.yaml only sketches the productReleases path to
// keep the draft short; componentReleases is the identical shape, not out
// of scope -- see its own comment at publisher-openapi.yaml:497-499). The
// private draft* methods hold the one real implementation each; the
// exported Put/Get/Delete/Approve/Reject/PrepareCollectionCommit/
// CancelPrepareCollectionCommit/CommitCollectionDraft methods are thin,
// path-prefix-selecting wrappers, mirroring cle.go's four-owner-type split.

func (c *Client) putCollectionDraft(ctx context.Context, pathPrefix, ownerUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error) {
	var draft teapublisher.CollectionDraft
	err := c.do(ctx, "PUT", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft", in, &draft)
	return draft, err
}

func (c *Client) getCollectionDraft(ctx context.Context, pathPrefix, ownerUUID string) (teapublisher.CollectionDraft, error) {
	var draft teapublisher.CollectionDraft
	err := c.do(ctx, "GET", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft", nil, &draft)
	return draft, err
}

func (c *Client) deleteCollectionDraft(ctx context.Context, pathPrefix, ownerUUID string) error {
	return c.do(ctx, "DELETE", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft", nil, nil)
}

func (c *Client) decideCollectionDraft(ctx context.Context, pathPrefix, ownerUUID, decision string, in teapublisher.ApprovalDecision) (teapublisher.CollectionDraft, error) {
	var draft teapublisher.CollectionDraft
	err := c.do(ctx, "POST", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft/"+decision, in, &draft)
	return draft, err
}

func (c *Client) prepareCollectionCommit(ctx context.Context, pathPrefix, ownerUUID string) (teapublisher.PrepareCommitResponse, error) {
	var resp teapublisher.PrepareCommitResponse
	err := c.do(ctx, "POST", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft/prepareCommit", nil, &resp)
	return resp, err
}

func (c *Client) cancelPrepareCollectionCommit(ctx context.Context, pathPrefix, ownerUUID string) error {
	return c.do(ctx, "POST", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft/cancelPrepare", nil, nil)
}

func (c *Client) commitCollectionDraft(ctx context.Context, pathPrefix, ownerUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error) {
	var collection tea.Collection
	err := c.do(ctx, "POST", "/"+pathPrefix+"/"+ownerUUID+"/collectionDraft/commit", in, &collection)
	return collection, err
}

// PutProductReleaseCollectionDraft creates or replaces
// productReleaseUUID's draft artifact list. Idempotent; callable
// repeatedly as artifacts arrive over however many sessions assembly
// takes. Bumps the draft's revision and resets its approval to "none".
// Returns an *APIError with StatusCode 409 (IsConflict) if the draft is
// locked by an outstanding PrepareCollectionCommit.
func (c *Client) PutProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error) {
	return c.putCollectionDraft(ctx, "productReleases", productReleaseUUID, in)
}

// PutComponentReleaseCollectionDraft is PutProductReleaseCollectionDraft
// for a component release.
func (c *Client) PutComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error) {
	return c.putCollectionDraft(ctx, "componentReleases", componentReleaseUUID, in)
}

// GetProductReleaseCollectionDraft fetches productReleaseUUID's current
// draft state, including a diff against the owner's current live
// collection.
func (c *Client) GetProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string) (teapublisher.CollectionDraft, error) {
	return c.getCollectionDraft(ctx, "productReleases", productReleaseUUID)
}

// GetComponentReleaseCollectionDraft is GetProductReleaseCollectionDraft
// for a component release.
func (c *Client) GetComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string) (teapublisher.CollectionDraft, error) {
	return c.getCollectionDraft(ctx, "componentReleases", componentReleaseUUID)
}

// DeleteProductReleaseCollectionDraft abandons productReleaseUUID's draft
// without publishing it.
func (c *Client) DeleteProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string) error {
	return c.deleteCollectionDraft(ctx, "productReleases", productReleaseUUID)
}

// DeleteComponentReleaseCollectionDraft is
// DeleteProductReleaseCollectionDraft for a component release.
func (c *Client) DeleteComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string) error {
	return c.deleteCollectionDraft(ctx, "componentReleases", componentReleaseUUID)
}

// ApproveProductReleaseCollectionDraft records approval of
// productReleaseUUID's draft at its *current* revision
// (design/publisher-service.md §7.9). in.Actor must differ from the
// draft's own draftedBy -- a self-approval is rejected with an *APIError
// StatusCode 403 (IsForbidden), not merely discouraged.
func (c *Client) ApproveProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string, in teapublisher.ApprovalDecision) (teapublisher.CollectionDraft, error) {
	return c.decideCollectionDraft(ctx, "productReleases", productReleaseUUID, "approve", in)
}

// ApproveComponentReleaseCollectionDraft is
// ApproveProductReleaseCollectionDraft for a component release.
func (c *Client) ApproveComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string, in teapublisher.ApprovalDecision) (teapublisher.CollectionDraft, error) {
	return c.decideCollectionDraft(ctx, "componentReleases", componentReleaseUUID, "approve", in)
}

// RejectProductReleaseCollectionDraft is
// ApproveProductReleaseCollectionDraft's symmetric rejection.
func (c *Client) RejectProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string, in teapublisher.ApprovalDecision) (teapublisher.CollectionDraft, error) {
	return c.decideCollectionDraft(ctx, "productReleases", productReleaseUUID, "reject", in)
}

// RejectComponentReleaseCollectionDraft is
// RejectProductReleaseCollectionDraft for a component release.
func (c *Client) RejectComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string, in teapublisher.ApprovalDecision) (teapublisher.CollectionDraft, error) {
	return c.decideCollectionDraft(ctx, "componentReleases", componentReleaseUUID, "reject", in)
}

// PrepareProductReleaseCollectionCommit is Collection Signing part 1
// (design/publisher-service.md §7.8): returns the would-be collection and
// the digest to sign over it, and locks the draft against further Put
// calls until Commit succeeds or CancelPrepare releases it. Returns an
// *APIError with StatusCode 409 (IsConflict) if there's no current,
// unexpired approval on record.
func (c *Client) PrepareProductReleaseCollectionCommit(ctx context.Context, productReleaseUUID string) (teapublisher.PrepareCommitResponse, error) {
	return c.prepareCollectionCommit(ctx, "productReleases", productReleaseUUID)
}

// PrepareComponentReleaseCollectionCommit is
// PrepareProductReleaseCollectionCommit for a component release.
func (c *Client) PrepareComponentReleaseCollectionCommit(ctx context.Context, componentReleaseUUID string) (teapublisher.PrepareCommitResponse, error) {
	return c.prepareCollectionCommit(ctx, "componentReleases", componentReleaseUUID)
}

// CancelPrepareProductReleaseCollectionCommit releases the lock
// PrepareProductReleaseCollectionCommit placed, without deleting the
// draft. Returns an *APIError with StatusCode 404 (IsNotFound) if no
// draft exists, or no lock is currently held.
func (c *Client) CancelPrepareProductReleaseCollectionCommit(ctx context.Context, productReleaseUUID string) error {
	return c.cancelPrepareCollectionCommit(ctx, "productReleases", productReleaseUUID)
}

// CancelPrepareComponentReleaseCollectionCommit is
// CancelPrepareProductReleaseCollectionCommit for a component release.
func (c *Client) CancelPrepareComponentReleaseCollectionCommit(ctx context.Context, componentReleaseUUID string) error {
	return c.cancelPrepareCollectionCommit(ctx, "componentReleases", componentReleaseUUID)
}

// CommitProductReleaseCollectionDraft is Collection Signing part 2 +
// Commit, atomic (design/publisher-service.md §7.8, §7.10): submit the
// evidence package produced by signing PrepareProductReleaseCollectionCommit's
// digest. The target re-derives that digest itself (defense against a
// stale/mismatched prepare response) and rejects (400, IsBadRequest)
// anything that fails verification -- nothing is published on failure.
// Returns the real, now-live collection on success.
func (c *Client) CommitProductReleaseCollectionDraft(ctx context.Context, productReleaseUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error) {
	return c.commitCollectionDraft(ctx, "productReleases", productReleaseUUID, in)
}

// CommitComponentReleaseCollectionDraft is
// CommitProductReleaseCollectionDraft for a component release.
func (c *Client) CommitComponentReleaseCollectionDraft(ctx context.Context, componentReleaseUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error) {
	return c.commitCollectionDraft(ctx, "componentReleases", componentReleaseUUID, in)
}
