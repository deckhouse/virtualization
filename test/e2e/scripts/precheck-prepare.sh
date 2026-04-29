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

# Build ginkgo command
CMD="go tool ginkgo --json-report=/tmp/e2e-specs.json --dry-run --no-color"

# Add label filter based on environment variables
if [ -n "$FOCUS" ]; then
    CMD="$CMD --focus=$FOCUS"
elif [ -n "$LABELS" ]; then
    CMD="$CMD --label-filter=$LABELS"
fi

# Run with suppressed stdout, but show stderr
$CMD 2>&1 > /dev/null

echo "Precheck prepare completed"
