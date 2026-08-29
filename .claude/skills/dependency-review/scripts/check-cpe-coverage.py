#!/usr/bin/env python3
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

"""Checks every dependency across the three SBOMs against the GCVE CPE
database (https://cpe.gcve.eu) by package URL, records the outcome as an
`opentea:cpeChecked` property on each component *inside* the SBOM itself
(matching the kamailio project's `kamailio:cpeSource` convention -- a
downstream consumer reading only the SBOM file should be able to tell
"checked, none found" from "never checked" without cross-referencing a
separate report), and also writes a human-readable summary to
sbom/cpe-coverage.md.

This deliberately does NOT auto-write a `cpe` field. Auto-selecting a CPE
match -- especially "pick the candidate with the most package-registry
mappings" -- produces confidently wrong answers whenever a vendor
namespace has many unrelated products (see SKILL.md's "CPE enrichment"
section for the reasoning and a concrete example of this going wrong). Any
candidate this script finds must be verified by a human/agent against the
exact product name and its evidence before ever being added to a
component's "cpe" field by hand -- at which point also add an
`opentea:cpeSource` property recording how it was verified, matching
kamailio's convention.

A dependency with zero candidates is not a failure of this script -- it's
the honest, and currently typical, outcome for Go-ecosystem packages (the
CPE dictionary skews toward traditional/commercial software; see the
report this writes for exact counts).

Run this *before* validate-sbom.sh, not after -- it rewrites the SBOM
files, so validation should check the state this script leaves behind, not
the other way around.
"""

import json
import sys
import urllib.parse
import urllib.request
from datetime import datetime, timezone

SBOM_FILES = ["sbom/server.cdx.json", "sbom/client.cdx.json", "sbom/shared.cdx.json"]
API_BASE = "https://cpe.gcve.eu/api/cpes"
PROPERTY_NAME = "opentea:cpeChecked"


def query(purl_prefix):
    url = f"{API_BASE}?purl_q={urllib.parse.quote(purl_prefix)}&per_page=10"
    with urllib.request.urlopen(url, timeout=15) as resp:  # nosec: fixed, hardcoded API_BASE, not user input
        return json.load(resp)


def set_property(component, value):
    """Sets PROPERTY_NAME to value on component's properties list,
    replacing any prior value from an earlier run rather than
    accumulating duplicates."""
    props = [p for p in component.get("properties", []) if p.get("name") != PROPERTY_NAME]
    props.append({"name": PROPERTY_NAME, "value": value})
    component["properties"] = props


def main():
    checked_at = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    detail_lines = []
    total_components = 0
    found_any = False
    query_cache = {}  # purl_prefix -> result, so a dep appearing in multiple SBOMs is only queried once

    for path in SBOM_FILES:
        try:
            with open(path) as f:
                bom = json.load(f)
        except FileNotFoundError:
            continue

        for component in bom.get("components", []):
            purl = component.get("purl", "")
            if not purl:
                continue
            purl_prefix = purl.split("@")[0]  # drop @version?qualifiers#subpath, purl_q does prefix matching
            name = component.get("name", purl_prefix)
            total_components += 1

            if purl_prefix not in query_cache:
                try:
                    query_cache[purl_prefix] = query(purl_prefix)
                except Exception as e:  # noqa: BLE001 -- report and continue, one dependency's network hiccup shouldn't abort the whole run
                    query_cache[purl_prefix] = e

            result = query_cache[purl_prefix]
            if isinstance(result, Exception):
                set_property(component, f"lookup failed {checked_at}: {result} -- not checked, retry")
                detail_lines.append(f"- **{name}** (`{purl_prefix}`): lookup failed ({result}) -- not checked, retry")
                continue

            items = result.get("items", [])
            if not items:
                set_property(component, f"{checked_at}, https://cpe.gcve.eu, no match")
                detail_lines.append(f"- **{name}** (`{purl_prefix}`): checked, no CPE found")
                continue

            found_any = True
            set_property(
                component,
                f"{checked_at}, https://cpe.gcve.eu, {len(items)} candidate(s) found, NOT verified "
                "-- see sbom/cpe-coverage.md",
            )
            detail_lines.append(f"- **{name}** (`{purl_prefix}`): {len(items)} candidate(s) found, NEEDS MANUAL VERIFICATION:")
            for item in items:
                detail_lines.append(f"  - `{item.get('cpe_uri')}` -- verify product name matches exactly and inspect evidence before using")

        with open(path, "w") as f:
            json.dump(bom, f, indent=2)
            f.write("\n")

    if total_components == 0:
        print("no dependencies found across sbom/*.cdx.json -- run the generate-*-sbom scripts first", file=sys.stderr)
        sys.exit(1)

    report_lines = [
        f"# CPE coverage check ({checked_at})",
        "",
        f"Checked {total_components} component entries across all three SBOMs against "
        "https://cpe.gcve.eu by package URL prefix. Per-dependency detail also recorded "
        f"directly on each SBOM component via an `{PROPERTY_NAME}` property -- this file is a "
        "human-readable summary of the same data, not the source of truth. No `cpe` field is "
        "written automatically; see generate-shared-sbom.py's docstring and SKILL.md for why.",
        "",
        *detail_lines,
    ]
    with open("sbom/cpe-coverage.md", "w") as f:
        f.write("\n".join(report_lines) + "\n")

    print(f"annotated {total_components} component(s) across {len(SBOM_FILES)} SBOM(s) and wrote sbom/cpe-coverage.md")
    if found_any:
        print("candidates found for at least one dependency -- see the report; do not auto-apply them")


if __name__ == "__main__":
    main()
