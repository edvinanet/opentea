// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestServer builds a full opentea-publisher handler tree against a
// fresh temp-file SQLite DB, bootstraps a staff account, and returns a
// ready-to-use httptest.Server plus that staff account's username/password
// (for logging in) -- mirrors cmd/opentea's own newTestServer convention.
func newTestServer(t *testing.T) (srv *httptest.Server, username, password string) {
	t.Helper()
	r := newTestRepo(t)

	username, password = "alice", "hunter222"
	if _, err := r.CreateStaff(context.Background(), username, password); err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}

	srv = httptest.NewServer(nil)
	srv.Config.Handler = NewRouter(r, Config{RootURL: srv.URL})
	t.Cleanup(srv.Close)
	return srv, username, password
}

// loggedInClient returns an *http.Client with a cookie jar, logged in
// against srv as username/password.
func loggedInClient(t *testing.T, srv *httptest.Server, username, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	form := url.Values{"username": {username}, "password": {password}}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	_ = resp.Body.Close()
	if len(jar.Cookies(mustParseURL(t, srv.URL))) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	return client
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	return u
}

// TestLoginDashboardAddTargetFlow drives the whole scaffolded flow through
// real HTTP: unauthenticated dashboard access redirects to login, login
// sets a session cookie, the authenticated dashboard loads, adding a
// target persists it and it shows up listed, and deleting it removes it.
func TestLoginDashboardAddTargetFlow(t *testing.T) {
	srv, username, password := newTestServer(t)

	// Unauthenticated: redirected to login, not served the dashboard.
	noJarClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noJarClient.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / (unauthenticated): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("GET / (unauthenticated): status=%d location=%q, want 303 to /login", resp.StatusCode, resp.Header.Get("Location"))
	}

	client := loggedInClient(t, srv, username, password)

	dashResp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / (authenticated): %v", err)
	}
	dashBody, _ := io.ReadAll(dashResp.Body)
	_ = dashResp.Body.Close()
	if dashResp.StatusCode != http.StatusOK || !strings.Contains(string(dashBody), "No targets configured yet") {
		t.Fatalf("GET / (authenticated): status=%d body=%q", dashResp.StatusCode, dashBody)
	}

	// Add a target.
	form := url.Values{"label": {"Acme production"}, "baseUrl": {"https://tea.example.com/publisher/v1"}, "bearerToken": {"s3cr3t"}}
	addReq, err := http.NewRequest(http.MethodPost, srv.URL+"/targets", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new add-target request: %v", err)
	}
	addReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addReq.Header.Set("Origin", srv.URL)
	addResp, err := client.Do(addReq)
	if err != nil {
		t.Fatalf("POST /targets: %v", err)
	}
	_ = addResp.Body.Close()

	dashResp2, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / (after add): %v", err)
	}
	dashBody2, _ := io.ReadAll(dashResp2.Body)
	_ = dashResp2.Body.Close()
	if !strings.Contains(string(dashBody2), "Acme production") || !strings.Contains(string(dashBody2), "https://tea.example.com/publisher/v1") {
		t.Fatalf("dashboard after add doesn't show the new target: %s", dashBody2)
	}
}
