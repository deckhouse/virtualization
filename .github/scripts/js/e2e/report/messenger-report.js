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

const { listMatchingFiles } = require("./fs-utils");

const genericArtifactMissingLabel = "E2E REPORT ARTIFACT NOT FOUND";
const testReportsMissingLabel = "TEST REPORTS NOT FOUND";

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
 * @param {{
 *   reportKind?: string,
 *   failedStage?: string,
 *   failedStageLabel?: string,
 *   status?: string,
 *   branch?: string,
 *   workflowRunUrl?: string
 * }} [fallback={}] Optional fallback data propagated from workflow outputs.
 * @returns {Record<string, any>} Synthetic report payload.
 */
function createMissingReport(clusterName, fallback = {}) {
  const reportKind =
    fallback.reportKind && fallback.reportKind !== "tests"
      ? fallback.reportKind
      : "artifact-missing";
  const failedStage =
    fallback.failedStage && fallback.failedStage !== "success"
      ? fallback.failedStage
      : "artifact-missing";
  const failedStageLabel =
    fallback.failedStageLabel ||
    (fallback.reportKind === "artifact-missing"
      ? testReportsMissingLabel
      : genericArtifactMissingLabel);
  const status = fallback.status || "missing";

  return {
    cluster: clusterName,
    storageType: clusterName,
    reportKind,
    status,
    statusMessage: buildStatusMessage(status, failedStageLabel),
    failedStage,
    failedStageLabel,
    branch: fallback.branch || "",
    workflowRunUrl: fallback.workflowRunUrl || "",
    metrics: {
      passed: 0,
      failed: 0,
      errors: 0,
      skipped: 0,
      total: 0,
      successRate: 0,
    },
    failedTests: [],
  };
}

/**
 * Escapes markdown table cell content and normalizes whitespace.
 *
 * @param {any} value Raw cell value.
 * @returns {string} Sanitized table cell string.
 */
function sanitizeCell(value) {
  return String(value || "—")
    .replace(/\|/g, "\\|")
    .replace(/\r?\n/g, " ")
    .trim();
}

/**
 * Normalizes markdown list item content to a single trimmed line.
 *
 * @param {any} value Raw list item value.
 * @returns {string} Sanitized list item string.
 */
function sanitizeListItem(value) {
  return String(value || "")
    .replace(/\r?\n/g, " ")
    .trim();
}

/**
 * Formats a numeric success rate as a percentage string.
 *
 * @param {number|string} value Raw rate value.
 * @returns {string} Formatted percentage.
 */
function formatRate(value) {
  const rate = Number(value || 0);
  return `${Number.isFinite(rate) ? rate.toFixed(2) : "0.00"}%`;
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
 * Renders a cluster name as a markdown link when a workflow URL is available.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {string} Markdown link or plain sanitized cluster name.
 */
function formatClusterLink(report) {
  const clusterName = sanitizeCell(report.cluster || report.storageType);
  return report.workflowRunUrl
    ? `[${clusterName}](${report.workflowRunUrl})`
    : clusterName;
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
    report.reportKind === "artifact-missing" ||
    report.failedStage === "artifact-missing" ||
    report.status === "missing"
  );
}

/**
 * Normalizes the configured Loop API base URL to the `/api/v4/posts` endpoint.
 *
 * @param {string} value Raw Loop API base URL.
 * @returns {string} Normalized posts endpoint URL or an empty string.
 */
function normalizeLoopApiBaseUrl(value) {
  const trimmedValue = String(value || "")
    .trim()
    .replace(/\/+$/, "");

  if (!trimmedValue) {
    return "";
  }

  if (trimmedValue.endsWith("/api/v4/posts")) {
    return trimmedValue;
  }

  if (trimmedValue.endsWith("/api/v4")) {
    return `${trimmedValue}/posts`;
  }

  return `${trimmedValue}/api/v4/posts`;
}

/**
 * Reads and normalizes the Loop posts API URL from environment variables.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {string} Normalized posts endpoint URL or an empty string.
 */
function getLoopPostsApiUrl(env = process.env) {
  return normalizeLoopApiBaseUrl(env.LOOP_API_BASE_URL);
}

/**
 * Parses the configured cluster list passed via workflow environment variables.
 *
 * @param {string} value JSON-encoded cluster list.
 * @returns {string[]} Ordered cluster names.
 */
function parseConfiguredClusters(value) {
  const parsedValue = JSON.parse(value || "[]");
  return Array.isArray(parsedValue) ? parsedValue : [];
}

/**
 * Converts a cluster name into a safe environment-variable suffix.
 *
 * @param {string} clusterName Raw cluster name.
 * @returns {string} Uppercased normalized environment key fragment.
 */
function normalizeClusterEnvKey(clusterName) {
  return String(clusterName || "")
    .trim()
    .replace(/[^a-zA-Z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .toUpperCase();
}

/**
 * Reads per-cluster fallback values exported by reusable workflow jobs.
 *
 * @param {string[]} configuredClusters Clusters that should appear in the message.
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {Record<string, {
 *   reportKind: string,
 *   status: string,
 *   failedStage: string,
 *   failedStageLabel: string,
 *   workflowRunUrl: string,
 *   branch: string
 * }>} Fallbacks indexed by cluster name.
 */
function readReportFallbacksFromEnv(configuredClusters, env = process.env) {
  const fallbackByCluster = {};

  for (const clusterName of configuredClusters) {
    const clusterKey = normalizeClusterEnvKey(clusterName);
    const reportKind = env[`REPORT_FALLBACK_${clusterKey}_REPORT_KIND`] || "";
    const status = env[`REPORT_FALLBACK_${clusterKey}_STATUS`] || "";
    const failedStage = env[`REPORT_FALLBACK_${clusterKey}_FAILED_STAGE`] || "";
    const failedStageLabel =
      env[`REPORT_FALLBACK_${clusterKey}_FAILED_STAGE_LABEL`] || "";
    const workflowRunUrl =
      env[`REPORT_FALLBACK_${clusterKey}_WORKFLOW_RUN_URL`] || "";
    const branch = env[`REPORT_FALLBACK_${clusterKey}_BRANCH`] || "";

    if (
      reportKind ||
      status ||
      failedStage ||
      failedStageLabel ||
      workflowRunUrl ||
      branch
    ) {
      fallbackByCluster[clusterName] = {
        reportKind,
        status,
        failedStage,
        failedStageLabel,
        workflowRunUrl,
        branch,
      };
    }
  }

  return fallbackByCluster;
}

/**
 * Reads messenger configuration from the environment prepared by the workflow.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {{
 *   reportsDir: string,
 *   configuredClusters: string[],
 *   reportFallbacks: Record<string, any>,
 *   loop: {
 *     apiUrl: string,
 *     channelId: string,
 *     token: string
 *   }
 * }} Normalized messenger configuration.
 */
function readMessengerConfigFromEnv(env = process.env) {
  const configuredClusters = parseConfiguredClusters(env.STORAGE_TYPES);

  return {
    reportsDir: env.REPORTS_DIR || "downloaded-artifacts",
    configuredClusters,
    reportFallbacks: readReportFallbacksFromEnv(configuredClusters, env),
    loop: {
      apiUrl: getLoopPostsApiUrl(env),
      channelId: String(env.LOOP_CHANNEL_ID || "").trim(),
      token: String(env.LOOP_TOKEN || "").trim(),
    },
  };
}

/**
 * Parses a Loop API response body if it is JSON, otherwise returns an empty
 * object and emits a warning for diagnostics.
 *
 * @param {string} responseText Raw response body.
 * @param {{ warning(message: string): void }} core GitHub core API.
 * @returns {Record<string, any>} Parsed response payload or an empty object.
 */
function parseLoopApiPayload(responseText, core) {
  if (!responseText) {
    return {};
  }

  try {
    return JSON.parse(responseText);
  } catch (error) {
    core.warning(
      `Loop API returned a non-JSON response body: ${error.message}`
    );
    return {};
  }
}

/**
 * Sends a single post to Loop and returns the parsed API payload.
 *
 * @param {{
 *   apiUrl: string,
 *   channelId: string,
 *   token: string,
 *   message: string,
 *   rootId?: string
 * }} request Loop API request payload.
 * @param {{
 *   info(message: string): void,
 *   warning(message: string): void
 * }} core GitHub core API.
 * @returns {Promise<Record<string, any>>} Parsed Loop API response.
 */
async function postToLoopApi(
  { apiUrl, channelId, token, message, rootId },
  core
) {
  const response = await fetch(apiUrl, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      channel_id: channelId,
      message,
      ...(rootId ? { root_id: rootId } : {}),
    }),
  });
  const responseText = await response.text();

  if (!response.ok) {
    throw new Error(
      `Loop API request failed with status ${response.status}: ${responseText}`
    );
  }

  const payload = parseLoopApiPayload(responseText, core);
  core.info(`Loop API accepted report with status ${response.status}`);
  return payload;
}

/**
 * Loads report JSON files from disk and injects synthetic reports for clusters
 * whose artifacts are missing.
 *
 * @param {string} reportsDir Directory containing `e2e_report_*.json`.
 * @param {string[]} configuredClusters Clusters expected in the final report.
 * @param {Record<string, any>} reportFallbacks Fallback data by cluster.
 * @param {{ warning(message: string): void }} core GitHub core API.
 * @returns {Record<string, any>[]} Ordered cluster reports.
 */
function readReports(reportsDir, configuredClusters, reportFallbacks, core) {
  const reportFiles = listMatchingFiles(reportsDir, /^e2e_report_.*\.json$/);
  const reports = [];

  for (const reportFile of reportFiles) {
    try {
      reports.push(JSON.parse(fs.readFileSync(reportFile, "utf8")));
    } catch (error) {
      core.warning(`Unable to parse ${reportFile}: ${error.message}`);
    }
  }

  const reportsByCluster = new Map();
  for (const report of reports) {
    const clusterName = getReportClusterKey(report);
    if (!clusterName) {
      core.warning(
        `Skipping report without cluster name from ${report.sourceReport || "parsed JSON payload"}`
      );
      continue;
    }

    reportsByCluster.set(clusterName, report);
  }

  for (const clusterName of configuredClusters) {
    if (!reportsByCluster.has(clusterName)) {
      reportsByCluster.set(
        clusterName,
        createMissingReport(clusterName, reportFallbacks[clusterName])
      );
    }
  }

  const orderedReports = sortReports(
    Array.from(reportsByCluster.values()),
    configuredClusters
  );
  return orderedReports;
}

/**
 * Renders the top-level messenger markdown message.
 *
 * @param {Record<string, any>[]} orderedReports Reports ordered for display.
 * @returns {string} Main markdown message.
 */
function buildMainMessage(orderedReports) {
  const reportDate = getReportDate(orderedReports);
  const branches = Array.from(
    new Set(orderedReports.map((report) => report.branch).filter(Boolean))
  );
  const lines = [`## DVP | E2E on nested clusters | ${reportDate}`, ""];

  if (branches.length === 1 && branches[0] !== "main") {
    lines.push(`Branch: \`${branches[0]}\``);
    lines.push("");
  }

  const testsReports = orderedReports.filter(
    (report) => report.reportKind === "tests" && getReportClusterKey(report)
  );
  const nonTestReports = orderedReports.filter(
    (report) => report.reportKind !== "tests" && getReportClusterKey(report)
  );
  const stageFailureReports = nonTestReports.filter(
    (report) => !isMissingReport(report)
  );
  const missingReports = nonTestReports.filter((report) => isMissingReport(report));

  if (testsReports.length > 0) {
    lines.push("### Test results");
    lines.push("");
    lines.push(
      "| Cluster | ✅ Passed | ⏭️ Skipped | ❌ Failed | ⚠️ Errors | Total | Success Rate |"
    );
    lines.push("|---|---:|---:|---:|---:|---:|---:|");

    for (const report of testsReports) {
      const metrics = report.metrics || {};
      lines.push(
        `| ${formatClusterLink(report)} | ${metrics.passed || 0} | ${
          metrics.skipped || 0
        } | ${metrics.failed || 0} | ${metrics.errors || 0} | ${
          metrics.total || 0
        } | ${formatRate(metrics.successRate)} |`
      );
    }

    lines.push("");
  }

  if (stageFailureReports.length > 0) {
    lines.push("### Cluster failures");
    lines.push("");

    for (const report of stageFailureReports) {
      lines.push(
        `- ${formatClusterLink(report)}: ${sanitizeListItem(
          report.failedStageLabel || report.statusMessage || report.failedStage
        )}`
      );
    }

    lines.push("");
  }

  if (missingReports.length > 0) {
    lines.push("### Missing reports");
    lines.push("");

    for (const report of missingReports) {
      lines.push(
        `- ${formatClusterLink(report)}: ${sanitizeListItem(
          report.failedStageLabel || report.statusMessage || report.failedStage
        )}`
      );
    }

    lines.push("");
  }

  return lines.join("\n").trim();
}

/**
 * Tells whether the report should contribute failed-test details to the thread.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {boolean} True when failed-test details should be rendered.
 */
function hasFailedTests(report) {
  if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
    return true;
  }

  return Boolean(
    (report.metrics && report.metrics.failed) ||
      (report.metrics && report.metrics.errors)
  );
}

/**
 * Extracts the top-level test group name from a failed test title.
 *
 * For Ginkgo titles like `[It] VirtualMachineOperationRestore restores ...`,
 * this returns `VirtualMachineOperationRestore`.
 *
 * @param {string} testName Full failed test name.
 * @returns {string} Top-level test group label.
 */
function getFailedTestGroupName(testName) {
  const normalizedName = sanitizeListItem(testName).replace(/^\[[^\]]+\]\s*/, "");
  const [groupName] = normalizedName.split(/\s+/, 1);
  return groupName || "Unknown";
}

/**
 * Aggregates failed test names into an ordered unique group list.
 *
 * @param {string[]} failedTests Failed testcase names.
 * @returns {string[]} Ordered unique group names.
 */
function summarizeFailedTestGroups(failedTests) {
  const groupNames = [];

  for (const testName of failedTests) {
    const groupName = getFailedTestGroupName(testName);
    if (!groupNames.includes(groupName)) {
      groupNames.push(groupName);
    }
  }

  return groupNames;
}

/**
 * Builds the thread reply body for a single cluster with failed tests.
 *
 * @param {Record<string, any>} report Cluster report payload.
 * @returns {string} Cluster-specific failed tests markdown.
 */
function buildFailedTestsClusterMessage(report) {
  const clusterName = sanitizeListItem(report.cluster || report.storageType);
  const lines = [`**${clusterName}**`];

  if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
    const failedGroups = summarizeFailedTestGroups(report.failedTests);
    lines.push("");
    lines.push("| Test group |");
    lines.push("|---|");
    for (const groupName of failedGroups) {
      lines.push(`| ${sanitizeCell(groupName)} |`);
    }
  } else {
    lines.push(
      "- No testcase-level failures were collected, but the E2E stage reported failures."
    );
  }

  return lines.join("\n");
}

/**
 * Renders thread markdown messages containing failed test names, if any.
 *
 * @param {Record<string, any>[]} orderedReports Reports ordered for display.
 * @returns {string[]} Thread markdown messages in publish order.
 */
function buildThreadMessages(orderedReports) {
  const testsReports = orderedReports.filter(
    (report) => report.reportKind === "tests"
  );
  const failedTestReports = testsReports.filter(hasFailedTests);

  if (failedTestReports.length === 0) {
    return [];
  }

  return [
    "### Failed tests",
    ...failedTestReports.map(buildFailedTestsClusterMessage),
  ];
}

/**
 * Reads cluster reports from disk and builds both messenger message bodies.
 *
 * @param {{
 *   reportsDir: string,
 *   configuredClusters: string[],
 *   reportFallbacks: Record<string, any>,
 *   core: { warning(message: string): void }
 * }} params Message rendering inputs.
 * @returns {{
 *   message: string,
 *   threadMessage: string,
 *   threadMessages: string[]
 * }} Rendered markdown payloads.
 */
function buildMessengerMessages({
  reportsDir,
  configuredClusters,
  reportFallbacks,
  core,
}) {
  const orderedReports = readReports(
    reportsDir,
    configuredClusters,
    reportFallbacks,
    core
  );
  const threadMessages = buildThreadMessages(orderedReports);
  return {
    message: buildMainMessage(orderedReports),
    threadMessage: threadMessages.join("\n\n"),
    threadMessages,
  };
}

/**
 * Publishes the main report and optional failed-tests thread to Loop.
 *
 * @param {{
 *   message: string,
 *   threadMessages: string[],
 *   loop: {
 *     apiUrl: string,
 *     channelId: string,
 *     token: string
 *   }
 * }} params Message payload and Loop credentials.
 * @param {{
 *   setOutput(name: string, value: string): void,
 *   info(message: string): void,
 *   warning(message: string): void
 * }} core GitHub core API.
 * @returns {Promise<void>}
 */
async function publishToLoop({ message, threadMessages, loop }, core) {
  if (!loop.apiUrl && !loop.channelId && !loop.token) {
    return;
  }

  if (!loop.apiUrl || !loop.channelId || !loop.token) {
    throw new Error(
      "LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required"
    );
  }

  const rootPost = await postToLoopApi(
    {
      apiUrl: loop.apiUrl,
      channelId: loop.channelId,
      token: loop.token,
      message,
    },
    core
  );

  let lastReplyPost = null;
  for (const replyMessage of threadMessages) {
    lastReplyPost = await postToLoopApi(
      {
        apiUrl: loop.apiUrl,
        channelId: loop.channelId,
        token: loop.token,
        message: replyMessage,
        rootId: rootPost.id,
      },
      core
    );
  }

  core.setOutput("root_post_id", rootPost.id || "");
  core.setOutput(
    "thread_post_id",
    lastReplyPost && lastReplyPost.id ? lastReplyPost.id : ""
  );
}

/**
 * Entry point used by `actions/github-script` to render and optionally publish
 * the aggregated E2E messenger report.
 *
 * @param {{
 *   core: {
 *     info(message: string): void,
 *     warning(message: string): void,
 *     setOutput(name: string, value: string): void
 *   }
 * }} params GitHub script dependencies.
 * @returns {Promise<{
 *   message: string,
 *   threadMessage: string,
 *   threadMessages: string[]
 * }>} Rendered messages.
 */
async function renderMessengerReport({ core }) {
  const config = readMessengerConfigFromEnv();
  const { message, threadMessage, threadMessages } = buildMessengerMessages({
    reportsDir: config.reportsDir,
    configuredClusters: config.configuredClusters,
    reportFallbacks: config.reportFallbacks,
    core,
  });

  core.info(message);
  core.setOutput("message", message);
  core.setOutput("thread_message", threadMessage);
  core.setOutput("thread_messages", JSON.stringify(threadMessages));

  try {
    await publishToLoop(
      { message, threadMessages, loop: config.loop },
      core
    );
  } catch (error) {
    core.warning(`Unable to deliver report to Loop API: ${error.message}`);
  }

  return { message, threadMessage, threadMessages };
}

module.exports = renderMessengerReport;
module.exports.createMissingReport = createMissingReport;
module.exports.buildMessengerMessages = buildMessengerMessages;
module.exports.getLoopPostsApiUrl = getLoopPostsApiUrl;
module.exports.readReportFallbacksFromEnv = readReportFallbacksFromEnv;
module.exports.readMessengerConfigFromEnv = readMessengerConfigFromEnv;
