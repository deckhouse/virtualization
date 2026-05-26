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

const { getReportClusterKey } = require("./model");

const defaultManifestPath = "tmp/messenger-charts/manifest.json";

function readChartManifest(manifestPath) {
  if (!fs.existsSync(manifestPath)) {
    return { clusters: {} };
  }

  return JSON.parse(fs.readFileSync(manifestPath, "utf8"));
}

function getClusterChartFiles(report) {
  const clusterKey = getReportClusterKey(report);
  if (!clusterKey) {
    return [];
  }

  const manifestPath = process.env.CHARTS_MANIFEST || defaultManifestPath;
  const manifest = readChartManifest(manifestPath);
  const files = ((manifest.clusters || {})[clusterKey] || []).map((file) => ({
    name: file.name,
    buffer: fs.readFileSync(path.resolve(file.path)),
    mimeType: file.mimeType || "image/png",
  }));

  return files;
}

module.exports = {
  getClusterChartFiles,
};
