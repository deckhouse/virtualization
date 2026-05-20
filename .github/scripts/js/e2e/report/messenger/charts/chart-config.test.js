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

const { buildClusterChartConfigs } = require("./chart-config");

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

  test("returns the five chart configs in display order", () => {
    const configs = buildClusterChartConfigs(specTimings);
    expect(configs.map(({ name }) => name)).toEqual([
      "status-doughnut",
      "pareto-slowest",
      "pass-rate-per-feature",
      "quantiles-per-feature",
      "feature-totals",
    ]);
  });

  test("handles an empty spec timings list", () => {
    const configs = buildClusterChartConfigs([]);
    expect(configs).toHaveLength(5);
    const labelsByName = Object.fromEntries(
      configs.map(({ name, config }) => [name, config.data.labels])
    );
    expect(labelsByName["status-doughnut"]).toEqual([
      "passed",
      "failed",
      "errors",
      "skipped",
    ]);
    expect(labelsByName["pareto-slowest"]).toEqual([]);
    expect(labelsByName["pass-rate-per-feature"]).toEqual([]);
    expect(labelsByName["quantiles-per-feature"]).toEqual([]);
    expect(labelsByName["feature-totals"]).toEqual([]);
  });
});
