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

const { aggregate } = require("./messenger/charts/data");
const { slowestSpecs } = require("./messenger/charts");
const {
  renderChartBuffer,
  sanitizeFilenamePart,
} = require("./messenger/charts/chart-renderer");
const { parseGinkgoReport } = require("./shared/ginkgo-report-utils");

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

function deriveStorageType(reportPath, fallbackStorage) {
  const baseName = path.basename(reportPath);
  const datedMatch = baseName.match(
    /^e2e_report_(.+)_(\d{4}-\d{2}-\d{2}.*)\.json$/
  );
  if (datedMatch) {
    return datedMatch[1];
  }

  const genericMatch = baseName.match(/^e2e_report_(.+?)_.*\.json$/);
  if (genericMatch) {
    return genericMatch[1];
  }

  if (fallbackStorage) {
    return fallbackStorage;
  }

  throw new Error(
    `Unable to derive storage type from file name "${baseName}". Pass --storage.`
  );
}

function readReport(jsonPath) {
  const content = fs.readFileSync(jsonPath, "utf8");
  const report = JSON.parse(content);
  if (Array.isArray(report.specTimings)) {
    return report;
  }

  return {
    specTimings: parseGinkgoReport(content).specTimings,
  };
}

function availableDescribes(specTimings) {
  return [
    ...new Set(
      (specTimings || [])
        .map((timing) => String((timing && timing.group) || "").trim())
        .filter(Boolean)
    ),
  ].sort((left, right) => left.localeCompare(right));
}

async function renderSlowestForDescribe({
  jsonPath,
  describe,
  outDir = "tmp/test-ci/report/out",
  storage,
}) {
  if (!jsonPath) {
    throw new Error("--json is required");
  }
  if (!describe) {
    throw new Error("--describe is required");
  }

  const resolvedJsonPath = path.resolve(jsonPath);
  const report = readReport(resolvedJsonPath);
  const specTimings = Array.isArray(report.specTimings)
    ? report.specTimings
    : [];
  const filteredTimings = specTimings.filter(
    (timing) => String((timing && timing.group) || "") === describe
  );

  if (filteredTimings.length === 0) {
    const describes = availableDescribes(specTimings);
    throw new Error(
      [
        `No specs found for Describe "${describe}".`,
        "Available Describes:",
        ...(describes.length > 0 ? describes : ["<none>"]).map(
          (name) => `- ${name}`
        ),
      ].join("\n")
    );
  }

  const chart = slowestSpecs(aggregate(filteredTimings));
  const buffer = await renderChartBuffer(chart);
  const storageName =
    storage ||
    report.storageType ||
    report.cluster ||
    deriveStorageType(resolvedJsonPath);
  const fileName = `${sanitizeFilenamePart(storageName)}-${sanitizeFilenamePart(
    describe
  )}-${chart.name}.png`;
  const chartDir = path.resolve(outDir, "charts");
  const targetPath = path.join(chartDir, fileName);

  fs.mkdirSync(chartDir, { recursive: true });
  fs.writeFileSync(targetPath, buffer);

  return targetPath;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(
      "Usage: node .github/scripts/js/e2e/report/render-slowest-for-describe.js --json <report.json> --describe <Describe> [--out-dir <dir>] [--storage <name>]"
    );
    return;
  }

  const targetPath = await renderSlowestForDescribe({
    jsonPath: args.json,
    describe: args.describe,
    outDir: args["out-dir"],
    storage: args.storage,
  });
  console.log(targetPath);
}

if (require.main === module) {
  main().catch((error) => {
    console.error(`[ERROR] ${error.message}`);
    process.exit(1);
  });
}

module.exports = {
  availableDescribes,
  deriveStorageType,
  renderSlowestForDescribe,
};
