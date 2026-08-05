package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/oej/opentea/internal/authn"
)

// Runner replays a Fixture's steps against a running opentea server,
// authenticated the same way a human using the GUI would be (a session
// cookie from POST /admin/ui/login).
type Runner struct {
	baseURL    string
	client     *http.Client
	cookie     *http.Cookie
	vars       map[string]any
	fixtureDir string // for resolving relative UPLOAD "file" paths
}

// Login authenticates against baseURL/admin/ui/login and returns a Runner
// ready to replay steps.
func Login(baseURL, username, password string) (*Runner, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.PostForm(baseURL+"/admin/ui/login", url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login failed: status %d: %s", resp.StatusCode, body)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == authn.SessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		return nil, fmt.Errorf("login succeeded (status %d) but no session cookie was set", resp.StatusCode)
	}

	return &Runner{baseURL: strings.TrimRight(baseURL, "/"), client: client, cookie: cookie, vars: map[string]any{}}, nil
}

// Run replays every step in order, stopping at the first failure.
func (r *Runner) Run(fixture Fixture) error {
	for i, step := range fixture.Steps {
		if err := r.runStep(step); err != nil {
			return fmt.Errorf("step %d (%s %s): %w", i, step.Op, step.Path, err)
		}
		fmt.Printf("step %d: %s %s OK\n", i, step.Op, step.Path)
	}
	return nil
}

func (r *Runner) runStep(step Step) error {
	path, err := substituteString(step.Path, r.vars)
	if err != nil {
		return err
	}

	switch step.Op {
	case "POST", "DELETE":
		return r.runJSONStep(step, path)
	case "UPLOAD":
		return r.runUploadStep(step, path)
	default:
		return fmt.Errorf("unknown op %q (expected POST, DELETE, or UPLOAD)", step.Op)
	}
}

func (r *Runner) runJSONStep(step Step, path string) error {
	var reader io.Reader
	hasBody := len(step.Body) > 0
	if hasBody {
		var bodyAny any
		if err := json.Unmarshal(step.Body, &bodyAny); err != nil {
			return fmt.Errorf("parse body: %w", err)
		}
		bodyAny, err := substituteJSON(bodyAny, r.vars)
		if err != nil {
			return err
		}
		b, err := json.Marshal(bodyAny)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(step.Op, r.baseURL+path, reader)
	if err != nil {
		return err
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	// Matches r.baseURL, the same origin the server issued r.cookie for --
	// required by requireRole's CSRF Origin check on state-changing requests.
	req.Header.Set("Origin", r.baseURL)
	req.AddCookie(r.cookie)

	resp, err := r.client.Do(req) //nolint:bodyclose // finish() (below) closes resp.Body via defer; the linter can't trace the close through that separate call
	if err != nil {
		return err
	}
	return r.finish(resp, step.Save)
}

func (r *Runner) runUploadStep(step Step, path string) error {
	filePath := step.File
	if r.fixtureDir != "" && !filepath.IsAbs(filePath) {
		filePath = filepath.Join(r.fixtureDir, filePath)
	}
	content, err := os.ReadFile(filePath) //nolint:gosec // filePath is derived from the operator's own local fixture JSON file, not remote input
	if err != nil {
		return fmt.Errorf("read upload file: %w", err)
	}

	fieldName := step.FieldName
	if fieldName == "" {
		fieldName = "file"
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filepath.Base(filePath))},
		"Content-Type":        {step.ContentType},
	})
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, r.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", r.baseURL)
	req.AddCookie(r.cookie)

	resp, err := r.client.Do(req) //nolint:bodyclose // finish() (below) closes resp.Body via defer; the linter can't trace the close through that separate call
	if err != nil {
		return err
	}
	return r.finish(resp, step.Save)
}

// finish checks the response status, and if step.Save is set, decodes the
// JSON body into the variable pool under that name.
func (r *Runner) finish(resp *http.Response, save string) error {
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	if save == "" || len(body) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode response to save as %q: %w", save, err)
	}
	r.vars[save] = decoded
	return nil
}
