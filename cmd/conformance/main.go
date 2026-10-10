// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command conformance runs the client-side TEA 1.0 consumer-API
// conformance suite (internal/conformance) against a server's /tea/v1
// root -- any server claiming to implement the specification, not just
// this repo's own. See docs/consumer-api-conformance-test-rig.md for the
// methodology, and docs/bundle-import-export-test-rig.md for how to load
// the reference datasets this suite checks onto a server first.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/oej/opentea/internal/conformance"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("conformance", flag.ExitOnError)
	server := fs.String("server", "", "TEA API root URL to test, e.g. http://localhost:8080/tea/v1 (required)")
	token := fs.String("token", "", "bearer token to send with every request, if the server requires authentication")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *server == "" {
		fmt.Fprintln(os.Stderr, "usage: conformance -server=<url> [-token=<bearer-token>]")
		return 1
	}

	report, err := conformance.Run(context.Background(), *server, conformance.Options{BearerToken: *token})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	printReport(report)
	if report.Correctness.Total == 0 || report.Correctness.Percent() < 100 {
		return 1
	}
	return 0
}

func printReport(report *conformance.Report) {
	fmt.Printf("Endpoint+method coverage: %d/%d (%.1f%%)\n",
		report.Coverage.Exercised, report.Coverage.Total, report.Coverage.Percent())
	for _, key := range report.Coverage.NeverCalled {
		fmt.Printf("  never called: %s\n", key)
	}

	fmt.Printf("Data correctness: %d/%d (%.1f%%)\n",
		report.Correctness.Passed, report.Correctness.Total, report.Correctness.Percent())
	for _, label := range report.Correctness.Failed {
		fmt.Printf("  FAILED: %s\n", label)
	}
}
