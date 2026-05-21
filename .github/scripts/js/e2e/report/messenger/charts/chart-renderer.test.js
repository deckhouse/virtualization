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

const mockRenderToBuffer = jest.fn().mockResolvedValue(Buffer.from("png"));

jest.mock("chartjs-node-canvas", () => ({
  ChartJSNodeCanvas: jest.fn().mockImplementation(() => ({
    renderToBuffer: mockRenderToBuffer,
  })),
}));

const { renderClusterCharts } = require("./chart-renderer");
const { ChartJSNodeCanvas } = require("chartjs-node-canvas");

describe("chart-renderer", () => {
  test("returns no files when spec timings are empty", async () => {
    await expect(renderClusterCharts({ specTimings: [] })).resolves.toEqual([]);
  });

  test("renders messenger cluster chart images", async () => {
    const files = await renderClusterCharts({
      cluster: "replicated",
      specTimings: [
        { name: "slow", group: "VM", state: "passed", runtimeMs: 90_000 },
      ],
    });

    expect(files.map(({ name }) => name)).toEqual([
      "replicated-feature-duration-status.png",
    ]);
    for (const file of files) {
      expect(file.buffer).toEqual(Buffer.from("png"));
      expect(file.mimeType).toBe("image/png");
    }
    expect(ChartJSNodeCanvas).toHaveBeenCalledWith(
      expect.objectContaining({ width: 1280, height: 640 })
    );
    expect(
      mockRenderToBuffer.mock.calls.every(
        ([config]) => config.options.devicePixelRatio === 2
      )
    ).toBe(true);
  });
});
