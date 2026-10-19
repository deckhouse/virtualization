#!/usr/bin/env bash

# Copyright 2026 Flant JSC
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

# Pick the storage class the nightly e2e run uses on a given guest OS stand.
#
# One nightly run per OS exercises one storage class, rotating by the day so no
# class is pinned forever. The schedule is an explicit table: one row per
# rotation day, one (OS, class) pair per OS. The rows are built so that redos and
# alt never share a class on the same day - in particular never both on
# replicated, where the suite is weakest - and every (OS, class) pair the stand
# supports is covered over the cycle. To add a stand or a class, add a column /
# rows to the table.
#
# Which classes are in play, and why some are excluded:
#   - ceph is not stood up on any stand yet, so it is used nowhere;
#   - replicated (nested-thin-r1) does not build on the ALT kernel (DRBD 9.2),
#     so an ALT run that lands on it is a KNOWN FAILURE, kept in the rotation on
#     purpose so coverage continues and the day it starts passing is visible;
#   - astra carries no replicated class (only nfs/local).
#
# Usage, from a job script:
#   eval "$(bash nightly-storage-class.sh "$E2E_OS")"
# which exports STORAGE_CLASS_NAME (consumed by the suite) and CSI (a label for
# the run), exactly the two variables the per-storage buttons set by hand.

set -euo pipefail

os="${1:?usage: nightly-storage-class.sh <redos|alt|astra>}"

sc_nfs=nfs
sc_local=nested-local-thin
sc_repl=nested-thin-r1

# The rotation schedule, one row per day in the cycle. redos and alt never share
# a class (so never both on replicated), astra only ever gets nfs/local, and over
# the three rows each stand covers every class it supports.
cases=(
  "redos ${sc_repl}  alt ${sc_local} astra ${sc_nfs}"
  "redos ${sc_nfs}   alt ${sc_repl}  astra ${sc_local}"
  "redos ${sc_local} alt ${sc_nfs}   astra ${sc_nfs}"
)

# Day of the month picks the row; it resets monthly, but every month is longer
# than the cycle, so each row still runs several times a month.
idx=$(( ( $(date -u +%-d) - 1 ) % ${#cases[@]} ))
read -r _ redos_sc _ alt_sc _ astra_sc <<<"${cases[$idx]}"

case "${os}" in
  redos) sc="${redos_sc}" ;;
  alt) sc="${alt_sc}" ;;
  astra) sc="${astra_sc}" ;;
  *)
    echo "[ERROR] unknown OS '${os}', expected one of: redos alt astra" >&2
    exit 1
    ;;
esac

case "$sc" in
  nfs) csi="nfs" ;;
  nested-thin-r1) csi="replicated" ;;
  nested-local-thin) csi="local" ;;
  *)
    echo "[ERROR] no CSI label for storage class '${sc}'" >&2
    exit 1
    ;;
esac

printf 'export STORAGE_CLASS_NAME=%q\n' "$sc"
printf 'export CSI=%q\n' "$csi"
