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

const stageMessage = {
  "bootstrap": "BOOTSTRAP CLUSTER",
  "configure-sdn": "CONFIGURE SDN",
  "storage-setup": "STORAGE SETUP",
  "virtualization-setup": "VIRTUALIZATION SETUP",
  "e2e-test": "E2E TEST",
  "ready": "CLUSTER READY",
  "artifact-missing": "TEST REPORTS NOT FOUND",
};

const clusterSetupStages = [
  "bootstrap",
  "configure-sdn",
  "storage-setup",
  "virtualization-setup",
];

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
 * @property {ClusterReportConfig} config
 */

/**
 * Escapes special characters in a string for safe use inside a RegExp source.
 *
 * @param {string} value Raw string value.
 * @returns {string} Escaped RegExp fragment.
 */
function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Creates a zero-filled metrics object for cluster report defaults.
 *
 * @returns {{
 *   passed: number,
 *   failed: number,
 *   errors: number,
 *   skipped: number,
 *   total: number,
 *   successRate: number
 * }} Zeroed metrics payload.
 */
function zeroMetrics() {
  return {
    passed: 0,
    failed: 0,
    errors: 0,
    skipped: 0,
    total: 0,
    successRate: 0,
  };
}

/**
 * Builds a user-facing status line for a workflow stage.
 *
 * @param {string} status Normalized stage status.
 * @param {string} stageLabel Human-readable stage label.
 * @returns {string} Rendered status message.
 */
function buildStatusMessage(status, stageLabel) {
  if (status === "success") {
    return `✅ ${stageLabel}`;
  }

  if (status === "cancelled") {
    return `⚠️ ${stageLabel} CANCELLED`;
  }

  if (status === "missing") {
    return `⚠️ ${stageLabel}`;
  }

  if (status === "not-run") {
    return `⚠️ ${stageLabel} NOT RUN`;
  }

  return `❌ ${stageLabel} FAILED`;
}

/**
 * Normalizes a GitHub Actions job result into the report status vocabulary.
 *
 * @param {string|undefined} resultValue Raw GitHub Actions result value.
 * @returns {"success"|"failure"|"cancelled"|"skipped"} Normalized result.
 */
function normalizeJobResult(resultValue) {
  const result = String(resultValue || "success").trim();
  if (result === "cancelled" || result === "skipped" || result === "success") {
    return result;
  }

  return "failure";
}

/**
 * Builds the cluster setup status from pre-E2E workflow stages.
 *
 * @param {StageResults} stageResults Per-stage GitHub Actions results.
 * @returns {{
 *   status: string,
 *   stage: string,
 *   stageLabel: string,
 *   message: string,
 *   reason: string
 * }} Normalized cluster setup status.
 */
function buildClusterStatus(stageResults) {
  for (const stageName of clusterSetupStages) {
    const stageResult = normalizeJobResult(stageResults[stageName]);
    if (stageResult !== "success") {
      const stageLabel = stageMessage[stageName] || stageName;
      return {
        status: stageResult === "cancelled" ? "cancelled" : "failure",
        stage: stageName,
        stageLabel,
        message: buildStatusMessage(stageResult, stageLabel),
        reason:
          stageResult === "cancelled"
            ? "cluster-stage-cancelled"
            : "cluster-stage-failed",
      };
    }
  }

  return {
    status: "success",
    stage: "ready",
    stageLabel: stageMessage.ready,
    message: buildStatusMessage("success", stageMessage.ready),
    reason: "",
  };
}

/**
 * Builds E2E test status from test job result and Ginkgo report availability.
 *
 * @param {string|undefined} testResult Raw E2E job result.
 * @param {string} reportSource Parsed report source.
 * @param {{ status: string }} clusterStatus Cluster setup status.
 * @param {GinkgoMetrics} [metrics={}] Parsed Ginkgo metrics.
 * @returns {{
 *   status: string,
 *   reason: string,
 *   message: string
 * }} Normalized test status.
 */
function buildTestStatus(testResult, reportSource, clusterStatus, metrics = {}) {
  const stageLabel = stageMessage["e2e-test"];

  if (clusterStatus.status !== "success") {
    return {
      status: "not-run",
      reason: "cluster-stage-failed",
      message: "E2E tests were not run because cluster setup did not finish",
    };
  }

  const normalizedResult = normalizeJobResult(testResult);

  if (reportSource === "ginkgo-json") {
    const hasReportedFailures =
      Number(metrics.failed || 0) > 0 || Number(metrics.errors || 0) > 0;
    const status =
      normalizedResult === "success" && hasReportedFailures
        ? "failure"
        : normalizedResult;

    return {
      status,
      reason: status === "success" ? "" : "ginkgo-failed",
      message:
        status === "success"
          ? "✅ E2E TESTS PASSED"
          : buildStatusMessage(status, stageLabel),
    };
  }

  if (reportSource === "ginkgo-json-invalid") {
    return {
      status: "missing",
      reason: "ginkgo-report-invalid",
      message: "⚠️ E2E TEST REPORT IS INVALID",
    };
  }

  if (normalizedResult === "success") {
    return {
      status: "missing",
      reason: "ginkgo-report-missing",
      message: "⚠️ E2E TEST REPORT NOT FOUND",
    };
  }

  if (normalizedResult === "cancelled") {
    return {
      status: "cancelled",
      reason: "e2e-cancelled",
      message: buildStatusMessage("cancelled", stageLabel),
    };
  }

  if (normalizedResult === "skipped") {
    return {
      status: "not-run",
      reason: "e2e-skipped",
      message: buildStatusMessage("not-run", stageLabel),
    };
  }

  return {
    status: "failure",
    reason: "ginkgo-report-missing",
    message: "❌ E2E TESTS FAILED, GINKGO REPORT NOT FOUND",
  };
}

/**
 * Builds flat summary fields derived from cluster and test statuses.
 *
 * @param {string} storageType Storage backend name.
 * @param {{ status: string, stage: string, stageLabel: string, message: string }} clusterStatus Cluster setup status.
 * @param {{ status: string, message: string }} testStatus Test status.
 * @returns {{
 *   failedStage: string,
 *   failedStageLabel: string,
 *   failedJobName: string,
 *   reportKind: string,
 *   status: string,
 *   statusMessage: string
 * }} Report summary descriptor.
 */
function buildReportSummary(storageType, clusterStatus, testStatus) {
  if (clusterStatus.status !== "success") {
    return {
      failedStage: clusterStatus.stage,
      failedStageLabel: clusterStatus.stageLabel,
      failedJobName: `${clusterStatus.stageLabel} (${storageType})`,
      reportKind: "stage-failure",
      status: clusterStatus.status,
      statusMessage: clusterStatus.message,
    };
  }

  if (testStatus.status === "missing") {
    const stageLabel = stageMessage["artifact-missing"];
    return {
      failedStage: "artifact-missing",
      failedStageLabel: stageLabel,
      failedJobName: `E2E test (${storageType})`,
      reportKind: "artifact-missing",
      status: "missing",
      statusMessage: testStatus.message,
    };
  }

  return {
    failedStage: testStatus.status === "success" ? "success" : "e2e-test",
    failedStageLabel:
      testStatus.status === "success" ? "SUCCESS" : stageMessage["e2e-test"],
    failedJobName: `E2E test (${storageType})`,
    reportKind: "tests",
    status: testStatus.status,
    statusMessage: testStatus.message,
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
 * @throws {Error} If `config` is missing or the report file cannot be written.
 */
async function buildClusterReport({ core, context, config } = {}) {
  if (!config) {
    throw new Error("buildClusterReport requires a config object");
  }

  const workflowRunUrl =
    config.workflowRunUrl ||
    `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
  const branchName =
    config.branchName || String(context.ref || "").replace(/^refs\/heads\//, "");
  const rawReportPattern = new RegExp(
    `^e2e_report_${escapeRegExp(config.storageType)}_.*\\.json$`
  );
  const rawReportPath = findSingleMatchingFile(
    config.reportsDir,
    rawReportPattern,
    "Ginkgo JSON report"
  );
  const clusterStatus = buildClusterStatus(config.stageResults);

  let parsedReport = {
    metrics: zeroMetrics(),
    failedTests: [],
    startedAt: null,
    source: "empty",
  };

  if (rawReportPath) {
    core.info(`Found Ginkgo JSON report: ${rawReportPath}`);
    try {
      parsedReport = {
        ...parseGinkgoReport(fs.readFileSync(rawReportPath, "utf8"), zeroMetrics),
        source: "ginkgo-json",
      };
    } catch (error) {
      parsedReport.source = "ginkgo-json-invalid";
      core.warning(
        `Unable to parse Ginkgo JSON report ${rawReportPath}: ${error.message}`
      );
    }
  } else {
    core.warning(
      `Ginkgo JSON report was not found for ${config.storageType} under ${config.reportsDir}`
    );
  }

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

  const report = {
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

  try {
    fs.writeFileSync(config.reportFile, `${JSON.stringify(report, null, 2)}\n`);
  } catch (error) {
    throw new Error(
      `Unable to write cluster report file ${config.reportFile}: ${error.message}`
    );
  }

  setReportOutputs(report, config.reportFile, core);
  core.info(`Created report file: ${config.reportFile}`);
  core.info(JSON.stringify(report, null, 2));

  return report;
}

module.exports = buildClusterReport;
module.exports.buildClusterStatus = buildClusterStatus;
