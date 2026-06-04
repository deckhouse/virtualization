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
  return report.workflowRunUrl ? `[${clusterName}](${report.workflowRunUrl})` : clusterName;
}

function splitReportsBySection(orderedReports) {
  const reports = orderedReports.filter(getReportClusterKey);

  return {
    testsReports: reports.filter(isTestResultReport),
    stageFailureReports: reports.filter(isClusterFailureReport),
    missingReports: reports.filter((report) => isMissingReport(report) && !isClusterFailureReport(report)),
  };
}

function renderBranchLine(orderedReports) {
  const branches = Array.from(new Set(orderedReports.map((report) => report.branch).filter(Boolean)));

  return branches.length === 1 && branches[0] !== "main" ? [`Branch: \`${branches[0]}\``, ""] : [];
}

function renderTestResultsSection(testsReports) {
  if (testsReports.length === 0) {
    return [];
  }

  const hasGinkgoErrors = testsReports.some((report) => Number((report.metrics || {}).errors || 0) > 0);
  const headerCells = [
    ":dvp: Cluster",
    "✅ Passed",
    "⏭️ Skipped",
    "❌ Failed",
    ...(hasGinkgoErrors ? ["⚠️ Errors"] : []),
    "📊 Total",
    "📈 Success Rate",
  ];
  const alignCells = ["---", "---:", "---:", "---:", ...(hasGinkgoErrors ? ["---:"] : []), "---:", "---:"];
  const row = (cells) => `| ${cells.join(" | ")} |`;

  const rows = [row(headerCells), row(alignCells)];

  for (const report of testsReports) {
    const metrics = report.metrics || {};
    rows.push(
      row([
        formatClusterLink(report),
        metrics.passed || 0,
        metrics.skipped || 0,
        metrics.failed || 0,
        ...(hasGinkgoErrors ? [metrics.errors || 0] : []),
        metrics.total || 0,
        formatRate(metrics.successRate),
      ])
    );
  }

  return ["### Test results", "", ...rows, ""];
}

/**
 * Renders a `### <title>` section followed by a bullet list of
 * `- <cluster link>: <message>` rows, one per report. Returns an empty
 * array when there are no reports so callers can spread the result into
 * the main message without conditional logic.
 *
 * @param {string} title Section heading text (without the leading `### `).
 * @param {Array<Record<string, any>>} reports Reports to render.
 * @param {function(Record<string, any>): string} getMessage Extracts the
 *   per-cluster message string from a report.
 * @returns {string[]} Markdown lines for the section.
 */
function renderBulletSection(title, reports, getMessage) {
  if (reports.length === 0) {
    return [];
  }

  const bullets = reports.map((report) => `- ${formatClusterLink(report)}: ${sanitizeListItem(getMessage(report))}`);

  return [`### ${title}`, "", ...bullets, ""];
}

function getClusterFailureMessage(report) {
  return (
    (report.clusterStatus && report.clusterStatus.message) ||
    report.statusMessage ||
    report.failedStageLabel ||
    report.failedStage
  );
}

function getMissingReportMessage(report) {
  return (
    report.statusMessage ||
    (report.testStatus && report.testStatus.message) ||
    (report.clusterStatus && report.clusterStatus.message) ||
    report.failedStageLabel
  );
}

/**
 * Builds the main E2E messenger report body.
 *
 * @param {Array<Record<string, any>>} orderedReports Cluster reports in display order.
 * @returns {string} Markdown message body.
 */
function buildMainMessage(orderedReports) {
  const reportDate = getReportDate(orderedReports);
  const { testsReports, stageFailureReports, missingReports } = splitReportsBySection(orderedReports);
  const lines = [
    `## :dvp: DVP | E2E on nested clusters | ${reportDate}`,
    "",
    ...renderBranchLine(orderedReports),
    ...renderBulletSection("Cluster failures", stageFailureReports, getClusterFailureMessage),
    ...renderBulletSection("Missing reports", missingReports, getMissingReportMessage),
    ...renderTestResultsSection(testsReports),
  ];

  return lines.join("\n").trim();
}

function hasFailedTests(report) {
  if (Array.isArray(report.failedTestDetails) && report.failedTestDetails.length > 0) {
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
  return Array.isArray(report.failedTestDetails) ? report.failedTestDetails : [];
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

  if (Array.isArray(report.failedTestDetails) && report.failedTestDetails.length > 0) {
    const failedGroups = summarizeFailedTestGroups(report);
    lines.push("");
    lines.push("| Tests | Reason |");
    lines.push("|---|---|");
    for (const group of failedGroups) {
      lines.push(`| ${sanitizeCell(group.name)} | ${sanitizeCell(group.reason)} |`);
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

function hasSpecTimings(report) {
  return Array.isArray(report.specTimings) && report.specTimings.length > 0;
}

function buildChartCaption(_files, chartsUnavailable) {
  return chartsUnavailable ? "Charts unavailable." : "";
}

/**
 * Builds optional per-cluster thread messages for failed tests and chart attachments.
 *
 * @param {Array<Record<string, any>>} orderedReports Cluster reports in display order.
 * @param {{
 *   getClusterChartFiles?: function(Record<string, any>): Promise<Array<{name: string, buffer: Buffer, mimeType: string}>>,
 *   core?: {warning?: function(string): void}
 * }} [options]
 * @returns {Promise<Array<{message: string, files: Array<{name: string, buffer: Buffer, mimeType: string}>}>>} Markdown thread payloads.
 */
async function buildThreadMessages(orderedReports, { getClusterChartFiles, core } = {}) {
  const testsReports = orderedReports.filter((report) => isTestResultReport(report));
  const threadMessages = [];
  let renderedFailedTestsHeading = false;

  for (const report of testsReports) {
    const messageParts = [];
    let files = [];
    let chartsUnavailable = false;

    if (getClusterChartFiles && hasSpecTimings(report)) {
      try {
        files = await getClusterChartFiles(report);
      } catch (error) {
        chartsUnavailable = true;
        if (core && typeof core.warning === "function") {
          core.warning(
            `Unable to prepare duration chart files for cluster ${getReportClusterKey(report) || "unknown"}: ${
              error.message
            }`
          );
        }
      }
    }

    if (!hasFailedTests(report) && files.length === 0 && !chartsUnavailable) {
      continue;
    }

    if (hasFailedTests(report)) {
      const clusterMessage = renderFailedTestsThreadMessage(report);
      messageParts.push(
        renderedFailedTestsHeading ? clusterMessage : ["### Failed tests", clusterMessage].join("\n\n")
      );
      renderedFailedTestsHeading = true;
    } else {
      messageParts.push(`**${formatClusterLink(report)}**`);
    }

    const chartCaption = buildChartCaption(files, chartsUnavailable);
    if (chartCaption) {
      messageParts.push(chartCaption);
    }

    threadMessages.push({
      message: messageParts.join("\n\n"),
      files,
    });
  }

  return threadMessages;
}

module.exports = {
  buildMainMessage,
  buildThreadMessages,
};
