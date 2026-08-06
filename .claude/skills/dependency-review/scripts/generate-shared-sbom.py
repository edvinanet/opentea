#!/usr/bin/env python3
"""Generate sbom/shared.cdx.json for pkg/tea, opentea's one genuinely
shared-between-server-and-client package.

pkg/tea has zero third-party dependencies today (verified via `go list
-deps`), so this writes a minimal, valid CycloneDX 1.6 document with an
empty component list -- which is itself the accurate, meaningful claim
("this package has no third-party footprint").

cyclonedx-gomod's own modes don't fit here: `app` requires a main package
(pkg/tea isn't one), and `mod` is scoped to the whole module, not a single
package within it. Rather than build a general-purpose filter to carve a
subset out of a whole-module SBOM for a case that doesn't exist yet, this
script deliberately refuses to guess: if pkg/tea ever gains a real
third-party dependency, it exits with an error instead of silently
emitting an incomplete or wrong SBOM. At that point, regenerate properly
with `cyclonedx-gomod mod` against the whole module and hand-curate the
result down to what pkg/tea actually uses (see SKILL.md).
"""

import json
import subprocess
import sys
import uuid
from datetime import datetime, timezone

MODULE = "github.com/oej/opentea"
PACKAGE = "pkg/tea"


def sh(*args):
    return subprocess.run(args, capture_output=True, text=True, check=True).stdout


def main():
    deps_raw = sh("go", "list", "-deps", "-json", f"./{PACKAGE}/...")
    third_party = []
    for line in _split_json_stream(deps_raw):
        if line.get("Standard"):
            continue
        import_path = line.get("ImportPath", "")
        if import_path.startswith(MODULE):
            continue
        third_party.append(import_path)

    if third_party:
        print(
            f"error: {PACKAGE} now imports third-party packages this script doesn't "
            f"handle: {sorted(set(third_party))}\n"
            "Regenerate with `cyclonedx-gomod mod` against the whole module and "
            "hand-curate sbom/shared.cdx.json's components down to these -- see "
            "SKILL.md's \"if pkg/tea ever gains a dependency\" section.",
            file=sys.stderr,
        )
        sys.exit(1)

    version = sh("git", "describe", "--tags", "--always", "--dirty").strip()
    bom_ref = f"pkg:golang/{MODULE}@{version}?type=module#{PACKAGE}"

    bom = {
        "$schema": "http://cyclonedx.org/schema/bom-1.6.schema.json",
        "bomFormat": "CycloneDX",
        "specVersion": "1.6",
        "serialNumber": f"urn:uuid:{uuid.uuid4()}",
        "version": 1,
        "metadata": {
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            # "tool" (this legacy array form) only allows vendor/name/version/
            # hashes/externalReferences -- no free-text description field,
            # unlike most other CycloneDX objects (additionalProperties:
            # false on #/definitions/tool). Why this is hand-written rather
            # than cyclonedx-gomod output belongs in this script's own
            # docstring, not squeezed in here.
            "tools": [{"vendor": "opentea", "name": "generate-shared-sbom.py"}],
            "component": {
                "bom-ref": bom_ref,
                "type": "library",
                "name": f"{MODULE}/{PACKAGE}",
                "version": version,
                "purl": bom_ref,
                "externalReferences": [{"url": f"https://{MODULE}", "type": "vcs"}],
            },
        },
        "components": [],
        "dependencies": [{"ref": bom_ref, "dependsOn": []}],
    }

    with open("sbom/shared.cdx.json", "w") as f:
        json.dump(bom, f, indent=2)
        f.write("\n")
    print("wrote sbom/shared.cdx.json (0 third-party components, as expected today)")


def _split_json_stream(text):
    """go list -json emits back-to-back JSON objects, not a JSON array --
    decode them one at a time."""
    decoder = json.JSONDecoder()
    idx = 0
    text = text.strip()
    while idx < len(text):
        obj, end = decoder.raw_decode(text, idx)
        yield obj
        idx = end
        while idx < len(text) and text[idx].isspace():
            idx += 1


if __name__ == "__main__":
    main()
