// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisher

// Enumerations from design/publisher-openapi.yaml, as untyped string
// constants (not a named type) so they drop into existing plain-string
// fields without any conversion -- mirrors pkg/tea/enums.go's own
// convention: a shared source of truth for valid values, not a
// type-safety change.

// evidence-submission.signatureFormat -- mirrors internal/trust.SignatureFormat's
// vocabulary (kept as separate constants rather than importing internal/trust,
// which this package, unlike internal/api, must not depend on).
const (
	SignatureFormatJWSDetached  = "jws-detached"
	SignatureFormatCMSDetached  = "cms-detached"
	SignatureFormatDSSEEnvelope = "dsse-envelope"
	SignatureFormatCOSESign1    = "cose-sign1"
)

// collection-draft.approval.status
const (
	ApprovalStatusNone     = "none"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)
