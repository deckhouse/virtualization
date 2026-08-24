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

# Builds tools/changelog and runs it from the root of the repository.
#
# The tool is built into a temporary directory and run from the repository root
# rather than started with `go run` from its own directory: it reads the list of
# allowed sections by a path relative to the repository, and `collect` commits
# in the working copy the pipeline checked out.
#
# GOWORK=off keeps the build to this one module. In workspace mode go resolves
# every module of the repository, including the kubevirt fork on fox, which
# these jobs have no reason to fetch.

run_changelog_tool() {
  local repo_dir="$1"
  shift

  local build_dir
  build_dir="$(mktemp -d)"
  # shellcheck disable=SC2064 # expand build_dir now, while it is still set.
  trap "rm -rf '${build_dir}'" EXIT

  GOWORK=off go -C "${repo_dir}/tools/changelog" build -o "${build_dir}/changelog" .
  (cd "${repo_dir}" && "${build_dir}/changelog" "$@")
}
