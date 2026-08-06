package webadmin

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestLimitBodyRejectsOversizedRequest is the regression test for
// maxWebadminBody: every /admin/ui POST used to accept a request body of
// any size. Temporarily lowers maxWebadminBody well below a normal login
// form's size, rather than generating a real 64 KiB body, since the limit
// itself is what's under test.
func TestLimitBodyRejectsOversizedRequest(t *testing.T) {
	srv, _ := newTestServer(t)

	original := maxWebadminBody
	maxWebadminBody = 10
	t.Cleanup(func() { maxWebadminBody = original })

	form := url.Values{"username": {"admin"}, "password": {"whatever"}}.Encode()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/admin/ui/login", strings.NewReader(form))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.test") // matches newTestServer's fixed cfg.RootURL

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /admin/ui/login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Fatalf("status = %d, want a 4xx for a body exceeding maxWebadminBody", resp.StatusCode)
	}
}

// TestLimitBodyEnforcesReadDeadline is the regression test for
// webadminBodyReadTimeout: an active but stalled request body (not idle,
// so IdleTimeout doesn't apply, and the top-level http.Server's ReadTimeout
// is deliberately zero for the unrelated /admin/v1 large-upload routes) used
// to be able to hold a connection open indefinitely. Lowers the deadline to
// a few milliseconds, opens a raw connection, sends headers declaring a
// body larger than what's actually written, and confirms the server
// responds (or closes the connection) well within a generous client-side
// deadline instead of hanging until that deadline fires -- if the fix
// regressed, this test would itself hang (surfaced as the client-side
// deadline below being exceeded), not silently pass.
func TestLimitBodyEnforcesReadDeadline(t *testing.T) {
	srv, _ := newTestServer(t)

	original := webadminBodyReadTimeout
	webadminBodyReadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { webadminBodyReadTimeout = original })

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Declares a 1000-byte body but only ever sends a few bytes of it --
	// a stalled request body, the scenario webadminBodyReadTimeout guards.
	request := "POST /admin/ui/login HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Origin: http://example.test\r\n" +
		"Content-Type: application/x-www-form-urlencoded\r\n" +
		"Content-Length: 1000\r\n" +
		"\r\n" +
		"username=a"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Generous relative to the 50ms server-side deadline above -- proves
	// the server doesn't hang, without asserting the exact response shape.
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 512)
	_, err = conn.Read(buf)
	if err != nil && !isTimeout(err) && err != io.EOF {
		// Any other error (e.g. connection reset) also proves the server
		// didn't hang -- only a client-side read timeout is a failure here.
		return
	}
	if isTimeout(err) {
		t.Fatal("conn.Read timed out waiting for the server -- it hung instead of enforcing webadminBodyReadTimeout")
	}
}

func isTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}
