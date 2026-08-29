// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command fixtures is a generic authenticated-HTTP-replay tool: it loads a
// structured JSON fixture file into a running opentea server via the
// existing /admin/v1 API. It is NOT the (deferred) reference publisher --
// that depends on an OpenAPI spec that doesn't exist yet. This is just a
// reusable way to seed demo/test data now, and to load the shared reference
// test-data set (testdata/fixtures/) for interoperability testing.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("fixtures", flag.ExitOnError)
	server := fs.String("server", "", "opentea server root URL, e.g. http://localhost:8080 (required)")
	username := fs.String("username", "", "admin username (required)")
	password := fs.String("password", "", "admin password (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *server == "" || *username == "" || *password == "" || len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: fixtures -server=<url> -username=<u> -password=<p> <fixture.json>")
		return fmt.Errorf("missing required arguments")
	}
	fixturePath := rest[0]

	fixture, err := loadFixtureFile(fixturePath)
	if err != nil {
		return err
	}

	runner, err := Login(*server, *username, *password)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	runner.fixtureDir = filepath.Dir(fixturePath)

	return runner.Run(fixture)
}
