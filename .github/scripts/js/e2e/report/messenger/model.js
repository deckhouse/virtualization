const genericArtifactMissingLabel = "E2E REPORT ARTIFACT NOT FOUND";

/**
 * Builds a user-facing status line for a cluster row or fallback report.
 *
 * @param {string} status Normalized cluster status.
 * @param {string} stageLabel Human-readable stage label.
 * @returns {string} Rendered status message.
 */
function buildStatusMessage(status, stageLabel) {
  if (status === "cancelled") {
    return `⚠️ ${stageLabel} CANCELLED`;
  }

  if (status === "failure") {
    return `❌ ${stageLabel} FAILED`;
  }

  if (status === "missing") {
    return `⚠️ ${stageLabel}`;
  }

  if (status === "success") {
    return "✅ SUCCESS";
  }

  return stageLabel;
}

/**
 * Creates a synthetic cluster report when the expected JSON artifact is absent.
 *
 * This allows the final messenger message to stay informative even when the
 * report-preparation step failed or never produced an artifact.
 *
 * @param {string} clusterName Cluster or storage name.
 * @returns {Record<string, any>} Synthetic report payload.
 */
function createMissingReport(clusterName) {
  return {
    schemaVersion: 1,
    cluster: clusterName,
    storageType: clusterName,
    reportKind: "artifact-missing",
    status: "missing",
    statusMessage: buildStatusMessage("missing", genericArtifactMissingLabel),
    failedStage: "artifact-missing",
    failedStageLabel: genericArtifactMissingLabel,
    branch: "",
    workflowRunUrl: "",
    clusterStatus: {
      status: "missing",
      stage: "artifact-missing",
      stageLabel: genericArtifactMissingLabel,
      message: buildStatusMessage("missing", genericArtifactMissingLabel),
      reason: "cluster-report-artifact-missing",
    },
    testStatus: {
      status: "not-run",
      reason: "cluster-report-artifact-missing",
      message: "E2E status is unavailable because cluster report artifact was not found",
    },
    metrics: {
      passed: 0,
      failed: 0,
      errors: 0,
      skipped: 0,
      total: 0,
      successRate: 0,
    },
    failedTests: [],
    reportSource: "missing-artifact",
  };
}

/**
 * Picks a report date from the first report that exposes `startedAt`.
 *
 * @param {Record<string, any>[]} reports Available cluster reports.
 * @returns {string} ISO date string (`YYYY-MM-DD`).
 */
function getReportDate(reports) {
  const datedReport = reports.find((report) => report.startedAt);
  if (!datedReport) {
    return new Date().toISOString().slice(0, 10);
  }

  return String(datedReport.startedAt).slice(0, 10);
}

/**
 * Orders reports by the configured cluster order and then by cluster name.
 *
 * @param {Record<string, any>[]} reports Reports to sort.
 * @param {string[]} preferredOrder Configured cluster order.
 * @returns {Record<string, any>[]} Sorted reports copy.
 */
function sortReports(reports, preferredOrder) {
  const orderMap = new Map(preferredOrder.map((name, index) => [name, index]));

  return [...reports].sort((left, right) => {
    const leftKey = left.storageType || left.cluster;
    const rightKey = right.storageType || right.cluster;
    const leftOrder = orderMap.has(leftKey)
      ? orderMap.get(leftKey)
      : Number.MAX_SAFE_INTEGER;
    const rightOrder = orderMap.has(rightKey)
      ? orderMap.get(rightKey)
      : Number.MAX_SAFE_INTEGER;

    if (leftOrder !== rightOrder) {
      return leftOrder - rightOrder;
    }

    return String(left.cluster || left.storageType).localeCompare(
      String(right.cluster || right.storageType)
    );
  });
}

/**
 * Extracts the normalized cluster key from a report payload.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {string} Cluster key or an empty string when it is missing.
 */
function getReportClusterKey(report) {
  return String(report.storageType || report.cluster || "").trim();
}

/**
 * Tells whether the report represents a missing artifact rather than a real
 * cluster-stage failure.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {boolean} True when the report describes a missing artifact.
 */
function isMissingReport(report) {
  return (
    (report.testStatus && report.testStatus.status === "missing") ||
    (report.clusterStatus && report.clusterStatus.status === "missing") ||
    report.reportKind === "artifact-missing" ||
    report.failedStage === "artifact-missing" ||
    report.status === "missing"
  );
}

/**
 * Tells whether the report describes a failed cluster setup stage.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {boolean} True for cluster-stage failures.
 */
function isClusterFailureReport(report) {
  if (report.clusterStatus) {
    return (
      report.clusterStatus.status !== "success" &&
      report.clusterStatus.status !== "missing"
    );
  }

  return report.reportKind !== "tests" && !isMissingReport(report);
}

/**
 * Tells whether the report should be rendered in the E2E test results table.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {boolean} True for reports with test status data.
 */
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
  createMissingReport,
  getReportClusterKey,
  getReportDate,
  isClusterFailureReport,
  isMissingReport,
  isTestResultReport,
  sortReports,
};
