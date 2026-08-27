# TEA Publisher — protocol and service design

**Status:** draft v0.9, for discussion. Nothing here is scheduled or approved; no
implementation exists yet. This document is the design opentea's `TODO.md` "Reference
publisher" entry has been blocked on since 2026-07-04.

**See `design/publisher-openapi.yaml`** for the actual OpenAPI 3.1 draft of the protocol
sketched in §8 below — this narrative document and that spec are meant to be read
together and kept in sync; the YAML file's own header explains its provenance (the old
`CycloneDX/transparency-exchange-api`'s `spec/publisher/` draft, superseded, plus oej's
`tea-trust-architecture` workflow document, followed) in more implementation-level detail
than repeated here.

**Revision history:**

- v0.1 designed a separate service that called opentea's existing `/admin/v1` (the
  object-model CRUD "explicit stand-in" `TODO.md`'s **Publisher API** entry describes).
  Wrong target — `/admin/v1` is opentea-specific, has no staging, and no atomicity between
  creating a collection and attaching its evidence.
- v0.2 corrected the target but overcorrected the architecture: it made `/publisher/v1` a
  new API *implemented by opentea itself*, with the publisher GUI folded into the same
  process. **Also wrong** — per direct correction: *"this is not a function of our
  existing opentea server, it is a new server that is supposed to be able to publish to
  any tea service using a standard protocol."*
- **v0.3 (this revision)** separates the two things that were tangled together in v0.2:
  a **protocol** (a standard write/publish API any conformant TEA server can implement —
  the actual missing piece `TODO.md`'s **Publisher API** entry names) and a **publisher
  server** (a new, standalone service — its own binary/deployment, own GUI, own staging
  state, own signing — that is a *client* of that protocol, able to target any server that
  implements it, not just opentea). This mirrors the read side's existing shape exactly:
  `spec/openapi.yaml` defines `/tea/v1` as a protocol any TEA server implements; `pkg/teaclient`
  + `cmd/teaclient` is the reference *client* of it, explicitly built to work "against any
  conformant TEA server, not just this repo's own reference server." The publisher is the
  same relationship, write side: a protocol, and a reference client of it — except the
  client here is a full service with a GUI, not a CLI.
- v0.3 also incorporates oej's `tea-trust-architecture` publisher workflow document
  (`tea-trust-arch/publisher/publisher-workflow.md`), read in full for this revision — see
  §5. It's the closest thing to a normative source this design has, and reshapes the
  workflow phases (§7) and adds two streams v0.1/v0.2 both missed entirely: CLE lifecycle
  and compliance documents.
- **v0.4 (this revision)** turns §8's sketch into an actual OpenAPI 3.1 draft,
  `design/publisher-openapi.yaml`, informed directly by reading
  `CycloneDX/transparency-exchange-api`'s own `spec/publisher/openapi.json` (the older,
  materially different draft the user pointed at: flat `tea_product`/`tea_leaf`/
  `tea_collection`, no Component concept, mutable collections via `PATCH`/`DELETE`, no
  separated artifact-vs-collection signing — superseded by the current spec's object model
  and by oej's workflow document, not followed). Also resolved one of v0.3's open
  questions in passing: compliance documents (§7.6) turn out to already have a home in the
  current spec as-is — `pkg/tea/enums.go`'s `IdentifierTypeComplianceDocument`
  (`"COMPLIANCE_DOCUMENT"`) — found while grounding the OpenAPI schemas in opentea's actual
  wire types, not a new invention.
- **v0.5 (this revision)** rewrote the OpenAPI's schemas against the actual verbatim
  `spec/openapi.yaml` (per an explicit requirement: base the API on the existing consumer
  objects, document every deviation with a reason — see that file's own "Base objects and
  deviations" section) rather than a loose Go-type-derived reconstruction. Also settled
  signing (§9, fully rewritten): the manufacturer's key is used inside the publisher
  software, never the target server, enforced by a prepare/sign/submit shape now applied
  symmetrically to *both* artifacts and collections (v0.4 only had it for collections);
  and simplified collection drafts to a singleton per owning release with no separate id
  (§7.7), prompted by confirming that a collection's UUID always equals its owning
  release's — already true in the consumer spec, not something this design needed to add.
- **v0.6 (this revision)** replaces §10 "Roles / actors" with a full **Authorization**
  section, splitting what was one vague topic into five distinct relationships (§10.1–10.5)
  that were being silently conflated: manufacturer staff ↔ publisher software, publisher
  software ↔ target server, approval enforcement, CI/CD credentials, and multi-target
  credential management. The firmest new decision: Layer A (staff login) must integrate
  with existing enterprise identity systems (OpenID Connect generally, direct LDAP/AD bind
  as a distinct option for on-prem AD without an OIDC front end) rather than reusing
  opentea's own bespoke local-account model — explicitly *not* the same shape as `/tea/v1`'s
  bearer-token authentication, per direct correction. Several sub-questions (bare OAuth2 vs.
  OIDC-only, LDAP/AD-bind's actual necessity, multi-IdP support, SAML) remain genuinely
  open, listed in §10.1 rather than resolved by assumption.
- **v0.7 (this revision)** adds §9.5, a second signing mode alongside §9.1–9.3's
  ephemeral-software-key one: Web PKI certificates, possibly HSM/PKCS#11-backed, possibly
  fully air-gapped. The key realization: `prepare`'s existing response already *is* the
  to-be-signed package needed for an offline/air-gapped ceremony — nothing new there — but
  a collection draft's mutability means it needs an explicit **lock**, held from
  `prepareCollectionCommit` until `commitCollectionDraft` succeeds or a new
  `cancelPrepare` operation releases it, so a signature that takes days to come back
  doesn't silently go stale against a draft that changed underneath it. Also surfaced two
  things §9.1–9.3 had implicitly hardcoded to Mode 1 without noticing: certificate
  verification needs a genuinely different code path (X.509 chain validation against a
  trust store, not `internal/trust`'s ephemeral-cert fingerprint matching), and the
  signature algorithm isn't necessarily Ed25519 once the certificate is Web PKI rather
  than opentea's own self-signed scheme.
- **v0.8 (this revision)** adds §13, a signed-webhook eventing model adopted from oej's
  separate `event-proposal/publisher-events.md` (read in full). Two real changes to
  existing sections, not just an addition: §9.5's air-gapped signing flow gains an actual
  push trigger (`collection.readyForSigning`'s `signingPayloadUrl`, pointing at the same
  TBS package §9.5 already produces, instead of relying on polling or a human remembering
  to check), and §7.9's approval step is reframed from a flat `approvedBy` field on
  `commit`'s request body into a real async workflow (`approval.required` →
  `granted`/`rejected`) needing new, not-yet-designed `approve`/`reject` operations. A
  category-by-category mapping (§13.3) checks the proposed events against every operation
  this design already has — the close fit across most rows is a reasonable sanity check on
  §7/§8's shape. Not yet reflected in `design/publisher-openapi.yaml`.
- **v0.9 (this revision)** designs the `approve`/`reject` operations §7.9/§13.3 had left
  as a named-but-undesigned gap, and reflects them in
  `design/publisher-openapi.yaml`: `collection-draft` gains `revision` (bumped on every
  `putCollectionDraft`, which now also resets any prior `approval`) and `approval`
  (`status`/`decidedBy`/`decidedAtRevision`/`comment`). Approval is enforced exactly once,
  at `prepareCollectionCommit` (409 without a current, matching approval) — not at
  `approve` itself (records a decision, nothing more) and not at `commitCollectionDraft`
  (only ever reached after prepare already checked). Maker-checker is a real, checked
  rule (403 on `actor == draftedBy`), not a convention. `commit-request` — the schema
  that used to carry `approvedBy` — is gone; with approval moved out, it was identical to
  `evidence-submission`, which `commitCollectionDraft` now takes directly.

## 1. Problem statement

Three things don't exist today:

- **A standard write protocol.** TEA has no publisher/write API in the official spec yet
  (post-1.0 per the spec's own roadmap — `TODO.md`'s **Publisher API** entry). Every TEA
  server that accepts writes today does so its own proprietary way; opentea's is
  `/admin/v1`, explicitly labeled a stand-in, not a spec. A publisher tool that wants to
  target more than one TEA server implementation has nothing standard to speak.
- **Staging and atomic, evidence-bound commit.** Assembling a release happens over time
  (an SBOM today, a VEX document next week); nothing in the TEA object model or any
  existing write API represents "in progress." And creating a collection and attaching its
  evidence are two separate calls wherever evidence-bundle support exists at all (opentea's
  `POST .../evidenceBundle`, `internal/admin/evidencebundle.go`) — nothing prevents a
  collection existing, live, unsigned.
- **A usable, process-shaped way for a human to actually do this.** No GUI exists anywhere
  in this space today; the closest thing is hand-assembling JSON against `/admin/v1`.

This document designs both halves: the protocol (§8), and the publisher server that speaks
it (§4, §7) — a new, standalone service, not a mode of opentea.

## 2. Goals

- A standard **publish protocol** any TEA-conformant server can implement — analogous to
  what `spec/openapi.yaml` already is for reads — so a publisher tool isn't locked to one
  server implementation.
- A **new, standalone publisher server**: its own service, own GUI, configurable to target
  any server implementing that protocol (opentea's own eventual implementation of it is
  one target among possibly several, not a dependency this design is built around).
- The GUI/workflow organized around the manufacturer's process, not the TEA object model.
- Real staging: an in-progress collection (and, per §7, in-progress artifact/CLE/compliance
  streams) that exists on the *publisher's own* server, independent of any target TEA
  server, until deliberately published.
- Atomic, evidence-bound publication: the protocol itself guarantees a collection can't
  become live on a target server without its evidence, not just "the publisher server tries
  to do both and hopes."
- Support for the actual signing model real publishing pipelines use: CI/CD signs
  artifacts as they're built (§5, §7.4); a separate, often human-gated, signature covers
  the assembled collection at publish time (§7.8).
- The routines asked for: add a product, add a product release, work with components,
  create and commit collections — expanded, per §5, to include CLE and compliance-document
  handling, which oej's own workflow document treats as equally first-class streams.

## 3. Non-goals (v1)

- Not part of opentea. opentea is, at most, one reference *implementation* of the target
  protocol (§8) — a separate, later piece of work, not bundled with building the publisher
  server itself. Nothing in the publisher server's design should assume opentea-specific
  behavior beyond what the protocol itself defines.
- Not multi-tenant in the sense of serving multiple unrelated manufacturers from one
  publisher deployment — one publisher instance represents one manufacturer/trust domain
  (it may still, per §7, target several different TEA servers for that one manufacturer).
- Not committing to all three transparency-log systems (Rekor/Sigsum/SCITT) on day one.
- Not an offline/air-gapped signing ceremony — later phase at the earliest.
- Not a general-purpose SBOM/VEX *authoring* tool, and not a CI/CD system — per §5/§7.4,
  artifact preparation and signing is CI/CD's job; the publisher server validates and
  stages what CI/CD hands it, it doesn't generate SBOMs or run builds itself.

## 4. Architecture

```
┌────────────────────┐     ┌──────────────────────────┐
│  CI/CD pipeline       │     │  Manufacturer staff        │
│  (builds, signs           │     │  (browser)                    │
│  artifacts)                 │     └──────────────┬───────────────┘
└──────────┬───────────┘                    │
           │  submits signed artifacts               │ GUI
           ▼                                             ▼
┌─────────────────────────────────────────────────────────────┐
│  Publisher server  (new, standalone -- own binary/deployment)   │
│                                                                    │
│  - GUI (process-shaped, §7)                                        │
│  - staging: draft collections, in-progress CLE/compliance streams   │
│  - validates artifact evidence submitted by CI/CD                    │
│  - drives collection-level signing at publish time                    │
│  - own DB (staging state, audit trail -- distinct from any target)     │
│  - own auth (manufacturer staff sessions; separate from any target's)   │
└───────────────────────────┬─────────────────────────────────────┘
                             │  TEA Publisher API (the protocol, §8)
                             │  -- HTTP, target configurable per publish
                             ▼
              ┌──────────────────────────────┐
              │  Target TEA server               │
              │  (opentea, or any other            │
              │   conformant implementation)         │
              │                                        │
              │  /tea/v1   (existing, unaffected)        │
              │  /publisher/v1  (new -- the protocol,     │
              │   this server's implementation of it)      │
              └──────────────────────────────┘
```

Key relationships:

- The **publisher server** is the thing actually being asked for: a new service with a
  GUI. It is a *client* of the protocol (§8), never a client of any target's proprietary
  admin API (opentea's `/admin/v1` included) — that's the mistake v0.1 made.
- **opentea implementing the protocol** (its own `/publisher/v1`, reusing
  `internal/repo`/`internal/trust` underneath, much like v0.2 sketched) is real, valuable,
  *separate* work — it's what makes opentea a usable target for testing/using the
  publisher server, but it is not part of "build the publisher server" and isn't a
  prerequisite for finishing this design. §8 specifies the protocol precisely enough that
  either piece can proceed independently once it's settled.
- **CI/CD is a first-class actor**, not a footnote — per oej's workflow document (§5), it's
  the typical signer of individual artifacts, submitting already-signed evidence to the
  publisher server directly (a plain HTTP client, no GUI involved), the same way a human
  does through the GUI for collection-level actions.
- The publisher server needs **its own persistence and auth**, independent of any target —
  it stages data (drafts, in-progress CLE/compliance work) that may target a server that's
  down, unreachable, or simply not yet chosen, and manufacturer staff need to authenticate
  to *it*, not to whichever TEA server they happen to be publishing to today.

## 5. What oej's publisher-workflow.md establishes

Read in full for this revision (`tea-trust-arch/publisher/publisher-workflow.md`,
status "Draft / Informational," explicitly stating *"The publisher OpenAPI specification
will define the normative behavior"* — i.e. it's the mental model this design's protocol
(§8) needs to formalize, not itself a spec). Core points that reshape this design:

- **Stable release identity vs. versioned publication streams.** A `productRelease`/
  `componentRelease` is stable once created. Collections, artifacts, CLE, and compliance
  references all evolve *independently* of that identity and of each other, each with its
  own version sequence. v0.2 already had this right for collections; it's now explicit for
  all four streams.
- **Immutability of published versions.** Once published, a collection/artifact/CLE
  version is never modified — updates are new versions. This is already exactly how
  opentea's own object model works (confirmed: no update path exists today for any of
  these — `TODO.md`'s ETag section notes this immutability assumption directly), so the
  protocol doesn't need to invent anything new here, only preserve it.
- **Two separate signing events, not one.** This is the biggest correction to v0.2, which
  only modeled collection-level signing:
  1. **Artifact Creation** (oej's phase 2) — CI/CD signs and timestamps each artifact
     individually, as it's built. This is the *typical*, expected path — not a human
     clicking "sign" in a GUI.
  2. **Artifact Validation** (phase 3) — the publisher server verifies what CI/CD
     submitted (signature integrity, certificate validity, timestamp plausibility,
     transparency inclusion) and stages it. This is exactly opentea Trust Architecture
     Phase 1's existing verify-before-store logic (`internal/trust.Verify`,
     `internal/admin/evidencebundle.go`) — the protocol needs the equivalent operation,
     just scoped to an artifact submitted by CI/CD rather than a collection submitted by a
     human.
  3. **Collection Signing** (phase 5) — a *second*, independent signature, over the
     assembled collection, after **Collection Assembly** (phase 4) references the
     already-validated artifacts (by digest). This is v0.2's `prepareCommit`/`commit` design
     (§7.8 here) — still correct, just now clearly scoped as the *collection's* signature,
     not artifacts'.
- **CLE and compliance documents are first-class evolving streams**, same standing as
  collections. Neither appeared anywhere in v0.1/v0.2 despite opentea already having full
  server-side CLE support (`internal/admin`'s `createCLEEventForOwner`/
  `createCLEDefinitionForOwner`, `pkg/tea.CLEEvent`/`CLESupportDefinition`) that a real
  publisher needs to expose. Compliance documents have no dedicated modeling in opentea
  today — oej's own document says they may be "represented as artifact," which fits
  opentea's existing `ArtifactTypeCertification`/`ArtifactTypeAttestation` types
  (`pkg/tea/enums.go`) reasonably well without inventing a new object type (open question,
  §11).
- **Approval is its own phase** (6), separate from both signing (5) and commit (7) — a
  human review step that gates publication but isn't itself a cryptographic operation.
- **Commit can span more than one collection.** Oej's phase 7 lists "publishing collection
  versions, publishing artifact versions, publishing CLE updates, updating DNS trust
  anchors" together, not "publish this one collection." Whether the protocol's commit
  operation should be able to batch multiple streams' updates in one atomic call, or stays
  scoped to one collection at a time (simpler, matches v0.2's design) is now an open
  question (§11) rather than a settled point — v0.2 had implicitly settled it without
  noticing there was a question.

## 6. Real-world scenarios (from oej's document, kept as design test cases)

Each of these should be expressible as a sequence of protocol operations (§8) without
needing a new operation invented per scenario — a good check on §8's completeness:

1. **Missing artifact added later.** Collection v1 ships firmware only; v2 adds an SBOM
   that wasn't ready at v1's publish time.
2. **VEX update.** Collection v1 has no vulnerability data; v2 adds a VEX document, no
   change to anything else in the release.
3. **SBOM correction.** A new artifact version (not a new collection version on its own)
   is published for the corrected SBOM; a subsequent collection version references it.
4. **Compliance update.** A compliance reference (e.g. an ISO 27001 identifier or
   certificate artifact) is added in a later collection version.
5. **Lifecycle change.** CLE moves from `active` to `end-of-life` — a new CLE version,
   entirely independent of any collection publish.

## 7. The manufacturer process, mapped to protocol operations

Restructured around oej's seven phases (§5), with opentea-flavored operation names for
concreteness (§8 is the authoritative shape; names here are illustrative).

### 7.1 Add a new product / 7.2 Add a new product release

Unchanged from v0.2 in substance — `product`/`productRelease` are the "stable release
identity" (§5). `CreateProduct`, `CreateProductRelease` protocol operations, staged
nowhere (creating one is already a complete, meaningful action).

### 7.3 Work with components

Find-or-create search, link (optionally pinned to a component release) — same as v0.2's
§7.3, unchanged.

### 7.4 Artifacts: preparation, creation, and validation (oej phases 1–3)

- **Preparation** (phase 1) happens entirely outside this design's scope — CI/CD builds,
  generates SBOM/VEX, computes digests. Not a protocol operation.
- **Artifact Creation** (phase 2): `POST /artifacts` (metadata) + `POST .../files`
  (content) — the artifact now exists, unsigned, with server-assigned `uuid`/`version`.
- **Prepare** (new in this revision, §9.2): `POST .../evidence/prepare` — the target
  server, the sole authority on the artifact's exact canonical form, returns a digest of
  it. Not oej's own phase numbering; a mechanical necessity this design adds so CI/CD never
  has to reimplement RFC 8785 canonicalization itself (§9.2 has the full reasoning).
- **Sign**, in the publisher software, against that digest (§9.1) — not a protocol call.
- **Artifact Validation** (phase 3): `POST .../evidence` — submit the signature and
  certificate (the evidence package, §9.3). The target server re-derives the digest from
  current state and verifies (§9.1 step 3) — mirrors opentea's existing
  `internal/trust.Verify` + digest-recomputation check
  (`internal/admin/evidencebundle.go`) — before storing. Typically called by CI/CD
  directly, no GUI involved. A submission that fails verification is rejected, not stored
  unverified. Once stored, the artifact is ready to be referenced by a collection (§7.7).

### 7.5 Manage lifecycle (CLE) — new in this revision

CLE events (`released`, `endOfSupport`, `endOfLife`, `withdrawn`, etc. —
`pkg/tea/enums.go`'s `CLEEventType*`) and support-policy definitions, against a product,
release, component, or component release. Independent of collections and of each other's
publish timing (§5) — a lifecycle change doesn't need a release or a collection publish
alongside it. Protocol operations mirror opentea's existing
`createCLEEventForOwner`/`createCLEDefinitionForOwner` shape closely enough that this is
likely close to a direct pass-through once a target implements §8, not a new design
problem.

### 7.6 Compliance documents

Resolved in v0.4: `pkg/tea/enums.go` already defines `IdentifierTypeComplianceDocument`
(`"COMPLIANCE_DOCUMENT"`) — a compliance reference is an `Identifier`, attachable to a
product/release/component/component-release the same way a `CPE`/`PURL`/`TEI` identifier
is, not a distinct protocol concept and not (necessarily) an artifact. A compliance
*document* with real file content still goes through the artifact path (§7.4) using an
appropriate existing `ArtifactType` (`CERTIFICATION`/`ATTESTATION`) when there's an actual
file to sign and validate, matching oej's own "or represented as artifact" framing — the
two aren't mutually exclusive, and neither needed inventing.

### 7.7 Collection assembly (draft) (oej phase 4)

Start a draft against a release, set/replace its artifact list (a list of already-
*validated*, per §7.4, artifact references), review with a diff against the release's
current live collection.

**A draft needs no identity of its own to allocate.** The consumer spec is explicit that a
collection's `uuid` "matches the UUID of the associated TEA Component Release or TEA
Product Release object" — confirmed directly against `spec/openapi.yaml`'s own `collection`
schema, and reflected in this draft's `collection.uuid` description (`design/publisher-openapi.yaml`).
A new collection version never gets a fresh UUID at all — the *same* UUID persists across
every version of a given release's collection, only `version` increments. There is
therefore nothing to reserve or request ahead of time: the eventual collection's identity
is already fully known the moment its owning release is (from the path parameter alone),
and a draft is naturally a **singleton per owner**, addressed directly by the owning
release's UUID — not a separately-allocated resource with its own id. (§8's `collectionDraft`
path reflects this: singular, no synthetic draft-id parameter, replacing the earlier
`/collectionDrafts/{draftId}` shape.) This also folds `createCollectionDraft` and
`setCollectionDraftArtifacts` into one idempotent `PUT` — there's no separate "start" step
worth keeping once there's no id for it to hand back.

### 7.8 Collection signing (oej phase 5)

v0.2's `prepareCommit` step, reframed as specifically the collection's own signature, kept
separate from artifact signing (§7.4) per §5's correction: the server (publisher or,
eventually, target — see §8) builds the canonical collection object and its digest;
whoever holds the collection-signing key (§9) signs it. Two-phase so no key custody
decision is baked into the protocol itself.

### 7.9 Approval (oej phase 6)

A human review/approval step, gating commit, separate from both signing steps above. Scope
of enforcement (publisher-only vs. protocol-enforced) is open (§11) — resolved in favor of
protocol-enforced by the design immediately below, though whether that's the *right* call
long-term is still fair to revisit.

**Reshaped by §13's event model into a real async workflow, not just a request-body
field — now designed in `design/publisher-openapi.yaml`.** The earlier assumption —
`commit`'s `approvedBy` field, checked or not per §10.3 — treated approval as something
already decided by the time `commit` is called. oej's separate event-delivery proposal
(§13) implies a cleaner shape, now built: `POST .../collectionDraft/approve` and
`.../reject` record a decision against the draft's *current* `revision` (a monotonic
counter, bumped on every `putCollectionDraft`, which also resets `approval` back to
`none` — editing invalidates whatever was approved before). Maker-checker is enforced,
not advisory: `approve`/`reject`'s `actor` must differ from the draft's own `draftedBy`,
both asserted by the publisher software from its own Layer A session (§10.1) — a
self-approval is rejected with 403, not merely discouraged. The actual enforcement point
is `prepareCollectionCommit`, not `approve` or `commitCollectionDraft`: prepare now
requires a current, matching approval to succeed at all (409 otherwise) — the draft is
locked immediately afterward (§9.5), so by the time signing and `commitCollectionDraft`
happen, approval has already been checked once and doesn't need checking again.
`commit-request` (the schema that used to carry `approvedBy`) is gone —
`commitCollectionDraft` takes `evidence-submission` directly now, since nothing was left
to add on top of it.

### 7.10 Commit (oej phase 7)

Publish to the chosen target server: at minimum, the signed collection plus its evidence,
atomically (§8). Whether this single commit action can also carry along newly-validated
artifact versions and pending CLE updates in the same atomic call, per oej's phase 7
listing them together, is open (§11) rather than settled.

## 8. The protocol (sketch)

This is the part that needs to be genuinely standard — implementable by opentea and by
any other conformant TEA server, the same way `spec/openapi.yaml` is. Sketched here at the
same illustrative level v0.2 sketched opentea-specific endpoints at; turning this into an
actual OpenAPI document is real, separate follow-on work once the shape below is agreed.

Conventional mount point: `/publisher/v1` (mirrors `/tea/v1`'s own convention — every
conformant server that implements this uses the same path, the way every conformant
server already serves `/tea/v1` at that path today).

- `POST /publisher/v1/products`, `POST .../products/{uuid}/releases` — stable-identity
  creation (§7.1–7.2).
- `GET /publisher/v1/components`, `POST /publisher/v1/components`,
  `POST .../productReleases/{uuid}/components` — component find/create/link (§7.3).
- `POST /publisher/v1/artifacts/{uuid}/{version}/evidence/prepare`,
  `POST .../evidence` — artifact validation (§7.4, §9.2): prepare returns a digest to sign
  over the artifact as it currently stands; submit carries the resulting evidence package
  (§9.3) — signature/certificate, and eventually timestamp/transparency-log data — which
  the target verifies and stores, mirroring opentea's existing `POST .../evidenceBundle`
  contract (`internal/admin/evidencebundle.go`) but under the standard path, split into
  two calls, and callable directly by CI/CD, not only by the publisher server's own
  GUI-driven flow.
- `POST /publisher/v1/products/{uuid}/cle/events`,
  `POST .../products/{uuid}/cle/definitions` (and the release/component/component-release
  equivalents) — lifecycle (§7.5), mirroring opentea's existing shape closely.
- `PUT /publisher/v1/productReleases/{uuid}/collectionDraft`,
  `GET`/`DELETE` on the same path — collection assembly (§7.7). Singular, no synthetic
  draft-id: per §7.7, a collection's identity is always its owning release's UUID, already
  known from the path, so there's nothing to allocate and at most one open draft per owner
  exists by construction, not by a separately-enforced constraint. `PUT` both creates (if
  none exists) and replaces the artifact list — no separate "start a draft" call, unlike
  v0.3's `POST .../collectionDrafts`. **Open question (§11):** does staging live on the
  *target* server (as sketched here) or entirely on the *publisher* server, with only the
  final commit ever reaching the target? The latter would remove this resource from the
  protocol entirely, in favor of the publisher holding all draft state itself and only
  ever calling the target at commit time — simpler protocol, more state the publisher
  server has to reconcile if a draft targets a server that later becomes unreachable.
- `POST .../collectionDraft/prepareCommit`, `POST .../collectionDraft/commit` — collection
  signing + atomic publish (§7.8, §7.10, §9): prepare returns a digest to sign, commit
  takes the resulting evidence package and atomically creates the collection and its
  evidence bundle, verified server-side, in one transaction (reusing the transaction-
  composition pattern `internal/repo/repo.go`'s `WithTx` already provides in opentea
  specifically, and something any implementer needs an equivalent of).

## 9. Signing and the evidence package

**Settled by explicit direction:** the manufacturer owns the private key, and signing
happens **in the publisher software** — not in a browser, not delegated to an external
agent by default, and never, under any circumstance, in the target TEA server. This
section works out what that requires of the protocol (§8) precisely, rather than leaving
it as an open question the way v0.3 did.

### 9.1 The core guarantee, and how it's actually enforced

*"The TEA service has no access to signing keys"* has to be true by construction, not by
policy — a target server that's merely asked nicely not to look at a key isn't actually a
guarantee. The mechanism that makes it structurally true, already present in §7.8/§8's
`prepareCommit`/`commit` split for collections, generalizes to **every** signing operation
in this protocol:

1. **Prepare.** The target server builds the canonical form of the object *as it currently
   stands* — never persists it — and returns a digest of that canonical form. The server
   does this because it, not the caller, is the authority on the object's exact canonical
   shape (see §9.2 for why that matters). No key material appears anywhere in this step,
   in either direction.
2. **Sign.** Entirely inside the publisher software, using the manufacturer's key. The
   target server is not involved in this step at all — it isn't a network participant,
   isn't asked, doesn't know it's happening.
3. **Submit.** The publisher software sends back the signature and certificate. The target
   server **re-derives the digest itself**, from the object's current state (which may
   have changed since prepare — an upload could have landed in between), and only
   verifies the submitted signature against that freshly-recomputed digest — never against
   whatever the caller merely claims the digest was. A stale or mismatched prepare
   response is rejected here, not trusted.

At no point does a private key, or anything derived from one beyond a finished signature,
cross into the target server's process. This is the one alternative mechanism worth naming
explicitly and rejecting: a target server could instead generate an ephemeral keypair
*for* the manufacturer and hand back only the private key, once, never storing it — but
that still requires the key to exist inside the target's process momentarily, which is
exactly the exposure this design avoids by never generating the key there at all. Rejected
for that reason, not considered further.

### 9.2 Why artifacts need a prepare step too, not just collections

v0.3 only put the prepare/submit split on collection commit (§7.8), and let artifact
evidence (§7.4) be a single-step submission — reasonable-looking, since CI/CD already has
the artifact's file bytes locally and can compute a digest over them itself. That
reasoning has a real gap: what gets signed is not the raw file — it's the **canonical form
of the whole owning object** (mirrors opentea's actual verification code exactly:
`internal/admin/evidencebundle.go`'s `createEvidenceBundleForOwner` recomputes
`SHA256Hex(Canonicalize(owner))` over the real `tea.Artifact`/`tea.Collection`, not over
any uploaded file). An `artifact`'s canonical form includes its server-assigned `uuid`,
`version`, and `createdDate` — fields CI/CD cannot know in advance of creation, and fields
whose *exact* RFC 8785 JSON Canonicalization (field ordering, number formatting, Unicode
normalization) CI/CD would otherwise need to reimplement independently, correctly, forever
in lockstep with whatever the target server's own canonicalizer does.

That's a real, avoidable interoperability hazard — a canonicalization mismatch between an
independent client implementation and the target server's own would silently produce a
digest that never matches, no matter how correct the signature over it is. The fix is the
same prepare step collections already use, applied to artifacts too: the target server,
which is going to verify (and is the sole authority on) the canonical form regardless, is
also the one that hands the digest out in the first place. One canonicalizer implementation
in the loop — the target's own — not N independent reimplementations across every
publisher-software/CI-CD-integration that ever exists.

**Revised artifact evidence flow (replaces §7.4's single-step description):**

1. `POST /artifacts` (metadata) + `POST .../files` (content) — unchanged from §7.4/§8.
2. `POST /artifacts/{uuid}/{version}/evidence/prepare` — **new**. Server builds the
   canonical `artifact` object as it currently stands and returns its digest. Callable
   again if more format files are added afterward (each call reflects current state; there
   is no "prepare, then it's locked" semantic — only submit finalizes anything).
3. Sign, entirely in the publisher software (§9.1 step 2), against that digest.
4. `POST /artifacts/{uuid}/{version}/evidence` — submit. Server re-derives the digest from
   current state and verifies (§9.1 step 3) before storing.

Collections already have this shape exactly (§7.8's `prepareCommit`/`commit`); this
revision brings artifacts to the same shape rather than leaving them as the one place the
protocol asked a client to trust its own canonicalization.

**Related idea, not adopted here, worth a future look:** unlike a collection (§7.7 —
identity is the owning release's UUID, already known, nothing to allocate), a *new*
artifact's `uuid` is server-assigned at creation, which is exactly the reason it needs the
`prepare` round-trip above at all — if the *client* instead reserved the UUID up front (a
`POST /artifacts/reserveUuid`-style operation, returning identity with nothing created
yet), it could know every field of the artifact object in advance and compute the
canonical digest entirely locally, collapsing create + prepare + submit into a single
"submit the whole already-signed object" call. Not designed further here — it would be a
real behavioral change (server-assigned → client-chosen identity for a new object type)
worth its own discussion rather than folding in as a side effect of this section, but it's
the same underlying insight that settled §7.7's collection-identity question, applied to
the one remaining case where identity genuinely isn't known in advance.

### 9.3 The evidence package

"Evidence package" is not new terminology being introduced here — it's what
`pkg/tea/trust.go`'s `EvidenceBundle` (and this draft's `evidence-bundle` schema, §8)
already names: a signed object reference, a signature, a certificate, and (once Trust
Architecture Phases 2–3 exist) timestamps and transparency-log entries. What v0.4's
OpenAPI draft got wrong was treating "submit an evidence package for an artifact" and
"submit an evidence package for a collection" as two differently-shaped request bodies
(`evidence-submission` vs. the old `commit-request`) instead of one shared shape used both
places. Fixed in this revision (§8's schemas): `commit-request` now composes
`evidence-submission` (adding only `approvedBy`, which is specific to commit's approval
step, §7.9) rather than duplicating its fields — one evidence-package request shape,
reused everywhere a signature is submitted, not reinvented per resource type.

### 9.4 What's still unbuilt

RFC 3161 timestamp acquisition and transparency-log submission (Trust Architecture Phases
2–3, tracked in `TODO.md`) — the wire shape already has room for both
(`EvidenceTimestamp`/`EvidenceTransparency`, unchanged), and both the artifact-evidence and
collection-commit submit steps (§9.2, §7.8) are the natural place to add them once they
exist, but neither this design nor its OpenAPI draft is blocked on that landing first.

### 9.5 Two signing modes: online key vs. Web PKI / HSM / air-gapped

§9.1–9.3 quietly assumed one shape: prepare, sign, submit, all in quick succession, inside
one continuous session. That's true for a software key the publisher software holds
directly, but not for every real manufacturer signing setup — a real requirement is a
**second mode**: signing with a **Web PKI certificate** (CA-issued, long-lived, reused
across many signatures — not `internal/trust`'s current ephemeral self-signed model),
where the private key may live in an **HSM**, reachable either live (e.g. over PKCS#11, or
a network HSM/KMS API — still synchronous, just a different signer behind the "sign" step)
or **air-gapped**, with no live connection at all. The air-gapped case genuinely can't fit
the synchronous shape — signing there means physically moving data across the gap, and can
take anywhere from minutes to days, spanning multiple publisher-software sessions.

**The mechanism this needs already mostly exists — `prepare`'s response *is* the
to-be-signed (TBS) package.** Nothing new is required to produce it; what's missing is
treating it as something exportable and re-importable rather than only ever a transient
API response consumed synchronously in the same call chain:

1. `prepareArtifactEvidence`/`prepareCollectionCommit` (§8, unchanged) — the digest plus
   the full canonical object. For an air-gapped ceremony, this response is saved to a file
   and physically carried to the signing environment — the canonical object being included
   alongside the bare digest matters more here than in the online case: a real signing
   ceremony wants a human or tool on the air-gapped side to see *what* they're attesting to,
   not blindly sign an opaque hash. §13's `collection.readyForSigning` event, fired the
   moment `prepareCollectionCommit` locks a draft, is what turns this from something a
   human has to remember to go fetch into something the signing environment (or whatever
   coordinates it) is actively notified of — its `signingPayloadUrl` field points straight
   back at the TBS package described here, not a separate mechanism.
2. Signing happens on the air-gapped side, entirely outside this protocol's reach (as with
   §9.1's synchronous case, the target server was never a participant in signing itself —
   this mode just stretches the time and the physical distance between prepare and submit,
   not the trust boundary).
3. The resulting signature (+ certificate, + chain) is carried back and fed into
   `submitArtifactEvidence`/`commitCollectionDraft` exactly as in the synchronous case — no
   protocol change needed there either; those operations already re-derive the digest from
   current state and verify against it (§9.1 step 3), whenever they're actually called.

**What genuinely is new: locking a collection draft for the duration of an outstanding
signature.** A collection draft (§7.7) is mutable — `PUT .../collectionDraft/artifacts`
can be called again at any time. If a `prepareCommit` response has been exported for an
air-gapped ceremony that might take days, and the draft changes in the meantime, the
eventually-returned signature will no longer match at submit time and gets rejected
(correctly — but a wasted, possibly slow, human-involving round trip). `prepareCollectionCommit`
should therefore **lock** the draft against further `PUT` calls as a side effect of being
called at all (uniform behavior regardless of mode — harmless for the synchronous case,
where the lock is held only briefly anyway, and a real correctness improvement over
today's design even there, closing a small existing race between prepare and commit). A
new operation releases it: `POST .../collectionDraft/cancelPrepare` — abandons the pending
signature (not the draft itself) and re-opens it for editing. Artifacts need no equivalent
lock — an artifact's content is already effectively frozen the moment upload finishes,
before evidence is ever prepared (§7.4).

**Certificate verification is genuinely a different code path for this mode, not just a
different input.** `internal/trust.ParseCertificateSubject`'s current logic
(fingerprint-derived identity, checked against `used_fingerprint` reuse detection) is
built specifically for Mode 1's ephemeral, single-use, self-signed certificates. A Web PKI
certificate needs real **X.509 chain validation against a configured trust store**
(root/intermediate CAs the manufacturer's PKI actually chains to) — and reuse across many
signatures is the *normal*, correct case for a persistent certificate, not a violation
`used_fingerprint`-style detection should flag. A target server supporting this mode needs
its own trust-anchor configuration, entirely separate from anything Trust Architecture
Phase 1 built.

**Signature format follows from the mode, not from a fixed default.** `jws-detached` (the
only format `internal/admin/evidencebundle.go` actually verifies today) fits Mode 1's
lightweight, API-native software-key case well. `cms-detached` — already in
`internal/trust.SignatureFormat`'s vocabulary, not yet implemented anywhere — is the
natural fit for Mode 2: CMS/PKCS#7 detached signatures are what most HSM and PKCS#11
tooling, and most regulated/enterprise signing ceremonies, already produce natively.

**Open, not designed further here:**

- Should the TBS export have a standardized file format/extension (for interop across
  different publisher-software implementations and different HSM-side tooling), or is an
  ad hoc JSON file (the `prepare` response, saved as-is) sufficient for a first version?
- Should `cancelPrepare`'s lock auto-expire after some timeout, so an abandoned air-gapped
  ceremony (nobody ever explicitly cancels) doesn't leave a draft stuck indefinitely?
- Is Mode 2 support mandatory for every protocol implementer, or an optional capability a
  target server may or may not offer — and if optional, how does a publisher discover
  whether a given target supports it before attempting it?

## 10. Authorization

"Authorization" isn't one question here — it's (at least) five distinct relationships,
each with its own answer, that get silently conflated if treated as one:

### 10.1 Layer A: manufacturer staff ↔ publisher software

**Settled by explicit direction, and deliberately not the consumer API's model:**
`/tea/v1`'s bearer-token authentication (`spec/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md`
§6) is built for machine/API clients presenting a token; manufacturer staff need to log in
as themselves, against whatever identity system their organization already runs — the
publisher must be able to connect to existing enterprise auth (OAuth2/OpenID Connect,
Microsoft AD), not invent its own bespoke local-account system the way opentea's own
`internal/authn` does for its own admin GUI.

Concretely, this makes the publisher software a **pluggable auth front end**, not a
single-IdP integration:

- **OpenID Connect** is the actual general-purpose mechanism, not bare OAuth2 — OAuth2
  alone is an authorization framework and doesn't itself carry verified identity claims
  ("this is jane@manufacturer.com, member of these groups"); OIDC is the identity layer
  built on top of it. Any standards-compliant OIDC provider is in scope this way — Entra
  ID (Azure AD), Okta, Keycloak, ADFS-as-OIDC, Google Workspace, etc. — without needing
  provider-specific integration work beyond OIDC itself.
- **Direct LDAP/Active Directory bind (or Kerberos/SPNEGO)** as a separate, distinct
  mechanism, for a manufacturer whose AD has no OIDC front end at all — a genuinely
  different protocol from OIDC, not just "another IdP."
- Whatever comes back (OIDC ID token claims, or an LDAP bind's returned attributes/group
  memberships) is normalized into a single internal shape — subject identity, display
  name/email, group memberships — the same spirit as
  `TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md` §7's "Normalized Principal Context"
  for the consumer side, though that section doesn't cover this and isn't reused directly.
- A **configurable claim/group-to-role mapping** (per deployment, not hardcoded) maps the
  normalized principal onto the publisher's own three roles: release manager, component
  maintainer, security/compliance approver.
- **Maker-checker is a real, enforced rule, not just a role check**: whoever approves a
  commit (§7.9) must be a different verified identity than whoever built the draft being
  approved — meaningful specifically because Layer A now provides real, externally
  verified identity rather than a locally-created account anyone with database access
  could forge.

**Still open, not yet answered:**

- Is bare OAuth2 (without OIDC) needed for some real caller, or is OIDC-only sufficient
  for every case "OAuth2" was meant to cover?
- Does "Microsoft AD" mean Entra ID as an OIDC provider (no extra work beyond OIDC support
  itself), on-prem AD via LDAP/Kerberos (a distinct integration), or both?
- Does one publisher deployment need to support multiple IdPs configured simultaneously
  (a manufacturer with more than one identity source), or is one IdP per deployment
  enough for v1?
- Is SAML in scope alongside OIDC? Not mentioned in the original direction, but a common
  enterprise-IdP alternative worth ruling in or out explicitly rather than by omission.

### 10.2 Layer B: publisher software ↔ target TEA server

Unaffected by Layer A's decision — this is `/publisher/v1`'s own protocol-level auth
(`design/publisher-openapi.yaml`'s `bearerAuth`), a service-level credential the publisher
holds per target, not a manufacturer staff member's own identity. Currently flat: one
bearer token, undifferentiated scope — meaning a single leaked token could do anything the
protocol allows, from linking a component to committing a collection. Open: does the
protocol need a capability vocabulary tokens can be scoped to (mirroring how
`internal/authz` already names read capabilities for `/tea/v1`, e.g.
`artifact.evidence.submit`, `product.create`, `collection.commit`), or does v1 stay
all-or-nothing like `/admin/v1` is today, with scoping deferred? This question is sharpened
by Layer D immediately below, not answerable in isolation from it.

### 10.3 Layer C: approval enforcement

**Superseded by §7.9/§13's `approve`/`reject` design (v0.9) — settled protocol-enforced,
not advisory.** This section originally leaned advisory (a flat `approvedBy` field on
`commit`, trusted from the caller, not independently checked by the target) on the
reasoning that requiring every protocol implementer to be identity-aware was too much
weight for a first version. Once `approve`/`reject` existed as real, separate operations
recording a decision against tracked draft state (§7.9), enforcing it became cheap rather
than heavy: `prepareCollectionCommit` just checks whether a current, matching approval is
on record (409 if not) — the target doesn't need to independently verify *who* the
approver is beyond the `actor` string the publisher software asserts (Layer A, §10.1),
only that *some* recorded decision exists and isn't the same identity as the drafter. That
narrower, cheaper check is what made protocol enforcement the right call after all.

### 10.4 Layer D: CI/CD credentials

A CI/CD pipeline calling `submitArtifactEvidence`/`prepareArtifactEvidence` (§7.4)
headlessly needs its **own, narrowly-scoped credential** — one that can do only artifact
evidence submission, not create products or commit collections — so a credential leaked
from a build log or CI environment can't be used for anything beyond its actual job. This
is the concrete case that makes Layer B's capability-scoping question not just
theoretical: without some scoping mechanism, there is no way to issue a CI/CD credential
that's actually least-privilege.

### 10.5 Layer E: multi-target credential management

One credential per target TEA server the publisher is configured to publish to (§11.7's
multi-target question), held in the publisher's own state (§9's staging/audit storage).
Not a hard design problem, just a real operational surface worth naming rather than
assuming away.

## 11. Open questions

1. **Where does collection staging live** — target server (protocol operations, as
   sketched in §8) or publisher-server-only (simpler protocol, more reconciliation work
   for the publisher when a target is unreachable)?
2. **Does commit span multiple streams** (collection + artifact versions + CLE updates
   together, per oej's phase 7) or stay scoped to one collection at a time (simpler,
   v0.2's original assumption)?
3. ~~Compliance documents~~ — resolved (§7.6): `COMPLIANCE_DOCUMENT` identifiers, plus the
   existing artifact path when there's real file content to sign.
4. ~~Approval enforcement~~ — resolved (§10.3, §7.9): protocol-enforced, via
   `prepareCollectionCommit` requiring a current, matching `approve` decision on record.
5. ~~Where does collection-signing actually execute~~ — resolved (§9.1): in the publisher
   software, against the manufacturer's own key, never the target server. Still open
   within that: whether v1 is GUI-driven-commit only, or CI/CD-drivable too (a fully
   automated pipeline that also commits collections, not just validates artifacts) — the
   protocol itself (§8) doesn't care which kind of client calls `commit`, so this is a
   publisher-software scoping question, not a protocol one.
6. **Transparency-log system(s)** to target first — Rekor, Sigsum, SCITT, or none in the
   protocol's first version.
7. **Multi-target publishing.** Can one publisher-server release/collection be committed to
   more than one target TEA server (e.g. a manufacturer mirrored across several TEA
   instances), and if so does that need protocol support (idempotency, consistent
   versioning across targets) or is it purely a publisher-server-side concern?
8. **opentea's own implementation of §8** — real, valuable, separate work (§4) — but not
   scoped, scheduled, or designed in this document. Worth its own follow-up once §8 is
   less of a sketch.
9. **Relationship to a future official TEA publisher spec.** If/when TEA's own spec
   defines a publisher API, does §8 become a candidate proposal for it, an opentea-flavored
   extension of it, or something else — worth being explicit about given who's writing
   this and the stated purpose of `spec/openapi.yaml`-style precedent already followed here.
10. **Capability-scoped protocol tokens** (§10.2, §10.4) — does `/publisher/v1` define a
    capability vocabulary for bearer tokens (so a CI/CD credential can be issued
    least-privilege, artifact-evidence-submission-only), or does v1 stay all-or-nothing
    per token, matching `/admin/v1`'s current flat model, with scoping deferred to a later
    protocol version?
11. **Layer A's auth mechanism specifics** — bare OAuth2 alongside OIDC, LDAP/AD-bind scope,
    multi-IdP-per-deployment, and SAML — all still open; see §10.1's own list rather than
    duplicated here.

## 12. Phased plan (draft)

- **Phase 1 — protocol v0 + publisher server skeleton, no signing.** §8's create/link/
  artifact-validate/collection-draft/commit shape (commit creates the collection, skips
  the signature requirement), against a single hardcoded or manually-configured target for
  now. Proves the staging model and the process-shaped GUI before adding cryptography.
  Includes basic Layer A login (§10.1) — a GUI skeleton needs *some* real auth from the
  start; single-IdP OIDC is the minimum viable slice, with LDAP/AD-bind, multi-IdP, and
  SAML (§10.1's open sub-questions) deferred until they're actually needed rather than
  gating Phase 1 on answering all of them first.
- **Phase 2 — collection signing.** `prepareCommit`/`commit`'s signature requirement (§7.8,
  §9) goes live.
- **Phase 3 — artifact validation, properly.** CI/CD-facing `POST .../evidence` (§7.4) as a
  first-class, documented, headless-client-usable operation (may well land before Phase 2
  in practice, since it has no GUI dependency at all).
- **Phase 4 — CLE and compliance streams** (§7.5–7.6).
- **Phase 5 — timestamps and transparency log**, once Trust Architecture Phases 2–3 exist
  on at least one real target implementation to integrate against.
- **Phase 6 — approval workflow**: `approve`/`reject` (§7.9, §10.3, now designed in
  `design/publisher-openapi.yaml`) plus §13's `approval.required`/`granted`/`rejected`
  events, and whichever of §10.1's deferred auth mechanisms (LDAP/AD-bind, multi-IdP,
  SAML) turn out to actually be needed by then. Belongs in this phase, not before it —
  approval has to exist as a real, enforced concept before there's anything meaningful to
  notify about.
- **Phase 7 — eventing** (§13). Signed webhook delivery for at least the collection/
  artifact/publication categories; message-bus transport and payload encryption
  explicitly deferred further, matching §13's own conformance levels.
- **Phase 8+ — opentea's own §8 implementation** (§11.8), multi-target support (§11.7), and
  whatever else §11 resolves into.

## 13. Eventing and notifications

Source: `oej/tea-trust-architecture`'s `event-proposal/publisher-events.md` (status
"Informative" — a proposal, not yet normative). Defines a **generic, transport-independent
event model** for asynchronous notifications about publisher-side workflow activity —
webhooks now, message bus (AMQP-style) marked future-compatible. Read in full for this
revision; adopted here as a companion to the request/response protocol §8 sketches, not a
replacement for any part of it.

### 13.1 Why this belongs alongside the protocol, not instead of it

Everything designed so far (§7, §8) is synchronous request/response: a caller asks, the
target answers. Real publisher workflows have state changes nobody explicitly asked
about *right now* — a draft becomes ready to sign, an air-gapped signing ceremony finally
completes, an approval is granted by someone who isn't the caller who requested it. Two
concrete places this design already needed exactly this and didn't have it:

- **§9.5's air-gapped signing.** A `prepareCollectionCommit` response can sit unsigned for
  days. Without a push notification, "is it ready yet" is either polling or a human
  remembering to check. This proposal's `collection.readyForSigning` event, fired the
  moment a draft locks, closes that gap directly — its `signingPayloadUrl` field points at
  the same TBS package §9.5 already produces.
- **§7.9's approval step.** Treating approval as a field on `commit`'s request body
  assumed the approval decision was already made by the time anyone calls commit. A real
  maker-checker workflow needs the *approver* to be notified there's something to review
  (`approval.required`) and the *drafter* to be notified of the outcome
  (`approval.granted`/`rejected`) — neither side is necessarily even present in the same
  session as the other action.

### 13.2 Adopted design principles (unchanged from the source)

- **Transport independence** — the event's own JSON shape is identical whether delivered
  by webhook or (later) a message bus; only the delivery wrapper differs.
- **Independent trust** — events are signed at the message level (§13.4), not merely
  protected by the webhook's own TLS connection. A relayed or queued event stays verifiable
  even after leaving the original TLS session that first carried it.
- **Optional confidentiality** — payload encryption to a subscriber's public key is
  possible but not required; most publisher events aren't sensitive enough to need it, but
  some subscribers' infrastructure may require it regardless.
- **Audit alignment** — every event carries an `auditEventId` tying it to the same
  audit-log concept `internal/admin/audit.go`'s `adminAuditLog` already gives opentea's own
  `/admin/v1`; an event stream and an audit trail describe the same underlying facts from
  two different angles, not two independent record-keeping systems.
- **Authentication, not authorization.** An event proves who sent it and that it wasn't
  altered in transit — it is explicitly **not** proof that the action it describes was
  authorized. Authorization stays where §10 already put it: enforced by the target
  server's own protocol operations, never inferred from having received an event about
  something.

### 13.3 Event categories mapped onto this design's operations

| Event | Fires from (this design) |
|---|---|
| `artifact.uploaded` | `uploadArtifactFile` (§7.4, §8) |
| `artifact.published` | `submitArtifactEvidence` succeeding (§7.4, §9.2) |
| `collection.created` / `collection.updated` | `putCollectionDraft` (§7.7, §8) |
| `collection.readyForSigning` | `prepareCollectionCommit` locking the draft (§9.5) |
| `collection.signed` | `commitCollectionDraft`'s signature verification step succeeding, before the transaction completes (§9.1 step 3) |
| `collection.validationFailed` | `commitCollectionDraft`/`submitArtifactEvidence` rejecting a bad signature or stale digest (§9.1 step 3) |
| `collection.published` | `commitCollectionDraft` completing (§7.10) |
| `approval.required` / `.granted` / `.rejected` | the new `approve`/`reject` operations §7.9 now implies — not yet designed in detail |
| `publication.commitStarted` / `.committed` / `.failed` | `commitCollectionDraft`'s own lifecycle (§7.10) — overlaps `collection.published`/`validationFailed` somewhat; whether both category sets are needed or one subsumes the other for this design's purposes is unresolved |
| `authorization.error` / `authentication.error` | any §10 layer rejecting a caller |
| `cle.updated` / `cle.versionCreated` / `cle.superseded` | `createProductCLEEvent` and equivalents (§7.5) |
| `product.archived` / `release.archived` | no equivalent operation exists in this design yet — products/releases have no delete or archive path defined here |

The close fit across most rows is a reasonable sanity check that §7/§8's operation shape
isn't obviously wrong — a genuinely different workflow model would have produced events
with nothing sensible to map to.

### 13.4 Signing, key distribution, delivery, and conformance — adopted as proposed

- **Message-level signing** reuses the same shape §9.3's evidence package already
  established (RFC 8785 canonicalization, a signature block naming its format/algorithm) —
  not a new mechanism, the same one applied to a different kind of payload.
- **`GET /event-keys`** — a key-discovery endpoint for verifying event signatures,
  supporting rotation. Distinct from `/publisher/v1`'s own certificate-based evidence
  signing (§9) — this key signs *notifications about* publisher activity, not the
  artifacts/collections themselves.
- **`POST /event-subscriptions`** — a subscriber registers a webhook target, an event-type
  filter list, and delivery-security preferences (signature mode, optional encryption).
- **Conformance levels carried over as-is**: mandatory (envelope, naming, audit binding,
  message signing), recommended (asymmetric signing, key publication, retry/idempotency),
  optional (payload encryption, message-bus transport) — a first implementation of this
  design's eventing only needs the mandatory tier plus signed webhooks to be conformant
  with the source proposal.

### 13.5 Open, not designed further here

- Does the *target* TEA server emit these events, the *publisher software*, or both
  (a target's `collection.published` and a publisher's own workflow events could be
  distinct, complementary streams rather than one)?
- Retry/idempotency semantics for webhook delivery (marked "Recommended" by the source,
  not mandatory) — undesigned here.
- Whether eventing is worth building at all for a v1 that's otherwise still a sketch (§12's
  Phase 7 placement reflects treating it as a real but late priority, not a core-path
  blocker).
- ~~The `approve`/`reject` operations §7.9/§13.3 imply~~ — designed, see §7.9's update and
  `design/publisher-openapi.yaml` directly (`approveCollectionDraft`/`rejectCollectionDraft`,
  scoped per-draft like everything else in §7.7–7.10).

Still not reflected in `design/publisher-openapi.yaml`: the eventing/webhook mechanism
itself (envelope, signing, `/event-keys`, `/event-subscriptions`) — only the
`approve`/`reject` operations §13 motivated were added. This section otherwise remains
design-only.

## 14. Cross-references

- `design/publisher-openapi.yaml` — the OpenAPI 3.1 draft §8 sketches; v0.4's actual
  deliverable (does not yet cover §13's eventing).
- `github.com/oej/tea-trust-architecture` → `event-proposal/publisher-events.md` — the
  source for §13, read in full for this revision.
- `TODO.md` → **Deferred phases** → **Reference publisher**, **Publisher API** — the
  entries this document exists to unblock.
- `TODO.md` → **Deferred phases** → **Trust architecture overlay** — Phases 2–6, several of
  which §9/§12 depend on directly.
- `github.com/oej/tea-trust-architecture` →
  `tea-trust-arch/publisher/publisher-workflow.md` — the workflow model §5–§7 are built
  from; read in full for this revision.
- `CycloneDX/transparency-exchange-api` → `spec/publisher/` (`README.md` + `openapi.json`)
  — the older, superseded publisher draft read in full for v0.4: flat
  `tea_product`/`tea_leaf`/`tea_collection`, no Component concept, mutable collections via
  direct `PATCH`/`DELETE`, no separated artifact-vs-collection signing. Basic REST/auth
  conventions carried forward into `design/publisher-openapi.yaml`; the resource and
  mutability model did not.
- `pkg/teaclient` + `cmd/teaclient` — the read-side precedent this design's client/protocol
  split (§4) mirrors directly.
- `internal/admin/evidencebundle.go`, `internal/trust/doc.go` — the verify-before-store
  logic §7.4/§7.8/§8 are built on, and what opentea's own eventual protocol implementation
  (§11.8) would reuse.
- `internal/repo/repo.go`'s `WithTx` — the transaction-composition pattern any atomic
  commit implementation (§8) needs an equivalent of.
