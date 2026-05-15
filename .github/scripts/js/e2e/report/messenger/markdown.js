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
  getReportClusterKey,
  getReportDate,
  isClusterFailureReport,
  isMissingReport,
  isTestResultReport,
} = require("./model");

function sanitizeCell(value) {
  return String(value || "—")
    .replace(/\|/g, "\\|")
    .replace(/\r?\n/g, " ")
    .trim();
}

function sanitizeListItem(value) {
  return String(value || "")
    .replace(/\r?\n/g, " ")
    .trim();
}

function formatRate(value) {
  const rate = Number(value || 0);
  return `${Number.isFinite(rate) ? rate.toFixed(2) : "0.00"}%`;
}

function formatClusterLink(report) {
  const clusterName = sanitizeCell(report.cluster || report.storageType);
  return report.workflowRunUrl
    ? `[${clusterName}](${report.workflowRunUrl})`
    : clusterName;
}

function splitReportsBySection(orderedReports) {
  const testsReports = orderedReports.filter(
    (report) => isTestResultReport(report) && getReportClusterKey(report)
  );
  const stageFailureReports = orderedReports.filter(
    (report) => isClusterFailureReport(report) && getReportClusterKey(report)
  );
  const missingReports = orderedReports.filter(
    (report) =>
      isMissingReport(report) &&
      !isClusterFailureReport(report) &&
      getReportClusterKey(report)
  );

  return {
    testsReports,
    stageFailureReports,
    missingReports,
  };
}

function renderBranchLine(orderedReports) {
  const branches = Array.from(
    new Set(orderedReports.map((report) => report.branch).filter(Boolean))
  );

  return branches.length === 1 && branches[0] !== "main"
    ? [`Branch: \`${branches[0]}\``, ""]
    : [];
}

function renderTestResultsSection(testsReports) {
  const lines = [];

  if (testsReports.length > 0) {
    const hasGinkgoErrors = testsReports.some(
      (report) => Number((report.metrics || {}).errors || 0) > 0
    );

    lines.push("### Test results");
    lines.push("");
    lines.push(
      hasGinkgoErrors
        ? "| Cluster | ✅ Passed | ⏭️ Skipped | ❌ Failed | ⚠️ Errors | Total | Success Rate |"
        : "| Cluster | ✅ Passed | ⏭️ Skipped | ❌ Failed | Total | Success Rate |"
    );
    lines.push(
      hasGinkgoErrors
        ? "|---|---:|---:|---:|---:|---:|---:|"
        : "|---|---:|---:|---:|---:|---:|"
    );

    for (const report of testsReports) {
      const metrics = report.metrics || {};
      lines.push(
        hasGinkgoErrors
          ? `| ${formatClusterLink(report)} | ${metrics.passed || 0} | ${
              metrics.skipped || 0
            } | ${metrics.failed || 0} | ${metrics.errors || 0} | ${
              metrics.total || 0
            } | ${formatRate(metrics.successRate)} |`
          : `| ${formatClusterLink(report)} | ${metrics.passed || 0} | ${
              metrics.skipped || 0
            } | ${metrics.failed || 0} | ${metrics.total || 0} | ${formatRate(
              metrics.successRate
            )} |`
      );
    }

    lines.push("");
  }

  return lines;
}

function renderClusterFailuresSection(stageFailureReports) {
  const lines = [];

  if (stageFailureReports.length > 0) {
    lines.push("### Cluster failures");
    lines.push("");

    for (const report of stageFailureReports) {
      lines.push(
        `- ${formatClusterLink(report)}: ${sanitizeListItem(
          (report.clusterStatus && report.clusterStatus.message) ||
            report.statusMessage ||
            report.failedStageLabel ||
            report.failedStage
        )}`
      );
    }

    lines.push("");
  }

  return lines;
}

function renderMissingReportsSection(missingReports) {
  const lines = [];

  if (missingReports.length > 0) {
    lines.push("### Missing reports");
    lines.push("");

    for (const report of missingReports) {
      lines.push(
        `- ${formatClusterLink(report)}: ${sanitizeListItem(
          report.statusMessage ||
            (report.testStatus && report.testStatus.message) ||
            (report.clusterStatus && report.clusterStatus.message) ||
            report.failedStageLabel
        )}`
      );
    }

    lines.push("");
  }

  return lines;
}

/**
 * Builds the main E2E messenger report body.
 *
 * @param {Array<Record<string, any>>} orderedReports Cluster reports in display order.
 * @returns {string} Markdown message body.
 */
function buildMainMessage(orderedReports) {
  const reportDate = getReportDate(orderedReports);
  const { testsReports, stageFailureReports, missingReports } =
    splitReportsBySection(orderedReports);
  const lines = [
    `## :dvp: DVP | E2E on nested clusters | ${reportDate}`,
    "",
    ...renderBranchLine(orderedReports),
    ...renderClusterFailuresSection(stageFailureReports),
    ...renderMissingReportsSection(missingReports),
    ...renderTestResultsSection(testsReports),
  ];

  return lines.join("\n").trim();
}

function hasFailedTests(report) {
  if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
    return true;
  }

  return Boolean(
    (report.testStatus && report.testStatus.status === "failure") ||
      (report.metrics && report.metrics.failed) ||
      (report.metrics && report.metrics.errors)
  );
}

function getFailedTestGroupName(testName) {
  const normalizedName = sanitizeListItem(testName).replace(
    /^\[[^\]]+\]\s*/,
    ""
  );
  const [groupName] = normalizedName.split(/\s+/, 1);
  return groupName || "Unknown";
}

function getFailedTestEntries(report) {
  if (
    Array.isArray(report.failedTestDetails) &&
    report.failedTestDetails.length > 0
  ) {
    return report.failedTestDetails.map((test) => ({
      name: test.name,
      reason: test.reason,
    }));
  }

  return (report.failedTests || []).map((testName) => ({
    name: testName,
    reason: "",
  }));
}

function summarizeFailedTestGroups(failedTestEntries) {
  const groups = new Map();

  for (const test of failedTestEntries) {
    const groupName = getFailedTestGroupName(test.name);
    const reason = sanitizeListItem(test.reason) || "—";
    if (!groups.has(groupName)) {
      groups.set(groupName, new Set());
    }
    groups.get(groupName).add(reason);
  }

  return Array.from(groups, ([name, reasons]) => ({
    name,
    reason: Array.from(reasons).join("<br>"),
  }));
}

function renderFailedTestsThreadMessage(report) {
  const clusterName = sanitizeListItem(report.cluster || report.storageType);
  const lines = [`**${clusterName}**`];

  if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
    const failedGroups = summarizeFailedTestGroups(
      getFailedTestEntries(report)
    );
    lines.push("");
    lines.push("| Tests | Reason |");
    lines.push("|---|---|");
    for (const group of failedGroups) {
      lines.push(
        `| ${sanitizeCell(group.name)} | ${sanitizeCell(group.reason)} |`
      );
    }
  } else {
    lines.push(
      `- ${
        sanitizeListItem(report.testStatus && report.testStatus.message) ||
        "No testcase-level failures were collected, but the E2E stage reported failures."
      }`
    );
  }

  return lines.join("\n");
}

/**
 * Builds optional failed-tests thread messages for clusters with failed tests.
 *
 * @param {Array<Record<string, any>>} orderedReports Cluster reports in display order.
 * @returns {string[]} Markdown thread message bodies.
 */
function buildThreadMessages(orderedReports) {
  const testsReports = orderedReports.filter((report) =>
    isTestResultReport(report)
  );
  const failedTestReports = testsReports.filter(hasFailedTests);

  if (failedTestReports.length === 0) {
    return [];
  }

  return failedTestReports.map((report, index) => {
    const clusterMessage = renderFailedTestsThreadMessage(report);
    return index === 0
      ? ["### Failed tests", clusterMessage].join("\n\n")
      : clusterMessage;
  });
}

module.exports = {
  buildMainMessage,
  buildThreadMessages,
};
