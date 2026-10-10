// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"
	"net/http"

	"github.com/oej/opentea/pkg/teaclient"
)

// Options configures Run.
type Options struct {
	// BearerToken, if non-empty, is attached to every request this run
	// makes -- for a server that requires authentication. Exchanging
	// credentials for one (POST /token) is outside Run's own scope: that
	// requires deployment-specific API key credentials this generic suite
	// has no way to obtain on its own, so /token shows up as never-called
	// in the coverage report unless the caller already holds a token.
	BearerToken string
}

// Report is Run's complete result: the two independent scores
// docs/consumer-api-conformance-test-rig.md describes, neither implying
// the other.
type Report struct {
	Coverage    CoverageReport
	Correctness CorrectnessReport
}

// Run checks every reference dataset's data-correctness assertions
// (CheckSimple, CheckComplex, CheckLifecycle) against the server at
// baseURL (its TEA API root, e.g. "https://example.com/tea/v1" -- the same
// string passed to teaclient.NewClient), while recording endpoint+method
// coverage across the whole run. The datasets must already be loaded on
// that server -- how they got there is outside this package's concern;
// see docs/bundle-import-export-test-rig.md for the admin-side mechanism
// this repo's own server supports.
func Run(ctx context.Context, baseURL string, opts Options) (*Report, error) {
	tracker, err := NewCoverageTracker(baseURL)
	if err != nil {
		return nil, err
	}

	hc := &http.Client{Transport: tracker.Transport(nil)}
	var clientOpts []teaclient.Option
	clientOpts = append(clientOpts, teaclient.WithHTTPClient(hc))
	if opts.BearerToken != "" {
		clientOpts = append(clientOpts, teaclient.WithBearerToken(opts.BearerToken))
	}
	client := teaclient.NewClient(baseURL, clientOpts...)

	cor := NewCorrectnessTracker()
	CheckSimple(ctx, client, cor)
	CheckComplex(ctx, client, cor)
	CheckLifecycle(ctx, client, cor)

	return &Report{Coverage: tracker.Report(), Correctness: cor.Report()}, nil
}
