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

const {
  buildStatusMessage,
  isClusterFailureReport,
  isMissingReport,
  isTestResultReport,
  zeroMetrics,
} = require("../shared/report-model");

const genericArtifactMissingLabel = "E2E REPORT ARTIFACT NOT FOUND";

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
      message:
        "E2E status is unavailable because cluster report artifact was not found",
    },
    metrics: zeroMetrics(),
    failedTests: [],
    reportSource: "missing-artifact",
  };
}

/**
 * Picks a report date from the first report that exposes `startedAt`.
 *
 * @param {Array<Record<string, any>>} reports Available cluster reports.
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
 * @param {Array<Record<string, any>>} reports Reports to sort.
 * @param {string[]} preferredOrder Configured cluster order.
 * @returns {Array<Record<string, any>>} Sorted reports copy.
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

module.exports = {
  createMissingReport,
  getReportClusterKey,
  getReportDate,
  isClusterFailureReport,
  isMissingReport,
  isTestResultReport,
  sortReports,
};
