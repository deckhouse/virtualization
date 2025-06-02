#!/bin/bash

# Copyright 2024 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FUZZ_TIME=${FUZZ_TIME:-2m}
PACKAGE_DIR="/$PWD/pkg/uploader"
TIMEOUT_DURATION=600  # 10 minutes max per test

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

echo "======================================================"
echo "  Uploader Package Fuzzing Tests"
echo "======================================================"
echo

log_info "Starting uploader package fuzzing tests"
log_info "Go version: $(go version)"
log_info "Working directory: $(pwd)"
log_info "Fuzz duration: $FUZZ_TIME"
log_info "Timeout per test: ${TIMEOUT_DURATION}s"
echo

if [ ! -d "$PACKAGE_DIR" ]; then
    log_error "Package directory not found: $PACKAGE_DIR"
    exit 1
fi

cd "$PACKAGE_DIR"

log_step "Checking package dependencies..."
if go list -deps . >/dev/null 2>&1; then
    log_info "Dependencies resolved successfully"
else
    log_warn "Dependency check failed, attempting to resolve..."
    if go mod tidy >/dev/null 2>&1 && go mod download >/dev/null 2>&1; then
        log_info "Dependencies resolved after cleanup"
    else
        log_warn "Dependency issues persist, continuing anyway..."
        log_info "Note: Some tests may fail due to missing dependencies"
    fi
fi
echo

FUZZ_TESTS=(
    "FuzzParseHTTPHeader"
    "FuzzNewContentReader"
    "FuzzValidateHTTPRequest"
    "FuzzSnappyDecompression"
)

log_step "Starting fuzzing tests (${#FUZZ_TESTS[@]} tests total)..."
echo

total_tests=${#FUZZ_TESTS[@]}
passed_tests=0
failed_tests=0

for i in "${!FUZZ_TESTS[@]}"; do
    test="${FUZZ_TESTS[$i]}"
    test_num=$((i + 1))

    echo "[$test_num/$total_tests] Running $test for $FUZZ_TIME..."

    start_time=$(date +%s)

    test_output=$(timeout $TIMEOUT_DURATION go test -fuzz="$test" -fuzztime="$FUZZ_TIME" -v 2>&1)
    test_exit_code=$?

    end_time=$(date +%s)
    duration=$((end_time - start_time))

    if [ $test_exit_code -eq 0 ]; then
        log_info "✓ $test completed successfully in ${duration}s"
        echo "$test_output" | cat
        ((passed_tests++))
    else
        if [ $test_exit_code -eq 124 ]; then
            log_warn "$test timed out after ${TIMEOUT_DURATION}s"
        elif [ $test_exit_code -eq 1 ]; then
            log_warn "$test found potential issues (exit code: $test_exit_code)"
            echo "$test_output" | cat
        else
            log_error "$test failed with exit code: $test_exit_code"
            echo "$test_output" | cat
        fi
        ((failed_tests++))
    fi

    echo "---"
done

echo
echo "======================================================"
echo "  Fuzzing Test Results"
echo "======================================================"
log_info "Total tests run: $total_tests"
log_info "Passed: $passed_tests"
if [ $failed_tests -gt 0 ]; then
    log_warn "Failed/Issues: $failed_tests"
else
    log_info "Failed/Issues: $failed_tests"
fi
echo

if [ $passed_tests -eq $total_tests ]; then
    log_info "🎉 All fuzzing tests completed successfully!"
    exit 0
else
    log_warn "Some tests had issues. Review output above for details."
    exit 1
fi
