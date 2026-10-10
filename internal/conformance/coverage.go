// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// CoverageTracker records which of Operations a conformance run actually
// exercised, by wrapping the pkg/teaclient.Client's underlying
// http.RoundTripper -- every request the client issues (through any of
// its methods, including the raw http.Client.Do calls DownloadAndVerifyTo/
// DownloadSignature make) passes through it, so no call site needs its
// own bookkeeping.
type CoverageTracker struct {
	basePath string // the server's API mount path, e.g. "/tea/v1" -- stripped before matching

	mu        sync.Mutex
	exercised map[string]bool
}

// NewCoverageTracker builds a tracker for a server whose TEA API root is
// baseURL (the same string passed to teaclient.NewClient) -- used to
// strip the server's own mount path (e.g. "/tea/v1") from each request's
// URL before matching it against Operations' path templates, which are
// written relative to that root, matching pkg/teaclient's own source.
func NewCoverageTracker(baseURL string) (*CoverageTracker, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &CoverageTracker{
		basePath:  strings.TrimRight(u.Path, "/"),
		exercised: map[string]bool{},
	}, nil
}

// Transport wraps next (nil means http.DefaultTransport) with this
// tracker's recording RoundTripper -- pass the result to
// teaclient.WithHTTPClient's *http.Client.Transport.
func (t *CoverageTracker) Transport(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &coverageRoundTripper{tracker: t, next: next}
}

type coverageRoundTripper struct {
	tracker *CoverageTracker
	next    http.RoundTripper
}

func (rt *coverageRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.tracker.record(req.Method, req.URL.Path)
	return rt.next.RoundTrip(req)
}

// record marks the Operation matching method/fullPath as exercised, if
// fullPath (after stripping this tracker's basePath) matches any
// registered Operation -- a request to an external url (an artifact
// format's own url/signatureUrl, outside the server's own API entirely)
// or any other unrecognized path is silently not tracked, same as it
// wouldn't be part of this server's own API surface either.
func (t *CoverageTracker) record(method, fullPath string) {
	rel := strings.TrimPrefix(fullPath, t.basePath)
	op, ok := Lookup(method, rel)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exercised[op.Key()] = true
}

// CoverageReport is NewCoverageTracker's Transport's accumulated result.
type CoverageReport struct {
	Exercised   int
	Total       int
	NeverCalled []string // "METHOD /path/template" for every Operation never exercised, sorted as Operations itself is ordered
}

// Percent is Exercised/Total as a percentage, or 0 if Total is 0.
func (r CoverageReport) Percent() float64 {
	if r.Total == 0 {
		return 0
	}
	return 100 * float64(r.Exercised) / float64(r.Total)
}

// Report summarizes what this tracker has recorded so far.
func (t *CoverageTracker) Report() CoverageReport {
	t.mu.Lock()
	defer t.mu.Unlock()

	report := CoverageReport{Total: len(Operations)}
	for _, op := range Operations {
		if t.exercised[op.Key()] {
			report.Exercised++
		} else {
			report.NeverCalled = append(report.NeverCalled, op.Key())
		}
	}
	return report
}
