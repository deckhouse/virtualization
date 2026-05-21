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

const {
  STATUS_COLORS,
  DURATION_COLORS,
  DURATION_LABELS,
  DEFAULT_TOP_N,
  toSeconds,
  durationBucket,
  formatSeconds,
  baseOptions,
} = require("../data");
const { valueLabelsPlugin } = require("../plugins");

function formatSlowestSpecLabel(seconds, { chart, dataIndex }) {
  const dataset = chart.data.datasets[0] || {};
  const state = (dataset.states || [])[dataIndex];
  const suffix = ["failed", "errors"].includes(state) ? ` [${state}]` : "";
  return `${formatSeconds(seconds)}${suffix}`;
}

function slowestSpecsLegendLabels() {
  const durationLabels = Object.entries(DURATION_COLORS).map(
    ([key, color]) => ({
      text: DURATION_LABELS[key],
      fillStyle: color,
      strokeStyle: color,
      lineWidth: 0,
    })
  );
  const statusOverlays = [
    ["failed", "Failed border"],
    ["errors", "Error border"],
  ].map(([status, text]) => ({
    text,
    fillStyle: "#ffffff",
    strokeStyle: STATUS_COLORS[status],
    lineWidth: 3,
  }));

  return [...durationLabels, ...statusOverlays];
}

function slowestSpecs({ all }, topN = DEFAULT_TOP_N) {
  const top = [...all]
    .sort(
      (left, right) =>
        right.runtimeMs - left.runtimeMs ||
        left.fullName.localeCompare(right.fullName)
    )
    .slice(0, topN);
  const annotated = top.map((timing) => {
    const isFailure = timing.state === "failed" || timing.state === "errors";
    return {
      timing,
      bucketColor: DURATION_COLORS[durationBucket(timing)],
      borderColor: isFailure ? STATUS_COLORS[timing.state] : "transparent",
      borderWidth: isFailure ? 3 : 0,
    };
  });

  return {
    name: "slowest-specs",
    size: { width: 2048, height: 720, pixelRatio: 2 },
    config: {
      type: "bar",
      data: {
        labels: annotated.map(({ timing }) => timing.fullName),
        datasets: [
          {
            label: "Duration, seconds",
            data: annotated.map(({ timing }) => toSeconds(timing.runtimeMs)),
            backgroundColor: annotated.map(({ bucketColor }) => bucketColor),
            borderColor: annotated.map(({ borderColor }) => borderColor),
            borderWidth: annotated.map(({ borderWidth }) => borderWidth),
            barPercentage: 0.55,
            categoryPercentage: 0.7,
            states: annotated.map(({ timing }) => timing.state),
          },
        ],
      },
      options: baseOptions(
        "Top slowest successful specs and failed specs (It/Entry)",
        {
          indexAxis: "y",
          plugins: {
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

module.exports = slowestSpecs;
