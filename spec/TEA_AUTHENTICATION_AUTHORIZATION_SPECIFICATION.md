# TEA Authentication and Authorization Specification

## Document Status

| Field | Value |
|---|---|
| Document name | TEA Authentication and Authorization Specification |
| Document type | Normative implementation specification with informational guidance |
| Version | 0.1 |
| Status | Working draft |
| Audience | TEA implementers, identity and access architects, product publishers, operators, security reviewers, and integration developers |
| Scope | Authentication, authorization, policy administration, audit logging, and authorization-change events for TEA services |
| Relationship to other documents | Extends the implementation guidance in the CycloneDX TEA authentication document without changing normative TEA resource objects or artifact formats |
| Normative status | Sections explicitly marked Normative define requirements for conforming implementations; examples and implementation notes are informational |
| Last updated | 2026-08-06 |

## Table of Contents

- [1. Introduction](#1-introduction)
- [2. Scope](#2-scope)
- [3. Conventions and Normative Language](#3-conventions-and-normative-language)
- [4. Terminology](#4-terminology)
- [5. Design Principles](#5-design-principles)
- [6. Authentication](#6-authentication)
- [7. Normalized Principal Context](#7-normalized-principal-context)
- [8. Authorization Deployment Profiles](#8-authorization-deployment-profiles)
- [9. Authorization Information Authorities](#9-authorization-information-authorities)
- [10. Authorization Model](#10-authorization-model)
- [11. Resource Scopes](#11-resource-scopes)
- [12. Capabilities](#12-capabilities)
- [13. Templates](#13-templates)
- [14. Entitlements and Assignments](#14-entitlements-and-assignments)
- [15. Policy Evaluation](#15-policy-evaluation)
- [16. Product and Release Access](#16-product-and-release-access)
- [17. Collection and Artifact Access](#17-collection-and-artifact-access)
- [18. Discovery, Listing, and Query Filtering](#18-discovery-listing-and-query-filtering)
- [19. Event and Notification Authorization](#19-event-and-notification-authorization)
- [20. Revocation, Freshness, and Caching](#20-revocation-freshness-and-caching)
- [21. Authorization Administration and Commercial Integration](#21-authorization-administration-and-commercial-integration)
- [22. Logging and Audit](#22-logging-and-audit)
- [23. Authorization Events](#23-authorization-events)
- [24. Failure Handling](#24-failure-handling)
- [25. Privacy and Security Considerations](#25-privacy-and-security-considerations)
- [26. Conformance Requirements](#26-conformance-requirements)
- [27. Implementation Profile for OpenTEA](#27-implementation-profile-for-opentea)
- [28. Examples](#28-examples)
- [29. Open Questions](#29-open-questions)
- [30. References](#30-references)

## 1. Introduction

The Transparency Exchange API (TEA) supports discovery and retrieval of software transparency information. Deployments range from public open-source repositories to commercial services where product, lifecycle, collection, SBOM, VEX, and other artifact access depends on a customer relationship, contract, certification role, or other entitlement.

The CycloneDX TEA authentication guidance permits HTTP bearer-token authentication and mutual TLS, while intentionally leaving token acquisition, token contents, and authorization policy to implementations. This specification defines a generic authentication and authorization model for TEA services while preserving that deployment flexibility.

The model supports:

- anonymous access to explicitly public resources;
- bearer-token and mutual-TLS authentication;
- self-contained token authorization;
- authorization maintained inside a TEA service;
- external authorization services;
- hybrid deployments combining these sources;
- publisher-managed product groups and release groups;
- organization, audience-group, principal-group, and principal assignments;
- access templates and scoped overrides;
- independent control of products, releases, lifecycle information, collections, and artifacts;
- authorization-aware event feeds and notification delivery;
- audit logging and authorization-change propagation.

Product groups, audience groups, access templates, release groups, and entitlements defined here are implementation constructs. They do not modify the normative TEA product, collection, artifact, or consumer OpenAPI objects.

## 2. Scope

### 2.1 Normative

This specification defines:

- how TEA authentication results are normalized for authorization;
- supported authorization deployment profiles;
- authorization subjects, resource scopes, capabilities, templates, and entitlements;
- deterministic policy evaluation and conflict resolution;
- information-hiding requirements for unauthorized resources;
- authorization requirements for events and subscriptions;
- revocation, caching, logging, audit, and authorization-event requirements;
- a minimum OpenTEA implementation profile.

This specification does not define:

- how a bearer token is initially acquired;
- a mandatory identity provider;
- a mandatory external policy engine;
- a universal sales, CRM, or licensing integration protocol;
- new normative TEA consumer resource objects;
- the trustworthiness of artifact content;
- validation of artifact or collection signatures;
- transport-level event delivery formats beyond the authorization requirements stated here.

Authentication authorizes neither an artifact nor its content. Artifact and collection signatures, when present, MUST be validated independently according to their applicable trust model.

## 3. Conventions and Normative Language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHOULD**, **SHOULD NOT**, **MAY**, and **OPTIONAL** in this document are to be interpreted as described in BCP 14 when, and only when, they appear in all capitals.

Sections marked **Normative** define conformance requirements. Sections marked **Informational** provide rationale, examples, or implementation guidance.

Unless stated otherwise, identifiers in examples are illustrative.

## 4. Terminology

### 4.1 Normative

**Acting organization**  
The organization in whose context an authenticated principal performs an operation.

**Artifact**  
A software transparency artifact associated with a TEA product, release, or collection, such as an SBOM, VEX, VDR, CLE, or attestation.

**Audience group**  
An implementation-specific, publisher-managed set of consumer organizations that receive similar authorization. A customer group is a business-specific kind of audience group.

**Authentication authority**  
The authority that validates a credential or issues a credential accepted by the TEA service.

**Authorization authority**  
The authority that owns an authorization attribute, entitlement, template, or final decision.

**Authorization decision point**  
The component that determines whether an action on a resource is allowed.

**Authorization enforcement point**  
The TEA component that enforces an authorization decision on an API request, query result, event, or delivery.

**Capability**  
A named action that may be allowed or denied, such as `product.read` or `artifact.download`.

**Entitlement**  
A time-bounded assignment of capabilities or an access template from a subject to a resource scope.

**Organization**  
A company, customer, publisher, regulator, certification body, open-source project, public body, or other entity represented in the authorization system.

**Principal**  
An authenticated human or workload identity. Anonymous callers are represented by a built-in virtual subject rather than an authenticated principal.

**Principal group**  
An implementation-specific set of principals, normally within an organization.

**Product group**  
An implementation-specific, publisher-managed set of products or releases used for authorization administration. A product line may be represented by a product group.

**Release group**  
An implementation-specific set of releases for a single product, such as a major-version family or maintenance track.

**Resource scope**  
The set of resources to which an entitlement or rule applies.

**Template**  
A reusable, versioned set of capability decisions.

**TEA service**  
One administrative TEA security domain. It may be implemented by multiple logical or physical components, including publication, consumer-access, and notification services.

## 5. Design Principles

### 5.1 Normative

1. Authentication MUST be completed before claims from a credential are trusted.
2. Authentication and authorization MUST be treated as separate decisions.
3. Authorization MUST be evaluated for the requested capability and resource scope.
4. A valid credential MUST NOT by itself grant access to restricted TEA resources.
5. The absence of an applicable allow decision MUST result in denial.
6. Authentication MUST NOT reduce access to resources that are explicitly public.
7. Higher-level access MUST NOT automatically disclose lower-level resources. Product visibility does not imply collection or artifact visibility.
8. A subscription MUST NOT grant access to products, collections, artifacts, or events.
9. Authorization decisions MUST be deterministic and explainable to authorized administrators.
10. Revocation MUST affect API access, event access, and queued notification delivery within a documented maximum propagation interval.
11. Service-local UUIDs MUST be qualified by their issuing authority when used across TEA services or independently managed components.
12. Artifact content identity SHOULD use a cryptographic digest rather than a service-local UUID.

## 6. Authentication

### 6.1 Supported methods — Normative

A TEA implementation MAY permit anonymous access.

When a TEA implementation requires authentication for consumer access, it MUST support at least one of:

- HTTP bearer-token authentication; or
- mutual TLS with a verifiable client certificate.

An implementation MAY support both methods. It MAY use different methods for different TEA endpoints or clients, provided its service metadata and operational documentation state which methods are accepted.

### 6.2 Bearer tokens — Normative

A bearer token MUST be transmitted using the HTTP `Authorization` header and the `Bearer` scheme.

The token MAY be:

- a self-contained JWT;
- an opaque token validated through introspection or a local credential store; or
- another bearer-token representation accepted by the deployment.

For a JWT, the TEA service MUST validate at least:

- the signature using a trusted key and permitted algorithm;
- the token issuer;
- the intended audience;
- expiration and not-before constraints where present;
- required identity claims;
- revocation or authorization-revision constraints required by deployment policy.

The service MUST NOT accept an unsigned JWT or select a verification algorithm solely from untrusted token input.

The token issuer, accepted algorithms, audience values, clock-skew policy, and key-rotation behavior MUST be configured explicitly.

### 6.3 Mutual TLS — Normative

For mutual TLS, the TEA service MUST validate:

- the client certificate chain against configured trust anchors;
- certificate validity periods;
- intended client-authentication usage;
- revocation status when required by deployment policy;
- the mapping from certificate identity to a principal.

Clients MUST be able to configure credentials per TEA service and MUST NOT assume that a certificate accepted by one TEA service is accepted by another.

### 6.4 Principal identity — Normative

An externally authenticated principal MUST be keyed by both authentication authority and subject identifier. A bare username, email address, certificate common name, or `sub` value MUST NOT be assumed globally unique.

Credential rotation MUST NOT require changing the stable principal identity when the authenticated subject remains the same.

### 6.5 Acting organization — Normative

If a principal can act for more than one organization, the request context MUST identify one acting organization. The service MUST NOT silently combine the union of entitlements from all organization memberships.

The acting organization MAY be derived from a trusted token claim, credential mapping, endpoint context, or explicit request selection validated by the service.

## 7. Normalized Principal Context

### 7.1 Normative

Every supported authentication method MUST produce a normalized principal context before authorization evaluation.

The context MUST contain:

- authentication state;
- authentication method;
- authentication authority;
- stable principal identifier for authenticated requests;
- acting organization when required;
- credential or authentication-event identifier suitable for audit without exposing the credential;
- token or credential validity bounds where applicable.

The context MAY contain:

- organization memberships;
- principal-group or audience-group identifiers;
- template references and revisions;
- authorization revision;
- authentication assurance information;
- issuer-specific claims retained for authorized policy mapping.

An implementation MUST map issuer-specific claims into the normalized model through configured claim mappings. Application handlers SHOULD NOT directly interpret arbitrary issuer-specific JWT claim names.

Example normalized context:

    principalId: principal-123
    issuer: https://identity.example.com
    authenticationMethod: bearer
    actingOrganizationId: customer-acme
    principalGroupIds:
      - vulnerability-management
    templateRefs:
      - id: vex-consumer
        revision: 4
    authorizationRevision: 19

## 8. Authorization Deployment Profiles

### 8.1 General requirement — Normative

A TEA implementation MAY derive authorization information from validated credential claims, locally maintained authorization data, an external authorization decision service, or a combination of these sources.

The implementation MUST document which deployment profile it uses and MUST identify the authority for each authorization attribute.

### 8.2 Self-contained token profile — Normative

In this profile, a validated bearer token contains enough authorization information for local evaluation.

The implementation:

- MUST reject stale or unsupported template and authorization revisions according to documented policy;
- MUST enforce local emergency suspensions and resource restrictions even when the token contains a broad allow claim;
- SHOULD use short token lifetimes where rapid revocation is required;
- SHOULD use template, organization, or group references rather than embedding large product lists or complete policy documents.

### 8.3 Identity-only token with local authorization — Normative

In this profile, the credential establishes identity and the TEA service maintains authorization information.

The implementation:

- MUST map the external identity to one stable local principal;
- MUST maintain or synchronize current organization memberships and entitlements;
- MUST propagate policy changes to every enforcing component;
- MUST expose sufficient administrative information to reconcile external identity and local authorization records.

### 8.4 Hybrid profile — Normative

In this profile, the credential supplies some authorization attributes and the TEA service supplies resource-specific policy.

The implementation MUST define:

- which authority owns each attribute;
- precedence between token claims and local restrictions;
- authorization-revision comparison behavior;
- behavior when token and local data disagree;
- maximum acceptable staleness for externally supplied group membership.

A local emergency restriction or resource-specific restriction MUST be capable of denying access granted by a broader token claim.

### 8.5 External decision service profile — Normative

In this profile, a TEA enforcement point requests a decision from an external authorization decision point.

The implementation MUST:

- authenticate the external decision service;
- protect decision requests and responses in transit;
- include the principal, acting organization, capability, and sufficient resource context;
- validate the returned decision and policy revision;
- define timeout, cache, and failure behavior;
- support bulk or scope-based evaluation for list and search endpoints;
- avoid one external network request per list item where this would create unbounded latency or load.

## 9. Authorization Information Authorities

### 9.1 Normative

Each authorization-relevant attribute MUST have one defined authoritative owner.

A deployment SHOULD assign authority as follows unless its architecture documents another arrangement:

| Information | Typical authority |
|---|---|
| Principal identity | Identity provider or certificate authority mapping |
| Credential validity | Identity provider or local credential service |
| Organization membership | Identity provider or customer directory |
| Audience-group membership | Sales or entitlement system |
| Principal-group membership | Customer directory or local authorization service |
| Template definition | TEA authorization service |
| Product-group membership | TEA publisher |
| Release-group membership | TEA publisher |
| Contract validity | Sales or licensing system |
| Product and artifact visibility | TEA publisher |
| Emergency restriction | TEA service administrator |

Synchronized records MUST retain source-system identity, external identifier, source revision, and last successful reconciliation time where available.

## 10. Authorization Model

### 10.1 Subjects — Normative

The authorization model MUST support these virtual or concrete subject categories:

- `everyone`, including anonymous and authenticated callers;
- `authenticated`, including every successfully authenticated principal;
- organization;
- audience group;
- principal group;
- individual principal.

An implementation MAY omit audience groups or principal groups if it does not expose corresponding administrative functionality.

### 10.2 Organizations and groups — Normative

An audience group is publisher-managed and groups consuming organizations. A principal group groups principals, normally within an organization. These concepts MUST remain distinct.

Product groups, release groups, audience groups, and principal groups MUST NOT be nested in the minimum implementation profile.

Group membership changes MUST be versioned, audited, and propagated as authorization changes.

### 10.3 Decision states — Normative

A capability rule has one of:

- `allow`;
- `deny`; or
- no decision, also called `inherit` in administrative interfaces.

No decision is not an allow. If policy evaluation ends without an applicable allow, the request MUST be denied.

### 10.4 Public access — Normative

Public access SHOULD be represented by assignments to the `everyone` subject rather than by bypassing authorization enforcement.

Authenticated callers MUST retain access to resources permitted to `everyone`, subject to a more-specific valid restriction.

## 11. Resource Scopes

### 11.1 Normative

An entitlement or assignment MUST identify a resource scope. Supported scopes SHOULD include:

- all products owned by a publisher or TEA service;
- product group or product line;
- product type;
- specific product;
- release group or maintenance track;
- specific release;
- specific collection;
- specific artifact.

Product type is a controlled classification. An implementation MUST NOT evaluate authorization against arbitrary unvalidated type strings.

A product or release MAY belong to multiple groups. Group membership is dynamic unless the entitlement explicitly references a versioned snapshot supported by the implementation.

### 11.2 Product and release groups — Normative

Product and release groups are implementation constructs and MUST NOT modify TEA product or release identity, content, or signatures.

Only an authorized administrator for the owning publisher MUST be permitted to change product or release group membership.

Membership changes MUST:

- increment a membership revision;
- invalidate affected authorization scopes and caches;
- affect subsequent API, feed, and notification decisions;
- create an administrative audit record;
- create a durable authorization-change event.

### 11.3 Version selectors — Normative

Specific release identifiers and publisher-managed release groups SHOULD be preferred over free-form version-range expressions.

If an implementation supports version ranges, it MUST require a declared comparison scheme and MUST define prerelease, build, and non-semantic version behavior. A lexical string comparison MUST NOT be used as a general version ordering rule.

## 12. Capabilities

### 12.1 Normative

The minimum capability vocabulary is:

| Capability | Protected operation or information |
|---|---|
| `product.discover` | Product existence and minimal identity |
| `product.read` | Product metadata |
| `release.discover` | Release existence and minimal identity |
| `release.read` | Release metadata |
| `lifecycle.current.read` | Current lifecycle and support summary |
| `lifecycle.history.read` | Historical CLE information |
| `collection.discover` | Collection existence and minimal metadata |
| `collection.read` | Collection metadata and relationships |
| `collection.download` | Original collection or bundle content |
| `artifact.discover` | Artifact existence and classification |
| `artifact.metadata.read` | Artifact metadata, digest, format, and dates |
| `artifact.download` | Artifact content |
| `insight.query` | Query or compute insights over authorized data |
| `events.read` | Read authorized events |
| `events.subscribe` | Create an event subscription |
| `subscription.manage` | Manage a subscription or delivery destination |

Administrative capabilities SHOULD be separate from consumer capabilities. The minimum administrative vocabulary is:

| Capability | Administrative operation |
|---|---|
| `template.read` | Inspect templates |
| `template.manage` | Create or revise templates |
| `entitlement.read` | Inspect entitlements |
| `entitlement.manage` | Grant, change, suspend, or revoke entitlements |
| `productGroup.read` | Inspect product groups |
| `productGroup.manage` | Manage product groups and membership |
| `audienceGroup.read` | Inspect audience groups |
| `audienceGroup.manage` | Manage audience groups and membership |
| `principalMembership.manage` | Manage principal organization or group membership |
| `authorization.audit.read` | Read protected authorization audit information |

### 12.2 Capability independence — Normative

Access to a higher-level resource MUST NOT imply lower-level capabilities unless an active template or rule explicitly grants them.

In particular:

- product access MUST NOT imply collection or artifact access;
- release access MUST NOT imply artifact discovery;
- artifact discovery MUST NOT imply artifact metadata or download access;
- collection access MUST NOT imply access to every referenced artifact;
- lifecycle-summary access MUST NOT imply historical CLE or raw CLE access.

## 13. Templates

### 13.1 Normative

A template is a reusable, versioned set of capability decisions. A template MUST NOT itself identify a principal, organization, product, or release. A template assignment supplies the subject and resource scope.

A template MUST have:

- stable identifier;
- human-readable name;
- revision;
- status;
- capability rules;
- creation and activation audit metadata.

Changing a template MUST create a new revision. Activation of a new revision MUST be explicit, audited, and propagated as an authorization change.

Template inheritance SHOULD NOT be supported in the minimum implementation profile.

### 13.2 Template references in credentials — Normative

A credential MAY carry:

- a template identifier;
- a template identifier and revision; or
- embedded capability claims.

Template identifier and revision SHOULD be preferred. A TEA service MUST reject or safely refresh an unrecognized or stale required template revision.

An embedded token template or capability claim MUST NOT override a more-specific local product, release, collection, artifact, or emergency restriction.

### 13.3 Informational examples

Typical templates include:

- Public;
- Lifecycle Only;
- Metadata Only;
- VEX Access;
- SBOM and VEX Access;
- Full Transparency Access.

Example lifecycle-only template:

    product.discover: allow
    product.read: allow
    release.discover: allow
    release.read: allow
    lifecycle.current.read: allow
    lifecycle.history.read: deny
    collection.discover: deny
    artifact.discover: deny
    artifact.download: deny

## 14. Entitlements and Assignments

### 14.1 Normative

An entitlement or assignment MUST contain:

- stable identifier;
- subject type and identifier;
- template reference or explicit capability rules;
- resource scope;
- status;
- revision;
- validity start and optional validity end;
- granting authority or publisher;
- audit provenance.

It SHOULD contain an external commercial or administrative reference when created from another system.

Example conceptual entitlement:

    subject:
      type: organization
      id: customer-acme
    template:
      id: sbom-and-vex-access
      revision: 4
    resource:
      type: productGroup
      id: enterprise-firewall-v4
    validFrom: 2026-01-01T00:00:00Z
    validUntil: 2028-12-31T23:59:59Z
    externalReference:
      system: sales-platform
      type: contract
      id: "93841"
      revision: "7"

### 14.2 Overrides — Normative

An implementation MAY support scoped capability overrides. Overrides MUST identify a subject, resource scope, capability, effect, revision, validity, and audit provenance.

Individual-principal overrides SHOULD be exceptional. Ordinary commercial access SHOULD be assigned to organizations or audience groups.

An organization administrator MAY narrow access for its principals or service accounts but MUST NOT broaden access beyond the organization's effective entitlement.

## 15. Policy Evaluation

### 15.1 Normative algorithm

For each requested capability, the enforcement point MUST:

1. Validate authentication if authentication is presented or required.
2. Construct the normalized principal and acting-organization context.
3. Resolve the protected resource and its authorization classifications without disclosing them to the caller.
4. Apply active emergency restrictions.
5. Collect applicable public, authenticated, organization, group, principal, template, entitlement, and override decisions.
6. Remove expired, not-yet-valid, suspended, stale, or otherwise inactive decisions.
7. Rank decisions by resource specificity.
8. Within equal resource specificity, rank decisions by subject specificity.
9. At equal resource and subject specificity, resolve `deny` over `allow`.
10. Deny if no applicable allow remains.
11. Record or correlate the decision according to Section 22.

### 15.2 Resource specificity — Normative

Resource specificity from broadest to narrowest is:

    all products
      < product type or product group
      < specific product
      < release group
      < specific release
      < specific collection or artifact

Product type and product group are equal specificity. When a resource matches conflicting rules at that level, `deny` MUST take precedence unless a narrower resource rule applies.

### 15.3 Subject specificity — Normative

Subject specificity from broadest to narrowest is:

    everyone
      < authenticated
      < organization or audience group
      < principal group
      < individual principal

Organization and audience group are equal specificity. Conflicting decisions from multiple equal-specificity groups MUST resolve to `deny`.

A broad individual-principal allow MUST NOT defeat a narrower product or release restriction. Resource specificity is evaluated before subject specificity.

### 15.4 Explainability — Normative

The authorization system MUST be able to produce an administrative explanation containing:

- principal and acting organization;
- capability;
- resource scope;
- decision;
- matched template, entitlement, group, and override references;
- policy and membership revisions;
- stable reason code.

The consumer response MUST NOT expose an explanation that reveals unauthorized resource existence or another organization's policy.

## 16. Product and Release Access

### 16.1 Normative

Authorization MAY grant product access by:

- all-products scope;
- product type;
- product group or product line;
- specific product;
- release group or version family;
- specific immutable release.

When a principal has access only to selected releases, the service MAY expose the minimum parent-product information required to interpret those releases. It MUST filter inaccessible releases, counts, latest-version indicators, relationships, and events.

A newly published release MUST NOT automatically become accessible unless it matches an active dynamic scope or is explicitly added to an entitled scope.

### 16.2 Informational commercial mappings

Common mappings include:

| Commercial rule | Authorization scope |
|---|---|
| Exact purchased version | Specific release |
| Purchased minor version plus patches | Release group |
| All releases in a major version | Release group |
| Active subscription to a product | Product plus entitlement validity |
| Product-line contract | Product group |
| Lifecycle information for all products | All-products scope with lifecycle-only template |

## 17. Collection and Artifact Access

### 17.1 Normative

Collection and artifact authorization MUST be evaluated independently of product and release visibility.

An artifact rule MAY be constrained by a controlled artifact classification, such as:

- SBOM;
- VEX;
- VDR;
- CLE;
- attestation;
- other implementation-defined controlled type.

A user MAY therefore be allowed to read products, releases, and current lifecycle information while being unable to discover or download SBOMs.

### 17.2 Shared artifacts — Normative

When an artifact is associated with several products or releases, access MUST be based on at least one currently authorized relationship and the required artifact capability.

The response MUST filter relationships to unauthorized products and releases. Access through one relationship MUST NOT disclose other protected relationships.

### 17.3 Signed collections — Normative

Filtering a signed collection changes its representation and invalidates the source signature.

The service MUST either:

- permit access to the complete signed representation;
- deny access to that representation; or
- return an explicitly derived representation with distinct identity and signature status.

The service MUST NOT remove restricted members while presenting the original signature as valid for the modified representation.

In the minimum implementation profile, `collection.download` SHOULD be allowed only when the principal may access every member embedded in the downloaded collection.

## 18. Discovery, Listing, and Query Filtering

### 18.1 Normative

Authorization MUST be applied before returning:

- exact identifier lookup results;
- search results;
- list entries;
- relationship links;
- counts and pagination totals;
- latest-version or latest-artifact indicators;
- artifact type summaries;
- lifecycle summaries;
- insight or expression-query results.

Unauthorized and nonexistent restricted resources SHOULD produce externally indistinguishable responses unless deployment policy explicitly permits existence disclosure.

An unauthorized resource SHOULD normally be omitted from lists and return `404 Not Found` on direct lookup.

Query execution, including CEL or another expression language, MUST operate on an authorization-filtered dataset. Query errors, timing, grouping, and counts MUST NOT be allowed to reveal protected resources.

### 18.2 Pagination — Normative

Pagination MUST be applied to the authorized result set. A page MUST NOT expose hidden entries through unfiltered totals, stable gaps, or cursor contents.

## 19. Event and Notification Authorization

### 19.1 Normative

Event existence and metadata are protected information. Fetch authorization alone is not sufficient.

The effective event set MUST be:

    requested filter
      INTERSECT current subject entitlements
      INTERSECT event visibility

An event subscription MUST record the stable principal and acting-organization context. It MUST NOT retain the original bearer token as a permanent authorization credential.

Authorization MUST be re-evaluated when:

- a feed request is served;
- a cursor is replayed;
- a webhook or other push delivery is attempted;
- a queued delivery is retried.

Revocation MUST prevent subsequent delivery and replay of newly unauthorized events.

### 19.2 Capability mapping — Normative

Event authorization MUST correspond to the affected resource layer. At minimum:

| Event information | Required capability |
|---|---|
| Product update | `product.read` |
| Support-status change | `lifecycle.current.read` |
| Release publication | `release.read` |
| Collection publication | `collection.discover` |
| Artifact publication | `artifact.discover` for the artifact type |
| Artifact detail or digest | `artifact.metadata.read` |
| Artifact download link | `artifact.download` |

A lifecycle-only consumer MUST NOT receive events that reveal restricted SBOM or VEX existence.

### 19.3 Cursors — Normative

An authorized event-feed cursor MUST be opaque, integrity protected, scoped to the acting organization and principal or equivalent authorization scope, bound to filter semantics, and associated with an authorization revision.

A cursor MUST NOT act as a credential. Every feed request MUST authenticate when its event scope requires authentication.

Raw global sequence values MUST NOT be exposed where sequence gaps would reveal hidden events.

## 20. Revocation, Freshness, and Caching

### 20.1 Normative

Every implementation MUST define:

- the maximum accepted age of authorization information;
- token and decision-cache lifetimes;
- policy and membership revision behavior;
- revocation propagation objectives;
- behavior when freshness cannot be established.

Restricted access MUST fail closed when required authorization state is unavailable or unacceptably stale.

### 20.2 Profile-specific mechanisms — Normative

Self-contained JWT deployments SHOULD combine short token lifetimes with a deny list, authorization revision, or equivalent urgent revocation mechanism.

Local authorization deployments MUST invalidate affected caches when local policy changes.

Hybrid deployments MUST compare relevant token and local authorization revisions according to documented policy.

External decision deployments MUST honor decision expiry or `cacheUntil` constraints and MUST NOT extend an allow decision beyond the authority's permitted cache duration.

### 20.3 Notification revocation — Normative

Queued notifications MUST reference current subscription and authorization state. A queued job SHOULD reference an immutable event and subscription rather than embed a previously authorized sensitive payload.

Revocation MUST cancel or deny subsequent retries.

## 21. Authorization Administration and Commercial Integration

### 21.1 Normative

Authorization administration APIs MUST be separate from the interoperable TEA consumer API unless a future TEA specification standardizes them.

Administrative operations MUST require explicit administrative capabilities and MUST be audited.

The system MUST support idempotent creation and update of externally managed entitlements. Externally provisioned records SHOULD retain:

- source system;
- external object type and identifier;
- source revision;
- synchronization timestamp;
- local revision;
- provenance or reason.

### 21.2 Informational producer workflow

A typical producer integration is:

    Sales, CRM, or licensing platform
      -> producer-specific adapter
      -> TEA entitlement administration API
      -> authorization database and outbox
      -> consumer and notification enforcement points

The external adapter translates commercial SKUs and contracts into TEA implementation concepts. The TEA service does not need to interpret invoices or sales-platform-specific objects.

For example:

    SKU FIREWALL-ENT-V4
      -> product group Enterprise Firewall 4.x

    Contract feature SECURITY-DOCUMENTS
      -> template SBOM and VEX Access

    CRM account ACME-1942
      -> organization customer-acme

### 21.3 Reconciliation — Normative

Where authorization is synchronized from an external system, the integration MUST support periodic reconciliation in addition to incremental updates.

Urgent revocation SHOULD be propagated immediately and MUST NOT wait solely for periodic reconciliation.

## 22. Logging and Audit

### 22.1 Log separation — Normative

A conforming service MUST distinguish:

- operational logs;
- security audit records;
- authorization-change events;
- consumer publication events.

These streams MAY share storage or transport but MUST have distinct schemas, access controls, and retention policies.

### 22.2 Correlation — Normative

Every API request MUST have a server-generated request identifier. Distributed operations SHOULD carry a trace or correlation identifier across authentication, authorization, data access, external decision, and notification components.

### 22.3 Authentication records — Normative

Security-relevant authentication outcomes MUST be recorded with:

- timestamp;
- authentication method;
- issuer or trust authority when known;
- stable principal identifier on success;
- safe credential identifier or fingerprint where available;
- result;
- structured reason code;
- request or correlation identifier;
- source network information according to privacy policy.

Supported internal reason codes SHOULD distinguish missing, malformed, expired, revoked, untrusted, invalid-audience, invalid-signature, and disabled-principal conditions.

External responses MUST NOT disclose diagnostic detail that assists credential attacks.

### 22.4 Authorization decision records — Normative

Authorization decisions for protected downloads, restricted events, administrative operations, and security-sensitive queries MUST be recorded or durably correlated with:

- decision identifier;
- timestamp;
- request or correlation identifier;
- principal and acting organization;
- capability;
- protected resource type and identifier;
- allow or deny result;
- stable reason code;
- authorization source;
- matched policy, template, entitlement, and group references as applicable;
- policy and membership revisions;
- external decision identifier and cache status where applicable.

Routine authorized list and metadata reads MAY use aggregate or lower-cost access logging according to documented policy. Administrative changes and restricted artifact downloads MUST NOT be sampled out of the audit trail.

### 22.5 Administrative audit — Normative

The service MUST audit:

- template lifecycle changes;
- entitlement grants, modifications, suspension, expiration, and revocation;
- organization, audience-group, principal-group, product-group, and release-group membership changes;
- public or restricted visibility changes;
- principal or credential suspension and revocation;
- emergency restrictions;
- trusted issuer, claim mapping, authorization provider, signing-key, and trust-anchor changes;
- subscription and delivery-destination changes.

An administrative audit record MUST identify the actor, acting organization, operation, target, previous and resulting revision or state, timestamp, request identifier, source system, and reason where required.

Authorization-state mutations MUST atomically create an immutable audit record and authorization outbox event. If either record cannot be committed, the mutation MUST fail.

### 22.6 Prohibited log content — Normative

The following MUST NOT be logged:

- complete bearer or refresh tokens;
- authorization headers;
- JWT signatures as reusable credential material;
- client secrets;
- private keys;
- webhook secrets;
- reusable session identifiers;
- protected cursor contents;
- signed download URLs;
- complete protected artifact or collection bodies.

Personal information in logs MUST be minimized. Stable internal principal identifiers SHOULD be used instead of email addresses.

### 22.7 Retention and access — Normative

The operator MUST document retention separately for operational, authentication, authorization-decision, administrative-audit, artifact-download, authorization-event, and dead-letter records.

Audit data MUST be access controlled, encrypted in transit, and protected at rest according to deployment policy. Customer administrators MUST NOT be able to inspect another customer's authorization or activity.

## 23. Authorization Events

### 23.1 Normative

Authorization changes that affect another enforcement point MUST create durable authorization events.

Events MUST be:

- immutable;
- uniquely identified;
- associated with subject or scope revisions;
- suitable for at-least-once delivery;
- idempotently processable;
- replayable for a documented retention period;
- isolated from consumer publication events.

Recommended event types include:

    authorization.templateUpdated
    authorization.templateActivated
    authorization.entitlementCreated
    authorization.entitlementUpdated
    authorization.entitlementSuspended
    authorization.entitlementRevoked
    authorization.entitlementExpired
    authorization.organizationMembershipChanged
    authorization.principalGroupMembershipChanged
    authorization.audienceGroupMembershipChanged
    authorization.productGroupMembershipChanged
    authorization.releaseGroupMembershipChanged
    authorization.principalSuspended
    authorization.credentialRevoked
    authorization.resourceVisibilityChanged
    authorization.emergencyRestrictionApplied
    authorization.scopeRevisionChanged

### 23.2 Event contents — Normative

An authorization event SHOULD contain:

- specification version;
- event type;
- event identifier;
- occurrence time;
- source authority;
- actor reference;
- affected authorization object and revision;
- affected authorization-scope identifiers;
- new scope revision;
- reason code;
- correlation identifier.

It SHOULD NOT embed a complete customer contract, token, or sensitive policy document. A consumer requiring full state SHOULD retrieve it from an authoritative administrative interface.

### 23.3 Processing — Normative

Authorization-event consumers MUST process events idempotently and MUST NOT replace newer authorization state with an older revision.

Authorization events MUST be used as applicable to:

- invalidate authorization caches;
- update consumer and notification authorization projections;
- invalidate or restart authorization-bound event scopes;
- cancel queued notifications;
- revoke or expire derived download access;
- update search visibility;
- monitor synchronization lag.

The implementation MUST define a maximum propagation interval for revocation events.

### 23.4 Security events — Normative

The service SHOULD produce restricted security events for:

- authentication-failure spikes;
- authorization-denial spikes;
- identity or authorization provider unavailability;
- policy-revision mismatches;
- audit-pipeline failure;
- authorization-event lag exceeding policy;
- suspected credential replay or resource enumeration;
- application of emergency restrictions.

Security events MUST NOT appear in consumer publication feeds.

## 24. Failure Handling

### 24.1 Normative

When authentication cannot be established for a protected request, access MUST be denied.

When current authorization cannot be established for a restricted resource, access MUST be denied unless a still-valid cached allow decision is explicitly permitted by policy.

Public access MAY continue during authorization-provider failure only when public classification is locally authoritative and does not depend on the unavailable provider.

An implementation MUST define behavior for:

- signing-key retrieval failure;
- token introspection failure;
- local authorization-store failure;
- external decision timeout or protocol error;
- stale authorization revision;
- event-projection lag;
- audit-pipeline failure;
- event cursor expiration;
- group-membership inconsistency.

Administrative authorization mutations MUST fail if their audit and outbox records cannot be committed.

## 25. Privacy and Security Considerations

### 25.1 Normative

Authorization protects resource existence as well as resource content. Product names, release timing, artifact types, support state, vulnerability activity, event sequence gaps, and relationship metadata MAY be sensitive.

The implementation MUST protect against:

- resource and identifier enumeration;
- unauthorized relationship disclosure;
- cross-tenant cache reuse;
- query-based inference;
- stale-token and stale-cache access;
- privilege escalation through group or template administration;
- confused-deputy behavior when a principal belongs to several organizations;
- logging of credentials or protected content;
- delivery of queued events after revocation.

Rate limiting SHOULD use principal, organization, credential, and endpoint context rather than relying only on source IP addresses.

Protected responses SHOULD use private cache controls. Cache keys for authorization-sensitive representations MUST include an appropriate authorization scope or MUST avoid shared caching.

### 25.2 External authorization privacy — Normative

Requests to an external authorization service MUST disclose only the principal and resource attributes required for the decision. Complete artifact content MUST NOT be transmitted merely to obtain an authorization decision.

## 26. Conformance Requirements

### 26.1 Normative

A conforming implementation MUST document:

- accepted authentication methods;
- trusted issuers or certificate authorities;
- bearer-token validation requirements;
- authorization deployment profile;
- attribute authorities and claim mappings;
- supported subjects, scopes, capabilities, and templates;
- policy precedence and conflict behavior;
- default unauthorized response behavior;
- authorization freshness and revocation objectives;
- audit retention and access controls;
- authorization-event retention and propagation objectives;
- unsupported optional features.

A conforming implementation MUST pass tests demonstrating:

- invalid, expired, wrong-audience, and untrusted credentials are rejected;
- issuer and subject together identify the principal;
- unauthenticated callers receive only public access;
- product access does not imply artifact access;
- release-scoped access hides other releases;
- collection and artifact relationships are filtered;
- conflicting equal-specificity rules resolve to deny;
- a narrower resource restriction defeats a broader grant;
- group removal and entitlement revocation affect reads, feeds, and queued notifications;
- old cursors cannot recover newly unauthorized events;
- signed collections are not silently filtered while retaining the source signature;
- administrative changes atomically create audit and authorization-event records;
- logs contain no complete credentials or protected artifact bodies.

## 27. Implementation Profile for OpenTEA

### 27.1 Normative profile

The initial OpenTEA implementation SHOULD implement the following constrained profile:

- bearer-token and mutual-TLS authentication, with deployments permitted to enable either or both;
- normalized principal identity using authentication authority plus subject;
- built-in `everyone` and `authenticated` subjects;
- organization and individual-principal subjects;
- optional non-nested audience and principal groups;
- all-products, product-group, specific-product, release-group, and specific-release scopes;
- flat, versioned templates without inheritance;
- explicit `allow`, `deny`, and absent/inherit rule states;
- resource specificity before subject specificity;
- deny precedence at equal specificity;
- default deny;
- organization-first commercial entitlements;
- artifact-type constraints for SBOM, VEX, VDR, CLE, attestation, and other controlled types;
- idempotent administrative integration with external sales or entitlement systems;
- transactional audit and authorization outbox creation;
- authorization-aware event feeds and delivery-time notification checks.

The initial implementation SHOULD defer:

- nested groups;
- arbitrary policy expression languages;
- template inheritance;
- unrestricted semantic-version expressions;
- routine per-artifact UUID grants;
- complex policy priorities;
- customer-defined grants that broaden publisher entitlements.

## 28. Examples

### 28.1 Public lifecycle with restricted artifacts — Informational

Global assignment:

    subject: everyone
    scope: all products
    template: Public Lifecycle

Template:

    product.discover: allow
    product.read: allow
    release.discover: allow
    release.read: allow
    lifecycle.current.read: allow
    collection.discover: deny
    artifact.discover: deny
    artifact.download: deny

An anonymous consumer can find products and support status but cannot determine whether an SBOM exists.

### 28.2 Product-group restriction — Informational

Global public access is overridden for an embargoed product group:

    subject: everyone
    scope: productGroup/embargoed-products
    overrides:
      product.discover: deny
      product.read: deny
      release.discover: deny
      artifact.discover: deny
      events.read: deny

The products are omitted from discovery and direct unauthorized lookup does not confirm their existence.

### 28.3 Commercial VEX access — Informational

    subject:
      type: organization
      id: customer-acme
    template: VEX Access
    scope:
      type: releaseGroup
      id: enterprise-firewall-v4
    validUntil: 2028-12-31T23:59:59Z

The template permits product and release metadata, lifecycle information, VEX discovery and download, and matching events. It denies SBOM discovery and download.

### 28.4 Principal restriction — Informational

Acme has full artifact access, but a monitoring service account is restricted to lifecycle and VEX:

    subject:
      type: principal
      id: monitoring-service
    scope:
      type: productGroup
      id: enterprise-products
    overrides:
      artifact.download[sbom]: deny
      artifact.download[vex]: allow

The principal-level override narrows the organization's group assignment. It cannot broaden access beyond Acme's effective entitlement.

### 28.5 Hybrid JWT and local policy — Informational

Validated JWT claims:

    issuer: https://identity.example.com
    subject: service-account-123
    audience: tea-consumer
    organization: customer-acme
    groups:
      - gold-customers
    authorizationRevision: 19

Local TEA policy:

    Gold Customers
      -> VEX Access
      -> Enterprise Products

    Product Secret Appliance
      -> everyone and Gold Customers
      -> product.discover: deny

The broad token group claim does not override the local product-specific restriction.

### 28.6 Authorization decision explanation — Informational

    ALLOW artifact.download for artifact VEX-123

    Principal service-account-123 acts for customer-acme.
    customer-acme belongs to audience group Gold Customers.
    Gold Customers has template VEX Access for Enterprise Products.
    The artifact is a VEX associated with an entitled release.
    No more-specific product, release, artifact, or principal rule denies access.
    Policy revision: 38.

The consumer receives only the normal API result. The detailed explanation is available to an authorized administrator and in protected audit records.

## 29. Open Questions

### 29.1 Informational

Before this working draft becomes stable, the project should decide:

- the exact OpenTEA administrative API objects and paths;
- whether audience groups are required in the first release;
- whether release groups may contain releases only or also products;
- maximum authorization and revocation propagation intervals;
- whether all protected metadata reads require individual audit records;
- supported external decision-service protocol or adapter interface;
- token claim namespace and claim-mapping configuration format;
- whether template assignments follow the active revision or may be revision-pinned;
- lifecycle and deletion behavior for archived groups;
- required audit and authorization-event retention periods;
- whether a public anonymous event feed is included in the initial implementation.

## 30. References

### 30.1 Normative references

- RFC 2119, *Key words for use in RFCs to Indicate Requirement Levels*.
- RFC 8174, *Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words*.
- RFC 6750, *The OAuth 2.0 Authorization Framework: Bearer Token Usage*.
- RFC 7519, *JSON Web Token (JWT)*.
- RFC 5280, *Internet X.509 Public Key Infrastructure Certificate and Certificate Revocation List Profile*.

### 30.2 Informational references

- CycloneDX Transparency Exchange API, `auth/readme.md`.
- CycloneDX Transparency Exchange API, `doc/tea-usecases.md`.
- CycloneDX Transparency Exchange API OpenAPI specification.
- OpenTEA Consumer Notification System Proposal.
