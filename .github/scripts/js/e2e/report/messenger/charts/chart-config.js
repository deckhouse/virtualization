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

const PALETTE = {
  bar: "#58a6ff",
  cumulative: "#a371f7",
  total: "#f0883e",
  p50: "#58a6ff",
  p90: "#d29922",
  max: "#f85149",
};

const DEFAULT_TOP_N = 15;

function toSeconds(ms) {
  return Number((ms / 1000).toFixed(2));
}

function normalize(timing) {
  const rawState = String((timing && timing.state) || "errors");
  const rawGroup = (timing && (timing.group || timing.name)) || "Ungrouped";
  return {
    name: String((timing && timing.name) || "Unnamed spec"),
    group: String(rawGroup),
    state: STATUSES.includes(rawState) ? rawState : "errors",
    runtimeMs: Math.max(0, Number((timing && timing.runtimeMs) || 0)),
  };
}

// Linear-interpolated quantile over a numerically sorted (asc) array.
// Mirrors Excel's PERCENTILE.INC / numpy's default percentile method.
function quantile(sortedAsc, q) {
  if (sortedAsc.length === 0) {
    return 0;
  }
  if (sortedAsc.length === 1) {
    return sortedAsc[0];
  }
  const pos = (sortedAsc.length - 1) * q;
  const base = Math.floor(pos);
  const upper = sortedAsc[base + 1];
  return upper === undefined
    ? sortedAsc[base]
    : sortedAsc[base] + (pos - base) * (upper - sortedAsc[base]);
}

function emptyStatusCount() {
  return { passed: 0, failed: 0, errors: 0, skipped: 0 };
}

// Single pass over the spec timings feeds every chart builder below.
function aggregate(specTimings) {
  const all = [];
  const byGroup = new Map();
  const byStatus = emptyStatusCount();
  let totalMs = 0;

  for (const raw of specTimings || []) {
    const timing = normalize(raw);
    all.push(timing);
    totalMs += timing.runtimeMs;
    byStatus[timing.state] += 1;

    let bucket = byGroup.get(timing.group);
    if (!bucket) {
      bucket = {
        durations: [],
        statusCount: emptyStatusCount(),
        total: 0,
      };
      byGroup.set(timing.group, bucket);
    }
    bucket.durations.push(timing.runtimeMs);
    bucket.statusCount[timing.state] += 1;
    bucket.total += timing.runtimeMs;
  }

  return { all, byGroup, byStatus, totalMs };
}

function baseOptions(title, extra = {}) {
  return {
    responsive: false,
    animation: false,
    plugins: {
      title: { display: true, text: title },
      legend: { display: true },
    },
    ...extra,
  };
}

function statusDoughnut({ byStatus }) {
  return {
    name: "status-doughnut",
    config: {
      type: "doughnut",
      data: {
        labels: STATUSES,
        datasets: [
          {
            data: STATUSES.map((status) => byStatus[status]),
            backgroundColor: STATUSES.map((status) => STATUS_COLORS[status]),
          },
        ],
      },
      options: baseOptions("E2E spec status distribution"),
    },
  };
}

function paretoSlowest({ all, totalMs }, topN = DEFAULT_TOP_N) {
  const top = [...all]
    .sort(
      (left, right) =>
        right.runtimeMs - left.runtimeMs || left.name.localeCompare(right.name)
    )
    .slice(0, topN);

  let runningMs = 0;
  const cumulativePercents = top.map((timing) => {
    runningMs += timing.runtimeMs;
    return totalMs > 0 ? Number(((runningMs / totalMs) * 100).toFixed(1)) : 0;
  });

  return {
    name: "pareto-slowest",
    config: {
      type: "bar",
      data: {
        labels: top.map((timing) => timing.name),
        datasets: [
          {
            type: "bar",
            label: "Duration, seconds",
            data: top.map((timing) => toSeconds(timing.runtimeMs)),
            backgroundColor: PALETTE.bar,
            xAxisID: "x",
            order: 2,
          },
          {
            type: "line",
            label: "Cumulative % of suite time",
            data: cumulativePercents,
            borderColor: PALETTE.cumulative,
            backgroundColor: PALETTE.cumulative,
            xAxisID: "x1",
            order: 1,
          },
        ],
      },
      options: baseOptions("Top slowest E2E specs (Pareto)", {
        indexAxis: "y",
        scales: {
          x: {
            beginAtZero: true,
            position: "bottom",
            title: { display: true, text: "Duration, seconds" },
          },
          x1: {
            beginAtZero: true,
            max: 100,
            position: "top",
            grid: { drawOnChartArea: false },
            title: { display: true, text: "Cumulative %" },
          },
        },
      }),
    },
  };
}

function sortedGroups(byGroup, compareFn) {
  return [...byGroup.entries()].sort(compareFn);
}

function passRatePerFeature({ byGroup }) {
  // Most-broken features go to the top: failures desc, then total runtime desc,
  // then alphabetical for a stable order.
  const entries = sortedGroups(byGroup, (left, right) => {
    const failsLeft = left[1].statusCount.failed + left[1].statusCount.errors;
    const failsRight =
      right[1].statusCount.failed + right[1].statusCount.errors;
    return (
      failsRight - failsLeft ||
      right[1].total - left[1].total ||
      left[0].localeCompare(right[0])
    );
  });

  const labels = entries.map(([name]) => name);
  const datasets = STATUSES.map((status) => ({
    label: status,
    data: entries.map(([, group]) => {
      const total = STATUSES.reduce(
        (sum, candidate) => sum + group.statusCount[candidate],
        0
      );
      return total > 0
        ? Number(((group.statusCount[status] / total) * 100).toFixed(1))
        : 0;
    }),
    backgroundColor: STATUS_COLORS[status],
  }));

  return {
    name: "pass-rate-per-feature",
    config: {
      type: "bar",
      data: { labels, datasets },
      options: baseOptions("Pass rate by feature, %", {
        indexAxis: "y",
        scales: {
          x: {
            stacked: true,
            beginAtZero: true,
            max: 100,
            title: { display: true, text: "% of specs" },
          },
          y: { stacked: true },
        },
      }),
    },
  };
}

function quantilesPerFeature({ byGroup }) {
  const entries = sortedGroups(
    byGroup,
    (left, right) =>
      right[1].total - left[1].total || left[0].localeCompare(right[0])
  );
  const sortedDurations = entries.map(([, group]) =>
    [...group.durations].sort((left, right) => left - right)
  );

  return {
    name: "quantiles-per-feature",
    config: {
      type: "bar",
      data: {
        labels: entries.map(([name]) => name),
        datasets: [
          {
            label: "p50",
            data: sortedDurations.map((durations) =>
              toSeconds(quantile(durations, 0.5))
            ),
            backgroundColor: PALETTE.p50,
          },
          {
            label: "p90",
            data: sortedDurations.map((durations) =>
              toSeconds(quantile(durations, 0.9))
            ),
            backgroundColor: PALETTE.p90,
          },
          {
            label: "max",
            data: sortedDurations.map((durations) =>
              toSeconds(durations[durations.length - 1] || 0)
            ),
            backgroundColor: PALETTE.max,
          },
        ],
      },
      options: baseOptions("Spec duration p50/p90/max by feature, seconds", {
        scales: {
          y: {
            beginAtZero: true,
            title: { display: true, text: "Seconds" },
          },
        },
      }),
    },
  };
}

function featureTotals({ byGroup }) {
  const entries = sortedGroups(
    byGroup,
    (left, right) =>
      right[1].total - left[1].total || left[0].localeCompare(right[0])
  );

  return {
    name: "feature-totals",
    config: {
      type: "bar",
      data: {
        labels: entries.map(([name]) => name),
        datasets: [
          {
            label: "Total duration, seconds",
            data: entries.map(([, group]) => toSeconds(group.total)),
            backgroundColor: PALETTE.total,
          },
        ],
      },
      options: baseOptions("Total duration by feature", {
        indexAxis: "y",
        scales: {
          x: {
            beginAtZero: true,
            title: { display: true, text: "Seconds" },
          },
        },
      }),
    },
  };
}

// Order of charts matches the order of attachments in the messenger thread.
const CHART_BUILDERS = [
  statusDoughnut,
  paretoSlowest,
  passRatePerFeature,
  quantilesPerFeature,
  featureTotals,
];

function buildClusterChartConfigs(specTimings) {
  const data = aggregate(specTimings);
  return CHART_BUILDERS.map((build) => build(data));
}

module.exports = {
  buildClusterChartConfigs,
};
