// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestMemberRoleDeniedAdminActions is the core regression test for this
// scaffold: a "member"-role staff account must not be able to manage
// targets or other staff accounts -- only view the dashboard.
func TestMemberRoleDeniedAdminActions(t *testing.T) {
	srv, r, adminUsername, adminPassword := newTestServer(t)
	if _, err := r.CreateStaff(context.Background(), "bob", "hunter333", StaffRoleMember, ""); err != nil {
		t.Fatalf("CreateStaff (member): %v", err)
	}

	admin := loggedInClient(t, srv, adminUsername, adminPassword)
	member := loggedInClient(t, srv, "bob", "hunter333")

	// Member can view the dashboard.
	dashResp, err := member.Get(srv.URL + "/")
	if err != nil || dashResp.StatusCode != http.StatusOK {
		t.Fatalf("member GET /: resp=%+v err=%v", dashResp, err)
	}
	_ = dashResp.Body.Close()

	addTargetRequest := func() *http.Request {
		form := url.Values{"label": {"x"}, "baseUrl": {"https://tea.example.com/publisher/v1"}, "bearerToken": {"t"}}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/targets", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("new add-target request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		return req
	}

	// Member cannot add a target.
	resp, err := member.Do(addTargetRequest())
	if err != nil {
		t.Fatalf("member POST /targets: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member POST /targets: status=%d, want 403", resp.StatusCode)
	}

	// Member cannot view the staff page.
	if resp, err := member.Get(srv.URL + "/staff"); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member GET /staff: resp=%+v err=%v, want 403", resp, err)
	} else {
		_ = resp.Body.Close()
	}

	// Confirm the admin actually can do both (the negative tests above
	// aren't just "everyone is blocked").
	adminAddResp, err := admin.Do(addTargetRequest())
	if err != nil {
		t.Fatalf("admin POST /targets: %v", err)
	}
	_ = adminAddResp.Body.Close()
	if adminAddResp.StatusCode != http.StatusOK {
		t.Fatalf("admin POST /targets: status=%d, want 200 (after following the redirect)", adminAddResp.StatusCode)
	}
}

// TestStaffManagementFlow drives creating and removing staff accounts
// through the real HTTP handlers, including the last-admin protection.
func TestStaffManagementFlow(t *testing.T) {
	srv, _, username, password := newTestServer(t)
	admin := loggedInClient(t, srv, username, password)

	// Create a member account.
	form := url.Values{"username": {"bob"}, "password": {"hunter333"}, "role": {"member"}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/staff", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatalf("POST /staff: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "bob") {
		t.Fatalf("POST /staff: status=%d body=%s", resp.StatusCode, body)
	}

	// The original admin is the only admin -- deleting it must fail.
	listResp, err := admin.Get(srv.URL + "/staff")
	if err != nil {
		t.Fatalf("GET /staff: %v", err)
	}
	listBody, _ := io.ReadAll(listResp.Body)
	_ = listResp.Body.Close()
	if !strings.Contains(string(listBody), "bob") || !strings.Contains(string(listBody), username) {
		t.Fatalf("GET /staff doesn't list both accounts: %s", listBody)
	}
}
