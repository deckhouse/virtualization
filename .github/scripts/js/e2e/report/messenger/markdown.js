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

/**
 * @typedef {Object} TestResultsColumn
 * @property {string} header Column header text rendered in the markdown row.
 * @property {string} align Column alignment for the markdown separator row
 *   (for example, "---" or "---:").
 * @property {function(Record<string, any>, Record<string, any>): (string|number)} value
 *   Cell renderer that receives the cluster report and its metrics object.
 */

/**
 * Builds the column descriptors for the test-results markdown table.
 *
 * Each column declares its header, its alignment for the markdown
 * separator row, and how to render the value for a cluster. The "Errors"
 * column is included only when at least one cluster reported Ginkgo
 * errors, so successful runs stay compact.
 *
 * @param {boolean} hasGinkgoErrors Whether any cluster reported Ginkgo errors.
 * @returns {TestResultsColumn[]} Ordered list of columns to render.
 */
function buildTestResultsColumns(hasGinkgoErrors) {
  const columns = [
    {
      header: ":dvp: Cluster",
      align: "---",
      value: (report) => formatClusterLink(report),
    },
    {
      header: "✅ Passed",
      align: "---:",
      value: (_report, metrics) => metrics.passed || 0,
    },
    {
      header: "⏭️ Skipped",
      align: "---:",
      value: (_report, metrics) => metrics.skipped || 0,
    },
    {
      header: "❌ Failed",
      align: "---:",
      value: (_report, metrics) => metrics.failed || 0,
    },
  ];

  if (hasGinkgoErrors) {
    columns.push({
      header: "⚠️ Errors",
      align: "---:",
      value: (_report, metrics) => metrics.errors || 0,
    });
  }

  columns.push(
    {
      header: "📊 Total",
      align: "---:",
      value: (_report, metrics) => metrics.total || 0,
    },
    {
      header: "📈 Success Rate",
      align: "---:",
      value: (_report, metrics) => formatRate(metrics.successRate),
    }
  );

  return columns;
}

/**
 * Joins a list of cells into a single markdown table row.
 *
 * @param {Array<string|number>} cells Ordered cell values for one row.
 * @returns {string} Markdown row string framed with pipe characters.
 */
function buildMarkdownRow(cells) {
  return `| ${cells.join(" | ")} |`;
}

function renderTestResultsSection(testsReports) {
  if (testsReports.length === 0) {
    return [];
  }

  const hasGinkgoErrors = testsReports.some(
    (report) => Number((report.metrics || {}).errors || 0) > 0
  );
  const columns = buildTestResultsColumns(hasGinkgoErrors);
  const rows = [
    buildMarkdownRow(columns.map((column) => column.header)),
    buildMarkdownRow(columns.map((column) => column.align)),
  ];

  for (const report of testsReports) {
    const metrics = report.metrics || {};
    rows.push(
      buildMarkdownRow(columns.map((column) => column.value(report, metrics)))
    );
  }

  return ["### Test results", "", ...rows, ""];
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
  const sanitizedName = sanitizeListItem(testName);
  const leadingTagMatch = sanitizedName.match(/^\[([^\]]+)\]\s*(.*)$/);
  const leadingTag = leadingTagMatch ? leadingTagMatch[1].trim() : "";
  const remainder = leadingTagMatch ? leadingTagMatch[2].trim() : sanitizedName;

  // Suite-level entries such as "[SynchronizedBeforeSuite]" or
  // "[SynchronizedAfterSuite]" have no body after the leading tag.
  // In that case the tag itself is the most informative group name.
  if (!remainder) {
    return leadingTag || "Unknown";
  }

  const [groupName] = remainder.split(/\s+/, 1);
  return groupName || leadingTag || "Unknown";
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

/**
 * Aggregates failed test entries from a cluster report into a deduplicated
 * list of "group" rows for the failed-tests thread message. Tests are
 * grouped by the first word of their leaf node text (or by the leading
 * Ginkgo tag for suite-level failures); reasons for the same group are
 * deduplicated and joined with "; ".
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {Array<{name: string, reason: string}>} Group rows for the thread table.
 */
function summarizeFailedTestGroups(report) {
  const groups = new Map();

  for (const test of getFailedTestEntries(report)) {
    const groupName = getFailedTestGroupName(test.name);
    const reason = sanitizeListItem(test.reason) || "—";
    if (!groups.has(groupName)) {
      groups.set(groupName, { reasons: new Set() });
    }
    groups.get(groupName).reasons.add(reason);
  }

  return Array.from(groups, ([name, group]) => ({
    name,
    reason: Array.from(group.reasons).join("; "),
  }));
}

function renderFailedTestsThreadMessage(report) {
  const lines = [`**${formatClusterLink(report)}**`];

  if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
    const failedGroups = summarizeFailedTestGroups(report);
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
