package teaclient

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetLatestArtifact(ctx context.Context, uuid string) (tea.Artifact, error) {
	var a tea.Artifact
	err := c.do(ctx, "GET", "/artifact/"+uuid+"/latest", nil, &a)
	return a, err
}

func (c *Client) GetArtifactByVersion(ctx context.Context, uuid string, version int) (tea.Artifact, error) {
	var a tea.Artifact
	err := c.do(ctx, "GET", "/artifact/"+uuid+"/"+strconv.Itoa(version), nil, &a)
	return a, err
}

// DownloadAndVerify fetches format.URL and checks the downloaded bytes
// against every checksum format declares, returning the bytes plus an error
// naming the first algorithm/mismatch found (nil error if all declared
// checksums match, or if none are declared -- an artifact-format with no
// checksums is spec-valid, just unverifiable). BLAKE3 is not yet supported
// (no stdlib or golang.org/x/crypto implementation without adding a new
// dependency) and is reported as an explicit "unsupported" error rather than
// silently skipped.
func (c *Client) DownloadAndVerify(ctx context.Context, format tea.ArtifactFormat) ([]byte, error) {
	if format.URL == "" {
		return nil, errors.New("teaclient: artifact format has no URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, format.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: data}
	}

	for _, cksum := range format.Checksums {
		if err := verifyChecksum(data, cksum); err != nil {
			return data, err
		}
	}
	return data, nil
}

func verifyChecksum(data []byte, cksum tea.Checksum) error {
	var sum []byte
	switch cksum.AlgType {
	case tea.ChecksumTypeMD5:
		h := md5.Sum(data)
		sum = h[:]
	case tea.ChecksumTypeSHA1:
		h := sha1.Sum(data)
		sum = h[:]
	case tea.ChecksumTypeSHA256:
		h := sha256.Sum256(data)
		sum = h[:]
	case tea.ChecksumTypeSHA384:
		h := sha512.Sum384(data)
		sum = h[:]
	case tea.ChecksumTypeSHA512:
		h := sha512.Sum512(data)
		sum = h[:]
	case tea.ChecksumTypeSHA3_256:
		h := sha3.Sum256(data)
		sum = h[:]
	case tea.ChecksumTypeSHA3_384:
		h := sha3.Sum384(data)
		sum = h[:]
	case tea.ChecksumTypeSHA3_512:
		h := sha3.Sum512(data)
		sum = h[:]
	case tea.ChecksumTypeBLAKE2b256:
		h := blake2b.Sum256(data)
		sum = h[:]
	case tea.ChecksumTypeBLAKE2b384:
		h := blake2b.Sum384(data)
		sum = h[:]
	case tea.ChecksumTypeBLAKE2b512:
		h := blake2b.Sum512(data)
		sum = h[:]
	default:
		return fmt.Errorf("teaclient: unsupported checksum algorithm %q (cannot verify)", cksum.AlgType)
	}
	got := hex.EncodeToString(sum)
	if !strings.EqualFold(got, cksum.AlgValue) {
		return fmt.Errorf("teaclient: %s checksum mismatch: got %s, want %s", cksum.AlgType, got, cksum.AlgValue)
	}
	return nil
}
