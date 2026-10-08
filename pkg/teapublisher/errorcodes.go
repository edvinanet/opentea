// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisher

// Stable, machine-readable ErrorResponse.Code values for /publisher/v1
// (external security review, docs/security-review-publisher-design-260828.md
// finding 15). Deliberately a finite, coarse-grained taxonomy -- grouping
// related failures into a code a caller can actually branch on, not a
// unique code per human-readable message. Named the same way pkg/tea's own
// Error* unknown-error-type constants are (UPPER_SNAKE_CASE strings), but a
// wholly separate vocabulary: /publisher/v1 is not governed by
// spec/openapi.yaml, and these codes describe this API's own failure
// modes, several of which (signature/certificate/digest verification) have
// no consumer-spec equivalent at all.
const (
	// ErrorInvalidPathParameter: a UUID or version path segment isn't
	// well-formed.
	ErrorInvalidPathParameter = "INVALID_PATH_PARAMETER"
	// ErrorInvalidRequestBody: the request body isn't valid JSON (or,
	// for an upload, isn't a valid multipart form) at all -- distinct from
	// ErrorMissingField/ErrorInvalidField, which mean the body parsed fine
	// but a field inside it didn't.
	ErrorInvalidRequestBody = "INVALID_REQUEST_BODY"
	// ErrorMissingField: a required field was absent or empty. See
	// ErrorResponse.Fields for which one(s).
	ErrorMissingField = "MISSING_FIELD"
	// ErrorInvalidField: a field was present but its value was invalid
	// (wrong enum value, wrong encoding, etc). See ErrorResponse.Fields.
	ErrorInvalidField = "INVALID_FIELD"
	// ErrorLimitExceeded: a request-shape limit was exceeded (e.g.
	// createArtifact's maxArtifactFormats).
	ErrorLimitExceeded = "LIMIT_EXCEEDED"
	// ErrorAmbiguousFormat: an upload's mediaType matches more than one of
	// the artifact's formats -- use formatId instead (finding 14).
	ErrorAmbiguousFormat = "AMBIGUOUS_FORMAT"
	// ErrorNotFound: no such resource (existence-hiding does not apply to
	// /publisher/v1 the way it does to /tea/v1 -- a publisher credential is
	// always an authenticated platform operator, not an anonymous reader).
	ErrorNotFound = "NOT_FOUND"
	// ErrorConflict: the request is individually well-formed but rejected
	// because of the resource's current state, and no more specific
	// conflict code below fits.
	ErrorConflict = "CONFLICT"
	// ErrorRevisionConflict: an optimistic-concurrency check
	// (createArtifactVersion's previousVersion, putCollectionDraft's
	// expectedRevision) failed -- someone else's edit landed first.
	ErrorRevisionConflict = "REVISION_CONFLICT"
	// ErrorApprovalRequired: prepareCollectionCommit/commitCollectionDraft
	// called with no current, unexpired approval on record.
	ErrorApprovalRequired = "APPROVAL_REQUIRED"
	// ErrorSelfApproval: the same actor (or the same authenticated
	// credential) that drafted is also the one deciding it -- maker-checker
	// violation, distinct from a plain scope-based ErrorForbidden.
	ErrorSelfApproval = "SELF_APPROVAL"
	// ErrorEvidenceAlreadySubmitted: uploadArtifactFile rejected because
	// evidence already exists for this artifact version -- content is
	// frozen once validated (finding 1).
	ErrorEvidenceAlreadySubmitted = "EVIDENCE_ALREADY_SUBMITTED"
	// ErrorFingerprintReused: the submitted certificate's fingerprint has
	// already been used for a different evidence bundle -- an ephemeral
	// signing key must not sign more than once (an identical retry of the
	// exact same bundle is handled separately as a safe replay, finding
	// 13, and never reaches this code).
	ErrorFingerprintReused = "FINGERPRINT_REUSED"
	// ErrorDigestMismatch: a submitted objectDigestValue doesn't match the
	// server's own recomputed digest of the object's current content --
	// the "stale prepare" case named by finding 15: the object changed
	// since the matching prepare call returned this digest.
	ErrorDigestMismatch = "DIGEST_MISMATCH"
	// ErrorUnsupportedSignatureFormat: signatureFormat isn't one this
	// server's current phase actually verifies (today, only
	// jws-detached) -- named by finding 15.
	ErrorUnsupportedSignatureFormat = "UNSUPPORTED_SIGNATURE_FORMAT"
	// ErrorSignatureInvalid: the signature doesn't verify against the
	// certificate and digest -- named by finding 15.
	ErrorSignatureInvalid = "SIGNATURE_INVALID"
	// ErrorCertificateInvalid: the certificate itself is malformed, expired,
	// or otherwise unusable (parsing the public key or the subject failed)
	// -- named by finding 15.
	ErrorCertificateInvalid = "CERTIFICATE_INVALID"
	// ErrorUnauthorized: no bearer credential was presented, or the one
	// presented doesn't resolve to a valid, unrevoked publisher credential.
	ErrorUnauthorized = "UNAUTHORIZED"
	// ErrorForbidden: a valid credential was presented, but its scope
	// doesn't permit this operation.
	ErrorForbidden = "FORBIDDEN"
	// ErrorInternal: an unexpected server-side failure, no detail leaked.
	// The only code ErrorResponse.Retryable is ever true for.
	ErrorInternal = "INTERNAL_ERROR"
)
