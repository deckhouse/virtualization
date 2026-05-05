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

const { findSingleMatchingFile } = require("./shared/fs-utils");
const { parseGinkgoReport } = require("./shared/ginkgo-report-utils");
const {
  buildClusterStatus,
  buildReportSummary,
  buildTestStatus,
  zeroMetrics,
} = require("./shared/report-model");

/**
 * @typedef {Record<string, string|undefined>} StageResults
 */

/**
 * @typedef {Object} GinkgoMetrics
 * @property {number} [failed]
 * @property {number} [errors]
 */

/**
 * @typedef {Object} ClusterReportCore
 * @property {function(string): void} info
 * @property {function(string): void} warning
 * @property {function(string, string): void} setOutput
 */

/**
 * @typedef {Object} ClusterReportContext
 * @property {string} serverUrl
 * @property {{ owner: string, repo: string }} repo
 * @property {string|number} runId
 * @property {string} [ref]
 */

/**
 * @typedef {Object} ClusterReportConfig
 * @property {string} storageType
 * @property {string} reportsDir
 * @property {string} reportFile
 * @property {string} [workflowRunUrl]
 * @property {string} [branchName]
 * @property {StageResults} stageResults
 */

/**
 * @typedef {Object} ClusterReportParams
 * @property {ClusterReportCore} core
 * @property {ClusterReportContext} context
 * @property {ClusterReportConfig} [config]
 */

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function readClusterReportConfigFromEnv(env = process.env) {
  const storageType = String(env.STORAGE_TYPE || "").trim();

  return {
    storageType,
    reportsDir: env.REPORTS_DIR || "test/e2e",
    reportFile: env.REPORT_FILE || `e2e_report_${storageType}.json`,
    workflowRunUrl: String(env.WORKFLOW_RUN_URL || "").trim(),
    branchName: String(env.BRANCH_NAME || "").trim(),
    stageResults: {
      bootstrap: env.BOOTSTRAP_RESULT,
      "configure-sdn": env.CONFIGURE_SDN_RESULT,
      "storage-setup": env.STORAGE_SETUP_RESULT,
      "virtualization-setup": env.VIRTUALIZATION_SETUP_RESULT,
      "e2e-test": env.E2E_TEST_RESULT,
    },
  };
}

function requireClusterReportConfig(config) {
  if (!config.storageType) {
    throw new Error("buildClusterReport requires storageType");
  }

  if (!config.reportsDir) {
    throw new Error("buildClusterReport requires reportsDir");
  }

  if (!config.reportFile) {
    throw new Error("buildClusterReport requires reportFile");
  }

  return {
    ...config,
    stageResults: config.stageResults || {},
  };
}

function getWorkflowRunUrl(config, context) {
  if (config.workflowRunUrl) {
    return config.workflowRunUrl;
  }

  return `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
}

function getBranchName(config, context) {
  return (
    config.branchName || String(context.ref || "").replace(/^refs\/heads\//, "")
  );
}

function findGinkgoReport(config) {
  const rawReportPattern = new RegExp(
    `^e2e_report_${escapeRegExp(config.storageType)}_.*\\.json$`
  );

  return findSingleMatchingFile(
    config.reportsDir,
    rawReportPattern,
    "Ginkgo JSON report"
  );
}

function parseGinkgoReportFile(rawReportPath, core) {
  if (!rawReportPath) {
    return {
      metrics: zeroMetrics(),
      failedTests: [],
      startedAt: null,
      source: "empty",
    };
  }

  core.info(`Found Ginkgo JSON report: ${rawReportPath}`);
  try {
    return {
      ...parseGinkgoReport(fs.readFileSync(rawReportPath, "utf8"), zeroMetrics),
      source: "ginkgo-json",
    };
  } catch (error) {
    core.warning(
      `Unable to parse Ginkgo JSON report ${rawReportPath}: ${error.message}`
    );
    return {
      metrics: zeroMetrics(),
      failedTests: [],
      startedAt: null,
      source: "ginkgo-json-invalid",
    };
  }
}

function buildReportPayload({
  config,
  context,
  workflowRunUrl,
  branchName,
  parsedReport,
  rawReportPath,
}) {
  const clusterStatus = buildClusterStatus(config.stageResults);
  const testStatus = buildTestStatus(
    config.stageResults["e2e-test"],
    parsedReport.source,
    clusterStatus,
    parsedReport.metrics
  );
  const reportSummary = buildReportSummary(
    config.storageType,
    clusterStatus,
    testStatus
  );

  return {
    schemaVersion: 1,
    cluster: config.storageType,
    storageType: config.storageType,
    reportKind: reportSummary.reportKind,
    status: reportSummary.status,
    statusMessage: reportSummary.statusMessage,
    failedStage: reportSummary.failedStage,
    failedStageLabel: reportSummary.failedStageLabel,
    failedJobName: reportSummary.failedJobName,
    workflowRunId: String(context.runId),
    workflowRunUrl,
    branch: branchName,
    clusterStatus,
    testStatus,
    startedAt: parsedReport.startedAt,
    metrics: parsedReport.metrics,
    failedTests: parsedReport.failedTests,
    sourceReport: rawReportPath,
    reportSource: parsedReport.source,
  };
}

/**
 * Exposes the generated report fields as GitHub Actions step outputs.
 *
 * @param {Record<string, any>} report Final cluster report payload.
 * @param {string} reportFile Path to the written JSON report file.
 * @param {ClusterReportCore} core GitHub core API.
 */
function setReportOutputs(report, reportFile, core) {
  core.setOutput("report_file", reportFile);
  core.setOutput("report_kind", report.reportKind || "");
  core.setOutput("status", report.status || "");
  core.setOutput("failed_stage", report.failedStage || "");
  core.setOutput("failed_stage_label", report.failedStageLabel || "");
  core.setOutput("workflow_run_url", report.workflowRunUrl || "");
  core.setOutput("branch", report.branch || "");
}

/**
 * Builds a per-cluster JSON report from workflow stage results and an optional
 * raw Ginkgo JSON report, writes it to disk, and publishes step outputs.
 *
 * @param {ClusterReportParams} params GitHub script dependencies.
 * @returns {Promise<Record<string, any>>} Generated cluster report.
 * @throws {Error} If config is incomplete or the report file cannot be written.
 */
async function buildClusterReport({ core, context, config } = {}) {
  const resolvedConfig = requireClusterReportConfig(
    config || readClusterReportConfigFromEnv()
  );

  const workflowRunUrl = getWorkflowRunUrl(resolvedConfig, context);
  const branchName = getBranchName(resolvedConfig, context);
  const rawReportPath = findGinkgoReport(resolvedConfig);

  if (!rawReportPath) {
    core.warning(
      `Ginkgo JSON report was not found for ${resolvedConfig.storageType} under ${resolvedConfig.reportsDir}`
    );
  }

  const parsedReport = parseGinkgoReportFile(rawReportPath, core);
  const report = buildReportPayload({
    config: resolvedConfig,
    context,
    workflowRunUrl,
    branchName,
    parsedReport,
    rawReportPath,
  });

  try {
    fs.writeFileSync(
      resolvedConfig.reportFile,
      `${JSON.stringify(report, null, 2)}\n`
    );
  } catch (error) {
    throw new Error(
      `Unable to write cluster report file ${resolvedConfig.reportFile}: ${error.message}`
    );
  }

  setReportOutputs(report, resolvedConfig.reportFile, core);
  core.info(`Created report file: ${resolvedConfig.reportFile}`);
  core.info(JSON.stringify(report, null, 2));

  return report;
}

module.exports = buildClusterReport;
module.exports.buildClusterStatus = buildClusterStatus;
module.exports.readClusterReportConfigFromEnv = readClusterReportConfigFromEnv;
