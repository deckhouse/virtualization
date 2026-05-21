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

const { listMatchingFiles } = require("./shared/fs-utils");
const { REPORT_FILE_PATTERN } = require("./shared/report-model");
const { getReportClusterKey } = require("./messenger/model");
const {
  deriveStorageType,
  renderSlowestForDescribe,
} = require("./render-slowest-for-describe");

function parseArgs(argv) {
  const args = {};

  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith("--")) {
      continue;
    }

    const key = token.slice(2);
    const value = argv[index + 1];
    if (!value || value.startsWith("--")) {
      args[key] = true;
      continue;
    }

    args[key] = value;
    index += 1;
  }

  return args;
}

function topDescribes(specTimings, topN = 5) {
  const totals = new Map();

  for (const timing of specTimings || []) {
    const group = String((timing && timing.group) || "").trim();
    if (!group) {
      continue;
    }

    totals.set(group, (totals.get(group) || 0) + Number(timing.runtimeMs || 0));
  }

  return [...totals.entries()]
    .sort(
      (left, right) => right[1] - left[1] || left[0].localeCompare(right[0])
    )
    .slice(0, topN)
    .map(([describe]) => describe);
}

function readReport(jsonPath) {
  return JSON.parse(fs.readFileSync(jsonPath, "utf8"));
}

async function renderTopDescribesForCluster({
  jsonPath,
  storage,
  outDir = "tmp",
  topN = 5,
}) {
  const report = readReport(jsonPath);
  const describes = topDescribes(report.specTimings, topN);
  const storageName =
    storage || getReportClusterKey(report) || deriveStorageType(jsonPath);
  const renderedFiles = [];

  for (const describe of describes) {
    renderedFiles.push(
      await renderSlowestForDescribe({
        jsonPath,
        describe,
        outDir,
        storage: storageName,
      })
    );
  }

  return renderedFiles;
}

async function renderTopDescribes({
  core = console,
  reportsDir = "downloaded-artifacts",
  outDir = "tmp",
  topN = 5,
} = {}) {
  const reportFiles = listMatchingFiles(reportsDir, REPORT_FILE_PATTERN);
  const renderedFiles = [];

  for (const reportFile of reportFiles) {
    try {
      const files = await renderTopDescribesForCluster({
        jsonPath: reportFile,
        outDir,
        topN,
      });
      renderedFiles.push(...files);
      if (core.info) {
        core.info(
          `Rendered ${files.length} slowest-specs charts from ${reportFile}`
        );
      }
    } catch (error) {
      if (core.warning) {
        core.warning(
          `Unable to render top Describe charts for ${reportFile}: ${error.message}`
        );
      }
    }
  }

  return renderedFiles;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(
      "Usage: node .github/scripts/js/e2e/report/render-top-describes.js [--reports-dir downloaded-artifacts] [--out-dir tmp] [--top-n 5]"
    );
    return;
  }

  const files = await renderTopDescribes({
    reportsDir: args["reports-dir"],
    outDir: args["out-dir"],
    topN: Number(args["top-n"] || 5),
  });
  files.forEach((file) => console.log(file));
}

if (require.main === module) {
  main().catch((error) => {
    console.error(`[ERROR] ${error.message}`);
    process.exit(1);
  });
}

module.exports = renderTopDescribes;
module.exports.renderTopDescribes = renderTopDescribes;
module.exports.renderTopDescribesForCluster = renderTopDescribesForCluster;
module.exports.topDescribes = topDescribes;
