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

const { findSingleMatchingFile } = require("./fs-utils");
const { parseGinkgoReport } = require("./ginkgo-report-utils");

const stageLabels = {
  bootstrap: "BOOTSTRAP CLUSTER",
  "configure-sdn": "CONFIGURE SDN",
  "storage-setup": "STORAGE SETUP",
  "virtualization-setup": "VIRTUALIZATION SETUP",
  "e2e-test": "E2E TEST",
  success: "SUCCESS",
  "artifact-missing": "TEST REPORTS NOT FOUND",
};

const preE2EStages = new Set([
  "bootstrap",
  "configure-sdn",
  "storage-setup",
  "virtualization-setup",
]);

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
 * Reads cluster report configuration from environment variables injected by the
 * reusable workflow or the local helper script.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {{
 *   storageType: string,
 *   reportsDir: string,
 *   reportFile: string,
 *   workflowRunUrlOverride: string,
 *   branchNameOverride: string,
 *   stageResults: {
 *     bootstrap: string|undefined,
 *     "configure-sdn": string|undefined,
 *     "storage-setup": string|undefined,
 *     "virtualization-setup": string|undefined,
 *     "e2e-test": string|undefined
 *   }
 * }} Normalized cluster report configuration.
 */
function readClusterConfigFromEnv(env = process.env) {
  const storageType = env.STORAGE_TYPE;

  return {
    storageType,
    reportsDir: env.E2E_REPORT_DIR || "test/e2e",
    reportFile: env.REPORT_FILE || `e2e_report_${storageType}.json`,
    workflowRunUrlOverride: env.WORKFLOW_RUN_URL || "",
    branchNameOverride: env.BRANCH_NAME || "",
    stageResults: {
      bootstrap: env.BOOTSTRAP_RESULT,
      "configure-sdn": env.CONFIGURE_SDN_RESULT,
      "storage-setup": env.CONFIGURE_STORAGE_RESULT,
      "virtualization-setup": env.CONFIGURE_VIRTUALIZATION_RESULT,
      "e2e-test": env.E2E_TEST_RESULT,
    },
  };
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
 * Builds a descriptor for a non-success stage result.
 *
 * @param {string} storageType Storage backend name.
 * @param {string} stageName Failed or cancelled stage name.
 * @param {string} resultValue Raw GitHub Actions result value.
 * @returns {{
 *   failedStage: string,
 *   failedStageLabel: string,
 *   failedJobName: string,
 *   reportKind: string,
 *   status: string,
 *   statusMessage: string
 * }} Descriptor used by the final cluster report.
 */
function getStageDescriptor(storageType, stageName, resultValue) {
  const result = (resultValue || "").trim();
  const stageLabel = stageLabels[stageName] || stageName;
  const reportKind = preE2EStages.has(stageName) ? "stage-failure" : "tests";

  if (result === "cancelled") {
    return {
      failedStage: stageName,
      failedStageLabel: stageLabel,
      failedJobName: `${stageLabel} (${storageType})`,
      reportKind,
      status: "cancelled",
      statusMessage: `⚠️ ${stageLabel} CANCELLED`,
    };
  }

  return {
    failedStage: stageName,
    failedStageLabel: stageLabel,
    failedJobName: `${stageLabel} (${storageType})`,
    reportKind,
    status: "failure",
    statusMessage: `❌ ${stageLabel} FAILED`,
  };
}

/**
 * Determines which workflow stage should be represented in the cluster report.
 *
 * The first non-success stage wins. If every stage succeeded, the cluster is
 * treated as test-capable and the Ginkgo JSON report is expected to describe
 * results.
 *
 * @param {string} storageType Storage backend name.
 * @param {{
 *   bootstrap: string|undefined,
 *   "configure-sdn": string|undefined,
 *   "storage-setup": string|undefined,
 *   "virtualization-setup": string|undefined,
 *   "e2e-test": string|undefined
 * }} stageResults Per-stage GitHub Actions results.
 * @returns {{
 *   failedStage: string,
 *   failedStageLabel: string,
 *   failedJobName: string,
 *   reportKind: string,
 *   status: string,
 *   statusMessage: string
 * }} Normalized stage descriptor.
 */
function determineStage(storageType, stageResults) {
  const orderedStages = [
    ["bootstrap", stageResults.bootstrap],
    ["configure-sdn", stageResults["configure-sdn"]],
    ["storage-setup", stageResults["storage-setup"]],
    ["virtualization-setup", stageResults["virtualization-setup"]],
    ["e2e-test", stageResults["e2e-test"]],
  ];

  for (const [stageName, resultValue] of orderedStages) {
    if ((resultValue || "success") !== "success") {
      return getStageDescriptor(storageType, stageName, resultValue);
    }
  }

  return {
    failedStage: "success",
    failedStageLabel: stageLabels.success,
    failedJobName: `E2E test (${storageType})`,
    reportKind: "tests",
    status: "success",
    statusMessage: "✅ SUCCESS",
  };
}

/**
 * Builds a synthetic report descriptor for a successful test stage that did
 * not produce any raw E2E artifact.
 *
 * @param {string} storageType Storage backend name.
 * @returns {{
 *   failedStage: string,
 *   failedStageLabel: string,
 *   failedJobName: string,
 *   reportKind: string,
 *   status: string,
 *   statusMessage: string
 * }} Artifact-missing descriptor.
 */
function buildArtifactMissingDescriptor(storageType) {
  const stageLabel = stageLabels["artifact-missing"];
  return {
    failedStage: "artifact-missing",
    failedStageLabel: stageLabel,
    failedJobName: `E2E test (${storageType})`,
    reportKind: "artifact-missing",
    status: "missing",
    statusMessage: `⚠️ ${stageLabel}`,
  };
}

/**
 * Exposes the generated report fields as GitHub Actions step outputs.
 *
 * @param {Record<string, any>} report Final cluster report payload.
 * @param {string} reportFile Path to the written JSON report file.
 * @param {{ setOutput(name: string, value: string): void }} core GitHub core API.
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
 * @param {{
 *   core: {
 *     info(message: string): void,
 *     warning(message: string): void,
 *     setOutput(name: string, value: string): void
 *   },
 *   context: {
 *     serverUrl: string,
 *     repo: { owner: string, repo: string },
 *     runId: string|number,
 *     ref?: string
 *   }
 * }} params GitHub script dependencies.
 * @returns {Promise<Record<string, any>>} Generated cluster report.
 */
async function buildClusterReport({ core, context }) {
  const config = readClusterConfigFromEnv();
  const workflowRunUrl =
    config.workflowRunUrlOverride ||
    `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
  const branchName =
    config.branchNameOverride ||
    String(context.ref || "").replace(/^refs\/heads\//, "");
  const rawReportPattern = new RegExp(
    `^e2e_report_${escapeRegExp(config.storageType)}_.*\\.json$`
  );
  const rawReportPath = findSingleMatchingFile(
    config.reportsDir,
    rawReportPattern,
    "Ginkgo JSON report"
  );
  const stageInfo = determineStage(config.storageType, config.stageResults);

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
      core.warning(
        `Unable to parse Ginkgo JSON report ${rawReportPath}: ${error.message}`
      );
    }
  } else {
    core.warning(
      `Ginkgo JSON report was not found for ${config.storageType} under ${config.reportsDir}`
    );
  }

  const effectiveStageInfo =
    stageInfo.status === "success" &&
    stageInfo.reportKind === "tests" &&
    parsedReport.source === "empty"
      ? buildArtifactMissingDescriptor(config.storageType)
      : stageInfo;

  const report = {
    cluster: config.storageType,
    storageType: config.storageType,
    reportKind: effectiveStageInfo.reportKind,
    status: effectiveStageInfo.status,
    statusMessage: effectiveStageInfo.statusMessage,
    failedStage: effectiveStageInfo.failedStage,
    failedStageLabel: effectiveStageInfo.failedStageLabel,
    failedJobName: effectiveStageInfo.failedJobName,
    workflowRunId: String(context.runId),
    workflowRunUrl,
    branch: branchName,
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
module.exports.determineStage = determineStage;
module.exports.parseGinkgoReport = parseGinkgoReport;
module.exports.buildArtifactMissingDescriptor = buildArtifactMissingDescriptor;
module.exports.readClusterConfigFromEnv = readClusterConfigFromEnv;
