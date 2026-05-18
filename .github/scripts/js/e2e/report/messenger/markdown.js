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

function formatDuration(runtimeMs) {
  const durationSeconds = Number(runtimeMs || 0) / 1000;
  if (durationSeconds < 60) {
    return `${durationSeconds.toFixed(1)}s`;
  }

  const minutes = Math.floor(durationSeconds / 60);
  const seconds = Math.round(durationSeconds % 60);
  return `${minutes}m ${seconds}s`;
}

function formatClusterLink(report) {
  const clusterName = sanitizeCell(report.cluster || report.storageType);
  return report.workflowRunUrl
    ? `[${clusterName}](${report.workflowRunUrl})`
    : clusterName;
}

function splitReportsBySection(orderedReports) {
  const reports = orderedReports.filter(getReportClusterKey);

  return {
    testsReports: reports.filter(isTestResultReport),
    stageFailureReports: reports.filter(isClusterFailureReport),
    missingReports: reports.filter(
      (report) => isMissingReport(report) && !isClusterFailureReport(report)
    ),
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
 * The "Errors" column is included only when at least one cluster reported
 * Ginkgo errors, so successful runs stay compact.
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

function renderDurationBar(runtimeMs, maxRuntimeMs, width = 10) {
  if (!maxRuntimeMs) {
    return "";
  }

  const filled = Math.max(
    1,
    Math.round((Number(runtimeMs || 0) / maxRuntimeMs) * width)
  );
  return `${"█".repeat(Math.min(width, filled))}${"░".repeat(
    Math.max(0, width - filled)
  )}`;
}

function renderTopSlowestSection(testsReports) {
  const rows = [];

  for (const report of testsReports) {
    const timings = Array.isArray(report.specTimings) ? report.specTimings : [];
    const topTimings = timings
      .slice()
      .sort(
        (left, right) =>
          Number(right.runtimeMs || 0) - Number(left.runtimeMs || 0) ||
          String(left.name || "").localeCompare(String(right.name || ""))
      )
      .slice(0, 3);

    for (const timing of topTimings) {
      rows.push({ report, timing });
    }
  }

  if (rows.length === 0) {
    return [];
  }

  const maxRuntimeMs = Math.max(
    ...rows.map(({ timing }) => Number(timing.runtimeMs || 0))
  );
  const tableRows = [
    "| Cluster | Test | Duration | Bar |",
    "|---|---|---:|---|",
  ];

  for (const { report, timing } of rows) {
    tableRows.push(
      buildMarkdownRow([
        formatClusterLink(report),
        sanitizeCell(timing.name),
        formatDuration(timing.runtimeMs),
        renderDurationBar(timing.runtimeMs, maxRuntimeMs),
      ])
    );
  }

  return ["### Top slowest tests", "", ...tableRows, ""];
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

  const bullets = reports.map(
    (report) =>
      `- ${formatClusterLink(report)}: ${sanitizeListItem(getMessage(report))}`
  );

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
  const { testsReports, stageFailureReports, missingReports } =
    splitReportsBySection(orderedReports);
  const lines = [
    `## :dvp: DVP | E2E on nested clusters | ${reportDate}`,
    "",
    ...renderBranchLine(orderedReports),
    ...renderBulletSection(
      "Cluster failures",
      stageFailureReports,
      getClusterFailureMessage
    ),
    ...renderBulletSection(
      "Missing reports",
      missingReports,
      getMissingReportMessage
    ),
    ...renderTestResultsSection(testsReports),
    ...renderTopSlowestSection(testsReports),
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

function hasSpecTimings(report) {
  return Array.isArray(report.specTimings) && report.specTimings.length > 0;
}

function renderChartCaption(files, chartsUnavailable) {
  if (files.length === 0 && !chartsUnavailable) {
    return "";
  }

  const lines = ["### Test durations"];
  if (files.length > 0) {
    lines.push("");
    lines.push("Attached charts:");
    lines.push("- Top slowest specs");
    lines.push("- Duration distribution");
    lines.push("- Total duration by feature");
    lines.push("- Duration by feature and status");
  }
  if (chartsUnavailable) {
    lines.push("");
    lines.push("Charts unavailable.");
  }

  return lines.join("\n");
}

/**
 * Builds optional per-cluster thread messages for failed tests and duration charts.
 *
 * @param {Array<Record<string, any>>} orderedReports Cluster reports in display order.
 * @param {{
 *   renderClusterCharts?: function(Record<string, any>): Promise<Array<{name: string, buffer: Buffer, mimeType: string}>>,
 *   core?: {warning?: function(string): void}
 * }} [options]
 * @returns {Promise<Array<{message: string, files: Array<{name: string, buffer: Buffer, mimeType: string}>}>>} Markdown thread payloads.
 */
async function buildThreadMessages(
  orderedReports,
  { renderClusterCharts, core } = {}
) {
  const testsReports = orderedReports.filter((report) =>
    isTestResultReport(report)
  );
  const threadMessages = [];
  let renderedFailedTestsHeading = false;

  for (const report of testsReports) {
    const messageParts = [];
    let files = [];
    let chartsUnavailable = false;

    if (renderClusterCharts && hasSpecTimings(report)) {
      try {
        files = await renderClusterCharts(report);
      } catch (error) {
        chartsUnavailable = true;
        if (core && typeof core.warning === "function") {
          core.warning(
            `Unable to render duration charts for cluster ${
              getReportClusterKey(report) || "unknown"
            }: ${error.message}`
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
        renderedFailedTestsHeading
          ? clusterMessage
          : ["### Failed tests", clusterMessage].join("\n\n")
      );
      renderedFailedTestsHeading = true;
    } else {
      messageParts.push(`**${formatClusterLink(report)}**`);
    }

    const chartCaption = renderChartCaption(files, chartsUnavailable);
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
