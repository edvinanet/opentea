// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/oej/opentea/pkg/teaclient"
)

// newClientFromFlags registers the -server/-token/-apikey/-json flags
// shared by every subcommand on fs, parses args, and builds a Client.
func newClientFromFlags(fs *flag.FlagSet, args []string) (client *teaclient.Client, jsonOut bool, rest []string, err error) {
	server := fs.String("server", "", "TEA server base URL, e.g. http://localhost:8080/tea/v1 (required)")
	token := fs.String("token", "", "optional bearer token (an already-issued, short-lived access token -- not an API key)")
	apiKey := fs.String("apikey", "", "optional API key as keyId:secret, exchanged for a short-lived access token via POST /token before use -- mutually exclusive with -token")
	jsonFlag := fs.Bool("json", false, "output raw JSON instead of a human-readable summary")

	if err := fs.Parse(reorderArgsForFlagParsing(fs, args)); err != nil {
		return nil, false, nil, err
	}
	if *server == "" {
		return nil, false, nil, errors.New("-server is required")
	}

	bearer, err := resolveBearerToken(*server, *token, *apiKey)
	if err != nil {
		return nil, false, nil, err
	}

	var opts []teaclient.Option
	if bearer != "" {
		opts = append(opts, teaclient.WithBearerToken(bearer))
	}
	return teaclient.NewClient(*server, opts...), *jsonFlag, fs.Args(), nil
}

// resolveBearerToken returns the bearer token to attach to requests: token
// as-is if given directly, or the access token obtained by exchanging
// apiKey (a "keyId:secret" pair, matching the admin GUI's own documented
// curl -u <keyId>:<secret> usage) against server's POST /token if apiKey is
// given instead. token and apiKey are mutually exclusive; both empty
// returns "" (no bearer -- fine for TEA's optional-auth public endpoints).
// This is the client-side half of TEA 1.0's baseline authentication flow:
// this CLI used to accept only an already-issued bearer token, with no way
// to perform the key exchange itself (docs/security-review-260923.md
// finding #10).
func resolveBearerToken(server, token, apiKey string) (string, error) {
	if token != "" && apiKey != "" {
		return "", errors.New("-token and -apikey are mutually exclusive")
	}
	if apiKey == "" {
		return token, nil
	}
	if server == "" {
		return "", errors.New("-apikey requires -server (an API key is exchanged against a specific TEA server's own /token endpoint)")
	}
	keyID, secret, ok := strings.Cut(apiKey, ":")
	if !ok {
		return "", errors.New("-apikey must be keyId:secret")
	}
	resp, err := teaclient.NewClient(server).ExchangeToken(context.Background(), keyID, secret)
	if err != nil {
		return "", fmt.Errorf("exchanging API key for access token: %w", err)
	}
	return resp.AccessToken, nil
}

// reorderArgsForFlagParsing moves every recognized flag (and, for
// non-boolean flags, its value) ahead of positional arguments. The stdlib
// flag package stops parsing at the first non-flag token, so without this,
// something like "teaclient list products -server=X" would silently ignore
// -server -- this lets flags appear anywhere on the command line, which is
// what most users will naturally try. fs must already have every flag it
// will accept registered (via fs.String/fs.Bool/etc.) before this is called.
func reorderArgsForFlagParsing(fs *flag.FlagSet, args []string) []string {
	boolFlags := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue // self-contained -name=value
		}
		if !boolFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i]) // -name value: consume the value token too
		}
	}
	return append(flags, positional...)
}
