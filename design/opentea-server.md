# OpenTEA server — architecture and service design

**Status:** draft v0.1, for discussion. This is a first design pass grounded in the
current OpenTEA Go implementation and the adjacent TEA consumer, discovery,
authentication/authorization, trust-architecture, bundle, and publisher design work.
It describes both the server that exists now and an intended production architecture.
Nothing in the target architecture is approved merely by appearing here.

This document deliberately separates:

- the **TEA protocol**, which OpenTEA implements but does not own;
- the **OpenTEA server**, a reference implementation of that protocol;
- the **management plane**, which is implementation-specific;
- the optional **TEA Trust Architecture** profile;
- the proposed **Publisher API**, which is a separate standard protocol; and
- optional **notification infrastructure**, which must not become the source of truth.

## 1. Purpose

OpenTEA is a server for publishing and consuming software-transparency information:
products, releases, components, collections, artifacts, lifecycle information, and
associated trust evidence. Its first responsibility is to implement the CycloneDX
Transparency Exchange API faithfully and interoperably. Its second responsibility is to
provide a practical reference deployment that can start small and evolve into a
production cloud service without changing TEA object identities or client-visible
semantics.

The server has three distinct audiences:

1. **Consumers** — users of products — discover products and retrieve authorized
   transparency information, via the standard TEA consumer API.
2. **Publishers** — manufacturers, open-source projects, and other entities that produce
   products — load, validate, and publish transparency information for what they own, via
   the standard Publisher API.
3. **Admins** — operators of an OpenTEA deployment — govern the server itself:
   configuration, accounts, authorization policy, bundle migration, and audit, via the
   proprietary `/admin/v1` surface.

Those audiences have different trust, availability, latency, and authorization needs.
They must share a consistent data model, but they do not need to share a network listener,
authentication method, scaling policy, or deployment unit.

## 2. Design principles

1. **Protocol conformance first.** `/tea/v1` behavior is defined by the TEA OpenAPI and
   discovery specifications. OpenTEA extensions must not silently alter standard wire
   objects.
2. **Secure by default.** Unknown identities, missing rules, dependency failures, and
   ambiguous ownership fail closed unless a resource is explicitly public.
3. **Published versions are immutable.** Corrections create new versions. Stored trust
   evidence always remains bound to the exact immutable object that was signed.
4. **UUID and content identity are different concepts.** UUIDs identify objects and
   versions within a TEA server. Blob checksums identify identical file content and enable
   safe deduplication.
5. **Separate control and data planes.** Public reads and artifact delivery must not expose
   operator, account-management, entitlement-management, or publisher functions.
6. **The database is authoritative for metadata.** Caches, search indexes, event streams,
   and notification services are derived projections.
7. **Blob storage is content-addressed and immutable.** Metadata points to SHA-256-addressed
   content; clients do not choose storage paths.
8. **Atomic state change, asynchronous side effects.** Database-visible publication and
   its audit/outbox records commit together. Notification, indexing, and CDN invalidation
   happen asynchronously from the outbox.
9. **One codebase may support several deployment profiles.** The reference profile can
   remain a single process with SQLite and filesystem storage; the cloud profile must be
   horizontally scalable and use shared infrastructure.
10. **Optional trust must be explicit.** A deployment that declares itself “Trusted TEA”
    must enforce the profile. A display flag without enforcement is not a trust guarantee.

## 3. Goals

- Implement the TEA consumer API, discovery flow, and conditional request behavior.
- Serve public and restricted product transparency data with fine-grained authorization.
- Support product groups, release groups, authorization templates, entitlements, and
  principal-specific overrides without embedding sales policy directly into handlers.
- Support a standard Publisher API when that specification is ready, while retaining the
  proprietary `/admin/v1` surface only as an implementation/operator API.
- Store and deliver large artifacts efficiently and safely.
- Support signed artifacts and collections under an optional trust profile.
- Provide auditable import, publication, authorization, and administrative operations.
- Scale the consumer plane horizontally for large polling or notification-driven client
  populations.
- Support backup, migration, and ownership transfer through validated product bundles.
- Be deployable as a small reference server and as a resilient cloud service.

## 4. Non-goals

- OpenTEA is not an SBOM, VEX, or compliance-document authoring tool.
- It is not a CI/CD system or a general artifact repository.
- It does not define TEA standards by implementation accident.
- The consumer API does not expose internal accounts, sales data, authorization rules, or
  administrative audit details.
- Notifications are not a guaranteed replacement for the consumer API. Clients must be
  able to recover missed events by reading current server state.
- The first production design does not require independent microservices for every
  package. A modular monolith with external state services is preferred until scaling or
  ownership boundaries justify extraction.

## 5. Current implementation

The current Go implementation is a modular monolith with one `cmd/opentea` process. It
wires four HTTP surfaces into one handler tree:

- `/tea/v1` — TEA consumer API;
- `/files/{sha256}` — artifact and distribution file delivery;
- `/admin/v1` — proprietary authenticated ingestion and administration API; and
- `/admin/ui` — server-rendered operator GUI.

The main internal package boundaries are:

| Package | Current responsibility |
|---|---|
| `internal/api` | Consumer API handlers, authentication context, authorization, pagination, cache policy, and ETags |
| `internal/admin` | Proprietary ingestion, user, authorization-policy, bundle, CLE, and evidence operations |
| `internal/webadmin` | Operator browser UI and sessions |
| `internal/authn` | Session and bearer-token authentication |
| `internal/authz` | Deterministic template/entitlement decision engine |
| `internal/repo` | Aggregate-oriented persistence and transactions |
| `internal/db` | SQLite connection and embedded migrations |
| `internal/storage` | Content-addressed blob abstraction and filesystem implementation |
| `internal/files` | Blob delivery |
| `internal/bundle` | Product bundle import, export, and validation |
| `internal/trust` | Canonical JSON, certificate, signature, and evidence support |
| `internal/httpx` | HTTP response, request-ID, security, parameter, and ETag helpers |
| `pkg/tea` | Shared TEA wire types |

Metadata is stored in SQLite. SQLite is configured with foreign keys, WAL mode, a busy
timeout, and a single database connection because it supports only one writer in this
design. Binary files are stored by SHA-256 under the local filesystem through the
`storage.Storage` interface.

This is an appropriate reference and development profile. It is not the final cloud
profile: a single SQLite writer, local blob directory, and one process cannot provide
horizontal scale or multi-instance failover.

## 6. Target logical architecture

```text
                                   ┌──────────────────────────────┐
                                   │ DNS / TEI /.well-known/tea  │
                                   └──────────────┬───────────────┘
                                                  │
                         ┌────────────────────────▼───────────────────────┐
                         │ Edge: TLS, routing, request limits, optional WAF│
                         └─────────────┬──────────────────────┬───────────┘
                                       │                      │
                         public/consumer plane       private management plane
                                       │                      │
                    ┌──────────────────▼───────┐   ┌──────────▼─────────────┐
                    │ Stateless TEA API nodes  │   │ Admin / Publisher nodes │
                    │ /tea/v1                  │   │ /admin/v1, /admin/ui,   │
                    │ authorization + ETags    │   │ /publisher/v1           │
                    └───────┬──────────┬───────┘   └──────┬──────────┬─────┘
                            │          │                   │          │
                    ┌───────▼───┐  ┌──▼──────────┐       │   ┌──────▼────────┐
                    │ Cache      │  │ Object/CDN  │       │   │ Identity/IdP   │
                    │ optional   │  │ artifact IO │       │   │ OIDC/mTLS/etc. │
                    └───────┬───┘  └──┬──────────┘       │   └───────────────┘
                            │          │                   │
                            └──────────┴──────────┬────────┘
                                                 │
                                     ┌───────────▼───────────┐
                                     │ Relational database    │
                                     │ metadata + authz +     │
                                     │ audit + transactional  │
                                     │ event outbox           │
                                     └───────────┬───────────┘
                                                 │
                                   ┌─────────────▼────────────┐
                                   │ Outbox dispatcher         │
                                   │ events, indexing, purge,  │
                                   │ notification integration  │
                                   └───────────────────────────┘
```

This is a logical decomposition, not a requirement to deploy every box as an independent
service. The first cloud implementation can keep API, admin, and worker code in one
repository and even one binary, while running them with different modes, listeners, and
replica counts.

## 7. Network surfaces and trust boundaries

### 7.1 Consumer plane

The consumer plane exposes only:

- the negotiated TEA read API;
- discovery;
- authorized artifact metadata; and
- immutable blob downloads.

It is designed for high fan-out, aggressive conditional caching, and horizontal scale.
It must never expose operator sessions or mutation endpoints.

### 7.2 Management plane

The management plane contains:

- `/admin/v1` for implementation-specific operation;
- `/admin/ui` for human operators;
- authorization-template and entitlement management;
- account, session, and API-token management;
- bundle import/export;
- audit search and operational statistics; and
- eventually the standard `/publisher/v1` endpoint.

It should use a separate listener and normally a separate network route. `TEA_ADMIN_LISTEN_ADDR`
already provides this today — an optional second listener that, when set, splits `/admin/v1` and
`/admin/ui` off onto their own address, separate from `/tea/v1` and `/files` (off by default, so
existing single-listener deployments are unaffected). The remaining gap is that both listeners
still share one TLS certificate/key pair; per-listener TLS termination is not yet supported.
Deployment policy may require VPN, private ingress, IP allowlists, mutual TLS, stronger
authentication, or a combination. Public reachability of `/tea/v1` must not imply reachability of
this plane.

### 7.3 Blob plane

Blob downloads may be served by the Go service in the reference profile. In the cloud
profile, the API should authorize the request and then use one of these patterns:

- stream from object storage through the API;
- issue a short-lived, audience-bound signed object URL; or
- redirect to an authorized CDN URL.

Restricted artifacts must never be exposed through permanent public object URLs. Cache
keys must include the authorization-relevant representation or use private caching.

### 7.4 Worker plane

Background workers process durable outbox records, garbage collection, asynchronous
bundle jobs, notification delivery, indexing, and reconciliation. Workers do not decide
authoritative publication state; they observe already-committed database state.

## 8. API surfaces

### 8.1 TEA consumer API

`/tea/v1` remains a faithful implementation of the current TEA OpenAPI. Version routing
must allow the discovery-advertised URL form without relying on undocumented proxy
rewrites. `TEA_API_BASE_PATH` already addresses this for the single-version case today —
the consumer API's mount path is configurable (default `/tea/v1`) instead of hardcoded, so
deployments can match whatever path their discovery document advertises without a rewriting
proxy in front. The open part is multi-version routing: if several TEA versions are served
simultaneously, each version must have an explicit compatibility and deprecation policy, which
`TEA_API_BASE_PATH` alone does not provide.

Consumer handlers should remain thin:

1. Parse and validate the request.
2. Resolve the authenticated principal and network context.
3. Evaluate authorization against the requested capability and resource.
4. Read a consistent representation from the repository/query layer.
5. Apply ETag and cache policy.
6. Serialize the standard wire representation.

### 8.2 Discovery

Discovery begins with a TEI, resolves its authority, obtains `/.well-known/tea`, negotiates
an API version, and calls `/discovery?tei=...`. Server responsibilities are:

- publish a schema-valid well-known document;
- return only authoritative mappings for TEIs owned by the deployment;
- return the standard discovery response and error shapes;
- advertise externally reachable root URLs rather than internal listener addresses;
- support HTTP freshness metadata for the well-known document; and
- avoid leaking the existence of unauthorized products where deployment policy requires
  non-disclosure.

The discovery test rig in `docs/discovery-test-rig.md` should become the interoperability
contract for this area once normative, RFC-interoperability, and security-profile cases
are separated.

### 8.3 Proprietary admin API

`/admin/v1` is an OpenTEA implementation surface, not a TEA standard. It can remain while
the Publisher API matures, but must not become the implicit specification for publishers.

Its long-term role should narrow to:

- server configuration and health;
- user and credential administration;
- authorization-policy administration;
- bundle migration and recovery;
- audit and operational queries; and
- exceptional operator repair.

Normal manufacturer publication should eventually use `/publisher/v1`, not `/admin/v1`.

### 8.4 Standard Publisher API

**Implemented 2026-08-30** — `internal/publisher`, mounted at `/publisher/v1` on the
management-plane listener (see §7.2), covering the full v1 protocol surface from
`design/publisher-openapi.yaml`: product/component/release/CLE creation, artifact create/
upload/evidence prepare+submit, and collection-draft put/get/delete/approve/reject/
prepareCommit/cancelPrepare/commit — both product-release- and component-release-owned.
Gated by a new `publisher_credential` bearer-token model with two structurally-enforced
scopes (`full`/`cicd`, matching `design/publisher-service.md` §10.4's split), admin-issued
via `/admin/v1/publisherCredentials`. Out of this pass, matching `design/publisher-service.md`
§12's own phasing: timestamps/transparency-log evidence, eventing/webhooks (so commit does
*not* yet write an outbox event, only the object+evidence — see `TODO.md`'s **Publisher API**
entry for the audit-record gap too), DNS trust-anchor publication, multi-target support.

The boundary this implements (resolved; see `design/publisher-service.md` §4 and §11 Q1):

- OpenTEA owns collection-draft staging, expiry/locking, and approve/reject enforcement
  (maker-checker) as protocol operations, regardless of which client calls them. This is
  required, not just convenient: two independent real workflows exist — CI/CD publishing
  directly to OpenTEA with a manufacturer's GUI publisher platform watching events and
  approving, and CI/CD publishing through an in-house GUI publisher platform first — and
  in the first workflow, OpenTEA is the only state CI/CD and the human approver share;
  they have no other common database;
- signing always happens client-side, in whichever publisher platform is calling (the GUI
  service, or a lightweight reference CLI client embedded in CI/CD) — never on OpenTEA;
- a publisher platform may additionally run its own internal, multi-team business approval
  (legal, compliance, security engineering) before it ever calls OpenTEA, but that process
  is entirely internal to the publisher platform and outside the standard protocol — the
  target only ever sees the one maker-checker decision the calling platform's
  authenticated identity asserts;
- OpenTEA assigns target-local identities and versions;
- OpenTEA returns the exact to-be-signed representation;
- commit consumes the current draft/approval state and atomically stores the object,
  evidence, audit record, and outbox event.

The standard Publisher API and `/admin/v1` may share application services and repository
transactions internally, but they must have separate handlers, authentication policy,
and wire contracts.

## 9. Domain and identity model

### 9.1 Stable objects and versions

- Product and component UUIDs identify stable lineages.
- Product-release and component-release UUIDs identify releases.
- A collection UUID equals its owning release UUID; collection version changes as its
  immutable published content changes.
- An artifact UUID identifies an artifact lineage within a server; artifact version
  identifies an immutable revision.
- A blob SHA-256 identifies exact binary content globally enough for storage deduplication,
  but is not a substitute for TEA object identity or authorization context.

### 9.2 Artifact deduplication

When identical content is uploaded more than once, the blob store should retain one
physical copy keyed by checksum. Separate artifact objects may reference it because they
can have different metadata, owners, authorization, evidence, and lifecycle.

Deduplication must occur below authorization. Possession of a checksum must not prove that
a caller may read the corresponding artifact, nor may upload timing reveal whether another
customer already stored the same content.

### 9.3 Bundle import

Bundle import crosses a server identity boundary. Imported UUIDs must not be assumed to be
globally unique. The importer should:

- validate the complete bundle and every blob checksum before mutation;
- resolve artifacts by content identity and compatible metadata where safe;
- allocate new target-local UUIDs when imported identities collide or are untrusted;
- rewrite all internal references consistently;
- produce an import mapping and audit record;
- execute metadata changes atomically; and
- avoid deleting pre-existing shared blobs during rollback.

If a signed collection or artifact representation changes because UUIDs or URLs are
rewritten, its old signature no longer covers the imported representation. The importer
must preserve it as source evidence only, mark it non-current, or require the manufacturer
to issue new evidence. It must never silently present the original signature as validating
the rewritten target-local object.

## 10. Persistence architecture

### 10.1 Reference profile

- SQLite metadata database.
- One database connection and writer.
- Filesystem content-addressed blob store.
- One process.
- Local backup coordinated across database and blob directory.

This profile is intended for development, interoperability tests, demos, and small trusted
installations.

### 10.2 Cloud profile

- PostgreSQL or another supported transactional relational database.
- Shared object storage with immutable keys and lifecycle support.
- Multiple stateless consumer API replicas.
- Independently scaled management/publisher replicas.
- Optional distributed cache.
- Durable transactional outbox and worker replicas.
- CDN or object-storage acceleration for artifact downloads.

The repository layer should be split into domain-oriented interfaces instead of exposing
database-specific behavior. SQLite and PostgreSQL implementations must pass the same
contract tests, including transaction isolation and concurrent version allocation.

### 10.3 Transaction boundaries

The following must be atomic:

- version allocation and immutable object creation;
- collection creation and evidence attachment in the trusted profile;
- reference-count or reachability changes used for blob garbage collection;
- publication state, audit record, and outbox event;
- entitlement/template activation and its audit record; and
- bundle-import metadata plus its UUID mapping.

Blob upload itself cannot generally share a relational transaction. Use a staged upload:

1. Stream into a temporary or uncommitted object while computing its digest and size.
2. Verify limits and expected metadata.
3. Promote or address the immutable blob by checksum.
4. Commit the metadata reference.
5. Let a reconciler remove abandoned temporary/unreferenced blobs after a safety window.

## 11. Authentication and authorization

### 11.1 Normalized principal context

All authentication methods should produce a normalized principal containing at least:

- stable subject ID and issuer;
- authentication method and assurance level;
- user, workload, or service principal type;
- groups/company/customer attributes when trusted;
- token/session expiry;
- source network information; and
- request correlation ID.

Supported deployments may use bearer JWTs, opaque tokens with introspection, mutual TLS,
local admin sessions, or an external policy/identity provider. Handlers must not embed
provider-specific claim parsing.

### 11.2 Consumer authorization

The decision engine evaluates capabilities against product, product-group, release,
release-group, collection, artifact, and artifact-type scope. Rules may apply to:

- everyone, including unauthenticated users;
- all authenticated users;
- customer/company/group membership; or
- a specific principal.

The intended model uses reusable versioned templates plus entitlements. More specific
resource and subject scopes override broader scopes, with explicit deny winning within the
same final specificity tier. No applicable rule means deny.

The server must support a public baseline template, group/customer defaults, and narrowly
scoped user overrides. Authorization may allow release and CLE metadata while denying SBOM,
VEX, or other artifact details and downloads.

### 11.3 Network policy

Identity authorization and network policy are independent inputs. Deployment policy may:

- deny requests from listed networks;
- permit selected public capabilities from trusted networks without authentication;
- restrict accounts to approved source networks; or
- require stronger authentication for sensitive capabilities or untrusted networks.

The evaluation order should be deterministic:

1. Validate trusted proxy provenance and derive the real client network.
2. Apply emergency and explicit network denies.
3. Authenticate where credentials are present or required.
4. Determine the required authentication strength.
5. Evaluate identity/group/template entitlements.
6. Apply resource-level authorization and field filtering.
7. Record the decision without logging secrets.

### 11.4 Publisher and operator authorization

Publisher capabilities must be separate from consumer read capabilities. Human approval
must be derived from authenticated identity, not a caller-supplied actor string. CI/CD
workloads require short-lived, narrowly scoped credentials and must be structurally unable
to approve their own publication.

Administrative roles should be replaced or supplemented by explicit capabilities as the
surface grows. A single `admin` role is too broad for production separation of duties.

## 12. Trust architecture

Plain TEA and Trusted TEA are distinct deployment profiles.

In a trusted profile:

- artifacts become immutable before evidence is accepted;
- evidence verification recomputes the canonical digest server-side;
- certificate and algorithm policy is explicit and versioned;
- a collection cannot become consumer-visible without required evidence;
- prepare/commit uses a single-use, expiring transaction;
- signature replay and stale preparation are rejected;
- evidence remains attached to the exact object/version it covers; and
- verification failures are auditable but never stored as valid evidence.

Trust-profile capabilities should be discoverable. A boolean display flag is insufficient:
the server must report which signing formats, algorithms, certificate modes, timestamp
services, and transparency systems it enforces.

Signing keys remain outside OpenTEA. The server receives certificates, signatures,
timestamps, and transparency evidence, never manufacturer private keys.

## 13. Caching, ETags, and polling

Every immutable object version can use a representation-derived strong ETag. UUID alone is
not a sufficient ETag for a resource whose representation can change; for an immutable
`(uuid, version)` representation it may contribute to, but does not replace, a canonical
representation hash.

Mutable list/latest/discovery resources should use a server-maintained change watermark or
canonical representation digest. Authorization must be part of cache correctness: two
principals receiving different filtered representations must not share an unsafe cache
entry.

Recommended behavior:

- Immutable artifact blobs: long-lived cache, strong SHA-256 ETag, byte-range support.
- Immutable object versions: strong ETag and long freshness where safe.
- `latest`, lists, CLE, and discovery: short freshness plus ETag revalidation.
- Authorized responses: `private` unless a shared-cache-safe authorization partition is
  deliberately implemented.
- Public responses: CDN-friendly where semantics allow.

Hourly per-product polling combined with ETags greatly reduces response bytes and database
work, but does not eliminate request volume. Notifications can reduce routine polling;
clients still need periodic reconciliation because events may be delayed or lost.

## 14. Events and notifications

Every committed mutation should append a normalized event to a transactional outbox in the
same database transaction. Events should contain:

- event ID and schema version;
- event type;
- object type, UUID, and version;
- manufacturer/server identity;
- actor principal type and stable ID;
- authorization/audit correlation ID;
- committed timestamp; and
- minimal routing metadata, without embedding restricted artifact contents.

An outbox dispatcher can publish to a notification service, webhook delivery system,
message bus, search indexer, cache invalidator, or audit archive.

The notification service may be a third deployment, but authorization remains essential.
It must not disclose that a restricted product, release, SBOM, or VEX exists. Subscription
creation must be authorized, and delivery should either be filtered at publication time or
contain only an opaque change hint that the consumer resolves through the authorized TEA
API.

Publisher accounts may also be consumer principals. These are roles/capabilities attached
to one principal, not mutually exclusive account types.

## 15. Scalability and availability

The expected workload is read-heavy: comparatively few manufacturer writes fan out to many
customers polling or downloading artifacts. Scale the read and write paths independently.

### 15.1 Consumer reads

- Stateless replicas behind a load balancer.
- Connection pooling to the relational database.
- Read replicas only where authorization and freshness semantics remain correct.
- CDN/object storage for large immutable files.
- ETags and conditional GET for lists, latest versions, collections, CLE, and discovery.
- Bounded pagination with stable cursors.
- Query indexes aligned with authorization scopes and common product/release lookups.
- Optional cache for resolved authorization decisions with revision-aware invalidation.

### 15.2 Publisher and admin writes

- Smaller independently scaled pool.
- Strict request-size, concurrency, and rate limits.
- Idempotency keys for retryable write workflows.
- Optimistic concurrency on mutable drafts/configuration.
- Database serialization or locking for version allocation.
- Asynchronous processing for large bundle jobs and expensive verification when protocol
  semantics permit it.

### 15.3 Storage controls

Track separately:

- used artifact bytes referenced by published collections;
- unused/uncommitted artifact bytes and count per principal;
- temporary upload bytes;
- evidence and audit growth; and
- egress volume.

Enforce per-upload size, concurrent upload, per-principal unused-storage, and request-rate
limits. Garbage collection must be reachability-based and delayed. Never delete a blob
solely because one referencing object was removed when another reference still exists.

### 15.4 Availability

- Health endpoints distinguish liveness from readiness.
- Readiness checks database and required storage access without performing destructive
  writes.
- Graceful shutdown drains requests and stops taking new work.
- Database migrations run as a controlled deployment step in clustered environments, not
  concurrently in every replica.
- Backup and restore are tested as one metadata-plus-blob recovery procedure.
- Recovery-point and recovery-time objectives are deployment policy and must be stated.

## 16. Security controls

- TLS 1.2 minimum, with TLS 1.3 preferred.
- Correct trusted-proxy configuration; forwarded headers are ignored unless the immediate
  proxy is explicitly trusted.
- Separate public and management listeners and ingress policy.
- Strict request, header, multipart, decompression, and JSON limits.
- Per-route deadlines appropriate to metadata versus streamed blobs.
- Rate limits for login, discovery, expensive authorization queries, uploads, and bundle
  operations.
- Secure cookies, CSRF protection, session rotation, and short idle/absolute lifetimes for
  the admin GUI.
- Password hashing or, preferably, enterprise OIDC for human operators.
- Secrets stored outside ordinary configuration files where platform facilities exist.
- Output encoding and restrictive CSP for the GUI.
- Safe file rendering: untrusted artifacts download as data and are not rendered inline
  without sandboxing and content validation.
- No server-side fetching of caller-provided URLs without explicit SSRF defenses.
- Cryptographic algorithm and certificate-policy allowlists.
- Dependency, container, SBOM, vulnerability, and secret scanning in CI.

## 17. Logging, audit, and observability

### 17.1 Operational logs

Structured logs should include request ID, route/operation ID, status, latency, byte counts,
principal type and stable pseudonymous identifier, authorization outcome code, and target
resource identifiers. They must not include bearer tokens, session cookies, private keys,
full certificate chains, uploaded document bodies, or unrestricted personal data.

### 17.2 Audit log

Audit is distinct from operational logging. Record every:

- authentication and high-value authorization failure;
- user, credential, template, entitlement, and group change;
- publisher prepare, approve, reject, commit, and cancellation;
- artifact upload/finalization and evidence verification result;
- bundle import/export and UUID mapping;
- destructive or exceptional operator action; and
- trust-policy or server-profile change.

Audit records should be append-only, exportable, retention-controlled, and tamper-evident
for high-assurance deployments.

### 17.3 Metrics and tracing

At minimum expose:

- request rate, error rate, and latency by operation;
- authorization allow/deny/error counts;
- database pool and query latency;
- blob upload/download bytes and failures;
- used, unused, and temporary storage;
- outbox depth and oldest event age;
- notification delivery success and retry age;
- cache hit/revalidation rates;
- signature-verification results; and
- active/expired drafts, prepares, and approvals.

Distributed traces should propagate across API, database, object storage, outbox, and
notification delivery without placing secrets in spans.

## 18. Failure handling

- Repository, authorization-store, and trust-verification failures fail closed.
- Client errors have stable machine-readable codes and correlation IDs.
- Retryability is explicit; clients must not infer it from prose.
- Write retries use idempotency keys.
- Publication never reports success until authoritative state is committed.
- Event delivery failure does not roll back an already-committed publication; it remains
  pending in the outbox.
- Object-storage failure before metadata commit aborts the operation; failure after an
  unreferenced blob write is repaired by reconciliation.
- Cache and search-index failure fall back to authoritative reads or a clear unavailable
  response; stale authorization must never be used silently.
- Partial bundle import is forbidden. Validation completes before mutation and metadata
  commits atomically.

## 19. Deployment profiles

### 19.1 Developer/reference

- One process and listener.
- SQLite.
- Filesystem blobs.
- Local admin user.
- Optional self-managed TLS.
- No external cache or event broker.

### 19.2 Small production/on-premises

- Separate public and management listeners.
- Reverse proxy or ingress TLS.
- SQLite only if single-instance limits and backup requirements are accepted; otherwise
  PostgreSQL.
- Shared or backed-up object storage.
- Enterprise OIDC for operators.
- Scheduled reconciliation and tested backup/restore.

### 19.3 Cloud/high availability

- Stateless consumer and management deployments with separate autoscaling.
- Managed PostgreSQL with high availability.
- Object storage and CDN.
- Secret manager and workload identity.
- Durable outbox workers and optional event broker.
- Central metrics, logs, traces, and audit export.
- Multi-zone operation and regularly tested recovery.

## 20. Testing and conformance

- Consumer API contract tests generated from the authoritative OpenAPI.
- Discovery rig tests, with normative and hardening profiles separated.
- Publisher API conformance tests once the protocol stabilizes.
- Repository contract tests against SQLite and PostgreSQL.
- Authorization decision tables covering templates, groups, principal overrides, network
  policy, deny precedence, expiration, and dependency failure.
- Trust test vectors for canonicalization, digest encoding, JWS/CMS profiles, certificate
  chains, replay, expiry, and algorithm rejection.
- Concurrency tests for version allocation, idempotency, drafts, prepares, imports, and
  garbage collection.
- Fault-injection tests for database, object storage, outbox, cache, and notification
  failures.
- Load tests modeling polling, ETag revalidation, release fan-out, and artifact downloads.
- Backup/restore and rolling-upgrade exercises.

## 21. Phased implementation plan

### Phase 1 — document and stabilize the current modular monolith

- Treat current package boundaries as explicit application boundaries.
- Reconcile README/configuration drift.
- Define public, admin, GUI, and file listener separation.
- Publish health, metrics, and standard error contracts.
- Complete resource and concurrency limits.

### Phase 2 — production persistence

- Introduce repository contract tests.
- Add PostgreSQL support.
- Add object-storage implementation and staged upload lifecycle.
- Add reachability-based orphan reconciliation.
- Move clustered migrations into an explicit deployment job.

### Phase 3 — authorization and identity hardening

- Finish normalized principal and external IdP support.
- Add capability-based operator and publisher authorization.
- Integrate network rules and authentication-strength requirements.
- Add decision/audit observability and safe cache invalidation.

### Phase 4 — trusted publication

- Resolve the Publisher API security-review blockers.
- Implement immutable artifact finalization and artifact version creation.
- Implement single-use prepare transactions and atomic evidence-bound commit.
- Make the Trusted TEA declaration enforceable and discoverable.

### Phase 5 — cloud read scaling

- Stateless consumer replicas.
- Authorization-safe caching.
- Object/CDN downloads.
- ETag coverage for every read shape.
- Database indexes and load testing against the expected population.

### Phase 6 — events and notification integration

- Transactional outbox.
- Worker/retry/dead-letter behavior.
- Authorized subscription integration.
- Consumer reconciliation workflows for missed events.

## 22. Open questions

1. Which TEA consumer versions must one deployment serve simultaneously?
2. Should version routing live in OpenTEA or always in a reverse proxy?
3. Is PostgreSQL the single cloud database target, or must the repository abstraction
   support additional engines?
4. ~~Does the publisher protocol keep editable drafts on the manufacturer publisher
   service, or on OpenTEA?~~ — resolved (§8.4): on OpenTEA, always, as protocol
   operations — required by the direct-CI/CD workflow, where OpenTEA is the only state
   CI/CD and a human approver share.
5. What precise capability vocabulary is shared across consumer, publisher, and operator
   authorization?
6. How are external JWT claims, internal entitlements, and external policy decisions
   combined and cached?
7. Which network-policy rules belong in OpenTEA versus ingress infrastructure?
8. What constitutes the mandatory Trusted TEA profile?
9. Which signing formats and certificate modes are mandatory for v1?
10. Is event delivery hosted by OpenTEA, by a shared third service, or both?
11. What information may an unauthorized discovery request reveal?
12. What are the default artifact size, unused-storage, rate, and concurrency limits?
13. What are the backup, recovery, retention, and audit requirements for each deployment
    profile?
14. How are target-local UUID mappings exposed during multi-target publication and bundle
    import?
15. Which operational APIs are standard, and which remain OpenTEA-specific?

## 23. Immediate design decisions recommended

Before adding major new code, settle these points:

1. Keep OpenTEA as a modular monolith initially, but separate public and management
   listeners and deployment roles.
2. Select PostgreSQL plus object storage as the cloud persistence profile while retaining
   SQLite/filesystem for reference deployments.
3. Keep collection-draft staging, expiry/locking, and approve/reject enforcement on
   OpenTEA as protocol operations (§8.4); a publisher platform may run its own additional
   internal business approval before calling OpenTEA, but that stays outside the protocol.
4. Define artifact finalization and version creation before implementing the Publisher
   API.
5. Define authenticated actor delegation and capability-scoped publisher credentials.
6. Make trust-profile enforcement real before advertising “Trusted TEA.”
7. Add a transactional outbox before building notification delivery or search projections.
8. Treat ETags, authorization-safe caching, and content-addressed object delivery as core
   scale mechanisms, not later optimizations.

## 24. Cross-references

- `design/publisher-service.md` — manufacturer-side publisher service and workflow.
- `design/publisher-openapi.yaml` — draft Publisher API.
- `docs/discovery-test-rig.md` — discovery interoperability test plan.
- `docs/security-review-disc-tests-260827.md` — discovery test-plan review.
- `docs/bundle-format.md` — product import/export format.
- OpenTEA source `README.md`, `README-admin.md`, `README-client.md`, `README-deploy.md`,
  `TODO.md`, and `SMOKE_TEST.md` — current implementation and operational behavior.
- CycloneDX `transparency-exchange-api/spec/openapi.yaml` — authoritative consumer API.
- CycloneDX `transparency-exchange-api/discovery/` — TEI discovery.
- CycloneDX `transparency-exchange-api/auth/` and the OpenTEA authorization specification
  — authentication and authorization model.
- `tea-trust-architecture` — optional trust and notification proposals.

