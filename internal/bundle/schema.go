// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema.json
var schemaJSON []byte

// schemaURL is an opaque identifier for the embedded schema resource -- not
// fetched over the network, just a name the compiler uses internally.
const schemaURL = "opentea://bundle/schema.json"

var (
	compileOnce sync.Once
	compiled    *jsonschema.Schema
	compileErr  error
)

func compiledSchema() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		compiled, compileErr = jsonschema.CompileString(schemaURL, string(schemaJSON))
	})
	return compiled, compileErr
}

// ValidateManifest checks raw manifest.json bytes against the bundle JSON
// Schema (internal/bundle/schema.json), returning every violation found
// rather than just the first one.
func ValidateManifest(raw []byte) error {
	s, err := compiledSchema()
	if err != nil {
		return fmt.Errorf("compile bundle schema: %w", err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("manifest.json is not valid JSON: %w", err)
	}

	if err := s.Validate(doc); err != nil {
		return fmt.Errorf("manifest.json failed schema validation: %w", err)
	}
	return nil
}
