# TEA Publisher — protocol and service design

**Status:** draft v0.19, for discussion. Nothing here is scheduled or approved; no
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
  `evidence-submission`, which `commitCollectionDraft` now takes directly. (This revision
  also fixed §10.3 and open question #4, which had drifted out of sync with the
  `approve`/`reject` design itself — both now correctly say protocol-enforced.)
- **v0.10 (this revision)** adds §14, consolidating CI/CD as an actor into one section
  instead of scattered mentions across §4/§5/§7.4/§9.2/§9.5/§10.4. Two real
  recommendations: a reference CI/CD client (mirroring `pkg/teaclient`'s role on the read
  side, §14.1) rather than expecting every pipeline to hand-roll HTTP calls, and workload
  identity federation (§14.2) — extending Layer A's already-necessary OIDC relying-party
  capability (§10.1) to also accept short-lived tokens a CI/CD platform mints for a
  running job, rather than provisioning long-lived static secrets as the default. The
  more consequential finding: full pipeline automation (CI/CD driving collection commit,
  not just artifact evidence) runs directly into §7.9's maker-checker rule, since the
  same automated identity can't be both drafter and approver by construction — left as a
  genuine open question (§11.12) with three named options, not resolved unilaterally.
  Also named, not designed: idempotency for retried pipeline steps (§14.4, §11.13) and
  air-gapped build environments as a distinct, larger problem from §9.5's air-gapped
  signing (§14.5).
- **v0.11 (this revision)** settles §14.3 by explicit decision: **Option C for v1 —
  human-in-the-loop is a requirement, not a policy default that can be turned off.**
  Options A (distinct automated approver) and B (policy-configurable approval) are
  rejected for v1, not deferred. Refined while settling it: the requirement attaches
  specifically to `approve`/`reject` (§7.9), not to `prepareCollectionCommit`/
  `commitCollectionDraft` — once a human has approved a draft at a specific `revision`,
  the mechanical steps that turn that approval into a signed, published `collection`
  carry no further judgment a human needs to exercise, and may run under a CI/CD
  credential. Narrowing it this far isn't a loophole: `prepareCollectionCommit`'s own
  precondition (§7.9) already guarantees nothing can be signed or published that a human
  didn't review in exactly the form it exists in at that moment, regardless of who
  makes the mechanical calls afterward. §10.4's CI/CD credential scope updated to match
  (may call the mechanical steps; may never call `approve`/`reject`) — and, since no
  capability-scoped tokens exist yet to make that boundary structurally real rather than
  conventional, open question #10 is now explicitly load-bearing for this requirement to
  mean anything at the protocol level, not merely a nice-to-have deferred indefinitely.
- **v0.12 (this revision)** adds §14.6: every CI/CD-triggered write fires the same event
  (§13.3) a human-triggered one would, carrying the same asserted identity (§10.1, §14.2)
  as the event's `actor` — `{"type": "system", "id": "..."}`, per the source proposal's
  own convention, distinguishable from a human actor at a glance. This is what makes
  autonomous pipeline activity visible to a human or dashboard watching the event stream,
  rather than something discovered later by reading logs — and specifically, why
  §14.3/§7.9's `approve`/`reject` restriction shows up as something observable
  (`approval.required` fired, no matching `granted`/`rejected` yet) rather than silent.
  Also fixed a stale table row in §13.3, which still described `approve`/`reject` as
  undesigned after v0.9 had already designed them.
- **v0.13 (this revision)** adds §9.6: per explicit direction, every open/pending piece
  of state needs an expiry, not just an implicit assumption someone eventually acts on
  it. Three distinct expirations, not one: the draft itself (`expiresAt`, refreshed on
  every edit — abandoned-draft hygiene), the prepare lock (`lockExpiresAt` — an abandoned
  air-gapped signing ceremony releases automatically rather than blocking the draft
  forever), and the approval decision (`approval.expiresAt` — a different concern from
  the other two: not cleanup, but not letting a review from months ago authorize a
  commit today even if the content it covered never changed). Reflected in
  `design/publisher-openapi.yaml`'s `collection-draft`/`approval` schemas and
  `prepareCollectionCommit`'s precondition check. Default durations and whether they're
  deployment-fixed or configurable per release class are both left open (§11.14).
- **v0.14 (this revision)** adds §15: expiry (§9.6) doesn't bound resource consumption —
  a credential scoped to only artifact-evidence operations (§10.4) could still upload
  unbounded data that's never referenced by anything, forever. Directly analogous to an
  unfixed gap opentea's own `/admin/v1` already has (`TODO.md`: capped single uploads,
  but no concurrency limit and no orphaned-blob GC) — named explicitly so this design
  doesn't quietly repeat it. Three dimensions on the target server (§15.1): per-upload
  size limit, a per-principal quota on outstanding (uncommitted) bytes and artifact
  count — the piece that actually distinguishes *capability* scope from *volume* bound —
  and orphaned-artifact expiry, the artifact-side counterpart to §9.6's draft expiry.
  Per explicit clarification, also applies to the publisher software and reference CI/CD
  client side (§15.2): client-side size checks as defense against misconfiguration (not
  a substitute for §15.1's enforcement), and a note that any local artifact staging the
  publisher software does for its own reasons (multi-target publishing, §11.7; air-gapped
  TBS exports, §9.5) inherits the identical orphaned-data problem, just on different
  infrastructure. Rate limiting named as a related, distinct, undesigned concern.
- **v0.15 (this revision)** splits §15.1's storage dimension in two, per explicit
  clarification: **used** artifact storage (referenced by a collection — real, expected
  data, deserving its own visible summary and a *capacity* ceiling, not a per-request
  rejection) is a different problem from **unused** artifact storage (never referenced by
  anything — no legitimate reason to be large, the actual abuse control, materially
  tighter than any used-storage ceiling). Checked `internal/model.Stats` directly: today
  it's counts only (`Products`/`Artifacts`/etc.), no byte totals at all — "a summary of
  used artifact sizes" doesn't yet exist in opentea, worth building rather than assumed
  present. `uploadArtifactFile`'s `429` response (`design/publisher-openapi.yaml`)
  narrowed to specifically mean the unused-artifact quota, not a generic one.
- **v0.16 (this revision)** rewrites §4 and resolves §11 Q1, prompted by `design/
  opentea-server.md` §8.4 independently proposing the opposite answer (drafts/approval
  owned by the manufacturer publisher service, not the target) during its own review.
  Reconciled against two real, named workflows: CI/CD publishing directly to the target
  with a GUI publisher platform watching events and approving, and CI/CD publishing
  through an in-house GUI publisher platform that runs its own multi-team business
  approval first. Both are clients of the same target-owned protocol; the first workflow
  structurally requires target-owned staging, since CI/CD and the human approver share no
  other state. Generalizes "the publisher server" into **publisher platform**: any
  signing-capable client of `/publisher/v1` — the GUI service (this document's primary
  subject) and the reference CLI client (§14.1) are its two named shapes. Signing always
  happens client-side in whichever shape is calling, never on the target.
- **v0.17 (this revision)** adds §16, DNS trust-anchor (TAPS) publication — previously
  named only in oej's phase-7 listing (§6) but never designed. Fetched and grounded
  against the actual normative source (`tea-trust-architecture/tea-trust-arch/
  11-dnssec-trust-anchor.md`) rather than general DNS/PKI practice: the record (a DNS CERT
  RR at `<fingerprint>.<trust-domain>`) is fully determined by the certificate already in
  hand, so the only real design surface is *who* may publish it (a publisher-platform
  capability, never CI/CD's credential directly, mirroring §9.1's signing boundary) and
  *how* (the DNS-access options from an earlier discussion, narrowed since the write
  surface is one record, not a zone). Three new open questions (§11 #16-18); narrows #2.
  Also adds domain-ownership verification to `TODO.md` as a separate, related item, not
  designed in this document.
- **v0.18 (this revision)** settles where the shared publisher wire-types/client library
  lives, per explicit direction that both opentea's own future `/publisher/v1`
  implementation and the publisher platform need it: `pkg/teapublisher` (wire types, §8),
  `pkg/teapublisherclient` (HTTP client), `cmd/teapublisherclient` (reference CLI) — all
  three in this repo, mirroring `pkg/tea`/`pkg/teaclient`/`cmd/teaclient` exactly. Resolves
  §14.1's previously-undetermined "where does this client's code live" note. `TODO.md`'s
  **Reference publisher** entry updated to match.
- **v0.19 (this revision)** puts `pkg/teapublisherclient`/`cmd/teapublisherclient` on hold,
  per explicit direction: the publisher platform's primary integration surface will mainly
  be its own GUI, not a CI/CD-embedded reference CLI, so building the CLI client next isn't
  the priority v0.18 assumed. Doesn't reopen anything already settled — §4's "publisher
  platform is a role, any signing-capable client" still holds, §9.1's signing boundary is
  unchanged, and `pkg/teapublisher` (§8, already scaffolded) stays exactly as designed,
  since both the GUI service and opentea's own future `/publisher/v1` server implementation
  need those wire types regardless. Only the reference-CLI layer on top of it moves to
  "later, if CI/CD-direct workflows need it" rather than "next." `TODO.md`'s **Reference
  publisher** entry updated to match.

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

Two real, named workflows drive this, both confirmed against actual manufacturer setups:
(a) CI/CD updates the target TEA server directly, with the manufacturer's GUI publisher
service watching events and acting as the human-in-the-loop approver; (b) CI/CD updates an
in-house GUI publisher platform, which runs its own multi-team business approval (legal,
compliance, security engineering) before it, in turn, publishes to the target. Both are
clients of the *same* target-owned protocol (§8) — they differ only in which credential
calls it and what happens, if anything, before that call. That generalizes "the publisher
server" (v0.3-v0.14's framing) into **publisher platform**: any signing-capable client of
`/publisher/v1`. Two concrete shapes:

```
┌────────────────────┐     ┌──────────────────────────┐
│  CI/CD pipeline       │     │  Manufacturer staff        │
│  (builds artifacts)         │     │  (browser)                    │
└──────────┬───────────┘     └──────────────┬───────────────┘
           │                                              │
           │ (a) direct: reference CLI client               │ GUI
           │     (§14.1) -- embedded in the pipeline,          │
           │     signs locally, calls the target with            │
           │     its own narrowly-scoped credential                 │
           │                                                          ▼
           │                                          ┌─────────────────────────────────┐
           │                                          │  GUI publisher server               │
           │                                          │  (new, standalone service)             │
           │                                          │                                          │
           │                                          │  - staging: draft collections,             │
           │                                          │    in-progress CLE/compliance streams        │
           │                                          │  - internal multi-team business approval      │
           │                                          │    (legal/compliance/security engineering) --  │
           │                                          │    entirely internal, not a protocol concept    │
           │                                          │  - drives collection-level signing               │
           │                                          │  - own DB + own auth (manufacturer staff           │
           │                                          │    sessions; separate from any target's)             │
           │                                          └───────────────────────┬─────────────────────────────┘
           │                                                                  │
           │  Both shapes call the same protocol, with different credentials  │
           ▼                                                                  ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│  Target TEA server  (opentea, or any other conformant implementation)                 │
│                                                                                          │
│  /tea/v1        (existing, unaffected)                                                    │
│  /publisher/v1  (new -- the protocol, §8) -- owns draft staging, expiry/locking, and         │
│                 approve/reject enforcement (maker-checker) regardless of which shape called it │
└───────────────────────────────────────────────────────────────────────────────────┘
```

Key relationships:

- **"Publisher platform" is a role, not one service.** The GUI service being designed here
  is the primary subject of this document, but the reference CI/CD client (§14.1) is
  equally a publisher platform instance — both sign locally and call the target's
  `/publisher/v1` as credentialed clients, never each other's APIs and never any target's
  proprietary admin API (opentea's `/admin/v1` included) — that's the mistake v0.1 made.
- **Collection staging and approval live on the target, always** (resolved, §11 Q1) — not
  because the target needs to understand business process, but because workflow (a) has no
  other shared place for CI/CD and the GUI service's human approver to rendezvous on the
  same draft; they don't share a database. Workflow (b)'s in-house multi-team approval is a
  separate, additional layer entirely internal to the GUI service, sitting *in front of* —
  not instead of — the target's own protocol-level maker-checker gate. The target only ever
  sees the one decision the calling publisher platform's credentialed identity asserts.
- **opentea implementing the protocol** (its own `/publisher/v1`, reusing
  `internal/repo`/`internal/trust` underneath, much like v0.2 sketched) is real, valuable,
  *separate* work — it's what makes opentea a usable target for testing/using either
  publisher platform shape, but it is not part of "build the publisher server" and isn't a
  prerequisite for finishing this design. §8 specifies the protocol precisely enough that
  either piece can proceed independently once it's settled.
- **CI/CD is a first-class actor**, not a footnote — per oej's workflow document (§5), and
  now confirmed as capable of either shape above: signing individual artifacts and calling
  the target directly via the reference client, or submitting to the GUI service, depending
  on which real workflow a given manufacturer runs.
- The **GUI publisher server** specifically (not the reference CLI client, which is
  stateless) needs **its own persistence and auth**, independent of any target — it stages
  data (drafts, in-progress CLE/compliance work, multi-team approval records) that may
  target a server that's down, unreachable, or simply not yet chosen, and manufacturer
  staff need to authenticate to *it*, not to whichever TEA server they happen to be
  publishing to today.

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
listing them together, is open (§11) rather than settled. Oej's phase 7 also lists
"updating DNS trust anchors" in the same breath — designed separately in §16, since it
targets a different system entirely (the manufacturer's own DNS, not any TEA server) and
can't share commit's transaction boundary.

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

**Shared wire types: `pkg/teapublisher`.** Both sides of this protocol need the same Go
structs — opentea's own future server implementation (§11 Q8) and every publisher-platform
shape (§4) calling it. `pkg/tea` already reuses this way for the base objects
(`product`/`release`/`collection`/`artifact`/etc. — most of `design/publisher-openapi.yaml`'s
schemas are those types verbatim), but stays scoped to *"the CycloneDX Transparency
Exchange API OpenAPI spec"* (its own doc comment) — the official spec, not this project's
own draft protocol. The publisher-only delta with no `/tea/v1` equivalent
(`collection-draft`, `approval-decision`, `evidence-submission`,
`prepare-commit-response`, and the rest of `design/publisher-openapi.yaml`'s
publisher-specific schemas) belongs in a new sibling package, `pkg/teapublisher`, importing
`pkg/tea` for the reused base objects rather than redefining them. Keeping it separate from
`pkg/tea` means the official-spec boundary stays honest, and if a real official TEA
Publisher API ever lands (open question #9), only this package needs to reconcile against
it. Settled 2026-08-29; see §14.1 for the client-library half of this decision.

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
- Is Mode 2 support mandatory for every protocol implementer, or an optional capability a
  target server may or may not offer — and if optional, how does a publisher discover
  whether a given target supports it before attempting it?

### 9.6 Expiry: nothing pending stays pending forever

Per explicit direction: every piece of open, not-yet-approved-or-committed state needs a
time bound — an abandoned draft, an unclaimed signing lock, or a stale approval shouldn't
be able to sit indefinitely and still mean something later. Three distinct things, each
with a different reason to expire and a different natural timescale:

- **The draft itself** (`collection-draft.expiresAt`, §7.7) — refreshed on every
  `putCollectionDraft`. An actively-assembled draft never expires out from under whoever's
  working on it; one nobody has touched in a long time eventually auto-abandons (same
  effect as `deleteCollectionDraft`). Reason: hygiene — an orphaned draft nobody remembers
  starting shouldn't sit in a "ready to be approved" state forever.
- **The prepare lock** (`collection-draft.lockExpiresAt`, §9.5) — set by
  `prepareCollectionCommit`, cleared by `cancelPrepare` or a successful
  `commitCollectionDraft`. Past its expiry the lock auto-releases (same effect as
  `cancelPrepare`), leaving the draft and its recorded approval intact. Reason: an
  air-gapped signing ceremony that was quietly abandoned (nobody remembered to call
  `cancelPrepare`) shouldn't permanently block further edits to the draft.
- **The approval decision itself** (`collection-draft.approval.expiresAt`, §7.9) — set
  when `approve` records a decision. `prepareCollectionCommit`'s precondition checks this
  alongside `decidedAtRevision` — an expired approval is treated exactly like no approval
  at all, requiring a fresh `approve` even though the content (`revision`) never changed.
  Reason: distinct from the other two — this isn't about cleaning up abandoned state, it's
  about not letting a review from months ago authorize a commit today. Circumstances a
  human's approval implicitly depended on (policy, the state of the artifacts' own
  evidence, the approver's own continued authority to approve) can change even when the
  draft's content doesn't.

**Not designed further here:** the actual default durations for any of the three (this
document doesn't propose numbers), whether they're fixed per deployment or configurable
per release class (mirrors §14.3's Option B question for approval enforcement generally —
a similar shape of question, not resolved the same way by default), and the mechanism by
which expiry actually takes effect (a background sweep vs. checked lazily whenever the
draft is next touched — an implementation detail, not a protocol question, but one every
implementer needs *an* answer to).

**Expiry alone doesn't bound resource consumption — that's a separate concern, §15.**
This section cleans up abandoned *draft* state, which is cheap (uuid+version
references). It says nothing about the *artifacts* those references point at, which are
where the actual storage cost is, and which have no expiry, size limit, or count limit
of their own anywhere in this design.

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
headlessly needs its **own, narrowly-scoped credential** — one that can do artifact
evidence submission, collection-draft assembly, and (per §14.3) the mechanical
`prepareCollectionCommit`/`commitCollectionDraft` steps, but categorically not
`approve`/`reject` (§14.3's actual human-in-the-loop gate) or product/component
creation — so a credential leaked from a build log or CI environment can't be used for
anything beyond its actual job, and specifically can't be the thing that quietly removes
the human from the loop. This is the concrete case that makes Layer B's capability-scoping
question not just theoretical: without some scoping mechanism, there is no way to issue a
CI/CD credential that's actually least-privilege, and no way to make §14.3's requirement
structurally true rather than merely conventional. §14 works
out how that credential is actually provisioned, and how far CI/CD can drive the rest of
the process beyond just this. Capability scope (what a credential may do) and quota
(§15, how much it may consume) are separate, both necessary — narrowly-scoped is not the
same as bounded-volume.

### 10.5 Layer E: multi-target credential management

One credential per target TEA server the publisher is configured to publish to (§11.7's
multi-target question), held in the publisher's own state (§9's staging/audit storage).
Not a hard design problem, just a real operational surface worth naming rather than
assuming away.

## 11. Open questions

1. ~~Where does collection staging live~~ — resolved (§4): the target server, as protocol
   operations (§8), always. Two independent real workflows confirmed this: direct CI/CD
   publication needs the target as the only shared rendezvous point between CI/CD and the
   GUI service's human approver (they don't share a database), and in-house-mediated
   publication still ends by calling the same target-owned protocol, just with a different
   credential after its own internal approval. Publisher-server-only staging would break
   the direct-CI/CD workflow entirely.
2. **Does commit span multiple streams** (collection + artifact versions + CLE updates
   together, per oej's phase 7) or stay scoped to one collection at a time (simpler,
   v0.2's original assumption)? Narrowed for the DNS-trust-anchor part of that same
   phase-7 listing (§16.5): that part can't share commit's atomic transaction regardless —
   it's workflow batching against a different system, not a candidate for cross-system
   atomicity. The collection+artifact+CLE part is still open on its own terms.
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
   less of a sketch. The shared wire-types/client library shape it would build on is now
   settled (`pkg/teapublisher`/`pkg/teapublisherclient`/`cmd/teapublisherclient`, §8/§14.1);
   what's still open is opentea's own server-side package layout, DB migration, and route
   wiring (also flagged in `design/opentea-server.md` §21 Phase 4, §22).
9. **Relationship to a future official TEA publisher spec.** If/when TEA's own spec
   defines a publisher API, does §8 become a candidate proposal for it, an opentea-flavored
   extension of it, or something else — worth being explicit about given who's writing
   this and the stated purpose of `spec/openapi.yaml`-style precedent already followed here.
10. **Capability-scoped protocol tokens** (§10.2, §10.4, §14.3) — does `/publisher/v1`
    define a capability vocabulary for bearer tokens (so a CI/CD credential can be issued
    least-privilege, and specifically cannot call `approve`/`reject`/`commitCollectionDraft`),
    or does v1 stay all-or-nothing per token, matching `/admin/v1`'s current flat model?
    **No longer a nice-to-have** — §14.3's human-in-the-loop requirement is only actually
    enforced, not just conventional, once this exists.
11. **Layer A's auth mechanism specifics** — bare OAuth2 alongside OIDC, LDAP/AD-bind scope,
    multi-IdP-per-deployment, and SAML — all still open; see §10.1's own list rather than
    duplicated here.
12. ~~How far CI/CD can drive the process alone~~ — resolved (§14.3): Option C for v1.
    CI/CD may create artifacts/evidence/drafts and even run the mechanical
    `prepareCollectionCommit`/`commitCollectionDraft` steps; only `approve`/`reject`
    require a human actor, by requirement, not by default-that-policy-can-relax — that's
    the actual gate, not the publish mechanics that follow it.
13. **Idempotency for the core protocol's write operations** (§14.4), not just event
    delivery (§13's own conformance tiers already flag it there).
14. **Expiry durations and configurability** (§9.6) — draft, lock, and approval expiry
    are all now designed as concepts; none has a proposed default duration, and whether
    those durations are fixed per deployment or configurable per release class is open.
15. **Resource limits** (§15) — per-upload size, used-storage capacity accounting,
    unused-storage abuse quota, and orphaned-artifact expiry are all named as necessary
    but none is designed in detail: no default numbers for any of the four, no decision
    on proactive quota-status exposure vs. purely reactive rejection, no chosen error
    status for a used-capacity vs. unused-quota breach (plausibly different codes,
    §15.1's own open question — one's operational, the other's adversarial).
16. **DNS trust-anchor transport for v1** (§16.3) — provider-API automation (via a
    `go-acme/lego`-style abstraction), display-and-verify (no credential custody), or
    both offered as options from day one?
17. **DNS trust-anchor rotation/expiry policy** (§16.4) — the source defines none. How
    long a superseded record stays published (old evidence needs it resolvable to
    validate), TTL guidance, and whether/when the publisher platform ever removes an old
    record automatically, are all undesigned.
18. **Does trust-anchor publication need a human approval step** (§16.2), matching
    §14.3's approve/reject maker-checker for collection commit, or does automated
    certificate validation alone satisfy the source's "policy enforcement, controlled
    access" requirement?

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
  in practice, since it has no GUI dependency at all). Natural point to start the
  reference CI/CD client (§14.1) too — the one piece of tooling that makes this phase
  actually usable by a real pipeline instead of only curl-able.
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
| `approval.required` | `prepareCollectionCommit` finding no current approval on record (§7.9, now designed — see `approveCollectionDraft`/`rejectCollectionDraft`) |
| `approval.granted` / `.rejected` | `approveCollectionDraft` / `rejectCollectionDraft` (§7.9) |
| `publication.commitStarted` / `.committed` / `.failed` | `commitCollectionDraft`'s own lifecycle (§7.10) — overlaps `collection.published`/`validationFailed` somewhat; whether both category sets are needed or one subsumes the other for this design's purposes is unresolved |
| `authorization.error` / `authentication.error` | any §10 layer rejecting a caller |
| `cle.updated` / `cle.versionCreated` / `cle.superseded` | `createProductCLEEvent` and equivalents (§7.5) |
| `product.archived` / `release.archived` | no equivalent operation exists in this design yet — products/releases have no delete or archive path defined here |

The close fit across most rows is a reasonable sanity check that §7/§8's operation shape
isn't obviously wrong — a genuinely different workflow model would have produced events
with nothing sensible to map to.

Every row fires identically regardless of *who* triggered the underlying operation — a
human through the GUI or CI/CD calling `/publisher/v1` headlessly (§14) produce the same
event for the same operation. What distinguishes them is the envelope's own `actor` field
(§4 of the source proposal: `"actor": {"type": "system", "id": "ci-system"}` in two of its
own worked examples) — the identical asserted identity (§10.1, §14.2) already threaded
through `putCollectionDraft`/`approve`/`reject`'s own `actor` field becomes the event's
`actor.id`, with `actor.type` distinguishing an automated caller from a human one. See
§14.6 for why this specifically matters for CI/CD.

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

## 14. CI/CD integration

CI/CD has been a recurring actor since §4 ("first-class, not an afterthought") without
ever getting a section of its own — mentioned in passing at §4, §5, §7.4, §9.2, §9.5, and
§10.4, but never consolidated into a coherent answer for "what does a real pipeline
integration actually look like." This section is that answer.

### 14.1 Integration surface: a reference client, not raw HTTP by default

`/publisher/v1` is a plain HTTP API (§8) — nothing stops a pipeline from calling it
directly — but every pipeline reimplementing digest handling, retries, and canonical-form
bookkeeping independently is exactly the kind of duplication this project already avoids
on the read side: `pkg/teaclient` + `cmd/teaclient` exist precisely so a TEA consumer
doesn't hand-roll HTTP calls against `/tea/v1`. The same shape belongs here — a reference
CI/CD client (library + CLI, e.g. `publish-artifact --type=BOM --file=sbom.json
--sign-with=...`) wrapping §7.4's create/upload/prepare/submit sequence into one command a
build step actually calls.

**Settled 2026-08-29** (previously left as "undetermined — depends on decisions about the
publisher project's own repo structure this design doesn't reach"): the wire types and
client library live in *this* repo, mirroring `pkg/tea`/`pkg/teaclient`/`cmd/teaclient`
exactly —

- **`pkg/teapublisher`** — shared wire types for the publisher-only delta (§8), importing
  `pkg/tea` for the reused base objects.
- **`pkg/teapublisherclient`** — the HTTP client library on top of it, used by both
  `cmd/teapublisherclient` below and, per §4's "publisher platform is a role, not one
  service," the full GUI service itself whenever it acts as a client of a target's
  `/publisher/v1`.
- **`cmd/teapublisherclient`** — the reference CLI (`publish-artifact ...` above).

**On hold as of v0.19**: the publisher platform's primary integration surface is expected
to mainly be its own GUI, not pipelines calling a reference CLI directly, so
`pkg/teapublisherclient`/`cmd/teapublisherclient` are deprioritized — not building them
next. `pkg/teapublisher` (the wire types both the GUI service and opentea's own future
server need either way) is unaffected and stays scaffolded as-is.

All three live alongside `pkg/tea`/`pkg/teaclient` because opentea's own future
`/publisher/v1` server implementation (§11 Q8) needs `pkg/teapublisher` as a direct
dependency regardless of where anything else ends up — the same reasoning that already
keeps `pkg/tea` in this repo despite being consumed externally. This does **not** reopen
§3/§4's settled point that the full GUI publisher platform (own DB, own auth, own staging)
is a separate, standalone project — only the shared library and reference CLI move into
this repo, not the application built on top of them.

### 14.2 Credential provisioning: reuse Layer A's OIDC machinery, don't invent a second one

§10.4 already established that CI/CD needs its own narrowly-scoped credential, distinct
from any human's session. How that credential actually gets into the pipeline's hands is
the part that wasn't designed. Two shapes:

- **Workload identity federation (recommended default).** Most major CI/CD platforms
  (GitHub Actions, GitLab CI, others) can mint a short-lived, platform-signed OIDC token
  for a running job, asserting claims like "this is workflow run W, from repo R, on ref
  B" — without any long-lived secret ever stored in the CI system at all. The publisher
  software already needs to be an OIDC relying party for Layer A (§10.1, human staff
  login); extending that same capability to also validate tokens from a CI/CD platform's
  own OIDC issuer (configured trust: issuer URL, expected audience, subject-claim matching
  rules) and exchange them for a short-lived, capability-scoped `/publisher/v1` credential
  is the same mechanism applied to a different class of principal — workload identity
  instead of human identity — not a second auth system to build and maintain.
- **Static scoped token (v1-minimum fallback).** For a CI/CD platform with no OIDC issuer
  of its own, or as a simpler starting point before workload federation is built: a
  manually-provisioned, long-lived bearer token, scoped to artifact-evidence operations
  only (depends on §11's open capability-scoping question, #10, actually landing). Weaker
  — a leaked token is a leaked token until manually rotated — but a reasonable starting
  point, and still strictly better than an unscoped, all-purpose credential.

### 14.3 How far can CI/CD drive the process alone?

**Settled: Option C, for v1 — human-in-the-loop is a requirement, not a default that
policy can turn off.** §7.4 (artifact creation + evidence) is unambiguously CI/CD's job.
Collection assembly (§7.7) may also be CI/CD-driven — a pipeline can call
`putCollectionDraft` freely. `approve`/`reject` (§7.9) always need a human actor — that's
where the requirement actually bites. Options A (a distinct automated approving identity)
and B (policy-configurable approval) were both considered and explicitly rejected for
v1, not merely deferred — a policy gate that can approve is still not a *human* in the
loop, which is the actual requirement, not a stand-in for "some check happened."

**Precisely scoped: only `approve`/`reject` need a human — `prepareCollectionCommit`/
`commitCollectionDraft` don't, and that's not a loophole.** Once a human has approved a
draft at a specific `revision` (§7.9), the mechanical steps that turn that approval into
a published, signed `collection` — building the digest, signing, submitting — carry no
further judgment a human needs to exercise; a CI/CD system (or the same pipeline that
called `putCollectionDraft`) finishing the job it already assembled, against content a
human already reviewed, doesn't reintroduce the thing the requirement exists to prevent.
`prepareCollectionCommit`'s own precondition (a current, matching approval, §7.9) is what
makes this safe: nothing can be signed or published that a human didn't review in
exactly the form it exists in at that moment, regardless of who makes the final calls.

Real manufacturers running fully continuous release trains will still want the
`approve`/`reject` gate itself relaxed eventually (Options A/B); that's a deliberate,
known limitation of v1, revisit only with an explicit decision to do so later, not by
default.

**This makes credential scoping (§11's open question #10) load-bearing, not optional.**
"Human-in-the-loop is a requirement" is only actually true if a CI/CD-issued credential
is *incapable* of calling `approve`/`reject`/`commitCollectionDraft` — not merely
expected, by convention, not to. Today's protocol has no capability-scoped tokens at all
(§10.2/§10.4); until that exists, a CI/CD credential is technically indistinguishable
from any other bearer token and *could* call every operation this section says it
shouldn't. §11's question #10 was previously "nice to have, deferred maybe"; it is now a
correctness dependency of this section's own requirement, not an independent nice-to-have
— still not designed here, but its priority just changed.

### 14.4 Idempotency

CI/CD pipelines retry failed steps routinely — a transient network error re-running
`createArtifact` or `uploadArtifactFile` shouldn't silently produce a duplicate artifact.
None of §7.4/§8's operations have idempotency semantics defined today. §13's own
conformance tiers already name retry/idempotency as "Recommended" for *event delivery*;
the same concern applies to the core protocol's write operations, arguably more urgently,
since those are the ones an automated pipeline actually calls unattended. Not designed
here beyond naming it — likely an idempotency-key request header/field, but the exact
mechanism is open.

### 14.5 Air-gapped build environments — distinct from §9.5, mostly out of scope

§9.5 designed for an air-gapped *signing* environment reachable only via a physically
carried to-be-signed package. An air-gapped *build* environment is a related but larger
problem — getting the artifact's actual bytes out, not just a digest to sign — and isn't
addressed by anything in this design. Worth naming so it isn't mistaken for something
§9.5 already covers, not designed further here.

### 14.6 Visibility: the event system is how autonomous CI/CD activity gets seen

CI/CD operates headlessly (§4, §14.1) — nobody is watching a GUI while a pipeline calls
`createArtifact`, `putCollectionDraft`, or (per §14.3) the mechanical
`prepareCollectionCommit`/`commitCollectionDraft` steps. §13's eventing model is what
makes that activity visible rather than something a human only discovers later by reading
logs: every one of those calls fires the same event (§13.3's mapping) it would if a human
had triggered it, carrying the CI/CD system's own asserted identity (§14.2) as the event's
`actor` — `{"type": "system", "id": "..."}`, distinguishable at a glance from a human
actor. A subscriber (a release-management dashboard, a notification bot, an audit system)
watching for `artifact.published`, `collection.created`, `collection.readyForSigning`,
etc. sees exactly what an automated pipeline has been doing, in near-real-time, without
needing any CI/CD-specific integration of its own — it's the same event stream either way.

This is also why §14.3's `approve`/`reject` restriction matters concretely, not just as a
rule: `approval.required` (fired the moment `prepareCollectionCommit` finds no current
approval — §13.3) is the signal that tells a human reviewer there's something to look at,
precisely because CI/CD cannot generate `approval.granted` itself. The event system
doesn't enforce that boundary (§14.3 already notes only capability-scoped tokens could do
that structurally), but it's what makes a human's absence from that step *visible* rather
than silent — a pipeline stalled waiting for approval shows up as an `approval.required`
event with no matching `approval.granted`/`rejected` yet, not as nothing happening.

## 15. Resource limits and abuse resistance

Raised directly: is §9.6's expiry enough to stop a rogue client uploading massive
artifacts and never committing anything? **No — expiry and quotas solve different
problems, and this design only has the first one.** §9.6 bounds *collection-draft*
state (lightweight uuid+version references, cheap to let sit around and expire later).
The actual storage cost is in **artifacts** (§7.4) — `createArtifact` +
`uploadArtifactFile` — which exist independently of any draft the moment they're
created, have no expiry, no size limit, and no count limit anywhere in this design. A
credential that can only do artifact evidence submission (§10.4's own scoped CI/CD
credential, deliberately unable to approve or commit anything) can *still* upload an
unbounded number of arbitrarily large files that are never referenced by anything,
forever. Narrow-scoping a credential's *capabilities* (§10.2) doesn't bound its
*volume* — those are orthogonal, and this design only designed the first one.

**This isn't a hypothetical — it's an unfixed gap opentea's own `/admin/v1` already has,
named in `TODO.md`**: `importProduct`/`receiveFile` cap a single upload at 1 GiB, but
"nothing caps how many of those can run at once," and there's "no cleanup/garbage-
collection for blobs that end up orphaned." A fresh design shouldn't quietly inherit the
same gap — it's cheaper to design the limit in now than to retrofit it later, which is
exactly the position that existing TODO entry is in.

Per explicit clarification, this applies on **both** ends of §4's client/protocol split —
the target server implementation *and* the publisher software (including the reference
CI/CD client, §14.1) that calls it — not only the side that ends up holding the bytes.

### 15.1 Target server enforcement

The side that actually bears the storage cost, and so the side that must enforce limits
regardless of what any client does or doesn't do — a well-behaved client is a courtesy,
never the actual control. Four distinct dimensions — **used and unused storage are
different problems with different limits, not one number**, per explicit clarification:

- **Per-upload size limit.** A cap on one `uploadArtifactFile` call's file size — mirrors
  opentea's own existing 1 GiB precedent, though the actual number is a deployment
  policy choice, not something this design should hardcode. Cheapest to implement,
  bounds the worst single request, does nothing about volume over many requests.
- **Used-artifact storage: accounting, not (necessarily) abuse prevention.** Total bytes
  of artifacts actually referenced by a collection — real, legitimate, expected data.
  This deserves its own visible summary (a natural extension of `GET /admin/v1/stats`'s
  `Products`/`Artifacts`-style counts — confirmed by reading `internal/model.Stats`:
  today it's counts only, no byte totals at all, so "there's a summary of used artifact
  sizes" isn't yet true of opentea, worth building rather than assuming already exists)
  and, separately, its own limit if one is wanted — but that limit is a **capacity**
  ceiling ("this deployment is provisioned for N TB"), not an abuse control. Exceeding it
  means legitimately needing more storage, not an attack; the right response is
  operational (alert, provision more), not rejecting the request outright the way the
  next bullet's limit should.
- **Unused-artifact storage: a separate, tighter quota — this is the actual abuse
  control.** Total bytes *and* count of artifacts not referenced by any collection at
  all (never drafted, or drafted then dropped). There is no legitimate reason for this
  number to be large — unlike used storage, which is expected to grow with real
  activity — so its limit should be materially tighter than any used-storage capacity
  ceiling, and a breach here is a signal worth treating as abuse (or at least a runaway
  pipeline), not a capacity conversation. Naturally attributed per credential/actor
  (§10's asserted identity) for the same reason §10.4 already singles out CI/CD: a
  script can accumulate unused uploads far faster than a human clicking through a GUI
  ever would, so it's the principal most likely to hit this limit — by mistake or by
  design.
- **Orphaned-artifact expiry**, alongside the quota above, not instead of it — the
  time-bound and volume-bound controls are complementary. An artifact never referenced by
  any collection within some generous window becomes eligible for cleanup, same shape as
  §9.6's draft `expiresAt`. Needs a longer window than a draft's own — §7.4's whole point
  is that an artifact may legitimately sit unreferenced for weeks before a collection
  picks it up ("the SBOM today, the VEX next week") — but "generous" is not "unbounded,"
  and the quota above is what catches a burst *before* that window even elapses.

**Also worth naming, not the same thing:** the number of simultaneously *open drafts*
doesn't actually need its own quota dimension — §7.7 already makes a draft a singleton
per owning release, so the only way to multiply drafts is to create more releases in the
first place, which product/release creation's own (undesigned) limits would need to
bound instead. And **rate limiting** (request frequency) is a related but distinct
concern from quota (cumulative resource consumption) — a client could stay within every
quota above while still hammering the target with requests; not designed here.

**Not designed further here:** actual default numbers for any of the above (deployment
policy, same stance as §9.6's expiry durations); whether quota status is something the
protocol should expose proactively (a `GET` a publisher-software client could check
before attempting a large upload, rather than only discovering the limit via a rejected
request) or purely reactive (a 4xx/5xx on the request that exceeds it); and the specific
status code quota-exceeded should return (413 for a single oversized upload is
unambiguous; a cumulative quota breach is less obviously one code over another — 429,
507, and a plain 403 all have a reasonable argument).

### 15.2 The publisher software and reference client

§15.1's enforcement is what actually matters — the target never has to trust a client's
own good behavior. But the publisher software (§4) and the reference CI/CD client
(§14.1) are not exempt from this concern just because they don't hold the data
long-term; they have their own version of it, for two separate reasons:

- **Defense in depth against a misconfigured, not just malicious, caller.** A CI/CD
  pipeline pointed at the wrong file, or a GUI form accepting a paste of the wrong thing,
  is a far more likely source of an absurd upload than deliberate abuse. The reference
  client (§14.1) should refuse (or at least loudly confirm) an obviously-oversized upload
  *before* spending the time and bandwidth sending it, rather than relying entirely on
  the target's own §15.1 rejection to catch it after the fact — the same relationship a
  client-side form validator has to the server-side check that's still the actual
  authority.
- **The publisher software's own storage isn't automatically exempt.** §11.7 (multi-target
  publishing, still open) would need the publisher software to hold artifact bytes itself
  — at least transiently, possibly longer if the same content is meant to reach more than
  one target — rather than only ever streaming through to a single target in one pass.
  §9.5's air-gapped TBS export (a saved file, physically carried) is a smaller, similar
  case: exported packages sitting in the publisher software's own storage indefinitely,
  never re-imported because a ceremony was abandoned, is the exact same "orphaned data,
  no cleanup" shape §15.1 names for the target — just relocated to different
  infrastructure. Neither of these is designed here; both inherit §15.1's reasoning
  directly whenever they are.

Not a reason to relax §15.1 — the target still enforces its own limits regardless of
what the publisher software does on its own side. It's an additive, not alternative,
responsibility.

## 16. DNS trust-anchor publication (TAPS)

Oej's phase 7 (§7.10) lists "updating DNS trust anchors" alongside collection/artifact/CLE
commit — the first mention of this capability in this document, previously undesigned.
Grounded directly against the normative source
(`tea-trust-architecture/tea-trust-arch/11-dnssec-trust-anchor.md`, fetched in full for
this revision), not general DNS/PKI practice.

### 16.1 What gets published is deterministic, not a policy choice

A trust anchor is one **DNS CERT record** (RFC 4398, PKIX type):

```
<fingerprint>.<trust-domain>. IN CERT PKIX 0 0 <base64-certificate>
```

`fingerprint` is the lowercase hex SHA-256 of the public key — already exactly what
`internal/trust`'s Phase 1 fingerprint identity computes (`internal/trust/keys.go`). The
certificate itself must carry a SAN DNS name matching that same
`<fingerprint>.<trust-domain>` name; per the source, "DNS names are derived from keys —
not assigned," which is what prevents a namespace conflict or spoofed claim. Once a
certificate exists (Phase 1's ephemeral self-signed certs, or a future Web PKI cert per
§9.5), the record name and content are fully determined by it — there's nothing for a
human to compose or choose, only a decision about *whether* to publish it (§16.2).

DNSSEC is explicitly **optional** at the protocol level — "DNSSEC authenticates delivery
of data, not trust in that data." Trust is the composite the source defines: "signature +
timestamp + certificate + DNS (+DNSSEC) + transparency" — DNS publication alone, with or
without DNSSEC, is never sufficient by itself. If a manufacturer's zone *is*
DNSSEC-signed, publishing or updating this record needs the zone's normal re-signing
pipeline to run afterward — an operational dependency on the authoritative DNS
server/provider, not something the publisher platform itself does.

### 16.2 Who is allowed to publish, and why CI/CD's credential can't

The source states this directly: separate "CI/CD functions (signing capability, no DNS
control)" from "DNS publication systems (policy enforcement, controlled access)," and
publication must never be "based on unsigned data, unvalidated certificates, direct CI/CD
write without validation."

This maps onto authorization layers already defined (§10): CI/CD's Layer D credential
(§10.4) — deliberately narrow, evidence-submission-scoped — must not be the thing that can
write DNS. Trust-anchor publication is a **publisher-platform** capability (the same
"never the target server, never a bare CI/CD credential" shape §9.1 already established
for signing itself), gated on the platform having independently validated the certificate
first: identity binding (SAN matches the computed fingerprint name), signature validity,
and timestamp validity — mirroring the same verify-before-store discipline
`internal/admin/evidencebundle.go` already enforces for evidence bundles server-side. The
source doesn't require a *human* approval step here specifically (unlike collection
commit, §7.9) — only that publication is policy-enforced and validated, not a raw,
unchecked write. Whether v1 requires a human step anyway, for consistency with §14.3's
approve/reject maker-checker, or trusts automated validation alone, is open (§11).

### 16.3 Transport: the DNS-access options already surveyed, narrowed by a smaller write surface

Because the write is always exactly one deterministic CERT record at a fixed name — never
broader zone management — whichever transport is chosen only needs record-scoped write
access, not a manufacturer's whole DNS zone:

- **Provider REST APIs** (Cloudflare, Route53, Google Cloud DNS, Azure DNS, ...) — what
  most real manufacturers are actually on. No single standard; rather than hand-roll a
  provider-abstraction layer, reuse `go-acme/lego`'s existing ~150-provider abstraction
  (built for the structurally identical ACME DNS-01 problem: create/clean up one record,
  given per-manufacturer credentials). Most providers support record- or zone-scoped
  tokens (e.g. a Cloudflare API token restricted to one zone), keeping the credential's
  blast radius to that one zone even though the write itself only ever touches one name
  within it.
- **Display-and-verify** (no credential custody) — publisher platform computes the exact
  record content and shows it to manufacturer staff to publish manually, then polls to
  confirm it landed. Zero DNS credentials held anywhere in the publisher platform;
  consistent with `design/opentea-server.md`'s "optional trust must be explicit" design
  principle. Manual, doesn't scale for automated key rotation.
- **RFC 2136 Dynamic Update (TSIG-signed)** — the DNS-native path, relevant only for a
  manufacturer self-hosting authoritative DNS (BIND/PowerDNS/Knot/NSD); essentially no
  managed/commercial provider exposes it to customers.

No default is chosen here — which of these is v1's default (or whether more than one is
offered) is open (§11).

### 16.4 Rotation is "publish a new record," not "update in place"

The source defines no rotation/expiry procedure at all. But because the record name is
*derived from the key* (§16.1), a new signing key automatically gets a new, distinct DNS
name — rotation is naturally "publish a new CERT record alongside the old one," not an
in-place overwrite. What's still genuinely open: how long a superseded record should stay
published (evidence signed under the old key still needs it resolvable to validate), any
TTL guidance, and whether/when the publisher platform should ever remove an old record
automatically (§11).

### 16.5 Not part of the standard protocol, and not the same transaction as commit

Trust-anchor publication targets the manufacturer's own DNS, not any particular TEA
server — symmetric with §9.1's signing boundary, it has nothing to do with `/publisher/v1`
and is entirely a publisher-platform-side concern, out of protocol scope.

This also narrows open question #2 (does commit span multiple streams): oej's phase 7
lists collection/artifact/CLE commit and "updating DNS trust anchors" together as one
workflow step, but they can't share one atomic transaction — a TEA server's database and
an external DNS provider are different systems with no shared transaction boundary. The
realistic shape is workflow batching (one manufacturer action triggers both, in sequence,
each independently policy-gated per §16.2) rather than cross-system atomicity — the
collection+artifact+CLE part of that question is still open on its own terms.

### 16.6 Distinct from domain-ownership verification

Related but separate: before a manufacturer can meaningfully publish a trust anchor (or
anything else) under a domain, the publisher platform needs to know they actually control
it. That's domain-ownership verification — tracked as its own item in `TODO.md`
(**Deferred phases** → **Publisher platform: domain-ownership verification**), not
designed here, since it's a prerequisite check, not part of the trust-anchor record format
or publication mechanism itself.

## 17. Cross-references

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
- `github.com/oej/tea-trust-architecture` →
  `tea-trust-arch/11-dnssec-trust-anchor.md` — the normative source for §16, fetched in
  full for this revision.
- `github.com/go-acme/lego` — the DNS-provider abstraction §16.3 points to reusing rather
  than hand-rolling.
