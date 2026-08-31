// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

const testToken = "test-bearer-token"

func newFakeServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, testToken)
}

func TestCreateProduct(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody teapublisher.ProductCreate
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tea.Product{UUID: "abc-123", Name: "Acme Widget"})
	})

	p, err := client.CreateProduct(context.Background(), teapublisher.ProductCreate{Name: "Acme Widget"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if gotAuth != "Bearer "+testToken {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/products" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody.Name != "Acme Widget" {
		t.Errorf("request body = %+v", gotBody)
	}
	if p.UUID != "abc-123" {
		t.Errorf("response = %+v", p)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		status int
		check  func(error) bool
		name   string
	}{
		{http.StatusNotFound, IsNotFound, "IsNotFound"},
		{http.StatusBadRequest, IsBadRequest, "IsBadRequest"},
		{http.StatusUnauthorized, IsUnauthorized, "IsUnauthorized"},
		{http.StatusForbidden, IsForbidden, "IsForbidden"},
		{http.StatusConflict, IsConflict, "IsConflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})
			_, err := client.CreateProduct(context.Background(), teapublisher.ProductCreate{Name: "x"})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !tt.check(err) {
				t.Fatalf("err = %v, want %s", err, tt.name)
			}
		})
	}
}

func TestPutProductReleaseCollectionDraft(t *testing.T) {
	var gotMethod, gotPath string
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewEncoder(w).Encode(teapublisher.CollectionDraft{OwnerUUID: "pr-1", Revision: 1})
	})

	draft, err := client.PutProductReleaseCollectionDraft(context.Background(), "pr-1", teapublisher.CollectionDraftArtifactList{
		Actor: "ci-pipeline",
		Artifacts: []teapublisher.ArtifactVersionRef{
			{UUID: "artifact-1", Version: 1},
		},
	})
	if err != nil {
		t.Fatalf("PutProductReleaseCollectionDraft: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/productReleases/pr-1/collectionDraft" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if draft.Revision != 1 {
		t.Errorf("draft = %+v", draft)
	}
}

func TestComponentReleaseCollectionDraftPaths(t *testing.T) {
	var gotPath string
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(teapublisher.CollectionDraft{})
	})

	if _, err := client.GetComponentReleaseCollectionDraft(context.Background(), "cr-1"); err != nil {
		t.Fatalf("GetComponentReleaseCollectionDraft: %v", err)
	}
	if gotPath != "/componentReleases/cr-1/collectionDraft" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestCommitProductReleaseCollectionDraft(t *testing.T) {
	var gotPath string
	var gotBody teapublisher.EvidenceSubmission
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tea.Collection{UUID: "pr-1", Version: 1})
	})

	collection, err := client.CommitProductReleaseCollectionDraft(context.Background(), "pr-1", teapublisher.EvidenceSubmission{
		ObjectDigestValue: "deadbeef",
		SignatureFormat:   "jws-detached",
		SignatureValue:    "c2ln",
		CertificatePEM:    "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
	})
	if err != nil {
		t.Fatalf("CommitProductReleaseCollectionDraft: %v", err)
	}
	if gotPath != "/productReleases/pr-1/collectionDraft/commit" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody.ObjectDigestValue != "deadbeef" {
		t.Errorf("request body = %+v", gotBody)
	}
	if collection.Version != 1 {
		t.Errorf("collection = %+v", collection)
	}
}

func TestUploadArtifactFile(t *testing.T) {
	var gotMethod, gotPath, gotMediaType, gotFileContent string
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotMediaType = r.FormValue("mediaType")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer func() { _ = file.Close() }()
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		gotFileContent = string(content)
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.UploadArtifactFile(context.Background(), "artifact-1", 1, "application/vnd.cyclonedx+json", "sbom.json", strings.NewReader(`{"ok":true}`))
	if err != nil {
		t.Fatalf("UploadArtifactFile: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/artifacts/artifact-1/1/files" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotMediaType != "application/vnd.cyclonedx+json" {
		t.Errorf("mediaType = %q", gotMediaType)
	}
	if gotFileContent != `{"ok":true}` {
		t.Errorf("file content = %q", gotFileContent)
	}
}

func TestUploadArtifactFileConflict(t *testing.T) {
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"already has evidence"}`))
	})

	err := client.UploadArtifactFile(context.Background(), "artifact-1", 1, "application/json", "f.json", strings.NewReader("x"))
	if !IsConflict(err) {
		t.Fatalf("err = %v, want IsConflict", err)
	}
}

func TestPrepareArtifactEvidence(t *testing.T) {
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifacts/artifact-1/2/evidence/prepare" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(teapublisher.PrepareArtifactEvidenceResponse{
			DigestToSign: "deadbeef",
			Artifact:     tea.Artifact{UUID: "artifact-1", Version: 2},
		})
	})

	resp, err := client.PrepareArtifactEvidence(context.Background(), "artifact-1", 2)
	if err != nil {
		t.Fatalf("PrepareArtifactEvidence: %v", err)
	}
	if resp.DigestToSign != "deadbeef" || resp.Artifact.Version != 2 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFindComponentsQueryEscaping(t *testing.T) {
	var gotRawQuery string
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]tea.Component{})
	})

	if _, err := client.FindComponents(context.Background(), "acme widget"); err != nil {
		t.Fatalf("FindComponents: %v", err)
	}
	if gotRawQuery != "q=acme+widget" {
		t.Errorf("raw query = %q", gotRawQuery)
	}
}

func TestDeleteAndCancelReturnNoBody(t *testing.T) {
	client := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := client.DeleteProductReleaseCollectionDraft(context.Background(), "pr-1"); err != nil {
		t.Fatalf("DeleteProductReleaseCollectionDraft: %v", err)
	}
	if err := client.CancelPrepareProductReleaseCollectionCommit(context.Background(), "pr-1"); err != nil {
		t.Fatalf("CancelPrepareProductReleaseCollectionCommit: %v", err)
	}
}
