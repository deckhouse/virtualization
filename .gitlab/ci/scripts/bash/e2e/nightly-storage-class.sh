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
# Each OS stand carries several storage classes, but one nightly run exercises
# one of them. Rather than pin an OS to a single class forever, the class
# rotates by the day: with a list of N working classes, day D uses index
# D mod N, so over N days the OS covers them all. The rotation is a pure
# function of the date - no state is stored anywhere - so any runner computes
# the same class for the same day.
#
# The lists hold only classes that actually work on that OS today. A class is
# added to a list once its stand can serve it:
#   - ceph is not stood up on any stand yet, so it is in no list;
#   - replicated (nested-thin-r1) does not build on the ALT kernel (DRBD 9.2),
#     so an ALT run that lands on it is a KNOWN FAILURE, left in the list on
#     purpose so the rotation keeps covering it and the day it starts passing
#     is visible. Remove this note and nothing else changes when it is fixed.
#
# Usage, from a job script:
#   eval "$(bash nightly-storage-class.sh "$E2E_OS")"
# which exports STORAGE_CLASS_NAME (consumed by the suite) and CSI (a label for
# the run), exactly the two variables the per-storage buttons set by hand.

set -euo pipefail

os="${1:?usage: nightly-storage-class.sh <redos|alt|astra>}"

# Day zero of the rotation. Arbitrary but fixed: moving it reshuffles which
# class each OS lands on, so it does not change.
base_date="2026-01-01"

case "$os" in
  redos) classes=(nfs nested-local-thin nested-thin-r1) ;;
  alt) classes=(nfs nested-local-thin nested-thin-r1) ;; # nested-thin-r1 is a known fail, see header
  astra) classes=(nfs nested-local-thin) ;;
  *)
    echo "[ERROR] unknown OS '${os}', expected one of: redos alt astra" >&2
    exit 1
    ;;
esac

days=$(( ( $(date -u +%s) - $(date -u -d "${base_date}" +%s) ) / 86400 ))
sc="${classes[$(( days % ${#classes[@]} ))]}"

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
