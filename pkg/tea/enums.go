// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package tea

// Enumerations from spec/openapi.yaml, as untyped string constants (not a
// named type) so they drop into existing plain-string fields without any
// conversion -- this is a shared source of truth for valid values, not a
// type-safety change.

// identifier-type
const (
	IdentifierTypeCPE                = "CPE"
	IdentifierTypeTEI                = "TEI"
	IdentifierTypePURL               = "PURL"
	IdentifierTypeComplianceDocument = "COMPLIANCE_DOCUMENT"
)

// compliance-document-type (TEA 1.0, spec/openapi.yaml) -- the set of
// values identifier.idValue may take when identifier.idType is
// IdentifierTypeComplianceDocument. Validated in internal/repo (the
// single point every identifier-write path funnels through), not here --
// this file is a shared source of valid *values*, not a validator.
const (
	ComplianceDocumentTypeSOC2TypeI                 = "SOC_2_TYPE_I"
	ComplianceDocumentTypeSOC2TypeII                = "SOC_2_TYPE_II"
	ComplianceDocumentTypeSOC3                      = "SOC_3"
	ComplianceDocumentTypeISO27001                  = "ISO_27001"
	ComplianceDocumentTypeISO27017                  = "ISO_27017"
	ComplianceDocumentTypeISO27018                  = "ISO_27018"
	ComplianceDocumentTypeISO27701                  = "ISO_27701"
	ComplianceDocumentTypeISO42001                  = "ISO_42001"
	ComplianceDocumentTypePCIDSS                    = "PCI_DSS"
	ComplianceDocumentTypeHIPAA                     = "HIPAA"
	ComplianceDocumentTypeFedRAMP                   = "FEDRAMP"
	ComplianceDocumentTypeGDPR                      = "GDPR"
	ComplianceDocumentTypeCSAStar                   = "CSA_STAR"
	ComplianceDocumentTypeNIST80053                 = "NIST_800_53"
	ComplianceDocumentTypeNIST800171                = "NIST_800_171"
	ComplianceDocumentTypeCMMC                      = "CMMC"
	ComplianceDocumentTypeHITRUST                   = "HITRUST"
	ComplianceDocumentTypeTISAX                     = "TISAX"
	ComplianceDocumentTypeCyberEssentials           = "CYBER_ESSENTIALS"
	ComplianceDocumentTypeCyberEssentialsPlus       = "CYBER_ESSENTIALS_PLUS"
	ComplianceDocumentTypeEUDeclarationOfConformity = "EU_DECLARATION_OF_CONFORMITY"
)

// artifact-type
const (
	ArtifactTypeAttestation     = "ATTESTATION"
	ArtifactTypeBOM             = "BOM"
	ArtifactTypeBuildMeta       = "BUILD_META"
	ArtifactTypeCertification   = "CERTIFICATION"
	ArtifactTypeFormulation     = "FORMULATION"
	ArtifactTypeLicense         = "LICENSE"
	ArtifactTypeReleaseNotes    = "RELEASE_NOTES"
	ArtifactTypeSecurityTXT     = "SECURITY_TXT"
	ArtifactTypeThreatModel     = "THREAT_MODEL"
	ArtifactTypeVulnerabilities = "VULNERABILITIES"
	ArtifactTypeOther           = "OTHER"
)

// checksum-type
const (
	ChecksumTypeMD5        = "MD5"
	ChecksumTypeSHA1       = "SHA-1"
	ChecksumTypeSHA256     = "SHA-256"
	ChecksumTypeSHA384     = "SHA-384"
	ChecksumTypeSHA512     = "SHA-512"
	ChecksumTypeSHA3_256   = "SHA3-256"
	ChecksumTypeSHA3_384   = "SHA3-384"
	ChecksumTypeSHA3_512   = "SHA3-512"
	ChecksumTypeBLAKE2b256 = "BLAKE2b-256"
	ChecksumTypeBLAKE2b384 = "BLAKE2b-384"
	ChecksumTypeBLAKE2b512 = "BLAKE2b-512"
	ChecksumTypeBLAKE3     = "BLAKE3"
)

// collection-update-reason-type
const (
	CollectionUpdateReasonInitialRelease  = "INITIAL_RELEASE"
	CollectionUpdateReasonVEXUpdated      = "VEX_UPDATED"
	CollectionUpdateReasonArtifactUpdated = "ARTIFACT_UPDATED"
	CollectionUpdateReasonArtifactAdded   = "ARTIFACT_ADDED"
	CollectionUpdateReasonArtifactRemoved = "ARTIFACT_REMOVED"
)

// collection-belongs-to-type
const (
	CollectionBelongsToComponentRelease = "COMPONENT_RELEASE"
	CollectionBelongsToProductRelease   = "PRODUCT_RELEASE"
)

// cle-event-type
const (
	CLEEventTypeReleased          = "released"
	CLEEventTypeEndOfDevelopment  = "endOfDevelopment"
	CLEEventTypeEndOfSupport      = "endOfSupport"
	CLEEventTypeEndOfLife         = "endOfLife"
	CLEEventTypeEndOfDistribution = "endOfDistribution"
	CLEEventTypeEndOfMarketing    = "endOfMarketing"
	CLEEventTypeSupersededBy      = "supersededBy"
	CLEEventTypeComponentRenamed  = "componentRenamed"
	CLEEventTypeWithdrawn         = "withdrawn"
)
