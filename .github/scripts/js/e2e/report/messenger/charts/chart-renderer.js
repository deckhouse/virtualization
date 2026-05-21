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

const { buildClusterChartConfigs } = require(".");

const defaultChartSize = { width: 1280, height: 640, pixelRatio: 2 };
const canvasInstances = new Map();

// Module-level singleton: ChartJSNodeCanvas startup (loading chart.js + setting
// up the cairo-backed canvas) is non-trivial, and the renderer is stateless
// between renderToBuffer calls. Reusing it across clusters keeps memory usage
// flat when the messenger report grows.
function loadChartRenderer({ width, height, pixelRatio } = defaultChartSize) {
  const rendererKey = `${width}x${height}@${pixelRatio}`;
  if (!canvasInstances.has(rendererKey)) {
    const { ChartJSNodeCanvas } = require("chartjs-node-canvas");
    canvasInstances.set(
      rendererKey,
      new ChartJSNodeCanvas({
        width,
        height,
        backgroundColour: "#ffffff",
      })
    );
  }

  return canvasInstances.get(rendererKey);
}

function normalizeChartSize(size) {
  return {
    ...defaultChartSize,
    ...(size || {}),
  };
}

function withDevicePixelRatio(config, pixelRatio) {
  return {
    ...config,
    options: {
      ...(config.options || {}),
      devicePixelRatio: pixelRatio,
    },
  };
}

async function renderChartBuffer({ config, size }) {
  const chartSize = normalizeChartSize(size);
  const renderer = loadChartRenderer(chartSize);
  return renderer.renderToBuffer(
    withDevicePixelRatio(config, chartSize.pixelRatio),
    "image/png"
  );
}

function sanitizeFilenamePart(value) {
  const fallback = "cluster";
  const safe = String(value || fallback).replace(/[^a-zA-Z0-9_-]+/g, "_");
  return safe || fallback;
}

async function renderClusterCharts(report) {
  if (
    !Array.isArray(report && report.specTimings) ||
    report.specTimings.length === 0
  ) {
    return [];
  }

  const configs = buildClusterChartConfigs(report.specTimings);
  const clusterName = sanitizeFilenamePart(
    report.cluster || report.storageType || "cluster"
  );

  return Promise.all(
    configs.map(async ({ name, config, size }) => {
      return {
        name: `${clusterName}-${name}.png`,
        buffer: await renderChartBuffer({ config, size }),
        mimeType: "image/png",
      };
    })
  );
}

module.exports = {
  renderClusterCharts,
  renderChartBuffer,
  sanitizeFilenamePart,
};
