// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command teaclient is a reference CLI for the CycloneDX Transparency
// Exchange API consumer read API, built on pkg/teaclient. Point it at any
// conformant TEA server for interoperability testing, not just this repo's
// own reference server.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("no command given")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "discover":
		return runDiscover(rest)
	case "get":
		return runGet(rest)
	case "list":
		return runList(rest)
	case "verify":
		return runVerify(rest)
	case "check":
		return runCheck(rest)
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `teaclient: reference CLI for the TEA consumer read API

Usage:
  teaclient discover <tei> [-server=<url>] [-token=<token>] [-json]
  teaclient get <product|product-release|component|component-release|product-release-collection|component-release-collection|artifact> <uuid> -server=<url> [-version=N] [-token=<token>] [-json]
  teaclient list <products|product-releases|components|component-releases> -server=<url> [-all] [-id-type=X -id-value=Y] [-page-size=N] [-token=<token>] [-json]
  teaclient verify artifact <uuid> -server=<url> [-version=N] [-token=<token>] [-json]
  teaclient check -server=<url> [-sample=N] [-token=<token>] [-json]

-server is required for every command except discover, where omitting it triggers TEI-authority
.well-known bootstrap discovery instead of querying one known server directly. -token attaches an
Authorization: Bearer header.`)
}
