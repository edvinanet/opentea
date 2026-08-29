// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command validate-sbom checks a CycloneDX SBOM two ways: against the real
// JSON Schema for the spec version it declares (not just "the JSON
// parses" -- see SKILL.md's "validation" section for why that distinction
// matters), and for referential integrity (every dependency graph edge
// points at a component or the root that actually exists, which schema
// validation alone doesn't catch).
//
// Reuses this project's own santhosh-tekuri/jsonschema/v5 dependency
// (already used by internal/bundle/schema.go) rather than adding a new one
// just for this -- same rule #2 in SKILL.md's own selection criteria.
//
// Usage: go run validate-sbom.go <sbom.cdx.json> [<sbom.cdx.json> ...]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run validate-sbom.go <sbom.cdx.json> [...]")
		os.Exit(2)
	}

	schema, err := loadSchema()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load schema: %v\n", err)
		os.Exit(1)
	}

	failed := false
	for _, path := range os.Args[1:] {
		if err := validateOne(schema, path); err != nil {
			fmt.Printf("FAIL %s: %v\n", path, err)
			failed = true
		} else {
			fmt.Printf("ok   %s\n", path)
		}
	}
	if failed {
		os.Exit(1)
	}
}

// loadSchema compiles bom-1.6.schema.json against the two schemas it
// itself $refs (spdx.schema.json, jsf-0.82.schema.json), all loaded from
// this skill's own assets/schemas/ -- no network fetch at validation time.
func loadSchema() (*jsonschema.Schema, error) {
	scriptDir, err := scriptDirectory()
	if err != nil {
		return nil, err
	}
	schemaDir := filepath.Join(scriptDir, "..", "assets", "schemas")

	c := jsonschema.NewCompiler()
	for _, name := range []string{"bom-1.6.schema.json", "spdx.schema.json", "jsf-0.82.schema.json"} {
		f, err := os.Open(filepath.Join(schemaDir, name)) //nolint:gosec // fixed set of filenames under this skill's own assets dir, not external input
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		defer func() { _ = f.Close() }()

		var doc any
		if err := json.NewDecoder(f).Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		id, _ := doc.(map[string]any)["$id"].(string)
		if id == "" {
			return nil, fmt.Errorf("%s has no $id", name)
		}
		if err := c.AddResource(id, mustReopen(schemaDir, name)); err != nil {
			return nil, fmt.Errorf("register %s as %s: %w", name, id, err)
		}
	}
	return c.Compile("http://cyclonedx.org/schema/bom-1.6.schema.json")
}

func mustReopen(dir, name string) *os.File {
	f, err := os.Open(filepath.Join(dir, name)) //nolint:gosec // see loadSchema's identical justification above
	if err != nil {
		panic(err) // can't happen: loadSchema already opened this same file successfully once
	}
	return f
}

// scriptDirectory returns the directory this script's schemas live next
// to. os.Executable() would point at `go run`'s temp build, not this
// source file, so instead this relies on the invocation convention
// documented in SKILL.md: always run from within scripts/ itself (the
// wrapper shell script does `cd "$(dirname "$0")"` first).
func scriptDirectory() (string, error) {
	return os.Getwd()
}

func validateOne(schema *jsonschema.Schema, path string) error {
	raw, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied argument to a local dev-tooling script, not remote input
	if err != nil {
		return err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	if err := schema.Validate(doc); err != nil {
		return fmt.Errorf("schema violation: %w", err)
	}
	return checkReferentialIntegrity(doc)
}

// checkReferentialIntegrity confirms every ref used in the dependency
// graph (both the "ref" being described and everything in its "dependsOn")
// actually names a component's bom-ref or the root metadata.component's
// bom-ref -- schema validation alone doesn't catch a dangling reference,
// since "dependsOn" entries are just free-form strings as far as the
// schema is concerned.
func checkReferentialIntegrity(doc any) error {
	root, _ := doc.(map[string]any)
	known := map[string]bool{}

	if metadata, ok := root["metadata"].(map[string]any); ok {
		if comp, ok := metadata["component"].(map[string]any); ok {
			if ref, ok := comp["bom-ref"].(string); ok {
				known[ref] = true
			}
		}
	}
	components, _ := root["components"].([]any)
	for _, c := range components {
		comp, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if ref, ok := comp["bom-ref"].(string); ok {
			known[ref] = true
		}
	}

	deps, _ := root["dependencies"].([]any)
	for _, d := range deps {
		dep, ok := d.(map[string]any)
		if !ok {
			continue
		}
		ref, _ := dep["ref"].(string)
		if ref != "" && !known[ref] {
			return fmt.Errorf("dependencies[].ref %q does not match any component's bom-ref", ref)
		}
		dependsOn, _ := dep["dependsOn"].([]any)
		for _, target := range dependsOn {
			t, _ := target.(string)
			if t != "" && !known[t] {
				return fmt.Errorf("dependencies[].dependsOn references %q, which does not match any component's bom-ref", t)
			}
		}
	}
	return nil
}
