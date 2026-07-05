package main

import (
	"context"
	"errors"
	"flag"
)

func runDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	client, jsonOut, rest, err := newClientFromFlags(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 1 {
		return errors.New("usage: teaclient discover <tei> -server=<url>")
	}

	results, err := client.Discover(context.Background(), rest[0])
	return printResult(results, err, jsonOut)
}
