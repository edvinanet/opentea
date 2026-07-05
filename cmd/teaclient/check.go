package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// checkReport is a deliberately modest first cut at an interoperability
// report: it walks reachable data and re-verifies pagination and a bounded
// sample of artifact checksums. Deeper spec-conformance checks (required
// fields, enum validity, cross-reference integrity) are a follow-up -- see
// TODO.md.
type checkReport struct {
	ProductsChecked          int      `json:"productsChecked"`
	ProductReleasesChecked   int      `json:"productReleasesChecked"`
	ComponentsChecked        int      `json:"componentsChecked"`
	ComponentReleasesChecked int      `json:"componentReleasesChecked"`
	ArtifactsVerified        int      `json:"artifactsVerified"`
	Failures                 []string `json:"failures"`
}

func (r *checkReport) fail(format string, args ...any) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	sample := fs.Int("sample", 5, "max number of artifacts to checksum-verify")
	client, jsonOut, _, err := newClientFromFlags(fs, args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	report := &checkReport{Failures: []string{}}

	products, err := teaclient.ListAll(func(token string) ([]tea.Product, bool, string, error) {
		resp, err := client.QueryProducts(ctx, teaclient.ListParams{PageToken: token})
		return resp.Results, resp.HasNext, resp.NextPageToken, err
	})
	if err != nil {
		report.fail("list products: %v", err)
	}
	report.ProductsChecked = len(products)

	for _, p := range products {
		releases, err := teaclient.ListAll(func(token string) ([]tea.ProductRelease, bool, string, error) {
			resp, err := client.ListReleasesByProduct(ctx, p.UUID, teaclient.ListParams{PageToken: token})
			return resp.Results, resp.HasNext, resp.NextPageToken, err
		})
		if err != nil {
			report.fail("list releases for product %s: %v", p.UUID, err)
			continue
		}
		report.ProductReleasesChecked += len(releases)
		for _, rel := range releases {
			if _, err := client.GetProductRelease(ctx, rel.UUID); err != nil {
				report.fail("re-fetch product release %s: %v", rel.UUID, err)
			}
		}
	}

	components, err := teaclient.ListAll(func(token string) ([]tea.Component, bool, string, error) {
		resp, err := client.QueryComponents(ctx, teaclient.ListParams{PageToken: token})
		return resp.Results, resp.HasNext, resp.NextPageToken, err
	})
	if err != nil {
		report.fail("list components: %v", err)
	}
	report.ComponentsChecked = len(components)

	componentReleases, err := teaclient.ListAll(func(token string) ([]tea.ComponentRelease, bool, string, error) {
		resp, err := client.QueryComponentReleases(ctx, teaclient.ListParams{PageToken: token})
		return resp.Results, resp.HasNext, resp.NextPageToken, err
	})
	if err != nil {
		report.fail("list component releases: %v", err)
	}
	report.ComponentReleasesChecked = len(componentReleases)

	verified := 0
	for _, cr := range componentReleases {
		if verified >= *sample {
			break
		}
		withCollection, err := client.GetComponentReleaseWithCollection(ctx, cr.UUID)
		if err != nil {
			report.fail("fetch component-release-with-collection %s: %v", cr.UUID, err)
			continue
		}
		for _, artifact := range withCollection.LatestCollection.Artifacts {
			if verified >= *sample {
				break
			}
			for i, format := range artifact.Formats {
				if len(format.Checksums) == 0 {
					continue
				}
				if _, err := client.DownloadAndVerify(ctx, format); err != nil {
					report.fail("verify artifact %s format[%d]: %v", artifact.UUID, i, err)
				}
				verified++
				break // one verified format per artifact is enough for a bounded sample
			}
		}
	}
	report.ArtifactsVerified = verified

	if jsonOut {
		if err := printResult(report, nil, true); err != nil {
			return err
		}
	} else {
		fmt.Printf("checked %d products, %d product releases, %d components, %d component releases; verified %d artifact checksum(s)\n",
			report.ProductsChecked, report.ProductReleasesChecked, report.ComponentsChecked, report.ComponentReleasesChecked, report.ArtifactsVerified)
		if len(report.Failures) == 0 {
			fmt.Println("no issues found")
		} else {
			fmt.Printf("%d issue(s) found:\n", len(report.Failures))
			for _, f := range report.Failures {
				fmt.Println("  -", f)
			}
		}
	}

	if len(report.Failures) > 0 {
		return fmt.Errorf("conformance check found %d issue(s)", len(report.Failures))
	}
	return nil
}
