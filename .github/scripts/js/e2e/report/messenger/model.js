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
  const missingStatusMessage = buildStatusMessage("missing", genericArtifactMissingLabel);
  const clusterStatus = {
    status: "missing",
    stage: "artifact-missing",
    stageLabel: genericArtifactMissingLabel,
    message: missingStatusMessage,
    reason: "cluster-report-artifact-missing",
  };
  const testStatus = {
    status: "not-run",
    reason: "cluster-report-artifact-missing",
    message: "E2E status is unavailable because cluster report artifact was not found",
  };

  return {
    schemaVersion: 1,
    cluster: clusterName,
    storageType: clusterName,
    reportKind: "artifact-missing",
    status: "missing",
    statusMessage: missingStatusMessage,
    failedStage: "artifact-missing",
    failedStageLabel: genericArtifactMissingLabel,
    branch: "",
    workflowRunUrl: "",
    clusterStatus,
    testStatus,
    metrics: zeroMetrics(),
    failedTests: [],
    failedTestDetails: [],
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
};
