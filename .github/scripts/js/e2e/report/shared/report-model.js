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

/** Matches every `e2e_report_*.json` file produced by the pipeline. */
const REPORT_FILE_PATTERN = /^e2e_report_.*\.json$/;

/**
 * Returns the canonical report file name for a given storage type.
 * @param {string} storageType
 * @returns {string}
 */
function reportFileName(storageType) {
  return `e2e_report_${storageType}.json`;
}

/**
 * Returns a regex that matches dated archive copies of a report file,
 * e.g. `e2e_report_replicated_2026-04-15.json`.
 * @param {string} storageType
 * @returns {RegExp}
 */
function archivedReportPattern(storageType) {
  const escaped = storageType.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`^e2e_report_${escaped}_.*\\.json$`);
}

/**
 * Returns a regex that matches Ginkgo stdout/stderr fallback logs,
 * e.g. `e2e_output_replicated_2026-04-15.log`.
 * @param {string} storageType
 * @returns {RegExp}
 */
function ginkgoOutputPattern(storageType) {
  const escaped = storageType.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`^e2e_output_${escaped}_.*\\.log$`);
}

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

function buildStatusMessage(status, stageLabel) {
  if (status === "success") {
    return `✅ ${stageLabel}`;
  }

  if (status === "cancelled") {
    return `⚠️ ${stageLabel} CANCELLED`;
  }

  if (status === "skipped") {
    return `⚠️ ${stageLabel} SKIPPED`;
  }

  if (status === "missing") {
    return `⚠️ ${stageLabel}`;
  }

  if (status === "not-run") {
    return `⚠️ ${stageLabel} NOT RUN`;
  }

  return `❌ ${stageLabel} FAILED`;
}

function normalizeJobResult(resultValue) {
  const result = String(resultValue || "success").trim();
  if (result === "cancelled" || result === "skipped" || result === "success") {
    return result;
  }

  return "failure";
}

function buildClusterStatus(stageResults) {
  for (const stageName of clusterSetupStages) {
    const stageResult = normalizeJobResult(stageResults[stageName]);
    if (stageResult !== "success") {
      const stageLabel = stageMessage[stageName] || stageName;
      const status =
        stageResult === "cancelled"
          ? "cancelled"
          : stageResult === "skipped"
            ? "skipped"
            : "failure";
      return {
        status,
        stage: stageName,
        stageLabel,
        message: buildStatusMessage(stageResult, stageLabel),
        reason: `cluster-stage-${status}`,
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

function buildTestStatus(
  testResult,
  reportSource,
  clusterStatus,
  metrics = {}
) {
  const stageLabel = stageMessage["e2e-test"];

  if (clusterStatus.status !== "success") {
    return {
      status: "not-run",
      reason: `cluster-stage-${clusterStatus.status}`,
      message: "E2E tests were not run because cluster setup did not finish",
    };
  }

  const normalizedResult = normalizeJobResult(testResult);

  if (reportSource === "ginkgo-json" || reportSource === "ginkgo-output") {
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

function isMissingReport(report) {
  return (
    (report.testStatus && report.testStatus.status === "missing") ||
    (report.clusterStatus && report.clusterStatus.status === "missing") ||
    report.reportKind === "artifact-missing" ||
    report.failedStage === "artifact-missing" ||
    report.status === "missing"
  );
}

function isClusterFailureReport(report) {
  if (report.clusterStatus) {
    return (
      report.clusterStatus.status !== "success" &&
      report.clusterStatus.status !== "missing"
    );
  }

  return report.reportKind !== "tests" && !isMissingReport(report);
}

function isTestResultReport(report) {
  if (report.clusterStatus && report.clusterStatus.status !== "success") {
    return false;
  }

  if (report.testStatus) {
    return (
      report.testStatus.status !== "not-run" &&
      report.testStatus.status !== "missing"
    );
  }

  return report.reportKind === "tests";
}

module.exports = {
  archivedReportPattern,
  buildClusterStatus,
  buildReportSummary,
  buildStatusMessage,
  buildTestStatus,
  ginkgoOutputPattern,
  isClusterFailureReport,
  isMissingReport,
  isTestResultReport,
  REPORT_FILE_PATTERN,
  reportFileName,
  zeroMetrics,
};
