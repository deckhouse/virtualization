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
  STATUSES,
  STATUS_COLORS,
  toSeconds,
  formatSeconds,
  baseOptions,
} = require("../data");
const { valueLabelsPlugin } = require("../plugins");

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
  const height = Math.max(640, 120 + labels.length * 36);

  return {
    name: "feature-duration-status",
    size: { width: 1280, height, pixelRatio: 2 },
    config: {
      type: "bar",
      data: { labels, datasets },
      options: baseOptions("Overall durations for Describes", {
        indexAxis: "y",
        plugins: {
          legend: { display: true },
          valueLabels: { formatter: formatSeconds },
        },
        scales: {
          x: {
            stacked: true,
            beginAtZero: true,
            ticks: { stepSize: 60 },
            title: { display: true, text: "Duration, seconds" },
          },
          y: { stacked: true },
        },
      }),
      plugins: [valueLabelsPlugin],
    },
  };
}

module.exports = featureDurationStatus;
