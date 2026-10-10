# TEA consumer API (`/tea/v1`) conformance test rig

A methodology and reference dataset for client-side conformance testing of the
**standard** TEA 1.0 consumer read API, against any server claiming to implement it — not
tied to any one implementation. Companion to `docs/bundle-import-export-test-rig.md`,
which describes the same reference data from the import/export side; this document
describes the data from the read side and the test methodology itself, in terms of the
specification only.

**Scope boundary, deliberate**: TEA Trust Architecture (evidence bundles / evidence bundle
references) is excluded — it is not part of the official TEA 1.0 consumer specification.
**Standard artifact/distribution signatures are in scope** — a format's or distribution's
signature URL, and the signature-download operations, are part of the specification
itself.

**This suite tests the standard, not any one implementation of it.** Every check is
grounded in what the TEA 1.0 specification actually requires. A server that behaves
differently from one particular reference implementation, but still conforms to the
specification, must pass. A server that matches one implementation's behavior but
violates the specification must fail.

## The one hard rule this whole suite is built around

Nothing in the TEA ecosystem guarantees that an object's identity (its UUID) is preserved
when data is transferred between servers — an export/import mechanism is free to assign
new identities on import, and a conformant consumer-facing server is not required to
expose any mapping back to whatever identity the data may have carried on another server.
**The suite never assumes an object's identity is known in advance.** Two independent
mechanisms resolve it instead, used together as a cross-check against each other, not as
a single point of failure:

1. **Identifier-based resolution (primary).** Every object in the reference dataset
   carries a stable, deterministic identifier of a kind the specification itself defines
   for exactly this purpose — a Package URL (PURL), or a Transparency Exchange Identifier
   (TEI) where a PURL doesn't fit. The suite resolves each object's real identity on the
   server under test via the specification's own identifier-filtered query operations
   (filtering a collection-of-objects listing by identifier type and value) before issuing
   any further call that references it. Exercising this filter at all is itself part of
   the endpoint-coverage score below, not just internal plumbing.
2. **Embedded test markers (cross-check).** Every free-text, non-normative field the
   specification defines on a relevant object (a product's or component's name, an
   artifact's name, a collection's update-reason comment) carries a short, recognizable
   marker identifying which reference-dataset object it belongs to. If identifier-based
   resolution and the embedded marker ever disagree about which object is which, that is a
   defect in the test suite's own bookkeeping to fix, not something to paper over — a real
   integrity check, not decoration. (Nothing about this requires the field's *content* to
   be non-realistic — the marker is additive to an otherwise ordinary name or comment, not
   a replacement for one.)

## Coverage, scored two independent ways

**1. Endpoint + method coverage.** A fixed registry of every (path, HTTP method) pair the
TEA 1.0 consumer specification defines, derived from the specification document itself.
Every call the suite makes against the server under test marks its operation exercised.
Score: exercised ÷ total, reported as a percentage alongside the explicit list of any
operation never called — the list matters as much as the number, since a high score that
quietly skips one operation is a worse result than a lower, honest one.

**2. Data-correctness coverage.** For each reference dataset, a known-correct structure —
object counts, specific field values, specific relationships between objects — is checked
against what the server under test actually returns. Every individual assertion increments
a verified-of-total count. Score: a second, independent percentage, about depth (was
everything that could be checked actually checked) and accuracy (was it right), not
breadth of which operations got called at all. A server can score well on coverage (it
answers every operation) while scoring poorly on correctness (what it answers is wrong) —
the two numbers are deliberately kept apart rather than combined into one, since they
measure different failure modes.

## Reference datasets (read-side summary)

| Dataset | Exercises |
|---|---|
| Simple | A baseline read of every object kind once; a release's collection history across two versions; an artifact's own version history |
| Complex | Pagination over a large (100+) object listing; identifier-filtered queries at scale; the same artifact object reachable from multiple distinct collections (shared-artifact reachability); resolving one format among several on a single artifact by media type; a collection with a multi-version history (more than two versions) |
| Lifecycle & compliance | Lifecycle (CLE) data at every entity level the specification defines it for; every lifecycle event type the specification defines; a compliance-document identifier surviving on the object kinds the specification restricts it to; signature download on an artifact that carries one |
| Deliberately invalid data | A client correctly rejecting an artifact whose downloaded content doesn't match its declared checksum, and correctly handling an artifact whose declared content is entirely unavailable to download — see "Deliberately invalid data" below |

See `docs/bundle-import-export-test-rig.md` for the exact structure of each dataset (exact
counts, identifiers, and relationships) — that level of detail belongs with the format
used to deliver the data, not with the methodology for reading it back.

## Deliberately invalid data

The specification's own correctness guarantees are not just the server's responsibility to
enforce on the way in — a conformant *client* must also not silently accept data that
fails those same guarantees on the way out, regardless of how it ended up on the server
(an implementation bug, a non-conformant exporter upstream, anything). Two scenarios, run
only against a disposable server instance deliberately seeded with invalid data — never
against a server under ordinary conformance testing, since no conformant deployment should
ever actually have this data:

- **Checksum mismatch.** An artifact whose server-reported checksum does not match the
  content actually returned when downloaded. A conformant client must detect this and
  report an error — never return the mismatched bytes to its own caller as if they were
  verified.
- **Missing content.** An artifact the server describes but cannot actually serve content
  for. A conformant client must fail cleanly and report the problem — never hang, crash,
  or report success with no content as if that were valid.

Both are reported as part of the data-correctness score, under their own named entries,
never folded into the good datasets' numbers — a client that scores well across the good
datasets while missing either of these has not actually passed.
