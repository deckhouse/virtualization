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
  buildDurationHistogramConfig,
  buildFeatureTotalsConfig,
  buildStatusStackedConfig,
  buildTopNConfig,
} = require("./chart-config");

const specTimings = [
  { name: "fast pass", group: "VM", state: "passed", runtimeMs: 10_000 },
  { name: "medium skip", group: "Disk", state: "skipped", runtimeMs: 60_000 },
  { name: "slow fail", group: "VM", state: "failed", runtimeMs: 301_000 },
  { name: "error", group: "Network", state: "errors", runtimeMs: 601_000 },
];

describe("chart-config", () => {
  test("builds deterministic top-N config", () => {
    expect(buildTopNConfig(specTimings, 3)).toMatchSnapshot();
  });

  test("builds deterministic duration histogram config", () => {
    expect(buildDurationHistogramConfig(specTimings)).toMatchSnapshot();
  });

  test("builds deterministic feature totals config", () => {
    expect(buildFeatureTotalsConfig(specTimings)).toMatchSnapshot();
  });

  test("builds deterministic status stacked config", () => {
    expect(buildStatusStackedConfig(specTimings)).toMatchSnapshot();
  });
});
