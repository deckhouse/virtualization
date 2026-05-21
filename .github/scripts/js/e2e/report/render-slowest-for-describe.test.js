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

const fs = require("fs");
const path = require("path");

jest.mock("./messenger/charts/chart-renderer", () => ({
  renderChartBuffer: jest.fn().mockResolvedValue(Buffer.from("png")),
  sanitizeFilenamePart: (value) =>
    String(value || "cluster").replace(/[^a-zA-Z0-9_-]+/g, "_") || "cluster",
}));

const { renderSlowestForDescribe } = require("./render-slowest-for-describe");
const { topDescribes } = require("./render-top-describes");
const { withTempDir } = require("./shared/test-utils");

function writeReport(tempDir, report) {
  const jsonPath = path.join(tempDir, "e2e_report_nfs_2026-05-15.json");
  fs.writeFileSync(jsonPath, JSON.stringify(report));
  return jsonPath;
}

describe("render-slowest-for-describe", () => {
  test("renders one slowest-specs PNG for the requested Describe", async () =>
    withTempDir("render-slowest-for-describe", async (tempDir) => {
      const jsonPath = writeReport(tempDir, {
        storageType: "nfs",
        specTimings: [
          { name: "fast", group: "VM", state: "passed", runtimeMs: 10_000 },
          { name: "slow", group: "VM", state: "passed", runtimeMs: 90_000 },
          { name: "disk", group: "Disk", state: "passed", runtimeMs: 30_000 },
        ],
      });

      const targetPath = await renderSlowestForDescribe({
        jsonPath,
        describe: "VM",
        outDir: tempDir,
      });

      expect(targetPath).toBe(
        path.join(tempDir, "charts", "nfs-VM-slowest-specs.png")
      );
      expect(fs.readFileSync(targetPath)).toEqual(Buffer.from("png"));
    }));

  test("fails with available Describe names when the requested one is absent", async () =>
    withTempDir("render-slowest-for-describe", async (tempDir) => {
      const jsonPath = writeReport(tempDir, {
        specTimings: [
          { name: "disk", group: "Disk", state: "passed", runtimeMs: 30_000 },
          { name: "vm", group: "VM", state: "passed", runtimeMs: 10_000 },
        ],
      });

      await expect(
        renderSlowestForDescribe({
          jsonPath,
          describe: "Network",
          outDir: tempDir,
        })
      ).rejects.toThrow("Available Describes:\n- Disk\n- VM");
    }));
});

describe("render-top-describes", () => {
  test("selects top Describes by total duration with name tiebreak", () => {
    expect(
      topDescribes(
        [
          { group: "VM", runtimeMs: 30_000 },
          { group: "Disk", runtimeMs: 20_000 },
          { group: "Network", runtimeMs: 20_000 },
          { group: "VM", runtimeMs: 5_000 },
        ],
        2
      )
    ).toEqual(["VM", "Disk"]);
  });
});
