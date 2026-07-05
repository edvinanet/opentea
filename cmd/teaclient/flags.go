package main

import (
	"errors"
	"flag"
	"strings"

	"github.com/oej/opentea/pkg/teaclient"
)

// newClientFromFlags registers the -server/-token/-json flags shared by
// every subcommand on fs, parses args, and builds a Client.
func newClientFromFlags(fs *flag.FlagSet, args []string) (client *teaclient.Client, jsonOut bool, rest []string, err error) {
	server := fs.String("server", "", "TEA server base URL, e.g. http://localhost:8080/tea/v1 (required)")
	token := fs.String("token", "", "optional bearer token")
	jsonFlag := fs.Bool("json", false, "output raw JSON instead of a human-readable summary")

	if err := fs.Parse(reorderArgsForFlagParsing(fs, args)); err != nil {
		return nil, false, nil, err
	}
	if *server == "" {
		return nil, false, nil, errors.New("-server is required")
	}

	var opts []teaclient.Option
	if *token != "" {
		opts = append(opts, teaclient.WithBearerToken(*token))
	}
	return teaclient.NewClient(*server, opts...), *jsonFlag, fs.Args(), nil
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
