#!/bin/bash
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
else
    CMD="$CMD --label-filter=!Slow"
fi

# Run with suppressed stdout, but show stderr
$CMD 2>&1 > /dev/null

echo "Precheck prepare completed"