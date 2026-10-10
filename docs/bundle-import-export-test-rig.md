# Product bundle import/export test rig

A reference set of bundle zip files (`docs/bundle-format.md`'s format — `manifest.json` +
`files/<sha256>`) for testing the `/admin/v1` product import/export mechanism itself:
`GET /admin/v1/products/{uuid}/export`, `POST /admin/v1/products/import`, and the
standalone `cmd/bundlecheck` validator. Companion to
`docs/consumer-api-conformance-test-rig.md`, which uses the same *good* datasets from the
other side — reading them back through `/tea/v1` once imported — but is a genuinely
different document because it tests a genuinely different thing: this one is about
opentea's own (hopefully-to-become-standard) import/export contract, not the TEA consumer
spec, and stays admin-side throughout.

Every dataset here is produced by `cmd/testbundlegen` (`go run ./cmd/testbundlegen`,
writing to `testdata/bundles/` by default) rather than hand-typed JSON — the complex
dataset alone has over a hundred component releases, which isn't something to author or
review by hand. The generator is itself the source of truth; the committed `.zip` files
under `testdata/bundles/` are its output, checked in so the data is usable without building
Go code first.

**A constraint this rig exists partly to prove out**: `bundle.ImportResult` returns only
creation counts, never a mapping from the bundle's authored UUIDs to whatever the
destination server actually assigned — and nothing in the format requires a destination to
preserve them at all. Every dataset below gives every entity a stable, deterministic
identifier (PURL, or TEI where a PURL doesn't fit) for exactly this reason: that's the only
reliable way to find an entity again after import, on this server or any other. See
`docs/consumer-api-conformance-test-rig.md`'s "Identity resolution" section for how the
companion suite actually uses this.

## Summary

| Dataset | File | Products | Components | Releases | Collections | What it exercises |
|---|---|---|---|---|---|---|
| Simple | `testdata/bundles/simple.zip` | 1 | 2 | 1 product + 4 component | 10 (2 versions × 5 releases) | Baseline import: small, every entity kind present once, artifact versioning (v1→v2 of the same SBOM) |
| Complex | `testdata/bundles/complex.zip` | 1 | 10 | 3 product + 100 component | 100+ (one 4-version history) | Scale (100+ component releases), shared-artifact dedup across many collections, multi-format artifact, collection version history |
| Lifecycle & compliance | `testdata/bundles/lifecycle-compliance.zip` | 1 | 2–3 | 2–3 product + 2–3 component | a few | Every CLE event type, CLE at all four entity levels, `COMPLIANCE_DOCUMENT` identifiers, a real compliance artifact |
| Bad bundles (5 files) | `testdata/bundles/bad-*.zip` | 1 each | minimal | minimal | minimal | Import/`bundlecheck` rejection, one specific defect per file |

## Dataset 1: Simple

**Product**: "Acme Simple Gadget", 1 release (`1.0.0`). **Components**: 2, each with 2
releases (`1.0.0`, `2.0.0`), both linked to the product release. Every one of the 5
resulting releases (1 product + 4 component) gets exactly 2 collection versions: version 1
references an SBOM artifact's version 1; version 2 references that *same* artifact's
version 2 (`updateReason.type: "ARTIFACT_UPDATED"`) — exercises artifact versioning and
collection versioning together, in the smallest shape that does both.

**Expected on import**: succeeds, creates 1 product, 1 product release, 2 components, 4
component releases, 5 distinct artifacts (one per release, each with 2 versions), 10
collections. Re-importing is a safe no-op — `AlreadyExisted` accounts for everything,
`Created` for nothing.

## Dataset 2: Complex

**Product**: "Acme MegaSuite", 3 releases (`1.0.0`, `2.0.0`, `3.0.0`) — deliberately kept
as **one bundle, one product**, not several cross-referencing ones: `internal/bundle`'s
dedup is exact-match-or-hard-error (`ErrImportIdentityConflict`) on a UUID collision with
differing data, never a silent merge, so splitting shared entities across bundles would
risk exactly that error for no benefit here.

**Components**: 10, each with 10 releases — 100 component releases. Product releases link
increasing coverage: `1.0.0` pins 6 components' earliest releases; `2.0.0` pins 8
(components 1–6 bumped to updated releases, 7–8 newly added, **but not every bump** — a
few component releases are pinned unchanged from `1.0.0`, deliberately, so the same
component release's collection is reachable from two different product releases); `3.0.0`
pins all 10, again with some releases carried over unchanged.

**Shared artifacts**: a `LICENSE`-type statement appears in literally every collection in
this bundle (100+ places) — the main "artifact used in multiple collections" case. A
second shared `BOM`-type "base image SBOM" appears in roughly half of the component
releases' collections (the ones modeling components that share a common base image).

**Collection version history**: product release `1.0.0`'s own collection gets 4 versions
(`INITIAL_RELEASE` → `VEX_UPDATED` → `ARTIFACT_ADDED` → `ARTIFACT_REMOVED`) — the
"multiple collections with different version numbers" case, at the product-release level
(the simple dataset already covers this shape at a smaller scale; existing hand-written
fixtures like `testdata/fixtures/edge-cases.json` cover it at component-release level —
this is deliberately the product-release variant, for coverage breadth).

**Multi-format artifact**: one `CERTIFICATION`-type "Regulatory Compliance Certificate"
with two formats on the *same* artifact — `application/pdf` and `application/msword` —
included in release `3.0.0`'s collection.

**File content**: most artifacts are `url`-only (no SHA-256 checksum, no `files/` entry —
confirmed this passes schema validation and `bundlecheck` cleanly, since
`export.go`'s `collectSHA256` only ever pulls a checksum into `files/` when it's SHA-256).
Exactly 2–3 artifacts get real embedded content (one SBOM, the certificate's PDF format) —
enough to exercise the embedding mechanism without the zip needing hundreds of fake files.

**Expected on import**: succeeds, creates 1 product, 3 product releases, 10 components,
100 component releases, 100+ collections. Import is idempotent despite the scale and the
reused-unchanged component releases — re-importing is still a safe no-op in full.

## Dataset 3: Lifecycle & compliance

**Product**: "Acme Lifecycle Device", 2–3 releases. **Components**: 2–3, a couple of
releases each — modest size; this dataset is about breadth of CLE/compliance coverage, not
scale.

**CLE**: one event of every `cleEvent.type` value (`released`, `endOfDevelopment`,
`endOfSupport`, `endOfLife`, `endOfDistribution`, `endOfMarketing`, `supersededBy`,
`componentRenamed`, `withdrawn` — all 9), spread across all four entity levels the `cle`
additive property exists on (product, product release, component, component release),
plus at least one `cle.definitions.support[]` entry.

**Compliance documents**: a `COMPLIANCE_DOCUMENT` identifier on a component *and* a
component release (TEA 1.0 restricts this identifier type to those two entity kinds only —
not products, product releases, or CLE events), using real `compliance-document-type`
enum values (e.g. `ISO_27001`, `SOC_2_TYPE_II`). One real `CERTIFICATION`/`ATTESTATION`
artifact with embedded content represents an actual uploaded compliance document, tying
the bare identifier reference to real content rather than leaving it as metadata alone.

**Expected on import**: succeeds, every CLE event type and both compliance identifiers
round-trip exactly as authored.

## Bad bundles (dataset 4, half A: must be rejected)

Each breaks exactly one thing, so a failure can be attributed precisely. All are tiny —
one product, minimal everything else — since the defect, not the scale, is the point.

| File | Defect | Expected `bundlecheck`/import outcome |
|---|---|---|
| `bad-checksum-mismatch.zip` | A `files/<sha256>` entry's real content doesn't match its own filename's hash | Rejected: hash mismatch, named file |
| `bad-missing-file.zip` | An artifact-format's SHA-256 checksum has no corresponding `files/` entry at all | Rejected: missing file for referenced checksum |
| `bad-format-version.zip` | `manifest.json`'s `formatVersion` is absent or not `"1.0"` | Rejected: schema validation failure |
| `bad-dangling-component-release.zip` | A product release's `components[].release` names a component-release UUID absent from the top-level `componentReleases[]` array | Rejected: dangling cross-reference |
| `bad-dangling-component.zip` | A product release's `components[].uuid` names a component UUID absent from the top-level `components[]` array | Rejected: dangling cross-reference |

Plain `POST /admin/v1/products/import` (no special parameter) and standalone `bundlecheck
bad-*.zip` must both reject every one of these, each with a distinct, identifiable error —
not a generic "import failed." None of this needs any code change; it exercises
`internal/bundle`'s existing validation exactly as documented in `docs/bundle-format.md`'s
own "Validation on import" section.

## Force-import (dataset 4, half B: made to land anyway)

The rejection half above proves the *importer* catches bad data. It doesn't prove a
*client* reading `/tea/v1` would catch the same bad data if it somehow ended up stored
anyway — a real possibility regardless of how careful this importer is (a different,
buggier exporter; a direct database edit; a future bug in this very validation). Testing
that requires a way to deliberately get bad data into a real server's storage on purpose.

**New, explicitly test-only mechanism**: `POST /admin/v1/products/import?force=true`
bypasses the schema-validation-first and file-hash-integrity checks, keeping the same
entity-creation logic otherwise — so `bad-checksum-mismatch.zip`, forced in, leaves the
server storing an artifact-format whose claimed checksum genuinely does not match the
bytes actually stored under it; `bad-missing-file.zip`, forced in, leaves a checksum with
no backing content to download at all.

This is gated behind a new config flag, **off by default**, checked *before* the `force`
parameter is even consulted — absent the flag, `force=true` is rejected outright
regardless of admin role:

| Env var | Default | Notes |
|---|---|---|
| `TEA_ALLOW_UNSAFE_IMPORT` | `false` | Test-tooling only. Never set this on a real deployment — it exists so this rig's bad datasets can be force-imported into a disposable test server for `docs/consumer-api-conformance-test-rig.md`'s client-side suite to read back, nothing else. |

**Expected**: with the flag unset, `force=true` is rejected the same as if it weren't
present at all — a stray query parameter must never be enough on its own. With the flag
set on a disposable test instance, `bad-checksum-mismatch.zip` and `bad-missing-file.zip`
both import successfully despite their defects. What happens next — a client detecting the
inconsistency on read — is `docs/consumer-api-conformance-test-rig.md`'s own payoff, not
this document's; this document's job ends at "the bad data is now really there."

## Regenerating

```bash
go run ./cmd/testbundlegen -out testdata/bundles/
bundlecheck testdata/bundles/simple.zip testdata/bundles/complex.zip testdata/bundles/lifecycle-compliance.zip
# bad-*.zip are expected to report INVALID -- that's their whole purpose, not a build failure.
```
