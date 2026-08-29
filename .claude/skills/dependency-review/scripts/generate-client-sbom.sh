#!/bin/sh
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

# Regenerates sbom/client.cdx.json: the full dependency closure actually
# reachable from cmd/teaclient (the client CLI), build-constraint aware.
set -eu
cd "$(dirname "$0")/../../../.."
mkdir -p sbom
cyclonedx-gomod app -json -output sbom/client.cdx.json -main cmd/teaclient .
echo "wrote sbom/client.cdx.json"
