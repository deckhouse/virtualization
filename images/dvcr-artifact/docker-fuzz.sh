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

# Simple Docker fuzzing script for uploader package

IMAGE_NAME="uploader-fuzz"
DOCKERFILE="Dockerfile.fuzz"
DEFAULT_FUZZ_TIME="2m"

show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Build and run all uploader package fuzzing tests in Docker"
    echo ""
    echo "Options:"
    echo "  -t TIME     Set fuzz duration (default: 2m)"
    echo "  -h          Show this help"
    echo ""
    echo "Examples:"
    echo "  $0          # Run all tests for 2m"
    echo "  $0 -t 5m    # Run all tests for 5m"
    echo ""
    echo "Direct Docker command:"
    echo "  docker run -it --rm --platform linux/amd64 \$(docker build --platform linux/amd64 -q -f Dockerfile.fuzz .)"
}

FUZZ_TIME="$DEFAULT_FUZZ_TIME"

while getopts "t:h" opt; do
    case $opt in
        t) FUZZ_TIME="$OPTARG" ;;
        h) show_help; exit 0 ;;
        *) echo "Invalid option. Use -h for help."; exit 1 ;;
    esac
done

if ! command -v docker &> /dev/null; then
    echo "Error: Docker not found. Please install Docker."
    exit 1
fi

if [ ! -f "$DOCKERFILE" ]; then
    echo "Error: $DOCKERFILE not found in current directory"
    echo "Make sure you're running this from the project root"
    exit 1
fi

if [ ! -f "fuzz.sh" ]; then
    echo "Error: fuzz.sh not found in current directory"
    exit 1
fi

echo "Building and running fuzzing tests..."
echo "Duration: $FUZZ_TIME"
echo

docker run --rm \
    --platform linux/amd64 \
    -e FUZZ_TIME="$FUZZ_TIME" \
    $(docker build --platform linux/amd64 -q -f "$DOCKERFILE" .)

echo
echo "✓ Fuzzing tests completed!"
