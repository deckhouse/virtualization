// Copyright 2026 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const STATUSES = ["passed", "failed", "errors", "skipped"];

const STATUS_COLORS = {
  passed: "#3fb950",
  failed: "#f85149",
  errors: "#d29922",
  skipped: "#8b949e",
};

const DURATION_COLORS = {
  fast: "#7ee787",
  medium: "#3fb950",
  slow: "#238636",
};
const DURATION_LABELS = {
  fast: "Fast <60s",
  medium: "Medium 60-300s",
  slow: "Slow >300s",
};

const DEFAULT_TOP_N = 15;
const SLOW_THRESHOLD_MS = 300_000;
const MEDIUM_THRESHOLD_MS = 60_000;

function toSeconds(ms) {
  return Number((ms / 1000).toFixed(2));
}

function normalize(timing) {
  const rawState = String((timing && timing.state) || "errors");
  const rawGroup = (timing && timing.group) || "Top-level Its";
  const name = String((timing && timing.name) || "Unnamed spec");
  const group = String(rawGroup);
  const runtimeMs = Number(timing && timing.runtimeMs);
  return {
    name,
    group,
    fullName: group === name ? name : `${group} / ${name}`,
    state: STATUSES.includes(rawState) ? rawState : "errors",
    runtimeMs: Number.isFinite(runtimeMs) && runtimeMs > 0 ? runtimeMs : 0,
  };
}

function emptyStatusMap() {
  return Object.fromEntries(STATUSES.map((status) => [status, 0]));
}

// Single pass over the spec timings feeds every chart builder below.
function aggregate(specTimings) {
  const all = [];
  const byGroup = new Map();

  for (const raw of specTimings || []) {
    const timing = normalize(raw);
    all.push(timing);

    let bucket = byGroup.get(timing.group);
    if (!bucket) {
      bucket = {
        statusCount: emptyStatusMap(),
        statusDurations: emptyStatusMap(),
        total: 0,
      };
      byGroup.set(timing.group, bucket);
    }
    bucket.statusCount[timing.state] += 1;
    bucket.statusDurations[timing.state] += timing.runtimeMs;
    bucket.total += timing.runtimeMs;
  }

  return { all, byGroup };
}

function durationBucket(timing) {
  if (timing.runtimeMs > SLOW_THRESHOLD_MS) {
    return "slow";
  }
  if (timing.runtimeMs >= MEDIUM_THRESHOLD_MS) {
    return "medium";
  }
  return "fast";
}

function formatSeconds(seconds) {
  return `${Number(seconds || 0).toFixed(seconds >= 10 ? 0 : 1)}s`;
}

function formatCount(count) {
  return String(Number(count || 0));
}

function mergeChartOptions(base, override) {
  return {
    ...base,
    ...override,
    plugins: { ...(base.plugins || {}), ...(override.plugins || {}) },
    scales: { ...(base.scales || {}), ...(override.scales || {}) },
  };
}

function baseOptions(title, extra = {}) {
  return mergeChartOptions(
    {
      responsive: false,
      animation: false,
      plugins: {
        title: { display: true, text: title },
        legend: { display: true },
      },
    },
    extra
  );
}

module.exports = {
  STATUSES,
  STATUS_COLORS,
  DURATION_COLORS,
  DURATION_LABELS,
  DEFAULT_TOP_N,
  toSeconds,
  normalize,
  aggregate,
  durationBucket,
  formatSeconds,
  formatCount,
  emptyStatusMap,
  baseOptions,
};
