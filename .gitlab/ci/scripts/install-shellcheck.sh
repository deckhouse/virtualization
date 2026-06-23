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

# Install ShellCheck into a writable directory if it is not already on $PATH.
#
# Used by the lint:shellcheck CI job on GitLab Runner shell executors where
# the tool is not preinstalled on the host (the runner pool mixes apt- and
# rpm-based hosts, so a distro-agnostic install is required). Downloads the
# official pre-compiled static binary from the koalaman/shellcheck GitHub
# Releases — works on any Linux with curl/wget + tar + xz, no root needed.
#
# Usage:
#   bash .gitlab/ci/scripts/install-shellcheck.sh [version] [install_dir]
#
# Defaults:
#   version     SHELLCHECK_VERSION env var, or "v0.11.0" (latest stable as
#               of 2026-06). Pin via env var to bump without editing YAML.
#   install_dir SHELLCHECK_INSTALL_DIR env var, or "${HOME}/.local/bin"
#               (created if missing).
#
# Environment:
#   SHELLCHECK_VERSION     override the version to install
#   SHELLCHECK_INSTALL_DIR override the install directory
#   SHELLCHECK_ARCH        override the release arch (x86_64 or aarch64;
#                          auto-detected from uname -m when unset)
#
# Exit codes:
#   0  - shellcheck is available (preinstalled or just installed)
#   1  - download/extract failed
#   2  - missing prerequisites (curl/wget, tar, xz)

set -euo pipefail

log()  { printf '[install-shellcheck] %s\n' "$*"; }
fail() { printf '[install-shellcheck] ERROR: %s\n' "$*" >&2; exit 1; }

# If shellcheck is already on PATH, nothing to do.
if command -v shellcheck >/dev/null 2>&1; then
  log "shellcheck already installed: $(command -v shellcheck)"
  shellcheck --version | sed -n '1,2p'
  exit 0
fi

VERSION="${1:-${SHELLCHECK_VERSION:-v0.11.0}}"
INSTALL_DIR="${2:-${SHELLCHECK_INSTALL_DIR:-${HOME}/.local/bin}}"

# Resolve the release arch suffix (default x86_64; aarch64 on arm64).
if [[ -z "${SHELLCHECK_ARCH:-}" ]]; then
  case "$(uname -m)" in
    aarch64|arm64) SHELLCHECK_ARCH="aarch64" ;;
    *)             SHELLCHECK_ARCH="x86_64" ;;
  esac
fi
ARCH_DIR="shellcheck-${VERSION}.linux.${SHELLCHECK_ARCH}"

# Prerequisites: a downloader + tar with xz support.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
else
  fail "neither curl nor wget is available; cannot download shellcheck"
fi
command -v tar >/dev/null 2>&1 || fail "tar is required to extract the release tarball"
# tar -xJ needs xz; check it explicitly so the error is readable.
command -v xz >/dev/null 2>&1 || fail "xz is required to decompress the .tar.xz release; install the xz-utils/xz package on the runner"

mkdir -p "${INSTALL_DIR}"

URL="https://github.com/koalaman/shellcheck/releases/download/${VERSION}/shellcheck-${VERSION}.linux.${SHELLCHECK_ARCH}"

log "Installing shellcheck ${VERSION} -> ${INSTALL_DIR}"
log "Downloading ${URL}"
fetch "${URL}" | tar -xJ --strip-components=1 -C "${INSTALL_DIR}" "${ARCH_DIR}/shellcheck"

chmod +x "${INSTALL_DIR}/shellcheck"

# Ensure the install dir is on PATH for the rest of the job. On the GitLab
# shell executor before_script and script share one bash session, so this
# export carries over to the task invocation in the script: section.
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) export PATH="${INSTALL_DIR}:${PATH}" ;;
esac

log "Installed: ${INSTALL_DIR}/shellcheck"
"${INSTALL_DIR}/shellcheck" --version | sed -n '1,2p'
