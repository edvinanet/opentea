#!/bin/sh
# Regenerates sbom/server.cdx.json: the full dependency closure actually
# reachable from cmd/opentea (the server binary), build-constraint aware.
set -eu
cd "$(dirname "$0")/../../../.."
mkdir -p sbom
cyclonedx-gomod app -json -output sbom/server.cdx.json -main cmd/opentea .
echo "wrote sbom/server.cdx.json"
