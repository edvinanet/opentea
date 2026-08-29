// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	all := fs.Bool("all", false, "fetch every page, not just the first")
	idType := fs.String("id-type", "", "filter: identifier type (CPE, TEI, PURL, COMPLIANCE_DOCUMENT)")
	idValue := fs.String("id-value", "", "filter: identifier value")
	pageSize := fs.Int("page-size", 25, "page size, 1-100")
	client, jsonOut, rest, err := newClientFromFlags(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 1 {
		return errors.New("usage: teaclient list <products|product-releases|components|component-releases> -server=<url>")
	}
	kind := rest[0]
	ctx := context.Background()
	params := teaclient.ListParams{PageSize: *pageSize, IDType: *idType, IDValue: *idValue}

	switch kind {
	case "products":
		if *all {
			results, err := teaclient.ListAll(func(token string) ([]tea.Product, bool, string, error) {
				p := params
				p.PageToken = token
				resp, err := client.QueryProducts(ctx, p)
				return resp.Results, resp.HasNext, resp.NextPageToken, err
			})
			return printResult(results, err, jsonOut)
		}
		resp, err := client.QueryProducts(ctx, params)
		return printResult(resp, err, jsonOut)

	case "product-releases":
		if *all {
			results, err := teaclient.ListAll(func(token string) ([]tea.ProductRelease, bool, string, error) {
				p := params
				p.PageToken = token
				resp, err := client.QueryProductReleases(ctx, p)
				return resp.Results, resp.HasNext, resp.NextPageToken, err
			})
			return printResult(results, err, jsonOut)
		}
		resp, err := client.QueryProductReleases(ctx, params)
		return printResult(resp, err, jsonOut)

	case "components":
		if *all {
			results, err := teaclient.ListAll(func(token string) ([]tea.Component, bool, string, error) {
				p := params
				p.PageToken = token
				resp, err := client.QueryComponents(ctx, p)
				return resp.Results, resp.HasNext, resp.NextPageToken, err
			})
			return printResult(results, err, jsonOut)
		}
		resp, err := client.QueryComponents(ctx, params)
		return printResult(resp, err, jsonOut)

	case "component-releases":
		if *all {
			results, err := teaclient.ListAll(func(token string) ([]tea.ComponentRelease, bool, string, error) {
				p := params
				p.PageToken = token
				resp, err := client.QueryComponentReleases(ctx, p)
				return resp.Results, resp.HasNext, resp.NextPageToken, err
			})
			return printResult(results, err, jsonOut)
		}
		resp, err := client.QueryComponentReleases(ctx, params)
		return printResult(resp, err, jsonOut)

	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}
