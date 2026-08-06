---
name: dependency-review
description: Use before adding a new third-party Go dependency to opentea, or after any go.mod/go.sum change, to evaluate the dependency, regenerate the project's three CycloneDX SBOMs (server, client, shared), and validate them (schema, referential integrity, CPE coverage).
---

# Dependency Review

Three jobs, always done together: evaluate a third-party Go dependency
against this project's selection criteria, keep the three CycloneDX SBOMs
under `sbom/` in sync with what's actually being built, and validate what
got generated rather than trusting it by construction.

## When to use this

- Before adding a new third-party dependency (`go get` introducing something
  not already in `go.mod`).
- After any change to `go.mod`/`go.sum` (a version bump counts too).
- On request, to audit an existing dependency against the criteria below.

## Selection criteria

Apply these, in order, before adding anything:

1. **Standard library first.** If `crypto/sha3`, `net/http`, etc. already do
   the job, use it -- even if it's a few more lines than a library call.
   `internal/idgen` hand-rolls UUIDv4 generation specifically to avoid a
   dependency; this project already leans this way.
2. **Justify the addition.** A dependency earns its place by saving real
   complexity or risk, not just typing. A ~20-line hand-rolled helper usually
   isn't worth a new supply-chain dependency; JSON Schema validation
   (`santhosh-tekuri/jsonschema`) clearly is.
3. **Maintenance and trust signals.** Active releases, real usage elsewhere,
   no unresolved CVEs (`govulncheck -mode=binary` against a built binary is
   the fast check -- see the earlier security review in this project's
   history for how). Check *subpackages* individually, not just the module:
   `golang.org/x/crypto` as a whole is trustworthy, but its `openpgp`
   subpackage is flagged unmaintained (GO-2026-5932) -- the umbrella
   module's reputation doesn't cover every package inside it.
4. **Minimal transitive footprint.** Fewer dependencies-of-dependencies means
   less audit surface. `modernc.org/sqlite` (pure Go, no cgo) is the existing
   example: it avoids a whole C-toolchain dependency at the cost of a
   handful of small `modernc.org/*` helper modules.
5. **Narrow scope over frameworks.** Prefer a library that does the one
   thing needed, not a multi-purpose toolkit that drags in far more surface
   than the task requires.
6. **License compatibility.** Check explicitly; don't assume.

If a candidate fails #1-#2, don't add it -- write the ~20 lines instead.
If it fails #3-#6, look for an alternative or flag the tradeoff explicitly
when proposing the addition.

## SBOM layout

Three separate SBOMs, not one, because the three targets have genuinely
different dependency closures (verify this is still true before assuming
it -- see "if the shape changes" below):

| File | Scope | Generator |
|---|---|---|
| `sbom/server.cdx.json` | Everything `cmd/opentea` (the server binary) actually depends on | `cyclonedx-gomod app -main cmd/opentea` |
| `sbom/client.cdx.json` | Everything `cmd/teaclient` (the client CLI) actually depends on | `cyclonedx-gomod app -main cmd/teaclient` |
| `sbom/shared.cdx.json` | `pkg/tea` only -- the one package genuinely imported by both server and client. Zero third-party dependencies today, so this SBOM has an empty `components` list; that's the accurate, intended state, not a bug. | hand-written script, see below |

`pkg/teaclient` (the client *library*, as opposed to the `cmd/teaclient`
CLI) is deliberately **not** part of the shared SBOM -- it's client-only
(the server never imports it), and today it's nearly identical to the
client SBOM anyway (`golang.org/x/crypto/blake2b` is its only real
third-party dependency). If that ever changes such that `pkg/teaclient` is
imported from server-side code too, revisit this split.

### Prerequisite

`cyclonedx-gomod` (the official CycloneDX Go SBOM generator) must be on
`PATH`. If missing:

```sh
go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
```

### Full sequence

Order matters: CPE-checking annotates the already-generated SBOM files in
place, so it must run *after* generating and *before* validating (which
should check the final state, not an intermediate one).

```sh
./.claude/skills/dependency-review/scripts/generate-server-sbom.sh
./.claude/skills/dependency-review/scripts/generate-client-sbom.sh
python3 .claude/skills/dependency-review/scripts/generate-shared-sbom.py
python3 .claude/skills/dependency-review/scripts/check-cpe-coverage.py
./.claude/skills/dependency-review/scripts/validate-sbom.sh
```

Run the whole sequence after any `go.mod`/`go.sum` change, even if it looks
like it only touches one target -- a shared dependency version bump can
affect more than one SBOM, and it's cheap to just regenerate all three.

**If you ever regenerate just one SBOM on its own** (skipping the full
sequence), re-run `check-cpe-coverage.py` afterward too: `generate-*`
overwrites the file from scratch via `cyclonedx-gomod`/hand-written JSON,
which has no notion of the `opentea:cpeChecked` properties the coverage
check previously added, so those get silently dropped otherwise.

### CPE and purl coverage

Every component gets a **purl** automatically (`cyclonedx-gomod` derives it
from the module path/version -- verified present on every real dependency
across all three SBOMs). It's trustworthy by construction (read straight
off the real, resolved module graph), so unlike CPE below there's no
separate "was this purl verified" step or annotation for it.

Nothing gets a **CPE** automatically -- purl identifies a component
*within its package ecosystem* (Go modules, here); CPE identifies it in
the vendor/product taxonomy NVD and most vulnerability databases key off
of, a different and complementary purpose. `cyclonedx-gomod` has no flag
for it; Go module paths don't map onto CPE's vendor:product structure
automatically.

Check coverage against the [GCVE CPE database](https://cpe.gcve.eu) (a
community-curated CPE dictionary with an open API, including purl-to-CPE
mappings for ecosystems where that mapping has been curated):

```sh
python3 .claude/skills/dependency-review/scripts/check-cpe-coverage.py
```

This does two things, matching the convention the kamailio project (a
prior, unrelated SBOM effort) established with its `kamailio:cpeSource`
property -- the check outcome belongs *on the component, inside the SBOM
itself*, not only in a side report a downstream consumer might not know to
cross-reference:

1. Sets an `opentea:cpeChecked` property on every component recording what
   was found (`"<date>, https://cpe.gcve.eu, no match"`, or, if the lookup
   returned candidates, a note pointing at the report below -- never an
   actual `cpe` value; see the manual-verification requirement below).
2. Writes `sbom/cpe-coverage.md`, a human-readable summary of the same
   per-component data -- convenient for review, not the source of truth.

As of 2026-08-06, all 10 of opentea's real third-party dependencies have
**zero** matches in that database. This is an honest, expected result, not
a script bug: CPE coverage skews toward traditional/commercial software,
and the Go module ecosystem (especially smaller, single-purpose libraries
like `modernc.org/mathutil` or `santhosh-tekuri/jsonschema`) is thinly
covered. Confirmed the API itself works correctly by testing a known-mapped
package (`pkg:npm/lodash`, which does return a match) before trusting the
zero-results for our own dependencies.

**If a future dependency (or a version bump) does return candidates, do
not auto-apply the top match.** The single biggest source of wrong SBOM
data in prior work on a different project was exactly that shortcut: when
there's no exact self-titled vendor:product match, falling back to "pick
the candidate with the most package-registry mappings" reliably produces
*confidently wrong* results for any vendor namespace with many unrelated
products -- it'll happily match your library to an unrelated, more
popular product under the same vendor umbrella. Instead:

1. Search/filter to candidates where the **product name matches your
   dependency exactly** (normalize case/hyphens/underscores first), not
   just the vendor.
2. Treat each candidate's purl mappings (which real registries/repos it
   points to) as evidence to inspect, not a ranking signal -- don't pick
   by match count.
3. Only once a candidate's evidence genuinely confirms it's the same
   software: add its `cpe_uri` to that component's `"cpe"` field by hand in
   the relevant `sbom/*.cdx.json`, **and** replace its `opentea:cpeChecked`
   property with an `opentea:cpeSource` one recording how it was verified
   (e.g. `"https://cpe.gcve.eu (product_q exact match, manually verified
   <date>)"` -- exactly kamailio's convention), then re-run the validator.
4. A community aggregator's chosen identifier can also disagree with
   whatever an authoritative vulnerability database (e.g. NVD) actually
   uses for the same product -- if you're about to cross-reference a CVE
   database by CPE, verify the identifier matches what that database
   itself expects, not just what GCVE returned. A near-zero vulnerability
   count for a widely-deployed library is a signal to double check the
   identifier, not a finding to report as "clean."

### Validating

Run after generating *and* after CPE-checking (see "Full sequence" above
for why the order matters):

```sh
./.claude/skills/dependency-review/scripts/validate-sbom.sh
```

This checks two things `cyclonedx-gomod` (or `check-cpe-coverage.py`)
producing well-formed-looking JSON does *not* guarantee on its own:

1. **Schema conformance** against the real CycloneDX 1.6 JSON Schema
   (`assets/schemas/bom-1.6.schema.json`, fetched once from the official
   CycloneDX specification repo, plus the two schemas it `$ref`s --
   `spdx.schema.json`, `jsf-0.82.schema.json`). Reuses this project's own
   `santhosh-tekuri/jsonschema/v5` dependency (already used by
   `internal/bundle/schema.go`) rather than adding a new one -- same rule
   #2 as this skill's own selection criteria above. This is not
   theoretical: it caught a real violation in `generate-shared-sbom.py`'s
   first draft (a `description` field on a legacy `tools[]` entry, which
   `#/definitions/tool`'s `additionalProperties: false` rejects) that
   "the JSON parses fine" would never have surfaced.
2. **Referential integrity**: every `dependencies[].ref` and
   `dependencies[].dependsOn[]` entry actually matches a component's (or
   the root's) `bom-ref`. The schema treats these as free-form strings, so
   a dangling reference passes schema validation cleanly -- this needs a
   separate check.

If the CycloneDX schema files under `assets/schemas/` are ever missing or
need updating to a newer spec version, re-fetch from
`https://raw.githubusercontent.com/CycloneDX/specification/master/schema/`
(`bom-<version>.schema.json`, `spdx.schema.json`, `jsf-0.82.schema.json`)
and update `validate-sbom.go`'s hardcoded `"bom-1.6.schema.json"` /
`http://cyclonedx.org/schema/bom-1.6.schema.json` references to match.

### If pkg/tea ever gains a third-party dependency

`generate-shared-sbom.py` deliberately **fails loudly** (non-zero exit, an
error listing what it found) rather than silently emit an incomplete SBOM,
because neither of `cyclonedx-gomod`'s modes scope cleanly to a single
non-main package: `app` mode requires a main package (`pkg/tea` isn't one),
and `mod` mode is scoped to the whole module (this repo is one module, so
that means everything, not just `pkg/tea`).

If that happens:

1. Run `cyclonedx-gomod mod -json -output /tmp/full-mod.json .` to get a
   correctly-formed CycloneDX component for the new dependency (accurate
   purl, license detection if used, etc.) -- don't hand-write component
   entries for real dependencies, only the trivial empty-list case is
   hand-written.
2. Copy that dependency's `components` entry (and any of its own transitive
   dependencies pkg/tea also pulls in) into `sbom/shared.cdx.json`,
   confirmed against `go list -deps ./pkg/tea/...`'s actual output.
3. Consider whether `generate-shared-sbom.py` is worth extending to do this
   automatically -- only if this becomes a recurring situation, not
   speculatively.

### Verifying the split still holds

The three-way split assumes `pkg/tea` has no third-party dependencies and
`cmd/teaclient`'s dependencies are a strict superset of nothing beyond
`pkg/teaclient`'s own. Spot-check with:

```sh
go list -deps ./pkg/tea/... | grep -v '^github.com/oej/opentea' | grep '\.'
```

An empty result confirms the shared SBOM's "zero components" state is still
accurate (any non-stdlib import path contains a dot, e.g. `golang.org/...`
or `github.com/...`; stdlib paths never do).
