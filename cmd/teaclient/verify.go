package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

type formatVerifyResult struct {
	FormatIndex int    `json:"formatIndex"`
	MediaType   string `json:"mediaType"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	version := fs.Int("version", 0, "artifact version to verify; 0 = latest")
	client, jsonOut, rest, err := newClientFromFlags(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 2 || rest[0] != "artifact" {
		return errors.New("usage: teaclient verify artifact <uuid> -server=<url>")
	}
	uuid := rest[1]
	ctx := context.Background()

	art, err := getArtifact(ctx, client, uuid, *version)
	if err != nil {
		return err
	}

	results := make([]formatVerifyResult, len(art.Formats))
	anyFail := false
	for i, format := range art.Formats {
		_, verr := client.DownloadAndVerify(ctx, format)
		results[i] = formatVerifyResult{FormatIndex: i, MediaType: format.MediaType, OK: verr == nil}
		if verr != nil {
			anyFail = true
			results[i].Error = verr.Error()
		}
	}

	if jsonOut {
		if err := printResult(results, nil, true); err != nil {
			return err
		}
	} else {
		fmt.Printf("artifact %s version %d: %d format(s)\n", art.UUID, art.Version, len(art.Formats))
		for _, r := range results {
			status := "OK"
			if !r.OK {
				status = "FAIL: " + r.Error
			}
			fmt.Printf("  [%d] %s: %s\n", r.FormatIndex, r.MediaType, status)
		}
	}

	if anyFail {
		return errors.New("one or more checksum verifications failed")
	}
	return nil
}

// getArtifact fetches the latest artifact revision (version <= 0) or a
// specific one, shared by verify and check.
func getArtifact(ctx context.Context, client *teaclient.Client, uuid string, version int) (tea.Artifact, error) {
	if version > 0 {
		return client.GetArtifactByVersion(ctx, uuid, version)
	}
	return client.GetLatestArtifact(ctx, uuid)
}
