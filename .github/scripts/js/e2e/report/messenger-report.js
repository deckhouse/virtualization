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
const { makeThreadedReportInLoop } = require("./messenger/loop-client");
const { readMessengerConfigFromEnv } = require("./messenger/config");
const {
  createMissingReport,
  getReportClusterKey,
} = require("./messenger/model");
const {
  buildMainMessage,
  buildThreadMessages,
} = require("./messenger/markdown");

/**
 * @typedef {Object} MessengerReportCore
 * @property {function(string): void} warning
 * @property {function(string): void} [info]
 * @property {function(string, string): void} [setOutput]
 */

/**
 * @typedef {Object} MessengerMessagesParams
 * @property {string} reportsDir
 * @property {string[]} configuredClusters
 * @property {MessengerReportCore} core
 */

/**
 * @typedef {Object} RenderMessengerReportParams
 * @property {MessengerReportCore} core
 * @property {string} [reportsDir]
 */

/**
 * Loads report JSON files from disk and injects synthetic reports for clusters
 * whose artifacts are missing.
 *
 * The result is ordered as follows:
 *  1. Configured clusters in their declared order (missing ones get synthetic reports).
 *  2. Any extra clusters found on disk, sorted alphabetically.
 *
 * @param {string} reportsDir Directory containing `e2e_report_*.json`.
 * @param {string[]} configuredClusters Clusters expected in the final report.
 * @param {MessengerReportCore} core GitHub core API.
 * @returns {Array<Record<string, any>>} Ordered cluster reports.
 */
function readReports(reportsDir, configuredClusters, core) {
  const reportFiles = listMatchingFiles(reportsDir, REPORT_FILE_PATTERN);
  const reportsByCluster = new Map();

  for (const reportFile of reportFiles) {
    try {
      const report = JSON.parse(fs.readFileSync(reportFile, "utf8"));
      const clusterName = getReportClusterKey(report);
      if (!clusterName) {
        // cluster-report.js always writes storageType; a missing key means
        // the file is corrupt or was not produced by this pipeline.
        throw new Error(`report is missing storageType/cluster fields`);
      }
      reportsByCluster.set(clusterName, report);
    } catch (error) {
      core.warning(`Unable to load ${reportFile}: ${error.message}`);
    }
  }

  // Configured clusters first, in declared order; missing ones get synthetic reports.
  const result = configuredClusters.map(
    (name) => reportsByCluster.get(name) ?? createMissingReport(name)
  );

  // Any extra clusters not in the configured list, sorted alphabetically.
  const configuredSet = new Set(configuredClusters);
  const extras = [];
  for (const [key, report] of reportsByCluster) {
    if (!configuredSet.has(key)) {
      extras.push(report);
    }
  }
  extras.sort((a, b) =>
    getReportClusterKey(a).localeCompare(getReportClusterKey(b))
  );

  return [...result, ...extras];
}

/**
 * Reads cluster reports from disk and builds both messenger message bodies.
 *
 * @param {MessengerMessagesParams} params Message rendering inputs.
 * @returns {{
 *   message: string,
 *   threadMessage: string,
 *   threadMessages: string[]
 * }} Rendered markdown payloads.
 */
function buildMessengerMessages({ reportsDir, configuredClusters, core }) {
  const orderedReports = readReports(reportsDir, configuredClusters, core);
  const threadMessages = buildThreadMessages(orderedReports);
  return {
    message: buildMainMessage(orderedReports),
    threadMessage: threadMessages.join("\n\n"),
    threadMessages,
  };
}

/**
 * Entry point used by `actions/github-script` to render and optionally publish
 * the aggregated E2E messenger report.
 *
 * @param {RenderMessengerReportParams} params GitHub script dependencies.
 * @returns {Promise<{
 *   message: string,
 *   threadMessage: string,
 *   threadMessages: string[]
 * }>} Rendered messages.
 */
async function renderMessengerReport({ core, reportsDir }) {
  const config = readMessengerConfigFromEnv();
  const { message, threadMessage, threadMessages } = buildMessengerMessages({
    reportsDir: reportsDir || config.reportsDir,
    configuredClusters: config.configuredClusters,
    core,
  });

  core.info(message);
  core.setOutput("message", message);
  core.setOutput("thread_message", threadMessage);
  core.setOutput("thread_messages", JSON.stringify(threadMessages));

  if (config.loop) {
    try {
      await makeThreadedReportInLoop({ message, threadMessages, loop: config.loop }, core);
    } catch (error) {
      core.warning(`Unable to deliver report to Loop API: ${error.message}`);
    }
  }

  return { message, threadMessage, threadMessages };
}

module.exports = renderMessengerReport;
