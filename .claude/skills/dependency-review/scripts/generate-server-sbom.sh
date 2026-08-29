#!/bin/sh
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

# Regenerates sbom/server.cdx.json: the full dependency closure actually
# reachable from cmd/opentea (the server binary), build-constraint aware.
set -eu
cd "$(dirname "$0")/../../../.."
mkdir -p sbom
cyclonedx-gomod app -json -output sbom/server.cdx.json -main cmd/opentea .
echo "wrote sbom/server.cdx.json"
