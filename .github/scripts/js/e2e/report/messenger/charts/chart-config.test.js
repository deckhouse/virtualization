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

const { buildClusterChartConfigs, slowestSpecs } = require("./chart-config");
const { aggregate } = require("./data");

const specTimings = [
  { name: "fast pass", group: "VM", state: "passed", runtimeMs: 10_000 },
  { name: "medium skip", group: "Disk", state: "skipped", runtimeMs: 60_000 },
  { name: "slow fail", group: "VM", state: "failed", runtimeMs: 301_000 },
  { name: "error", group: "Network", state: "errors", runtimeMs: 601_000 },
  { name: "passing peer", group: "VM", state: "passed", runtimeMs: 45_000 },
];

describe("chart-config", () => {
  test("builds deterministic cluster chart configs", () => {
    expect(buildClusterChartConfigs(specTimings)).toMatchSnapshot();
  });

  test("returns the messenger chart config in display order", () => {
    const configs = buildClusterChartConfigs(specTimings);
    expect(configs.map(({ name }) => name)).toEqual([
      "feature-duration-status",
    ]);
  });

  test("handles an empty spec timings list", () => {
    const configs = buildClusterChartConfigs([]);
    expect(configs).toHaveLength(1);
    const labelsByName = Object.fromEntries(
      configs.map(({ name, config }) => [name, config.data.labels])
    );
    expect(labelsByName["feature-duration-status"]).toEqual([]);
  });

  test("normalizes non-numeric runtimes to zero", () => {
    const configs = buildClusterChartConfigs([
      { runtimeMs: "slow", name: "x", group: "g", state: "passed" },
    ]);
    const numericValues = configs.flatMap(({ config }) =>
      config.data.datasets.flatMap((dataset) =>
        dataset.data.filter((value) => typeof value === "number")
      )
    );

    expect(numericValues).toContain(0);
    expect(numericValues.some((value) => Number.isNaN(value))).toBe(false);
  });

  test("builds slowest specs sorted by duration descending", () => {
    const chart = slowestSpecs(
      aggregate([
        { name: "middle", group: "VM", state: "passed", runtimeMs: 90_000 },
        { name: "slow b", group: "VM", state: "passed", runtimeMs: 180_000 },
        { name: "slow a", group: "Disk", state: "passed", runtimeMs: 180_000 },
        { name: "fast", group: "Network", state: "passed", runtimeMs: 10_000 },
      ])
    );

    expect(chart.config.data.labels).toEqual([
      "Disk / slow a",
      "VM / slow b",
      "VM / middle",
      "Network / fast",
    ]);
    expect(chart.config.data.datasets[0].data).toEqual([180, 180, 90, 10]);
  });
});
