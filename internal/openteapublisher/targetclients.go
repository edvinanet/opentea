// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// targetclients.go is the one place that builds a client against a stored
// Target -- both the read side (pkg/teaclient, for live-querying a
// target's /tea/v1) and the write side (pkg/teapublisherclient, for
// /publisher/v1), plus a shared helper for turning a *teapublisherclient.APIError
// into a human-readable message.
package openteapublisher

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/oej/opentea/pkg/teaclient"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// defaultConsumerAPIBasePath is TEA_API_BASE_PATH's own default
// (internal/config/config.go) -- the path this project's own server
// mounts /tea/v1 at unless an operator overrides it. teaClientForTarget
// uses this directly rather than deriving anything from Target.BaseURL's
// own "/publisher/v1" path (a sibling, independently-configurable mount,
// not something a /tea/v1 root is derivable from in general): a known,
// documented v1 assumption that this target is this project's own
// server running with its default consumer-API path, not a generic
// discovery mechanism for "any conformant TEA server" (see TODO.md).
const defaultConsumerAPIBasePath = "/tea/v1"

// teaPublisherClientForTarget builds a teapublisherclient.Client for
// targetUUID, presenting that target's own stored, full-scoped
// bearer_token. Used by both /cicdapi/v1 (cicdapi.go -- requireCICDCredential
// has already established the caller may act as cicd for this target; the
// target itself never sees "cicd" here, since it isn't a distinct
// credential on that side, only a narrower one issued by opentea-publisher,
// cicd_credential) and the GUI's own write actions (products.go,
// collectiondraft.go -- full-scoped throughout, §18.11).
func (s *Server) teaPublisherClientForTarget(ctx context.Context, targetUUID string) (*teapublisherclient.Client, error) {
	target, err := s.repo.GetTarget(ctx, targetUUID)
	if err != nil {
		return nil, err
	}
	return teapublisherclient.NewClient(target.BaseURL, target.BearerToken), nil
}

// teaClientForTarget builds a teaclient.Client (the read-only /tea/v1
// client) for targetUUID, authenticated with that same stored bearer_token
// -- not anonymous. A target may have narrowed its default anonymous
// entitlement away from the permissive "everyone/all_products" bootstrap
// row; reusing the full-scoped credential opentea-publisher already holds
// for this target means these read panels still see everything regardless
// of that entitlement configuration, rather than depending on it staying
// wide open.
//
// The /tea/v1 root is computed from Target.BaseURL's own scheme+host plus
// defaultConsumerAPIBasePath -- not Target.BaseURL verbatim, which is the
// /publisher/v1 root instead (a sibling, independently-configurable
// mount). See defaultConsumerAPIBasePath's own doc comment for the
// assumption this makes.
func (s *Server) teaClientForTarget(ctx context.Context, targetUUID string) (*teaclient.Client, error) {
	target, err := s.repo.GetTarget(ctx, targetUUID)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(target.BaseURL)
	if err != nil {
		return nil, err
	}
	base.Path = defaultConsumerAPIBasePath
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return teaclient.NewClient(base.String(), teaclient.WithBearerToken(target.BearerToken)), nil
}

// decodeAPIErrorMessage returns a human-readable message for err: if it's
// a *teapublisherclient.APIError, its Body is now always a structured
// teapublisher.ErrorResponse (every /publisher/v1 error path writes this
// shape), so this decodes it and returns .Message; falls back to the raw
// body text if decoding fails (e.g. a non-JSON response from a
// non-conformant target), or to err.Error() for anything that isn't an
// APIError at all (e.g. the target being unreachable).
func decodeAPIErrorMessage(err error) string {
	var apiErr *teapublisherclient.APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	var parsed teapublisher.ErrorResponse
	if jsonErr := json.Unmarshal(apiErr.Body, &parsed); jsonErr == nil && parsed.Message != "" {
		return parsed.Message
	}
	return string(apiErr.Body)
}
