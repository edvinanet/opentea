// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package conformance implements a client-side conformance test suite for
// the standard TEA 1.0 consumer read API (/tea/v1), built on pkg/teaclient
// -- usable against any server claiming to implement the specification,
// not just this repo's own. See docs/consumer-api-conformance-test-rig.md
// for the spec-facing methodology this package implements, and
// docs/bundle-import-export-test-rig.md for the reference datasets
// (internal/testdataset) it reads back.
//
// Two independent scores, per the methodology doc: endpoint+method
// coverage (did the run call every operation the spec defines at least
// once) and data correctness (did the server's answers match what the
// reference datasets are known to contain). Neither implies the other.
package conformance

import (
	"fmt"
	"regexp"
	"strings"
)

// Operation is one (HTTP method, path template) pair the TEA 1.0 consumer
// specification defines.
type Operation struct {
	Method      string
	PathPattern string // the spec's own {param}-style template, e.g. "/product/{uuid}"
	OperationID string // spec/openapi.yaml's own operationId, for a human-readable report
	pattern     *regexp.Regexp
}

// Key identifies an Operation uniquely for coverage bookkeeping.
func (op Operation) Key() string { return op.Method + " " + op.PathPattern }

// paramPattern maps a {param} path-template placeholder to the regex
// fragment that matches a real value for it -- {uuid} is opaque (any
// non-"/" run: this suite doesn't assume opentea's own UUID shape is
// universal), the two version placeholders are integers.
var paramPattern = map[string]string{
	"{uuid}":              `[^/]+`,
	"{artifactVersion}":   `[0-9]+`,
	"{collectionVersion}": `[0-9]+`,
}

// compilePattern turns a spec path template into an anchored regex. Kept
// as a small compiler (rather than hand-writing 28 regexes) so this
// table's PathPattern column reads identically to the spec document itself
// -- easy to audit by eye against spec/openapi.yaml in the sibling
// transparency-exchange-api checkout.
func compilePattern(template string) *regexp.Regexp {
	re := regexp.QuoteMeta(template)
	for param, frag := range paramPattern {
		re = strings.ReplaceAll(re, regexp.QuoteMeta(param), frag)
	}
	return regexp.MustCompile("^" + re + "$")
}

// Operations is every (method, path) pair the TEA 1.0 consumer
// specification defines, hand-transcribed from spec/openapi.yaml (read in
// the sibling transparency-exchange-api checkout, v1.0.0) on 2026-10-10.
// Regenerate by running, against that checkout:
//
//	awk '
//	  /^  \/[^ ]*:$/ { path=$1; sub(/:$/,"",path); next }
//	  /^    (get|post|put|delete|patch|head|options):$/ {
//	    method=$1; sub(/:$/,"",method); print toupper(method), path
//	  }
//	' spec/openapi.yaml
//
// and diffing the result against the list below -- the spec's own 2-space
// path / 4-space method YAML indentation makes this reliable without a
// YAML parser dependency (there is no existing one in this module; see
// .claude/skills/dependency-review's selection criteria: a one-time,
// 28-line extraction doesn't justify adding one).
var Operations = []Operation{
	{Method: "GET", PathPattern: "/discovery", OperationID: "discover"},
	{Method: "GET", PathPattern: "/products", OperationID: "queryProducts"},
	{Method: "GET", PathPattern: "/product/{uuid}", OperationID: "getProduct"},
	{Method: "GET", PathPattern: "/product/{uuid}/releases", OperationID: "getProductReleases"},
	{Method: "GET", PathPattern: "/product/{uuid}/cle", OperationID: "getProductCLE"},
	{Method: "GET", PathPattern: "/productReleases", OperationID: "queryProductReleases"},
	{Method: "GET", PathPattern: "/productRelease/{uuid}", OperationID: "getProductRelease"},
	{Method: "GET", PathPattern: "/productRelease/{uuid}/cle", OperationID: "getProductReleaseCLE"},
	{Method: "GET", PathPattern: "/productRelease/{uuid}/collections", OperationID: "getProductReleaseCollections"},
	{Method: "GET", PathPattern: "/productRelease/{uuid}/collection/latest", OperationID: "getLatestProductReleaseCollection"},
	{Method: "GET", PathPattern: "/productRelease/{uuid}/collection/{collectionVersion}", OperationID: "getProductReleaseCollection"},
	{Method: "GET", PathPattern: "/components", OperationID: "queryComponents"},
	{Method: "GET", PathPattern: "/component/{uuid}", OperationID: "getComponent"},
	{Method: "GET", PathPattern: "/component/{uuid}/releases", OperationID: "getComponentReleases"},
	{Method: "GET", PathPattern: "/component/{uuid}/cle", OperationID: "getComponentCLE"},
	{Method: "GET", PathPattern: "/componentReleases", OperationID: "queryComponentReleases"},
	{Method: "GET", PathPattern: "/componentRelease/{uuid}", OperationID: "getComponentRelease"},
	{Method: "GET", PathPattern: "/componentRelease/{uuid}/cle", OperationID: "getComponentReleaseCLE"},
	{Method: "GET", PathPattern: "/componentRelease/{uuid}/collections", OperationID: "getComponentReleaseCollections"},
	{Method: "GET", PathPattern: "/componentRelease/{uuid}/collection/latest", OperationID: "getLatestComponentReleaseCollection"},
	{Method: "GET", PathPattern: "/componentRelease/{uuid}/collection/{collectionVersion}", OperationID: "getComponentReleaseCollection"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/latest", OperationID: "getLatestArtifact"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/latest/download", OperationID: "downloadLatestArtifact"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/latest/signature/download", OperationID: "downloadLatestArtifactSignature"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/{artifactVersion}", OperationID: "getArtifactByVersion"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/{artifactVersion}/download", OperationID: "downloadArtifactByVersion"},
	{Method: "GET", PathPattern: "/artifact/{uuid}/{artifactVersion}/signature/download", OperationID: "downloadArtifactSignatureByVersion"},
	{Method: "POST", PathPattern: "/token", OperationID: "exchangeToken"},
}

func init() {
	for i := range Operations {
		Operations[i].pattern = compilePattern(Operations[i].PathPattern)
	}
	// Fail fast (at package init, not at some later call site) if two
	// patterns ever became ambiguous -- every pattern here is "^...$"
	// anchored, so distinguishing path shapes (e.g. ".../latest" vs
	// ".../latest/download") never overlap by construction, but this
	// guards a future entry added without checking that.
	seen := map[string]bool{}
	for _, op := range Operations {
		if seen[op.Key()] {
			panic(fmt.Sprintf("conformance: duplicate operation registered: %s", op.Key()))
		}
		seen[op.Key()] = true
	}
}

// Lookup finds the Operation matching method and relPath (the request
// path with the server's own API base path already stripped, e.g.
// "/product/abc-123", not "/tea/v1/product/abc-123"). ok is false for any
// path this registry doesn't recognize (a request to an external url, or
// a server extension outside the spec).
func Lookup(method, relPath string) (op Operation, ok bool) {
	for _, candidate := range Operations {
		if candidate.Method == method && candidate.pattern.MatchString(relPath) {
			return candidate, true
		}
	}
	return Operation{}, false
}
