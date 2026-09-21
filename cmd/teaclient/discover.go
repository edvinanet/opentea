// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/oej/opentea/pkg/teaclient"
)

// runDiscover is deliberately not built on the shared newClientFromFlags
// (flags.go) -- every other subcommand requires a known -server since they
// have no TEI-only entry point, but discover's whole point (when -server
// is omitted) is to find a server from nothing but a TEI, via
// teaclient.BootstrapDiscover.
func runDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	server := fs.String("server", "", "TEA server base URL, e.g. http://localhost:8080/tea/v1 (optional for TEI discovery -- omit to use TEI-authority .well-known bootstrap discovery instead; required for -purl)")
	purl := fs.String("purl", "", "Package URL (PURL) to discover instead of a TEI -- requires -server; there is no .well-known bootstrap flow for a bare PURL (TEA 1.0, spec/openapi.yaml), the caller must already know which server to ask")
	token := fs.String("token", "", "optional bearer token")
	jsonFlag := fs.Bool("json", false, "output raw JSON instead of a human-readable summary")

	if err := fs.Parse(reorderArgsForFlagParsing(fs, args)); err != nil {
		return err
	}

	var opts []teaclient.Option
	if *token != "" {
		opts = append(opts, teaclient.WithBearerToken(*token))
	}

	if *purl != "" {
		if *server == "" {
			return errors.New("usage: teaclient discover -purl=<purl> -server=<url> [-token=<token>] [-json]\n  -purl has no .well-known bootstrap flow, -server is required")
		}
		client := teaclient.NewClient(*server, opts...)
		results, err := client.DiscoverByPURL(context.Background(), *purl)
		return printResult(results, err, *jsonFlag)
	}

	rest := fs.Args()
	if len(rest) < 1 {
		return errors.New("usage: teaclient discover <tei> [-server=<url>] [-token=<token>] [-json]\n  omitting -server triggers TEI-authority .well-known bootstrap discovery\n   or: teaclient discover -purl=<purl> -server=<url> [-token=<token>] [-json]")
	}
	tei := rest[0]

	if *server != "" {
		client := teaclient.NewClient(*server, opts...)
		results, err := client.Discover(context.Background(), tei)
		return printResult(results, err, *jsonFlag)
	}

	result, err := teaclient.BootstrapDiscover(context.Background(), tei, opts...)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printResult(result, nil, true)
	}
	fmt.Printf("resolved server: %s\n", result.ServerURL)
	return printResult(result.Info, nil, false)
}
