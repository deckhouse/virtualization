#!/usr/bin/env bash

# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generate JSON report via ginkgo dry-run for precheck preparation
# This script suppresses output while preserving error reporting

set -e

LABELS_FILE="/tmp/e2e-specs.json"

# Get Ginkgo label filter from environment or use default
LABEL_FILTER="${GINKGO_LABELS_FILTER:-}"

# Run ginkgo dry-run to collect spec labels
# --dry-run=drone: generates JSON report without running tests
# -v: verbose output
# --json-report: write results to JSON file for precheck processing

echo "Preparing prechecks..."
ginkgo_opts=("--dry-run=drone" "-v" "--json-report=$LABELS_FILE")

# Add label filter if specified
if [ -n "$LABEL_FILTER" ]; then
    ginkgo_opts+=("--label-filter=$LABEL_FILTER")
fi

# Suppress most output but preserve errors
if ! ./e2e.test "${ginkgo_opts[@]}" 2>&1; then
    # Check if it's just a dry-run limitation error
    if ./e2e.test "${ginkgo_opts[@]}" 2>&1 | grep -q "cannot test virtual machine"; then
        echo "Precheck preparation complete (some tests skipped due to dry-run limitations)"
        exit 0
    fi
    exit 1
fi

echo "Precheck preparation complete"