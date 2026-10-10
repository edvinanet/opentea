// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"bytes"
	"context"
	"crypto/md5"  //nolint:gosec // MD5 is a spec-defined checksum type (tea.ChecksumTypeMD5) a conformant client must be able to verify against, not a security choice
	"crypto/sha1" //nolint:gosec // SHA-1 is a spec-defined checksum type (tea.ChecksumTypeSHA1) a conformant client must be able to verify against, not a security choice
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/crypto/blake2b"

	"github.com/oej/opentea/pkg/tea"
)

// GetLatestArtifact fetches the newest revision of artifact uuid (GET /artifact/{uuid}/latest).
func (c *Client) GetLatestArtifact(ctx context.Context, uuid string) (tea.Artifact, error) {
	var a tea.Artifact
	err := c.do(ctx, "GET", "/artifact/"+uuid+"/latest", nil, &a)
	return a, err
}

// GetArtifactByVersion fetches one specific revision of artifact uuid
// (GET /artifact/{uuid}/{version}).
func (c *Client) GetArtifactByVersion(ctx context.Context, uuid string, version int) (tea.Artifact, error) {
	var a tea.Artifact
	err := c.do(ctx, "GET", "/artifact/"+uuid+"/"+strconv.Itoa(version), nil, &a)
	return a, err
}

// maxDownloadAndVerifyBody bounds DownloadAndVerify's in-memory buffer --
// reuses client.go's existing maxResponseBody (10 MiB) rather than
// inventing a new limit. DownloadAndVerifyTo itself stays unbounded (see
// its own doc comment): only this convenience method holds the whole
// response in memory, so only it needs a cap.
const maxDownloadAndVerifyBody = maxResponseBody

// DownloadAndVerify fetches artifactUUID/version/format's content, verifies
// it against every checksum format declares, and returns the full
// downloaded bytes, capped at maxDownloadAndVerifyBody. It's a thin
// wrapper around DownloadAndVerifyTo for callers that want the content in
// memory; callers that only need verification (or want to stream to disk)
// should call DownloadAndVerifyTo directly with io.Discard (or a file) as
// dst instead, which never buffers the download at all and isn't subject
// to this limit -- for artifacts that may exceed it, prefer that instead
// of this convenience method.
func (c *Client) DownloadAndVerify(ctx context.Context, artifactUUID string, version int, format tea.ArtifactFormat) ([]byte, error) {
	var buf bytes.Buffer
	lw := &limitWriter{w: &buf, remaining: maxDownloadAndVerifyBody}
	if err := c.DownloadAndVerifyTo(ctx, artifactUUID, version, format, lw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// limitWriter wraps an io.Writer, erroring once more than remaining bytes
// have been written -- bounds DownloadAndVerify's buffer without changing
// DownloadAndVerifyTo's own unbounded-streaming contract (which io.Discard
// callers specifically rely on). DownloadAndVerifyTo's io.MultiWriter stops
// at the first writer to error within a single Write call, so this
// propagates cleanly through io.Copy as soon as the limit is exceeded; a
// partial hash update on the final over-limit chunk is harmless since the
// whole call errors out and no checksum is ever claimed as verified.
type limitWriter struct {
	w         io.Writer
	remaining int64
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > lw.remaining {
		return 0, fmt.Errorf("teaclient: response exceeds %d byte limit", maxDownloadAndVerifyBody)
	}
	n, err := lw.w.Write(p)
	lw.remaining -= int64(n)
	return n, err
}

// DownloadAndVerifyTo streams a format's content into dst while checking it
// against every checksum format declares -- unlike DownloadAndVerify, the
// download is never buffered in memory (pass io.Discard as dst to verify
// without keeping the content at all, bounding a CLI or embedding
// application's memory use regardless of how large a malicious or
// malfunctioning remote server's response is). artifactUUID/version
// identify the artifact format belongs to -- TEA 1.0 (spec/openapi.yaml)
// reserves format.URL for genuinely external locations; when it's empty,
// content is self-hosted and retrieved from the TEA server's own
// /artifact/{uuid}/{version}/download endpoint instead, selecting this
// format by its mediaType (format itself carries no back-reference to its
// owning artifact, hence these two extra parameters). External URLs are
// fetched without this client's bearer token, matching the spec's own
// rule ("A TEA access token is sent only to the TEA server's own API base
// URL"); the self-hosted request does carry it, same as every other call
// this client makes. Returns an error naming the first unsupported
// algorithm or mismatch found (checked in format.Checksums order, once the
// whole body has been read and hashed), nil if all declared checksums
// match, or if none are declared -- an artifact-format with no checksums
// is spec-valid, just unverifiable. BLAKE3 is not yet supported (no
// stdlib or golang.org/x/crypto implementation without adding a new
// dependency); an unsupported algorithm is caught before the download
// starts, not after.
func (c *Client) DownloadAndVerifyTo(ctx context.Context, artifactUUID string, version int, format tea.ArtifactFormat, dst io.Writer) error {
	return c.downloadContentAndVerifyTo(ctx, "/artifact/"+artifactUUID+"/"+strconv.Itoa(version)+"/download", format, dst)
}

// DownloadLatestAndVerifyTo is DownloadAndVerifyTo's counterpart for the
// latest-revision download endpoint (GET /artifact/{uuid}/latest/download)
// rather than one already-known version -- for a caller that only wants
// whatever revision is newest right now, not a specific one. Same
// self-hosted/external resolution and checksum-verification contract as
// DownloadAndVerifyTo; see its own doc comment for both.
func (c *Client) DownloadLatestAndVerifyTo(ctx context.Context, artifactUUID string, format tea.ArtifactFormat, dst io.Writer) error {
	return c.downloadContentAndVerifyTo(ctx, "/artifact/"+artifactUUID+"/latest/download", format, dst)
}

// downloadContentAndVerifyTo is DownloadAndVerifyTo/DownloadLatestAndVerifyTo's
// shared implementation, parameterized over the self-hosted download path
// (versioned or "latest") the two callers differ on.
func (c *Client) downloadContentAndVerifyTo(ctx context.Context, selfHostedPath string, format tea.ArtifactFormat, dst io.Writer) error {
	hashers, err := newChecksumHashers(format.Checksums)
	if err != nil {
		return err
	}

	var req *http.Request
	if format.URL != "" {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, format.URL, nil)
		if err != nil {
			return err
		}
	} else {
		u := c.baseURL + selfHostedPath + "?mediaType=" + url.QueryEscape(format.MediaType)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		if c.bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.bearerToken)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return &APIError{StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}
	}

	writers := make([]io.Writer, 0, len(hashers)+1)
	for _, h := range hashers {
		writers = append(writers, h.hash)
	}
	writers = append(writers, dst)
	if _, err := io.Copy(io.MultiWriter(writers...), resp.Body); err != nil {
		return err
	}

	for _, h := range hashers {
		got := hex.EncodeToString(h.hash.Sum(nil))
		if !strings.EqualFold(got, h.cksum.AlgValue) {
			return fmt.Errorf("teaclient: %s checksum mismatch: got %s, want %s", h.cksum.AlgType, got, h.cksum.AlgValue)
		}
	}
	return nil
}

// DownloadSignature fetches the detached signature for one format of a
// specific artifact revision (GET /artifact/{uuid}/{version}/signature/download,
// or the format's own SignatureURL if it names an external location).
// Unlike DownloadAndVerify/DownloadAndVerifyTo, there is no checksum to
// verify against: the specification "makes no assumption about the
// signature technology in use" and defines no checksum field for a
// signature itself -- this returns the signature exactly as published, for
// the caller to verify however it knows how to. A 404 with TEAErrorCode
// "SIGNATURE_NOT_FOUND" means the revision exists but this format has no
// published signature; "OBJECT_UNKNOWN" means the revision itself is
// unknown (or concealed) -- both surface as a plain *APIError, distinguish
// via TEAErrorCode.
func (c *Client) DownloadSignature(ctx context.Context, artifactUUID string, version int, format tea.ArtifactFormat) ([]byte, error) {
	return c.downloadSignature(ctx, "/artifact/"+artifactUUID+"/"+strconv.Itoa(version)+"/signature/download", format)
}

// DownloadLatestSignature is DownloadSignature's counterpart for the
// latest-revision signature endpoint (GET /artifact/{uuid}/latest/signature/download).
func (c *Client) DownloadLatestSignature(ctx context.Context, artifactUUID string, format tea.ArtifactFormat) ([]byte, error) {
	return c.downloadSignature(ctx, "/artifact/"+artifactUUID+"/latest/signature/download", format)
}

// downloadSignature is DownloadSignature/DownloadLatestSignature's shared
// implementation, bounded by maxResponseBody like every other in-memory
// response this package reads.
func (c *Client) downloadSignature(ctx context.Context, selfHostedPath string, format tea.ArtifactFormat) ([]byte, error) {
	var req *http.Request
	var err error
	if format.SignatureURL != "" {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, format.SignatureURL, nil)
	} else {
		u := c.baseURL + selfHostedPath + "?mediaType=" + url.QueryEscape(format.MediaType)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err == nil && c.bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.bearerToken)
		}
	}
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxResponseBody {
		return nil, fmt.Errorf("teaclient: signature download for %s exceeds %d byte limit", selfHostedPath, maxResponseBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}
	}
	return body, nil
}

// checksumHasher pairs a declared checksum with the running hash.Hash that
// will verify it, so DownloadAndVerifyTo can write the download through all
// of them at once via io.MultiWriter and check each afterward.
type checksumHasher struct {
	cksum tea.Checksum
	hash  hash.Hash
}

// newChecksumHashers builds one checksumHasher per checksum, failing fast
// (before any download starts) if any algorithm isn't supported.
func newChecksumHashers(checksums []tea.Checksum) ([]checksumHasher, error) {
	out := make([]checksumHasher, 0, len(checksums))
	for _, cksum := range checksums {
		h, err := newHasher(cksum.AlgType)
		if err != nil {
			return nil, err
		}
		out = append(out, checksumHasher{cksum: cksum, hash: h})
	}
	return out, nil
}

func newHasher(algType string) (hash.Hash, error) {
	switch algType {
	case tea.ChecksumTypeMD5:
		return md5.New(), nil //nolint:gosec // verifying a spec-defined checksum type, not using MD5 for security
	case tea.ChecksumTypeSHA1:
		return sha1.New(), nil //nolint:gosec // verifying a spec-defined checksum type, not using SHA-1 for security
	case tea.ChecksumTypeSHA256:
		return sha256.New(), nil
	case tea.ChecksumTypeSHA384:
		return sha512.New384(), nil
	case tea.ChecksumTypeSHA512:
		return sha512.New(), nil
	case tea.ChecksumTypeSHA3_256:
		return sha3.New256(), nil
	case tea.ChecksumTypeSHA3_384:
		return sha3.New384(), nil
	case tea.ChecksumTypeSHA3_512:
		return sha3.New512(), nil
	case tea.ChecksumTypeBLAKE2b256:
		return blake2b.New256(nil)
	case tea.ChecksumTypeBLAKE2b384:
		return blake2b.New384(nil)
	case tea.ChecksumTypeBLAKE2b512:
		return blake2b.New512(nil)
	default:
		return nil, fmt.Errorf("teaclient: unsupported checksum algorithm %q (cannot verify)", algType)
	}
}
