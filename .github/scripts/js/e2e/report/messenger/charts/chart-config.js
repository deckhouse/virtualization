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
  fast: "#76e3ea",
  medium: "#58a6ff",
  slow: "#a371f7",
};

const DEFAULT_TOP_N = 15;
const SLOW_THRESHOLD_MS = 300_000;
const MEDIUM_THRESHOLD_MS = 60_000;

function toSeconds(ms) {
  return Number((ms / 1000).toFixed(2));
}

function normalize(timing) {
  const rawState = String((timing && timing.state) || "errors");
  const rawGroup = (timing && (timing.group || timing.name)) || "Ungrouped";
  const name = String((timing && timing.name) || "Unnamed spec");
  const group = String(rawGroup);
  return {
    name,
    group,
    fullName: group === name ? name : `${group} / ${name}`,
    state: STATUSES.includes(rawState) ? rawState : "errors",
    runtimeMs: Math.max(0, Number((timing && timing.runtimeMs) || 0)),
  };
}

function emptyStatusCount() {
  return { passed: 0, failed: 0, errors: 0, skipped: 0 };
}

function emptyStatusDurations() {
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
        statusCount: emptyStatusCount(),
        statusDurations: emptyStatusDurations(),
        total: 0,
      };
      byGroup.set(timing.group, bucket);
    }
    bucket.statusCount[timing.state] += 1;
    bucket.statusDurations[timing.state] += timing.runtimeMs;
    bucket.total += timing.runtimeMs;
  }

  return { all, byGroup, byStatus, totalMs };
}

function formatSeconds(seconds) {
  return `${Number(seconds || 0).toFixed(seconds >= 10 ? 0 : 1)}s`;
}

function formatCount(count) {
  return String(Number(count || 0));
}

function formatSlowestSpecLabel(seconds, { chart, dataIndex, datasetIndex }) {
  const dataset = chart.data.datasets[datasetIndex] || {};
  const state = (dataset.states || [])[dataIndex];
  const suffix = ["failed", "errors"].includes(state) ? ` [${state}]` : "";
  return `${formatSeconds(seconds)}${suffix}`;
}

function slowestSpecsLegendLabels() {
  return [
    {
      text: "Fast <60s",
      fillStyle: DURATION_COLORS.fast,
      strokeStyle: DURATION_COLORS.fast,
      lineWidth: 0,
    },
    {
      text: "Medium 60-300s",
      fillStyle: DURATION_COLORS.medium,
      strokeStyle: DURATION_COLORS.medium,
      lineWidth: 0,
    },
    {
      text: "Slow >300s",
      fillStyle: DURATION_COLORS.slow,
      strokeStyle: DURATION_COLORS.slow,
      lineWidth: 0,
    },
    {
      text: "Failed border",
      fillStyle: "#ffffff",
      strokeStyle: STATUS_COLORS.failed,
      lineWidth: 3,
    },
    {
      text: "Error border",
      fillStyle: "#ffffff",
      strokeStyle: STATUS_COLORS.errors,
      lineWidth: 3,
    },
  ];
}

function drawValueLabels(chart, _args, options) {
  const { ctx, data } = chart;
  const formatter = options && options.formatter;
  if (typeof formatter !== "function") {
    return;
  }

  ctx.save();
  ctx.font = "12px sans-serif";
  ctx.fillStyle = "#24292f";
  ctx.textBaseline = "middle";

  chart.getSortedVisibleDatasetMetas().forEach((meta) => {
    meta.data.forEach((element, dataIndex) => {
      const rawValue = data.datasets[meta.index].data[dataIndex];
      if (!rawValue) {
        return;
      }

      const label = formatter(rawValue, {
        chart,
        dataIndex,
        datasetIndex: meta.index,
      });
      if (!label) {
        return;
      }

      const props = element.getProps(["x", "y", "base"], true);
      const isHorizontal = chart.options.indexAxis === "y";
      const isStacked = Boolean(
        isHorizontal
          ? chart.options.scales &&
              chart.options.scales.x &&
              chart.options.scales.x.stacked
          : chart.options.scales &&
              chart.options.scales.y &&
              chart.options.scales.y.stacked
      );

      if (isHorizontal) {
        const barWidth = Math.abs(props.x - props.base);
        ctx.textAlign = isStacked && barWidth > 34 ? "center" : "left";
        ctx.fillText(
          label,
          isStacked && barWidth > 34 ? (props.x + props.base) / 2 : props.x + 6,
          props.y
        );
        return;
      }

      ctx.textAlign = "center";
      ctx.fillText(label, props.x, props.y - 8);
    });
  });

  ctx.restore();
}

const valueLabelsPlugin = {
  id: "valueLabels",
  afterDatasetsDraw: drawValueLabels,
};

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

function slowestSpecs({ all }, topN = DEFAULT_TOP_N) {
  const top = [...all]
    .sort(
      (left, right) =>
        right.runtimeMs - left.runtimeMs ||
        left.fullName.localeCompare(right.fullName)
    )
    .slice(0, topN);

  return {
    name: "slowest-specs",
    config: {
      type: "bar",
      data: {
        labels: top.map((timing) => timing.fullName),
        datasets: [
          {
            label: "Duration, seconds",
            data: top.map((timing) => toSeconds(timing.runtimeMs)),
            backgroundColor: top.map(
              (timing) => DURATION_COLORS[durationBucket(timing)]
            ),
            borderColor: top.map((timing) =>
              ["failed", "errors"].includes(timing.state)
                ? STATUS_COLORS[timing.state]
                : "transparent"
            ),
            borderWidth: top.map((timing) =>
              ["failed", "errors"].includes(timing.state) ? 3 : 0
            ),
            states: top.map((timing) => timing.state),
          },
        ],
      },
      options: baseOptions(
        "Top slowest successful specs and failed specs (It/Entry)",
        {
          indexAxis: "y",
          plugins: {
            title: {
              display: true,
              text: "Top slowest successful specs and failed specs (It/Entry)",
            },
            legend: {
              display: true,
              labels: { generateLabels: slowestSpecsLegendLabels },
            },
            valueLabels: { formatter: formatSlowestSpecLabel },
          },
          scales: {
            x: {
              beginAtZero: true,
              ticks: { stepSize: 60 },
              title: { display: true, text: "Duration, seconds" },
            },
          },
          layout: {
            padding: { top: 16, bottom: 8 },
          },
        }
      ),
      plugins: [valueLabelsPlugin],
    },
  };
}

function sortedGroups(byGroup, compareFn) {
  return [...byGroup.entries()].sort(compareFn);
}

function problemCount(group) {
  return group.statusCount.failed + group.statusCount.errors;
}

function featureDurationStatus({ byGroup }) {
  // Most-broken features go to the top: failures desc, then total runtime desc,
  // then alphabetical for a stable order.
  const entries = sortedGroups(byGroup, (left, right) => {
    return (
      problemCount(right[1]) - problemCount(left[1]) ||
      right[1].total - left[1].total ||
      left[0].localeCompare(right[0])
    );
  });

  const labels = entries.map(([name]) => name);
  const datasets = STATUSES.map((status) => ({
    label: status,
    data: entries.map(([, group]) => toSeconds(group.statusDurations[status])),
    backgroundColor: STATUS_COLORS[status],
  }));

  return {
    name: "feature-duration-status",
    config: {
      type: "bar",
      data: { labels, datasets },
      options: baseOptions("Overall durations for Describes", {
        indexAxis: "y",
        plugins: {
          title: {
            display: true,
            text: "Overall durations for Describes",
          },
          legend: { display: true },
          valueLabels: { formatter: formatSeconds },
        },
        scales: {
          x: {
            stacked: true,
            beginAtZero: true,
            title: { display: true, text: "Duration, seconds" },
          },
          y: { stacked: true },
        },
      }),
      plugins: [valueLabelsPlugin],
    },
  };
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

function durationBuckets({ all }) {
  const buckets = [
    { key: "slow", label: "Slow >300s", counts: emptyStatusCount() },
    { key: "medium", label: "Medium 60-300s", counts: emptyStatusCount() },
    { key: "fast", label: "Fast <60s", counts: emptyStatusCount() },
  ];
  const byBucket = new Map(buckets.map((bucket) => [bucket.key, bucket]));
  for (const timing of all) {
    byBucket.get(durationBucket(timing)).counts[timing.state] += 1;
  }

  return {
    name: "duration-buckets",
    config: {
      type: "bar",
      data: {
        labels: buckets.map((bucket) => bucket.label),
        datasets: STATUSES.map((status) => ({
          label: status,
          data: buckets.map((bucket) => bucket.counts[status]),
          backgroundColor: STATUS_COLORS[status],
          barPercentage: 0.5,
          categoryPercentage: 0.6,
        })),
      },
      options: baseOptions("It/Entry duration buckets by status", {
        indexAxis: "y",
        plugins: {
          title: {
            display: true,
            text: "It/Entry duration buckets by status",
          },
          legend: { display: true },
          valueLabels: { formatter: formatCount },
        },
        scales: {
          x: {
            stacked: true,
            beginAtZero: true,
            ticks: { precision: 0 },
            title: { display: true, text: "Specs" },
          },
          y: { stacked: true },
        },
      }),
      plugins: [valueLabelsPlugin],
    },
  };
}

// Order of charts matches the order of attachments in the messenger thread.
const CHART_BUILDERS = [featureDurationStatus, slowestSpecs, durationBuckets];

function buildClusterChartConfigs(specTimings) {
  const data = aggregate(specTimings);
  return CHART_BUILDERS.map((build) => build(data));
}

module.exports = {
  buildClusterChartConfigs,
};
