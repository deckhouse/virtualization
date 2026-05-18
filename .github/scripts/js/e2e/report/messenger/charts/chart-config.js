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

const statusColors = {
  passed: "#3fb950",
  failed: "#f85149",
  errors: "#d29922",
  skipped: "#8b949e",
};

function normalizeTiming(timing) {
  return {
    name: String(timing.name || "Unnamed spec"),
    group: String(timing.group || timing.name || "Ungrouped"),
    state: String(timing.state || "errors"),
    runtimeMs: Math.max(0, Number(timing.runtimeMs || 0)),
  };
}

function seconds(ms) {
  return Number((ms / 1000).toFixed(2));
}

function baseOptions(title, extra = {}) {
  return {
    responsive: false,
    animation: false,
    plugins: {
      title: {
        display: true,
        text: title,
      },
      legend: {
        display: true,
      },
    },
    ...extra,
  };
}

function sortByRuntimeDesc(left, right) {
  return (
    right.runtimeMs - left.runtimeMs || left.name.localeCompare(right.name)
  );
}

function groupTotals(specTimings) {
  const totals = new Map();
  for (const rawTiming of specTimings || []) {
    const timing = normalizeTiming(rawTiming);
    if (!totals.has(timing.group)) {
      totals.set(timing.group, 0);
    }
    totals.set(timing.group, totals.get(timing.group) + timing.runtimeMs);
  }

  return Array.from(totals, ([group, runtimeMs]) => ({
    group,
    runtimeMs,
  })).sort(
    (left, right) =>
      right.runtimeMs - left.runtimeMs || left.group.localeCompare(right.group)
  );
}

function buildTopNConfig(specTimings, n = 15) {
  const timings = (specTimings || [])
    .map(normalizeTiming)
    .sort(sortByRuntimeDesc)
    .slice(0, n);

  return {
    name: "top-slowest",
    config: {
      type: "bar",
      data: {
        labels: timings.map((timing) => timing.name),
        datasets: [
          {
            label: "Duration, seconds",
            data: timings.map((timing) => seconds(timing.runtimeMs)),
            backgroundColor: "#58a6ff",
          },
        ],
      },
      options: baseOptions("Top slowest E2E specs", {
        indexAxis: "y",
        scales: {
          x: { beginAtZero: true },
        },
      }),
    },
  };
}

function buildDurationHistogramConfig(
  specTimings,
  buckets = [30, 60, 300, 600, Infinity]
) {
  const counts = buckets.map(() => 0);
  for (const rawTiming of specTimings || []) {
    const durationSeconds =
      Number(normalizeTiming(rawTiming).runtimeMs || 0) / 1000;
    const bucketIndex = buckets.findIndex(
      (bucket) => durationSeconds <= bucket
    );
    counts[bucketIndex >= 0 ? bucketIndex : buckets.length - 1] += 1;
  }

  let previous = 0;
  const labels = buckets.map((bucket) => {
    const label =
      bucket === Infinity ? `>${previous}s` : `${previous}-${bucket}s`;
    previous = bucket;
    return label;
  });

  return {
    name: "duration-histogram",
    config: {
      type: "bar",
      data: {
        labels,
        datasets: [
          {
            label: "Specs",
            data: counts,
            backgroundColor: "#a371f7",
          },
        ],
      },
      options: baseOptions("E2E spec duration distribution", {
        scales: {
          y: { beginAtZero: true, ticks: { precision: 0 } },
        },
      }),
    },
  };
}

function buildFeatureTotalsConfig(specTimings) {
  const totals = groupTotals(specTimings);

  return {
    name: "feature-totals",
    config: {
      type: "bar",
      data: {
        labels: totals.map((entry) => entry.group),
        datasets: [
          {
            label: "Total duration, seconds",
            data: totals.map((entry) => seconds(entry.runtimeMs)),
            backgroundColor: "#f0883e",
          },
        ],
      },
      options: baseOptions("E2E duration by feature", {
        scales: {
          y: { beginAtZero: true },
        },
      }),
    },
  };
}

function buildStatusStackedConfig(specTimings) {
  const groups = new Map();
  for (const rawTiming of specTimings || []) {
    const timing = normalizeTiming(rawTiming);
    if (!groups.has(timing.group)) {
      groups.set(timing.group, { passed: 0, failed: 0, errors: 0, skipped: 0 });
    }
    const status = Object.prototype.hasOwnProperty.call(
      statusColors,
      timing.state
    )
      ? timing.state
      : "errors";
    groups.get(timing.group)[status] += timing.runtimeMs;
  }

  const labels = Array.from(groups.keys()).sort();
  const statuses = ["passed", "failed", "errors", "skipped"];

  return {
    name: "status-stacked",
    config: {
      type: "bar",
      data: {
        labels,
        datasets: statuses.map((status) => ({
          label: status,
          data: labels.map((label) => seconds(groups.get(label)[status])),
          backgroundColor: statusColors[status],
        })),
      },
      options: baseOptions("E2E duration by feature and status", {
        scales: {
          x: { stacked: true },
          y: { beginAtZero: true, stacked: true },
        },
      }),
    },
  };
}

module.exports = {
  buildDurationHistogramConfig,
  buildFeatureTotalsConfig,
  buildStatusStackedConfig,
  buildTopNConfig,
};
