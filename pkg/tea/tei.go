// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package tea

import "net/url"

// ExtractTEIAuthority returns the domain-name/authority component of a TEI
// (Transparency Exchange Identifier) in the form
// "tei://<domain-name>/<type>/<identifier>". The authority is the host DNS
// is queried against and .well-known/tea is fetched from during TEA's
// bootstrap discovery flow (see pkg/teaclient.BootstrapDiscover).
//
// This intentionally does NOT decode or otherwise interpret the
// <type>/<identifier> path segments -- discovery only needs the authority;
// the resolved server receives the original, unmodified TEI string as its
// own ?tei= query value (ordinary RFC 3986 URL/percent-encoding, applied by
// net/url like any other query value -- the identifier segment is not
// BASE64URL-encoded), matching how a direct (non-bootstrap) Discover call
// already works.
//
// A port in the authority is rejected: the TEA discovery spec is explicit
// that "the port number is not part of the TEI" -- a non-default port is
// only ever learned from an HTTPS/SVCB DNS record or an endpoints[].url in
// the fetched .well-known/tea document, never from the TEI itself (verified
// against CycloneDX/transparency-exchange-api's discovery/readme.md at the
// exact commit this package was built against, be64bc7; found via an
// external review of docs/discovery-test-rig.md, 2026-08-27, whose original
// TEST-14 incorrectly modeled the opposite).
//
// Built against the TEI URL syntax from CycloneDX/transparency-exchange-api
// PR #261 (unmerged at the time this was written) -- the prior URN syntax
// (urn:tei:<type>:<domain>:<id>) is not accepted here; if that PR's syntax
// changes before merging, revisit this.
func ExtractTEIAuthority(tei string) (string, error) {
	u, err := url.Parse(tei)
	if err != nil {
		return "", &TEIError{TEI: tei, Reason: "not a valid URL: " + err.Error()}
	}
	if u.Scheme != "tei" {
		return "", &TEIError{TEI: tei, Reason: "scheme must be \"tei\", got " + quoteOrEmpty(u.Scheme)}
	}
	if u.Host == "" {
		return "", &TEIError{TEI: tei, Reason: "missing authority (domain-name) component"}
	}
	if u.Port() != "" {
		return "", &TEIError{TEI: tei, Reason: "authority must not include a port -- a port comes from an HTTPS/SVCB record or endpoints[].url during discovery, never the TEI itself"}
	}
	return u.Host, nil
}

// TEIError reports why a TEI string could not be parsed.
type TEIError struct {
	TEI    string
	Reason string
}

func (e *TEIError) Error() string {
	return "tea: invalid TEI " + quoteOrEmpty(e.TEI) + ": " + e.Reason
}

func quoteOrEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return "\"" + s + "\""
}
