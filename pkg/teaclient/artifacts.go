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
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
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

// DownloadAndVerify fetches format.URL, verifies it against every checksum
// format declares, and returns the full downloaded bytes. It's a thin
// wrapper around DownloadAndVerifyTo for callers that want the content in
// memory; callers that only need verification (or want to stream to disk)
// should call DownloadAndVerifyTo directly with io.Discard (or a file) as
// dst instead, which never buffers the download at all -- this wrapper's
// bytes.Buffer is itself unbounded against a malicious or oversized
// response, same as the pre-streaming implementation.
func (c *Client) DownloadAndVerify(ctx context.Context, format tea.ArtifactFormat) ([]byte, error) {
	var buf bytes.Buffer
	err := c.DownloadAndVerifyTo(ctx, format, &buf)
	return buf.Bytes(), err
}

// DownloadAndVerifyTo streams format.URL's content into dst while checking
// it against every checksum format declares -- unlike DownloadAndVerify,
// the download is never buffered in memory (pass io.Discard as dst to
// verify without keeping the content at all, bounding a CLI or embedding
// application's memory use regardless of how large a malicious or
// malfunctioning remote server's response is). Returns an error naming the
// first unsupported algorithm or mismatch found (checked in format.Checksums
// order, once the whole body has been read and hashed), nil if all declared
// checksums match, or if none are declared -- an artifact-format with no
// checksums is spec-valid, just unverifiable. BLAKE3 is not yet supported
// (no stdlib or golang.org/x/crypto implementation without adding a new
// dependency); an unsupported algorithm is caught before the download
// starts, not after.
func (c *Client) DownloadAndVerifyTo(ctx context.Context, format tea.ArtifactFormat, dst io.Writer) error {
	if format.URL == "" {
		return errors.New("teaclient: artifact format has no URL")
	}
	hashers, err := newChecksumHashers(format.Checksums)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, format.URL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return &APIError{StatusCode: resp.StatusCode, Body: body}
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
