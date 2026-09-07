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

# Runs the e2e suite for the merge-request buttons and stops it when the job is
# cancelled.
#
# Cancelling a job signals nothing: the runner exits the process that owns the
# job, and the job shell, this wrapper and the ginkgo tree all keep running -
# only the shell's parent changes. Left alone the suite runs on for 10 minutes
# on a cluster the pool is already cleaning, and holds the log pipe so the job
# hangs in `canceling`. The nightly GitHub workflow tears its runner down
# instead, which is why this lives here and `run-nightly-e2e.sh` stays the
# plain, shared suite invocation.

set -Eeuo pipefail

: "${E2E_SCRIPT_DIR:?}"

wrapper_pid=$$
job_shell_pid=$PPID
# The runner process that owns the job. Its death is the only sign of a cancel,
# so the watchdog compares against this value; 0 disables that check.
job_shell_ppid="$(awk '{ sub(/^.*\) /, ""); print $2 }' "/proc/${job_shell_pid}/stat" 2>/dev/null || echo 0)"
suite_pgid_file="$(mktemp)"
watchdog_log="${CI_PROJECT_DIR:-$PWD}/e2e_watchdog.log"

# `echo $$` inside the new session reports its leader's pid, which is also the
# process group id to signal. Deriving it from $! instead would break the
# moment setsid decides to fork.
# shellcheck disable=SC2016  # $$ and $1 must expand in the inner shell, not here
setsid bash -c 'echo $$ > "$1"; shift; exec bash "$@"' \
  suite "${suite_pgid_file}" "${E2E_SCRIPT_DIR}/run-nightly-e2e.sh" &
suite_pid=$!

# The watchdog gets a session of its own, so that whatever ends the job cannot
# take the killer with it.
# shellcheck disable=SC2016  # the body runs in the inner shell with its own args
setsid bash -c '
  wrapper_pid=$1 suite_pid=$2 pgid_file=$3 job_shell_pid=$4 job_shell_ppid=$5

  say() { printf "%s watchdog: %s\n" "$(date -u +%H:%M:%SZ)" "$1"; }

  # "state ppid" of a pid, or nonzero if it is gone. Read from /proc because
  # kill -0 also answers for a killed-but-unreaped zombie. /proc/pid/stat is
  # "pid (comm) state ppid ..." and comm may contain spaces, hence the cut at
  # the last ")".
  proc_state() {
    local stat state ppid
    stat="$(cat "/proc/$1/stat" 2>/dev/null)" || return 1
    stat="${stat##*) }"
    read -r state ppid _ <<<"${stat}" || true
    # A here-string always ends in a newline, so read succeeds on empty input
    # too - without this a gone pid would look like a parsed one.
    [ -n "${state}" ] && [ -n "${ppid}" ] || return 1
    printf "%s %s\n" "${state}" "${ppid}"
  }

  # Orphaning is the cancel signal; gone and zombie cover a killed shell.
  job_over_reason() {
    local info s p
    if ! info="$(proc_state "${job_shell_pid}")"; then
      printf "job shell %s is gone" "${job_shell_pid}"; return 0
    fi
    read -r s p <<<"${info}"
    if [ "${s}" = "Z" ]; then
      printf "job shell %s is a zombie" "${job_shell_pid}"; return 0
    fi
    if [ "${job_shell_ppid}" != "0" ] && [ "${p}" != "${job_shell_ppid}" ]; then
      printf "job shell %s was orphaned (parent %s -> %s)" \
        "${job_shell_pid}" "${job_shell_ppid}" "${p}"; return 0
    fi
    if ! info="$(proc_state "${wrapper_pid}")"; then
      printf "wrapper %s is gone" "${wrapper_pid}"; return 0
    fi
    read -r s p <<<"${info}"
    if [ "${s}" = "Z" ]; then
      printf "wrapper %s is a zombie" "${wrapper_pid}"; return 0
    fi
    return 1
  }

  say "watching job shell ${job_shell_pid} (parent ${job_shell_ppid}), wrapper ${wrapper_pid}, suite ${suite_pid}"
  ticks=0
  while ! reason="$(job_over_reason)" && kill -0 "${suite_pid}" 2>/dev/null; do
    # Once a minute, so a watchdog that never fires can be told from a blind
    # one without another cancel to reproduce it.
    ticks=$((ticks + 1))
    if [ $((ticks % 12)) = 0 ]; then
      say "alive: shell=[$(proc_state "${job_shell_pid}")] wrapper=[$(proc_state "${wrapper_pid}")]"
    fi
    sleep 5
  done
  [ -n "${reason:-}" ] && say "${reason}"

  if ! kill -0 "${suite_pid}" 2>/dev/null; then
    say "suite exited on its own, nothing to do"
    exit 0
  fi

  pgid="$(cat "${pgid_file}" 2>/dev/null)"
  [ -n "${pgid}" ] || pgid="${suite_pid}"
  say "job is over, sending TERM to process group ${pgid}"
  kill -TERM -- "-${pgid}" 2>/dev/null || say "TERM to -${pgid} failed"
  # Ginkgo needs a moment to finish the spec it is inside (grace-period 1m in
  # the Taskfile); anything still alive after that is not going to exit.
  sleep 60
  if kill -0 "${suite_pid}" 2>/dev/null; then
    say "suite still alive, sending KILL to process group ${pgid}"
    kill -KILL -- "-${pgid}" 2>/dev/null || say "KILL to -${pgid} failed"
  else
    say "suite stopped after TERM"
  fi
' watchdog "${wrapper_pid}" "${suite_pid}" "${suite_pgid_file}" "${job_shell_pid}" \
  "${job_shell_ppid}" >> "${watchdog_log}" 2>&1 < /dev/null &
watchdog_pid=$!

set +e
wait "${suite_pid}"
suite_exit_code=$?
set -e

# The suite is gone; stop the watchdog before its late KILL can hit a process
# group id that the runner has reused by then.
kill "${watchdog_pid}" 2>/dev/null || true
rm -f "${suite_pgid_file}"

exit "${suite_exit_code}"
