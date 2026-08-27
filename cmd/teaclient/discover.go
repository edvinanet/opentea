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
	server := fs.String("server", "", "TEA server base URL, e.g. http://localhost:8080/tea/v1 (optional -- omit to use TEI-authority .well-known bootstrap discovery instead)")
	token := fs.String("token", "", "optional bearer token")
	jsonFlag := fs.Bool("json", false, "output raw JSON instead of a human-readable summary")

	if err := fs.Parse(reorderArgsForFlagParsing(fs, args)); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 1 {
		return errors.New("usage: teaclient discover <tei> [-server=<url>] [-token=<token>] [-json]\n  omitting -server triggers TEI-authority .well-known bootstrap discovery")
	}
	tei := rest[0]

	var opts []teaclient.Option
	if *token != "" {
		opts = append(opts, teaclient.WithBearerToken(*token))
	}

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
