#!/bin/sh
# Validates all three SBOMs (or specific files passed as arguments) against
# the real CycloneDX 1.6 JSON Schema plus a referential-integrity check.
# See validate-sbom.go's doc comment for what "validated" means here.
set -eu
cd "$(dirname "$0")"
REPO_ROOT="$(cd ../../../.. && pwd)"

if [ "$#" -gt 0 ]; then
	go run validate-sbom.go "$@"
else
	go run validate-sbom.go "$REPO_ROOT/sbom/server.cdx.json" "$REPO_ROOT/sbom/client.cdx.json" "$REPO_ROOT/sbom/shared.cdx.json"
fi
