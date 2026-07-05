package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	version := fs.Int("version", 0, "revision to fetch (artifact) or version to fetch (collection); 0 = latest")
	client, jsonOut, rest, err := newClientFromFlags(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return errors.New("usage: teaclient get <kind> <uuid> -server=<url> (kinds: product, product-release, component, component-release, product-release-collection, component-release-collection, artifact)")
	}
	kind, uuid := rest[0], rest[1]
	ctx := context.Background()

	switch kind {
	case "product":
		v, err := client.GetProduct(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "product-release":
		v, err := client.GetProductRelease(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "component":
		v, err := client.GetComponent(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "component-release":
		v, err := client.GetComponentReleaseWithCollection(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "product-release-collection":
		if *version > 0 {
			v, err := client.GetCollectionForProductRelease(ctx, uuid, *version)
			return printResult(v, err, jsonOut)
		}
		v, err := client.GetLatestCollectionForProductRelease(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "component-release-collection":
		if *version > 0 {
			v, err := client.GetCollectionForComponentRelease(ctx, uuid, *version)
			return printResult(v, err, jsonOut)
		}
		v, err := client.GetLatestCollectionForComponentRelease(ctx, uuid)
		return printResult(v, err, jsonOut)
	case "artifact":
		if *version > 0 {
			v, err := client.GetArtifactByVersion(ctx, uuid, *version)
			return printResult(v, err, jsonOut)
		}
		v, err := client.GetLatestArtifact(ctx, uuid)
		return printResult(v, err, jsonOut)
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}
