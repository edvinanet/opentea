// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Fixture is a small, generic "authenticated HTTP replay" format for loading
// structured test/demo data into a running opentea server via the existing
// /admin/v1 API. It is not the (deferred, spec-driven) reference publisher --
// just a reusable loader.
type Fixture struct {
	Steps []Step `json:"steps"`
}

// Step is either a JSON request (POST/DELETE) or a file upload (UPLOAD).
// Path and any string values inside Body may contain "{{name.field}}"
// placeholders resolved against previously-Saved response data.
type Step struct {
	Op   string          `json:"op"` // "POST" | "DELETE" | "UPLOAD"
	Path string          `json:"path"`
	Body json.RawMessage `json:"body,omitempty"`

	// UPLOAD only:
	File        string `json:"file,omitempty"`      // path relative to the fixture file
	FieldName   string `json:"fieldName,omitempty"` // multipart field name, default "file"
	ContentType string `json:"contentType,omitempty"`

	// If set, the decoded JSON response is stored under this name for
	// later steps' placeholders to reference.
	Save string `json:"save,omitempty"`
}

func loadFixtureFile(path string) (Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is this CLI's own command-line argument, operator-controlled, not remote input
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture file: %w", err)
	}
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return Fixture{}, fmt.Errorf("parse fixture file: %w", err)
	}
	return fixture, nil
}
