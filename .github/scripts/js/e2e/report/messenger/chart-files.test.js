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

const { getClusterChartFiles } = require("./chart-files");
const { withTempDir } = require("../shared/test-utils");

describe("chart-files", () => {
  afterEach(() => {
    delete process.env.CHARTS_MANIFEST;
  });

  test("returns no files when the manifest is missing", () => {
    expect(getClusterChartFiles({ cluster: "replicated" })).toEqual([]);
  });

  test("loads chart files listed in the Python manifest", async () =>
    withTempDir("chart-files", async (tempDir) => {
      const chartPath = path.join(tempDir, "replicated-feature-duration-status.png");
      const manifestPath = path.join(tempDir, "manifest.json");
      fs.writeFileSync(chartPath, Buffer.from("png"));
      fs.writeFileSync(
        manifestPath,
        JSON.stringify({
          clusters: {
            replicated: [
              {
                name: "replicated-feature-duration-status.png",
                path: chartPath,
                mimeType: "image/png",
              },
            ],
          },
        })
      );
      process.env.CHARTS_MANIFEST = manifestPath;

      const files = await getClusterChartFiles({ cluster: "replicated" });

      expect(files.map(({ name }) => name)).toEqual(["replicated-feature-duration-status.png"]);
      expect(files[0].buffer).toEqual(Buffer.from("png"));
      expect(files[0].mimeType).toBe("image/png");
    }));
});
