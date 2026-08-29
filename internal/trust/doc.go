// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package trust implements the cryptographic core of oej's TEA Trust
// Architecture overlay (github.com/oej/tea-trust-architecture): Ed25519
// signing/verification, ephemeral-key lifecycle, fingerprint-derived
// identity, short-lived self-signed certificates, and RFC 8785 JSON
// canonicalization for evidence-bundle digests.
//
// This package answers "is this evidence of an artifact/collection's origin
// and integrity valid" -- it has no notion of who is allowed to read
// anything. internal/authz answers that separate question ("can this
// caller read this resource") for /tea/v1 consumers and is deliberately not
// imported here, nor does this package add to authz's capability
// vocabulary. See the phased trust-architecture implementation plan for the
// full boundary rationale.
//
// Like internal/authz, this package has no DB or HTTP imports -- it is pure
// logic, unit-testable with no SQLite involved. internal/repo persists the
// evidence-bundle shapes this package produces/verifies;
// internal/admin wires it into HTTP handlers.
package trust
